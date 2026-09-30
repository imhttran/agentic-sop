// Package llamacpp adapts a llama.cpp llama-server (an OpenAI-compatible
// endpoint) to the provider interface. It shares its implementation with the MLX
// adapter via package openai; only the identity and default endpoint differ.
package llamacpp

import (
	"time"

	"github.com/imhttran/agentic-sop/internal/provider"
	"github.com/imhttran/agentic-sop/internal/provider/openai"
)

// DefaultBaseURL is llama-server's default local address.
const DefaultBaseURL = "http://127.0.0.1:8080"

// EnvBaseURL is the environment variable that overrides the configured llama.cpp
// endpoint (the same setting internal/agent reads).
const EnvBaseURL = "SOP_LLAMACPP_BASE_URL"

// New returns a llama.cpp provider for baseURL.
func New(baseURL string, timeout time.Duration) provider.Provider {
	return openai.New(provider.LlamaCPP, baseURL, timeout)
}
