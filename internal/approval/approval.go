// Package approval is SOP's human approval application boundary: the single,
// authoritative place a client reads an active human approval gate and records a
// decision on it.
//
// It exists because a human gate is a SOP lifecycle decision, not something a
// client may manufacture. A client (the CLI, a controller, MCP, or a future
// consumer) never infers approval from a BLOCKED/NEEDS_HUMAN/WAITING_FOR_HUMAN
// task status, and never mutates SOP state directly: it reads SOP's explicit
// approval request and asks SOP to record a decision through this boundary.
//
// SOP remains the lifecycle authority. Approving does not force a task PASS and
// does not bypass validation, review, JEV, or the quality gate; it resolves only
// the specific gate that raised the request, and returns the task to runnable
// work through the domain's existing requeue operation. Declining preserves the
// truthful lifecycle state and never manufactures completion.
//
// The service is deterministic and I/O-free itself: persistence and task state
// are injected, so a decision is unit-testable without a database, a clock, or a
// model.
package approval

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/store"
)

var (
	// ErrApprovalNotRequested is returned when a task has no approval request to
	// resolve. A BLOCKED or NEEDS_HUMAN task with no explicit request is not an
	// approval boundary, so it must not be approvable.
	ErrApprovalNotRequested = errors.New("no approval request pending")
	// ErrApprovalResolved is returned when a decision is recorded against a
	// request that was already resolved the other way (an approve on a declined
	// request, or a decline on an approved one). Repeating the same decision is
	// idempotent and succeeds.
	ErrApprovalResolved = errors.New("approval request already resolved")
	// ErrApprovalNotApplicable is returned when a pending request is no longer
	// applicable — for example the task completed after the request was raised, so
	// the stale request can no longer authorize anything.
	ErrApprovalNotApplicable = errors.New("approval request is no longer applicable")
	// ErrApprovalSupersedeInput is returned when a supersession is missing its
	// required operator authorization: a non-empty reason and an identifiable
	// operator are both mandatory.
	ErrApprovalSupersedeInput = errors.New("approval supersede requires a reason and an operator")
	// ErrApprovalInvalidateInput is returned when an invalidation is missing its
	// required operator authorization: a non-empty reason and an identifiable
	// operator are both mandatory.
	ErrApprovalInvalidateInput = errors.New("approval invalidate requires a reason and an operator")
	// ErrApprovalInvalidateNotApplicable is returned when an invalidation is
	// refused because the request cannot be invalidated: the head is not PENDING,
	// the task is satisfied (the supersede operation applies instead), or the plan
	// revision is not proven to have changed the task definition. It is separate
	// from ErrApprovalNotApplicable so a caller can tell a stale-decision refusal
	// from an invalidate refusal.
	ErrApprovalInvalidateNotApplicable = errors.New("approval invalidation is not applicable")
)

// Record persists one task's approval request head plus its append-only history.
// The run package's *run.Run implements it.
type Record interface {
	Approval() (domain.ApprovalRequest, bool)
	SaveApproval(domain.ApprovalRequest) error
	AppendApprovalHistory(domain.ApprovalRequest) error
	ApprovalHistory() []domain.ApprovalRequest
}

// Tasks is the task-store slice the boundary needs: to validate that a request
// belongs to a real, non-stale task, and to resume it through the domain
// lifecycle. *store.Store implements it.
type Tasks interface {
	Get(id string) (*domain.Task, error)
	Save(task *domain.Task) error
}

// Service is the human approval application boundary.
type Service struct {
	tasks  Tasks
	record func(taskID string) Record
	now    func() time.Time
}

// New returns a Service over a task store and a per-task record factory. A nil
// task store is permitted for callers that only record requests (the lifecycle),
// but approval decisions validate the task, so production callers supply one.
func New(tasks Tasks, record func(taskID string) Record) *Service {
	return &Service{tasks: tasks, record: record, now: time.Now}
}

