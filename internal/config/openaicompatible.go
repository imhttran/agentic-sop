package config

import (
	"os"
	"strings"

	"github.com/imhttran/agentic-sop/internal/model"
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
	// EnvOpenAICompatibleAPIKey supplies the shared OpenRouter (or other
	// OpenAI-compatible) credential. It is scoped to the shared endpoint's origin:
	// see ResolveOpenAICompatibleRoute.
	EnvOpenAICompatibleAPIKey = "SOP_OPENAI_COMPATIBLE_API_KEY"
	// DefaultOpenAICompatibleBaseURL is the built-in endpoint used when neither
	// the environment nor the configuration names one.
	DefaultOpenAICompatibleBaseURL = "http://127.0.0.1:8000"
)

// ClassBaseURLEnvKey returns the optional per-class endpoint override, for
// example SOP_MODEL_SMALL_BASE_URL. It takes precedence over the shared provider
// endpoint for that class only.
func ClassBaseURLEnvKey(c model.Class) string { return model.ClassEnvKey(c, "BASE_URL") }

// ClassAPIKeyEnvKey returns the optional per-class credential, for example
// SOP_MODEL_SMALL_API_KEY. It is used only when the class selects an endpoint
// that needs its own key.
func ClassAPIKeyEnvKey(c model.Class) string { return model.ClassEnvKey(c, "API_KEY") }

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

// ResolveOpenAICompatibleEndpointForClass returns the endpoint a class's
// openai_compatible model is served at. An optional SOP_MODEL_<CLASS>_BASE_URL
// takes precedence over the shared provider endpoint; when it is absent the
// shared resolution (environment > configuration > default) is unchanged.
func (c Config) ResolveOpenAICompatibleEndpointForClass(class model.Class) string {
	if class.Valid() {
		if v := strings.TrimSpace(os.Getenv(ClassBaseURLEnvKey(class))); v != "" {
			return v
		}
	}
	return c.ResolveOpenAICompatibleEndpoint()
}

// ResolveOpenAICompatibleRoute resolves the endpoint and credential for one
// class's openai_compatible model. The credential is scoped to its destination:
//
//  1. an optional SOP_MODEL_<CLASS>_API_KEY is used for that class's endpoint;
//  2. otherwise the shared SOP_OPENAI_COMPATIBLE_API_KEY is used only when the
//     class's effective endpoint has the SAME origin as the shared endpoint;
//  3. otherwise no credential is sent.
//
// A class endpoint override to a different origin (for example a local oMLX
// server while the shared endpoint is OpenRouter) therefore never receives the
// shared key. Neither endpoint nor key is logged.
func (c Config) ResolveOpenAICompatibleRoute(class model.Class) (endpoint, apiKey string) {
	shared := c.ResolveOpenAICompatibleEndpoint()
	endpoint = c.ResolveOpenAICompatibleEndpointForClass(class)
	if class.Valid() {
		if v := strings.TrimSpace(os.Getenv(ClassAPIKeyEnvKey(class))); v != "" {
			return endpoint, v
		}
	}
	sharedKey := strings.TrimSpace(os.Getenv(EnvOpenAICompatibleAPIKey))
	if sharedKey == "" {
		return endpoint, ""
	}
	if model.SameOrigin(endpoint, shared) {
		return endpoint, sharedKey
	}
	return endpoint, ""
}
