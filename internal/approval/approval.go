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
	// Status is the request status (PENDING/APPROVED/DECLINED).
	Status domain.ApprovalStatus
	// RequestedAt is when the request was first recorded.
	RequestedAt time.Time
	// RequestedBy identifies who/what raised the request.
	RequestedBy string
	// DecidedAt is when the decision was recorded, if resolved.
	DecidedAt time.Time
	// DecidedBy identifies who decided, if resolved.
	DecidedBy string
	// Note is the optional decision context, if resolved.
	Note string
	// LifecycleAction is what SOP did in response, if resolved.
	LifecycleAction string
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
}

// DecisionInput is the metadata a client records with a decision.
type DecisionInput struct {
	// By identifies the human/client making the decision.
	By string
	// Note is optional free-form context.
	Note string
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
		ID:          s.newID(taskID),
		TaskID:      taskID,
		Kind:        kind,
		Target:      in.Target,
		Reason:      in.Reason,
		Evidence:    in.Evidence,
		Stage:       in.Stage,
		Disposition: in.Disposition,
		RequestedAt: s.now().UTC(),
		RequestedBy: in.RequestedBy,
		Status:      domain.ApprovalPending,
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
		Applicable:  req.Status == domain.ApprovalPending && (task == nil || !task.IsSatisfied()),
		TaskID:      req.TaskID,
		Kind:        req.Kind,
		Target:      req.Target,
		Reason:      req.Reason,
		Evidence:    req.Evidence,
		Stage:       req.Stage,
		Disposition: req.Disposition,
		Status:      req.Status,
		RequestedAt: req.RequestedAt,
		RequestedBy: req.RequestedBy,
		TaskStatus:  statusOf(task),
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
