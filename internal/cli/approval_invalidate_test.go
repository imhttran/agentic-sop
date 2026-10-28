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

// acceptedChange seeds one task-scoped accepted-change record in plan metadata.
type acceptedChange struct {
	TaskID       string
	AcceptedAt   time.Time
	DigestAfter  string
	DigestBefore string
}

// invalidationFixtureOptions describes the state a test needs. The zero value is
// an unsatisfied task with a PENDING legacy request and no acceptance evidence.
type invalidationFixtureOptions struct {
	taskStatus domain.TaskStatus
	headStatus domain.ApprovalStatus
	// digest is the definition digest recorded on the request. Empty is a legacy
	// request.
	digest string
	// planFP is the plan fingerprint recorded on the request.
	planFP string
	// requestedAt is when the request was recorded.
	requestedAt time.Time
	// legacyReconciled is written to metadata reconciled_tasks. It is legacy
	// provenance only and must never be read as acceptance evidence.
	legacyReconciled []string
	// accepted is the task-scoped accepted-change evidence, when any.
	accepted *acceptedChange
	// metaGeneratedAt is the plan-wide write time. It defaults to a time after the
	// request, which is exactly the "no-op plan write after the request" case.
	metaGeneratedAt time.Time
	// definition is the persisted stage's deliverables.
	definition []string
}

type invalidationFixture struct {
	dir     string
	runDir  string
	planFP  string
	digest  string // current definition digest
	request time.Time
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
	writeJSONDoc(t, filepath.Join(stateDir, "plan.json"), map[string]any{
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
	})
	current := mustTaskDefinitionDigest(t, dir, "S001")

	requestedAt := opts.requestedAt
	if requestedAt.IsZero() {
		requestedAt = time.Date(2024, 5, 1, 11, 0, 0, 0, time.UTC)
	}
	generatedAt := opts.metaGeneratedAt
	if generatedAt.IsZero() {
		generatedAt = requestedAt.Add(2 * time.Hour)
	}

	meta := map[string]any{
		"source":        "docs/PLAN.md",
		"source_kind":   "plan",
		"source_sha256": "revised-plan-fingerprint",
		"plan_id":       "fixture",
		"generated_at":  generatedAt,
	}
	if opts.legacyReconciled != nil {
		meta["reconciled_tasks"] = opts.legacyReconciled
	}
	if a := opts.accepted; a != nil {
		after := a.DigestAfter
		if after == "" {
			after = current
		}
		before := a.DigestBefore
		if before == "" {
			before = "previous-definition-digest"
		}
		meta["accepted_changes"] = []map[string]any{{
			"task_id":                  a.TaskID,
			"accepted_at":              a.AcceptedAt,
			"source_sha256":            "revised-plan-fingerprint",
			"definition_digest_before": before,
			"definition_digest_after":  after,
		}}
	}
	writeJSONDoc(t, filepath.Join(stateDir, "plan.meta.json"), meta)

	runDir := filepath.Join(stateDir, "runs", "S001")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir run: %v", err)
	}
	head := opts.headStatus
	if head == "" {
		head = domain.ApprovalPending
	}
	writeJSONDoc(t, filepath.Join(runDir, "approval.json"), domain.ApprovalRequest{
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
		RequestedAt:      requestedAt,
		RequestedBy:      "sop",
		Status:           head,
	})
	return invalidationFixture{dir: dir, runDir: runDir, planFP: "revised-plan-fingerprint", digest: current, request: requestedAt}
}

func invalidationDeps(dir string) deps {
	return deps{getwd: func() (string, error) { return dir, nil }}
}

