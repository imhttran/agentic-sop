package question

import (
	"strings"
	"time"
)

// PlannerSettings is the opt-in planner configuration. The feature is OFF unless
// Enabled is set, so ordinary execution behavior is unchanged by default.
type PlannerSettings struct {
	Enabled        bool
	Mode           string
	RoundLimit     int
	PauseOnTimeout bool
}

// Action is the reconciler's deterministic decision for one task's planner state.
type Action string

const (
	// ActionNone: nothing to do (no question, or a terminal question state).
	ActionNone Action = ""
	// ActionWait: a PENDING question — park at WAITING_FOR_PLANNER and wait.
	ActionWait Action = "WAIT"
	// ActionApplyThenContinue: an ANSWERED question — apply, then continue.
	ActionApplyThenContinue Action = "APPLY_THEN_CONTINUE"
	// ActionContinue: an APPLIED question — continue from the saved stage.
	ActionContinue Action = "CONTINUE"
	// ActionRoundsExhausted: the planner round limit is reached.
	ActionRoundsExhausted Action = "ROUNDS_EXHAUSTED"
	// ActionSkipHumanGate / Cancelled / Terminal / Stale: the reconciler must not act.
	ActionSkipHumanGate Action = "SKIP_HUMAN_GATE"
	ActionSkipCancelled Action = "SKIP_CANCELLED"
	ActionSkipTerminal  Action = "SKIP_TERMINAL"
	ActionSkipStale     Action = "SKIP_STALE"
	// ActionDisabled: the planner feature is off.
	ActionDisabled Action = "DISABLED"
)

// ReconcileView is the authoritative external state the reconciler needs about a
// task. It carries only value facts (no stores), so Decide is pure and testable.
type ReconcileView struct {
	TaskID       string
	TaskTerminal bool
	Cancelled    bool
	HumanGate    bool
	// Rounds is the number of questions raised for this task so far.
	Rounds int
	// Question is the task's current planner question, if any.
	Question *Question
	// Continuation is the durable continuation for that question, if any.
	Continuation *Continuation
	// Current bindings, for staleness checks.
	CurrentPlanRevision    string
	CurrentRunEpoch        string
	CurrentRepoFingerprint string
	// Now is the reconciliation clock (for deadline evaluation).
	Now time.Time
}

// Decide returns the deterministic action and a human-readable reason for a task's
// planner state. It guards, in order: disabled mode, human gates, cancellation,
// terminal task state, staleness, then the question lifecycle.
func Decide(s PlannerSettings, v ReconcileView) (Action, string) {
	if !s.Enabled {
		return ActionDisabled, "planner mode is disabled"
	}
	if v.HumanGate {
		return ActionSkipHumanGate, "task is at a human gate; the human decision owns it"
	}
	if v.Cancelled {
		return ActionSkipCancelled, "the work was cancelled"
	}
	if v.TaskTerminal {
		return ActionSkipTerminal, "task is in a terminal state"
	}
	if v.Question == nil {
		return ActionNone, ""
	}
	if reason, stale := staleBinding(v); stale {
		return ActionSkipStale, reason
	}
	switch v.Question.State {
	case StatePending:
		if s.RoundLimit > 0 && v.Rounds > s.RoundLimit {
			return ActionRoundsExhausted, "planner round limit reached"
		}
		if s.PauseOnTimeout && !v.Question.Deadline.IsZero() && !v.Now.IsZero() && v.Now.After(v.Question.Deadline) {
			return ActionWait, "question deadline passed; paused (no automatic retry)"
		}
		return ActionWait, "question is pending; park at WAITING_FOR_PLANNER"
	case StateAnswered:
		return ActionApplyThenContinue, "answer recorded; apply then continue"
	case StateApplied:
		if v.Continuation != nil && v.Continuation.State.Terminal() {
			return ActionNone, "continuation is " + string(v.Continuation.State)
		}
		return ActionContinue, "answer applied; continue from the saved stage"
	default:
		return ActionNone, "question is in a terminal state: " + string(v.Question.State)
	}
}

// staleBinding reports whether the question's binding no longer matches the current
// plan revision, run epoch, or repository identity.
func staleBinding(v ReconcileView) (string, bool) {
	q := v.Question
	if q == nil {
		return "", false
	}
	if b, cur := strings.TrimSpace(q.PlanRevision), strings.TrimSpace(v.CurrentPlanRevision); b != "" && cur != "" && b != cur {
		return "plan revision changed: bound " + b + ", current " + cur, true
	}
	if b, cur := strings.TrimSpace(q.RunEpoch), strings.TrimSpace(v.CurrentRunEpoch); b != "" && cur != "" && b != cur {
		return "run epoch changed (run superseded)", true
	}
	if b, cur := strings.TrimSpace(q.RepoFingerprint), strings.TrimSpace(v.CurrentRepoFingerprint); b != "" && cur != "" && b != cur {
		return "repository identity changed", true
	}
	return "", false
}
