package failure

import (
	"errors"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// TestClassifyDispositions pins the required failure-classification contract:
// each common failure maps to the disposition SOP's existing lifecycle should
// apply.
func TestClassifyDispositions(t *testing.T) {
	cases := []struct {
		name        string
		ev          Evidence
		disposition Disposition
		kind        Kind
	}{
		// --- AUTO_FIX ---
		{
			name:        "missing deterministic test coverage",
			ev:          Evidence{Source: "VALIDATE", TestsMissing: true},
			disposition: AutoFix, kind: MissingTestCoverage,
		},
		{
			name:        "compiler error with clear diagnostics",
			ev:          Evidence{Source: "VALIDATE", BuildFailed: true},
			disposition: AutoFix, kind: CompilerError,
		},
		{
			name:        "deterministic failing test with an established contract",
			ev:          Evidence{Source: "VALIDATE", TestFailed: true},
			disposition: AutoFix, kind: TestFailure,
		},
		{
			name:        "stale test against the authoritative domain model",
			ev:          Evidence{Source: "VALIDATE", TestFailed: true, StaleTest: true},
			disposition: AutoFix, kind: StaleTest,
		},
		{
			name:        "integration wiring failure",
			ev:          Evidence{Source: "VALIDATE", TestFailed: true, IntegrationWiring: true},
			disposition: AutoFix, kind: IntegrationWiring,
		},
		{
			name:        "lint/static analysis failure",
			ev:          Evidence{Source: "VALIDATE", LintFailed: true},
			disposition: AutoFix, kind: LintFailure,
		},
		{
			name:        "regression introduced by the current task",
			ev:          Evidence{Source: "VALIDATE", TestFailed: true, Regression: true},
			disposition: AutoFix, kind: Regression,
		},
		{
			name:        "conflicting tests resolved by the plan/domain contract",
			ev:          Evidence{Source: "VALIDATE", TestFailed: true, ConflictResolvedByPlan: true},
			disposition: AutoFix, kind: TestFailure,
		},

		// --- CONTINUE ---
		{
			name: "iteration budget exhausted with unfinished work",
			ev: Evidence{Source: "IMPLEMENT", Outcome: &agent.Outcome{
				Status: agent.OutcomeFailed,
				Reason: "the Ollama agent IMPLEMENT did not complete after 24 iterations (model=x, mutation_observed=true, tool_calls=24, termination=iteration_limit)",
			}},
			disposition: Continue, kind: IncompleteImplementation,
		},
		{
			name: "tool budget exhausted with unfinished work",
			ev: Evidence{Source: "FIX", Outcome: &agent.Outcome{
				Status: agent.OutcomeFailed,
				Reason: "tool-call limit reached (40 tool calls); the model did not finish",
			}},
			disposition: Continue, kind: IncompleteImplementation,
		},
		{
			name: "agent made no repository change; a retry may succeed",
			ev: Evidence{Source: "IMPLEMENT", Outcome: &agent.Outcome{
				Status: agent.OutcomeNeedsHuman,
				Reason: "the Ollama agent IMPLEMENT made no repository change after 20 iterations (model=x, tool_calls=20, termination=no_change); a retry may succeed",
			}},
			disposition: Continue, kind: IncompleteImplementation,
		},

		// --- RETRY ---
		{
			name:        "empty provider response",
			ev:          Evidence{Source: "PLAN", Err: errors.New("ollama PLAN returned empty output")},
			disposition: Retry, kind: EmptyResponse,
		},
		{
			name:        "transient provider failure",
			ev:          Evidence{Source: "IMPLEMENT", Err: errors.New("request failed: dial tcp 127.0.0.1:11434: connect: connection refused")},
			disposition: Retry, kind: TransientProvider,
		},
		{
			name:        "transient provider failure inside an outcome reason",
			ev:          Evidence{Source: "IMPLEMENT", Outcome: &agent.Outcome{Status: agent.OutcomeFailed, Reason: "request failed: http 503 service unavailable"}},
			disposition: Retry, kind: TransientProvider,
		},

		// --- NEEDS_HUMAN ---
		{
			name:        "genuinely ambiguous contract",
			ev:          Evidence{Source: "IMPLEMENT", RequirementsConflict: true},
			disposition: NeedsHuman, kind: AmbiguousContract,
		},
		{
			name:        "structured approval boundary",
			ev:          Evidence{Source: "IMPLEMENT", ApprovalRequired: true},
			disposition: NeedsHuman, kind: ApprovalRequired,
		},
		{
			name:        "current-run approval request",
			ev:          Evidence{Source: "IMPLEMENT", Approval: ApprovalRequest},
			disposition: NeedsHuman, kind: ApprovalRequired,
		},
		{
			// Authorization prose is NOT a gate: APPROVAL_REQUIRED is structured only,
			// so an authorization-sounding reason is treated as unfinished work.
			name:        "authorization prose is not an approval boundary",
			ev:          Evidence{Source: "IMPLEMENT", Outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: "required operation needs human authorization"}},
			disposition: Continue, kind: IncompleteImplementation,
		},
		{
			name:        "destructive or unsafe operation",
			ev:          Evidence{Source: "IMPLEMENT", DestructiveOperation: true},
			disposition: NeedsHuman, kind: DestructiveOperation,
		},
		{
			name:        "security boundary",
			ev:          Evidence{Source: "IMPLEMENT", SecurityBoundary: true},
			disposition: NeedsHuman, kind: SecurityBoundary,
		},

		// --- REPLAN ---
		{
			name:        "invalid plan assumptions",
			ev:          Evidence{Source: "PLAN", PlanInvalid: true},
			disposition: Replan, kind: ReplanRequired,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Classify(tc.ev)
			if got.Disposition != tc.disposition {
				t.Errorf("disposition = %s, want %s (kind=%s reason=%q)", got.Disposition, tc.disposition, got.Kind, got.Reason)
			}
			if tc.kind != "" && got.Kind != tc.kind {
				t.Errorf("kind = %s, want %s", got.Kind, tc.kind)
			}
			if got.Confidence == "" {
				t.Error("classification has no confidence")
			}
			if got.Reason == "" {
				t.Error("classification has no reason")
			}
		})
	}
}

