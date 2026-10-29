package cli

import (
	"fmt"
	"strings"

	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/notify"
)

// Transition labels the emitter understands. A recorded SOP lifecycle transition
// names one of these labels; the emitter maps each label to exactly one
// notification event kind.
const (
	// TransitionCompleted: the task or job reached its outcome.
	TransitionCompleted = "COMPLETED"
	// TransitionFailed: SOP recorded a failure.
	TransitionFailed = "FAILED"
	// TransitionBlocked: SOP parked the task with a BLOCKED status. A BLOCKED
	// transition without an explicit PENDING approval request is a failure, not a
	// human gate.
	TransitionBlocked = "BLOCKED"
	// TransitionWaitingForHuman: SOP parked the task at an explicit PENDING
	// approval request.
	TransitionWaitingForHuman = "WAITING_FOR_HUMAN"
)

// transitionKind maps a recorded transition label to the one event kind it
// produces. An unknown label is rejected at emission time.
func transitionKind(label string) (notify.Kind, bool) {
	switch strings.TrimSpace(label) {
	case TransitionCompleted:
		return notify.KindCompleted, true
	case TransitionFailed, TransitionBlocked:
		return notify.KindFailed, true
	case TransitionWaitingForHuman:
		return notify.KindWaitingForHuman, true
	default:
		return "", false
	}
}

// EventSink records one produced notification event. The lifecycle emitter writes
// exactly one enqueue per distinct event id.
//
// A concrete durable notify.Outbox implementation is a later delivery-layer task
// and is deliberately out of scope here: the emitter depends on this narrow sink
// so the durable store can be supplied when it lands, and tests inject an
// in-memory fake.
type EventSink interface {
	// Enqueue records e. It returns recorded=false when an event with the same id
	// was already enqueued, so a replay never produces a second delivery.
	Enqueue(e notify.Event) (recorded bool, err error)
}

// LifecycleEmitter turns one recorded SOP lifecycle transition into at most one
// notification event.
//
// The emitter derives the event kind from the transition label SOP actually
// recorded, never from a client assertion. It dedups on the deterministic event
// id: repeated status reads, repeated drains, and a replayed run stage all yield
// the same id and enqueue once. A new attempt, a new job, or a new approval
// request yields a different id and therefore its own event.
//
// Read paths must use Emit and rely on the sink's id dedup: they never mutate
// SOP state themselves.
type LifecycleEmitter struct {
	Sink EventSink
}

// NewLifecycleEmitter builds an emitter over sink. A nil sink is reported when an
// event is emitted, so construction never panics.
func NewLifecycleEmitter(sink EventSink) *LifecycleEmitter {
	return &LifecycleEmitter{Sink: sink}
}

// EmitRequest is the recorded transition the emitter turns into an event. It is
// assembled from SOP's own records: the transition label and attempt come from
// the recorded transition, the approval request from the persisted request, and
// the job id from the run layer.
type EmitRequest struct {
	// Project names the project the transition happened in.
	Project string
	// TaskID names the task, when the transition concerns one.
	TaskID string
	// JobID names the run (job) the transition belongs to, when known.
	JobID string
	// Attempt is the recorded attempt number (>= 1).
	Attempt int
	// Transition is the recorded SOP transition label.
	Transition string
	// Reason is SOP's recorded reason.
	Reason string
	// NextAction is the next action an operator or agent can execute.
	NextAction string
	// Outcome is the recorded outcome of a completed transition.
	Outcome string
	// Approval is the persisted approval request for a WAITING_FOR_HUMAN
	// transition. It must be PENDING.
	Approval *domain.ApprovalRequest
	// ResolutionCommand resolves the human gate.
	ResolutionCommand string
}

// Kind derives the event kind from the recorded transition and approval request.
// A BLOCKED transition never maps to waiting_for_human: only a WAITING_FOR_HUMAN
// transition carrying a PENDING approval request does.
func (r EmitRequest) kind() (notify.Kind, error) {
	kind, ok := transitionKind(r.Transition)
	if !ok {
		return "", fmt.Errorf("lifecycle notify: unknown transition %q", r.Transition)
	}
	if kind == notify.KindWaitingForHuman {
		if r.Approval == nil {
			return "", fmt.Errorf("lifecycle notify: transition %q requires an approval request", r.Transition)
		}
		if r.Approval.Status != domain.ApprovalPending {
			return "", fmt.Errorf("lifecycle notify: transition %q requires a PENDING approval request, got %q",
				r.Transition, string(r.Approval.Status))
		}
		return notify.KindWaitingForHuman, nil
	}
	return kind, nil
}

