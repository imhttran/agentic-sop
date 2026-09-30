package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/domain"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/store"
)

// approvalArtifact reads the task's persisted approval request head, if any.
func approvalArtifact(t *testing.T, dir, taskID string) (domain.ApprovalRequest, bool) {
	t.Helper()
	return runpkg.At(runpkg.Dir(dir, taskID)).Approval()
}

func TestApproveWithoutRequestIsAnError(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	// A BLOCKED task carrying a human reason but no explicit approval request is
	// NOT an approval boundary.
	seedTask(t, dir, &domain.Task{ID: "S001", Title: "x", Status: domain.BLOCKED, BlockedReason: domain.REVIEW_UNRESOLVED, MaxAttempts: 3})

	code, _, stderr := runCLI(t, dir, "approve", "S001")
	if code != exitError {
		t.Fatalf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "no approval request") {
		t.Errorf("stderr = %q, want it to report no approval request", stderr)
	}

	// The task is untouched: approve never mutates state outside the lifecycle.
	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	got, err := st.Get("S001")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.BLOCKED {
		t.Errorf("task status = %s, want BLOCKED (unchanged)", got.Status)
	}
}

func TestApprovalStatusWithoutRequest(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "S001", Title: "x", Status: domain.BLOCKED, BlockedReason: domain.RETRIES_EXHAUSTED, MaxAttempts: 3})

	code, stdout, stderr := runCLI(t, dir, "approval", "S001")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "Approval: none") {
		t.Errorf("stdout = %q, want an explicit no-approval report", stdout)
	}
}

// TestHumanGateRecordsApprovableRequest is the dogfood scenario: a task reaches a
// genuine human approval gate, SOP records an explicit resolvable request, a
// human approves it, and the decision is persisted with provenance.
func TestHumanGateRecordsApprovableRequest(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join("docs", "PLAN.md"), autoPlanDoc)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")
	// A genuine human boundary, reached through a structured action kind (a
	// destructive/irreversible operation), not through approval prose.
	a := outcomeAgent{outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: "the operation is destructive and cannot be undone"}}

	if code, _, _ := runInjectedCLI(t, dir, "diff\n", a, "run"); code != exitError {
		t.Fatalf("run code=%d, want %d", code, exitError)
	}

	req, ok := approvalArtifact(t, dir, "S001")
	if !ok {
		t.Fatalf("no approval request recorded for a genuine human gate")
	}
	if req.Status != domain.ApprovalPending || req.Kind != domain.ApprovalNeedsHuman {
		t.Fatalf("request = %+v, want a PENDING NEEDS_HUMAN request", req)
	}
	if req.Stage != string(runpkg.WaitingForHuman) {
		t.Errorf("request stage = %q, want %q", req.Stage, runpkg.WaitingForHuman)
	}

	// The read API reports the gate as present and applicable.
	code, stdout, stderr := runCLI(t, dir, "approval", "S001")
	if code != exitOK {
		t.Fatalf("approval code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "Approval: PENDING") {
		t.Errorf("stdout = %q, want a pending approval", stdout)
	}

	// Approve it through the application boundary.
	code, stdout, stderr = runCLI(t, dir, "approve", "S001", "--by", "reviewer", "--note", "lgtm")
	if code != exitOK {
		t.Fatalf("approve code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "approved S001") || !strings.Contains(stdout, "Approval: APPROVED") {
		t.Errorf("stdout = %q, want an approved report", stdout)
	}

	got, ok := approvalArtifact(t, dir, "S001")
	if !ok {
		t.Fatalf("approval request vanished after approval")
	}
	if got.Status != domain.ApprovalApproved || got.Decision == nil {
		t.Fatalf("persisted request = %+v, want APPROVED with a decision", got)
	}
	if got.Decision.DecidedBy != "reviewer" || got.Decision.Note != "lgtm" {
		t.Errorf("decision provenance = %+v", got.Decision)
	}
	if got.Decision.RequestID != got.ID || got.Decision.TaskID != "S001" {
		t.Errorf("decision is not tied to its request/task: %+v", got.Decision)
	}

	// The decision is durable provenance in the append-only history.
	history := runpkg.At(runpkg.Dir(dir, "S001")).ApprovalHistory()
	if len(history) != 1 || history[0].ID != got.ID || history[0].Status != domain.ApprovalApproved {
		t.Fatalf("history = %+v, want the approved request", history)
	}

	// A repeat approve is idempotent and does not duplicate the decision.
	code, stdout, stderr = runCLI(t, dir, "approve", "S001")
	if code != exitOK {
		t.Fatalf("repeat approve code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "already approved") {
		t.Errorf("stdout = %q, want an idempotent report", stdout)
	}
	if history := runpkg.At(runpkg.Dir(dir, "S001")).ApprovalHistory(); len(history) != 1 {
		t.Errorf("idempotent approve appended history: %d entries", len(history))
	}

	// `sop report` surfaces the approval boundary and its decision.
	code, stdout, stderr = runCLI(t, dir, "report", "S001")
	if code != exitOK {
		t.Fatalf("report code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "Approval:") || !strings.Contains(stdout, "APPROVED") {
		t.Errorf("report = %q, want the approval decision", stdout)
	}
}

// TestDeclineHumanGatePreservesLifecycle proves a decline is recorded but never
// manufactures completion.
func TestDeclineHumanGatePreservesLifecycle(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join("docs", "PLAN.md"), autoPlanDoc)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")
	a := outcomeAgent{outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: "the operation is destructive and cannot be undone"}}

	if code, _, _ := runInjectedCLI(t, dir, "diff\n", a, "run"); code != exitError {
		t.Fatalf("run code=%d, want %d", code, exitError)
	}

	code, stdout, stderr := runCLI(t, dir, "decline", "S001", "--note", "not this way")
	if code != exitOK {
		t.Fatalf("decline code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "declined S001") || !strings.Contains(stdout, "Approval: DECLINED") {
		t.Errorf("stdout = %q, want a decline report", stdout)
	}

	got, ok := approvalArtifact(t, dir, "S001")
	if !ok || got.Status != domain.ApprovalDeclined {
		t.Fatalf("persisted request = %+v (ok=%t), want DECLINED", got, ok)
	}

	// The task is NOT completed: decline preserves the truthful lifecycle state.
	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	task, err := st.Get("S001")
	if err != nil {
		t.Fatal(err)
	}
	if task.IsSatisfied() {
		t.Fatalf("decline manufactured completion: %s", task.Status)
	}
}

// TestApproveLeavesRepositoryUntouched proves the approval boundary is pure
// persistence: it never mutates the working tree, so a user's unrelated dirty
// changes survive.
func TestApproveLeavesRepositoryUntouched(t *testing.T) {
	dir := newDirtyRepo(t)
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "S001", Title: "x", Status: domain.BLOCKED, BlockedReason: domain.RETRIES_EXHAUSTED, MaxAttempts: 3})
	rn, err := runpkg.New(dir, "S001")
	if err != nil {
		t.Fatal(err)
	}
	if err := rn.SaveApproval(domain.ApprovalRequest{
		ID: "g", TaskID: "S001", Kind: domain.ApprovalNeedsHuman, Target: "S001",
		RequestedAt: time.Now().UTC(), Status: domain.ApprovalPending,
	}); err != nil {
		t.Fatal(err)
	}

	if code, _, stderr := runCLI(t, dir, "approve", "S001"); code != exitOK {
		t.Fatalf("approve code=%d stderr=%s", code, stderr)
	}

	if got, err := os.ReadFile(filepath.Join(dir, "tracked.txt")); err != nil || string(got) != "user edit\n" {
		t.Errorf("tracked.txt = %q, %v; approval reverted the user's edit", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "untracked.txt")); err != nil || string(got) != "scratch\n" {
		t.Errorf("untracked.txt = %q, %v; approval deleted the user's untracked file", got, err)
	}
}