// ctrl002NoChangeReason is the real outcome reason from the CTRL002 dogfood run:
// the model exhausted its tool budget during discovery, made no repository change,
// and described the credential/secret work it never got to do. The harness then
// appended its deterministic no-change note. The incidental "credential"/"secret"
// words must not turn this resumable run into a human decision.
const ctrl002NoChangeReason = "No repository change could be performed in this invocation: the tool budget was exhausted by required discovery (reading internal/sopclient/activity.go, run.go, service.go, types.go, store.go, tests, web handlers/server, cmd, templates, README) before any write tool could be executed. The CTRL002 fixes (replacing the leaky substring denylist in sanitizeDetail with structural credential/secret/JWT/URL-credential redaction and full-blob redaction instead of trusting truncation, adding ordering/timestamp guarantee tests, adding a CLI activity output path, and reverting the unrelated .agent-sdlc/config.yaml provider/model change that caused the scope-mismatch blocking finding) have not been applied to the working tree. Since no file was modified, I cannot truthfully report completion. [no repository change was made; retrying]"

// TestToolBudgetExhaustionIsNotAHumanBoundary pins the fix for the CTRL002
// dogfood failure: an invocation that exhausted its budget without changing the
// repository is CONTINUE, even when the model's explanation mentions words that
// would otherwise look like a human boundary (here "credential"/"secret").
func TestToolBudgetExhaustionIsNotAHumanBoundary(t *testing.T) {
	ev := Evidence{Source: "IMPLEMENT", Outcome: &agent.Outcome{
		Status: agent.OutcomeNeedsHuman,
		Reason: ctrl002NoChangeReason,
	}}
	got := Classify(ev)
	if got.Disposition != Continue {
		t.Fatalf("disposition = %s (kind=%s), want CONTINUE", got.Disposition, got.Kind)
	}
	if got.Kind != IncompleteImplementation {
		t.Errorf("kind = %s, want %s", got.Kind, IncompleteImplementation)
	}
}

// TestHarnessBudgetSignalsAreContinue proves the deterministic budget-exhaustion
// phrasings SOP's own harness emits are CONTINUE regardless of surrounding prose.
func TestHarnessBudgetSignalsAreContinue(t *testing.T) {
	reasons := []string{
		"the Ollama agent IMPLEMENT made no repository change after 24 iterations; a retry may succeed [no repository change was made; retrying]",
		"the Ollama agent FIX did not complete after 24 iterations (termination=iteration_limit)",
		"tool-call limit reached (80 tool calls); the model did not finish",
		"the Ollama agent IMPLEMENT did not finalize (termination=finalization_limit)",
	}
	for _, reason := range reasons {
		t.Run(reason, func(t *testing.T) {
			got := Classify(Evidence{Source: "IMPLEMENT", Outcome: &agent.Outcome{Status: agent.OutcomeFailed, Reason: reason}})
			if got.Disposition != Continue {
				t.Errorf("disposition = %s (kind=%s), want CONTINUE", got.Disposition, got.Kind)
			}
		})
	}
}

// TestAutoFixRespectsBoundedFixBudget proves a repeatedly failing AUTO_FIX is
// bounded: once the configured fix budget is spent the disposition escalates to a
// human rather than looping forever.
func TestAutoFixRespectsBoundedFixBudget(t *testing.T) {
	ev := Evidence{Source: "VALIDATE", TestFailed: true, FixCycles: 2, MaxFixCycles: 3}
	if got := Classify(ev); got.Disposition != AutoFix {
		t.Fatalf("with budget remaining: disposition = %s, want AUTO_FIX", got.Disposition)
	}

	ev.FixCycles = 3 // budget spent
	got := Classify(ev)
	if got.Disposition != NeedsHuman {
		t.Errorf("with budget spent: disposition = %s, want NEEDS_HUMAN (bounded)", got.Disposition)
	}
	if got.Kind != AutoFixExhausted {
		t.Errorf("with budget spent: kind = %s, want %s", got.Kind, AutoFixExhausted)
	}
}

// TestUnknownEvidenceFailsClosedToHuman proves the classifier never guesses: with
// no authoritative signal it escalates.
func TestUnknownEvidenceFailsClosedToHuman(t *testing.T) {
	if got := Classify(Evidence{}); got.Disposition != NeedsHuman {
		t.Errorf("empty evidence: disposition = %s, want NEEDS_HUMAN", got.Disposition)
	}
	if got := Classify(Evidence{Err: errors.New("some unexplained internal error")}); got.Disposition != NeedsHuman {
		t.Errorf("unrecognized error: disposition = %s, want NEEDS_HUMAN", got.Disposition)
	}
}

