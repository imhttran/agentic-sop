package run

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/imhttran/agentic-sop/internal/domain"
)

// These tests pin invariant I1: an external completion supersedes a task's
// PENDING approval head into the append-only history and leaves no actionable
// PENDING head. They are deterministic and use only the run-directory artifact
// I/O (no provider, no clock, no state database).

func pendingApprovalHead(taskID, id string) domain.ApprovalRequest {
	return domain.ApprovalRequest{
		ID:          id,
		TaskID:      taskID,
		Kind:        domain.ApprovalNeedsHuman,
		Reason:      "human decision required",
		RequestedBy: "sop",
		Status:      domain.ApprovalPending,
	}
}

func writeApprovalHead(t *testing.T, dir string, req domain.ApprovalRequest) {
	t.Helper()
	if err := At(dir).SaveApproval(req); err != nil {
		t.Fatalf("seed approval head: %v", err)
	}
}

// Pending approval: it is superseded into history, the head is resolved, and the
// original PENDING snapshot is preserved.
func TestResolvePendingApprovalSupersedesAndPreservesHistory(t *testing.T) {
	dir := t.TempDir()
	writeApprovalHead(t, dir, pendingApprovalHead("T1", "req-1"))

	resolved, err := ResolvePendingApproval(At(dir))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !resolved {
		t.Fatal("want resolved=true for a PENDING head")
	}

	head, ok := At(dir).Approval()
	if !ok {
		t.Fatal("the head artifact must still be present after a supersede")
	}
	if head.Status == domain.ApprovalPending {
		t.Errorf("head status = %s, want a resolved status", head.Status)
	}
	if head.Decision != nil {
		t.Errorf("head decision = %+v, want nil (no decision is synthesized)", head.Decision)
	}

	hist := At(dir).ApprovalHistory()
	if len(hist) != 1 {
		t.Fatalf("history = %+v, want exactly one snapshot", hist)
	}
	if hist[0].ID != "req-1" || hist[0].Status != domain.ApprovalPending {
		t.Errorf("history snapshot = %+v, want the original PENDING req-1 preserved", hist[0])
	}
}

// No pending approval: a task with no recorded request is a no-op.
func TestResolvePendingApprovalNoHeadIsNoop(t *testing.T) {
	dir := t.TempDir()
	resolved, err := ResolvePendingApproval(At(dir))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved {
		t.Fatal("want resolved=false when no head exists")
	}
	if _, ok := At(dir).Approval(); ok {
		t.Error("no head must remain absent")
	}
	if h := At(dir).ApprovalHistory(); len(h) != 0 {
		t.Errorf("history = %+v, want none", h)
	}
}

// Already resolved: a head that is not PENDING is left untouched and no history is
// appended.
func TestResolvePendingApprovalAlreadyResolvedIsNoop(t *testing.T) {
	dir := t.TempDir()
	head := pendingApprovalHead("T1", "req-1")
	head.Status = domain.ApprovalApproved
	writeApprovalHead(t, dir, head)

	resolved, err := ResolvePendingApproval(At(dir))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved {
		t.Fatal("want resolved=false for an already-resolved head")
	}
	if got, _ := At(dir).Approval(); got.Status != domain.ApprovalApproved {
		t.Errorf("head status = %s, want unchanged APPROVED", got.Status)
	}
	if h := At(dir).ApprovalHistory(); len(h) != 0 {
		t.Errorf("history = %+v, want none", h)
	}
}

// Repeated external completion: the second resolution is a no-op and never
// duplicates the history entry.
func TestResolvePendingApprovalRepeatDoesNotDuplicateHistory(t *testing.T) {
	dir := t.TempDir()
	writeApprovalHead(t, dir, pendingApprovalHead("T1", "req-1"))

	if _, err := ResolvePendingApproval(At(dir)); err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	resolved, err := ResolvePendingApproval(At(dir))
	if err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	if resolved {
		t.Fatal("a repeated resolution must be a no-op")
	}
	if h := At(dir).ApprovalHistory(); len(h) != 1 {
		t.Fatalf("history = %+v, want exactly one snapshot", h)
	}
}

// Partial failure: a retry after the history was appended but the head was not
// rewritten must not duplicate the audit entry.
func TestResolvePendingApprovalPartialFailureDoesNotDuplicateHistory(t *testing.T) {
	dir := t.TempDir()
	head := pendingApprovalHead("T1", "req-1")
	writeApprovalHead(t, dir, head)
	// Simulate an earlier attempt that archived the snapshot but died before it could
	// rewrite the head, leaving approval.json still PENDING.
	if err := At(dir).AppendApprovalHistory(head); err != nil {
		t.Fatalf("seed history: %v", err)
	}

	resolved, err := ResolvePendingApproval(At(dir))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !resolved {
		t.Fatal("want resolved=true")
	}
	if h := At(dir).ApprovalHistory(); len(h) != 1 {
		t.Fatalf("history = %+v, want exactly one snapshot (no duplicate)", h)
	}
	if got, _ := At(dir).Approval(); got.Status == domain.ApprovalPending {
		t.Error("head must not remain PENDING")
	}
}

// Approval-history write failure: the resolution errors and leaves the head PENDING
// (no partial supersede).
func TestResolvePendingApprovalHistoryWriteFailure(t *testing.T) {
	dir := t.TempDir()
	writeApprovalHead(t, dir, pendingApprovalHead("T1", "req-1"))
	// Make the history path unwritable by occupying it with a directory.
	if err := os.Mkdir(filepath.Join(dir, approvalHistoryName), 0o755); err != nil {
		t.Fatalf("seed history dir: %v", err)
	}

	if _, err := ResolvePendingApproval(At(dir)); err == nil {
		t.Fatal("want an error when the approval history cannot be written")
	}
	head, ok := At(dir).Approval()
	if !ok || head.Status != domain.ApprovalPending {
		t.Errorf("head = %+v, want unchanged PENDING", head)
	}
}

// Approval-head write failure: the history snapshot is appended, then the head write
// fails, so the resolution errors while preserving the audit trail.
func TestResolvePendingApprovalHeadWriteFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root: a read-only file is still writable")
	}
	dir := t.TempDir()
	writeApprovalHead(t, dir, pendingApprovalHead("T1", "req-1"))
	headPath := filepath.Join(dir, approvalArtifactName)
	if err := os.Chmod(headPath, 0o444); err != nil {
		t.Fatalf("chmod head read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(headPath, 0o644) })

	if _, err := ResolvePendingApproval(At(dir)); err == nil {
		t.Fatal("want an error when the approval head cannot be written")
	}
	// The audit snapshot was appended before the head write failed.
	if h := At(dir).ApprovalHistory(); len(h) != 1 || h[0].ID != "req-1" {
		t.Errorf("history = %+v, want the archived snapshot preserved", h)
	}
	// The head is still the original PENDING request (the failed write did not apply).
	if head, ok := At(dir).Approval(); !ok || head.Status != domain.ApprovalPending {
		t.Errorf("head = %+v, want unchanged PENDING", head)
	}
}
