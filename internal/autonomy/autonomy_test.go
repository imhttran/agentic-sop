package autonomy

import (
	"errors"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/failure"
)

// riskDecision drives the full pipeline a caller uses: failure evidence -> the
// deterministic classifier -> the autonomy policy. It is how the lifecycle
// composes the two layers.
func riskDecision(t *testing.T, ev failure.Evidence, level Level) Decision {
	t.Helper()
	return Decide(failure.Classify(ev), PolicyFor(level))
}

// TestRecoverableFailuresAreAutomaticUnderHigh pins the core invariant for the
// high level: every common automation failure is handled without a human.
func TestRecoverableFailuresAreAutomaticUnderHigh(t *testing.T) {
	cases := []struct {
		name   string
		ev     failure.Evidence
		action Action
	}{
		{"provider timeout", failure.Evidence{Source: "IMPLEMENT", Err: errors.New("request failed: i/o timeout after 30s")}, ActionAutoRetry},
		{"http 500", failure.Evidence{Source: "IMPLEMENT", Err: errors.New("request failed: http 500 internal server error")}, ActionAutoRetry},
		{"connection reset", failure.Evidence{Source: "IMPLEMENT", Err: errors.New("read tcp: connection reset by peer")}, ActionAutoRetry},
		{"empty provider response", failure.Evidence{Source: "PLAN", Err: errors.New("ollama PLAN returned empty output")}, ActionAutoRetry},
		{"productive iteration exhaustion", failure.Evidence{Source: "IMPLEMENT", Outcome: &agent.Outcome{
			Status: agent.OutcomeFailed,
			Reason: "the Ollama agent IMPLEMENT did not complete after 24 iterations (mutation_observed=true, termination=iteration_limit)",
		}}, ActionAutoContinue},
		{"no-change discovery", failure.Evidence{Source: "IMPLEMENT", Outcome: &agent.Outcome{
			Status: agent.OutcomeNeedsHuman,
			Reason: "the Ollama agent IMPLEMENT made no repository change after 24 iterations; a retry may succeed [no repository change was made; retrying]",
		}}, ActionAutoContinue},
		{"deterministic build failure", failure.Evidence{Source: "VALIDATE", BuildFailed: true}, ActionAutoFix},
		{"deterministic unit failure", failure.Evidence{Source: "VALIDATE", TestFailed: true}, ActionAutoFix},
		{"missing required test", failure.Evidence{Source: "VALIDATE", TestsMissing: true}, ActionAutoFix},
		{"lint failure", failure.Evidence{Source: "VALIDATE", LintFailed: true}, ActionAutoFix},
		{"malformed JEV output", failure.Evidence{Source: "JEV", JEVFailed: true, JEVReason: "jev: provider failure (MALFORMED_OUTPUT)"}, ActionAutoRetry},
		{"recoverable JEV incomplete", failure.Evidence{Source: "JEV", JEVFailed: true, JEVReason: "JEV analysis did not complete (INCOMPLETE)"}, ActionAutoRetry},
		{"deterministic JEV blocking finding", failure.Evidence{Source: "VALIDATE", BlockingFindings: 1}, ActionAutoFix},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := riskDecision(t, tc.ev, High)
			if d.RequiresHuman {
				t.Fatalf("requires human; action = %s reason = %q", d.Action, d.Reason)
			}
			if d.Action != tc.action {
				t.Errorf("action = %s, want %s (reason=%q)", d.Action, tc.action, d.Reason)
			}
			if d.Level != High {
				t.Errorf("level = %s, want HIGH", d.Level)
			}
			if d.Reason == "" {
				t.Error("decision has no reason")
			}
		})
	}
}

