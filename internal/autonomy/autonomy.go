// Package autonomy is SOP's centralized, deterministic risk-based autonomy
// policy: the single place that decides whether a failure or a requested action
// may be handled automatically or must wait for a human.
//
// It exists because human approval had drifted into a generic recovery mechanism.
// A provider timeout, an empty response, a malformed JEV result, an exhausted
// tool budget, a deterministic test failure, and a no-change invocation are all
// automation problems with deterministic answers; escalating them to a human
// teaches operators to approve without reading. Human approval is reserved for
// genuine RISK or unresolved HUMAN INTENT.
//
// The decision flow is:
//
//	classification + risk + policy = lifecycle action
//
// Risk and disposition are kept separate: a failure classification says WHAT went
// wrong; the risk model and the configured level say whether SOP may resolve it.
// The package is pure — no I/O, no clock, no LLM — so a decision is reproducible
// and unit-testable, and the final safety decision never depends on free-form
// model prose.
//
// SOP remains the execution authority. A decision is a value the existing
// lifecycle acts on; this package starts no work and transitions no state.
package autonomy

import (
	"strings"

	"github.com/imhttran/agentic-sop/internal/failure"
)

// Level selects how much recovery SOP performs without asking. It is the coarse
// knob an operator sets; the individual Policy flags refine it.
type Level string

const (
	// Low is conservative: retries and productive continuations are automatic,
	// but mutation (a deterministic fix) and reconciliation wait for a human.
	Low Level = "low"
	// Balanced (the default) automates the recoveries SOP already trusts — retry,
	// continue, deterministic fix, bounded infrastructure recovery — and reserves
	// humans for authority boundaries, ambiguity, and plan-semantic changes.
	Balanced Level = "balanced"
	// High automates everything that is safe and deterministic, including
	// reconciliation of semantically-equivalent changes to executed tasks, and
	// treats bounded automation exhaustion as a terminal state rather than a human
	// decision. It is appropriate for local agentic-sop dogfooding.
	High Level = "high"
)

// Valid reports whether l is a known level.
func (l Level) Valid() bool {
	switch l {
	case Low, Balanced, High:
		return true
	default:
		return false
	}
}

// normalize maps an unknown or empty level to the conservative default.
func normalize(l Level) Level {
	if l.Valid() {
		return l
	}
	return Balanced
}

// ApprovalRisk is how consequential an action is. It is independent of the
// failure disposition: a RETRY of a transient failure is RiskLow, a REPLAN of an
// ambiguous plan is RiskHigh, and neither is itself a human decision.
type ApprovalRisk string

const (
	RiskNone         ApprovalRisk = "NONE"
	RiskLow          ApprovalRisk = "LOW"
	RiskMedium       ApprovalRisk = "MEDIUM"
	RiskHigh         ApprovalRisk = "HIGH"
	RiskIrreversible ApprovalRisk = "IRREVERSIBLE"
)

// Action is the lifecycle action the policy selected. It maps onto SOP's existing
// machinery: the AUTO_* actions use the bounded requeue path or the existing fix
// loop, TERMINAL blocks the task as an automation failure, and HUMAN_APPROVAL is
// the human boundary.
type Action string

const (
	ActionAutoRetry     Action = "AUTO_RETRY"
	ActionAutoContinue  Action = "AUTO_CONTINUE"
	ActionAutoFix       Action = "AUTO_FIX"
	ActionAutoReconcile Action = "AUTO_RECONCILE"
	// ActionTerminal is a bounded automation failure: SOP stopped, but no human
	// decision exists. It is deliberately distinct from HUMAN_APPROVAL.
	ActionTerminal Action = "TERMINAL"
	// ActionHumanApproval is the only action that requires a human.
	ActionHumanApproval Action = "HUMAN_APPROVAL_REQUIRED"
)

