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

// openAITextCapabilities is the set the text-only OpenAI-compatible providers
// (llama.cpp, MLX/oMLX) can actually serve. They generate text for planning, test
// design, diagnosis and review, but cannot mutate the repository, so IMPLEMENT and
// FIX are excluded. Sharing one set keeps the two identities honest and identical:
// the transport, not the identity, is what determines the capability ceiling.
var openAITextCapabilities = NewCapabilities(Plan, DesignTests, DiagnoseFailure, Review)

// OpenAICompatible is an Agent backed by an OpenAI-compatible chat-completions
// endpoint (POST /v1/chat/completions). It is the shared execution transport for
// llama.cpp's llama-server and an MLX/oMLX runtime; the identity only changes the
// error text, never the wire protocol. It performs model inference only — it
// cannot mutate the repository or workflow state.
type OpenAICompatible struct {
	id      string
	baseURL string
	model   string
	apiKey  string
	client  *http.Client
}

// LlamaCpp and MLX are the two configured identities of the shared transport.
type (
	LlamaCpp = OpenAICompatible
	MLX      = OpenAICompatible
)

// NewOpenAICompatible returns an OpenAI-compatible provider for the given provider
// identity, base URL, model, and optional API key (empty for local servers without
// auth). id is the canonical provider name (for example agent.ProviderLlamaCpp) and
// is used only for diagnostics.
func NewOpenAICompatible(id, baseURL, model, apiKey string, timeout time.Duration) (*OpenAICompatible, error) {
	baseURL = normalizeBaseURL(baseURL)
	model = strings.TrimSpace(model)
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("openai-compatible provider: identity is required")
	}
	if baseURL == "" {
		return nil, fmt.Errorf("%s: %w", id, errBaseURLRequired)
	}
	if model == "" {
		return nil, fmt.Errorf("%s: model is required", id)
	}
	if timeout <= 0 {
		timeout = defaultProviderTimeout
	}
	return &OpenAICompatible{
		id:      id,
		baseURL: baseURL,
		model:   model,
		apiKey:  strings.TrimSpace(apiKey),
		client:  &http.Client{Timeout: timeout},
	}, nil
}

// NewLlamaCpp returns an OpenAI-compatible provider for the given base URL,
// model, and optional API key (empty for local servers without auth).
func NewLlamaCpp(baseURL, model, apiKey string, timeout time.Duration) (*LlamaCpp, error) {
	return NewOpenAICompatible(ProviderLlamaCpp, baseURL, model, apiKey, timeout)
}

// NewLlamaCppFromEnv builds an OpenAI-compatible provider from the environment,
// using configuredModel when SOP_LLAMACPP_MODEL is unset. Only the base URL and
// model have defaults; the timeout and API key are optional.
func NewLlamaCppFromEnv(configuredModel string) (Agent, error) {
	baseURL := strings.TrimSpace(os.Getenv(EnvLlamaCppBaseURL))
	if baseURL == "" {
		baseURL = defaultLlamaCppBaseURL
	}
	model := strings.TrimSpace(os.Getenv(EnvLlamaCppModel))
	if model == "" {
		model = strings.TrimSpace(configuredModel)
	}
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
func (c *OpenAICompatible) Generate(ctx context.Context, request Request) (Response, error) {
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
		return Response{}, fmt.Errorf("%s %s: %w", c.id, request.Capability, err)
	}
	if detail := rawError(out.Error); detail != "" {
		return Response{}, fmt.Errorf("%s %s: %s", c.id, request.Capability, detail)
	}
	if len(out.Choices) == 0 {
		return Response{}, fmt.Errorf("%s %s returned no choices", c.id, request.Capability)
	}
	return requireNonEmpty(c.id, request.Capability, out.Choices[0].Message.Content)
}

// Capabilities declares the text-generation capabilities the OpenAI-compatible
// providers serve. They cannot mutate the repository, so IMPLEMENT and FIX are
// excluded; such requests are rejected rather than silently forwarded.
func (c *OpenAICompatible) Capabilities() Capabilities { return openAITextCapabilities }
