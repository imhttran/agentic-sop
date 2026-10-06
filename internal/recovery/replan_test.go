package recovery

import (
	"testing"

	"github.com/imhttran/agentic-sop/internal/failure"
	"github.com/imhttran/agentic-sop/internal/model"
)

func replanPolicy() Policy { return Policy{Replan: true, MaxReplans: 1} }

// TestReplanEligibleForRecoverableFailure proves a recoverable implementation
// failure is replannable, on the SAME class (a replan is not escalation).
func TestReplanEligibleForRecoverableFailure(t *testing.T) {
	ev := Evidence{Class: model.ClassMedium, Kind: failure.CompilerError, Disposition: failure.AutoFix, Attempt: 1}
	d := Decide(replanPolicy(), ev)
	if d.Action != ActionReplan {
		t.Fatalf("action = %s, want replan", d.Action)
	}
	if d.ToClass != model.ClassMedium {
		t.Errorf("ToClass = %q, want the unchanged class %q", d.ToClass, model.ClassMedium)
	}
}

// TestReplanBoundN1N proves the N-1/N/N+1 bound for MaxReplans = 1: 0 replans is
// permitted (N-1), the single replan is permitted (N), and a second is denied
// (N+1) — with escalation off, the caller keeps its existing behavior (none).
func TestReplanBoundN1N(t *testing.T) {
	p := replanPolicy()
	if d := Decide(p, Evidence{Class: model.ClassMedium, Kind: failure.CompilerError, Disposition: failure.AutoFix, Replans: 0, Attempt: 1}); d.Action != ActionReplan {
		t.Fatalf("replans=0 action = %s, want replan", d.Action)
	}
	if d := Decide(p, Evidence{Class: model.ClassMedium, Kind: failure.CompilerError, Disposition: failure.AutoFix, Replans: 1, Attempt: 2}); d.Action != ActionNone {
		t.Fatalf("replans=1 (N+1 attempt) action = %s, want none (no second replan)", d.Action)
	}
}

// TestReplanNeverForBlockedOrHuman proves a non-eligible failure never replans:
// BLOCK/NO_PROGRESS, a plan-invalid disposition, a security boundary, and an
// approval boundary.
func TestReplanNeverForBlockedOrHuman(t *testing.T) {
	p := replanPolicy()
	cases := []struct {
		name string
		ev   Evidence
	}{
		{"no_progress", Evidence{Class: model.ClassMedium, Kind: failure.NoProgress, Disposition: failure.Block}},
		{"plan_invalid", Evidence{Class: model.ClassMedium, Kind: failure.ReplanRequired, Disposition: failure.Replan}},
		{"security", Evidence{Class: model.ClassMedium, Kind: failure.SecurityBoundary, Disposition: failure.NeedsHuman}},
		{"approval", Evidence{Class: model.ClassMedium, Kind: failure.ApprovalRequired, Disposition: failure.NeedsHuman}},
	}
	for _, tc := range cases {
		if d := Decide(p, tc.ev); d.Action == ActionReplan {
			t.Errorf("%s must never replan, got %s", tc.name, d.Action)
		}
	}
}

// TestReplanIsOffByDefault proves an unconfigured policy never replans.
func TestReplanIsOffByDefault(t *testing.T) {
	if d := Decide(Policy{}, Evidence{Class: model.ClassMedium, Kind: failure.CompilerError, Disposition: failure.AutoFix}); d.Action != ActionNone {
		t.Errorf("default policy action = %s, want none", d.Action)
	}
}

// TestReplanIsAValidAction proves the new action is part of the closed Action set,
// so a persisted or rendered decision can never carry an unknown action.
func TestReplanIsAValidAction(t *testing.T) {
	if !ActionReplan.Valid() {
		t.Fatalf("ActionReplan %q must be a valid action", ActionReplan)
	}
}

// TestReplanThenEscalate proves the two recoveries layer: with replanning and
// escalation both enabled, a recoverable failure replans once on the SAME class,
// and the next failure escalates to the next class (the replan budget is spent).
func TestReplanThenEscalate(t *testing.T) {
	p := Policy{Enabled: true, MaxEscalations: 2, Replan: true, MaxReplans: 1}

	first := Decide(p, Evidence{Class: model.ClassSmall, Kind: failure.CompilerError, Disposition: failure.AutoFix, Replans: 0, Attempt: 1})
	if first.Action != ActionReplan || first.ToClass != model.ClassSmall {
		t.Fatalf("first = %+v, want a replan on the same class", first)
	}

	// The replan attempt failed: the replan budget is spent, so the SAME failure now
	// escalates one class.
	second := Decide(p, Evidence{Class: model.ClassSmall, Kind: failure.CompilerError, Disposition: failure.AutoFix, Replans: 1, Attempt: 2})
	if second.Action != ActionEscalate || second.ToClass != model.ClassMedium {
		t.Fatalf("second = %+v, want an escalation to medium", second)
	}
}

// TestReplanEligibleForFixBudgetExhaustion documents the deliberate scope of
// replanning: the bounded fix-cycle allowance is a QUALITY signal the existing
// policy already treats as recoverable (escalatable), so a strategy change is also
// permitted. This is not the hard AGENT-004 execution budget: iteration/tool-call
// exhaustion classifies as a continuation and stale exhaustion as a BLOCK, so
// neither is replannable here (the tests above prove BLOCK never replans).
func TestReplanEligibleForFixBudgetExhaustion(t *testing.T) {
	ev := Evidence{Class: model.ClassMedium, Kind: failure.AutoFixExhausted, Disposition: failure.NeedsHuman, Replans: 0}
	if d := Decide(replanPolicy(), ev); d.Action != ActionReplan {
		t.Fatalf("action = %s, want replan for the bounded fix allowance", d.Action)
	}
}

// TestReplanNeverForContinuation proves the hard-budget / unfinished signals (a
// continuation) keep their existing retry behavior and never replan.
func TestReplanNeverForContinuation(t *testing.T) {
	p := Policy{Replan: true, MaxReplans: 2}
	for _, ev := range []Evidence{
		{Class: model.ClassMedium, Kind: failure.IncompleteImplementation, Disposition: failure.Continue},
		{Class: model.ClassMedium, Disposition: failure.Continue},
	} {
		if d := Decide(p, ev); d.Action == ActionReplan {
			t.Errorf("a continuation must never replan, got %s", d.Action)
		}
	}
}
