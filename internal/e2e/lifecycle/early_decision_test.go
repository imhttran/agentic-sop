package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/decision"
	"github.com/imhttran/agentic-sop/internal/e2e/harness"
)

// defaultThresholds is the routing policy shared by the early-decision flows
// under test. It is a plain value (no external configuration, no network).
func defaultThresholds() decision.Thresholds {
	return decision.Thresholds{RouteToStrongModel: 0.7, RequireHuman: 0.4}
}

// decisionProvider is the deterministic early-decision provider. It is pure
// rule-based, so the lifecycle below needs no OpenJEV install and no LLM.
func decisionProvider(t *testing.T) decision.Provider {
	t.Helper()
	p, err := decision.NewProvider("deterministic")
	if err != nil {
		t.Fatalf("NewProvider(deterministic): %v", err)
	}
	return p
}

// TestEarlyDecisionLifecycleFlows drives the full early-decision lifecycle
// (provider -> decision -> routing policy -> downstream target) for every
// Choice flow using the fake analyzer, and asserts the Route outcome and the
// FakeProvider request recording explicitly. P3-015 / S2.
func TestEarlyDecisionLifecycleFlows(t *testing.T) {
	t.Run("LOW routes to the small model", func(t *testing.T) {
		provider := decisionProvider(t)
		analyzer := harness.NewFakeProvider(harness.Response("low", harness.Completed("low flow", false)))
		clock := harness.NewClock(time.Unix(0, 0))
		ids := harness.NewIDs("early-decision")

		req := decision.Request{UseCase: "implement", Subject: "fix a typo"}
		d, err := provider.Decide(context.Background(), req)
		if err != nil {
			t.Fatalf("decide: %v", err)
		}
		if d.Choice != decision.Low {
			t.Fatalf("choice = %s, want LOW", d.Choice)
		}

		// The routing policy is the shared downstream target decision.
		if got := decision.Route(defaultThresholds(), d); got != decision.SmallModel {
			t.Fatalf("route = %s, want SMALL_MODEL", got)
		}

		// Drive the target (fake analyzer) directly: no network, no LLM.
		resp, err := analyzer.Generate(context.Background(), agent.Request{
			Capability: agent.Implement,
			Task:       req.Subject,
		})
		if err != nil {
			t.Fatalf("generate: %v", err)
		}
		if resp.Outcome == nil || resp.Outcome.Status != agent.OutcomeCompleted {
			t.Fatalf("outcome = %+v, want completed", resp.Outcome)
		}

		// Request recording and call counts are asserted, deterministically.
		if analyzer.Calls() != 1 {
			t.Fatalf("analyzer calls = %d, want 1", analyzer.Calls())
		}
		recorded := analyzer.Requests()
		if len(recorded) != 1 || recorded[0].Task != req.Subject {
			t.Fatalf("recorded requests = %+v, want one with task %q", recorded, req.Subject)
		}

		// Deterministic clock and ids: no wall clock, no real ids.
		if got, want := clock.Now().Unix(), int64(0); got != want {
			t.Fatalf("clock = %d, want %d", got, want)
		}
		if first, second := ids.Next(), ids.Next(); first != "early-decision-1" || second != "early-decision-2" {
			t.Fatalf("ids = %q,%q, want sequential values", first, second)
		}
	})

	t.Run("MEDIUM routes to the strong model", func(t *testing.T) {
		provider := decisionProvider(t)
		analyzer := harness.NewFakeProvider(harness.Response("medium", harness.Completed("medium flow", true)))

		d, err := provider.Decide(context.Background(), decision.Request{
			UseCase: "implement",
			Signals: map[string]float64{"criteria": 3},
		})
		if err != nil {
			t.Fatalf("decide: %v", err)
		}
		if d.Choice != decision.Medium {
			t.Fatalf("choice = %s, want MEDIUM", d.Choice)
		}
		if got := decision.Route(defaultThresholds(), d); got != decision.StrongModel {
			t.Fatalf("route = %s, want STRONG_MODEL", got)
		}

		if _, err := analyzer.Generate(context.Background(), agent.Request{Capability: agent.Implement, Task: "medium"}); err != nil {
			t.Fatalf("generate: %v", err)
		}
		if analyzer.Calls() != 1 {
			t.Fatalf("analyzer calls = %d, want 1", analyzer.Calls())
		}
	})

	t.Run("HIGH routes to a human", func(t *testing.T) {
		provider := decisionProvider(t)
		analyzer := harness.NewFakeProvider(harness.Response("high", harness.NeedsHuman("escalated")))

		d, err := provider.Decide(context.Background(), decision.Request{
			UseCase: "implement",
			Subject: "database migration",
		})
		if err != nil {
			t.Fatalf("decide: %v", err)
		}
		if d.Choice != decision.High {
			t.Fatalf("choice = %s, want HIGH", d.Choice)
		}
		if got := decision.Route(defaultThresholds(), d); got != decision.HumanTarget {
			t.Fatalf("route = %s, want HUMAN", got)
		}

		resp, err := analyzer.Generate(context.Background(), agent.Request{Capability: agent.Implement, Task: "high"})
		if err != nil {
			t.Fatalf("generate: %v", err)
		}
		if resp.Outcome == nil || resp.Outcome.Status != agent.OutcomeNeedsHuman {
			t.Fatalf("outcome = %+v, want needs_human", resp.Outcome)
		}
		if analyzer.Calls() != 1 {
			t.Fatalf("analyzer calls = %d, want 1", analyzer.Calls())
		}
	})

	t.Run("HUMAN routes to a human", func(t *testing.T) {
		// A HUMAN choice is produced by policy/upstream, not the deterministic
		// classifier, so it is constructed explicitly and routed.
		d := decision.Decision{Choice: decision.Human, Confidence: 0.5}
		if got := decision.Route(defaultThresholds(), d); got != decision.HumanTarget {
			t.Fatalf("route = %s, want HUMAN", got)
		}

		analyzer := harness.NewFakeProvider(harness.Response("human", harness.NeedsHuman("needs human")))
		resp, err := analyzer.Generate(context.Background(), agent.Request{Capability: agent.Review, Task: "human"})
		if err != nil {
			t.Fatalf("generate: %v", err)
		}
		if resp.Outcome == nil || resp.Outcome.Status != agent.OutcomeNeedsHuman {
			t.Fatalf("outcome = %+v, want needs_human", resp.Outcome)
		}
		if analyzer.Calls() != 1 {
			t.Fatalf("analyzer calls = %d, want 1", analyzer.Calls())
		}
	})
}