// TestHumanBoundariesOutrankVerification proves an explicit human boundary is not
// overridden by an otherwise auto-fixable verification failure.
func TestHumanBoundariesOutrankVerification(t *testing.T) {
	got := Classify(Evidence{Source: "VALIDATE", TestFailed: true, ApprovalRequired: true})
	if got.Disposition != NeedsHuman {
		t.Errorf("disposition = %s, want NEEDS_HUMAN (approval outranks AUTO_FIX)", got.Disposition)
	}
}

// TestRetryableHelper pins which dispositions the lifecycle routes to the bounded
// requeue path.
func TestRetryableHelper(t *testing.T) {
	cases := map[Disposition]bool{
		Continue: true, Retry: true,
		AutoFix: false, Replan: false, NeedsHuman: false, "": false,
	}
	for d, want := range cases {
		if got := (Classification{Disposition: d}).Retryable(); got != want {
			t.Errorf("Retryable(%q) = %v, want %v", d, got, want)
		}
	}
}

// jevMalformedReason is the real reason a fail-closed JEV analysis produces after
// its bounded corrective retries are exhausted (the CTRL007 dogfood failure). It
// must be classified as a bounded, non-human failure with the reason preserved.
const jevMalformedReason = "JEV analysis failed (fail closed): jev: provider failure (MALFORMED_OUTPUT) after 3 attempts: model output is not valid JSON: invalid character '#' looking for beginning of value"

// TestJEVAnalysisFailureIsBoundedNotHuman pins the fix for the CTRL007 dogfood
// failure: a JEV analysis that cannot produce a valid result is fail-closed but
// not a human decision. It is a bounded retry, and its authoritative reason is
// preserved verbatim.
func TestJEVAnalysisFailureIsBoundedNotHuman(t *testing.T) {
	got := Classify(Evidence{Source: "VALIDATE", JEVFailed: true, JEVReason: jevMalformedReason})
	if got.Disposition != Retry {
		t.Fatalf("disposition = %s, want RETRY", got.Disposition)
	}
	if got.Kind != JEVAnalysisFailure {
		t.Errorf("kind = %s, want %s", got.Kind, JEVAnalysisFailure)
	}
	if got.Reason != jevMalformedReason {
		t.Errorf("reason = %q, want the preserved JEV reason", got.Reason)
	}
}

// TestJEVAnalysisFailureWithoutReasonStillBounded asserts a JEV failure with no
// recorded reason still classifies as a bounded, non-human retry with a reason.
func TestJEVAnalysisFailureWithoutReasonStillBounded(t *testing.T) {
	got := Classify(Evidence{Source: "VALIDATE", JEVFailed: true})
	if got.Disposition != Retry || got.Kind != JEVAnalysisFailure {
		t.Fatalf("got %s/%s, want RETRY/%s", got.Disposition, got.Kind, JEVAnalysisFailure)
	}
	if got.Reason == "" {
		t.Error("classification has no reason")
	}
}

// TestJEVFailureDoesNotOverrideHumanBoundary asserts an explicit human boundary
// still wins over a coincident JEV analysis failure.
func TestJEVFailureDoesNotOverrideHumanBoundary(t *testing.T) {
	got := Classify(Evidence{Source: "VALIDATE", JEVFailed: true, JEVReason: jevMalformedReason, ApprovalRequired: true})
	if got.Disposition != NeedsHuman {
		t.Errorf("disposition = %s, want NEEDS_HUMAN (a human boundary outranks a JEV failure)", got.Disposition)
	}
}

// ctrl008Reason is the real outcome reason from the CTRL008 dogfood run: the
// agent exhausted its bounded tool budget after productive repository discovery
// and made no repository change. Its explanation mentions ordinary
// security-relevant implementation words (secret, token, prompt, "raw"), but it
// does not request authorization or propose crossing any security boundary.
const ctrl008Reason = "No repository edits were performed in this invocation; discovery completed but tools ended before any write. " +
	"Remaining, fully-scoped work (all sources confirmed against source, no sopclient/API changes needed): " +
	"wire evidenceState into templates; apply the not-run/unavailable convention to empty states; " +
	"add regression tests asserting absent data never renders PASS and no raw prompt/secret text is shown " +
	"for a fixture task carrying prompt/token fields. A later bounded invocation should perform these edits."

// TestProductiveDiscoveryNoMutationIsContinue pins the fix for the CTRL008 dogfood
// failure: an invocation that exhausted its budget after productive discovery made
// no repository change, but that is a bounded continuation — not a human decision —
// and the security-relevant vocabulary in the agent's own prose must not turn it
// into a SECURITY_BOUNDARY.
func TestProductiveDiscoveryNoMutationIsContinue(t *testing.T) {
	got := Classify(Evidence{Source: "IMPLEMENT", Outcome: &agent.Outcome{
		Status: agent.OutcomeNeedsHuman,
		Reason: ctrl008Reason,
	}})
	if got.Disposition != Continue {
		t.Fatalf("disposition = %s (kind=%s reason=%q), want CONTINUE", got.Disposition, got.Kind, got.Reason)
	}
	if got.Kind != IncompleteImplementation {
		t.Errorf("kind = %s, want %s", got.Kind, IncompleteImplementation)
	}
	if got.Kind == SecurityBoundary {
		t.Errorf("kind = %s, want no security boundary for ordinary implementation prose", got.Kind)
	}
}

