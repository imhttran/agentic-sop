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
