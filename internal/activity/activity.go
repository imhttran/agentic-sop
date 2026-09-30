// Package activity is SOP's structured activity model: a small, observer-only
// stream of "what the run is doing right now" events. It exists so a long-running
// task is observable while it runs instead of only after it finishes.
//
// It is deliberately not a lifecycle. SOP remains the orchestration and state
// authority: activity events observe existing lifecycle transitions and tool
// execution, and an event never feeds a decision, a gate, or task state. A run
// with no sink registered behaves exactly as before — every Emit on a nil
// Recorder is a no-op — so reporting can be added or removed without changing
// execution.
//
// The Event is the stable boundary: the CLI renders it today, and a future
// controller/API consumer can subscribe to the same events without parsing CLI
// text.
package activity

import (
	"context"
	"sync"
	"time"
)

// Stage vocabulary. A stage names where in the lifecycle an event happened. The
// values are the run's vocabulary, shared by every producer (the CLI lifecycle,
// the agent's tool loop) so consumers never need to know who emitted an event.
const (
	// StageStart marks the beginning of a task's execution.
	StageStart = "START"
	// StagePlan marks PLAN generation.
	StagePlan = "PLAN"
	// StageDiscover marks read/inspect activity (discovery).
	StageDiscover = "DISCOVER"
	// StageChange marks repository-mutating activity (a change).
	StageChange = "CHANGE"
	// StageImplement marks the implementation agent call.
	StageImplement = "IMPLEMENT"
	// StageFix marks a bounded fix cycle.
	StageFix = "FIX"
	// StageValidate marks deterministic validation.
	StageValidate = "VALIDATE"
	// StageReview marks the review stage.
	StageReview = "REVIEW"
	// StageJEV marks the optional JEV analysis.
	StageJEV = "JEV"
	// StageQuality marks the quality gate decision.
	StageQuality = "QUALITY"
	// StageClassify marks the failure classification (kind + disposition).
	StageClassify = "CLASSIFY"
	// StageAutonomy marks the risk-based autonomy decision (action + risk + level),
	// so it is visible why SOP continued automatically or stopped for a human.
	StageAutonomy = "AUTONOMY"
	// StageFinalize marks an agent entering its tool-free finalization phase.
	StageFinalize = "FINALIZE"
	// StageComplete marks a successful terminal outcome.
	StageComplete = "COMPLETE"
	// StageFailed marks a failed terminal outcome.
	StageFailed = "FAILED"
	// StageBlocked marks a human boundary / blocked outcome.
	StageBlocked = "BLOCKED"
	// StageApproval marks a human approval request or decision on it.
	StageApproval = "APPROVAL"
)

// Event is one structured activity observation. Action and Detail are a short
// summary ("reading", "internal/run/jev.go"), never raw arguments, prompts, model
// output, or file contents; producers are responsible for keeping them concise and
// free of secrets.
type Event struct {
	// TaskID identifies the task the activity belongs to, when known.
	TaskID string `json:"task_id,omitempty"`
	// Stage is the lifecycle stage (see the Stage* constants).
	Stage string `json:"stage"`
	// Action is the short verb or command being performed.
	Action string `json:"action,omitempty"`
	// Detail qualifies the action (for example the path being read).
	Detail string `json:"detail,omitempty"`
	// Timestamp is when the event was emitted (UTC).
	Timestamp time.Time `json:"timestamp"`
}

// Sink consumes activity events. Implementations must tolerate being called from
// multiple goroutines.
type Sink interface {
	Emit(Event)
}

// Func adapts a function to a Sink.
type Func func(Event)

// Emit calls f.
func (f Func) Emit(e Event) { f(e) }

// Multi fans an event out to every non-nil sink, in order. It is the boundary a
// producer uses to feed more than one consumer at once (for example a CLI
// renderer and a persisted/streamed artifact) from a single Recorder.
type Multi []Sink

// Emit forwards e to each sink that is present.
func (m Multi) Emit(e Event) {
	for _, s := range m {
		if s == nil {
			continue
		}
		s.Emit(e)
	}
}

// Recorder stamps events with a task identifier and timestamp and forwards them
// to a Sink. It is safe to call Emit on a nil Recorder: the event is dropped, so
// producers never need a nil check. A nil Sink likewise drops events.
type Recorder struct {
	taskID string
	now    func() time.Time
	sink   Sink

	mu sync.Mutex
}

// New returns a Recorder for taskID that timestamps with the wall clock.
func New(taskID string, sink Sink) *Recorder { return WithClock(taskID, sink, time.Now) }

// WithClock returns a Recorder that reads the time from now. It exists so tests
// can drive timestamps without sleeping.
func WithClock(taskID string, sink Sink, now func() time.Time) *Recorder {
	if now == nil {
		now = time.Now
	}
	return &Recorder{taskID: taskID, now: now, sink: sink}
}

// Emit records one activity event. It is a no-op on a nil Recorder or when no
// sink is registered, so a disabled reporter never changes execution.
func (r *Recorder) Emit(stage, action, detail string) {
	if r == nil || r.sink == nil {
		return
	}
	e := Event{TaskID: r.taskID, Stage: stage, Action: action, Detail: detail, Timestamp: r.now()}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sink.Emit(e)
}

// Enabled reports whether the recorder will forward events (a non-nil recorder
// with a sink). Producers can use it to skip building an expensive detail.
func (r *Recorder) Enabled() bool { return r != nil && r.sink != nil }

// recorderKey is the context key under which a Recorder travels, so an in-process
// agent can observe activity without threading a reporter through every call.
type recorderKey struct{}

// WithRecorder returns a copy of ctx carrying r. A nil r returns ctx unchanged,
// so a disabled reporter is indistinguishable from one that was never created.
func WithRecorder(ctx context.Context, r *Recorder) context.Context {
	if r == nil {
		return ctx
	}
	return context.WithValue(ctx, recorderKey{}, r)
}

// FromContext returns the Recorder carried by ctx, or nil when none is present.
// The result is nil-safe, so callers can Emit unconditionally.
func FromContext(ctx context.Context) *Recorder {
	if ctx == nil {
		return nil
	}
	r, _ := ctx.Value(recorderKey{}).(*Recorder)
	return r
}
