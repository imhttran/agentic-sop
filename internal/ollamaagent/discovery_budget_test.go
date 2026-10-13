package ollamaagent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/budget"
)

// DISCOVERY-BUDGET — deterministic regression for the task-scoped discovery
// allowance.
//
// These cases run the scripted fake Ollama server only: no real model, provider,
// network, or credential is involved, and every fixture lives in a per-test
// t.TempDir(), so repeated runs are deterministic. The allowance is configured
// through the canonical budget (the same path an operator's
// SOP_OLLAMA_DISCOVERY_ITERATIONS override takes), so the tests exercise the real
// production execution path rather than a test-only knob.

// TestPolicyDiscoveryWindowIsBoundedAndBudgetScoped proves the discovery window the
// capability policy advertises is the built-in default when the budget is
// unconfigured, follows the operator's task-scoped allowance, and is always strictly
// below the capability's hard iteration ceiling, so the allowance can never consume
// the whole budget or disable the safety ceiling.
func TestPolicyDiscoveryWindowIsBoundedAndBudgetScoped(t *testing.T) {
	if got := PolicyFor(agent.Implement).DiscoveryWindow; got != implementNowAfter {
		t.Errorf("default IMPLEMENT discovery window = %d, want %d", got, implementNowAfter)
	}
	if got := PolicyFor(agent.Fix).DiscoveryWindow; got != implementNowAfter {
		t.Errorf("default FIX discovery window = %d, want %d", got, implementNowAfter)
	}

	widened := budget.Defaults()
	widened.DiscoveryIterations = 20
	if got := policyFor(agent.Implement, widened).DiscoveryWindow; got != 20 {
		t.Errorf("configured IMPLEMENT discovery window = %d, want 20", got)
	}
	if got := policyFor(agent.Fix, widened).DiscoveryWindow; got != 20 {
		t.Errorf("configured FIX discovery window = %d, want 20", got)
	}

	// An allowance at the budget maximum is still capped below the ceiling.
	maxed := budget.Defaults()
	maxed.DiscoveryIterations = budget.MaxDiscoveryIterations
	impl := policyFor(agent.Implement, maxed)
	if impl.DiscoveryWindow <= implementNowAfter {
		t.Errorf("max allowance did not widen the IMPLEMENT window: %d", impl.DiscoveryWindow)
	}
	if impl.DiscoveryWindow >= impl.MaxIterations {
		t.Errorf("IMPLEMENT discovery window %d must stay below the ceiling %d", impl.DiscoveryWindow, impl.MaxIterations)
	}
	fix := policyFor(agent.Fix, maxed)
	if fix.DiscoveryWindow >= fix.MaxIterations {
		t.Errorf("FIX discovery window %d must stay below the ceiling %d", fix.DiscoveryWindow, fix.MaxIterations)
	}
}

// TestDiscoveryTurnsResolution pins the resolution and clamping of the task-scoped
// allowance independent of the ceiling: unset falls back to the built-in default, a
// positive value is used verbatim while it fits, and any value at or above the
// ceiling is capped to ceiling-1 (never zero).
func TestDiscoveryTurnsResolution(t *testing.T) {
	cases := []struct {
		name      string
		discovery int
		ceiling   int
		want      int
	}{
		{"unset keeps the default", 0, maxIterationsImplement, implementNowAfter},
		{"explicit below the ceiling", 20, maxIterationsImplement, 20},
		{"minimum kept", budget.MinDiscoveryIterations, maxIterationsImplement, budget.MinDiscoveryIterations},
		{"at the ceiling capped below", maxIterationsImplement, maxIterationsImplement, maxIterationsImplement - 1},
		{"above the max still capped below", budget.MaxDiscoveryIterations, maxIterationsImplement, maxIterationsImplement - 1},
		{"fix ceiling caps lower", budget.MaxDiscoveryIterations, maxIterationsFix, maxIterationsFix - 1},
		{"default capped below a small ceiling", 0, 10, 9},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := budget.Defaults()
			b.DiscoveryIterations = tc.discovery
			if got := discoveryTurns(b, tc.ceiling); got != tc.want {
				t.Errorf("discoveryTurns(discovery=%d, ceiling=%d) = %d, want %d", tc.discovery, tc.ceiling, got, tc.want)
			}
		})
	}
}

// TestDiscoveryLimitDefaultsWhenUnset proves every discovery/stale decision reads
// one effective window, and that a state built without a configured allowance (as a
// direct unit test does) falls back to the built-in default, so behavior is
// unchanged when no allowance is configured.
func TestDiscoveryLimitDefaultsWhenUnset(t *testing.T) {
	if got := newExecutionState().discoveryLimit(); got != implementNowAfter {
		t.Errorf("fresh state discovery limit = %d, want %d", got, implementNowAfter)
	}
	st := newExecutionState()
	st.discoveryWindow = 20
	if got := st.discoveryLimit(); got != 20 {
		t.Errorf("configured discovery limit = %d, want 20", got)
	}
	if got := (&executionState{}).discoveryLimit(); got != implementNowAfter {
		t.Errorf("zero-window discovery limit = %d, want the default %d", got, implementNowAfter)
	}
}