// TestNeedsHumanClassificationWithoutRequestIsNotApproval proves a task parked at
// NEEDS_HUMAN with no explicit SOP approval request is NOT an approval boundary:
// clients must not infer one from the status or classification.
func TestNeedsHumanClassificationWithoutRequestIsNotApproval(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "S001", Title: "x", Status: domain.PLANNED, MaxAttempts: 3})

	// A run that parked at the human stage and recorded a NEEDS_HUMAN
	// classification, but recorded NO explicit approval request.
	rn, err := runpkg.New(dir, "S001")
	if err != nil {
		t.Fatal(err)
	}
	if err := rn.SetStage(runpkg.WaitingForHuman); err != nil {
		t.Fatal(err)
	}
	if err := rn.Write("classification.json", `{"disposition":"NEEDS_HUMAN","kind":"APPROVAL_REQUIRED"}`); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := runCLI(t, dir, "approval", "S001")
	if code != exitOK {
		t.Fatalf("approval code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "Approval: none") {
		t.Errorf("stdout = %q, want no approval boundary", stdout)
	}

	if code, _, _ := runCLI(t, dir, "approve", "S001"); code != exitError {
		t.Errorf("approve code=%d, want %d (no explicit request)", code, exitError)
	}
}

// TestApproveDoesNotBypassGates proves an approval only returns the task to
// runnable work: it never completes the task or skips the lifecycle.
func TestApproveDoesNotBypassGates(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "S001", Title: "x", Status: domain.BLOCKED, BlockedReason: domain.RETRIES_EXHAUSTED, MaxAttempts: 3})
	rn, err := runpkg.New(dir, "S001")
	if err != nil {
		t.Fatal(err)
	}
	if err := rn.SaveApproval(domain.ApprovalRequest{
		ID: "g", TaskID: "S001", Kind: domain.ApprovalNeedsHuman, Target: "S001",
		RequestedAt: time.Now().UTC(), Status: domain.ApprovalPending,
	}); err != nil {
		t.Fatal(err)
	}

	if code, _, stderr := runCLI(t, dir, "approve", "S001"); code != exitOK {
		t.Fatalf("approve code=%d stderr=%s", code, stderr)
	}

	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	got, err := st.Get("S001")
	if err != nil {
		t.Fatal(err)
	}
	if got.IsSatisfied() {
		t.Fatalf("approve completed the task (%s); it must only resume it", got.Status)
	}
	if got.Status != domain.PLANNED {
		t.Fatalf("approve left the task at %s, want PLANNED (the full lifecycle still runs)", got.Status)
	}
}