// View is the read projection of a task's approval boundary: the present-or-
// absent gate plus SOP's authoritative fields, shaped so a client can render it
// without classifying SOP state itself.
type View struct {
	// Present is true only when SOP has recorded an approval request for the task.
	// It is the source of truth for whether an approval boundary exists; a task
	// status alone never makes it true.
	Present bool
	// Applicable is true when a decision may actually be recorded: the request is
	// present and still PENDING.
	Applicable bool
	// TaskID is the task the boundary concerns.
	TaskID string
	// Kind is SOP's reason category for the gate.
	Kind domain.ApprovalKind
	// Target names what the gate concerns.
	Target string
	// Reason is the short authoritative reason the gate was raised.
	Reason string
	// Evidence is the supporting detail behind the gate.
	Evidence string
	// Stage is the run lifecycle stage at request time.
	Stage string
	// Disposition is the failure-classification disposition at request time.
	Disposition string
	// Status is the request status (PENDING/APPROVED/DECLINED/SUPERSEDED).
	Status domain.ApprovalStatus
	// RequestedAt is when the request was first recorded.
	RequestedAt time.Time
	// RequestedBy identifies who/what raised the request.
	RequestedBy string
	// DecidedAt is when the decision (or supersession) was recorded, if resolved.
	DecidedAt time.Time
	// DecidedBy identifies who decided (or superseded), if resolved.
	DecidedBy string
	// Note is the optional decision (or supersession) context, if resolved.
	Note string
	// LifecycleAction is what SOP did in response, if resolved.
	LifecycleAction string
	// Superseded is true when the head was superseded by an operator rather than
	// decided by a human. It disambiguates a resolved head without overloading
	// LifecycleAction or Approved.
	Superseded bool
	// Invalidated is true when the head was invalidated by an operator because the
	// plan it authorized no longer matches the task definition. It is not a
	// decision: it authorizes nothing, so a consumer never reads it as applicable.
	Invalidated bool
	// DefinitionDigest is the task definition digest the request was raised
	// against, when it was known at request time. It is empty for a legacy request.
	DefinitionDigest string
	// Invalidation is the recorded invalidation provenance, nil unless the head was
	// invalidated. It carries the reason, actor, plan revisions, and both definition
	// digests so a caller can report the decision without reading the artifact.
	Invalidation *domain.ApprovalInvalidation
	// TaskStatus is the task's current persisted status.
	TaskStatus domain.TaskStatus
}

// RequestInput describes a human gate SOP is recording.
type RequestInput struct {
	TaskID      string
	Kind        domain.ApprovalKind
	Target      string
	Reason      string
	Evidence    string
	Stage       string
	Disposition string
	RequestedBy string
	// DefinitionDigest is the digest of the task definition the request is raised
	// against, when the caller knows it. A request that records it lets a future
	// invalidation decide applicability precisely instead of falling back to
	// reconciliation provenance. An empty digest keeps the request legacy.
	DefinitionDigest string
	// PlanFingerprint is the active plan source fingerprint the request is raised
	// against, when the caller knows it. It is audit context for a later
	// invalidation. An empty fingerprint keeps the request's revision unknown.
	PlanFingerprint string
}

// DecisionInput is the metadata a client records with a decision.
type DecisionInput struct {
	// By identifies the human/client making the decision.
	By string
	// Note is optional free-form context.
	Note string
}

// SupersedeInput is the operator authorization for superseding a stale PENDING
// approval request whose task is already satisfied. Both fields are mandatory:
// a supersession is an explicit, audited operator action, not an implicit one.
type SupersedeInput struct {
	// Reason is the audited justification for the supersession (required).
	Reason string
	// By identifies the operator authorizing the supersession (required).
	By string
}

