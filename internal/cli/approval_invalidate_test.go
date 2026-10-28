package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/planflow"
	"github.com/imhttran/agentic-sop/internal/store"
)

// invalidationFixture seeds a project with one task, a persisted plan stage, plan
// provenance, and a PENDING approval head, so the invalidation command and the
// execution gate can be exercised in-process without a provider or a model.
type invalidationFixture struct {
	dir     string
	runDir  string
	planFP  string
	metaGen time.Time
}

type invalidationFixtureOptions struct {
	taskStatus domain.TaskStatus
	headStatus domain.ApprovalStatus
	// digest is the definition digest recorded on the request. Empty means a
	// legacy request.
	digest string
	// planFP is the plan fingerprint recorded on the request.
	planFP string
	// requestedAt is when the request was recorded.
	requestedAt time.Time
	// reconciled lists the tasks the plan provenance records as reconciled.
	reconciled []string
	// definition is the persisted stage's deliverables.
	definition []string
}

func writeJSONDoc(t *testing.T, path string, v any) {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal %s: %v", path, err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func newInvalidationFixture(t *testing.T, opts invalidationFixtureOptions) invalidationFixture {
	t.Helper()
	dir := t.TempDir()
	stateDir := filepath.Join(dir, config.DirName)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir state: %v", err)
	}

	st, err := store.Open(filepath.Join(stateDir, "state.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	status := opts.taskStatus
	if status == "" {
		status = domain.READY
	}
	if err := st.Save(&domain.Task{ID: "S001", Title: "task", Status: status}); err != nil {
		t.Fatalf("save task: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	definition := opts.definition
	if len(definition) == 0 {
		definition = []string{"`internal/notify/event.go` — the event value."}
	}
	plan := map[string]any{
		"project": "fixture",
		"summary": "fixture",
		"stages": []map[string]any{{
			"id":                  "S001",
			"title":               "task",
			"objective":           "do the thing",
			"dependencies":        []string{},
			"deliverables":        definition,
			"acceptance_criteria": []string{"the thing is done"},
			"acceptance_checks":   []string{"go build ./..."},
		}},
	}
	writeJSONDoc(t, filepath.Join(stateDir, "plan.json"), plan)

	gen := time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)
	if opts.requestedAt.IsZero() {
		opts.requestedAt = gen.Add(-time.Hour)
	}
	if opts.reconciled == nil {
		opts.reconciled = []string{"S001"}
	}
	meta := map[string]any{
		"source":           "docs/PLAN.md",
		"source_kind":      "plan",
		"source_sha256":    "revised-plan-fingerprint",
		"plan_id":          "fixture",
		"generated_at":     gen,
		"reconciled_tasks": opts.reconciled,
	}
	writeJSONDoc(t, filepath.Join(stateDir, "plan.meta.json"), meta)

	runDir := filepath.Join(stateDir, "runs", "S001")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir run: %v", err)
	}
	head := domain.ApprovalStatus(opts.headStatus)
	if head == "" {
		head = domain.ApprovalPending
	}
	req := domain.ApprovalRequest{
		ID:               "S001-1",
		TaskID:           "S001",
		Kind:             domain.ApprovalNeedsHuman,
		Target:           "S001",
		Reason:           "required acceptance evidence is missing or failed",
		Evidence:         "deliverable-2 not met",
		Stage:            "WAITING_FOR_HUMAN",
		Disposition:      "NEEDS_HUMAN",
		DefinitionDigest: opts.digest,
		PlanFingerprint:  opts.planFP,
		RequestedAt:      opts.requestedAt,
		RequestedBy:      "sop",
		Status:           head,
	}
	writeJSONDoc(t, filepath.Join(runDir, "approval.json"), req)
	return invalidationFixture{dir: dir, runDir: runDir, planFP: "revised-plan-fingerprint", metaGen: gen}
}

func invalidationDeps(dir string) deps {
	return deps{getwd: func() (string, error) { return dir, nil }}
}

// TestCLIInvalidateLegacyRequestAfterReconciliation covers the reported deadlock:
// a request raised before the definition changed, with no recorded digest, is
// invalidated once SOP's own provenance proves the change was accepted later.
func TestCLIInvalidateLegacyRequestAfterReconciliation(t *testing.T) {
	f := newInvalidationFixture(t, invalidationFixtureOptions{taskStatus: domain.READY})
	var out, errb bytes.Buffer
	code := runApprovalInvalidate(
		[]string{"S001", "--reason", "evidence references a removed deliverable", "--by", "operator"},
		&out, &errb, invalidationDeps(f.dir))
	if code != exitOK {
		t.Fatalf("exit = %d, want OK; stderr=%s", code, errb.String())
	}
	if !strings.Contains(out.String(), "invalidated S001") {
		t.Errorf("output = %q", out.String())
	}
	if !strings.Contains(out.String(), "operator") || !strings.Contains(out.String(), "removed deliverable") {
		t.Errorf("output missing actor or reason: %q", out.String())
	}

	var head domain.ApprovalRequest
	readApprovalHead(t, f.runDir, &head)
	if head.Status != domain.ApprovalInvalidated {
		t.Fatalf("head status = %s, want INVALIDATED", head.Status)
	}
	if head.Decision != nil {
		t.Errorf("invalidation must not record a decision: %+v", head.Decision)
	}
	if head.Invalidation == nil {
		t.Fatal("invalidation record missing")
	}
	if got := head.Invalidation.Reason; !strings.Contains(got, "removed deliverable") {
		t.Errorf("recorded reason = %q", got)
	}
	if head.Invalidation.InvalidatedBy != "operator" {
		t.Errorf("recorded actor = %q", head.Invalidation.InvalidatedBy)
	}
	if head.Invalidation.PlanFingerprintAfter != f.planFP {
		t.Errorf("plan fingerprint after = %q, want %q", head.Invalidation.PlanFingerprintAfter, f.planFP)
	}
	if head.Invalidation.DefinitionDigestAfter == "" {
		t.Error("revised definition digest not recorded")
	}

	// The original request and its evidence survive verbatim in the history.
	history := readApprovalHistory(t, f.runDir)
	if len(history) != 1 {
		t.Fatalf("history entries = %d, want 1", len(history))
	}
	orig := history[0]
	if orig.ID != "S001-1" || orig.Status != domain.ApprovalPending {
		t.Errorf("history entry = %s/%s, want the original PENDING request", orig.ID, orig.Status)
	}
	if orig.Evidence != "deliverable-2 not met" {
		t.Errorf("history evidence = %q", orig.Evidence)
	}

	// Invalidation is not completion: the task is untouched and still runnable.
	task := readTask(t, f.dir, "S001")
	if task.Status != domain.READY {
		t.Errorf("task status = %s, want READY", task.Status)
	}
	if task.IsSatisfied() {
		t.Error("invalidation must not satisfy the task")
	}
	if code := approvalExecutionGate(f.dir, task, &out, &errb); code != exitOK {
		t.Errorf("execution gate = %d, want OK so fresh execution can verify", code)
	}
}

// TestCLIInvalidateStampedRequestUsesDigest covers a request that recorded its
// definition digest: the revised definition differs, so invalidation is allowed
// and records both digests.
func TestCLIInvalidateStampedRequestUsesDigest(t *testing.T) {
	f := newInvalidationFixture(t, invalidationFixtureOptions{
		taskStatus: domain.READY,
		digest:     "stale-definition-digest",
		planFP:     "original-plan-fingerprint",
	})
	var out, errb bytes.Buffer
	code := runApprovalInvalidate(
		[]string{"S001", "--reason", "definition changed", "--by", "operator"},
		&out, &errb, invalidationDeps(f.dir))
	if code != exitOK {
		t.Fatalf("exit = %d, want OK; stderr=%s", code, errb.String())
	}
	var head domain.ApprovalRequest
	readApprovalHead(t, f.runDir, &head)
	if head.Invalidation == nil {
		t.Fatal("invalidation record missing")
	}
	if head.Invalidation.DefinitionDigestBefore != "stale-definition-digest" {
		t.Errorf("digest before = %q", head.Invalidation.DefinitionDigestBefore)
	}
	if head.Invalidation.DefinitionDigestAfter == "" ||
		head.Invalidation.DefinitionDigestAfter == head.Invalidation.DefinitionDigestBefore {
		t.Errorf("digest after = %q, want the revised digest", head.Invalidation.DefinitionDigestAfter)
	}
	if head.Invalidation.PlanFingerprintBefore != "original-plan-fingerprint" {
		t.Errorf("plan fingerprint before = %q", head.Invalidation.PlanFingerprintBefore)
	}
}

// TestCLIInvalidateRefusesInapplicableRequests proves the command refuses every
// case where the evidence is still applicable or the request is not decidable,
// and records nothing.
func TestCLIInvalidateRefusesInapplicableRequests(t *testing.T) {
	t.Run("usage requires reason and actor", func(t *testing.T) {
		f := newInvalidationFixture(t, invalidationFixtureOptions{taskStatus: domain.READY})
		var out, errb bytes.Buffer
		if code := runApprovalInvalidate([]string{"S001", "--reason", "r"}, &out, &errb, invalidationDeps(f.dir)); code != exitUsage {
			t.Errorf("exit = %d, want usage", code)
		}
		var head domain.ApprovalRequest
		readApprovalHead(t, f.runDir, &head)
		if head.Status != domain.ApprovalPending {
			t.Errorf("head changed on a usage error: %s", head.Status)
		}
	})

	t.Run("unchanged definition is refused", func(t *testing.T) {
		// A stamped request whose digest equals the current definition digest.
		seed := newInvalidationFixture(t, invalidationFixtureOptions{taskStatus: domain.READY})
		digest := mustTaskDefinitionDigest(t, seed.dir, "S001")
		f := newInvalidationFixture(t, invalidationFixtureOptions{
			taskStatus: domain.READY,
			digest:     digest,
		})
		var out, errb bytes.Buffer
		if code := runApprovalInvalidate([]string{"S001", "--reason", "r", "--by", "operator"}, &out, &errb, invalidationDeps(f.dir)); code == exitOK {
			t.Fatal("expected refusal when the definition is unchanged")
		}
		if !strings.Contains(errb.String(), "unchanged") {
			t.Errorf("stderr = %q, want the unchanged-definition refusal", errb.String())
		}
		var head domain.ApprovalRequest
		readApprovalHead(t, f.runDir, &head)
		if head.Status != domain.ApprovalPending {
			t.Errorf("head changed on refusal: %s", head.Status)
		}
	})

	t.Run("legacy request without reconciliation is refused", func(t *testing.T) {
		f := newInvalidationFixture(t, invalidationFixtureOptions{
			taskStatus: domain.READY,
			reconciled: []string{},
		})
		var out, errb bytes.Buffer
		if code := runApprovalInvalidate([]string{"S001", "--reason", "r", "--by", "operator"}, &out, &errb, invalidationDeps(f.dir)); code == exitOK {
			t.Fatal("expected refusal without reconciliation provenance")
		}
		if !strings.Contains(errb.String(), "reconciliation provenance") {
			t.Errorf("stderr = %q", errb.String())
		}
	})

	t.Run("reconciliation older than the request is refused", func(t *testing.T) {
		f := newInvalidationFixture(t, invalidationFixtureOptions{
			taskStatus:  domain.READY,
			requestedAt: time.Date(2024, 5, 1, 13, 0, 0, 0, time.UTC),
		})
		var out, errb bytes.Buffer
		if code := runApprovalInvalidate([]string{"S001", "--reason", "r", "--by", "operator"}, &out, &errb, invalidationDeps(f.dir)); code == exitOK {
			t.Fatal("expected refusal when reconciliation predates the request")
		}
		if !strings.Contains(errb.String(), "after the request was raised") {
			t.Errorf("stderr = %q", errb.String())
		}
	})

	t.Run("satisfied task is refused", func(t *testing.T) {
		f := newInvalidationFixture(t, invalidationFixtureOptions{taskStatus: domain.LOCAL_DONE})
		var out, errb bytes.Buffer
		if code := runApprovalInvalidate([]string{"S001", "--reason", "r", "--by", "operator"}, &out, &errb, invalidationDeps(f.dir)); code == exitOK {
			t.Fatal("expected refusal for a satisfied task")
		}
		if !strings.Contains(errb.String(), "supersede") {
			t.Errorf("stderr = %q, want a pointer to supersede", errb.String())
		}
	})
}

// TestExecutionGatePreservesBlockingSemantics proves the gate change is additive:
// pending and declined heads still block, and only an invalidated head permits
// execution.
func TestExecutionGatePreservesBlockingSemantics(t *testing.T) {
	cases := []struct {
		name   string
		status domain.ApprovalStatus
		want   int
	}{
		{"pending blocks", domain.ApprovalPending, exitApprovalBlocked},
		{"declined blocks", domain.ApprovalDeclined, exitApprovalBlocked},
		{"invalidated permits execution", domain.ApprovalInvalidated, exitOK},
		{"approved permits execution", domain.ApprovalApproved, exitOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newInvalidationFixture(t, invalidationFixtureOptions{
				taskStatus: domain.READY,
				headStatus: tc.status,
			})
			var out, errb bytes.Buffer
			task := readTask(t, f.dir, "S001")
			if code := approvalExecutionGate(f.dir, task, &out, &errb); code != tc.want {
				t.Fatalf("gate = %d, want %d; stdout=%s stderr=%s", code, tc.want, out.String(), errb.String())
			}
		})
	}
}