// TestCLIInvalidateLegacyRequestAfterReconciliation covers the reported deadlock: a
// legacy request is invalidated once a task-scoped acceptance recorded after the
// request proves the definition changed.
func TestCLIInvalidateLegacyRequestAfterReconciliation(t *testing.T) {
	f := newInvalidationFixture(t, invalidationFixtureOptions{
		taskStatus: domain.READY,
		accepted: &acceptedChange{
			TaskID:     "S001",
			AcceptedAt: time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC),
		},
	})
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
	if !strings.Contains(head.Invalidation.Reason, "removed deliverable") {
		t.Errorf("recorded reason = %q", head.Invalidation.Reason)
	}
	if head.Invalidation.InvalidatedBy != "operator" {
		t.Errorf("recorded actor = %q", head.Invalidation.InvalidatedBy)
	}
	if head.Invalidation.PlanFingerprintAfter != f.planFP {
		t.Errorf("plan fingerprint after = %q, want %q", head.Invalidation.PlanFingerprintAfter, f.planFP)
	}
	if head.Invalidation.DefinitionDigestAfter != f.digest {
		t.Errorf("definition digest after = %q, want %q", head.Invalidation.DefinitionDigestAfter, f.digest)
	}
	if !head.Invalidation.DefinitionAcceptedAt.Equal(time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)) {
		t.Errorf("acceptance time = %v", head.Invalidation.DefinitionAcceptedAt)
	}

	history := readApprovalHistory(t, f.runDir)
	if len(history) != 1 {
		t.Fatalf("history entries = %d, want 1", len(history))
	}
	if orig := history[0]; orig.ID != "S001-1" || orig.Status != domain.ApprovalPending || orig.Evidence != "deliverable-2 not met" {
		t.Errorf("history entry = %+v, want the original PENDING request verbatim", orig)
	}

	task := readTask(t, f.dir, "S001")
	if task.Status != domain.READY || task.IsSatisfied() {
		t.Errorf("task status = %s, satisfied = %v; invalidation must not complete it", task.Status, task.IsSatisfied())
	}
	if code := approvalExecutionGate(f.dir, task, &out, &errb); code != exitOK {
		t.Errorf("execution gate = %d, want OK so fresh execution can verify", code)
	}
}

// TestCLIInvalidateStampedRequestUsesDigest covers a stamped request invalidated by
// a task-scoped acceptance that covers the current definition.
func TestCLIInvalidateStampedRequestUsesDigest(t *testing.T) {
	f := newInvalidationFixture(t, invalidationFixtureOptions{
		taskStatus: domain.READY,
		digest:     "stale-definition-digest",
		planFP:     "original-plan-fingerprint",
		accepted: &acceptedChange{
			TaskID:     "S001",
			AcceptedAt: time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC),
		},
	})
	var out, errb bytes.Buffer
	if code := runApprovalInvalidate(
		[]string{"S001", "--reason", "definition changed", "--by", "operator"},
		&out, &errb, invalidationDeps(f.dir)); code != exitOK {
		t.Fatalf("exit = %d, want OK; stderr=%s", code, errb.String())
	}
	var head domain.ApprovalRequest
	readApprovalHead(t, f.runDir, &head)
	if head.Invalidation.DefinitionDigestBefore != "stale-definition-digest" {
		t.Errorf("digest before = %q", head.Invalidation.DefinitionDigestBefore)
	}
	if head.Invalidation.DefinitionDigestAfter != f.digest {
		t.Errorf("digest after = %q, want current", head.Invalidation.DefinitionDigestAfter)
	}
}