// InvalidateInput is the operator authorization and revision evidence for
// invalidating a PENDING approval request because the plan it authorized no
// longer matches the task definition. The service is deterministic and I/O-free:
// every value below is supplied by the caller (the run package resolves the
// persisted definition and reconciliation provenance), and Invalidate writes no
// task field and reads no clock, store, or plan beyond what the Service already
// injects.
//
// Guard contract (fixed order, fail closed, nothing written on refusal):
//
//  1. Reason and By must be non-empty, else ErrApprovalInvalidateInput.
//  2. The head must exist for this task and be PENDING; an APPROVED, DECLINED,
//     SUPERSEDED, or INVALIDATED head is refused with ErrApprovalResolved naming
//     the recorded status, so a recorded human decision is never overwritten.
//  3. A satisfied task is refused with ErrApprovalInvalidateNotApplicable naming
//     the supersede operation, so a satisfied task is never invalidated into
//     re-execution.
//  4. The definition must be proven to have changed after the request was raised.
//     When the head records a DefinitionDigest, CurrentDefinitionDigest must
//     differ from it (an equal digest refuses). When the head records no digest
//     (a legacy request), the caller must prove the definition was accepted as
//     reconciled after the request: DefinitionAcceptedAsReconciled must be true,
//     and ReconciledAt must be strictly after head.RequestedAt. Both the recorded
//     and the current digest must be present. Otherwise it refuses with
//     ErrApprovalInvalidateNotApplicable.
//
// On success the original PENDING head is appended verbatim to the append-only
// history exactly once (deduped by request id on retry, reusing
// historyHasRequest) before the head is rewritten to INVALIDATED with an
// ApprovalInvalidation record carrying every supplied field. Decision stays nil:
// an invalidation is not a decision and authorizes nothing.
type InvalidateInput struct {
	// Reason is the audited justification for the invalidation (required).
	Reason string
	// By identifies the operator authorizing the invalidation (required).
	By string
	// CurrentPlanFingerprint is the plan source fingerprint of the revised plan.
	CurrentPlanFingerprint string
	// PriorPlanFingerprint is the plan source fingerprint the request was raised
	// against. When empty, the service cannot recover it from the head, so the
	// recorded value is reported as empty.
	PriorPlanFingerprint string
	// CurrentDefinitionDigest is the task definition digest of the revised
	// definition. It is compared with the head's recorded digest when that digest
	// is non-empty.
	CurrentDefinitionDigest string
	// DefinitionAcceptedAsReconciled is true when the caller proved that the
	// reconciled definition change was explicitly accepted after the request was
	// raised. It is required for a legacy head (no recorded digest).
	DefinitionAcceptedAsReconciled bool
	// ReconciledAt is when the plan was reconciled with the task definition (UTC).
	// For a legacy head it must be strictly after the request's RequestedAt.
	ReconciledAt time.Time
}

// Result is the outcome of recording a decision.
type Result struct {
	// View is the resulting approval boundary projection.
	View View
	// Idempotent is true when the same decision had already been recorded, so the
	// call changed nothing.
	Idempotent bool
}

// Request records (or refreshes) the task's explicit approval request. It is
// idempotent for the same still-pending gate: a re-park refreshes the context and
// keeps the same request id. A request after a resolved one, or for a different
// kind of gate, supersedes the previous head into the history and starts a new
// request, so a stale request can never resolve a newer gate.
func (s *Service) Request(in RequestInput) (domain.ApprovalRequest, error) {
	taskID := strings.TrimSpace(in.TaskID)
	if taskID == "" {
		return domain.ApprovalRequest{}, errors.New("approval: task id is required")
	}
	kind := in.Kind
	if kind == "" {
		kind = domain.ApprovalNeedsHuman
	}

	rec := s.record(taskID)
	if head, ok := rec.Approval(); ok {
		if head.Status == domain.ApprovalPending && head.Kind == kind {
			// The same gate is still open: refresh its context, keep its identity.
			head.Target = in.Target
			head.Reason = in.Reason
			head.Evidence = in.Evidence
			head.Stage = in.Stage
			head.Disposition = in.Disposition
			if d := strings.TrimSpace(in.DefinitionDigest); d != "" {
				head.DefinitionDigest = d
			}
			if in.RequestedBy != "" {
				head.RequestedBy = in.RequestedBy
			}
			if err := rec.SaveApproval(head); err != nil {
				return domain.ApprovalRequest{}, err
			}
			return head, nil
		}
		// The previous gate is stale: keep it as provenance before replacing it, but
		// only archive it if it was never resolved (a resolved request was already
		// archived when its decision was recorded).
		if !head.Status.Resolved() {
			if err := rec.AppendApprovalHistory(head); err != nil {
				return domain.ApprovalRequest{}, err
			}
		}
	}

	req := domain.ApprovalRequest{
		ID:               s.newID(taskID),
		TaskID:           taskID,
		Kind:             kind,
		Target:           in.Target,
		Reason:           in.Reason,
		Evidence:         in.Evidence,
		Stage:            in.Stage,
		Disposition:      in.Disposition,
		DefinitionDigest: strings.TrimSpace(in.DefinitionDigest),
		PlanFingerprint:  strings.TrimSpace(in.PlanFingerprint),
		RequestedAt:      s.now().UTC(),
		RequestedBy:      in.RequestedBy,
		Status:           domain.ApprovalPending,
	}
	if err := rec.SaveApproval(req); err != nil {
		return domain.ApprovalRequest{}, err
	}
	return req, nil
}