// TestTopicWordsNeverImplySecurityBoundary proves the classifier decides on the
// requested action, not on security-related words appearing anywhere in the
// agent's explanation.
func TestTopicWordsNeverImplySecurityBoundary(t *testing.T) {
	reasons := []string{
		"discovery finished but no edits were made; remaining work is to add a regression test ensuring secrets are not rendered",
		"no raw prompt/secret echoing; the sanitizer must redact token values before rendering, then continue with the edits",
		"required by the acceptance criteria: sanitize token values before rendering so credentials are never displayed",
	}
	for _, reason := range reasons {
		t.Run(reason, func(t *testing.T) {
			got := Classify(Evidence{Source: "IMPLEMENT", Outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: reason}})
			if got.Disposition == NeedsHuman || got.Kind == SecurityBoundary {
				t.Errorf("classification = %+v, want a non-human continuation (no security boundary from topic words)", got)
			}
		})
	}
}

// TestSecurityDecisionStillNeedsHuman proves a genuine security DECISION is still
// a human boundary: an action that changes a trust or privilege boundary, not a
// topic word.
func TestSecurityDecisionStillNeedsHuman(t *testing.T) {
	reasons := []string{
		"the change would weaken authentication so any caller is trusted",
		"exposing local command execution to remote unauthenticated clients is required",
	}
	for _, reason := range reasons {
		t.Run(reason, func(t *testing.T) {
			got := Classify(Evidence{Source: "IMPLEMENT", Outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: reason}})
			if got.Disposition != NeedsHuman || got.Kind != SecurityBoundary {
				t.Errorf("classification = %+v, want NEEDS_HUMAN/SECURITY_BOUNDARY", got)
			}
		})
	}
}

// TestBudgetExhaustionAloneNeverSecurityBoundary pins requirements 8-10: tool
// budget exhaustion, no mutation, and retry exhaustion by themselves never imply
// a security boundary.
func TestBudgetExhaustionAloneNeverSecurityBoundary(t *testing.T) {
	cases := []Evidence{
		{Source: "IMPLEMENT", Outcome: &agent.Outcome{Status: agent.OutcomeFailed, Reason: "tool-call limit reached (40 tool calls); the model did not finish"}},
		{Source: "IMPLEMENT", Outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: "the agent made no repository change"}},
		{Source: "VALIDATE", TestFailed: true, FixCycles: 3, MaxFixCycles: 3},
	}
	for _, ev := range cases {
		if got := Classify(ev); got.Kind == SecurityBoundary {
			t.Errorf("evidence %+v classified as SECURITY_BOUNDARY (%s)", ev.Source, got.Reason)
		}
	}
}

// TestUnexplainedNeedsHumanIsAContinuation proves SOP does not park an agent that
// used the needs_human status without reporting an actual boundary: the harness
// uses needs_human to request a bounded requeue, so it is unfinished work.
func TestUnexplainedNeedsHumanIsAContinuation(t *testing.T) {
	got := Classify(Evidence{Source: "IMPLEMENT", Outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: "stopped here"}})
	if got.Disposition != Continue {
		t.Errorf("disposition = %s (kind=%s), want CONTINUE", got.Disposition, got.Kind)
	}
}

// TestUnexplainedFailureStillFailsClosed proves the fallback did not weaken the
// fail-closed default for a plain failed outcome.
func TestUnexplainedFailureStillFailsClosed(t *testing.T) {
	got := Classify(Evidence{Source: "IMPLEMENT", Outcome: &agent.Outcome{Status: agent.OutcomeFailed, Reason: "boom"}})
	if got.Disposition != NeedsHuman {
		t.Errorf("disposition = %s, want NEEDS_HUMAN (a hard failure is terminal)", got.Disposition)
	}
}

// ctrl008RealReason is the verbatim outcome reason persisted by the CTRL008
// dogfood run. It must classify as a bounded continuation, not SECURITY_BOUNDARY.
const ctrl008RealReason = "No repository edits were performed in this invocation; discovery completed but tools ended before any write. Remaining, fully-scoped work (all sources confirmed against source, no sopclient/API changes needed): (1) internal/web/render.go — the dead helpers evidenceState/notRunLabel are defined but unused; wire evidenceState into templates and split wording so absent-persisted data renders 'unavailable' while present-but-empty renders 'not run'; absence must never yield a PASS-class badge. (2) templates/task.html — add labeled rows/sections not yet present, using confirmed sources only: Started/elapsed (no StartedAt on TaskDetail; only TaskSummary.UpdatedAt, RunInfo.GeneratedAt/UpdatedAt, Attempt.Timestamp — render a derived elapsed/relative value or explicit 'unavailable', never invent one); Provider/model (RunInfo.Provider/Model); Fix cycles (RunInfo.FixCycles); Retry attempts (TaskSummary.Retries() (int,bool) distinguishing true zero from absence); Blocked reason (TaskSummary.BlockedReason, today only raw BlockedBy is shown); Failure classification (Run.Classification{Kind,Disposition,Confidence,Reason}); Recovery disposition (Task.Recovery / Task.Recovering()); Report (Task.Report ReportRef{Present,Path,Name}). (3) templates/task.html + partials — split Validation/Review/JEV/Quality into four visually distinct individually-labeled sections. (4) partials/status.html, review.html, ci.html — apply the not-run/unavailable convention to empty states. (5) static/app.css — add small-screen rules for the detail page. (6) internal/web tests — add regression tests asserting CTRL008 field labels are present, absent data never renders PASS, no raw prompt/secret text for a fixture task carrying prompt/token fields, and viewport meta + responsive rules exist. Verification pending: go build ./... and go test ./internal/web/... ./internal/sopclient/.... A later bounded invocation should perform these edits; no sopclient/API changes are required."