// TestCLIInvalidateRefusesInapplicableRequests proves every refusal case, and that
// a refusal writes nothing.
func TestCLIInvalidateRefusesInapplicableRequests(t *testing.T) {
	afterRequest := time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)

	t.Run("usage requires reason and actor", func(t *testing.T) {
		f := newInvalidationFixture(t, invalidationFixtureOptions{accepted: &acceptedChange{TaskID: "S001", AcceptedAt: afterRequest}})
		var out, errb bytes.Buffer
		if code := runApprovalInvalidate([]string{"S001", "--reason", "r"}, &out, &errb, invalidationDeps(f.dir)); code != exitUsage {
			t.Errorf("exit = %d, want usage", code)
		}
		assertHeadPending(t, f.runDir)
	})

	t.Run("changed digest without accepted reconciliation is refused", func(t *testing.T) {
		// A stamped request whose definition changed, but nothing accepted the change.
		f := newInvalidationFixture(t, invalidationFixtureOptions{
			digest: "stale-definition-digest",
		})
		var out, errb bytes.Buffer
		if code := runApprovalInvalidate([]string{"S001", "--reason", "r", "--by", "operator"}, &out, &errb, invalidationDeps(f.dir)); code == exitOK {
			t.Fatal("a different digest alone must not authorize invalidation")
		}
		if !strings.Contains(errb.String(), "accepted definition change") {
			t.Errorf("stderr = %q, want the accepted-change refusal", errb.String())
		}
		assertHeadPending(t, f.runDir)
	})

	t.Run("provider failure alone is refused", func(t *testing.T) {
		// The reported APPR-004 shape: a legacy request raised by a provider failure,
		// with no accepted definition change anywhere.
		f := newInvalidationFixture(t, invalidationFixtureOptions{taskStatus: domain.BLOCKED})
		var head domain.ApprovalRequest
		readApprovalHead(t, f.runDir, &head)
		head.Reason = "ollama returned an empty response"
		head.Evidence = "the current run is at an authoritative approval boundary; a human decision is required"
		writeJSONDoc(t, filepath.Join(f.runDir, "approval.json"), head)

		var out, errb bytes.Buffer
		if code := runApprovalInvalidate([]string{"S001", "--reason", "provider failure", "--by", "operator"}, &out, &errb, invalidationDeps(f.dir)); code == exitOK {
			t.Fatal("a provider failure must not qualify as an accepted definition change")
		}
		if !strings.Contains(errb.String(), "accepted definition change") {
			t.Errorf("stderr = %q", errb.String())
		}
		assertHeadPending(t, f.runDir)
	})

	t.Run("reconciliation before the request plus a later no-op plan write is refused", func(t *testing.T) {
		// The acceptance is task-scoped and real, but it predates the request. The
		// plan-wide generated_at is afterwards, which must not be used as proof.
		f := newInvalidationFixture(t, invalidationFixtureOptions{
			requestedAt:     time.Date(2024, 5, 1, 11, 0, 0, 0, time.UTC),
			metaGeneratedAt: time.Date(2024, 5, 1, 18, 0, 0, 0, time.UTC),
			accepted: &acceptedChange{
				TaskID:     "S001",
				AcceptedAt: time.Date(2024, 5, 1, 9, 0, 0, 0, time.UTC),
			},
		})
		var out, errb bytes.Buffer
		if code := runApprovalInvalidate([]string{"S001", "--reason", "r", "--by", "operator"}, &out, &errb, invalidationDeps(f.dir)); code == exitOK {
			t.Fatal("a plan write after the request must not substitute for acceptance after it")
		}
		if !strings.Contains(errb.String(), "after the request was raised") {
			t.Errorf("stderr = %q", errb.String())
		}
		assertHeadPending(t, f.runDir)
	})

	t.Run("unrelated reconciliation after the request is refused", func(t *testing.T) {
		// Another task's accepted change must never make this request eligible.
		f := newInvalidationFixture(t, invalidationFixtureOptions{
			accepted: &acceptedChange{TaskID: "OTHER-999", AcceptedAt: afterRequest},
		})
		var out, errb bytes.Buffer
		if code := runApprovalInvalidate([]string{"S001", "--reason", "r", "--by", "operator"}, &out, &errb, invalidationDeps(f.dir)); code == exitOK {
			t.Fatal("another task's reconciliation must not qualify this request")
		}
		if !strings.Contains(errb.String(), "accepted definition change") {
			t.Errorf("stderr = %q", errb.String())
		}
		assertHeadPending(t, f.runDir)
	})

	t.Run("legacy reconciled list without acceptance records is refused", func(t *testing.T) {
		// reconciled_tasks is legacy provenance, not task-scoped acceptance evidence.
		f := newInvalidationFixture(t, invalidationFixtureOptions{
			legacyReconciled: []string{"S001"},
			metaGeneratedAt:  afterRequest,
		})
		var out, errb bytes.Buffer
		if code := runApprovalInvalidate([]string{"S001", "--reason", "r", "--by", "operator"}, &out, &errb, invalidationDeps(f.dir)); code == exitOK {
			t.Fatal("legacy reconciled_tasks must not authorize invalidation")
		}
		assertHeadPending(t, f.runDir)
	})

	t.Run("acceptance covering another definition is refused", func(t *testing.T) {
		f := newInvalidationFixture(t, invalidationFixtureOptions{
			accepted: &acceptedChange{
				TaskID:      "S001",
				AcceptedAt:  afterRequest,
				DigestAfter: "some-other-definition",
			},
		})
		var out, errb bytes.Buffer
		if code := runApprovalInvalidate([]string{"S001", "--reason", "r", "--by", "operator"}, &out, &errb, invalidationDeps(f.dir)); code == exitOK {
			t.Fatal("an acceptance of a different definition must not authorize this request")
		}
		if !strings.Contains(errb.String(), "does not cover the current definition") {
			t.Errorf("stderr = %q", errb.String())
		}
		assertHeadPending(t, f.runDir)
	})

	t.Run("unchanged definition is refused", func(t *testing.T) {
		f := newInvalidationFixture(t, invalidationFixtureOptions{
			digest: "will-be-replaced",
		})
		// Stamp the request with the CURRENT digest so the definition is unchanged.
		var head domain.ApprovalRequest
		readApprovalHead(t, f.runDir, &head)
		head.DefinitionDigest = f.digest
		writeJSONDoc(t, filepath.Join(f.runDir, "approval.json"), head)

		var out, errb bytes.Buffer
		if code := runApprovalInvalidate([]string{"S001", "--reason", "r", "--by", "operator"}, &out, &errb, invalidationDeps(f.dir)); code == exitOK {
			t.Fatal("expected refusal when the definition is unchanged")
		}
		assertHeadPending(t, f.runDir)
	})

	t.Run("satisfied task is refused", func(t *testing.T) {
		f := newInvalidationFixture(t, invalidationFixtureOptions{
			taskStatus: domain.LOCAL_DONE,
			accepted:   &acceptedChange{TaskID: "S001", AcceptedAt: afterRequest},
		})
		var out, errb bytes.Buffer
		if code := runApprovalInvalidate([]string{"S001", "--reason", "r", "--by", "operator"}, &out, &errb, invalidationDeps(f.dir)); code == exitOK {
			t.Fatal("expected refusal for a satisfied task")
		}
		if !strings.Contains(errb.String(), "supersede") {
			t.Errorf("stderr = %q, want a pointer to supersede", errb.String())
		}
	})
}