// TestHumanBoundariesAreNeverAutomatic proves the authority boundaries require a
// human at every level, including the most autonomous.
func TestHumanBoundariesAreNeverAutomatic(t *testing.T) {
	cases := []struct {
		name string
		ev   failure.Evidence
	}{
		{"conflicting authoritative requirements", failure.Evidence{Source: "PLAN", RequirementsConflict: true}},
		{"destructive operation", failure.Evidence{Source: "IMPLEMENT", DestructiveOperation: true}},
		{"security boundary", failure.Evidence{Source: "IMPLEMENT", SecurityBoundary: true}},
		{"explicit approval gate", failure.Evidence{Source: "IMPLEMENT", ApprovalRequired: true}},
		{"invalid plan assumptions", failure.Evidence{Source: "PLAN", PlanInvalid: true}},
	}
	for _, tc := range cases {
		for _, level := range []Level{Low, Balanced, High} {
			t.Run(tc.name+"/"+string(level), func(t *testing.T) {
				d := riskDecision(t, tc.ev, level)
				if !d.RequiresHuman || d.Action != ActionHumanApproval {
					t.Fatalf("action = %s requiresHuman = %v, want human at %s", d.Action, d.RequiresHuman, level)
				}
				if d.Classification.Disposition != failure.NeedsHuman {
					t.Errorf("classification disposition = %s, want NEEDS_HUMAN", d.Classification.Disposition)
				}
				if d.Risk == RiskNone {
					t.Error("a human boundary must carry a risk")
				}
			})
		}
	}
}

// TestExhaustionIsTerminalUnderHigh proves a bounded automation exhaustion is a
// terminal state under the high level — not a human decision — while the
// conservative levels keep the human boundary.
func TestExhaustionIsTerminalUnderHigh(t *testing.T) {
	ev := failure.Evidence{Source: "VALIDATE", TestFailed: true, FixCycles: 3, MaxFixCycles: 3}

	high := riskDecision(t, ev, High)
	if high.RequiresHuman {
		t.Errorf("high: requires human; want a terminal automation state")
	}
	if high.Action != ActionTerminal {
		t.Errorf("high: action = %s, want TERMINAL", high.Action)
	}
	if high.Classification.Kind != failure.AutoFixExhausted {
		t.Errorf("high: kind = %s, want AUTO_FIX_EXHAUSTED", high.Classification.Kind)
	}

	for _, level := range []Level{Low, Balanced} {
		d := riskDecision(t, ev, level)
		if !d.RequiresHuman {
			t.Errorf("%s: requires human = false; want the conservative human boundary", level)
		}
	}
}

// TestLevelsSelectDifferentDecisions proves the three levels differ as designed.
func TestLevelsSelectDifferentDecisions(t *testing.T) {
	// A deterministic failure with fix budget remaining.
	ev := failure.Evidence{Source: "VALIDATE", TestFailed: true, FixCycles: 0, MaxFixCycles: 3}

	low := riskDecision(t, ev, Low)
	if !low.RequiresHuman {
		t.Errorf("low: a deterministic fix must require approval, got %s", low.Action)
	}

	for _, level := range []Level{Balanced, High} {
		d := riskDecision(t, ev, level)
		if d.RequiresHuman || d.Action != ActionAutoFix {
			t.Errorf("%s: decision = %+v, want AUTO_FIX", level, d)
		}
	}

	// A semantically-equivalent change to an executed task.
	if d := DecidePlanChange(PlanChangeExecutedEquivalent, PolicyFor(Low)); !d.RequiresHuman {
		t.Errorf("low: equivalent executed change must require approval, got %s", d.Action)
	}
	if d := DecidePlanChange(PlanChangeExecutedEquivalent, PolicyFor(Balanced)); !d.RequiresHuman {
		t.Errorf("balanced: equivalent executed change must require approval, got %s", d.Action)
	}
	if d := DecidePlanChange(PlanChangeExecutedEquivalent, PolicyFor(High)); d.RequiresHuman || d.Action != ActionAutoReconcile {
		t.Errorf("high: decision = %+v, want AUTO_RECONCILE", d)
	}
}

// TestTransientRetryIsAutomaticAtEveryLevel proves the safe provider retry is not
// gated by a conservative level.
func TestTransientRetryIsAutomaticAtEveryLevel(t *testing.T) {
	ev := failure.Evidence{Source: "PLAN", Err: errors.New("temporary failure: connection refused")}
	for _, level := range []Level{Low, Balanced, High} {
		d := riskDecision(t, ev, level)
		if d.RequiresHuman || d.Action != ActionAutoRetry {
			t.Errorf("%s: decision = %+v, want AUTO_RETRY", level, d)
		}
	}
}

