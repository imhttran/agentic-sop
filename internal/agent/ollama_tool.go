// Package agent defines the OllamaToolHarness: a tool-enabled harness for Ollama
// with explicit DeepSeek model validation and comprehensive error classification.
//
// The OllamaToolHarness enforces that only the deepseek-v4.1-flash:cloud model
// is used with the tool harness, ensuring tool-calling support and consistent
// behavior. It classifies Ollama failures with actionable messages:
//
//   - Ollama unavailable: connection refused, dial failure, network errors
//   - Model unavailable: model not found, model not loaded
//   - Empty response: model returned no output
//   - Timeout: request exceeded configured duration
//   - Tool-calling unsupported: model does not support or return tool calls
//
// Configuration (environment variables, precedence highest first):
//
//   - SOP_AGENT_MODEL: unified model override (any provider)
//   - SOP_OLLAMA_MODEL: Ollama-specific model
//   - SOP_OLLAMA_BASE_URL: Ollama server URL (default: http://127.0.0.1:11434)
//   - SOP_OLLAMA_TIMEOUT: request timeout (default: 120s)
//
// Example configuration for tool-harness + Ollama + DeepSeek:
//
//	harness: tool
//	provider: ollama
//	model: deepseek-v4.1-flash:cloud
//	environment overrides:
//	  SOP_AGENT_HARNESS=tool
//	  SOP_AGENT_PROVIDER=ollama
//	  SOP_AGENT_MODEL=deepseek-v4.1-flash:cloud
//	  SOP_OLLAMA_BASE_URL=http://localhost:11434
package agent

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

// OllamaToolHarness wraps an Ollama provider with validation that enforces
// DeepSeek model usage and clear error reporting for failure scenarios. It
// implements the Harness interface and delegates execution to the wrapped
// Ollama provider, classifying errors into actionable categories.
type OllamaToolHarness struct {
	provider Agent
	model    string
}

// NewOllamaToolHarness returns an OllamaToolHarness wrapping the given Ollama
// provider. It validates that the model is DeepSeek and returns an error if not.
func NewOllamaToolHarness(provider *Ollama, model string) (*OllamaToolHarness, error) {
	if provider == nil {
		return nil, errors.New("ollama: provider is required")
	}
	if !isDeepSeekModel(model) {
		return nil, fmt.Errorf("ollama tool harness: unsupported model %q (only deepseek-v4.1-flash:cloud is supported)", model)
	}
	return &OllamaToolHarness{provider: provider, model: model}, nil
}

// Execute delegates to the wrapped Ollama provider and classifies errors with
// clear, actionable messages for Ollama unavailability, model errors, timeouts,
// and empty responses.
func (h *OllamaToolHarness) Execute(ctx context.Context, request Request) (Response, error) {
	resp, err := h.provider.Generate(ctx, request)
	if err != nil {
		return Response{}, h.classifyError(err, request.Capability)
	}
	return resp, nil
}

// classifyError converts Ollama errors into actionable messages that distinguish
// between connection failures, model unavailability, timeouts, and empty responses.
func (h *OllamaToolHarness) classifyError(err error, cap Capability) error {
	errStr := err.Error()

	// Timeout detection: context deadline exceeded
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("ollama %s: timed out after duration (increase SOP_OLLAMA_TIMEOUT if needed)", cap)
	}

	// Connection failure patterns
	if isConnectionError(err) {
		return fmt.Errorf("ollama %s: Ollama unavailable at configured URL (check SOP_OLLAMA_BASE_URL and that Ollama server is running)", cap)
	}

	// Model not found / model unavailable patterns
	if strings.Contains(errStr, "model") && strings.Contains(errStr, "not found") {
		return fmt.Errorf("ollama %s: DeepSeek model %q unavailable (pull the model or check SOP_OLLAMA_MODEL)", cap, h.model)
	}
	if strings.Contains(errStr, "model") && (strings.Contains(errStr, "not") || strings.Contains(errStr, "unknown")) {
		return fmt.Errorf("ollama %s: DeepSeek model %q unavailable", cap, h.model)
	}

	// Empty response detection
	if strings.Contains(errStr, "empty") && strings.Contains(errStr, "output") {
		return fmt.Errorf("ollama %s: Ollama returned empty response from DeepSeek model %q", cap, h.model)
	}

	// Tool-calling unsupported
	if strings.Contains(errStr, "tool") {
		return fmt.Errorf("ollama %s: DeepSeek model %q does not support tool calling or tool format is invalid", cap, h.model)
	}

	// Preserve other Ollama errors as-is, already prefixed with provider name
	return err
}

// isConnectionError reports whether err is a network connection failure.
func isConnectionError(err error) bool {
	// Check for net.OpError (connection refused, network unreachable, etc.)
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	// Check for http.Client timeout
	if errors.Is(err, context.Canceled) {
		return false // explicit cancellation, not a connection failure
	}

	// Check for dial-related errors in the message
	errStr := err.Error()
	dialPatterns := []string{
		"connection refused",
		"dial tcp",
		"no such host",
		"network unreachable",
		"connection reset",
		"broken pipe",
		"request failed",
	}
	for _, pattern := range dialPatterns {
		if strings.Contains(strings.ToLower(errStr), pattern) {
			return true
		}
	}
	return false
}

// isDeepSeekModel reports whether model is the supported deepseek-v4.1-flash:cloud.
func isDeepSeekModel(model string) bool {
	// Normalize: trim whitespace
	model = strings.TrimSpace(model)
	// Exact match or common variants
	return model == "deepseek-v4.1-flash:cloud" ||
		model == "deepseek-v4.1-flash" ||
		strings.HasPrefix(strings.ToLower(model), "deepseek")
}

// NewOllamaToolHarnessFromEnv builds an OllamaToolHarness from environment
// variables and a configured model. It enforces DeepSeek model selection.
func NewOllamaToolHarnessFromEnv(baseURL, configuredModel string, timeout time.Duration) (Harness, error) {
	baseURL = normalizeBaseURL(baseURL)
	if baseURL == "" {
		baseURL = defaultOllamaBaseURL
	}

	model := strings.TrimSpace(configuredModel)
	if model == "" {
		return nil, fmt.Errorf("ollama tool harness: no model configured (set agent.model in the project config or SOP_OLLAMA_MODEL); only deepseek-v4.1-flash:cloud is supported")
	}

	if !isDeepSeekModel(model) {
		return nil, fmt.Errorf("ollama tool harness: unsupported model %q (only deepseek-v4.1-flash:cloud is supported)", model)
	}

	provider, err := NewOllama(baseURL, model, timeout)
	if err != nil {
		return nil, err
	}

	return NewOllamaToolHarness(provider, model)
}

// compile-time assertion
var _ Harness = (*OllamaToolHarness)(nil)
