package provider

import "strings"

// Cap is a tri-state capability flag. Unlike a bool it can say "unknown", which
// matters for validation: an undetermined capability MUST NOT be treated as a
// missing one, so a provider that cannot report a capability never causes a
// valid model selection to be rejected.
type Cap string

const (
	// CapUnknown: the provider could not determine the capability.
	CapUnknown Cap = "unknown"
	// CapNo: the provider reported the capability as absent.
	CapNo Cap = "no"
	// CapYes: the provider reported the capability as present.
	CapYes Cap = "yes"
)

// Known reports whether the capability was determined (yes or no).
func (c Cap) Known() bool { return c == CapYes || c == CapNo }

// Present reports whether the capability is known to be present.
func (c Cap) Present() bool { return c == CapYes }

// String renders the capability state.
func (c Cap) String() string {
	if c == "" {
		return string(CapUnknown)
	}
	return string(c)
}

// Capabilities is the typed set of things a model can do, as far as its provider
// can determine. It is evidence only: it MUST NOT by itself change task state,
// approval, validation, review, quality gates, or the model class.
//
// Every field defaults to CapUnknown. Only fields a provider can determine
// reliably SHOULD be set; a speculative CapYes is worse than an honest
// CapUnknown.
type Capabilities struct {
	Chat             Cap
	Tools            Cap
	Streaming        Cap
	Images           Cap
	Embeddings       Cap
	Reasoning        Cap
	StructuredOutput Cap
}

// capabilityFields is the canonical, stable order used when rendering.
var capabilityFields = []struct {
	name string
	get  func(Capabilities) Cap
}{
	{"chat", func(c Capabilities) Cap { return c.Chat }},
	{"tools", func(c Capabilities) Cap { return c.Tools }},
	{"streaming", func(c Capabilities) Cap { return c.Streaming }},
	{"images", func(c Capabilities) Cap { return c.Images }},
	{"embeddings", func(c Capabilities) Cap { return c.Embeddings }},
	{"reasoning", func(c Capabilities) Cap { return c.Reasoning }},
	{"structured_output", func(c Capabilities) Cap { return c.StructuredOutput }},
}

// String renders the capabilities as a deterministic "name=state" list, omitting
// fields that are unknown so the output shows only what was actually determined.
func (c Capabilities) String() string {
	var parts []string
	for _, f := range capabilityFields {
		if v := f.get(c); v.Known() {
			parts = append(parts, f.name+"="+v.String())
		}
	}
	if len(parts) == 0 {
		return "none determined"
	}
	return strings.Join(parts, " ")
}