// Emit turns r into one notification event and enqueues it through the sink. It
// returns the event and whether the sink newly recorded it. A repeated emission
// for the same transition returns the same event with recorded=false.
func (e *LifecycleEmitter) Emit(r EmitRequest) (notify.Event, bool, error) {
	if e == nil || e.Sink == nil {
		return notify.Event{}, false, fmt.Errorf("lifecycle notify: no event sink is configured")
	}
	kind, err := r.kind()
	if err != nil {
		return notify.Event{}, false, err
	}
	params := notify.Params{
		Kind:              kind,
		Project:           r.Project,
		JobID:             r.JobID,
		TaskID:            r.TaskID,
		Attempt:           r.Attempt,
		Transition:        r.Transition,
		Reason:            r.Reason,
		NextAction:        r.NextAction,
		Outcome:           r.Outcome,
		Approval:          r.Approval,
		ResolutionCommand: r.ResolutionCommand,
	}
	event, err := notify.NewEvent(params)
	if err != nil {
		return notify.Event{}, false, fmt.Errorf("lifecycle notify: %w", err)
	}
	recorded, err := e.Sink.Enqueue(event)
	if err != nil {
		return notify.Event{}, false, fmt.Errorf("lifecycle notify: enqueue event %q: %w", event.ID, err)
	}
	return event, recorded, nil
}

// EmitCompleted emits the completed event for a terminal success transition.
func (e *LifecycleEmitter) EmitCompleted(project, taskID, jobID string, attempt int, outcome, nextAction string) (notify.Event, bool, error) {
	return e.Emit(EmitRequest{
		Project:    project,
		TaskID:     taskID,
		JobID:      jobID,
		Attempt:    attempt,
		Transition: TransitionCompleted,
		Outcome:    outcome,
		NextAction: nextAction,
	})
}

// EmitFailed emits the failed event for a recorded failure transition. A generic
// BLOCKED task with no approval request is a failure, so blocked=true records the
// BLOCKED transition label and blocked=false records FAILED; both yield a failed
// event and never waiting_for_human.
func (e *LifecycleEmitter) EmitFailed(project, taskID, jobID string, attempt int, blocked bool, reason, nextAction string) (notify.Event, bool, error) {
	label := TransitionFailed
	if blocked {
		label = TransitionBlocked
	}
	return e.Emit(EmitRequest{
		Project:    project,
		TaskID:     taskID,
		JobID:      jobID,
		Attempt:    attempt,
		Transition: label,
		Reason:     reason,
		NextAction: nextAction,
	})
}

// EmitApproval emits the waiting_for_human event for a task parked at the given
// PENDING approval request. The request identity, reason, and resolution command
// populate the event. A non-PENDING or invalid request returns an error and
// enqueues nothing.
func (e *LifecycleEmitter) EmitApproval(project, taskID, jobID string, attempt int, approval *domain.ApprovalRequest, resolutionCommand string) (notify.Event, bool, error) {
	if approval == nil {
		return notify.Event{}, false, fmt.Errorf("lifecycle notify: approval request is required")
	}
	if strings.TrimSpace(approval.ID) == "" {
		return notify.Event{}, false, fmt.Errorf("lifecycle notify: approval request has no id")
	}
	return e.Emit(EmitRequest{
		Project:           project,
		TaskID:            taskID,
		JobID:             jobID,
		Attempt:           attempt,
		Transition:        TransitionWaitingForHuman,
		Reason:            approval.Reason,
		Approval:          approval,
		ResolutionCommand: resolutionCommand,
	})
}

// MemoryEventSink is an in-memory EventSink that dedups on event id. It backs
// tests and any caller that has no durable outbox yet.
type MemoryEventSink struct {
	// Order records the event ids in first-enqueue order.
	Order []string
	byID  map[string]notify.Event
}

// NewMemoryEventSink returns an empty in-memory sink.
func NewMemoryEventSink() *MemoryEventSink {
	return &MemoryEventSink{byID: map[string]notify.Event{}}
}

// Enqueue records e once per event id.
func (m *MemoryEventSink) Enqueue(e notify.Event) (bool, error) {
	if strings.TrimSpace(e.ID) == "" {
		return false, fmt.Errorf("memory event sink: event has no id")
	}
	if m.byID == nil {
		m.byID = map[string]notify.Event{}
	}
	if _, ok := m.byID[e.ID]; ok {
		return false, nil
	}
	m.byID[e.ID] = e
	m.Order = append(m.Order, e.ID)
	return true, nil
}

// Events returns the enqueued events in first-enqueue order.
func (m *MemoryEventSink) Events() []notify.Event {
	out := make([]notify.Event, 0, len(m.Order))
	for _, id := range m.Order {
		out = append(out, m.byID[id])
	}
	return out
}

// Len returns the number of distinct enqueued events.
func (m *MemoryEventSink) Len() int { return len(m.Order) }
