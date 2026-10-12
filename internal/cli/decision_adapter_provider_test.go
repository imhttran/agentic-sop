package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/decision"
)

// TestDecisionProviderAdapterFactory exercises the REAL production provider
// factory (decisionProviderFromConfig) end-to-end: it builds the connector from
// configuration and drives it against a deterministic fake adapter subprocess.
func TestDecisionProviderAdapterFactory(t *testing.T) {
	// Disabled by default: no provider, no-op, no process.
	if p, err := decisionProviderFromConfig(config.Config{}); err != nil || p != nil {
		t.Fatalf("disabled decision layer = (%v, %v), want (nil, nil)", p, err)
	}

	t.Setenv("GO_WANT_CLI_ADAPTER_HELPER", "1")
	t.Setenv("ADAPTER_CHOICE", "high")

	cfg := config.Config{Decision: config.DecisionConfig{
		Provider:        "adapter",
		Enabled:         true,
		AdapterCommand:  []string{os.Args[0], "-test.run=^TestDecisionAdapterHelperProcess$", "--"},
		AdapterProvider: "julia",
		AdapterModel:    "julia-1",
	}}

	p, err := decisionProviderFromConfig(cfg)
	if err != nil {
		t.Fatalf("decisionProviderFromConfig: %v", err)
	}
	if p == nil || p.Name() != "adapter" {
		t.Fatalf("provider = %v, want the adapter connector", p)
	}

	d, err := p.Decide(context.Background(), decision.Request{UseCase: "change-risk", Subject: "x"})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if d.Choice != decision.High || d.Confidence != 0.9 {
		t.Fatalf("decision = %+v, want HIGH/0.9", d)
	}
}

// TestDecisionProviderAdapterUnknownSelectionFailsClosed proves an unknown
// adapter selection is rejected at construction (fail closed).
func TestDecisionProviderAdapterUnknownSelectionFailsClosed(t *testing.T) {
	cfg := config.Config{Decision: config.DecisionConfig{
		Provider:        "adapter",
		Enabled:         true,
		AdapterCommand:  []string{"/nonexistent"},
		AdapterProvider: "skynet",
	}}
	if _, err := decisionProviderFromConfig(cfg); err == nil {
		t.Fatal("unknown adapter provider must fail closed")
	}
}

// TestDecisionAdapterHelperProcess is the deterministic fake adapter subprocess.
func TestDecisionAdapterHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_CLI_ADAPTER_HELPER") != "1" {
		return
	}
	provider := "julia"
	id := "change-risk"
	for i, a := range os.Args {
		if a == "-provider" && i+1 < len(os.Args) {
			provider = os.Args[i+1]
		}
		if a == "-file" && i+1 < len(os.Args) {
			if b, err := os.ReadFile(os.Args[i+1]); err == nil {
				var r struct {
					Questions []struct {
						ID string `json:"id"`
					} `json:"questions"`
				}
				if json.Unmarshal(b, &r) == nil && len(r.Questions) > 0 {
					id = r.Questions[0].ID
				}
			}
		}
	}
	choice := os.Getenv("ADAPTER_CHOICE")
	fmt.Printf(`{"provider":%q,"model":"julia-1","answers":{%q:{"type":"choice","choice":%q,"confidence":0.9}},"usage":{}}`, provider, id, choice)
	os.Exit(0)
}