// TestEarlyDecisionRouteThresholds asserts the routing policy against explicit
// Thresholds combinations (RouteToStrongModel / RequireHuman). P3-015 / S2.
func TestEarlyDecisionRouteThresholds(t *testing.T) {
	thresholds := decision.Thresholds{RouteToStrongModel: 0.7, RequireHuman: 0.4}
	cases := []struct {
		name string
		d    decision.Decision
		want decision.Target
	}{
		{"high choice needs a human", decision.Decision{Choice: decision.High, Confidence: 0.9}, decision.HumanTarget},
		{"human choice", decision.Decision{Choice: decision.Human, Confidence: 0.5}, decision.HumanTarget},
		{"below require_human needs a human", decision.Decision{Choice: decision.Low, Confidence: 0.3}, decision.HumanTarget},
		{"medium choice uses the strong model", decision.Decision{Choice: decision.Medium, Confidence: 0.8}, decision.StrongModel},
		{"below route_to_strong uses the strong model", decision.Decision{Choice: decision.Low, Confidence: 0.6}, decision.StrongModel},
		{"confident low uses the small model", decision.Decision{Choice: decision.Low, Confidence: 0.95}, decision.SmallModel},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := decision.Route(thresholds, tc.d); got != tc.want {
				t.Fatalf("Route = %s, want %s", got, tc.want)
			}
		})
	}
}