// Approval returns the task's approval boundary projection. A task with no
// recorded request reports Present=false; a missing task is reported as not
// found.
func (s *Service) Approval(taskID string) (View, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return View{}, errors.New("approval: task id is required")
	}

	task, err := s.task(taskID)
	if err != nil {
		return View{}, err
	}

	head, ok := s.record(taskID).Approval()
	if !ok {
		return View{TaskID: taskID, TaskStatus: statusOf(task)}, nil
	}
	return s.view(head, task), nil
}

// Approve records an approval of the task's active request and resumes the task
// through the domain's existing requeue operation when it is blocked. It is
// idempotent: approving an already-approved request succeeds without change.
func (s *Service) Approve(taskID string, in DecisionInput) (Result, error) {
	return s.decide(taskID, true, in)
}

// Decline records a decline of the task's active request. It preserves the
// truthful lifecycle state and never manufactures completion.
func (s *Service) Decline(taskID string, in DecisionInput) (Result, error) {
	return s.decide(taskID, false, in)
}

// Supersede is the explicit, operator-authorized supersession of a stale PENDING
// approval request whose task is already satisfied. It exists because such a
// request can neither be decided (the approval boundary refuses a decision on a
// satisfied task with ErrApprovalNotApplicable) nor externally completed (external
// completion is rejected on an already-complete task), so without an explicit
// supersession the shared closure gates (LC-004) would block a state an operator
// cannot leave by any supported operation.
//
// It is permitted ONLY for a PENDING head whose task is satisfied. A pending
// request on an unsatisfied task is refused (it must be decided, not superseded),
// and a non-pending head is refused. It requires both an explicit reason and an
// identifiable operator.
//
// Persistence is fail-closed and ordered: the original PENDING request is appended
// verbatim to the append-only approval-history.json first, then the head is
// rewritten to SUPERSEDED. A history-append failure returns the error and does not
// touch the head; a head-write failure returns the error and reports no successful
// supersession, leaving the head PENDING (the appended history entry is the only
// residual and is deduplicated by the idempotency check on retry). A repeated call
// on an already-superseded head is idempotent: it appends no duplicate history entry
// and does not change the head.
func (s *Service) Supersede(taskID string, in SupersedeInput) (Result, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return Result{}, errors.New("approval: task id is required")
	}
	reason := strings.TrimSpace(in.Reason)
	by := strings.TrimSpace(in.By)
	if reason == "" || by == "" {
		return Result{}, ErrApprovalSupersedeInput
	}

	rec := s.record(taskID)
	head, ok := rec.Approval()
	if !ok || head.TaskID != taskID {
		// No request, or a record that is not this task's: never superseded.
		return Result{}, fmt.Errorf("%w for task %s", ErrApprovalNotRequested, taskID)
	}

	task, err := s.task(taskID)
	if err != nil {
		return Result{}, err
	}

	// An already-superseded head is idempotent: report provenance, write nothing.
	if head.Status == domain.ApprovalSuperseded {
		return Result{View: s.view(head, task), Idempotent: true}, nil
	}
	// Only a PENDING request can be superseded. An APPROVED/DECLINED head is a
	// recorded human decision and must not be overwritten by a supersession.
	if head.Status != domain.ApprovalPending {
		return Result{}, fmt.Errorf("%w as %s for task %s", ErrApprovalResolved, head.Status, taskID)
	}
	// Supersession is permitted only when the task is satisfied: a pending request
	// on live work must be decided, not superseded.
	if task == nil || !task.IsSatisfied() {
		return Result{}, fmt.Errorf("%w for task %s (the request must be decided, not superseded)", ErrApprovalNotApplicable, taskID)
	}

	// Step 1: preserve the original PENDING request verbatim in the append-only
	// history, unless a snapshot with the same request id is already recorded (a
	// retry after a partial failure must not duplicate the audit entry).
	if !s.historyHasRequest(rec, head.ID) {
		if err := rec.AppendApprovalHistory(head); err != nil {
			// Fail closed: the head is untouched and still PENDING.
			return Result{}, err
		}
	}

	// Step 2: rewrite the head to SUPERSEDED, recording the operator, reason,
	// timestamp, and original request identity. This second write is what makes the
	// supersession durable; a failure here is reported and leaves the head PENDING.
	head.Status = domain.ApprovalSuperseded
	head.Decision = &domain.ApprovalDecision{
		RequestID: head.ID,
		TaskID:    taskID,
		// A supersession is not an approval: Approved is false, consistent with the
		// existing Decision fields. The SUPERSEDED status and LifecycleAction=NONE
		// disambiguate it from a decline.
		Approved:        false,
		DecidedAt:       s.now().UTC(),
		DecidedBy:       by,
		Note:            reason,
		LifecycleAction: domain.ApprovalActionNone,
	}
	if err := rec.SaveApproval(head); err != nil {
		return Result{}, err
	}
	return Result{View: s.view(head, task)}, nil
}

