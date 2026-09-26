package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// Environment variables configuring the Ollama provider.
const (
	EnvOllamaBaseURL = "SOP_OLLAMA_BASE_URL"
	EnvOllamaModel   = "SOP_OLLAMA_MODEL"
	EnvOllamaTimeout = "SOP_OLLAMA_TIMEOUT"
)

// defaultOllamaBaseURL is Ollama's default local address.
const defaultOllamaBaseURL = "http://127.0.0.1:11434"

// Ollama is an Agent backed by a local Ollama server's chat API.
type Ollama struct {
	baseURL string
	model   string
	client  *http.Client
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
	return &Ollama{baseURL: baseURL, model: model, client: &http.Client{Timeout: timeout}}, nil
}

// NewOllamaFromEnv builds an Ollama provider from the environment. The model is
// required; the base URL and timeout fall back to Ollama's default address and
// the shared default timeout.
func NewOllamaFromEnv() (Agent, error) {
	model := strings.TrimSpace(os.Getenv(EnvOllamaModel))
	if model == "" {
		return nil, errMissingEnv("ollama", EnvOllamaModel)
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

	var out struct {
		Message chatMessage     `json:"message"`
		Error   json.RawMessage `json:"error"`
	}
	if err := postJSON(ctx, o.client, o.baseURL+"/api/chat", "", body, &out); err != nil {
		return Response{}, fmt.Errorf("ollama %s: %w", request.Capability, err)
	}
	if detail := rawError(out.Error); detail != "" {
		return Response{}, fmt.Errorf("ollama %s: %s", request.Capability, detail)
	}
	return requireNonEmpty("ollama", request.Capability, out.Message.Content)
}
