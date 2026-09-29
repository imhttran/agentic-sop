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
	return Decide(failure.Classify(ev), Context{}, PolicyFor(level))
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
		name     string
		ev       failure.Evidence
		category RiskCategory
	}{
		{"conflicting authoritative requirements", failure.Evidence{Source: "PLAN", RequirementsConflict: true}, CategoryAmbiguousRequirements},
		{"destructive operation", failure.Evidence{Source: "IMPLEMENT", DestructiveOperation: true}, CategoryDestructive},
		{"security boundary", failure.Evidence{Source: "IMPLEMENT", SecurityBoundary: true}, CategorySecuritySensitive},
		{"explicit approval gate", failure.Evidence{Source: "IMPLEMENT", ApprovalRequired: true}, CategoryAmbiguousRequirements},
		{"invalid plan assumptions", failure.Evidence{Source: "PLAN", PlanInvalid: true}, CategoryAmbiguousRequirements},
	}
	for _, tc := range cases {
		for _, level := range []Level{Low, Balanced, High} {
			t.Run(tc.name+"/"+string(level), func(t *testing.T) {
				d := riskDecision(t, tc.ev, level)
				if !d.RequiresHuman || d.Action != ActionHumanApproval {
					t.Fatalf("action = %s requiresHuman = %v, want human at %s", d.Action, d.RequiresHuman, level)
				}
				if d.Category != tc.category {
					t.Errorf("category = %s, want %s", d.Category, tc.category)
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

// TestExternalPublishRequiresHuman proves risk the classifier cannot see is
// honored from the decision context.
func TestExternalPublishRequiresHuman(t *testing.T) {
	c := failure.Classification{Kind: failure.Unknown, Disposition: failure.NeedsHuman, Confidence: failure.Low}
	d := Decide(c, Context{ExternalPublish: true}, PolicyFor(High))
	if !d.RequiresHuman || d.Category != CategoryExternalPublish {
		t.Fatalf("decision = %+v, want a human external-publish boundary", d)
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
	pc := PlanChange{Kind: PlanChangeExecutedEquivalent, Tasks: []string{"S001"}}
	if d := DecidePlanChange(pc, PolicyFor(Low)); !d.RequiresHuman {
		t.Errorf("low: equivalent executed change must require approval, got %s", d.Action)
	}
	if d := DecidePlanChange(pc, PolicyFor(Balanced)); !d.RequiresHuman {
		t.Errorf("balanced: equivalent executed change must require approval, got %s", d.Action)
	}
	if d := DecidePlanChange(pc, PolicyFor(High)); d.RequiresHuman || d.Action != ActionAutoReconcile {
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
	if d := Decide(c, Context{}, p); !d.RequiresHuman {
		t.Errorf("an explicit auto_fix: false must require a human, got %s", d.Action)
	}
}

// TestPlanChangeDecisions pins every reconciliation classification.
func TestPlanChangeDecisions(t *testing.T) {
	high := PolicyFor(High)
	cases := []struct {
		kind        PlanChangeKind
		action      Action
		requires    bool
		wantRiskMin ApprovalRisk
	}{
		{PlanChangeCosmetic, ActionAutoReconcile, false, RiskNone},
		{PlanChangeUnexecuted, ActionAutoReconcile, false, RiskLow},
		{PlanChangeExecutedEquivalent, ActionAutoReconcile, false, RiskMedium},
		{PlanChangeExecutedMaterial, ActionHumanApproval, true, RiskHigh},
		{PlanChangeAmbiguous, ActionHumanApproval, true, RiskHigh},
		{PlanChangeDestructive, ActionHumanApproval, true, RiskIrreversible},
		{PlanChangeSecurity, ActionHumanApproval, true, RiskHigh},
	}
	for _, tc := range cases {
		t.Run(string(tc.kind), func(t *testing.T) {
			d := DecidePlanChange(PlanChange{Kind: tc.kind}, high)
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