// Policy is the resolved autonomy policy: a level plus explicit overrides. It is
// deterministic and is built from configuration; it never depends on model output.
type Policy struct {
	// Level is the coarse autonomy level (low, balanced, high).
	Level Level
	// AutoRetry permits bounded retries of transient failures.
	AutoRetry bool
	// AutoContinue permits bounded continuation of productive incomplete work.
	AutoContinue bool
	// AutoFix permits the bounded, deterministic fix loop.
	AutoFix bool
	// AutoReconcileSafeChanges permits automatic reconciliation of semantically
	// safe plan changes (for example a descriptive-only change to an executed task).
	AutoReconcileSafeChanges bool
}

// DefaultLevel is the conservative, backward-compatible default: an omitted
// `autonomy` block resolves to Balanced, which automates exactly the recoveries
// SOP already trusts and does not silently adopt maximum autonomy.
const DefaultLevel = Balanced

// PolicyFor returns the built-in policy for a level. An unknown level resolves to
// the conservative default.
func PolicyFor(level Level) Policy {
	p := Policy{Level: normalize(level)}
	switch p.Level {
	case Low:
		p.AutoRetry, p.AutoContinue, p.AutoFix, p.AutoReconcileSafeChanges = true, true, false, false
	case High:
		p.AutoRetry, p.AutoContinue, p.AutoFix, p.AutoReconcileSafeChanges = true, true, true, true
	default: // Balanced
		p.AutoRetry, p.AutoContinue, p.AutoFix, p.AutoReconcileSafeChanges = true, true, true, false
	}
	return p
}

// Decision is the policy's verdict: the lifecycle action, the risk it carries,
// whether a human is required, and why. Classification is the effective
// classification the lifecycle should act on (its disposition is adjusted when the
// policy overrides an automated disposition with a human boundary).
type Decision struct {
	Action         Action
	Level          Level
	Risk           ApprovalRisk
	RequiresHuman  bool
	Reason         string
	Classification failure.Classification
}

// Decide applies the policy to a failure classification. It is pure and
// deterministic.
func Decide(c failure.Classification, p Policy) Decision {
	// 1. Authority boundaries the classifier already recognized: destructive,
	//    irreversible, security-sensitive, or ambiguous. These are human decisions
	//    at every level.
	if risk, ok := boundaryRisk(c); ok {
		return humanDecision(c, p, risk, reasonOr(c.Reason, "the failure crosses an authority boundary that requires a human"))
	}

	// 2. Bounded automation exhaustion. Under the high level it is a terminal
	//    automation state — SOP tried and stopped; no human decision exists. The
	//    conservative levels keep the human boundary, so an operator who never opted
	//    into high autonomy keeps the previous behavior.
	if c.Kind == failure.AutoFixExhausted {
		if p.Level == High {
			return auto(ActionTerminal, c, p, RiskMedium, "bounded automatic recovery was exhausted; SOP stopped without a human decision")
		}
		return humanDecision(c, p, RiskMedium, reasonOr(c.Reason, "bounded automatic recovery was exhausted"))
	}

	// 3. A recoverable automation the level permits is applied automatically; one it
	//    forbids becomes a human boundary.
	switch c.Disposition {
	case failure.Retry:
		if p.AutoRetry {
			return auto(ActionAutoRetry, c, p, RiskLow, "a transient failure is retried automatically within its bound")
		}
	case failure.Continue:
		if p.AutoContinue {
			return auto(ActionAutoContinue, c, p, RiskLow, "productive incomplete work continues automatically")
		}
	case failure.AutoFix:
		if p.AutoFix {
			return auto(ActionAutoFix, c, p, RiskMedium, "a deterministic failure is fixed automatically")
		}
	case failure.Replan:
		return humanDecision(c, p, RiskMedium, reasonOr(c.Reason, "the plan assumptions are invalid and require a human decision"))
	}

	// 4. A needs_human classification, an automation the level disallows, or an
	//    unknown disposition: fail closed to a human.
	return humanDecision(c, p, riskOf(c.Kind), reasonOr(c.Reason, "the configured autonomy level requires human approval for this failure"))
}