// Invalidate is the explicit, operator-authorized invalidation of a PENDING
// approval request because the plan it authorized no longer matches the task
// definition after an accepted plan revision. It exists because such a request
// can no longer authorize the work it was raised for, and it must be closed
// without being read as an approval, a decline, or a supersession.
//
// It is permitted ONLY for a PENDING head on an unsatisfied task, when the caller
// proves the definition changed after the request was raised (see InvalidateInput).
// It requires both an explicit reason and an identifiable operator. It records no
// task field: the task's status, attempt, satisfaction, and completion are
// unchanged, so nothing is approved, satisfied, or completed.
//
// Persistence is fail-closed and ordered, mirroring Supersede: the original
// PENDING request is appended verbatim to the append-only history first (deduped
// by request id so a retry appends no duplicate), then the head is rewritten to
// INVALIDATED with the audit record. A history-append failure returns the error
// and leaves the head PENDING. A repeated call on an already-invalidated head is
// idempotent: it appends no duplicate history entry and does not change the head.
func (s *Service) Invalidate(taskID string, in InvalidateInput) (Result, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return Result{}, errors.New("approval: task id is required")
	}
	reason := strings.TrimSpace(in.Reason)
	by := strings.TrimSpace(in.By)
	if reason == "" || by == "" {
		return Result{}, ErrApprovalInvalidateInput
	}

	rec := s.record(taskID)
	head, ok := rec.Approval()
	if !ok || head.TaskID != taskID {
		// No request, or a record that is not this task's: never invalidated.
		return Result{}, fmt.Errorf("%w for task %s", ErrApprovalNotRequested, taskID)
	}

	task, err := s.task(taskID)
	if err != nil {
		return Result{}, err
	}

	// An already-invalidated head is idempotent: report provenance, write nothing.
	if head.Status == domain.ApprovalInvalidated {
		return Result{View: s.view(head, task), Idempotent: true}, nil
	}
	// Only a PENDING request can be invalidated. An APPROVED/DECLINED head records a
	// human decision and must not be overwritten; a SUPERSEDED head was already
	// closed by an operator.
	if head.Status != domain.ApprovalPending {
		return Result{}, fmt.Errorf("%w as %s for task %s", ErrApprovalResolved, head.Status, taskID)
	}
	// A satisfied task is never invalidated into re-execution: the supersede
	// operation is the correct action for a completed task's stale request.
	if task != nil && task.IsSatisfied() {
		return Result{}, fmt.Errorf("%w for task %s (task is %s; use supersede instead)", ErrApprovalInvalidateNotApplicable, taskID, task.Status)
	}
	// Refuse unless the definition is proven to have changed after the request was
	// raised, so a still-current definition is never invalidated.
	if err := invalidationProvesRevision(head, in); err != nil {
		return Result{}, err
	}

	// Step 1: preserve the original PENDING request verbatim in the append-only
	// history, unless a snapshot with the same request id is already recorded (a
	// retry after a partial failure must not duplicate the audit entry).
	if !s.historyHasRequest(rec, head.ID) {
		if err := rec.AppendApprovalHistory(head); err != nil {
			// Fail closed: the head is untouched and still PENDING.
			return Result{}, err
		}
	}

	// Step 2: rewrite the head to INVALIDATED with the audit record. Decision stays
	// nil: an invalidation is not a decision and authorizes nothing.
	head.Status = domain.ApprovalInvalidated
	head.Invalidation = &domain.ApprovalInvalidation{
		Reason:                 reason,
		InvalidatedBy:          by,
		InvalidatedAt:          s.now().UTC(),
		PlanFingerprintBefore:  firstNonEmptyString(in.PriorPlanFingerprint, head.PlanFingerprint),
		PlanFingerprintAfter:   strings.TrimSpace(in.CurrentPlanFingerprint),
		ReconciledAt:           in.ReconciledAt.UTC(),
		DefinitionDigestBefore: head.DefinitionDigest,
		DefinitionDigestAfter:  strings.TrimSpace(in.CurrentDefinitionDigest),
	}
	if err := rec.SaveApproval(head); err != nil {
		return Result{}, err
	}
	return Result{View: s.view(head, task)}, nil
}