// TestCTRL008ExactReasonContinues pins the fix against the verbatim CTRL008
// reason, so a future marker change cannot silently reintroduce the false
// SECURITY_BOUNDARY.
func TestCTRL008ExactReasonContinues(t *testing.T) {
	got := Classify(Evidence{Source: "IMPLEMENT", Outcome: &agent.Outcome{
		Status: agent.OutcomeNeedsHuman,
		Reason: ctrl008RealReason,
	}})
	if got.Disposition != Continue || got.Kind != IncompleteImplementation {
		t.Fatalf("classification = %+v, want CONTINUE/INCOMPLETE_IMPLEMENTATION", got)
	}
}

// ctrl010CompilerError is the verbatim compiler error from the CTRL010 dogfood
// run: an in-progress edit left a dangling reference and the build went red.
const ctrl010CompilerError = "internal/sopclient/checkpoint_read.go:100:16: undefined: checkpointFromDetail"

// TestBuildFailureOutranksAgentProse pins the CTRL010 fix: structured deterministic
// evidence (a red build) classifies as a compiler error even when the agent's own
// outcome is empty, says only that it could not complete, or literally reports no
// authoritative signal. The agent is never asked to classify its own failure, and a
// deterministic build failure is never UNKNOWN.
func TestBuildFailureOutranksAgentProse(t *testing.T) {
	cases := []struct {
		name string
		ev   Evidence
	}{
		{
			name: "no agent outcome at all",
			ev:   Evidence{Source: "VALIDATE", BuildFailed: true, Detail: ctrl010CompilerError},
		},
		{
			name: "agent says only it could not complete",
			ev: Evidence{Source: "FIX", BuildFailed: true, Detail: ctrl010CompilerError,
				Outcome: &agent.Outcome{Status: agent.OutcomeFailed, Reason: "could not complete"}},
		},
		{
			name: "agent reports no authoritative signal",
			ev: Evidence{Source: "FIX", BuildFailed: true, Detail: ctrl010CompilerError,
				Outcome: &agent.Outcome{Status: agent.OutcomeFailed, Reason: "the agent reported a failure without an authoritative signal"}},
		},
		{
			name: "agent outcome is a bare failed with no reason",
			ev:   Evidence{Source: "FIX", BuildFailed: true, Detail: ctrl010CompilerError, Outcome: &agent.Outcome{Status: agent.OutcomeFailed}},
		},
		{
			name: "agent outcome is needs_human with no boundary",
			ev:   Evidence{Source: "FIX", BuildFailed: true, Detail: ctrl010CompilerError, Outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: "stopped"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Classify(tc.ev)
			if got.Kind != CompilerError {
				t.Errorf("kind = %s, want %s (structured build evidence must win)", got.Kind, CompilerError)
			}
			if got.Disposition != AutoFix {
				t.Errorf("disposition = %s, want AUTO_FIX", got.Disposition)
			}
			if got.Confidence != High {
				t.Errorf("confidence = %s, want HIGH", got.Confidence)
			}
			if !strings.Contains(got.Reason, "checkpointFromDetail") {
				t.Errorf("reason = %q, want it to preserve the compiler diagnostic", got.Reason)
			}
		})
	}
}

// TestDeterministicVerificationKinds pins the distinct deterministic failure kinds:
// a failing test is a TEST_FAILURE, and a red build (which includes a test binary
// that does not compile) is a COMPILER_ERROR.
func TestDeterministicVerificationKinds(t *testing.T) {
	cases := []struct {
		name string
		ev   Evidence
		kind Kind
	}{
		{"deterministic unit assertion failure", Evidence{Source: "VALIDATE", TestFailed: true}, TestFailure},
		{"test binary compile failure", Evidence{Source: "VALIDATE", BuildFailed: true}, CompilerError},
		{"validation command failure", Evidence{Source: "VALIDATE", LintFailed: true}, LintFailure},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Classify(tc.ev)
			if got.Kind != tc.kind || got.Disposition != AutoFix {
				t.Errorf("classification = %+v, want AUTO_FIX/%s", got, tc.kind)
			}
		})
	}
}

// TestStructuredProviderFailuresAreNotUnknown proves provider evidence classifies
// directly (RETRY) and never falls through to UNKNOWN.
func TestStructuredProviderFailuresAreNotUnknown(t *testing.T) {
	reasons := map[string]Kind{
		"request failed: http 500 internal server error": TransientProvider,
		"request failed: i/o timeout after 30s":          TransientProvider,
		"ollama returned an empty response":              EmptyResponse,
	}
	for reason, want := range reasons {
		t.Run(reason, func(t *testing.T) {
			got := Classify(Evidence{Source: "IMPLEMENT", Err: errors.New(reason)})
			if got.Disposition != Retry || got.Kind != want {
				t.Errorf("classification = %+v, want RETRY/%s", got, want)
			}
		})
	}
}

// TestStructuredEvidencePrecludesUnknown proves UNKNOWN is reached only with no
// authoritative evidence at all: any structured signal above it classifies
// directly.
func TestStructuredEvidencePrecludesUnknown(t *testing.T) {
	structured := []Evidence{
		{Source: "VALIDATE", BuildFailed: true},
		{Source: "VALIDATE", TestFailed: true},
		{Source: "IMPLEMENT", Err: errors.New("request failed: http 500")},
		{Source: "VALIDATE", JEVFailed: true},
		{Source: "IMPLEMENT", Outcome: &agent.Outcome{Status: agent.OutcomeFailed, Reason: "did not complete after 24 iterations"}},
	}
	for _, ev := range structured {
		if got := Classify(ev); got.Kind == Unknown {
			t.Errorf("evidence %+v classified as UNKNOWN; structured evidence must decide", ev.Source)
		}
	}

	// With nothing structured, UNKNOWN is the honest answer.
	got := Classify(Evidence{})
	if got.Kind != Unknown || got.Confidence != Low {
		t.Errorf("empty evidence = %+v, want UNKNOWN/LOW", got)
	}
}