// PlanChangeKind classifies a requested change to an executed task for
// reconciliation. It is deterministic and derived from the plan diff, never from
// model output.
type PlanChangeKind string

const (
	// PlanChangeExecutedEquivalent: executed tasks changed only in descriptive
	// text; their executable semantics are unchanged.
	PlanChangeExecutedEquivalent PlanChangeKind = "EXECUTED_EQUIVALENT"
	// PlanChangeExecutedMaterial: an executed task's executable semantics changed.
	PlanChangeExecutedMaterial PlanChangeKind = "EXECUTED_MATERIAL"
)

// DecidePlanChange applies the policy to a change to an executed task. It is pure
// and deterministic. Reconciliation decisions carry no failure classification.
func DecidePlanChange(kind PlanChangeKind, p Policy) Decision {
	if kind == PlanChangeExecutedEquivalent && p.AutoReconcileSafeChanges {
		return Decision{
			Action: ActionAutoReconcile,
			Level:  p.Level,
			Risk:   RiskMedium,
			Reason: "executed tasks changed only in descriptive text; their semantics are unchanged",
		}
	}
	// A material change to executed behavior requires authorization at every level:
	// the intent cannot be established deterministically, which is exactly the
	// human-decision case. An equivalent change is the same under a level that does
	// not permit safe reconciliation.
	reason := "an executed task's executable semantics changed; explicit approval is required"
	risk := RiskHigh
	if kind == PlanChangeExecutedEquivalent {
		reason = "an executed task's definition changed and the autonomy level requires approval"
		risk = RiskMedium
	}
	return Decision{Action: ActionHumanApproval, Level: p.Level, Risk: risk, RequiresHuman: true, Reason: reason}
}

// boundaryRisk reports whether a classification is an authority boundary, and its
// risk severity. These are human decisions at every level.
func boundaryRisk(c failure.Classification) (ApprovalRisk, bool) {
	switch c.Kind {
	case failure.DestructiveOperation:
		return RiskIrreversible, true
	case failure.SecurityBoundary, failure.ApprovalRequired, failure.AmbiguousContract:
		return RiskHigh, true
	default:
		return RiskNone, false
	}
}

// riskOf maps a failure kind to the risk it carries when the policy does not
// otherwise classify it. The authority-boundary kinds are handled by boundaryRisk
// before this is reached.
func riskOf(kind failure.Kind) ApprovalRisk {
	switch kind {
	case failure.ReplanRequired:
		return RiskHigh
	case failure.Unknown,
		failure.CompilerError, failure.TestFailure, failure.LintFailure, failure.StaleTest,
		failure.IntegrationWiring, failure.Regression, failure.MissingTestCoverage,
		failure.BlockingFindings:
		return RiskMedium
	case "":
		return RiskNone
	default:
		return RiskLow
	}
}

// auto builds an automated decision that keeps the classification's disposition.
func auto(a Action, c failure.Classification, p Policy, risk ApprovalRisk, reason string) Decision {
	return Decision{
		Action:         a,
		Level:          p.Level,
		Risk:           risk,
		Reason:         reason,
		Classification: c,
	}
}

// humanDecision builds a human-approval decision and overrides the effective
// classification's disposition so the lifecycle treats it as a human boundary.
func humanDecision(c failure.Classification, p Policy, risk ApprovalRisk, reason string) Decision {
	c.Disposition = failure.NeedsHuman
	return Decision{
		Action:         ActionHumanApproval,
		Level:          p.Level,
		Risk:           risk,
		RequiresHuman:  true,
		Reason:         reason,
		Classification: c,
	}
}

// reasonOr returns reason when it carries text, otherwise fallback.
func reasonOr(reason, fallback string) string {
	if r := strings.TrimSpace(reason); r != "" {
		return r
	}
	return fallback
}
