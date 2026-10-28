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
	// ApprovalSuperseded: an operator authorized the supersession of a stale
	// PENDING request whose task is already satisfied. It is NOT a human decision:
	// the request could no longer authorize anything (the task had completed), so
	// the operator recorded an audited reason and preserved the original request in
	// the append-only history rather than deciding it. It is resolved, so the shared
	// closure gates treat the gate as leaveable.
	ApprovalSuperseded ApprovalStatus = "SUPERSEDED"
	// ApprovalInvalidated: an operator invalidated a PENDING request because the
	// plan it authorized no longer matches the task definition. It is NOT a human
	// decision and NOT an acceptance: no human approved the request, and the
	// invalidation never authorizes the work. It is a distinct resolved status, so
	// no consumer can read it as APPROVED, DECLINED, or SUPERSEDED. It is resolved
	// because the request can no longer authorize anything, so the shared closure
	// gates treat the gate as leaveable. The audit provenance lives in the
	// ApprovalInvalidation record on the request.
	ApprovalInvalidated ApprovalStatus = "INVALIDATED"
)

// Resolved reports whether a decision has been recorded for the request. A
// SUPERSEDED or INVALIDATED request is resolved: an operator closed it, so no
// decision is pending on it.
func (s ApprovalStatus) Resolved() bool {
	return s == ApprovalApproved || s == ApprovalDeclined || s == ApprovalSuperseded || s == ApprovalInvalidated
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

// ApprovalInvalidation is the durable audit record of an operator invalidation of
// an approval request: the operator's reason and identity, when the invalidation
// happened, the plan source fingerprint on both sides of the revision, when the
// plan was reconciled with the task definition, and the task definition digest on
// both sides of the revision.
//
// It is deliberately separate from ApprovalDecision. It is not a decision, it
// carries no Approved value, and it never records a lifecycle action: an
// invalidation closes the request because the plan it authorized is stale, and it
// authorizes nothing. A request carries this record only when it was invalidated;
// its Status is then ApprovalInvalidated.
type ApprovalInvalidation struct {
	// Reason is the operator's recorded justification for the invalidation.
	Reason string `json:"reason"`
	// InvalidatedBy identifies the operator that invalidated the request.
	InvalidatedBy string `json:"invalidated_by"`
	// InvalidatedAt is when the invalidation was recorded (UTC).
	InvalidatedAt time.Time `json:"invalidated_at"`
	// PlanFingerprintBefore is the plan source fingerprint the request was raised
	// against.
	PlanFingerprintBefore string `json:"plan_fingerprint_before,omitempty"`
	// PlanFingerprintAfter is the plan source fingerprint of the revised plan.
	PlanFingerprintAfter string `json:"plan_fingerprint_after,omitempty"`
	// DefinitionAcceptedAt is when this task's definition change was explicitly
	// accepted (UTC). It is task-scoped acceptance evidence, never the plan-wide
	// write time. It is zero for a legacy acceptance recorded before that evidence
	// existed.
	DefinitionAcceptedAt time.Time `json:"definition_accepted_at"`
	// DefinitionDigestBefore is the task definition digest the request was raised
	// against.
	DefinitionDigestBefore string `json:"definition_digest_before,omitempty"`
	// DefinitionDigestAfter is the task definition digest of the revised definition.
	DefinitionDigestAfter string `json:"definition_digest_after,omitempty"`
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
	// DefinitionDigest is the digest of the task definition the request was raised
	// against, when the definition was known at request time. It is optional: a
	// request may predate a known definition. A later invalidation compares it with
	// the revised definition digest.
	DefinitionDigest string `json:"definition_digest,omitempty"`
	// PlanFingerprint is the active plan source fingerprint the request was raised
	// against, when it was known at request time. It is audit context for a later
	// invalidation; an empty value means the request's plan revision is unknown.
	PlanFingerprint string `json:"plan_fingerprint,omitempty"`
	// RequestedAt is when the request was first recorded (UTC).
	RequestedAt time.Time `json:"requested_at"`
	// RequestedBy identifies who/what raised the request, when known.
	RequestedBy string `json:"requested_by,omitempty"`
	// Status is the current request status
	// (PENDING/APPROVED/DECLINED/SUPERSEDED/INVALIDATED).
	Status ApprovalStatus `json:"status"`
	// Decision is the recorded decision once the request is resolved, nil while
	// pending. A superseded request records the supersession provenance here (the
	// operator, reason, timestamp, and request identity) with Approved=false. An
	// invalidated request leaves Decision nil: an invalidation is not a decision.
	Decision *ApprovalDecision `json:"decision,omitempty"`
	// Invalidation is the audit record of an operator invalidation, nil when the
	// request was never invalidated. It never sets Decision or a LifecycleAction.
	Invalidation *ApprovalInvalidation `json:"invalidation,omitempty"`
}
