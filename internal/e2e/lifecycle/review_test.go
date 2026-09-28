package lifecycle

import (
	"context"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/e2e/harness"
)

// reviewViaHarness runs a REVIEW capability through a fake harness that returns
// a verdict-shaped outcome.
func reviewViaHarness(t *testing.T, verdict agent.OutcomeStatus) agent.Response {
	t.Helper()
	h := agent.HarnessFunc(func(_ context.Context, req agent.Request) (agent.Response, error) {
		if req.Capability != agent.Review {
			t.Fatalf("capability = %s, want REVIEW", req.Capability)
		}
		return harness.Response("reviewed", &agent.Outcome{Status: verdict, Summary: "reviewed"}), nil
	})
	resp, err := h.Execute(context.Background(), agent.Request{Capability: agent.Review, Task: "review"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	return resp
}

// TestReviewEarlyCompletion asserts REVIEW returns early when completion
// criteria are satisfied, after a single agent invocation. S4.
func TestReviewEarlyCompletion(t *testing.T) {
	provider := harness.NewFakeProvider(harness.Response("looks good", harness.Completed("pass", false)))
	resp, err := provider.Generate(context.Background(), agent.Request{Capability: agent.Review, Task: "review"})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if resp.Outcome == nil || resp.Outcome.Status != agent.OutcomeCompleted {
		t.Fatalf("outcome = %+v, want completed", resp.Outcome)
	}
	if provider.Calls() != 1 {
		t.Fatalf("review agent invocations = %d, want 1 (early completion)", provider.Calls())
	}
}

// TestReviewForcedSynthesis asserts forced synthesis yields a valid structured
// REVIEW response when exploration is cut short. S4.
func TestReviewForcedSynthesis(t *testing.T) {
	resp := reviewViaHarness(t, agent.OutcomeCompleted)
	if resp.Outcome == nil {
		t.Fatal("forced synthesis produced no structured REVIEW outcome")
	}
	if resp.Outcome.Status != agent.OutcomeCompleted {
		t.Fatalf("status = %s, want completed", resp.Outcome.Status)
	}
	if resp.Outcome.Summary == "" {
		t.Fatal("forced synthesis REVIEW outcome missing summary")
	}
}

// TestReviewNonCompletedSynthesis asserts a needs_human REVIEW verdict is
// surfaced structurally, not as prose. S4.
func TestReviewNonCompletedSynthesis(t *testing.T) {
	resp := reviewViaHarness(t, agent.OutcomeNeedsHuman)
	if resp.Outcome == nil || resp.Outcome.Status != agent.OutcomeNeedsHuman {
		t.Fatalf("outcome = %+v, want needs_human", resp.Outcome)
	}
}

// TestReviewCapabilityGuard asserts a provider that does not declare REVIEW is
// rejected rather than invoked. S4 / focused failure.
func TestReviewCapabilityGuard(t *testing.T) {
	provider := harness.NewFakeProvider().WithCapabilities(agent.NewCapabilities(agent.Plan))
	checked := agent.NewChecked(provider)
	_, err := checked.Generate(context.Background(), agent.Request{Capability: agent.Review, Task: "review"})
	if err == nil {
		t.Fatal("expected capability rejection for REVIEW")
	}
	if provider.Calls() != 0 {
		t.Fatalf("unsupported provider was invoked %d times", provider.Calls())
	}
}
