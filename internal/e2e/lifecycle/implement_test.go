package lifecycle

import (
	"context"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/e2e/harness"
)

// implementViaHarness runs a single IMPLEMENT capability through a fake harness
// and returns the structured outcome, mirroring how SOP consumes it.
func implementViaHarness(t *testing.T, outcome *agent.Outcome) agent.Response {
	t.Helper()
	h := agent.HarnessFunc(func(_ context.Context, req agent.Request) (agent.Response, error) {
		if req.Capability != agent.Implement {
			t.Fatalf("capability = %s, want IMPLEMENT", req.Capability)
		}
		return harness.Response("done", outcome), nil
	})
	resp, err := h.Execute(context.Background(), agent.Request{Capability: agent.Implement, Task: "edit"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	return resp
}

// TestImplementEarlyMutation asserts IMPLEMENT reports an intended repository
// change (mutation) when the stage calls for it. S2.
func TestImplementEarlyMutation(t *testing.T) {
	resp := implementViaHarness(t, harness.Completed("wrote files", true))
	if resp.Outcome == nil {
		t.Fatal("missing structured outcome")
	}
	if resp.Outcome.Status != agent.OutcomeCompleted {
		t.Fatalf("status = %s, want completed", resp.Outcome.Status)
	}
	if !resp.Outcome.ChangesExpected {
		t.Fatal("expected changes_expected = true for a mutating stage")
	}
}

// TestImplementNoMutationTransition asserts the transition to a no-mutation
// outcome when the stage makes no change. S2.
func TestImplementNoMutationTransition(t *testing.T) {
	resp := implementViaHarness(t, harness.Completed("no change needed", false))
	if resp.Outcome == nil || resp.Outcome.Status != agent.OutcomeCompleted {
		t.Fatalf("outcome = %+v, want completed", resp.Outcome)
	}
	if resp.Outcome.ChangesExpected {
		t.Fatal("expected changes_expected = false for a no-mutation stage")
	}
}

// TestImplementForcedFinalization asserts forced finalization produces the
// required finalization outcome (needs_human / failed) rather than silence. S2.
func TestImplementForcedFinalization(t *testing.T) {
	cases := []struct {
		name    string
		outcome *agent.Outcome
		want    agent.OutcomeStatus
	}{
		{"needs human", harness.NeedsHuman("blocked"), agent.OutcomeNeedsHuman},
		{"failed", harness.Failed("boom"), agent.OutcomeFailed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := implementViaHarness(t, tc.outcome)
			if resp.Outcome == nil {
				t.Fatal("missing structured outcome")
			}
			if resp.Outcome.Status != tc.want {
				t.Fatalf("status = %s, want %s", resp.Outcome.Status, tc.want)
			}
			if resp.Outcome.ChangesExpected {
				t.Fatal("non-completed outcome must not claim changes_expected")
			}
		})
	}
}

// TestImplementOutcomeParsedFromContent asserts a structured outcome embedded in
// JSON content is parsed by the command protocol (backward-compatible). S2.
func TestImplementOutcomeParsedFromContent(t *testing.T) {
	content := `{"status":"completed","summary":"did it","changes_expected":true}`
	outcome := agent.ParseOutcome(content)
	if outcome == nil {
		t.Fatal("expected parsed outcome")
	}
	if outcome.Status != agent.OutcomeCompleted || !outcome.ChangesExpected {
		t.Fatalf("outcome = %+v, want completed with changes", outcome)
	}
}

// TestImplementProseOutcomeIgnored asserts legacy prose content yields no
// structured outcome (never inferred from prose). S2.
func TestImplementProseOutcomeIgnored(t *testing.T) {
	if outcome := agent.ParseOutcome("I changed some files, all good"); outcome != nil {
		t.Fatalf("prose must not yield an outcome, got %+v", outcome)
	}
}