// TestUnknownIsNotABoundary proves UNKNOWN is not promoted to an authority
// boundary: it is not one of the boundary kinds, so the autonomy policy (not the
// classifier) decides what to do with it. It never carries a boundary risk on its
// own.
func TestUnknownIsNotABoundary(t *testing.T) {
	got := Classify(Evidence{})
	switch got.Kind {
	case SecurityBoundary, ApprovalRequired, DestructiveOperation, AmbiguousContract:
		t.Errorf("empty evidence classified as boundary kind %s; UNKNOWN must stay unclassified", got.Kind)
	}
	if got.Kind != Unknown {
		t.Errorf("kind = %s, want UNKNOWN", got.Kind)
	}
}

// TestHarnessNoChangeSignalOutranksVerification pins the precedence between a
// deterministic build failure and SOP's OWN harness budget/no-change signal: when
// the harness reports the invocation stopped short of changing the repository, the
// red tree is red because the work is not done yet, so the disposition is a bounded
// CONTINUE, not a fix cycle or an escalation.
func TestHarnessNoChangeSignalOutranksVerification(t *testing.T) {
	got := Classify(Evidence{
		Source:      "FIX",
		BuildFailed: true,
		Outcome: &agent.Outcome{
			Status: agent.OutcomeNeedsHuman,
			Reason: "the Ollama agent FIX made no repository change after 24 iterations; a retry may succeed [no repository change was made; retrying]",
		},
	})
	if got.Disposition != Continue || got.Kind != IncompleteImplementation {
		t.Fatalf("classification = %+v, want CONTINUE/INCOMPLETE_IMPLEMENTATION", got)
	}
}

// TestUnclassifiedOutcomeStillFailsClosed guards the tightened UNKNOWN semantics:
// an agent outcome with no structured evidence and no recognized signal still fails
// closed rather than being guessed at.
func TestUnclassifiedOutcomeStillFailsClosed(t *testing.T) {
	got := Classify(Evidence{Source: "IMPLEMENT", Outcome: &agent.Outcome{Status: agent.OutcomeFailed, Reason: "boom"}})
	if got.Disposition != NeedsHuman || got.Kind != Unknown {
		t.Errorf("classification = %+v, want NEEDS_HUMAN/UNKNOWN", got)
	}
}

// ctrl011ApprovalTopicReason models the CTRL011 dogfood invocation: productive
// discovery of an approval feature, exhausted turn budget, no mutation. It is
// saturated with approval TOPIC words (approval, approve, ApproveTask, decline,
// NEEDS_HUMAN, WAITING_FOR_HUMAN, StageWaitingForHuman, gate) but requests nothing.
const ctrl011ApprovalTopicReason = "No repository change was made this invocation: the bounded turn budget was exhausted by required discovery " +
	"before any file could be written. Discovery for the Human Approval Controls task is complete. Established facts: sopclient already exposes " +
	"Client.ApproveTask, which truthfully returns ErrOperationUnsupported, and OpApproveTask is StatusUnsupported; SOP currently exposes no `sop approve` " +
	"operation, so the controller must not simulate approval. Remaining implementation is deterministic: add internal/sopclient/approval.go, expose " +
	"Store.Approval on TaskDetail, render an approve/decline control only when SOP reports a real human-boundary state (a NEEDS_HUMAN disposition or a " +
	"WAITING_FOR_HUMAN stage, i.e. StageWaitingForHuman), implement a non-mutating decline button, document that approval is unsupported, and add tests " +
	"covering the approval gate. A later bounded invocation should perform these edits."

// ctrl012Reason models the CTRL012 dogfood outcome: a partially implemented
// reconciliation/approval feature. It is saturated with approval and
// reconciliation vocabulary (explicit approval, per-task approval, accept-changed,
// reconciliation controls, NEEDS_HUMAN) but requests nothing for the CURRENT run.
const ctrl012Reason = "Incomplete: I rewrote internal/sopclient/changed_task.go (added ChangedTasks/ChangedExecutedTask read model " +
	"and Store.ChangedTasks reading SOP's reconcile.json) and internal/sopclient/boundary.go (flipped OpGetChangedExecutedTasks/OpAcceptChangedTask " +
	"to StatusSupported with real Client.ChangedExecutedTasks/AcceptChangedTask). However the blocking finding also requires web handlers/routes/templates " +
	"surfacing the changed-task list and per-task approval controls, which I did not add, and I did not update internal/sopclient/boundary_test.go, " +
	"whose assertions still expect these ops to be unsupported and ErrOperationUnsupported. As left, go test ./internal/sopclient/... fails and the " +
	"acceptance criteria 'all changed executed tasks reported before mutation' and 'each changed task requires explicit approval' are still not wired " +
	"into the UI. More work is required to finish: update boundary_test.go, add project-page changed-task presentation + per-task accept routes/handlers, " +
	"enforce reconcile-before-mutation ordering, and add tests."