// TestPolicyOverridesRespectExplicitFlags proves a configured flag overrides the
// level's built-in setting.
func TestPolicyOverridesRespectExplicitFlags(t *testing.T) {
	p := PolicyFor(High)
	p.AutoFix = false
	c := failure.Classification{Kind: failure.TestFailure, Disposition: failure.AutoFix, Confidence: failure.High}
	if d := Decide(c, p); !d.RequiresHuman {
		t.Errorf("an explicit auto_fix: false must require a human, got %s", d.Action)
	}
}

// TestPlanChangeDecisions pins both reconciliation classifications.
func TestPlanChangeDecisions(t *testing.T) {
	high := PolicyFor(High)
	cases := []struct {
		kind        PlanChangeKind
		action      Action
		requires    bool
		wantRiskMin ApprovalRisk
	}{
		{PlanChangeExecutedEquivalent, ActionAutoReconcile, false, RiskMedium},
		{PlanChangeExecutedMaterial, ActionHumanApproval, true, RiskHigh},
	}
	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			d := DecidePlanChange(tc.kind, high)
			if d.Action != tc.action || d.RequiresHuman != tc.requires {
				t.Fatalf("decision = %+v, want action %s requiresHuman %v", d, tc.action, tc.requires)
			}
			if d.Risk != tc.wantRiskMin {
				t.Errorf("risk = %s, want %s", d.Risk, tc.wantRiskMin)
			}
		})
	}
}

// TestDefaultPolicyIsConservative proves an empty/unknown level resolves to the
// conservative default and never to maximum autonomy.
func TestDefaultPolicyIsConservative(t *testing.T) {
	for _, level := range []Level{"", "nonsense"} {
		p := PolicyFor(level)
		if p.Level != Balanced {
			t.Errorf("PolicyFor(%q).Level = %s, want BALANCED", level, p.Level)
		}
	}
	if DefaultLevel != Balanced {
		t.Errorf("DefaultLevel = %s, want BALANCED", DefaultLevel)
	}
}

// ctrl008Outcome is the CTRL008 dogfood outcome: productive discovery that made no
// repository change, described with ordinary security-relevant implementation
// words. It is unfinished work, not a human decision.
var ctrl008Outcome = &agent.Outcome{
	Status: agent.OutcomeNeedsHuman,
	Reason: "No repository edits were performed in this invocation; discovery completed but tools ended before any write. " +
		"Remaining, fully-scoped work: add regression tests asserting absent data never renders PASS and no raw " +
		"prompt/secret text is shown for a fixture task carrying prompt/token fields. A later bounded invocation should perform these edits.",
}

// TestProductiveIncompleteIsAutoContinue pins the CTRL008 fix at the autonomy
// level: productive, low-risk incomplete work is AUTO_CONTINUE with LOW risk — not
// SECURITY_BOUNDARY, not HUMAN_APPROVAL_REQUIRED — under the automatic levels.
func TestProductiveIncompleteIsAutoContinue(t *testing.T) {
	ev := failure.Evidence{Source: "IMPLEMENT", Outcome: ctrl008Outcome}
	for _, level := range []Level{Balanced, High} {
		d := riskDecision(t, ev, level)
		if d.RequiresHuman || d.Action != ActionAutoContinue {
			t.Fatalf("%s: decision = %+v, want AUTO_CONTINUE without a human", level, d)
		}
		if d.Risk != RiskLow {
			t.Errorf("%s: risk = %s, want LOW", level, d.Risk)
		}
		if d.Classification.Kind != failure.IncompleteImplementation {
			t.Errorf("%s: kind = %s, want %s", level, d.Classification.Kind, failure.IncompleteImplementation)
		}
	}
}

// TestLowLevelRetainsCorrectClassification proves the classification and risk do
// not depend on the autonomy level: under LOW the same scenario is still LOW risk
// and INCOMPLETE_IMPLEMENTATION (never mislabeled a security boundary), even if a
// conservative policy would stop earlier.
func TestLowLevelRetainsCorrectClassification(t *testing.T) {
	d := riskDecision(t, failure.Evidence{Source: "IMPLEMENT", Outcome: ctrl008Outcome}, Low)
	if d.Classification.Kind != failure.IncompleteImplementation || d.Classification.Disposition != failure.Continue {
		t.Errorf("classification = %+v, want CONTINUE/INCOMPLETE_IMPLEMENTATION", d.Classification)
	}
	if d.Risk != RiskLow {
		t.Errorf("risk = %s, want LOW (the level must not change the factual classification)", d.Risk)
	}
	if d.Action != ActionAutoContinue {
		t.Errorf("action = %s, want AUTO_CONTINUE (a safe continuation is allowed at every level)", d.Action)
	}
}