// TestEarlyDecisionDeterministicRepeats asserts the lifecycle is repeatable:
// the same inputs yield identical decisions and routes across repeated runs.
// P3-015 / S2.
func TestEarlyDecisionDeterministicRepeats(t *testing.T) {
	provider := decisionProvider(t)
	req := decision.Request{UseCase: "implement", Subject: "add concurrency"}

	var first decision.Decision
	for i := 0; i < 5; i++ {
		d, err := provider.Decide(context.Background(), req)
		if err != nil {
			t.Fatalf("run %d decide: %v", i, err)
		}
		if i == 0 {
			first = d
			continue
		}
		if d.Choice != first.Choice || d.Confidence != first.Confidence {
			t.Fatalf("run %d = %+v, want identical to %+v", i, d, first)
		}
		if decision.Route(defaultThresholds(), d) != decision.Route(defaultThresholds(), first) {
			t.Fatalf("run %d route differs", i)
		}
	}
}

// TestEarlyDecisionProviderErrorNotSuccess asserts a failing analyzer surfaces
// its error rather than a synthesized completed outcome. No manufactured
// success. P3-015 / S3.
func TestEarlyDecisionProviderErrorNotSuccess(t *testing.T) {
	sentinel := errors.New("analyzer unavailable")
	analyzer := harness.NewFakeProviderWithErr(sentinel)

	resp, err := analyzer.Generate(context.Background(), agent.Request{Capability: agent.Implement, Task: "boom"})
	if err == nil {
		t.Fatal("expected the provider error to propagate")
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want %v", err, sentinel)
	}
	if resp.Outcome != nil {
		t.Fatalf("failed provider produced a structured outcome %+v, want nil", resp.Outcome)
	}
	if analyzer.Calls() != 1 {
		t.Fatalf("analyzer calls = %d, want 1", analyzer.Calls())
	}
}

// TestEarlyDecisionUnsupportedProvider asserts an unsupported decision provider
// name fails clearly instead of silently falling back to the deterministic
// provider. P3-015 / S3.
func TestEarlyDecisionUnsupportedProvider(t *testing.T) {
	for _, name := range []string{"jev", "llm", "openai"} {
		p, err := decision.NewProvider(name)
		if err == nil {
			t.Fatalf("NewProvider(%q) succeeded with %v, want an error", name, p)
		}
		if p != nil {
			t.Fatalf("NewProvider(%q) returned a provider %v despite error", name, p)
		}
	}
}

// TestEarlyDecisionAdapterNotSilentlySuccessful asserts malformed or failing
// adapter output is never treated as success. P3-015 / S3.
func TestEarlyDecisionAdapterNotSilentlySuccessful(t *testing.T) {
	t.Run("malformed output", func(t *testing.T) {
		adapter := &harness.Adapter{Name: "malformed", Malformed: true}
		resp, err := adapter.Run(agent.Request{Capability: agent.Implement, Task: "x"})
		if err == nil {
			t.Fatal("expected malformed adapter output to fail")
		}
		if resp.Outcome != nil {
			t.Fatalf("malformed adapter produced an outcome %+v, want nil", resp.Outcome)
		}
	})

	t.Run("non-zero exit", func(t *testing.T) {
		adapter := &harness.Adapter{Name: "exit", ExitCode: 1, Stderr: "boom"}
		resp, err := adapter.Run(agent.Request{Capability: agent.Implement, Task: "x"})
		if err == nil {
			t.Fatal("expected failing adapter to return an error")
		}
		if !strings.Contains(err.Error(), "exited 1") {
			t.Fatalf("err = %v, want it to report the exit code", err)
		}
		if resp.Outcome != nil {
			t.Fatalf("failing adapter produced an outcome %+v, want nil", resp.Outcome)
		}
		if len(adapter.Received()) != 1 {
			t.Fatalf("adapter received %d requests, want 1", len(adapter.Received()))
		}
	})
}
