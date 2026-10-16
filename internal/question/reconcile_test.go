package question

import (
	"testing"
	"time"
)

func enabledPlanner() PlannerSettings { return PlannerSettings{Enabled: true, RoundLimit: 3} }

// TestDecideGuards pins the reconciler's deterministic decisions and the guards it
// must respect: disabled mode, human gates, cancellation, terminal tasks, and stale
// bindings take precedence over the question lifecycle.
func TestDecideGuards(t *testing.T) {
	pending := &Question{ID: "q1", Identity: "T1", State: StatePending}
	cases := []struct {
		name string
		s    PlannerSettings
		v    ReconcileView
		want Action
	}{
		{"disabled", PlannerSettings{}, ReconcileView{Question: pending}, ActionDisabled},
		{"human gate", enabledPlanner(), ReconcileView{Question: pending, HumanGate: true}, ActionSkipHumanGate},
		{"cancelled", enabledPlanner(), ReconcileView{Question: pending, Cancelled: true}, ActionSkipCancelled},
		{"terminal task", enabledPlanner(), ReconcileView{Question: pending, TaskTerminal: true}, ActionSkipTerminal},
		{"no question", enabledPlanner(), ReconcileView{}, ActionNone},
		{"stale plan", enabledPlanner(), ReconcileView{Question: &Question{ID: "q", State: StatePending, PlanRevision: "p@old"}, CurrentPlanRevision: "p@new"}, ActionSkipStale},
		{"stale run", enabledPlanner(), ReconcileView{Question: &Question{ID: "q", State: StatePending, RunEpoch: "e1"}, CurrentRunEpoch: "e2"}, ActionSkipStale},
		{"stale repo", enabledPlanner(), ReconcileView{Question: &Question{ID: "q", State: StatePending, RepoFingerprint: "/a"}, CurrentRepoFingerprint: "/b"}, ActionSkipStale},
		{"pending waits", enabledPlanner(), ReconcileView{Question: pending}, ActionWait},
		{"rounds exhausted", PlannerSettings{Enabled: true, RoundLimit: 1}, ReconcileView{Question: pending, Rounds: 2}, ActionRoundsExhausted},
		{"answered applies", enabledPlanner(), ReconcileView{Question: &Question{ID: "q", State: StateAnswered}}, ActionApplyThenContinue},
		{"applied continues", enabledPlanner(), ReconcileView{Question: &Question{ID: "q", State: StateApplied}}, ActionContinue},
		{"applied completed is nothing", enabledPlanner(), ReconcileView{Question: &Question{ID: "q", State: StateApplied}, Continuation: &Continuation{State: ContinuationCompleted}}, ActionNone},
		{"rejected none", enabledPlanner(), ReconcileView{Question: &Question{ID: "q", State: StateRejected}}, ActionNone},
	}
	for _, tc := range cases {
		got, _ := Decide(tc.s, tc.v)
		if got != tc.want {
			t.Errorf("%s: action = %q, want %q", tc.name, got, tc.want)
		}
	}
}

// TestDecidePauseOnTimeout proves a timed-out pending question stays paused (WAIT)
// rather than failing or auto-retrying.
func TestDecidePauseOnTimeout(t *testing.T) {
	now := time.Now()
	q := &Question{ID: "q", State: StatePending, Deadline: now.Add(-time.Minute)}
	a, reason := Decide(PlannerSettings{Enabled: true, RoundLimit: 3, PauseOnTimeout: true}, ReconcileView{Question: q, Now: now})
	if a != ActionWait {
		t.Fatalf("action = %q, want WAIT (paused)", a)
	}
	if reason == "" {
		t.Fatal("want a reason")
	}
}