// ctrl010CompileError is the CTRL010 dogfood compiler error: a dangling reference
// left by an in-progress edit.
const ctrl010CompileError = "internal/sopclient/checkpoint_read.go:100:16: undefined: checkpointFromDetail"

// TestBuildFailureIsAutoFixLowRisk pins the CTRL010 dogfood decision: a deterministic
// build failure with fix budget remaining is AUTO_FIX with LOW risk under the
// automatic levels, and it is never UNKNOWN or a human decision merely because the
// agent's own status was failed.
func TestBuildFailureIsAutoFixLowRisk(t *testing.T) {
	ev := failure.Evidence{
		Source:       "FIX",
		BuildFailed:  true,
		Detail:       ctrl010CompileError,
		Outcome:      &agent.Outcome{Status: agent.OutcomeFailed},
		FixCycles:    0,
		MaxFixCycles: 3,
	}
	for _, level := range []Level{Balanced, High} {
		d := riskDecision(t, ev, level)
		if d.RequiresHuman || d.Action != ActionAutoFix {
			t.Fatalf("%s: decision = %+v, want AUTO_FIX without a human", level, d)
		}
		if d.Risk != RiskLow {
			t.Errorf("%s: risk = %s, want LOW", level, d.Risk)
		}
		if d.Classification.Kind != failure.CompilerError {
			t.Errorf("%s: kind = %s, want COMPILER_ERROR", level, d.Classification.Kind)
		}
	}

	// Under LOW the fix is withheld for a human, but the classification and risk stay
	// correct: the level never changes what happened.
	low := riskDecision(t, ev, Low)
	if !low.RequiresHuman || low.Action != ActionHumanApproval {
		t.Errorf("low: decision = %+v, want HUMAN_APPROVAL_REQUIRED", low)
	}
	if low.Classification.Kind != failure.CompilerError || low.Risk != RiskLow {
		t.Errorf("low: classification = %+v risk = %s, want COMPILER_ERROR/LOW", low.Classification, low.Risk)
	}
}

// TestStructuredProviderFailuresRetryEveryLevel proves provider evidence yields a
// bounded RETRY at every level, never UNKNOWN and never a human decision.
func TestStructuredProviderFailuresRetryEveryLevel(t *testing.T) {
	errs := []error{
		errors.New("request failed: http 500 internal server error"),
		errors.New("request failed: i/o timeout"),
		errors.New("ollama returned an empty response"),
	}
	for _, err := range errs {
		for _, level := range []Level{Low, Balanced, High} {
			d := riskDecision(t, failure.Evidence{Source: "IMPLEMENT", Err: err}, level)
			if d.RequiresHuman || d.Action != ActionAutoRetry {
				t.Errorf("%s %v: decision = %+v, want AUTO_RETRY", level, err, d)
			}
			if d.Classification.Kind == failure.Unknown {
				t.Errorf("%s %v: classified as UNKNOWN", level, err)
			}
		}
	}
}

// TestExhaustedBuildFixIsBoundedNotUnknown proves a build failure that survives its
// bounded fix budget terminates as AUTO_FIX_EXHAUSTED (a terminal/human boundary by
// policy), never as the misleading UNKNOWN.
func TestExhaustedBuildFixIsBoundedNotUnknown(t *testing.T) {
	ev := failure.Evidence{Source: "VALIDATE", BuildFailed: true, FixCycles: 3, MaxFixCycles: 3}
	high := riskDecision(t, ev, High)
	if high.Classification.Kind != failure.AutoFixExhausted || high.Action != ActionTerminal {
		t.Errorf("high: decision = %+v, want AUTO_FIX_EXHAUSTED/TERMINAL", high)
	}
	balanced := riskDecision(t, ev, Balanced)
	if balanced.Classification.Kind != failure.AutoFixExhausted {
		t.Errorf("balanced: kind = %s, want AUTO_FIX_EXHAUSTED", balanced.Classification.Kind)
	}
}

