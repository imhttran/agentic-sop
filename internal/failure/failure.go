// Package failure classifies why a task's lifecycle stopped short of a pass and
// what SOP should do about it. It is a deterministic, provider-free classifier:
// given structured evidence it returns a disposition, and SOP's EXISTING
// lifecycle acts on that disposition. It never executes anything and never
// introduces a second lifecycle engine.
//
// The decision rule is:
//
//	failure -> classify -> is authoritative intent available?
//	                     -> is correction safe and scoped?
//	                     -> AUTO_FIX / CONTINUE / RETRY when possible
//	                     -> NEEDS_HUMAN only when necessary
//
// NEEDS_HUMAN is reserved for genuinely ambiguous decisions or explicit
// approval/safety boundaries — never merely because the agent ran out of
// iterations, ran out of tools, left work unfinished, still needs tests written,
// hit a deterministic test failure, needs more investigation, or failed a first
// fix attempt. Those are AUTO_FIX, CONTINUE, or RETRY.
package failure

import (
	"fmt"
	"strings"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// Disposition is what SOP should do about a failure. It maps onto the existing
// lifecycle: AUTO_FIX feeds the bounded fix loop, CONTINUE/RETRY use the bounded
// requeue path, and NEEDS_HUMAN/REPLAN use the existing human boundary. SOP
// remains the execution authority.
type Disposition string

const (
	// AutoFix: the intended behavior can be determined safely; the existing
	// bounded fix loop should correct it.
	AutoFix Disposition = "AUTO_FIX"
	// Continue: required work remains but no human decision is required; the
	// task should be requeued so a later invocation continues.
	Continue Disposition = "CONTINUE"
	// Retry: a transient infrastructure/provider failure; requeue and retry.
	Retry Disposition = "RETRY"
	// Replan: the task/plan assumptions are invalid and cannot be corrected
	// locally while preserving the acceptance criteria.
	Replan Disposition = "REPLAN"
	// NeedsHuman: genuinely ambiguous, or an explicit approval/safety boundary.
	NeedsHuman Disposition = "NEEDS_HUMAN"
)

// Confidence records how strong the classification's evidence is.
type Confidence string

const (
	High   Confidence = "HIGH"
	Medium Confidence = "MEDIUM"
	Low    Confidence = "LOW"
)

// Kind is the specific failure class.
type Kind string

const (
	MissingTestCoverage      Kind = "MISSING_TEST_COVERAGE"
	CompilerError            Kind = "COMPILER_ERROR"
	TestFailure              Kind = "TEST_FAILURE"
	StaleTest                Kind = "STALE_TEST"
	IntegrationWiring        Kind = "INTEGRATION_WIRING"
	LintFailure              Kind = "LINT_FAILURE"
	Regression               Kind = "REGRESSION"
	BlockingFindings         Kind = "BLOCKING_FINDINGS"
	IncompleteImplementation Kind = "INCOMPLETE_IMPLEMENTATION"
	AutoFixExhausted         Kind = "AUTO_FIX_EXHAUSTED"
	TransientProvider        Kind = "TRANSIENT_PROVIDER"
	EmptyResponse            Kind = "EMPTY_RESPONSE"
	ToolFailure              Kind = "TOOL_FAILURE"
	ReplanRequired           Kind = "REPLAN_REQUIRED"
	AmbiguousContract        Kind = "AMBIGUOUS_CONTRACT"
	ApprovalRequired         Kind = "APPROVAL_REQUIRED"
	SecurityBoundary         Kind = "SECURITY_BOUNDARY"
	DestructiveOperation     Kind = "DESTRUCTIVE_OPERATION"
	Unknown                  Kind = "UNKNOWN"
)

// Classification is the classifier's verdict for one failure.
type Classification struct {
	Kind        Kind        `json:"kind"`
	Disposition Disposition `json:"disposition"`
	Confidence  Confidence  `json:"confidence"`
	Reason      string      `json:"reason"`
}

// Retryable reports whether the disposition is one the lifecycle should feed to
// the bounded requeue path (CONTINUE or RETRY). A blank classification is not
// retryable.
func (c Classification) Retryable() bool {
	return c.Disposition == Continue || c.Disposition == Retry
}

// Evidence is the structured failure evidence the classifier decides on. Callers
// supply only what they know; absent evidence is indistinguishable from false.
type Evidence struct {
	// Source names what failed (for example "IMPLEMENT", "VALIDATE", "PLAN").
	Source string
	// Outcome is the non-completed structured outcome an agent reported, if any.
	Outcome *agent.Outcome
	// Err is the provider/infrastructure error, if the call failed outright.
	Err error

	// Deterministic verification evidence.
	BuildFailed      bool
	TestFailed       bool
	LintFailed       bool
	TestsMissing     bool
	BlockingFindings int

	// Authoritative signals, supplied when a caller can determine them. They
	// select the specific AUTO_FIX kind; their absence does not change the
	// disposition (a test failure is still auto-fixable).
	StaleTest              bool
	IntegrationWiring      bool
	Regression             bool
	ConflictResolvedByPlan bool

	// Explicit boundaries. Any of these makes the failure a human decision
	// regardless of everything else.
	PlanInvalid          bool
	RequirementsConflict bool
	ApprovalRequired     bool
	SecurityBoundary     bool
	DestructiveOperation bool

	// Fix-loop budget. A zero MaxFixCycles means "unknown/unbounded".
	FixCycles    int
	MaxFixCycles int
}

// Classify returns the disposition and reason for the failure described by ev.
// It is pure and deterministic: the same evidence always yields the same result.
func Classify(ev Evidence) Classification {
	// 1. Explicit human boundaries win over every automated disposition: a human
	//    decision, approval, or safety limit is not something SOP may resolve.
	if ev.ApprovalRequired {
		return human(ApprovalRequired, "an explicit approval is required before this change may proceed")
	}
	if ev.SecurityBoundary {
		return human(SecurityBoundary, "the change crosses a security or safety boundary that requires authorization")
	}
	if ev.DestructiveOperation {
		return human(DestructiveOperation, "the change is destructive or irreversible and requires authorization")
	}
	if ev.RequirementsConflict {
		return human(AmbiguousContract, "requirements conflict and no authoritative contract resolves them")
	}

	// 2. Invalid plan assumptions cannot be corrected locally while preserving
	//    the task's acceptance criteria.
	if ev.PlanInvalid {
		return Classification{Kind: ReplanRequired, Disposition: Replan, Confidence: High,
			Reason: "the task/plan assumptions are invalid and cannot be corrected locally while preserving the acceptance criteria"}
	}

	// 3. An agent-reported failure outcome.
	if c, ok := fromOutcome(ev); ok {
		return c
	}

	// 4. An infrastructure/provider error.
	if c, ok := fromError(ev); ok {
		return c
	}

	// 5. A deterministic verification failure: the intended behavior can be
	//    determined from the task, the domain contract, and the diagnostics, so
	//    the existing bounded fix loop resolves it.
	if c, ok := fromVerification(ev); ok {
		return c
	}

	// 6. No authoritative signal: fail closed to a human rather than guess.
	return Classification{Kind: Unknown, Disposition: NeedsHuman, Confidence: Low,
		Reason: "no authoritative signal was available; a human decision is required"}
}

// fromOutcome classifies a non-completed agent outcome. It examines the reason
// text for the boundary, transient, and incomplete signals the harness emits,
// then falls back to respecting the status the agent reported.
func fromOutcome(ev Evidence) (Classification, bool) {
	if ev.Outcome == nil {
		return Classification{}, false
	}
	reason := strings.TrimSpace(firstNonEmpty(ev.Outcome.Reason, ev.Outcome.Summary))
	text := strings.ToLower(reason)

	if kind, ok := humanKind(text); ok {
		return human(kind, describe(reason, "the agent reported a boundary that requires a human")), true
	}
	if kind, ok := transientKind(text); ok {
		return retry(kind, describe(reason, "a transient failure occurred")), true
	}
	if matchesAny(text, incompleteMarkers) {
		return Classification{Kind: IncompleteImplementation, Disposition: Continue, Confidence: High,
			Reason: describe(reason, "the implementation is unfinished; no human decision is required")}, true
	}

	// No recognized signal. An agent that explicitly asked for a human is
	// honored; a plain failure is parked at the human boundary rather than
	// silently retried forever.
	return human(Unknown, describe(reason, "the agent reported a failure without an authoritative signal")), true
}

// fromError classifies a provider/infrastructure error by its text.
func fromError(ev Evidence) (Classification, bool) {
	if ev.Err == nil {
		return Classification{}, false
	}
	text := strings.ToLower(ev.Err.Error())

	if kind, ok := humanKind(text); ok {
		return human(kind, ev.Err.Error()), true
	}
	if matchesAny(text, emptyMarkers) {
		return retry(EmptyResponse, describe(ev.Err.Error(), "the provider returned an empty response")), true
	}
	if matchesAny(text, transientMarkers) {
		return retry(TransientProvider, describe(ev.Err.Error(), "a transient infrastructure failure occurred")), true
	}
	if matchesAny(text, toolMarkers) {
		return retry(ToolFailure, describe(ev.Err.Error(), "a tool failure occurred")), true
	}
	if matchesAny(text, incompleteMarkers) {
		return Classification{Kind: IncompleteImplementation, Disposition: Continue, Confidence: Medium,
			Reason: describe(ev.Err.Error(), "the implementation is unfinished; no human decision is required")}, true
	}
	return Classification{}, false
}

// fromVerification classifies a deterministic verification failure as AUTO_FIX,
// unless the configured fix budget is already spent, in which case the failure is
// bounded and escalates to a human.
func fromVerification(ev Evidence) (Classification, bool) {
	autoFixable := ev.BuildFailed || ev.TestFailed || ev.LintFailed || ev.TestsMissing || ev.BlockingFindings > 0
	if !autoFixable {
		return Classification{}, false
	}

	if ev.MaxFixCycles > 0 && ev.FixCycles >= ev.MaxFixCycles {
		return Classification{Kind: AutoFixExhausted, Disposition: NeedsHuman, Confidence: High,
			Reason: fmt.Sprintf("automatic fixes were exhausted (%d/%d) without resolving the failure", ev.FixCycles, ev.MaxFixCycles)}, true
	}

	switch {
	case ev.IntegrationWiring:
		return autoFix(IntegrationWiring, "an integration wiring error with established interfaces can be corrected"), true
	case ev.Regression:
		return autoFix(Regression, "a regression introduced by the current task can be corrected"), true
	case ev.StaleTest:
		return autoFix(StaleTest, "a stale test conflicts with the authoritative contract; the test should be updated"), true
	case ev.ConflictResolvedByPlan:
		return autoFix(TestFailure, "the plan/domain contract resolves the conflicting tests; the tests should be updated"), true
	case ev.TestsMissing:
		return autoFix(MissingTestCoverage, "required deterministic test coverage is missing"), true
	case ev.BuildFailed:
		return autoFix(CompilerError, "the build failed with clear diagnostics"), true
	case ev.LintFailed:
		return autoFix(LintFailure, "lint/static analysis failed"), true
	case ev.TestFailed:
		return autoFix(TestFailure, "a deterministic test failed against an established contract"), true
	default:
		return autoFix(BlockingFindings, "blocking review findings remain"), true
	}
}

// human builds a NEEDS_HUMAN classification.
func human(kind Kind, reason string) Classification {
	return Classification{Kind: kind, Disposition: NeedsHuman, Confidence: High, Reason: reason}
}

// retry builds a RETRY classification.
func retry(kind Kind, reason string) Classification {
	return Classification{Kind: kind, Disposition: Retry, Confidence: High, Reason: reason}
}

// autoFix builds an AUTO_FIX classification.
func autoFix(kind Kind, reason string) Classification {
	return Classification{Kind: kind, Disposition: AutoFix, Confidence: High, Reason: reason}
}

// describe returns detail when it carries text, otherwise the fallback reason.
func describe(detail, fallback string) string {
	if d := strings.TrimSpace(detail); d != "" {
		return fallback + ": " + d
	}
	return fallback
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

// Boundary markers: an explicit human decision/approval/safety boundary.
var (
	approvalMarkers    = []string{"approval", "approve", "authoriz", "consent", "sign-off", "signoff", "permission"}
	securityMarkers    = []string{"security", "safety", "unsafe", "vulnerab", "credential", "secret", "sensitive data"}
	destructiveMarkers = []string{"destructive", "irreversible", "cannot be undone", "can't be undone", "data loss", "permanent damage", "destroys"}
	conflictMarkers    = []string{"conflict", "ambiguous", "ambiguity", "contradict", "unclear", "cannot determine", "can't determine", "no authoritative", "product decision", "requires a decision"}
)

// humanKind recognizes a human-boundary signal in text, in a fixed precedence
// order, and reports the matching kind.
func humanKind(text string) (Kind, bool) {
	switch {
	case matchesAny(text, approvalMarkers):
		return ApprovalRequired, true
	case matchesAny(text, securityMarkers):
		return SecurityBoundary, true
	case matchesAny(text, destructiveMarkers):
		return DestructiveOperation, true
	case matchesAny(text, conflictMarkers):
		return AmbiguousContract, true
	default:
		return "", false
	}
}

// transientKind recognizes a transient provider/tool signal in text.
func transientKind(text string) (Kind, bool) {
	if matchesAny(text, emptyMarkers) {
		return EmptyResponse, true
	}
	if matchesAny(text, transientMarkers) {
		return TransientProvider, true
	}
	if matchesAny(text, toolMarkers) {
		return ToolFailure, true
	}
	return "", false
}

// Signal markers. They are matched case-insensitively as substrings.
var (
	emptyMarkers     = []string{"empty output", "empty response", "returned empty", "no output", "blank response", "empty completion"}
	transientMarkers = []string{
		"connection refused", "connection reset", "reset by peer", "no such host",
		"i/o timeout", "timeout", "timed out", "deadline exceeded", "request canceled",
		"temporarily unavailable", "service unavailable", "unavailable",
		"http 429", "http 5", "too many requests", "server error", "broken pipe",
		"network", "eof", "transient",
	}
	toolMarkers       = []string{"tool failed", "tool failure", "tool error", "tool call failed"}
	incompleteMarkers = []string{
		"no repository change", "did not complete", "did not finalize",
		"iteration_limit", "finalization_limit", "iteration limit",
		"tool-call limit", "tool call limit", "max tool", "max_tool_calls",
		"budget", "ran out of", "exhausted", "unfinished", "incomplete",
		"a retry may succeed", "did not finish", "not finish",
	}
)

// matchesAny reports whether text contains any marker.
func matchesAny(text string, markers []string) bool {
	for _, m := range markers {
		if strings.Contains(text, m) {
			return true
		}
	}
	return false
}
