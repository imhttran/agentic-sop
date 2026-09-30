// Package mlx adapts an Apple-Silicon MLX / oMLX runtime to the provider
// interface. It targets a generic OpenAI-compatible HTTP boundary (GET
// /v1/models), so SOP is not coupled to a specific MLX server implementation.
// The implementation is shared with the llama.cpp adapter via package openai.
package mlx

import (
	"time"

	"github.com/imhttran/agentic-sop/internal/provider"
	"github.com/imhttran/agentic-sop/internal/provider/openai"
)

// DefaultBaseURL is a common MLX / oMLX server address.
const DefaultBaseURL = "http://127.0.0.1:8000"

// EnvBaseURL is the environment variable that overrides the configured MLX
// endpoint.
const EnvBaseURL = "SOP_MLX_BASE_URL"

// New returns an MLX provider for baseURL.
func New(baseURL string, timeout time.Duration) provider.Provider {
	return openai.New(provider.MLX, baseURL, timeout)
}
