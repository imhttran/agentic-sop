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
//
// A boundary is decided on the ACTION or DECISION the agent is requesting, never
// on topic words in its prose: an agent saying it will "sanitize token values" or
// "keep secrets out of the rendered page" is describing ordinary (even
// security-relevant) implementation work, not asking to cross a security
// boundary. The same holds for approval, and goes further: APPROVAL_REQUIRED is
// STRUCTURED ONLY. It comes exclusively from the current run's lifecycle state (an
// active approval request, the WAITING_FOR_HUMAN stage, or an active plan approval
// gate), never from free-form prose and never from inspected/domain/fixture values.
// An agent IMPLEMENTING approval or reconciliation controls (an approve/accept
// button, an ApproveTask interface, tests for an approval gate, a rendered
// NEEDS_HUMAN or WAITING_FOR_HUMAN state, a changed-task accept flow, or another
// component's explicitly-unsupported approve operation) is doing ordinary work,
// not asking the current execution to be authorized. A destructive/irreversible
// action, a security DECISION (weakening authentication, exposing a local
// capability to unauthenticated clients), or an unresolved requirements conflict
// is still a human boundary, recognized from the action wording. Structured
// Evidence fields are authoritative when a caller can set them.
//
// Evidence precedence is fixed and structured evidence outranks prose: explicit
// human boundaries, then an invalid plan, then SOP's OWN deterministic budget/
// no-change signals (a harness budget exhaustion, or a claimed change with none
// produced), then deterministic verification (build, tests, lint, findings), then a
// JEV analysis failure, then an infrastructure error, and only then the agent's own
// outcome (its structured status plus its free-form summary). A deterministic
// compiler error is therefore never UNKNOWN merely because the agent's summary did
// not name it: the structured `go build` result is authoritative.
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
	// NoChangesProduced: a mutating invocation reported success (or reported that it
	// had applied a fix) while leaving the working tree unchanged, when SOP expected a
	// change. It is SOP's OWN deterministic verdict — an empty diff observed against a
	// claimed completion — never an agent's prose. Unlike the harness budget/no-change
	// signal (IncompleteImplementation, which means the work is merely unfinished), the
	// invocation CLAIMED the work was done, so it is a legitimate implementation
	// failure that a stronger model, or a tighter change scope, may resolve; the
	// bounded recovery policy may therefore escalate it rather than fail closed.
	NoChangesProduced Kind = "NO_CHANGES_PRODUCED"
	AutoFixExhausted  Kind = "AUTO_FIX_EXHAUSTED"
	TransientProvider Kind = "TRANSIENT_PROVIDER"
	EmptyResponse     Kind = "EMPTY_RESPONSE"
	ToolFailure       Kind = "TOOL_FAILURE"
	// JEVAnalysisFailure: the optional JEV analysis ran but could not produce a
	// usable result (a malformed/invalid structured output after the bounded
	// corrective retries, or an INCOMPLETE/ERROR status). It is a bounded,
	// non-human failure.
	JEVAnalysisFailure   Kind = "JEV_ANALYSIS_FAILURE"
	ReplanRequired       Kind = "REPLAN_REQUIRED"
	AmbiguousContract    Kind = "AMBIGUOUS_CONTRACT"
	ApprovalRequired     Kind = "APPROVAL_REQUIRED"
	SecurityBoundary     Kind = "SECURITY_BOUNDARY"
	DestructiveOperation Kind = "DESTRUCTIVE_OPERATION"
	Unknown              Kind = "UNKNOWN"
)

// ApprovalBoundary names the CURRENT run's structured source of a human approval
// gate. It is the only thing besides the ApprovalRequired shorthand that can
// produce APPROVAL_REQUIRED, and it is set from SOP's own current-run lifecycle
// state — never from free-form prose and never from inspected/domain/fixture
// values (which describe the feature being implemented, not the current run).
type ApprovalBoundary string

