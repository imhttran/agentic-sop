package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/imhttran/agentic-sop/internal/model"
)

// ValidateSelection checks that a resolved model selection can plausibly run
// before SOP spends any agent work on it: the provider is known and registered,
// it is reachable when reachability can be determined, the selected model exists
// when the provider can authoritatively enumerate models, and the model can chat.
//
// It is a read-only observation — it starts no execution and changes no state.
// It is deliberately conservative about what it does NOT know:
//
//   - a provider that cannot enumerate models (ErrDiscoveryUnsupported) does NOT
//     cause an "absent" verdict; absence is reported only when discovery is
//     authoritative;
//   - a capability the provider reports as unknown is NOT treated as missing.
//
// It never substitutes one provider or model for another. On failure the caller
// decides, deterministically, whether to proceed, skip, or stop — that policy is
// not this function's.
func ValidateSelection(ctx context.Context, reg *Registry, sel model.Selection) error {
	if reg == nil {
		return errors.New("provider: no registry")
	}
	if strings.TrimSpace(sel.Provider) == "" {
		return fmt.Errorf("provider: selection names no provider")
	}
	id, err := ParseID(sel.Provider)
	if err != nil {
		return err
	}
	p, err := reg.Get(id)
	if err != nil {
		return err
	}

	if name := strings.TrimSpace(sel.Model); name == "" {
		return fmt.Errorf("provider %s: %w", id, ErrModelRequired)
	}

	// Reachability. Unknown and degraded reachability do not fail validation:
	// only a definite "unavailable" does.
	if h := p.Health(ctx); h.Status == HealthUnavailable {
		return fmt.Errorf("provider %s: %w: %s", id, ErrProviderUnavailable, h.Message)
	}

	// Model presence, only when discovery is authoritative.
	infos, err := p.Models(ctx)
	switch {
	case errors.Is(err, ErrDiscoveryUnsupported):
		// Cannot tell: do not claim absence.
	case err != nil:
		return fmt.Errorf("provider %s: enumerate models: %w", id, err)
	default:
		if !containsModel(infos, sel.Model) {
			return fmt.Errorf("provider %s: %w: %q", id, ErrModelNotFound, sel.Model)
		}
	}

	// Basic chat capability, only when the provider determined it.
	caps, err := p.Capabilities(ctx, sel.Model)
	if err != nil {
		return fmt.Errorf("provider %s: capabilities for %q: %w", id, sel.Model, err)
	}
	if caps.Chat.Known() && !caps.Chat.Present() {
		return fmt.Errorf("provider %s: model %q cannot chat", id, sel.Model)
	}
	return nil
}

// containsModel reports whether the enumerated set includes name.
func containsModel(infos []ModelInfo, name string) bool {
	name = strings.TrimSpace(name)
	for _, m := range infos {
		if m.Name == name {
			return true
		}
	}
	return false
}