// TestApprovalsListingSeparatesInvalidated proves an invalidated request is never
// presented as an applicable gate.
func TestApprovalsListingSeparatesInvalidated(t *testing.T) {
	f := newInvalidationFixture(t, invalidationFixtureOptions{
		taskStatus: domain.READY,
		headStatus: domain.ApprovalInvalidated,
	})
	var out, errb bytes.Buffer
	if code := runApprovals([]string{"--json"}, &out, &errb, invalidationDeps(f.dir)); code != exitOK {
		t.Fatalf("exit = %d; stderr=%s", code, errb.String())
	}
	var doc struct {
		Approvals []map[string]any `json:"approvals"`
	}
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("decode listing: %v (%s)", err, out.String())
	}
	if len(doc.Approvals) != 0 {
		t.Fatalf("invalidated request listed as applicable: %+v", doc.Approvals)
	}
}

// mustTaskDefinitionDigest reads the fixture's current definition digest through
// the same helper the command uses, so a test can stamp a request with it.
func mustTaskDefinitionDigest(t *testing.T, dir, id string) string {
	t.Helper()
	idn, err := planflow.TaskDefinitionIdentity(dir, id)
	if err != nil {
		t.Fatalf("definition identity: %v", err)
	}
	if !idn.Present {
		t.Fatal("definition identity absent for the fixture stage")
	}
	return idn.Digest
}

func readApprovalHead(t *testing.T, runDir string, out *domain.ApprovalRequest) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(runDir, "approval.json"))
	if err != nil {
		t.Fatalf("read head: %v", err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatalf("decode head: %v", err)
	}
}

func readApprovalHistory(t *testing.T, runDir string) []domain.ApprovalRequest {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(runDir, "approval-history.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("read history: %v", err)
	}
	var out []domain.ApprovalRequest
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	return out
}

func readTask(t *testing.T, dir, id string) *domain.Task {
	t.Helper()
	st, err := store.Open(filepath.Join(dir, config.DirName, "state.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	task, err := st.Get(id)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	return task
}