// TestExecutionGatePreservesBlockingSemantics proves the gate change is additive.
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
			f := newInvalidationFixture(t, invalidationFixtureOptions{headStatus: tc.status})
			var out, errb bytes.Buffer
			task := readTask(t, f.dir, "S001")
			if code := approvalExecutionGate(f.dir, task, &out, &errb); code != tc.want {
				t.Fatalf("gate = %d, want %d", code, tc.want)
			}
		})
	}
}

// TestApprovalsListingSeparatesInvalidated proves an invalidated request is never
// presented as an applicable gate.
func TestApprovalsListingSeparatesInvalidated(t *testing.T) {
	f := newInvalidationFixture(t, invalidationFixtureOptions{headStatus: domain.ApprovalInvalidated})
	var out, errb bytes.Buffer
	if code := runApprovals([]string{"--json"}, &out, &errb, invalidationDeps(f.dir)); code != exitOK {
		t.Fatalf("exit = %d; stderr=%s", code, errb.String())
	}
	var doc struct {
		Approvals []map[string]any `json:"approvals"`
	}
	if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
		t.Fatalf("decode listing: %v", err)
	}
	if len(doc.Approvals) != 0 {
		t.Fatalf("invalidated request listed as applicable: %+v", doc.Approvals)
	}
}

func assertHeadPending(t *testing.T, runDir string) {
	t.Helper()
	var head domain.ApprovalRequest
	readApprovalHead(t, runDir, &head)
	if head.Status != domain.ApprovalPending || head.Invalidation != nil {
		t.Errorf("refusal mutated the head: status=%s invalidation=%+v", head.Status, head.Invalidation)
	}
}

// mustTaskDefinitionDigest reads the fixture's current definition digest through
// the same helper the command uses.
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