// TestCTRL012PlanReconciliationIsNotApprovalRequired pins the CTRL012 fix: a task
// implementing plan-reconciliation/approval controls, whose summary is full of
// approval and reconciliation vocabulary, is NEVER APPROVAL_REQUIRED. With no
// structured current-run approval state, it is a bounded continuation.
func TestCTRL012PlanReconciliationIsNotApprovalRequired(t *testing.T) {
	got := Classify(Evidence{Source: "IMPLEMENT", Outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: ctrl012Reason}})
	if got.Kind == ApprovalRequired || got.Disposition == NeedsHuman {
		t.Fatalf("classification = %+v, want a non-human continuation (approval/reconciliation is the subject, not a request)", got)
	}
	if got.Disposition != Continue || got.Kind != IncompleteImplementation {
		t.Errorf("classification = %+v, want CONTINUE/INCOMPLETE_IMPLEMENTATION", got)
	}
}

// TestCTRL012FailingTestsAutoFix proves the failing deterministic tests in the
// CTRL012 scenario are the authoritative signal: the structured test result
// outranks the approval-heavy prose and the failure is auto-fixable.
func TestCTRL012FailingTestsAutoFix(t *testing.T) {
	got := Classify(Evidence{
		Source:     "VALIDATE",
		Outcome:    &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: ctrl012Reason},
		TestFailed: true,
		Detail:     "UNIT_TEST `go test ./internal/sopclient/...`: boundary_test.go still expects these ops to be unsupported",
	})
	if got.Kind != TestFailure || got.Disposition != AutoFix {
		t.Fatalf("classification = %+v, want AUTO_FIX/TEST_FAILURE", got)
	}
}

// TestCTRL011ApprovalSubjectIsNotAnApprovalRequest pins the CTRL011 fix: an
// invocation that is IMPLEMENTING an approval feature, and exhausted its budget
// after productive discovery, is a bounded continuation — not APPROVAL_REQUIRED and
// not a human decision. The approval vocabulary is the subject of the work, not a
// request that the current execution be authorized.
func TestCTRL011ApprovalSubjectIsNotAnApprovalRequest(t *testing.T) {
	got := Classify(Evidence{Source: "IMPLEMENT", Outcome: &agent.Outcome{
		Status: agent.OutcomeNeedsHuman,
		Reason: ctrl011ApprovalTopicReason,
	}})
	if got.Disposition != Continue || got.Kind != IncompleteImplementation {
		t.Fatalf("classification = %+v, want CONTINUE/INCOMPLETE_IMPLEMENTATION", got)
	}
	if got.Kind == ApprovalRequired {
		t.Error("implementing approval controls must not be read as an approval request")
	}
	if got.Confidence != High {
		t.Errorf("confidence = %s, want HIGH", got.Confidence)
	}
}

// TestApprovalTopicWordsAreNotApprovalRequests proves none of the approval-related
// statements SOP will see while implementing approval functionality is treated as a
// request for the current execution to be authorized.
func TestApprovalTopicWordsAreNotApprovalRequests(t *testing.T) {
	reasons := []string{
		"implement approval UI",
		"add an ApproveTask interface to the client",
		"render the NEEDS_HUMAN state read-only",
		"test approval behavior end to end",
		"add an unsupported approval capability descriptor",
		"document that approval is unsupported",
		"implement a decline button",
		"add tests for explicit approval gates",
		"inspect StageWaitingForHuman",
		"render the label human approval required",
		"the test fixture sets disposition = NEEDS_HUMAN",
		"SOP exposes no `sop approve` command; the contract marks OpApproveTask unsupported",
	}
	for _, reason := range reasons {
		t.Run(reason, func(t *testing.T) {
			got := Classify(Evidence{Source: "IMPLEMENT", Outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: reason}})
			if got.Kind == ApprovalRequired || got.Disposition == NeedsHuman {
				t.Errorf("classification = %+v, want a non-human continuation (approval topic, not a request)", got)
			}
		})
	}
}

// TestCapabilityGapWithUnsupportedContractContinues pins capability-gap semantics:
// a missing operation whose behavior the authoritative contract already fixes (an
// explicitly-unsupported capability the consumer must report truthfully) does not
// prevent deterministic work, so it is a continuation, not a human decision.
func TestCapabilityGapWithUnsupportedContractContinues(t *testing.T) {
	reason := "SOP exposes no `sop approve` operation. OpApproveTask is StatusUnsupported and Client.ApproveTask returns ErrOperationUnsupported, " +
		"so the controller can implement the read-only boundary truthfully and must not simulate approval; implementation locations are known."
	got := Classify(Evidence{Source: "IMPLEMENT", Outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: reason}})
	if got.Disposition != Continue || got.Kind != IncompleteImplementation {
		t.Fatalf("classification = %+v, want CONTINUE/INCOMPLETE_IMPLEMENTATION (unsupported is not unresolved intent)", got)
	}
	if got.Kind == ApprovalRequired {
		t.Error("a missing capability with an authoritative unsupported contract is not an approval boundary")
	}
}

// TestStructuredApprovalSignalsRequireHuman proves the structured signals that MAY
// establish a human boundary still do: an explicit approval requirement, a
// destructive/irreversible action, and an unresolved requirements conflict. Current
// lifecycle state, not prose, is what these represent.
func TestStructuredApprovalSignalsRequireHuman(t *testing.T) {
	cases := map[string]Evidence{
		"explicit approval gate":           {Source: "IMPLEMENT", ApprovalRequired: true},
		"destructive operation":            {Source: "IMPLEMENT", DestructiveOperation: true},
		"unresolved requirements conflict": {Source: "PLAN", RequirementsConflict: true},
	}
	for name, ev := range cases {
		t.Run(name, func(t *testing.T) {
			got := Classify(ev)
			if got.Disposition != NeedsHuman {
				t.Errorf("classification = %+v, want NEEDS_HUMAN", got)
			}
			if got.Confidence != High {
				t.Errorf("confidence = %s, want HIGH", got.Confidence)
			}
		})
	}
}

