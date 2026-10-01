package recovery

import (
	"testing"

	"github.com/imhttran/agentic-sop/internal/failure"
	"github.com/imhttran/agentic-sop/internal/model"
)

// on is the policy used by most cases: escalation enabled with the default
// escalation budget.
func on(max int) Policy { return Policy{Enabled: true, MaxEscalations: max} }

func TestDecideLadder(t *testing.T) {
	tests := []struct {
		name       string
		policy     Policy
		ev         Evidence
		wantAction Action
		wantTo     model.Class
		wantReason string
	}{
		{
			name:       "escalation disabled is a no-op",
			policy:     Policy{Enabled: false, MaxEscalations: 2},
			ev:         Evidence{Class: model.ClassSmall, Kind: failure.CompilerError, Disposition: failure.AutoFix, Attempt: 1},
			wantAction: ActionNone,
			wantReason: ReasonDisabled,
		},
		{
			name:       "small implementation failure escalates to medium",
			policy:     on(2),
			ev:         Evidence{Class: model.ClassSmall, Kind: failure.TestFailure, Disposition: failure.AutoFix, Stage: "test", Attempt: 1},
			wantAction: ActionEscalate,
			wantTo:     model.ClassMedium,
			wantReason: ReasonImplementationFailed,
		},
		{
			name:       "medium implementation failure escalates to large",
			policy:     on(2),
			ev:         Evidence{Class: model.ClassMedium, Kind: failure.BlockingFindings, Disposition: failure.AutoFix, Stage: "review", Attempt: 1},
			wantAction: ActionEscalate,
			wantTo:     model.ClassLarge,
			wantReason: ReasonImplementationFailed,
		},
		{
			name:       "exhausted fix budget still escalates (a stronger model may repair it)",
			policy:     on(2),
			ev:         Evidence{Class: model.ClassMedium, Kind: failure.AutoFixExhausted, Disposition: failure.NeedsHuman, Stage: "test", Attempt: 1},
			wantAction: ActionEscalate,
			wantTo:     model.ClassLarge,
			wantReason: ReasonImplementationFailed,
		},
		{
			name:       "large cannot escalate further",
			policy:     on(2),
			ev:         Evidence{Class: model.ClassLarge, Kind: failure.TestFailure, Disposition: failure.AutoFix, Stage: "test", Attempt: 3},
			wantAction: ActionHuman,
			wantReason: ReasonNoLargerClass,
		},
		{
			name:       "escalation budget spent yields a human boundary",
			policy:     on(0),
			ev:         Evidence{Class: model.ClassSmall, Kind: failure.CompilerError, Disposition: failure.AutoFix, Attempt: 2},
			wantAction: ActionHuman,
			wantReason: ReasonEscalationLimit,
		},
		{
			name:       "second escalation allowed with budget two",
			policy:     on(2),
			ev:         Evidence{Class: model.ClassMedium, Escalations: 1, Kind: failure.TestFailure, Disposition: failure.AutoFix, Attempt: 2},
			wantAction: ActionEscalate,
			wantTo:     model.ClassLarge,
			wantReason: ReasonImplementationFailed,
		},
		{
			name:       "escalation budget exhausted at the limit",
			policy:     on(1),
			ev:         Evidence{Class: model.ClassMedium, Escalations: 1, Kind: failure.TestFailure, Disposition: failure.AutoFix, Attempt: 2},
			wantAction: ActionHuman,
			wantReason: ReasonEscalationLimit,
		},
		{
			name:       "transient provider failure retries the same class",
			policy:     on(2),
			ev:         Evidence{Class: model.ClassSmall, Kind: failure.TransientProvider, Disposition: failure.Retry, Stage: "provider", Attempt: 1},
			wantAction: ActionRetrySame,
			wantReason: ReasonTransient,
		},
		{
			name:       "tool failure retries the same class",
			policy:     on(2),
			ev:         Evidence{Class: model.ClassMedium, Kind: failure.ToolFailure, Disposition: failure.Retry, Attempt: 1},
			wantAction: ActionRetrySame,
			wantReason: ReasonTransient,
		},
		{
			name:       "unfinished implementation continues on the same class",
			policy:     on(2),
			ev:         Evidence{Class: model.ClassSmall, Kind: failure.IncompleteImplementation, Disposition: failure.Continue, Attempt: 1},
			wantAction: ActionRetrySame,
			wantReason: ReasonContinuation,
		},
		{
			name:       "security boundary is never escalated",
			policy:     on(2),
			ev:         Evidence{Class: model.ClassSmall, Kind: failure.SecurityBoundary, Disposition: failure.NeedsHuman, Attempt: 1},
			wantAction: ActionHuman,
			wantReason: ReasonSafety,
		},
		{
			name:       "approval required is never escalated",
			policy:     on(2),
			ev:         Evidence{Class: model.ClassSmall, Kind: failure.ApprovalRequired, Disposition: failure.NeedsHuman, Attempt: 1},
			wantAction: ActionHuman,
			wantReason: ReasonSafety,
		},
		{
			name:       "high risk beats the small-class optimization",
			policy:     on(2),
			ev:         Evidence{Class: model.ClassSmall, Kind: failure.DestructiveOperation, Disposition: failure.NeedsHuman, Attempt: 1},
			wantAction: ActionHuman,
			wantReason: ReasonSafety,
		},
		{
			name:       "invalid plan is a human decision, not an escalation",
			policy:     on(2),
			ev:         Evidence{Class: model.ClassMedium, Kind: failure.ReplanRequired, Disposition: failure.Replan, Attempt: 1},
			wantAction: ActionHuman,
			wantReason: ReasonPlanInvalid,
		},
		{
			name:       "unclassified failure fails closed",
			policy:     on(2),
			ev:         Evidence{Class: model.ClassSmall, Attempt: 1},
			wantAction: ActionHuman,
			wantReason: ReasonUnclassified,
		},
		{
			name:       "a plainly unknown kind is a fail-closed boundary, not a safety one",
			policy:     on(2),
			ev:         Evidence{Class: model.ClassSmall, Kind: failure.Unknown, Disposition: failure.NeedsHuman, Attempt: 1},
			wantAction: ActionHuman,
			wantReason: ReasonUnclassified,
		},
		{
			name:       "unknown class cannot escalate",
			policy:     on(2),
			ev:         Evidence{Class: "", Kind: failure.TestFailure, Disposition: failure.AutoFix, Attempt: 1},
			wantAction: ActionHuman,
			wantReason: ReasonNoLargerClass,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Decide(tc.policy, tc.ev)
			if got.Action != tc.wantAction {
				t.Fatalf("Action = %q, want %q (decision: %+v)", got.Action, tc.wantAction, got)
			}
			if got.ToClass != tc.wantTo {
				t.Errorf("ToClass = %q, want %q", got.ToClass, tc.wantTo)
			}
			if got.Reason != tc.wantReason {
				t.Errorf("Reason = %q, want %q", got.Reason, tc.wantReason)
			}
			if got.FromClass != tc.ev.Class {
				t.Errorf("FromClass = %q, want %q", got.FromClass, tc.ev.Class)
			}
			if got.Attempt != tc.ev.Attempt {
				t.Errorf("Attempt = %d, want %d", got.Attempt, tc.ev.Attempt)
			}
		})
	}
}

