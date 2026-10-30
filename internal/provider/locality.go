package provider

import (
	"context"
	"strings"

	"github.com/imhttran/agentic-sop/internal/model"
)

// LocalityReporter is the optional contract a provider implements when its
// runtime reports a named model's locality from authoritative evidence (for
// example Ollama's remote_host field in /api/show). It is read-only: an
// implementation issues no generation request, changes no SOP state, and
// reports what the runtime reported without printing endpoints or credentials.
// A provider without such evidence must not implement the contract: reporting
// locality that no runtime supplied would be a guess.
type LocalityReporter interface {
	// Locality returns the locality the runtime reports for the named model, or
	// the zero model.Locality ("not determined") when it has no authoritative
	// evidence: an unsupported discovery, a failed request, or an absent model
	// all report not-determined rather than an assumption. It never reads
	// locality from the model name or the endpoint host.
	Locality(ctx context.Context, name string) model.Locality
}

// ModelLocality returns the locality the named model's provider reports from
// authoritative runtime evidence, resolved through reg. It returns the zero
// model.Locality ("not determined") for an unknown provider, an unregistered
// provider, a provider without the LocalityReporter contract (unsupported
// discovery), a failed request, an absent model, or an empty runtime report.
// It never substitutes a guess: the lookup itself consults no endpoint host,
// no ":cloud" suffix, and no model name — the only source is the provider's
// own report. The lookup is a pure observation: it sends no generation request
// and changes no SOP state.
func ModelLocality(ctx context.Context, reg *Registry, id ID, modelName string) model.Locality {
	if reg == nil || !id.Valid() {
		return ""
	}
	p, err := reg.Get(id)
	if err != nil || p == nil {
		return ""
	}
	reporter, ok := p.(LocalityReporter)
	if !ok {
		return ""
	}
	name := strings.TrimSpace(modelName)
	if name == "" {
		return ""
	}
	loc := reporter.Locality(ctx, name)
	if loc != model.LocalityLocal && loc != model.LocalityCloud {
		return ""
	}
	return loc
}
