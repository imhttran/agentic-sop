package cli

import (
	"fmt"
	"strings"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/decision"
	"github.com/imhttran/agentic-sop/internal/decision/bridge"
)

// adapterDecisionKind is the canonical decision kind the decision-evidence
// checkpoint requests. It mirrors internal/cli/decision_evidence.go.
const adapterDecisionKind = "change-risk"

// adapterProfileRegistry builds the config-driven M3 profile registry for the
// selected adapter, derived from the JEV-206 adapter contracts. Selecting an
// adapter is configuration, not a policy branch: the registry is data, and an
// unknown selection fails closed rather than guessing.
func adapterProfileRegistry(cfg config.Config) (*bridge.Registry, error) {
	switch name := strings.TrimSpace(cfg.Decision.AdapterProvider); name {
	case "", "nimble":
		return bridge.NewRegistry(&bridge.Profile{Name: "nimble", Version: 1, Kinds: map[string]bridge.KindSpec{
			adapterDecisionKind: {Type: bridge.TypeChoice, ChoiceMap: map[string]decision.Choice{
				"LOW": decision.Low, "MEDIUM": decision.Medium, "HIGH": decision.High,
			}},
		}}), nil
	case "julia":
		return bridge.NewRegistry(&bridge.Profile{Name: "julia", Version: 1, Kinds: map[string]bridge.KindSpec{
			adapterDecisionKind: {Type: bridge.TypeChoice, ChoiceMap: map[string]decision.Choice{
				"low": decision.Low, "medium": decision.Medium, "high": decision.High, "human": decision.Human,
			}},
		}}), nil
	case "clef":
		return bridge.NewRegistry(&bridge.Profile{Name: "clef", Version: 1, Kinds: map[string]bridge.KindSpec{
			adapterDecisionKind: {Type: bridge.TypeChoice, ChoiceMap: map[string]decision.Choice{
				"LOW": decision.Low, "MEDIUM": decision.Medium, "HIGH": decision.High, "HUMAN": decision.Human,
			}},
		}}), nil
	default:
		return nil, fmt.Errorf("config: unknown decision.adapter_provider %q (want nimble, julia, clef)", name)
	}
}