// TestApprovalProseIsNotAnApprovalBoundary proves authorization-sounding agent prose
// never produces APPROVAL_REQUIRED: an approval gate is structured only, coming
// from the current run's lifecycle state. These reasons are unfinished work, not a
// request that the current execution be authorized.
func TestApprovalProseIsNotAnApprovalBoundary(t *testing.T) {
	reasons := []string{
		"needs auth",
		"this requires human authorization to proceed",
		"the operation requires approval to proceed",
		"awaiting authorization",
		"each changed task requires explicit approval",
		"per-task approval controls and accept-changed routes remain",
	}
	for _, reason := range reasons {
		t.Run(reason, func(t *testing.T) {
			got := Classify(Evidence{Source: "IMPLEMENT", Outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: reason}})
			if got.Kind == ApprovalRequired {
				t.Errorf("classification = %+v, want NOT APPROVAL_REQUIRED (prose is not a gate)", got)
			}
		})
	}
}

// TestStructuredApprovalBoundariesRequireHuman proves each structured current-run
// approval source produces APPROVAL_REQUIRED, and that APPROVAL_REQUIRED is a
// structured-only outcome.
func TestStructuredApprovalBoundariesRequireHuman(t *testing.T) {
	cases := map[string]Evidence{
		"active approval request":   {Source: "IMPLEMENT", Approval: ApprovalRequest},
		"waiting for human stage":   {Source: "IMPLEMENT", Approval: ApprovalWaitingForHuman},
		"plan approval gate":        {Source: "IMPLEMENT", Approval: ApprovalPlanGate},
		"approval-required boolean": {Source: "IMPLEMENT", ApprovalRequired: true},
	}
	for name, ev := range cases {
		t.Run(name, func(t *testing.T) {
			got := Classify(ev)
			if got.Kind != ApprovalRequired || got.Disposition != NeedsHuman {
				t.Errorf("classification = %+v, want NEEDS_HUMAN/APPROVAL_REQUIRED", got)
			}
			if got.Confidence != High {
				t.Errorf("confidence = %s, want HIGH", got.Confidence)
			}
		})
	}
}

// TestApprovalGateStructuredOutranksProseAndVerification proves the structured
// approval gate outranks a deterministic verification failure and unrelated prose.
func TestApprovalGateStructuredOutranksProseAndVerification(t *testing.T) {
	got := Classify(Evidence{Source: "VALIDATE", TestFailed: true, Approval: ApprovalRequest})
	if got.Kind != ApprovalRequired || got.Disposition != NeedsHuman {
		t.Fatalf("classification = %+v, want NEEDS_HUMAN/APPROVAL_REQUIRED", got)
	}
	// An authoritative reason is preserved verbatim.
	got = Classify(Evidence{Source: "IMPLEMENT", Approval: ApprovalPlanGate, ApprovalReason: "the plan gate requires explicit approval"})
	if got.Reason != "the plan gate requires explicit approval" {
		t.Errorf("reason = %q, want the supplied authoritative reason", got.Reason)
	}
}

// TestInspectedHumanStateIsNotCurrentApprovalGate makes the distinction explicit:
// inspected/fixture state (another component's or a domain value's NEEDS_HUMAN /
// WAITING_FOR_HUMAN) is not the CURRENT execution's lifecycle state, while a
// structured approval signal for the current run is a human boundary.
func TestInspectedHumanStateIsNotCurrentApprovalGate(t *testing.T) {
	inspected := []string{
		"the source renders StageWaitingForHuman and the NEEDS_HUMAN disposition",
		"the test fixture sets disposition = NEEDS_HUMAN and stage = WAITING_FOR_HUMAN",
		"the recovery control is shown when the plan gate is active",
	}
	for _, reason := range inspected {
		t.Run(reason, func(t *testing.T) {
			got := Classify(Evidence{Source: "IMPLEMENT", Outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: reason}})
			if got.Kind == ApprovalRequired || got.Disposition == NeedsHuman {
				t.Errorf("classification = %+v, want a continuation (inspected state is not the current gate)", got)
			}
		})
	}

	// The current run's genuine approval gate is a structured signal.
	got := Classify(Evidence{Source: "IMPLEMENT", ApprovalRequired: true})
	if got.Kind != ApprovalRequired || got.Disposition != NeedsHuman {
		t.Errorf("structured approval gate = %+v, want NEEDS_HUMAN/APPROVAL_REQUIRED", got)
	}
}

// TestExplicitHumanRequestStillNeedsHuman proves the tightened fallback still
// honors an explicit request for a human DECISION (an unresolved requirements
// conflict). Authorization/approval prose is covered separately: it is not a gate.
func TestExplicitHumanRequestStillNeedsHuman(t *testing.T) {
	reasons := []string{"this needs a decision", "the requirements conflict with no authoritative resolution"}
	for _, reason := range reasons {
		t.Run(reason, func(t *testing.T) {
			got := Classify(Evidence{Source: "IMPLEMENT", Outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: reason}})
			if got.Disposition != NeedsHuman {
				t.Errorf("classification = %+v, want NEEDS_HUMAN", got)
			}
		})
	}
}
