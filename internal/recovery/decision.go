// Package recovery is SOP's deterministic, bounded execution-recovery policy
// (Phase 5).
//
// When an execution attempt fails a quality gate, SOP must decide what to do
// next. That decision is policy, not model judgement: this package maps typed
// failure evidence to one of a closed set of actions — retry the same class,
// escalate to a larger model class, hand the task to a human, or block it — using
// fixed rules. It is pure and deterministic (the same evidence always yields the
// same decision) and it carries no authority beyond that decision: it never
// transitions task state, mutates the repository, calls a provider, chooses a
// concrete model name, grants approval, or persists anything.
//
// The repository-wide rule applies here unchanged:
//
//	evidence -> structured classification -> deterministic SOP policy -> action
//
// Recovery reads the typed failure classification (internal/failure) and the
// model class the failed attempt used. It never parses agent prose, and it never
// accepts a class the model proposed.
//
// Recovery is separate from routing. Routing (internal/router, internal/model)
// decides which class a task starts on; recovery decides what to do after an
// attempt fails. The model-class ladder is one-way and bounded:
//
//	small -> medium -> large -> human/block
//
// There is no class above large, and recovery never downgrades a class after a
// failure.
package recovery

import (
	"github.com/imhttran/agentic-sop/internal/failure"
	"github.com/imhttran/agentic-sop/internal/model"
)

// Action is the closed set of recovery outcomes. It names a policy decision, not
// a task state: the caller applies the existing SOP lifecycle (requeue, human
// boundary, or block) exactly as it does today.
type Action string

const (
	// ActionNone: recovery did not apply (escalation disabled, or no class was
	// known for the failed attempt). The caller keeps its existing behavior.
	ActionNone Action = "none"
	// ActionRetrySame: do not escalate; the failure is transient or a bounded
	// continuation, so the existing bounded retry/continuation path handles it.
	ActionRetrySame Action = "retry_same"
	// ActionEscalate: retry the task on the next larger model class, in this
	// invocation, with the failed attempt's bounded context handed forward.
	ActionEscalate Action = "escalate"
	// ActionHuman: no automatic escalation applies and a human decision is
	// required (a safety/approval boundary, an invalid plan, or the escalation
	// limit). The caller applies the existing human boundary.
	ActionHuman Action = "human"
	// ActionBlock: the task must not be retried. Reserved for the caller's
	// terminal automation path; Decide does not currently produce it.
	ActionBlock Action = "block"
	// ActionReplan: retry the task on the SAME class with a changed strategy, in
	// this invocation, with the failed attempt's bounded context handed forward.
	// Replanning changes the approach, never the resource: it is not escalation,
	// not a retry of the same strategy, and it never resets a budget. It is OFF by
	// default (Policy.Replan) and bounded by Policy.MaxReplans.
	ActionReplan Action = "replan"
)

// Valid reports whether a is a known action.
func (a Action) Valid() bool {
	switch a {
	case ActionNone, ActionRetrySame, ActionEscalate, ActionHuman, ActionBlock, ActionReplan:
		return true
	default:
		return false
	}
}

// Reason phrases. Like model.Reason* and router.Reason*, these are a fixed,
// deterministic set — never model-generated prose — so a recovery decision stays
// auditable and is never parsed back to drive a decision.
const (
	// ReasonDisabled: the escalation feature flag is off.
	ReasonDisabled = "model escalation is disabled"
	// ReasonImplementationFailed: a legitimate quality-gate failure the next class
	// may resolve.
	ReasonImplementationFailed = "implementation validation failed"
	// ReasonTransient: an infrastructure/provider failure; a larger model does not
	// help.
	ReasonTransient = "transient provider failure; the same class is retried"
	// ReasonContinuation: the work is unfinished rather than incorrect; the bounded
	// continuation path resumes it.
	ReasonContinuation = "the implementation is unfinished; the same class continues"
	// ReasonSafety: a safety, approval, or destructive-operation boundary; risk
	// policy outranks model escalation.
	ReasonSafety = "safety or approval boundary; a human decision is required"
	// ReasonPlanInvalid: the task/plan assumptions are invalid and a larger model
	// cannot repair them.
	ReasonPlanInvalid = "the plan assumptions are invalid; a human decision is required"
	// ReasonNoLargerClass: the attempt already used the largest class.
	ReasonNoLargerClass = "no larger model class is available"
	// ReasonEscalationLimit: the configured escalation budget is spent.
	ReasonEscalationLimit = "maximum escalations reached; a human decision is required"
	// ReasonUnclassified: no authoritative failure classification was available, so
	// recovery fails closed rather than escalating.
	ReasonUnclassified = "the failure was not classified; a human decision is required"
	// ReasonReplan: a recoverable implementation failure may change strategy once,
	// on the same class, before a larger model or a human is considered.
	ReasonReplan = "a recoverable failure may change strategy once"
)

// Evidence is the typed evidence a recovery decision is made from. It reuses the
// existing failure vocabulary (internal/failure) rather than defining a parallel
// one: Kind and Disposition are the classifier's verdict for the failed attempt.
type Evidence struct {
	// Class is the model class the failed attempt used. An unknown/empty class
	// cannot be escalated.
	Class model.Class
	// Kind is the typed failure kind from the failure classifier (internal/failure).
	Kind failure.Kind
	// Disposition is the failure classifier's disposition.
	Disposition failure.Disposition
	// Stage names where the attempt failed (build, test, lint, review, provider),
	// when known. It is descriptive evidence and does not by itself select an
	// action.
	Stage string
	// Escalations is how many escalations this task has already taken.
	Escalations int
	// Attempt is the 1-based attempt number that failed.
	Attempt int
	// Replans is how many times this task has already changed strategy.
	Replans int
}

// Decision is the deterministic recovery outcome for one failed attempt.
type Decision struct {
	// Action is the policy decision.
	Action Action `json:"action"`
	// FromClass is the class the failed attempt used.
	FromClass model.Class `json:"from_class,omitempty"`
	// ToClass is the class an ActionEscalate decision selects.
	ToClass model.Class `json:"to_class,omitempty"`
	// Reason is the deterministic explanation (see the Reason* constants).
	Reason string `json:"reason"`
	// Attempt is the 1-based number of the attempt that failed.
	Attempt int `json:"attempt"`
}

// Policy is the bounded-escalation policy. It is resolved from configuration and
// the SOP_MODEL_* environment by internal/model.
type Policy struct {
	// Enabled turns automatic escalation on. It is OFF by default, so an existing
	// installation's behavior is unchanged.
	Enabled bool
	// MaxEscalations bounds how many times a task may be escalated in one attempt
	// ladder. 0 disables escalation even when Enabled is true.
	MaxEscalations int
	// Replan turns bounded strategy replanning on. It is OFF by default, so an
	// existing installation's behavior is unchanged. Replanning is a distinct
	// recovery from escalation: it keeps the class and changes the strategy.
	Replan bool
	// MaxReplans bounds how many times a task may change strategy in one attempt
	// ladder. 0 disables replanning even when Replan is true.
	MaxReplans int
}

// NextClass returns the class one step above c on the model-class ladder, and
// whether such a class exists. It is the single owner of the ladder order, so no
// caller restates it.
func NextClass(c model.Class) (model.Class, bool) {
	switch c {
	case model.ClassSmall:
		return model.ClassMedium, true
	case model.ClassMedium:
		return model.ClassLarge, true
	default:
		return "", false
	}
}
