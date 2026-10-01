package agent

import (
	"os"
	"strings"
	"time"
)

// Environment variables configuring the OpenAI-compatible MLX / oMLX provider.
// SOP_MLX_BASE_URL is the same setting the provider/runtime layer reads
// (internal/provider/mlx.EnvBaseURL): one name for one endpoint.
const (
	EnvMLXBaseURL = "SOP_MLX_BASE_URL"
	EnvMLXModel   = "SOP_MLX_MODEL"
	EnvMLXTimeout = "SOP_MLX_TIMEOUT"
	EnvMLXAPIKey  = "SOP_MLX_API_KEY"
)

// defaultMLXBaseURL is a common MLX / oMLX server address.
const defaultMLXBaseURL = "http://127.0.0.1:8000"

// NewMLX returns an OpenAI-compatible provider for an Apple-Silicon MLX / oMLX
// runtime. It shares the llama.cpp execution transport (OpenAICompatible): only
// the identity and default endpoint differ, so SOP is not coupled to a specific
// MLX server implementation.
func NewMLX(baseURL, model, apiKey string, timeout time.Duration) (*MLX, error) {
	return NewOpenAICompatible(ProviderMLX, baseURL, model, apiKey, timeout)
}

// NewMLXFromEnv builds an MLX provider from the environment, using configuredModel
// when SOP_MLX_MODEL is unset. Only the base URL has a default; the model is
// required from one of the two sources.
func NewMLXFromEnv(configuredModel string) (Agent, error) {
	baseURL := strings.TrimSpace(os.Getenv(EnvMLXBaseURL))
	if baseURL == "" {
		baseURL = defaultMLXBaseURL
	}
	model := strings.TrimSpace(os.Getenv(EnvMLXModel))
	if model == "" {
		model = strings.TrimSpace(configuredModel)
	}
	if model == "" {
		return nil, errNoModel("mlx", EnvMLXModel)
	}
	timeout, err := timeoutFromEnv(EnvMLXTimeout)
	if err != nil {
		return nil, err
	}
	return NewMLX(baseURL, model, os.Getenv(EnvMLXAPIKey), timeout)
}