// invalidationProvesRevision reports whether in proves the task definition was
// accepted as changed after head was raised. It encodes the guard contract in
// InvalidateInput: an equal recorded digest refuses, and a legacy head (no
// recorded digest) requires an accepted-as-reconciled proof strictly after the
// request. Both digests must be present so the audit record is complete.
func invalidationProvesRevision(head domain.ApprovalRequest, in InvalidateInput) error {
	current := strings.TrimSpace(in.CurrentDefinitionDigest)
	if current == "" {
		return fmt.Errorf("%w for task %s (the current definition digest is required)", ErrApprovalInvalidateNotApplicable, head.TaskID)
	}
	recorded := strings.TrimSpace(head.DefinitionDigest)
	if recorded != "" {
		// A stamped request: the current definition must actually differ.
		if recorded == current {
			return fmt.Errorf("%w for task %s (the definition digest is unchanged)", ErrApprovalInvalidateNotApplicable, head.TaskID)
		}
		return nil
	}
	// A legacy request: prove the reconciled definition change was accepted after
	// the request was raised.
	if !in.DefinitionAcceptedAsReconciled {
		return fmt.Errorf("%w for task %s (a legacy request requires reconciliation provenance)", ErrApprovalInvalidateNotApplicable, head.TaskID)
	}
	if in.ReconciledAt.IsZero() || !in.ReconciledAt.After(head.RequestedAt) {
		return fmt.Errorf("%w for task %s (reconciliation must be accepted after the request was raised)", ErrApprovalInvalidateNotApplicable, head.TaskID)
	}
	return nil
}

// firstNonEmptyString returns the first value with non-space content, so an
// audit field prefers the caller's explicit revision evidence and falls back to
// what SOP itself recorded on the request.
func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

// historyHasRequest reports whether the append-only history already contains a
// snapshot with the given request id. It makes supersession idempotent across a
// partial failure (history appended, head not yet rewritten).
func (s *Service) historyHasRequest(rec Record, id string) bool {
	if id == "" {
		return false
	}
	for _, req := range rec.ApprovalHistory() {
		if req.ID == id {
			return true
		}
	}
	return false
}

