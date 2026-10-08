package run

import (
	"github.com/imhttran/agentic-sop/internal/domain"
)

// External-completion approval resolution (invariant I1).
//
// An external completion records that a task's work already exists, performed
// outside the SOP execution. Any approval request still PENDING for that task can
// no longer authorize anything, so leaving it as the head would break invariant I1
// (a completed task must not retain an actionable PENDING approval head).
// ResolvePendingApproval supersedes such a head: it archives the request verbatim
// into the append-only approval-history.json and then rewrites the head with a
// resolved status, so approval.json no longer reports PENDING while the original
// PENDING snapshot is preserved in the audit trail.
//
// A superseded gate is not a human decision. The domain defines exactly three
// statuses (PENDING/APPROVED/DECLINED) and Resolved() covers the latter two, so the
// head is recorded as DECLINED with a nil Decision — no decision is synthesized.
// The archived snapshot keeps the original PENDING status, so the history shows the
// request was superseded by an external completion, not decided.
//
// It is idempotent: when there is no head, or the head is already resolved, it is a
// no-op and appends nothing. When a snapshot with the same request id is already in
// the history (for example after a partial failure of an earlier attempt), it does
// not append a duplicate. It reads and writes only the task's own run directory
// (approval.json and approval-history.json) and touches no other artifact.

// ResolvePendingApproval supersedes a task's PENDING approval head into
// approval-history.json and rewrites the head with a resolved status. It returns
// true when a resolution was performed and false on the idempotent no-op path.
func ResolvePendingApproval(r *Run) (bool, error) {
	if r == nil {
		return false, nil
	}
	head, ok := r.Approval()
	if !ok {
		// No request recorded: nothing to resolve.
		return false, nil
	}
	if head.Status != domain.ApprovalPending {
		// Already resolved (or otherwise not PENDING): a repeat external completion
		// must be a no-op and must never duplicate history.
		return false, nil
	}
	// Archive the original PENDING snapshot into the append-only history, unless the
	// same request id is already recorded (a retry after a partial failure must not
	// duplicate the audit entry).
	if !approvalHistoryHasRequest(r, head.ID) {
		if err := r.AppendApprovalHistory(head); err != nil {
			return false, err
		}
	}
	// Rewrite the head with a resolved status and no decision, so approval.json no
	// longer reports PENDING while the history keeps the original request. This
	// second write is what makes the supersede durable; a failure here leaves the
	// head PENDING and is reported to the caller.
	head.Status = domain.ApprovalDeclined
	head.Decision = nil
	if err := r.SaveApproval(head); err != nil {
		return false, err
	}
	return true, nil
}

// approvalHistoryHasRequest reports whether the append-only approval history
// already contains a snapshot with the given request id. It is a pure read and
// makes resolution idempotent across a partial failure (history appended, head not
// yet rewritten).
func approvalHistoryHasRequest(r *Run, id string) bool {
	if id == "" {
		return false
	}
	for _, req := range r.ApprovalHistory() {
		if req.ID == id {
			return true
		}
	}
	return false
}