// TestWidenedDiscoveryAllowanceExtendsExplorationBeforeNoProgress proves the central
// requirement: with a widened task-scoped allowance, a discovery-heavy run that only
// reads (novel, successful, non-mutating) keeps making credited progress for longer
// before the no-progress guard fires, and still terminates through the existing
// fail-closed mechanism at the configured allowance plus the stale bound — strictly
// below the iteration ceiling. It also proves the configured allowance is recorded
// in the no-progress diagnostic.
func TestWidenedDiscoveryAllowanceExtendsExplorationBeforeNoProgress(t *testing.T) {
	const window = 20 // wider than the default implementNowAfter (12), below the ceiling
	dir := t.TempDir()
	seedToolFiles(t, dir, "pkg", window+maxNoProgressIterations+4)

	responses := make([]string, 0, window+maxNoProgressIterations+2)
	for i := 0; i < window+maxNoProgressIterations+2; i++ {
		responses = append(responses, readToolCall(i))
	}
	fake, srv := newFakeOllama(t, responses...)

	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 200
	cfg.Budget = budget.Defaults()
	cfg.Budget.DiscoveryIterations = window

	h := New(cfg, dir)
	_, err := h.Execute(context.Background(), implementRequest())

	var stalled *noProgressError
	if !errors.As(err, &stalled) {
		t.Fatalf("err = %v, want *noProgressError", err)
	}

	// The stop point follows the CONFIGURED allowance, not the fixed default: the run
	// is credited for discovery through the widened window and only then accumulates
	// the stale allowance.
	if got, want := fake.count(), window+maxNoProgressIterations; got != want {
		t.Errorf("model turns = %d, want %d (configured window + staleIterations)", got, want)
	}
	if got := fake.count(); got <= implementNowAfter+maxNoProgressIterations {
		t.Errorf("model turns = %d, want more than the default-window stop %d", got, implementNowAfter+maxNoProgressIterations)
	}
	if got := fake.count(); got >= maxIterationsImplement {
		t.Errorf("model turns = %d, want strictly below the %d-iteration ceiling", got, maxIterationsImplement)
	}

	// The configured allowance is recorded in the execution diagnostic, and the run
	// still ends through the existing fail-closed no-progress mechanism.
	msg := err.Error()
	for _, want := range []string{"IMPLEMENT_NO_PROGRESS", "termination=no_progress", "a retry may succeed"} {
		if !strings.Contains(msg, want) {
			t.Errorf("err = %q, want it to contain %q", msg, want)
		}
	}
	if want := fmt.Sprintf("discovery_allowance=%d", window); !strings.Contains(msg, want) {
		t.Errorf("err = %q, want it to record %s", msg, want)
	}
}

// TestNarrowDiscoveryAllowanceStillTerminatesBounded proves the other bound: a
// narrower allowance ends a no-progress run sooner, still bounded by the configured
// allowance plus the stale limit and never exceeding the iteration ceiling. It is
// the direct counterpart to the widened case, so the allowance is shown to move the
// stop point in both directions without ever unbounding it.
func TestNarrowDiscoveryAllowanceStillTerminatesBounded(t *testing.T) {
	const window = 3 // narrower than the default implementNowAfter (12)
	dir := t.TempDir()
	seedToolFiles(t, dir, "pkg", implementNowAfter+maxNoProgressIterations+4)

	responses := make([]string, 0, implementNowAfter+maxNoProgressIterations+2)
	for i := 0; i < implementNowAfter+maxNoProgressIterations+2; i++ {
		responses = append(responses, readToolCall(i))
	}
	fake, srv := newFakeOllama(t, responses...)

	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 200
	cfg.Budget = budget.Defaults()
	cfg.Budget.DiscoveryIterations = window

	h := New(cfg, dir)
	_, err := h.Execute(context.Background(), implementRequest())

	var stalled *noProgressError
	if !errors.As(err, &stalled) {
		t.Fatalf("err = %v, want *noProgressError", err)
	}
	if got, want := fake.count(), window+maxNoProgressIterations; got != want {
		t.Errorf("model turns = %d, want %d (narrow window + staleIterations)", got, want)
	}
	if got := fake.count(); got >= maxIterationsImplement {
		t.Errorf("model turns = %d, want strictly below the %d-iteration ceiling", got, maxIterationsImplement)
	}
}

// TestDiscoveryAllowanceIsProviderAgnostic proves the allowance is a deterministic
// execution limit carried on the policy, not a model- or provider-derived value: the
// same budget yields the same window regardless of the configured model, so no
// per-model hardcoding is involved.
func TestDiscoveryAllowanceIsProviderAgnostic(t *testing.T) {
	b := budget.Defaults()
	b.DiscoveryIterations = 20
	want := policyFor(agent.Implement, b).DiscoveryWindow
	for _, model := range []string{"deepseek-v4.1-flash:cloud", "codex", "nemotron", "some-other-model"} {
		cfg := testConfig("http://127.0.0.1:0")
		cfg.Model = model
		cfg.Budget = b
		if got := policyFor(agent.Implement, cfg.EffectiveBudget()).DiscoveryWindow; got != want {
			t.Errorf("model %q discovery window = %d, want %d (the allowance must not depend on the model)", model, got, want)
		}
	}
}
