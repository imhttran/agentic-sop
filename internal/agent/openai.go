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

// Environment variables configuring the OpenAI-compatible provider used for
// llama.cpp's llama-server (and other OpenAI-compatible local endpoints).
const (
	EnvLlamaCppBaseURL = "SOP_LLAMACPP_BASE_URL"
	EnvLlamaCppModel   = "SOP_LLAMACPP_MODEL"
	EnvLlamaCppTimeout = "SOP_LLAMACPP_TIMEOUT"
	EnvLlamaCppAPIKey  = "SOP_LLAMACPP_API_KEY"
)

const (
	// defaultLlamaCppBaseURL is llama-server's default local address.
	defaultLlamaCppBaseURL = "http://127.0.0.1:8080"
	// defaultLlamaCppModel matches llama-server's single-model convention.
	defaultLlamaCppModel = "local"
)

// LlamaCpp is an Agent backed by an OpenAI-compatible chat completions
// endpoint, such as llama.cpp's llama-server.
type LlamaCpp struct {
	baseURL string
	model   string
	apiKey  string
	client  *http.Client
}

// NewLlamaCpp returns an OpenAI-compatible provider for the given base URL,
// model, and optional API key (empty for local servers without auth).
func NewLlamaCpp(baseURL, model, apiKey string, timeout time.Duration) (*LlamaCpp, error) {
	baseURL = normalizeBaseURL(baseURL)
	model = strings.TrimSpace(model)
	if baseURL == "" {
		return nil, fmt.Errorf("llamacpp: %w", errBaseURLRequired)
	}
	if model == "" {
		return nil, fmt.Errorf("llamacpp: model is required")
	}
	if timeout <= 0 {
		timeout = defaultProviderTimeout
	}
	return &LlamaCpp{
		baseURL: baseURL,
		model:   model,
		apiKey:  strings.TrimSpace(apiKey),
		client:  &http.Client{Timeout: timeout},
	}, nil
}

// NewLlamaCppFromEnv builds an OpenAI-compatible provider from the environment.
// Only the base URL and model have defaults; the timeout and API key are
// optional.
func NewLlamaCppFromEnv() (Agent, error) {
	baseURL := strings.TrimSpace(os.Getenv(EnvLlamaCppBaseURL))
	if baseURL == "" {
		baseURL = defaultLlamaCppBaseURL
	}
	model := strings.TrimSpace(os.Getenv(EnvLlamaCppModel))
	if model == "" {
		model = defaultLlamaCppModel
	}
	timeout, err := timeoutFromEnv(EnvLlamaCppTimeout)
	if err != nil {
		return nil, err
	}
	return NewLlamaCpp(baseURL, model, os.Getenv(EnvLlamaCppAPIKey), timeout)
}

// Generate validates the request and calls POST /v1/chat/completions with a
// single user message rendered from the request.
func (c *LlamaCpp) Generate(ctx context.Context, request Request) (Response, error) {
	if err := request.Validate(); err != nil {
		return Response{}, err
	}

	body := chatRequest{
		Model:    c.model,
		Messages: []chatMessage{{Role: "user", Content: renderPrompt(request)}},
		Stream:   false,
	}

	var out struct {
		Choices []struct {
			Message chatMessage `json:"message"`
		} `json:"choices"`
		Error json.RawMessage `json:"error"`
	}
	if err := postJSON(ctx, c.client, c.baseURL+"/v1/chat/completions", c.apiKey, body, &out); err != nil {
		return Response{}, fmt.Errorf("llamacpp %s: %w", request.Capability, err)
	}
	if detail := rawError(out.Error); detail != "" {
		return Response{}, fmt.Errorf("llamacpp %s: %s", request.Capability, detail)
	}
	if len(out.Choices) == 0 {
		return Response{}, fmt.Errorf("llamacpp %s returned no choices", request.Capability)
	}
	return requireNonEmpty("llamacpp", request.Capability, out.Choices[0].Message.Content)
}