const (
	// ApprovalNone: the current run is not at an approval boundary.
	ApprovalNone ApprovalBoundary = ""
	// ApprovalRequest: the current task/run has an active (unresolved) SOP approval
	// request.
	ApprovalRequest ApprovalBoundary = "APPROVAL_REQUEST"
	// ApprovalWaitingForHuman: the current run's lifecycle stage is
	// WAITING_FOR_HUMAN.
	ApprovalWaitingForHuman ApprovalBoundary = "WAITING_FOR_HUMAN"
	// ApprovalPlanGate: the current plan has an active explicit approval gate.
	ApprovalPlanGate ApprovalBoundary = "PLAN_APPROVAL_GATE"
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
	// Detail is the bounded diagnostic for a deterministic verification failure
	// (for example the compiler error line from a failing `go build`). It is
	// optional provenance: when set it is appended to the classification reason so
	// a report explains WHY SOP chose the failure kind instead of relying on the
	// agent's summary. It never changes the disposition.
	Detail string
	// NoChangesProduced is SOP's own deterministic verdict that a mutating
	// invocation reported success (or a fix) while producing no repository change,
	// when a change was expected. SOP observes it directly from an empty
	// working-tree diff against a claimed completion, so it is authoritative
	// evidence — never agent prose. It is distinct from the harness budget/no-change
	// signal (a continuation): the invocation claimed the work was done, so it is an
	// implementation failure.
	NoChangesProduced bool

	// Authoritative signals, supplied when a caller can determine them. They
	// select the specific AUTO_FIX kind; their absence does not change the
	// disposition (a test failure is still auto-fixable).
	StaleTest              bool
	IntegrationWiring      bool
	Regression             bool
	ConflictResolvedByPlan bool

	// Explicit boundaries. Any of these makes the failure a human decision
	// regardless of everything else. ApprovalRequired is the boolean shorthand for
	// Approval; either establishes the CURRENT run's approval gate. They are set
	// from SOP's own current-run lifecycle state, never from agent prose.
	PlanInvalid          bool
	RequirementsConflict bool
	ApprovalRequired     bool
	Approval             ApprovalBoundary
	// ApprovalReason is the authoritative reason for the current approval gate, when
	// a caller has one. It is preserved verbatim in the classification.
	ApprovalReason       string
	SecurityBoundary     bool
	DestructiveOperation bool

	// JEVFailed reports that the optional JEV analysis ran but could not produce
	// a usable result: a malformed/invalid provider result (after the bounded
	// corrective retries), or an INCOMPLETE/ERROR status. It is an analysis
	// failure, not a human decision — fail-closed, but never NEEDS_HUMAN on its
	// own.
	JEVFailed bool
	// JEVReason is the JEV failure's authoritative reason, preserved verbatim in
	// the classification and the report.
	JEVReason string

	// Fix-loop budget. A zero MaxFixCycles means "unknown/unbounded".
	FixCycles    int
	MaxFixCycles int
}