// TestApproveBlockedTaskResumesIt proves a recorded gate on a BLOCKED task is
// resolvable and resumes the task through the domain's existing requeue path.
func TestApproveBlockedTaskResumesIt(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "S001", Title: "x", Status: domain.BLOCKED, BlockedReason: domain.RETRIES_EXHAUSTED, Attempt: 3, MaxAttempts: 3})

	// SOP records the gate (as the lifecycle would when it parks a task).
	rn, err := runpkg.New(dir, "S001")
	if err != nil {
		t.Fatalf("new run: %v", err)
	}
	if err := rn.SaveApproval(domain.ApprovalRequest{
		ID:          "S001-gate",
		TaskID:      "S001",
		Kind:        domain.ApprovalNeedsHuman,
		Target:      "S001",
		Reason:      "conflicting requirements",
		Stage:       string(runpkg.WaitingForHuman),
		Disposition: "NEEDS_HUMAN",
		RequestedAt: time.Now().UTC(),
		Status:      domain.ApprovalPending,
	}); err != nil {
		t.Fatalf("seed approval: %v", err)
	}

	code, stdout, stderr := runCLI(t, dir, "approve", "S001")
	if code != exitOK {
		t.Fatalf("approve code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "lifecycle: REQUEUED") {
		t.Errorf("stdout = %q, want a REQUEUED lifecycle action", stdout)
	}

	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	got, err := st.Get("S001")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.PLANNED {
		t.Errorf("task status = %s, want PLANNED (resumed)", got.Status)
	}
	if got.Attempt != 3 {
		t.Errorf("approve spent a retry: attempt = %d, want 3", got.Attempt)
	}
}

// TestApprovalDecisionRecordedInActivity proves the decision reaches the task's
// activity stream, so a controller/CLI reading it sees the human decision.
func TestApprovalDecisionRecordedInActivity(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "S001", Title: "x", Status: domain.BLOCKED, BlockedReason: domain.REVIEW_UNRESOLVED, MaxAttempts: 3})
	rn, err := runpkg.New(dir, "S001")
	if err != nil {
		t.Fatalf("new run: %v", err)
	}
	if err := rn.SaveApproval(domain.ApprovalRequest{
		ID: "g", TaskID: "S001", Kind: domain.ApprovalNeedsHuman, Target: "S001",
		RequestedAt: time.Now().UTC(), Status: domain.ApprovalPending,
	}); err != nil {
		t.Fatalf("seed approval: %v", err)
	}

	if code, _, stderr := runCLI(t, dir, "approve", "S001", "--by", "reviewer"); code != exitOK {
		t.Fatalf("approve code=%d stderr=%s", code, stderr)
	}

	data, err := os.ReadFile(filepath.Join(runpkg.Dir(dir, "S001"), runpkg.ActivityArtifactName))
	if err != nil {
		t.Fatalf("read activity artifact: %v", err)
	}
	var found bool
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var ev struct {
			Stage  string `json:"stage"`
			Action string `json:"action"`
		}
		if json.Unmarshal([]byte(line), &ev) != nil {
			continue
		}
		if ev.Stage == "APPROVAL" && ev.Action == "APPROVED" {
			found = true
		}
	}
	if !found {
		t.Errorf("activity artifact has no APPROVAL/APPROVED event:\n%s", data)
	}
}

// TestMCPApprovalToolsDelegateToBoundary proves the MCP surface reuses the same
// application boundary (no second implementation).
func TestMCPApprovalToolsDelegateToBoundary(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "S001", Title: "x", Status: domain.BLOCKED, BlockedReason: domain.REVIEW_UNRESOLVED, MaxAttempts: 3})

	tools := map[string]bool{}
	for _, tool := range mcpTools(dir, deps{}) {
		tools[tool.Name] = true
	}
	for _, want := range []string{"sop_approval", "sop_approve", "sop_decline"} {
		if !tools[want] {
			t.Errorf("MCP tool %s not registered", want)
		}
	}

	// A task with no request is not approvable through MCP either.
	for _, tool := range mcpTools(dir, deps{}) {
		if tool.Name != "sop_approve" {
			continue
		}
		_, err := tool.Handler(nil, json.RawMessage(`{"taskId":"S001"}`))
		if err == nil || !strings.Contains(err.Error(), "no approval request") {
			t.Errorf("sop_approve err = %v, want no approval request", err)
		}
	}
}