func (s *Service) decide(taskID string, approve bool, in DecisionInput) (Result, error) {
	taskID = strings.TrimSpace(taskID)
	if taskID == "" {
		return Result{}, errors.New("approval: task id is required")
	}

	rec := s.record(taskID)
	head, ok := rec.Approval()
	if !ok || head.TaskID != taskID {
		// No request, or a record that is not this task's: never approved.
		return Result{}, fmt.Errorf("%w for task %s", ErrApprovalNotRequested, taskID)
	}

	task, err := s.task(taskID)
	if err != nil {
		return Result{}, err
	}

	switch head.Status {
	case domain.ApprovalApproved:
		if approve {
			return Result{View: s.view(head, task), Idempotent: true}, nil
		}
		return Result{}, fmt.Errorf("%w as APPROVED for task %s", ErrApprovalResolved, taskID)
	case domain.ApprovalDeclined:
		if !approve {
			return Result{View: s.view(head, task), Idempotent: true}, nil
		}
		return Result{}, fmt.Errorf("%w as DECLINED for task %s", ErrApprovalResolved, taskID)
	case domain.ApprovalSuperseded:
		// A superseded request is resolved: it can no longer be decided, and
		// repeating any decision is refused rather than silently re-resolved.
		return Result{}, fmt.Errorf("%w as SUPERSEDED for task %s", ErrApprovalResolved, taskID)
	}

	// A pending request whose task has since completed is stale: it can no longer
	// authorize anything. Archive it rather than applying a decision to it.
	if task != nil && task.IsSatisfied() {
		_ = rec.AppendApprovalHistory(head)
		return Result{}, fmt.Errorf("%w for task %s (task is %s)", ErrApprovalNotApplicable, taskID, task.Status)
	}

	action := domain.ApprovalActionNone
	if approve && task != nil && task.Status == domain.BLOCKED {
		// Resume through the domain's existing requeue operation — the same
		// lifecycle authority `sop retry` uses — rather than writing status
		// directly. It does not spend a retry: an approval is a human decision, not
		// a repeated failed attempt.
		staged := *task
		if err := staged.RequeueWithoutSpending(); err != nil {
			return Result{}, err
		}
		if err := s.tasks.Save(&staged); err != nil {
			return Result{}, err
		}
		task = &staged
		action = domain.ApprovalActionRequeued
	} else if approve && task != nil && task.IsRunnable() {
		// A productive human-boundary outcome already returned the task to PLANNED;
		// the decision records the authorization as provenance.
		action = domain.ApprovalActionAlreadyRunnable
	}

	if approve {
		head.Status = domain.ApprovalApproved
	} else {
		head.Status = domain.ApprovalDeclined
	}
	head.Decision = &domain.ApprovalDecision{
		RequestID:       head.ID,
		TaskID:          taskID,
		Approved:        approve,
		DecidedAt:       s.now().UTC(),
		DecidedBy:       strings.TrimSpace(in.By),
		Note:            strings.TrimSpace(in.Note),
		LifecycleAction: action,
	}
	if err := rec.SaveApproval(head); err != nil {
		return Result{}, err
	}
	if err := rec.AppendApprovalHistory(head); err != nil {
		return Result{}, err
	}
	return Result{View: s.view(head, task)}, nil
}

// task loads the task through the store, mapping a missing task to
// ErrApprovalNotRequested: an approval boundary cannot exist for a task SOP does
// not know.
func (s *Service) task(taskID string) (*domain.Task, error) {
	if s.tasks == nil {
		return nil, nil
	}
	task, err := s.tasks.Get(taskID)
	if err != nil {
		if store.IsNotFound(err) {
			return nil, fmt.Errorf("%w for task %s", ErrApprovalNotRequested, taskID)
		}
		return nil, err
	}
	return task, nil
}

// view builds the read projection. Applicable is derived only from SOP's own
// record: a gate can be acted on only while it is present and pending.
func (s *Service) view(req domain.ApprovalRequest, task *domain.Task) View {
	v := View{
		Present: true,
		// A decision may be recorded only while the request is pending AND its task can
		// still act on it: a request whose task already completed is stale, and decide()
		// refuses it with ErrApprovalNotApplicable. Reporting it as applicable would
		// advertise a decision the boundary would reject. A nil task (a record-only
		// service) has nothing to be stale against, so the pending request stands.
		Applicable:       req.Status == domain.ApprovalPending && (task == nil || !task.IsSatisfied()),
		TaskID:           req.TaskID,
		Kind:             req.Kind,
		Target:           req.Target,
		Reason:           req.Reason,
		Evidence:         req.Evidence,
		Stage:            req.Stage,
		Disposition:      req.Disposition,
		Status:           req.Status,
		RequestedAt:      req.RequestedAt,
		RequestedBy:      req.RequestedBy,
		Superseded:       req.Status == domain.ApprovalSuperseded,
		Invalidated:      req.Status == domain.ApprovalInvalidated,
		DefinitionDigest: req.DefinitionDigest,
		Invalidation:     req.Invalidation,
		TaskStatus:       statusOf(task),
	}
	if d := req.Decision; d != nil {
		v.DecidedAt = d.DecidedAt
		v.DecidedBy = d.DecidedBy
		v.Note = d.Note
		v.LifecycleAction = d.LifecycleAction
	}
	return v
}

// newID mints a fresh request id. The timestamp guarantees a new gate never
// reuses a previous request's id, so a decision cannot be misapplied.
func (s *Service) newID(taskID string) string {
	return fmt.Sprintf("%s-%d", taskID, s.now().UTC().UnixNano())
}

// statusOf returns a task's status, or "" when there is no task.
func statusOf(task *domain.Task) domain.TaskStatus {
	if task == nil {
		return ""
	}
	return task.Status
}