// Classify returns the disposition and reason for the failure described by ev.
// It is pure and deterministic: the same evidence always yields the same result.
func Classify(ev Evidence) Classification {
	// 1. Explicit human boundaries win over every automated disposition: a human
	//    decision, approval, or safety limit is not something SOP may resolve. The
	//    approval gate is STRUCTURED — it comes from the current run's lifecycle
	//    state (an active approval request, the WAITING_FOR_HUMAN stage, or an active
	//    plan approval gate), never from agent prose, a task title, an acceptance
	//    criterion, or inspected domain/fixture state. Free-form prose therefore can
	//    never manufacture APPROVAL_REQUIRED.
	if ev.ApprovalRequired || ev.Approval != ApprovalNone {
		return approvalHuman(ev)
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

	// 3. SOP's own deterministic budget/no-change signal. The agent harness (and the
	//    outcome reconciler) emit these phrasings when an invocation stopped short of
	//    changing the repository — in particular "no repository change after N" and
	//    "tool-call limit reached". Because SOP generated them, they are authoritative
	//    that the work is merely unfinished, and they take precedence over a
	//    verification failure: a tree that is still red is red because the work is not
	//    done yet, not because this invocation regressed it, so the correct disposition
	//    is to continue with a fresh bounded invocation rather than spend a fix cycle or
	//    escalate. A genuine human decision is reported without these markers.
	if c, ok := fromHarness(ev); ok {
		return c
	}

	// 3b. SOP's own deterministic verdict that a mutating invocation CLAIMED success
	//     without changing the repository. Like the harness signal above, SOP produced
	//     it so it is authoritative; unlike that signal, the invocation asserted the
	//     work was done rather than merely running out of budget, so it is an
	//     implementation failure (AUTO_FIX) that the bounded fix loop — and, with
	//     escalation enabled, a stronger model — may resolve.
	if ev.NoChangesProduced {
		return autoFix(NoChangesProduced, "the invocation reported success but produced no repository changes")
	}

	// 4. Deterministic verification: a build, test, lint, or coverage result (or a
	//    blocking finding) is STRUCTURED evidence about what actually happened, so
	//    it outranks the agent's own free-form outcome. A red `go build` with an
	//    undefined symbol is a compiler error even when the agent's summary is empty
	//    or says something else; it is never UNKNOWN. The bounded fix budget still
	//    applies: once it is spent this returns the distinct AutoFixExhausted signal.
	if c, ok := fromVerification(ev); ok {
		return c
	}

	// 5. A JEV analysis failure: JEV ran but could not produce a valid result (a
	//    malformed/invalid structured output after the bounded corrective retries,
	//    or an INCOMPLETE/ERROR status). It is a bounded, non-human failure —
	//    fail-closed, but never NEEDS_HUMAN — and its authoritative reason is
	//    preserved. A valid JEV result with blocking findings is not this case; it
	//    is weighed like any other finding by the gate and the fix loop.
	if ev.JEVFailed {
		reason := strings.TrimSpace(ev.JEVReason)
		if reason == "" {
			reason = "JEV analysis could not produce a valid result; no human decision is required"
		}
		return retry(JEVAnalysisFailure, reason)
	}

	// 6. An infrastructure/provider error (a structured, authoritative signal).
	if c, ok := fromError(ev); ok {
		return c
	}

	// 7. An agent-reported failure outcome: the agent's structured status plus its
	//    own free-form summary. It is the weakest structured signal, so it is
	//    consulted only after every deterministic and infrastructure signal above.
	if c, ok := fromOutcome(ev); ok {
		return c
	}

	// 8. No authoritative signal: fail closed to a human rather than guess.
	return Classification{Kind: Unknown, Disposition: NeedsHuman, Confidence: Low,
		Reason: "no authoritative signal was available; a human decision is required"}
}

// fromHarness recognizes SOP's own deterministic budget/no-change signal: the
// phrasings the agent harness and the outcome reconciler emit when an invocation
// stopped short of changing the repository. Because SOP generated them, they are
// authoritative evidence that the work is merely unfinished, so they are checked
// before a verification failure: a tree that is still red is red because the work is
// not done yet, so the correct disposition is a bounded continuation rather than a
// spent fix cycle or a human boundary.
func fromHarness(ev Evidence) (Classification, bool) {
	if ev.Outcome == nil {
		return Classification{}, false
	}
	reason := strings.TrimSpace(firstNonEmpty(ev.Outcome.Reason, ev.Outcome.Summary))
	if !harnessIncomplete(strings.ToLower(reason)) {
		return Classification{}, false
	}
	return Classification{Kind: IncompleteImplementation, Disposition: Continue, Confidence: High,
		Reason: describe(reason, "the invocation exhausted its budget without changing the repository; no human decision is required")}, true
}

// fromOutcome classifies a non-completed agent outcome. It examines the reason
// text for the boundary, transient, and incomplete signals the model emits, and
// finally falls back on the status the agent reported: an unexplained `needs_human`
// is a bounded continuation (SOP's harness uses it to request a requeue), while an
// unexplained `failed` fails closed to a human. SOP's own deterministic budget/
// no-change signal is handled earlier, by fromHarness.
func fromOutcome(ev Evidence) (Classification, bool) {
	if ev.Outcome == nil {
		return Classification{}, false
	}
	reason := strings.TrimSpace(firstNonEmpty(ev.Outcome.Reason, ev.Outcome.Summary))
	text := strings.ToLower(reason)

	// An explicit human-boundary ACTION/DECISION: an authorization request, a
	// destructive/irreversible action, a security decision, or an unresolved
	// requirements conflict. Topic words alone never match.
	if kind, ok := humanActionKind(text); ok {
		return human(kind, describe(reason, "the agent reported a boundary that requires a human")), true
	}
	// A provider transport failure is reported as a failed outcome; a needs_human
	// outcome is reporting blocked or unfinished work, so its prose is not scanned
	// for transport markers ("unavailable", "network") that also occur in ordinary
	// descriptions of the work still to do.
	if ev.Outcome.Status != agent.OutcomeNeedsHuman {
		if kind, ok := transientKind(text); ok {
			return retry(kind, describe(reason, "a transient failure occurred")), true
		}
	}
	if matchesAny(text, incompleteMarkers) {
		return Classification{Kind: IncompleteImplementation, Disposition: Continue, Confidence: High,
			Reason: describe(reason, "the implementation is unfinished; no human decision is required")}, true
	}

	// No explicit boundary signal. SOP's own harness uses needs_human to request a
	// bounded requeue — it is not authoritative that a human is needed — so an
	// unexplained needs_human is unfinished work, not a human decision. A plain
	// `failed` outcome with no signal still fails closed rather than looping.
	if ev.Outcome.Status == agent.OutcomeNeedsHuman {
		return Classification{Kind: IncompleteImplementation, Disposition: Continue, Confidence: Medium,
			Reason: describe(reason, "the agent stopped without an explicit human-boundary signal; the work continues unless a boundary is reported")}, true
	}
	return human(Unknown, describe(reason, "the agent reported a failure without an authoritative signal")), true
}

// fromError classifies a provider/infrastructure error by its text.
func fromError(ev Evidence) (Classification, bool) {
	if ev.Err == nil {
		return Classification{}, false
	}
	text := strings.ToLower(ev.Err.Error())

	// As in fromOutcome: a deterministic harness budget/no-change signal is
	// authoritative over incidental human-boundary keywords.
	if harnessIncomplete(text) {
		return Classification{Kind: IncompleteImplementation, Disposition: Continue, Confidence: Medium,
			Reason: describe(ev.Err.Error(), "the invocation exhausted its budget without changing the repository; no human decision is required")}, true
	}

	if kind, ok := humanActionKind(text); ok {
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
// bounded and escalates to a human. It is the strongest structured signal, so the
// classifier consults it before any agent-reported outcome. A red build is the
// authoritative BUILD_FAILURE class; the existing vocabulary names it CompilerError.
func fromVerification(ev Evidence) (Classification, bool) {
	autoFixable := ev.BuildFailed || ev.TestFailed || ev.LintFailed || ev.TestsMissing || ev.BlockingFindings > 0
	if !autoFixable {
		return Classification{}, false
	}

	if ev.MaxFixCycles > 0 && ev.FixCycles >= ev.MaxFixCycles {
		return Classification{Kind: AutoFixExhausted, Disposition: NeedsHuman, Confidence: High,
			Reason: describe(ev.Detail, fmt.Sprintf("automatic fixes were exhausted (%d/%d) without resolving the failure", ev.FixCycles, ev.MaxFixCycles))}, true
	}

	switch {
	case ev.IntegrationWiring:
		return autoFix(IntegrationWiring, describe(ev.Detail, "an integration wiring error with established interfaces can be corrected")), true
	case ev.Regression:
		return autoFix(Regression, describe(ev.Detail, "a regression introduced by the current task can be corrected")), true
	case ev.StaleTest:
		return autoFix(StaleTest, describe(ev.Detail, "a stale test conflicts with the authoritative contract; the test should be updated")), true
	case ev.ConflictResolvedByPlan:
		return autoFix(TestFailure, describe(ev.Detail, "the plan/domain contract resolves the conflicting tests; the tests should be updated")), true
	case ev.TestsMissing:
		return autoFix(MissingTestCoverage, describe(ev.Detail, "required deterministic test coverage is missing")), true
	case ev.BuildFailed:
		return autoFix(CompilerError, describe(ev.Detail, "the build failed with clear diagnostics")), true
	case ev.LintFailed:
		return autoFix(LintFailure, describe(ev.Detail, "lint/static analysis failed")), true
	case ev.TestFailed:
		return autoFix(TestFailure, describe(ev.Detail, "a deterministic test failed against an established contract")), true
	default:
		return autoFix(BlockingFindings, describe(ev.Detail, "blocking review findings remain")), true
	}
}

// human builds a NEEDS_HUMAN classification.
func human(kind Kind, reason string) Classification {
	return Classification{Kind: kind, Disposition: NeedsHuman, Confidence: High, Reason: reason}
}

// approvalHuman builds the APPROVAL_REQUIRED classification for a structured
// current-run approval gate, preserving a caller-supplied authoritative reason
// when one exists.
func approvalHuman(ev Evidence) Classification {
	reason := strings.TrimSpace(ev.ApprovalReason)
	if reason == "" {
		reason = "the current run is at an authoritative approval boundary; a human decision is required"
	}
	return human(ApprovalRequired, reason)
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

// harnessIncompleteMarkers are phrases SOP's own agent harness (and the outcome
// reconciler) emit deterministically when an invocation stopped short of changing
// the repository: iteration/tool-budget exhaustion, a no-change finalization, or a
// no-change failure surfaced as a retryable boundary. Because SOP generated them,
// they are authoritative evidence that the work is merely unfinished, so they take
// precedence over any human-boundary keyword in the model's own explanation. A
// genuine human decision (approval, safety, ambiguity) is reported without these
// markers and still reaches NEEDS_HUMAN.
var harnessIncompleteMarkers = []string{
	"no repository change was made; retrying",
	"made no repository change after",
	"tool-call limit reached",
	"did not complete after",
	"did not finalize",
}

// harnessIncomplete reports whether text carries a deterministic harness
// budget/no-change signal.
func harnessIncomplete(text string) bool {
	return matchesAny(text, harnessIncompleteMarkers)
}

// Boundary markers: an explicit human decision/safety boundary inferred from the
// agent's own ACTION wording. APPROVAL is deliberately absent: an approval gate is
// STRUCTURED (Evidence.Approval / Evidence.ApprovalRequired, set from the current
// run's lifecycle state) and is never inferred from prose, a task title, an
// acceptance criterion, or inspected/fixture state. An agent may describe approval,
// reconciliation, accept-changed, or NEEDS_HUMAN/WAITING_FOR_HUMAN concepts — even
// quote them from requirements it is implementing — without that being a request
// for the current execution to be authorized.
var (
	destructiveMarkers = []string{"destructive", "irreversible", "cannot be undone", "can't be undone", "data loss", "permanent damage", "destroys"}
	// conflictMarkers recognize an unresolved requirements CONFLICT or an explicit
	// request for an authoritative decision about one — not mere uncertainty about
	// what to do next, and not a request for authorization (approval is structured
	// only). The bare "needs/requires a human" phrasings are deliberately absent: a
	// request to be authorized is approval, which is never inferred from prose.
	conflictMarkers = []string{
		"conflict", "ambiguous", "ambiguity", "contradict", "unclear",
		"cannot determine", "can't determine", "no authoritative", "product decision",
		"requires a decision", "needs a decision",
	}
)

// securityDecisionMarkers recognize a security DECISION or authorization — an
// action that changes a trust/privilege boundary — rather than the topic of the
// work. Narrow on purpose: common implementation words (secret, token, prompt,
// sanitize, security, boundary, raw data, credential) are deliberately absent, so
// an ordinary security-relevant change required by the task's acceptance criteria
// is not misread as a request to cross a boundary.
var securityDecisionMarkers = []string{
	"weaken", "weakening",
	"bypass",
	"unauthenticated",
	"grant access", "grants access", "grant new",
	"escalate privilege", "privilege boundary",
	"trust boundary",
	"authentication policy", "authorization policy",
	"remote execution", "remote clients",
	"expose local",
}

// humanActionKind recognizes a human-boundary ACTION or DECISION in text, in a
// fixed precedence order, and reports the matching kind. It matches what the
// agent is asking to DO, not which words its prose happens to contain: an agent
// implementing approval controls, rendering a NEEDS_HUMAN state, or describing
// another component's unsupported capability is not requesting authorization for
// the current execution. APPROVAL is deliberately not inferred from prose: a
// current approval gate is the structured Evidence.Approval / ApprovalRequired
// signal, set from the current run's lifecycle state. A security DECISION, a
// destructive action, or an explicit requirements conflict is still recognized
// from the action wording.
func humanActionKind(text string) (Kind, bool) {
	switch {
	case matchesAny(text, securityDecisionMarkers):
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
