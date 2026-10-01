package agent

import (
	"os"
	"strings"
)

// Environment variables for the generic OpenAI-compatible provider's
// configuration surface. SOP_OPENAI_COMPATIBLE_BASE_URL is the same setting the
// config layer resolves (internal/config.EnvOpenAICompatibleBaseURL), so the two
// layers share one name rather than drifting.
//
// The endpoint precedence is environment > configuration > default. This layer
// resolves the environment and default tiers (OpenAICompatibleEndpointFromEnv);
// the composition root (internal/cli) adds the project-configuration tier,
// because the agent layer cannot import internal/config (config imports
// autonomy, which imports agent).
const (
	EnvOpenAICompatibleBaseURL = "SOP_OPENAI_COMPATIBLE_BASE_URL"
	EnvOpenAICompatibleModel   = "SOP_OPENAI_COMPATIBLE_MODEL"
	EnvOpenAICompatibleTimeout = "SOP_OPENAI_COMPATIBLE_TIMEOUT"
	EnvOpenAICompatibleAPIKey  = "SOP_OPENAI_COMPATIBLE_API_KEY"
)

// defaultOpenAICompatibleBaseURL is the generic OpenAI-compatible provider's
// built-in endpoint. It matches config.DefaultOpenAICompatibleBaseURL so the
// execution path and the configuration resolver cannot diverge
// (drift-guarded by TestOpenAICompatibleEndpointResolution).
const defaultOpenAICompatibleBaseURL = "http://127.0.0.1:8000"

// OpenAICompatibleEndpointFromEnv returns the generic provider's endpoint from
// the environment (SOP_OPENAI_COMPATIBLE_BASE_URL), or the built-in default. It
// resolves the environment and default tiers of the documented precedence; a
// caller that holds the project configuration resolves the configuration tier
// (config.Config.ResolveOpenAICompatibleEndpoint) before construction.
func OpenAICompatibleEndpointFromEnv() string {
	if v := strings.TrimSpace(os.Getenv(EnvOpenAICompatibleBaseURL)); v != "" {
		return v
	}
	return defaultOpenAICompatibleBaseURL
}

// NewOpenAICompatibleExecution builds the shared OpenAI-compatible execution agent
// (POST /v1/chat/completions) for the generic openai_compatible identity at
// endpoint. It uses the same transport the llama.cpp and MLX adapters use — only
// the identity and endpoint differ, never the wire protocol — so it is not tied
// to any single runtime (oMLX, vLLM, LM Studio, LocalAI, ...). The configured
// model name is sent unchanged.
//
// The model is resolved with the same precedence as FromConfig:
// SOP_AGENT_MODEL > SOP_OPENAI_COMPATIBLE_MODEL > configuredModel. The API key
// and timeout are optional (a local endpoint needs no credential); a credential,
// when set, is used only for the Authorization header and is never logged or
// recorded.
func NewOpenAICompatibleExecution(endpoint, configuredModel string) (Agent, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		endpoint = defaultOpenAICompatibleBaseURL
	}

	model := strings.TrimSpace(os.Getenv(EnvAgentModel))
	if model == "" {
		model = strings.TrimSpace(os.Getenv(EnvOpenAICompatibleModel))
	}
	if model == "" {
		model = strings.TrimSpace(configuredModel)
	}
	if model == "" {
		return nil, errNoModel(ProviderOpenAICompatible, EnvOpenAICompatibleModel)
	}

	timeout, err := timeoutFromEnv(EnvOpenAICompatibleTimeout)
	if err != nil {
		return nil, err
	}
	return NewOpenAICompatible(ProviderOpenAICompatible, endpoint, model, os.Getenv(EnvOpenAICompatibleAPIKey), timeout)
}
