package commandprofile

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/decision"
	"github.com/imhttran/agentic-sop/internal/decision/bridge"
)

// This is the JEV-213 offline integration test. It exercises the REAL production
// connector (bridge.Build -> private temp request file -> adapter subprocess ->
// strict decode -> bridge.Interpret) and then the deterministic governance step
// (decision.Validate -> decision.Route), against a deterministic fake adapter
// subprocess. No live inference.

func integrationRegistry(profile string) *bridge.Registry {
	if profile == "nimble" {
		return bridge.NewRegistry(&bridge.Profile{Name: "nimble", Version: 1, Kinds: map[string]bridge.KindSpec{
			"change-risk": {Type: bridge.TypeChoice, ChoiceMap: map[string]decision.Choice{
				"LOW": decision.Low, "MEDIUM": decision.Medium, "HIGH": decision.High,
			}},
		}})
	}
	return bridge.NewRegistry(&bridge.Profile{Name: "julia", Version: 1, Kinds: map[string]bridge.KindSpec{
		"change-risk": {Type: bridge.TypeChoice, ChoiceMap: map[string]decision.Choice{
			"low": decision.Low, "medium": decision.Medium, "high": decision.High, "human": decision.Human,
		}},
	}})
}

func integrationConnector(t *testing.T, profile, mode, choice string) *Connector {
	t.Helper()
	t.Setenv("GO_WANT_ADAPTER_HELPER", "1")
	t.Setenv("ADAPTER_MODE", mode)
	t.Setenv("ADAPTER_CHOICE", choice)
	c, err := New(Options{
		Registry: integrationRegistry(profile),
		Argv:     []string{os.Args[0], "-test.run=^TestAdapterHelperProcess$", "--"},
		Provider: profile,
		Model:    profile + "-1",
		Timeout:  10 * time.Second,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return c
}

func integrationThresholds() decision.Thresholds {
	return decision.Thresholds{RouteToStrongModel: 0.7, RequireHuman: 0.4}
}

func TestAdapterConnectorIntegration(t *testing.T) {
	t.Run("successful evidence reaches deterministic governance", func(t *testing.T) {
		c := integrationConnector(t, "julia", "success", "high")
		d, err := c.Decide(context.Background(), decision.Request{UseCase: "change-risk", Subject: "harden the analyzer"})
		if err != nil {
			t.Fatalf("Decide: %v", err)
		}
		if err := decision.Validate(d); err != nil {
			t.Fatalf("governance rejected valid evidence: %v", err)
		}
		if got := decision.Route(integrationThresholds(), d); got != decision.HumanTarget {
			t.Fatalf("route = %v, want HUMAN (a HIGH choice requires a human)", got)
		}
	})

	t.Run("human approval boundary remains enforced", func(t *testing.T) {
		c := integrationConnector(t, "julia", "success", "human")
		d, err := c.Decide(context.Background(), decision.Request{UseCase: "change-risk", Subject: "escalate"})
		if err != nil {
			t.Fatalf("Decide: %v", err)
		}
		if got := decision.Route(integrationThresholds(), d); got != decision.HumanTarget {
			t.Fatalf("route = %v, want HUMAN", got)
		}
	})

	t.Run("unsupported, malformed, and unavailable responses fail closed", func(t *testing.T) {
		// Unsupported capability: a kind with no profile.
		c := integrationConnector(t, "julia", "success", "high")
		if _, err := c.Decide(context.Background(), decision.Request{UseCase: "no-such-kind"}); !errors.Is(err, decision.ErrUnsupportedCapability) {
			t.Errorf("unsupported: err = %v, want ErrUnsupportedCapability", err)
		}
		// Malformed output.
		if _, err := integrationConnector(t, "julia", "malformed", "high").Decide(context.Background(), decision.Request{UseCase: "change-risk"}); !errors.Is(err, decision.ErrInvalidResult) {
			t.Errorf("malformed: err = %v, want ErrInvalidResult", err)
		}
		// Unavailable provider (exit 3).
		if _, err := integrationConnector(t, "julia", "unavailable", "high").Decide(context.Background(), decision.Request{UseCase: "change-risk"}); !errors.Is(err, decision.ErrProviderFailure) {
			t.Errorf("unavailable: err = %v, want ErrProviderFailure", err)
		}
	})

	t.Run("no implicit fallback to an unapproved provider", func(t *testing.T) {
		c := integrationConnector(t, "julia", "wrongprovider", "high")
		if _, err := c.Decide(context.Background(), decision.Request{UseCase: "change-risk"}); !errors.Is(err, decision.ErrInvalidResult) {
			t.Fatalf("fallback: err = %v, want ErrInvalidResult", err)
		}
	})

	t.Run("adapter is disabled without explicit configuration", func(t *testing.T) {
		if _, err := New(Options{}); err == nil {
			t.Fatal("a connector with no registry/command must fail closed")
		}
	})

	t.Run("julia and nimble profile compatibility", func(t *testing.T) {
		jd, err := integrationConnector(t, "julia", "success", "high").Decide(context.Background(), decision.Request{UseCase: "change-risk"})
		if err != nil {
			t.Fatalf("julia: %v", err)
		}
		nd, err := integrationConnector(t, "nimble", "success", "HIGH").Decide(context.Background(), decision.Request{UseCase: "change-risk"})
		if err != nil {
			t.Fatalf("nimble: %v", err)
		}
		if jd.Choice != decision.High || nd.Choice != decision.High {
			t.Fatalf("julia=%v nimble=%v, want both HIGH", jd.Choice, nd.Choice)
		}
	})

	t.Run("no credentials in persisted evidence or errors", func(t *testing.T) {
		_, err := integrationConnector(t, "julia", "unavailable", "high").Decide(context.Background(), decision.Request{UseCase: "change-risk"})
		if err == nil {
			t.Fatal("expected an error")
		}
		low := strings.ToLower(err.Error())
		for _, bad := range []string{"api_key", "bearer", "password", "secret", "token", "http://"} {
			if strings.Contains(low, bad) {
				t.Errorf("error leaks %q: %q", bad, err.Error())
			}
		}
	})
}
