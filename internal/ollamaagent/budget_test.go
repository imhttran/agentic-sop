package ollamaagent

import (
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/budget"
)

// TestPolicyForUsesTheBudget proves the canonical budget is the single source of
// the hard iteration ceilings: a smaller budget yields a smaller ceiling, and the
// soft thresholds scale with it.
func TestPolicyForUsesTheBudget(t *testing.T) {
	b := budget.Defaults()
	b.ImplementIterations = 5
	impl := policyFor(agent.Implement, b)
	if impl.MaxIterations != 5 {
		t.Errorf("IMPLEMENT ceiling = %d, want the budget's 5", impl.MaxIterations)
	}
	b.FixIterations = 4
	if got := policyFor(agent.Fix, b).MaxIterations; got != 4 {
		t.Errorf("FIX ceiling = %d, want the budget's 4", got)
	}
}

// TestStalledHonoursTheStaleLimit proves the no-progress guard stops at the
// budget's stale limit: N-1 stale turns are allowed, the Nth stops, and the
// default budget preserves the shipped bound.
func TestStalledHonoursTheStaleLimit(t *testing.T) {
	const limit = 3
	st := newExecutionState()
	for i := 1; i < limit; i++ {
		if st.stalled(false, false, limit) {
			t.Fatalf("stalled at %d, want it allowed before the limit %d", i, limit)
		}
	}
	if !st.stalled(false, false, limit) {
		t.Errorf("did not stall at the limit %d", limit)
	}
	// A discovery resets the streak (activity through the window).
	st2 := newExecutionState()
	st2.stalled(false, false, limit)
	if st2.stalled(false, true, limit) {
		t.Errorf("novel discovery must not stall")
	}
	if st2.consecutiveNoProgress != 0 {
		t.Errorf("discovery must reset the stale streak, got %d", st2.consecutiveNoProgress)
	}
}

// TestEffectiveBudget proves a Config built directly still gets the defaults,
// while an explicit MaxToolCalls is preserved.
func TestEffectiveBudget(t *testing.T) {
	if got := (Config{}).EffectiveBudget(); got != budget.Defaults() {
		t.Errorf("zero Config effective budget = %+v, want the defaults", got)
	}
	c := Config{MaxToolCalls: 200}
	if got := c.EffectiveBudget().ToolCalls; got != 200 {
		t.Errorf("effective tool-call limit = %d, want the explicit 200", got)
	}
}
