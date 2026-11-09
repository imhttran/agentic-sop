package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/perf"
)

// Environment variables configuring the Ollama provider.
const (
	EnvOllamaBaseURL = "SOP_OLLAMA_BASE_URL"
	EnvOllamaModel   = "SOP_OLLAMA_MODEL"
	EnvOllamaTimeout = "SOP_OLLAMA_TIMEOUT"
)

// defaultOllamaBaseURL is Ollama's default local address.
const defaultOllamaBaseURL = "http://127.0.0.1:11434"

// ollamaCapabilities is the set the native text-only Ollama provider can
// actually serve. It generates text for planning, test design, diagnosis and
// review, but cannot mutate the repository, so IMPLEMENT and FIX are excluded.
var ollamaCapabilities = NewCapabilities(Plan, DesignTests, DiagnoseFailure, Review)

// Ollama is an Agent backed by a local Ollama server's chat API.
type Ollama struct {
	baseURL         string
	model           string
	client          *http.Client
	maxOutputTokens int
}

// NewOllama returns an Ollama provider for the given base URL and model. An
// empty base URL, empty model, or non-positive timeout is rejected rather than
// silently defaulted, so a misconfiguration is visible immediately.
func NewOllama(baseURL, model string, timeout time.Duration) (*Ollama, error) {
	baseURL = normalizeBaseURL(baseURL)
	model = strings.TrimSpace(model)
	if baseURL == "" {
		return nil, fmt.Errorf("ollama: %w", errBaseURLRequired)
	}
	if model == "" {
		return nil, fmt.Errorf("ollama: model is required")
	}
	if timeout <= 0 {
		timeout = defaultProviderTimeout
	}
	maxOutputTokens, err := MaxOutputTokensFromEnv()
	if err != nil {
		return nil, err
	}
	return &Ollama{baseURL: baseURL, model: model, client: &http.Client{Timeout: timeout}, maxOutputTokens: maxOutputTokens}, nil
}

// NewOllamaFromEnv builds an Ollama provider from the environment, using
// configuredModel when SOP_OLLAMA_MODEL is unset. The model is required from one
// of the two sources; the base URL and timeout fall back to Ollama's default
// address and the shared default timeout.
func NewOllamaFromEnv(configuredModel string) (Agent, error) {
	model := strings.TrimSpace(os.Getenv(EnvOllamaModel))
	if model == "" {
		model = strings.TrimSpace(configuredModel)
	}
	if model == "" {
		return nil, errNoModel("ollama", EnvOllamaModel)
	}
	baseURL := strings.TrimSpace(os.Getenv(EnvOllamaBaseURL))
	if baseURL == "" {
		baseURL = defaultOllamaBaseURL
	}
	timeout, err := timeoutFromEnv(EnvOllamaTimeout)
	if err != nil {
		return nil, err
	}
	return NewOllama(baseURL, model, timeout)
}

// Generate validates the request and calls Ollama's /api/chat endpoint with a
// single user message rendered from the request.
func (o *Ollama) Generate(ctx context.Context, request Request) (Response, error) {
	if err := request.Validate(); err != nil {
		return Response{}, err
	}

	body := chatRequest{
		Model:    o.model,
		Messages: []chatMessage{{Role: "user", Content: renderPrompt(request)}},
		Stream:   false,
	}
	if o.maxOutputTokens > 0 {
		body.Options = &chatOptions{NumPredict: o.maxOutputTokens}
	}

	var out struct {
		Message         chatMessage     `json:"message"`
		Error           json.RawMessage `json:"error"`
		DoneReason      string          `json:"done_reason"`
		PromptEvalCount *int            `json:"prompt_eval_count"`
		EvalCount       *int            `json:"eval_count"`
	}
	if err := postJSON(ctx, o.client, o.baseURL+"/api/chat", "", body, &out); err != nil {
		return Response{}, fmt.Errorf("ollama %s: %w", request.Capability, withTimeoutHint(err, EnvOllamaTimeout))
	}
	reported := out.PromptEvalCount != nil && out.EvalCount != nil
	input, output := 0, 0
	if out.PromptEvalCount != nil {
		input = *out.PromptEvalCount
	}
	if out.EvalCount != nil {
		output = *out.EvalCount
	}
	perf.AddUsageWithReport(ctx, o.model, input, output, reported)
	if detail := rawError(out.Error); detail != "" {
		return Response{}, fmt.Errorf("ollama %s: %s", request.Capability, detail)
	}
	if OutputTokenLimited(out.DoneReason) {
		return Response{}, &TruncationError{
			Provider:   ProviderOllama,
			Capability: request.Capability,
			Limit:      o.maxOutputTokens,
			Reason:     out.DoneReason,
		}
	}
	return requireNonEmpty("ollama", request.Capability, out.Message.Content)
}

// Capabilities declares the text-generation capabilities the Ollama provider
// serves. It cannot mutate the repository, so IMPLEMENT and FIX are excluded;
// such requests are rejected rather than silently forwarded.
func (o *Ollama) Capabilities() Capabilities { return ollamaCapabilities }
