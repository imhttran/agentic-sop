package recovery

import (
	"github.com/imhttran/agentic-sop/internal/failure"
)

// Decide maps typed failure evidence to a recovery action. It is pure and
// deterministic: the same evidence and policy always yield the same decision.
//
// # Precedence
//
//  1. escalation disabled           -> none (the caller's behavior is unchanged)
//  2. safety / approval / plan      -> human  (risk policy outranks escalation)
//  3. transient provider failure    -> retry_same (a larger model does not help)
//  4. unfinished (continuation)     -> retry_same
//  5. implementation failure        -> escalate one class, or human when the
//     ladder is exhausted or the escalation
//     budget is spent
//  6. anything unclassified         -> human  (fail closed: never escalate on
//     evidence SOP could not classify)
//
// A high-risk or safety-sensitive failure can therefore never be resolved by
// silently spending a larger model: it can only be escalated when the classifier
// says the failure is a legitimate implementation failure.
func Decide(p Policy, ev Evidence) Decision {
	d := Decision{FromClass: ev.Class, Attempt: ev.Attempt}

	if !p.Enabled {
		d.Action, d.Reason = ActionNone, ReasonDisabled
		return d
	}

	// 2. Boundaries that a larger model must never bypass. This is the safety
	//    override: risk and approval policy outrank escalation.
	switch ev.Kind {
	case failure.ApprovalRequired, failure.SecurityBoundary,
		failure.DestructiveOperation, failure.AmbiguousContract:
		d.Action, d.Reason = ActionHuman, ReasonSafety
		return d
	case failure.ReplanRequired:
		d.Action, d.Reason = ActionHuman, ReasonPlanInvalid
		return d
	case failure.Unknown:
		// No authoritative signal: fail closed. It is a human boundary, but naming it
		// a safety boundary would misreport why.
		d.Action, d.Reason = ActionHuman, ReasonUnclassified
		return d
	}
	if ev.Disposition == failure.Replan {
		d.Action, d.Reason = ActionHuman, ReasonPlanInvalid
		return d
	}

	// 3. Infrastructure/provider failures: a bigger model does not fix a timeout,
	//    an empty response, or a tool failure, so the same class is retried.
	if ev.Disposition == failure.Retry || transientKind(ev.Kind) {
		d.Action, d.Reason = ActionRetrySame, ReasonTransient
		return d
	}

	// 4. Unfinished-but-productive work is a continuation, not an escalation.
	if ev.Disposition == failure.Continue || ev.Kind == failure.IncompleteImplementation {
		d.Action, d.Reason = ActionRetrySame, ReasonContinuation
		return d
	}

	// 5. A genuine implementation failure may be resolved by a stronger model.
	if !implementationFailure(ev) {
		// No authoritative classification: fail closed rather than escalate.
		d.Action, d.Reason = ActionHuman, ReasonUnclassified
		return d
	}

	next, ok := NextClass(ev.Class)
	if !ok {
		// The largest class already ran, or no class was known: there is nothing to
		// escalate to, so the existing human/terminal boundary applies.
		d.Action, d.Reason = ActionHuman, ReasonNoLargerClass
		return d
	}
	if ev.Escalations >= p.MaxEscalations {
		d.Action, d.Reason = ActionHuman, ReasonEscalationLimit
		return d
	}
	d.Action, d.ToClass, d.Reason = ActionEscalate, next, ReasonImplementationFailed
	return d
}

// transientKind reports whether a failure kind is an infrastructure/provider
// failure that a larger model cannot resolve.
func transientKind(k failure.Kind) bool {
	switch k {
	case failure.TransientProvider, failure.EmptyResponse, failure.ToolFailure,
		failure.JEVAnalysisFailure:
		return true
	default:
		return false
	}
}

// implementationFailure reports whether the classified failure is a legitimate
// implementation/quality failure that a stronger model may resolve. It is
// deliberately explicit: a needs-human signal that is not the bounded
// fix-budget exhaustion (which a stronger model may still beat) is NOT an
// implementation failure, so an unrecognized human signal can never escalate.
func implementationFailure(ev Evidence) bool {
	if ev.Kind == failure.AutoFixExhausted {
		// The bounded deterministic repair budget is spent. A larger model is a
		// legitimate next step before involving a human.
		return true
	}
	if ev.Disposition == failure.NeedsHuman {
		return false
	}
	switch ev.Kind {
	case failure.CompilerError, failure.TestFailure, failure.StaleTest,
		failure.IntegrationWiring, failure.LintFailure, failure.Regression,
		failure.BlockingFindings, failure.MissingTestCoverage:
		return true
	}
	return ev.Disposition == failure.AutoFix
}
