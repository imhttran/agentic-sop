package config

import (
	"os"
	"strings"
)

// Configuration surface for the generic OpenAI-compatible provider identity.
//
// Phase 1 only RECOGNIZES and CONFIGURES this provider: the endpoint resolves
// here, and internal/agent keeps the provider non-executable. The endpoint is
// independent of SOP_MLX_BASE_URL and SOP_LLAMACPP_BASE_URL, so configuring the
// generic provider can never move the specialized mlx or llamacpp endpoints.
const (
	// OpenAICompatibleEndpointKey documents the generic OpenAI-compatible
	// provider's configuration key: providers.openai_compatible.endpoint.
	OpenAICompatibleEndpointKey = "providers.openai_compatible.endpoint"
	// EnvOpenAICompatibleBaseURL overrides the configured endpoint. It is
	// deliberately distinct from SOP_MLX_BASE_URL / SOP_LLAMACPP_BASE_URL.
	EnvOpenAICompatibleBaseURL = "SOP_OPENAI_COMPATIBLE_BASE_URL"
	// DefaultOpenAICompatibleBaseURL is the built-in endpoint used when neither
	// the environment nor the configuration names one.
	DefaultOpenAICompatibleBaseURL = "http://127.0.0.1:8000"
)

// OpenAICompatibleEndpoint returns the endpoint configured for the generic
// OpenAI-compatible provider, or the empty string when none is configured. An
// empty result means "not configured": ResolveOpenAICompatibleEndpoint then falls
// back to the built-in default, so configuration can never invent an endpoint.
func (p Providers) OpenAICompatibleEndpoint() string {
	return strings.TrimSpace(p.OpenaiCompatible.Endpoint)
}

// ResolveOpenAICompatibleEndpoint returns the effective generic
// OpenAI-compatible endpoint using the documented Phase 1 precedence:
//
//  1. environment (SOP_OPENAI_COMPATIBLE_BASE_URL, which a .env file may supply),
//  2. configuration (providers.openai_compatible.endpoint),
//  3. the built-in default (DefaultOpenAICompatibleBaseURL).
//
// It is a pure, string-only resolution: it performs no network I/O, constructs no
// provider runtime, and never triggers discovery. An unset or blank value at one
// layer falls through to the next, so a whitespace-only override cannot mask a
// configured endpoint. Recognition and configuration are all that Phase 1
// provides — the provider remains non-executable; see internal/agent.
func (c Config) ResolveOpenAICompatibleEndpoint() string {
	if env := strings.TrimSpace(os.Getenv(EnvOpenAICompatibleBaseURL)); env != "" {
		return env
	}
	if cfg := c.Providers.OpenAICompatibleEndpoint(); cfg != "" {
		return cfg
	}
	return DefaultOpenAICompatibleBaseURL
}
