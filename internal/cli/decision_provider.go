package cli

import (
	"strings"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/decision"
	"github.com/imhttran/agentic-sop/internal/decision/command"
)

// decisionProviderFromConfig constructs the optional, provider-neutral decision
// provider from configuration.
//
// It returns (nil, nil) when the capability is disabled or no provider is
// configured: the capability is OFF by default, and constructing it is a strict
// no-op. It contains no provider-specific branch — a provider is selected by
// configuration, never by SOP policy, and adding a provider requires no change
// here beyond naming its neutral transport ("command"). It never grants the
// provider policy, lifecycle, approval, commit, merge, or execution authority;
// it only builds a decision.Provider.
func decisionProviderFromConfig(cfg config.Config) (decision.Provider, error) {
	if !cfg.Decision.Enabled {
		return nil, nil
	}
	switch name := strings.TrimSpace(cfg.Decision.Provider); name {
	case "command":
		return command.New("command", cfg.Decision.Command)
	default:
		// "" and "deterministic" return the in-process deterministic provider;
		// any other name fails closed rather than silently falling back.
		return decision.NewProvider(name)
	}
}
