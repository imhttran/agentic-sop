package domain

import "time"

// ApprovalStatus is the lifecycle of an explicit human approval request.
//
// It is deliberately separate from TaskStatus: an approval request records that
// SOP is (or was) waiting at a human gate, while the task status records where
// the task is in the implementation lifecycle. A BLOCKED or NEEDS_HUMAN task that
// has no approval request is not asking for approval.
type ApprovalStatus string

const (
	// ApprovalPending: recorded and awaiting a human decision.
	ApprovalPending ApprovalStatus = "PENDING"
	// ApprovalApproved: a human approved the request.
	ApprovalApproved ApprovalStatus = "APPROVED"
	// ApprovalDeclined: a human declined the request.
	ApprovalDeclined ApprovalStatus = "DECLINED"
)

// Resolved reports whether a decision has been recorded for the request.
func (s ApprovalStatus) Resolved() bool {
	return s == ApprovalApproved || s == ApprovalDeclined
}

// ApprovalKind names why SOP parked the task at a human gate. It is SOP's own
// classification, never inferred by a client from a task status.
type ApprovalKind string

const (
	// ApprovalNeedsHuman: the autonomy policy required a human decision (a genuine
	// authority boundary — destructive/irreversible/security authorization or
	// unresolved ambiguity), so SOP recorded an explicit, resolvable request rather
	// than only a blocked status.
	ApprovalNeedsHuman ApprovalKind = "NEEDS_HUMAN"
)

// Approval lifecycle action labels recorded with a decision. They name what SOP
// did through its existing lifecycle authority as a result of the decision; they
// are not state transitions of their own.
const (
	// ApprovalActionNone: no lifecycle action was required.
	ApprovalActionNone = "NONE"
	// ApprovalActionRequeued: the task was returned to PLANNED so the next run
	// resumes it.
	ApprovalActionRequeued = "REQUEUED"
	// ApprovalActionAlreadyRunnable: the task was already PLANNED, so the decision
	// only recorded provenance.
	ApprovalActionAlreadyRunnable = "ALREADY_RUNNABLE"
)

// ApprovalDecision is the recorded human decision on one approval request. It is
// audit provenance: who decided, when, and what SOP did as a result.
type ApprovalDecision struct {
	// RequestID is the approval request the decision resolved.
	RequestID string `json:"request_id"`
	// TaskID is the task the decision concerns.
	TaskID string `json:"task_id"`
	// Approved is true for an approval and false for a decline.
	Approved bool `json:"approved"`
	// DecidedAt is when the decision was recorded (UTC).
	DecidedAt time.Time `json:"decided_at"`
	// DecidedBy identifies the human/client that made the decision, when supplied.
	DecidedBy string `json:"decided_by,omitempty"`
	// Note is optional free-form context recorded with the decision.
	Note string `json:"note,omitempty"`
	// LifecycleAction is the short label of what SOP did in response (one of the
	// ApprovalAction* constants).
	LifecycleAction string `json:"lifecycle_action,omitempty"`
}

// ApprovalRequest is SOP's authoritative, persisted representation of one human
// approval gate. It exists so a client resolves a gate by asking SOP (and by
// acting on this record) rather than inferring one from BLOCKED, NEEDS_HUMAN, or
// WAITING_FOR_HUMAN. A task in one of those states with no ApprovalRequest is not
// an approval boundary.
type ApprovalRequest struct {
	// ID is the unique identity of this request. A new gate produces a new ID, so
	// a decision can never be applied to a different gate than the one requested.
	ID string `json:"id"`
	// TaskID is the task the gate concerns.
	TaskID string `json:"task_id"`
	// Kind is SOP's reason category for the gate.
	Kind ApprovalKind `json:"kind"`
	// Target names what the gate concerns (the task id).
	Target string `json:"target"`
	// Reason is the short authoritative reason the gate was raised.
	Reason string `json:"reason,omitempty"`
	// Evidence is the supporting detail (validation/review/classification text).
	Evidence string `json:"evidence,omitempty"`
	// Stage is the run lifecycle stage at request time.
	Stage string `json:"stage,omitempty"`
	// Disposition is the failure-classification disposition at request time.
	Disposition string `json:"disposition,omitempty"`
	// RequestedAt is when the request was first recorded (UTC).
	RequestedAt time.Time `json:"requested_at"`
	// RequestedBy identifies who/what raised the request, when known.
	RequestedBy string `json:"requested_by,omitempty"`
	// Status is the current request status (PENDING/APPROVED/DECLINED).
	Status ApprovalStatus `json:"status"`
	// Decision is the recorded decision once the request is resolved, nil while
	// pending.
	Decision *ApprovalDecision `json:"decision,omitempty"`
}
