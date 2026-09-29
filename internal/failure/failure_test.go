package failure

import (
	"errors"
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
			name:        "explicit approval boundary",
			ev:          Evidence{Source: "IMPLEMENT", ApprovalRequired: true},
			disposition: NeedsHuman, kind: ApprovalRequired,
		},
		{
			name:        "explicit approval boundary in an outcome reason",
			ev:          Evidence{Source: "IMPLEMENT", Outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: "required operation needs human authorization"}},
			disposition: NeedsHuman, kind: ApprovalRequired,
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

// TestExplicitHumanRequestStillNeedsHuman proves the tightened fallback still
// honors an explicit request for a human decision.
func TestExplicitHumanRequestStillNeedsHuman(t *testing.T) {
	reasons := []string{"needs auth", "required operation needs human authorization", "this needs a decision"}
	for _, reason := range reasons {
		t.Run(reason, func(t *testing.T) {
			got := Classify(Evidence{Source: "IMPLEMENT", Outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: reason}})
			if got.Disposition != NeedsHuman {
				t.Errorf("classification = %+v, want NEEDS_HUMAN", got)
			}
		})
	}
}