// TestDecideIsDeterministic proves the policy is a pure function: the same
// evidence always yields the same decision.
func TestDecideIsDeterministic(t *testing.T) {
	ev := Evidence{Class: model.ClassSmall, Kind: failure.TestFailure, Disposition: failure.AutoFix, Stage: "test", Attempt: 1}
	first := Decide(on(2), ev)
	for i := 0; i < 50; i++ {
		if got := Decide(on(2), ev); got != first {
			t.Fatalf("decision %d = %+v, want %+v", i, got, first)
		}
	}
}

// TestLadderNeverWraps proves the ladder is one-way: no next class exists for
// large, and no class ever escalates downward.
func TestLadderNeverWraps(t *testing.T) {
	if next, ok := NextClass(model.ClassLarge); ok {
		t.Fatalf("large escalated to %q; the ladder must not wrap", next)
	}
	for _, from := range []model.Class{model.ClassSmall, model.ClassMedium} {
		next, ok := NextClass(from)
		if !ok {
			t.Fatalf("no next class for %q", from)
		}
		rank := map[model.Class]int{model.ClassSmall: 0, model.ClassMedium: 1, model.ClassLarge: 2}
		if rank[next] <= rank[from] {
			t.Fatalf("escalation %q -> %q is not upward", from, next)
		}
	}
}

// TestActionsAreClosed proves every action is a member of the closed vocabulary.
func TestActionsAreClosed(t *testing.T) {
	for _, a := range []Action{ActionNone, ActionRetrySame, ActionEscalate, ActionHuman, ActionBlock} {
		if !a.Valid() {
			t.Errorf("action %q is not valid", a)
		}
	}
	if Action("arbitrary").Valid() {
		t.Error("an arbitrary action must not be valid")
	}
}
