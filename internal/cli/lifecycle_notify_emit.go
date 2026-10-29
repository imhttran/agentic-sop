package cli

import (
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/notify"
	"github.com/imhttran/agentic-sop/internal/store"
)

// errNoLifecycleStore reports that an event cannot be recorded because the
// project path is empty.
var errNoLifecycleStore = errors.New("lifecycle notify: no project to record the event")

// lifecycleSink is the process-wide event sink the lifecycle entry points emit
// through. Production leaves it nil, so each emit opens the project's durable
// notification outbox for the transition it records. A background job runs in a
// detached child process; its terminal event must land in the shared SQLite
// outbox rather than a process-local buffer that dies with that process. Tests
// install an isolated in-memory sink with setLifecycleSink.
var lifecycleSink EventSink

// lifecycleEmitter returns the emitter over the process-wide sink. A nil sink is
// passed through so Emit reports that no sink is configured.
func lifecycleEmitter() *LifecycleEmitter {
	return NewLifecycleEmitter(lifecycleSink)
}

// setLifecycleSink replaces the process-wide sink and returns the previous one.
// Tests use it to install an isolated sink; production leaves it nil so emit
// falls back to the durable outbox.
func setLifecycleSink(s EventSink) EventSink {
	prev := lifecycleSink
	lifecycleSink = s
	return prev
}

// emitTransition enqueues one lifecycle event for projectDir. When a sink is
// installed (tests) it uses that sink; otherwise it opens the project's durable
// notification outbox and enqueues there, so an event recorded by a short-lived
// process (a detached background job) survives to be delivered. Emission is
// best-effort provenance: a store that cannot be opened or a transition the
// emitter rejects yields no event and never changes the run outcome.
func emitTransition(projectDir string, req EmitRequest) (notify.Event, bool, error) {
	if lifecycleSink != nil {
		return NewLifecycleEmitter(lifecycleSink).Emit(req)
	}
	outbox, err := openLifecycleOutbox(projectDir)
	if err != nil {
		return notify.Event{}, false, err
	}
	defer outbox.Close()
	return NewLifecycleEmitter(outbox).Emit(req)
}

// emitTransitionApprovalJob emits the waiting_for_human event for a task parked
// at the PENDING approval request SOP recorded, carrying the recorded job id. It
// derives the event from the recorded request and never from a task status or
// prose. A request that is not PENDING produces no event. The resolution command
// names the recorded request's task, so a client that follows it resolves that
// exact gate.
func emitTransitionApprovalJob(projectDir, taskID, jobID string, attempt int, req *domain.ApprovalRequest) (notify.Event, bool, error) {
	if req == nil || req.Status != domain.ApprovalPending {
		return notify.Event{}, false, nil
	}
	target := strings.TrimSpace(req.TaskID)
	if target == "" {
		target = taskID
	}
	resolution := "sop approve"
	if target != "" {
		resolution += " " + target
	}
	return emitTransition(projectDir, EmitRequest{
		Project:           transitionProject(projectDir),
		TaskID:            taskID,
		JobID:             jobID,
		Attempt:           attempt,
		Transition:        TransitionWaitingForHuman,
		Reason:            req.Reason,
		Approval:          req,
		ResolutionCommand: resolution,
	})
}

// emitTransitionApproval emits the waiting_for_human event for a task parked at
// the PENDING approval request SOP recorded. It is the task-scoped form of
// emitTransitionApprovalJob and is kept so the existing call sites stay valid.
func emitTransitionApproval(projectDir, taskID string, attempt int, req *domain.ApprovalRequest) (notify.Event, bool, error) {
	return emitTransitionApprovalJob(projectDir, taskID, "", attempt, req)
}

// emitTransitionCompleted emits the completed event for a task that reached its
// local outcome.
func emitTransitionCompleted(projectDir, taskID, jobID string, attempt int, outcome, nextAction string) (notify.Event, bool, error) {
	return emitTransition(projectDir, EmitRequest{
		Project:    transitionProject(projectDir),
		TaskID:     taskID,
		JobID:      jobID,
		Attempt:    attempt,
		Transition: TransitionCompleted,
		Outcome:    outcome,
		NextAction: nextAction,
	})
}

// emitTransitionFailed emits the failed event for a task SOP blocked or failed. A
// generic BLOCKED task with no approval request is a failure, never a human gate,
// so it is emitted with the BLOCKED transition label.
func emitTransitionFailed(projectDir, taskID, jobID string, attempt int, blocked bool, reason, nextAction string) (notify.Event, bool, error) {
	label := TransitionFailed
	if blocked {
		label = TransitionBlocked
	}
	return emitTransition(projectDir, EmitRequest{
		Project:    transitionProject(projectDir),
		TaskID:     taskID,
		JobID:      jobID,
		Attempt:    attempt,
		Transition: label,
		Reason:     reason,
		NextAction: nextAction,
	})
}

// transitionProject names the project an event belongs to. It prefers the
// configured project name so an event agrees with what a human sees in
// notifications; otherwise it falls back to the project directory's base name.
func transitionProject(projectDir string) string {
	dir := strings.TrimSpace(projectDir)
	if dir == "" {
		return "unknown"
	}
	if cfg, err := loadConfigOrDefault(dir); err == nil {
		if name := strings.TrimSpace(cfg.Project.Name); name != "" {
			return name
		}
	}
	return filepath.Base(dir)
}

// outboxEventSink adapts the durable SQLite notification outbox to the emitter's
// EventSink. Enqueue records one event row and one delivery row per configured
// channel; the id and (event_id, channel) conflict rules make a repeated enqueue
// idempotent, so a replay never produces a second event or delivery.
type outboxEventSink struct {
	st  *store.Store
	cfg config.Config
	now func() time.Time
}

// Enqueue records e once. It reports recorded=false when the event id was already
// present, so the emitter distinguishes a first emission from a replay.
func (o *outboxEventSink) Enqueue(e notify.Event) (bool, error) {
	now := o.now
	if now == nil {
		now = time.Now
	}
	t := now().UTC()
	existed, err := o.st.NotificationEventExists(e.ID)
	if err != nil {
		return false, err
	}
	channels := notificationChannelList(o.cfg)
	if err := o.st.EnqueueNotification(storeNotificationEvent(e), channels, o.cfg.Notifications.Retry.MaxAttempts, t, t); err != nil {
		return false, err
	}
	return !existed, nil
}

// Close releases the underlying store.
func (o *outboxEventSink) Close() error { return o.st.Close() }

// notificationChannelList returns the configured delivery channels for an event.
// An unavailable or unset channel yields no delivery rows; the event row is still
// recorded so it is never silently lost.
func notificationChannelList(cfg config.Config) []string {
	switch cfg.NotifyChannel() {
	case config.NotifyChannelFile, config.NotifyChannelWebhook:
		return []string{cfg.NotifyChannel()}
	default:
		return nil
	}
}

// openLifecycleOutbox opens the project's durable notification outbox as an
// event sink. A project without initialized state cannot record an event.
func openLifecycleOutbox(projectDir string) (*outboxEventSink, error) {
	dir := strings.TrimSpace(projectDir)
	if dir == "" {
		return nil, errNoLifecycleStore
	}
	cfg, err := loadConfigOrDefault(dir)
	if err != nil {
		return nil, err
	}
	st, err := store.Open(statePath(dir))
	if err != nil {
		return nil, err
	}
	return &outboxEventSink{st: st, cfg: cfg, now: time.Now}, nil
}
