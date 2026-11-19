package agent

import (
	"fmt"
	"os"
	"strconv"
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

	// EnvMLXToolLoop enables the governed oMLX tool loop for the mlx provider. It is
	// opt-in: by default the MLX/oMLX path stays the text-only transport it has
	// always been and advertises only the four text-generation capabilities. When
	// enabled, SOP builds the same bounded in-process tool loop the native Ollama
	// harness uses, so IMPLEMENT and FIX are served through the existing controlled
	// tools (permission checks, edit tracking, and iteration limits) rather than a
	// new execution path.
	EnvMLXToolLoop = "SOP_MLX_TOOL_LOOP"

	// EnvMLXEnableThinking opts the oMLX tool loop back into a model's reasoning
	// channel. Reasoning is disabled by default because it consumes the bounded
	// output budget and can leave a turn with no content or tool call; an operator
	// who wants it sets this to true.
	EnvMLXEnableThinking = "SOP_MLX_ENABLE_THINKING"
)

// MLXToolLoopEnabled reports whether the governed oMLX tool loop is enabled. An
// unset value keeps the existing text-only behavior. A set value must parse as a
// boolean; a malformed value is an actionable error rather than a silent
// fallback.
func MLXToolLoopEnabled() (bool, error) { return boolEnv(EnvMLXToolLoop) }

// MLXThinkingEnabled reports whether the oMLX tool loop may use a model's
// reasoning channel. Unset keeps it disabled; a malformed value is an error.
func MLXThinkingEnabled() (bool, error) { return boolEnv(EnvMLXEnableThinking) }

// boolEnv parses an optional boolean environment setting with one consistent
// rule: unset is false, and anything else must be a valid boolean.
func boolEnv(name string) (bool, error) {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return false, nil
	}
	on, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("invalid %s %q: want true or false", name, v)
	}
	return on, nil
}

// MLXTimeoutFromEnv resolves SOP_MLX_TIMEOUT with the same rules as the other
// provider timeouts: unset keeps the default, and a set value must be a positive
// Go duration. It is exported so the governed oMLX constructor and the text-only
// MLX factory cannot diverge on timeout validation.
func MLXTimeoutFromEnv() (time.Duration, error) {
	return timeoutFromEnv(EnvMLXTimeout)
}

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
