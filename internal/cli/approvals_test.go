package cli

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/domain"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
)

// `sop approvals`: the read-only enumeration of the tasks SOP reports at an
// applicable human approval gate. It creates nothing, resolves nothing, and
// classifies nothing itself.

// seedGate records a pending approval request for a task, exactly as the lifecycle
// does when it parks one at a human boundary.
func seedGate(t *testing.T, dir, taskID string) {
	t.Helper()
	rn, err := runpkg.New(dir, taskID)
	if err != nil {
		t.Fatalf("new run: %v", err)
	}
	if err := rn.SaveApproval(domain.ApprovalRequest{
		ID:          taskID + "-gate",
		TaskID:      taskID,
		Kind:        domain.ApprovalNeedsHuman,
		Target:      taskID,
		Reason:      "a human decision is required",
		Stage:       string(runpkg.WaitingForHuman),
		Disposition: "NEEDS_HUMAN",
		RequestedAt: time.Now().UTC(),
		Status:      domain.ApprovalPending,
	}); err != nil {
		t.Fatalf("seed approval: %v", err)
	}
}

func TestApprovalsEmpty(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	// A BLOCKED task with no explicit request is not an approval boundary.
	seedTask(t, dir, &domain.Task{ID: "S001", Title: "x", Status: domain.BLOCKED, BlockedReason: domain.RETRIES_EXHAUSTED, MaxAttempts: 3})

	code, stdout, stderr := runCLI(t, dir, "approvals")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "no pending approvals") {
		t.Errorf("stdout = %q, want the empty listing", stdout)
	}
}

// TestApprovalsListsOnlyApplicableGates proves the enumeration is driven by SOP's
// own record: a task with no request is not listed, and a request whose task has
// since completed (a stale gate) is not offered as applicable either.
func TestApprovalsListsOnlyApplicableGates(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "S001", Title: "waiting", Status: domain.BLOCKED, BlockedReason: domain.RETRIES_EXHAUSTED, MaxAttempts: 3})
	seedTask(t, dir, &domain.Task{ID: "S002", Title: "no gate", Status: domain.BLOCKED, BlockedReason: domain.RETRIES_EXHAUSTED, MaxAttempts: 3})
	seedTask(t, dir, &domain.Task{ID: "S003", Title: "already done", Status: domain.LOCAL_DONE, MaxAttempts: 3})

	seedGate(t, dir, "S001") // pending, applicable
	seedGate(t, dir, "S003") // pending, but the task completed: stale

	code, stdout, stderr := runCLI(t, dir, "approvals")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "S001") {
		t.Errorf("stdout = %q, want S001 listed", stdout)
	}
	if strings.Contains(stdout, "S002") {
		t.Error("a task with no approval request must not be listed")
	}
	if strings.Contains(stdout, "S003") {
		t.Errorf("a stale gate (task already completed) must not be listed as applicable:\n%s", stdout)
	}

	// Read-only: the gate is still pending and the task still BLOCKED.
	req, ok := approvalArtifact(t, dir, "S001")
	if !ok || req.Status != domain.ApprovalPending {
		t.Errorf("listing resolved the gate: %+v (ok=%t)", req, ok)
	}
}

// TestApprovalsJSON proves the machine-readable listing a client consumes: the same
// fields the single-task view projects, so a client renders a gate identically
// whether it read one or listed many.
func TestApprovalsJSON(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "S001", Title: "waiting", Status: domain.BLOCKED, BlockedReason: domain.RETRIES_EXHAUSTED, MaxAttempts: 3})
	seedGate(t, dir, "S001")

	code, stdout, stderr := runCLI(t, dir, "approvals", "--json")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}

	var doc struct {
		Version   int `json:"version"`
		Approvals []struct {
			TaskID string `json:"task_id"`
			Kind   string `json:"kind"`
			Stage  string `json:"stage"`
			Status string `json:"status"`
			Reason string `json:"reason"`
		} `json:"approvals"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("stdout is not the listing document: %v\n%s", err, stdout)
	}
	if doc.Version != 1 {
		t.Errorf("version = %d, want 1", doc.Version)
	}
	if len(doc.Approvals) != 1 {
		t.Fatalf("approvals = %+v, want exactly one", doc.Approvals)
	}
	got := doc.Approvals[0]
	if got.TaskID != "S001" || got.Kind != string(domain.ApprovalNeedsHuman) || got.Status != string(domain.ApprovalPending) {
		t.Errorf("approval = %+v, want S001/NEEDS_HUMAN/PENDING", got)
	}
	if got.Stage != string(runpkg.WaitingForHuman) {
		t.Errorf("stage = %q, want %q", got.Stage, runpkg.WaitingForHuman)
	}
	if got.Reason == "" {
		t.Error("reason is empty; the listing must carry SOP's own reason")
	}
}

// TestApprovalsJSONEmptyIsAnArray proves an empty listing serializes as [] rather
// than null, so a client can iterate it without a nil check.
func TestApprovalsJSONEmptyIsAnArray(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)

	code, stdout, stderr := runCLI(t, dir, "approvals", "--json")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, `"approvals": []`) {
		t.Errorf("stdout = %q, want an empty array", stdout)
	}
}

// TestApprovalsRejectsUnknownArgs proves the listing takes no positional arguments.
func TestApprovalsRejectsUnknownArgs(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)

	code, _, stderr := runCLI(t, dir, "approvals", "S001")
	if code != exitUsage {
		t.Fatalf("code=%d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr, "usage: sop approvals") {
		t.Errorf("stderr = %q", stderr)
	}
}