// ctrl011Outcome is the CTRL011 dogfood outcome: productive discovery of an
// approval feature with the bounded budget spent before any mutation. The prose is
// saturated with approval vocabulary, but it describes the work being implemented.
var ctrl011Outcome = &agent.Outcome{
	Status: agent.OutcomeNeedsHuman,
	Reason: "No repository change was made this invocation: the bounded turn budget was exhausted by required discovery. " +
		"Remaining implementation for the Human Approval Controls task: add approval.go, render an approve/decline control only " +
		"for a NEEDS_HUMAN/WAITING_FOR_HUMAN boundary, and add tests for the approval gate.",
}

// TestCTRL011ApprovalSubjectIsLowRiskContinue pins the CTRL011 dogfood decision at
// the autonomy level: implementing approval controls is low-risk, deterministic
// work, so the automatic levels continue it instead of requiring approval.
func TestCTRL011ApprovalSubjectIsLowRiskContinue(t *testing.T) {
	ev := failure.Evidence{Source: "IMPLEMENT", Outcome: ctrl011Outcome}
	for _, level := range []Level{Balanced, High} {
		d := riskDecision(t, ev, level)
		if d.RequiresHuman || d.Action != ActionAutoContinue {
			t.Fatalf("%s: decision = %+v, want AUTO_CONTINUE without a human", level, d)
		}
		if d.Risk != RiskLow {
			t.Errorf("%s: risk = %s, want LOW", level, d.Risk)
		}
		if d.Classification.Kind != failure.IncompleteImplementation {
			t.Errorf("%s: kind = %s, want INCOMPLETE_IMPLEMENTATION", level, d.Classification.Kind)
		}
	}

	// LOW may stop for a human per its conservative policy, but the factual
	// classification and risk stay correct — never APPROVAL_REQUIRED/HIGH.
	low := riskDecision(t, ev, Low)
	if low.Classification.Kind == failure.ApprovalRequired {
		t.Errorf("low: kind = APPROVAL_REQUIRED, want the approval subject not to be an approval request")
	}
	if low.Risk != RiskLow {
		t.Errorf("low: risk = %s, want LOW", low.Risk)
	}
}

// TestSecurityDecisionRequiresHumanAtEveryLevel proves a genuine security DECISION
// (an action that changes a trust boundary) is a human boundary at every level,
// while ordinary security-related implementation prose is not.
func TestSecurityDecisionRequiresHumanAtEveryLevel(t *testing.T) {
	ev := failure.Evidence{Source: "IMPLEMENT", Outcome: &agent.Outcome{
		Status: agent.OutcomeNeedsHuman,
		Reason: "required change would weaken authentication and expose local command execution to remote unauthenticated clients",
	}}
	for _, level := range []Level{Low, Balanced, High} {
		d := riskDecision(t, ev, level)
		if !d.RequiresHuman || d.Action != ActionHumanApproval {
			t.Fatalf("%s: decision = %+v, want HUMAN_APPROVAL_REQUIRED", level, d)
		}
		if d.Classification.Kind != failure.SecurityBoundary || d.Risk != RiskHigh {
			t.Errorf("%s: classification = %+v risk = %s, want SECURITY_BOUNDARY/HIGH", level, d.Classification, d.Risk)
		}
	}
}

// TestNoProgressIsNotAutoContinue proves a bounded no-progress stop is never
// continued automatically at any autonomy level: it requires a human/operator
// decision, unlike productive incomplete work.
func TestNoProgressIsNotAutoContinue(t *testing.T) {
	c := failure.Classification{Kind: failure.NoProgress, Disposition: failure.NeedsHuman, Confidence: failure.High,
		Reason: "the invocation made no repository progress within its bounded stale allowance"}
	for _, level := range []Level{Low, Balanced, High} {
		if d := Decide(c, PolicyFor(level)); d.Action == ActionAutoContinue {
			t.Fatalf("%s: a bounded no-progress stop must not auto-continue: %+v", level, d)
		}
	}
}
