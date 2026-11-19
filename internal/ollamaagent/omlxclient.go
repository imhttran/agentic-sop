package ollamaagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/perf"
)

// defaultOMLXBaseURL is the local oMLX OpenAI-compatible endpoint.
const defaultOMLXBaseURL = "http://127.0.0.1:8000"

// openAIChatClient adapts an OpenAI-compatible chat-completions endpoint (oMLX)
// to the same single-turn chatClient the Ollama client implements. The bounded
// tool loop, the controlled tool executor, permission checks, edit tracking, and
// iteration limits are unchanged: this file only carries model turns.
//
// The loop still speaks its one-JSON-object-per-turn protocol. The client asks
// the endpoint for a JSON object and parses either a content tool call
// ({"tool":...,"args":...}) or native OpenAI tool_calls, so both forms reach the
// existing turnToolCall seam.
type openAIChatClient struct {
	provider        string
	baseURL         string
	model           string
	apiKey          string
	http            *http.Client
	maxBytes        int
	maxOutputTokens int
	enableThinking  bool
	retryDelay      time.Duration
}

// newOpenAIChatClient builds the oMLX transport from the shared harness Config.
// It never executes a tool and never mutates the repository; those stay in the
// tool loop.
func newOpenAIChatClient(cfg Config) chatClient {
	provider := strings.TrimSpace(cfg.Provider)
	if provider == "" {
		provider = "mlx"
	}
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if baseURL == "" {
		baseURL = defaultOMLXBaseURL
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	return &openAIChatClient{
		provider:        provider,
		baseURL:         baseURL,
		model:           cfg.Model,
		apiKey:          strings.TrimSpace(cfg.APIKey),
		http:            &http.Client{Timeout: timeout},
		maxBytes:        maxHTTPResponseBytes,
		maxOutputTokens: cfg.MaxOutputTokens,
		enableThinking:  cfg.EnableThinking,
		retryDelay:      defaultProviderRetryDelay,
	}
}

// openAIChatMessage is one request message. Native-tool mode carries the
// assistant tool_calls and the matching tool_call_id result; the default path
// remains plain role/content.
type openAIChatMessage struct {
	Role       string                  `json:"role"`
	Content    string                  `json:"content"`
	ToolCalls  []openAIToolCallRequest `json:"tool_calls,omitempty"`
	ToolCallID string                  `json:"tool_call_id,omitempty"`
}

// openAIResponseFormat requests a single JSON object, matching the loop's
// protocol and keeping a final outcome or tool call parseable.
type openAIResponseFormat struct {
	Type string `json:"type"`
}

type openAIFunctionSpec struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters"`
}

type openAIToolSpec struct {
	Type     string             `json:"type"`
	Function openAIFunctionSpec `json:"function"`
}

type openAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type openAIToolCallRequest struct {
	ID       string             `json:"id"`
	Type     string             `json:"type"`
	Function openAIFunctionCall `json:"function"`
}

type openAIChatRequest struct {
	Model              string                `json:"model"`
	Messages           []openAIChatMessage   `json:"messages"`
	Stream             bool                  `json:"stream"`
	MaxTokens          int                   `json:"max_tokens,omitempty"`
	ResponseFormat     *openAIResponseFormat `json:"response_format,omitempty"`
	ChatTemplateKwargs map[string]any        `json:"chat_template_kwargs,omitempty"`
	Tools              []openAIToolSpec      `json:"tools,omitempty"`
	ToolChoice         string                `json:"tool_choice,omitempty"`
}

type openAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openAIChoice struct {
	Message struct {
		Content   string           `json:"content"`
		ToolCalls []openAIToolCall `json:"tool_calls"`
	} `json:"message"`
	FinishReason string `json:"finish_reason"`
}

type openAIChatResponse struct {
	Model   string          `json:"model"`
	Choices []openAIChoice  `json:"choices"`
	Error   json.RawMessage `json:"error"`
	Usage   struct {
		PromptTokens     *int `json:"prompt_tokens"`
		CompletionTokens *int `json:"completion_tokens"`
	} `json:"usage"`
}

// chat is the bounded retry wrapper: it re-sends the same conversation until the
// provider succeeds or the fixed provider-retry budget is exhausted. A retry
// happens before a returned tool call is dispatched, so it cannot duplicate a
// tool invocation.
func (c *openAIChatClient) chat(ctx context.Context, messages []chatMessage) (string, []toolCall, error) {
	return c.chatWithTools(ctx, messages, nil)
}

// chatWithTools is the native-tool variant: it declares only the supplied tools
// and omits response_format. The bounded retry policy is identical.
func (c *openAIChatClient) chatWithTools(ctx context.Context, messages []chatMessage, tools []nativeToolSpec) (string, []toolCall, error) {
	var lastErr error
	for attempt := 1; attempt <= maxProviderAttempts; attempt++ {
		content, calls, err := c.chatOnceWithTools(ctx, messages, tools)
		if err == nil {
			return content, calls, nil
		}
		lastErr = err
		if attempt == maxProviderAttempts || !retryableProviderError(err) {
			return "", nil, err
		}
		if err := c.retryBackoff(ctx, attempt); err != nil {
			return "", nil, err
		}
	}
	return "", nil, lastErr
}

// chatOnce performs exactly one POST /v1/chat/completions. It never retries
// itself; chat owns the bounded retry policy.
func (c *openAIChatClient) chatOnce(ctx context.Context, messages []chatMessage) (string, []toolCall, error) {
	return c.chatOnceWithTools(ctx, messages, nil)
}

func (c *openAIChatClient) chatOnceWithTools(ctx context.Context, messages []chatMessage, tools []nativeToolSpec) (string, []toolCall, error) {
	if err := perf.CheckTokenBudget(ctx); err != nil {
		return "", nil, err
	}
	body := openAIChatRequest{
		Model:     c.model,
		Messages:  openAIMessages(messages),
		Stream:    false,
		MaxTokens: c.maxOutputTokens,
	}
	if len(tools) > 0 {
		body.Tools = toOpenAITools(tools)
		body.ToolChoice = "auto"
		// Native tool turns omit response_format; the prompt asks for the final
		// SOP outcome JSON once the model stops calling tools.
	} else {
		body.ResponseFormat = &openAIResponseFormat{Type: "json_object"}
	}
	if !c.enableThinking {
		// Qwen-style oMLX models otherwise spend the entire bounded output
		// budget on reasoning_content and return no content or tool call.
		body.ChatTemplateKwargs = map[string]any{"enable_thinking": false}
	} else {
		// Send an explicit true rather than omitting the field: a server whose
		// template defaults thinking off would otherwise ignore the operator's
		// SOP_MLX_ENABLE_THINKING=true request.
		body.ChatTemplateKwargs = map[string]any{"enable_thinking": true}
	}
	data, err := json.Marshal(body)
	if err != nil {
		return "", nil, fmt.Errorf("%s: encode request: %w", c.provider, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(data))
	if err != nil {
		return "", nil, fmt.Errorf("%s: build request: %w", c.provider, err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return "", nil, c.transportFailure(ctx, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, int64(c.maxBytes)))
	if err != nil {
		// A timeout can occur after the response headers arrive while reading
		// the body. Route it through the same classifier as connection-level
		// failures so it remains a typed, retryable provider timeout.
		return "", nil, c.transportFailure(ctx, err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", nil, &ollamaHTTPError{provider: c.provider, statusCode: resp.StatusCode, baseURL: c.baseURL, body: string(raw)}
	}

	var out openAIChatResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", nil, fmt.Errorf("%s: parse response: %w", c.provider, err)
	}
	if detail := rawErrorMessage(out.Error); detail != "" {
		return "", nil, fmt.Errorf("%s: %s", c.provider, detail)
	}
	if len(out.Choices) == 0 {
		return "", nil, fmt.Errorf("%s returned no choices", c.provider)
	}
	reported := out.Usage.PromptTokens != nil && out.Usage.CompletionTokens != nil
	input, output := 0, 0
	if out.Usage.PromptTokens != nil {
		input = *out.Usage.PromptTokens
	}
	if out.Usage.CompletionTokens != nil {
		output = *out.Usage.CompletionTokens
	}
	perf.AddUsageWithReport(ctx, c.model, input, output, reported)

	choice := out.Choices[0]
	if agent.OutputTokenLimited(choice.FinishReason) {
		return "", nil, &agent.TruncationError{
			Provider: c.provider,
			Limit:    c.maxOutputTokens,
			Reason:   choice.FinishReason,
		}
	}
	calls := normalizeOpenAIToolCalls(choice.Message.ToolCalls)
	if strings.TrimSpace(choice.Message.Content) == "" && len(calls) == 0 {
		return "", nil, errEmptyResponse
	}
	return choice.Message.Content, calls, nil
}

// transportFailure classifies a transport-layer failure. A context deadline, an
// http.Client timeout, or an error whose Timeout() is true is a timeout; every
// other reachability failure is a transport error. Keeping the distinction
// explicit is what lets the retry boundary and the operator hint stay accurate.
func (c *openAIChatClient) transportFailure(ctx context.Context, err error) error {
	var timeout interface{ Timeout() bool }
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) ||
		(errors.As(err, &timeout) && timeout.Timeout()) {
		return &ollamaTimeoutError{provider: c.provider, timeout: c.http.Timeout, env: agent.EnvMLXTimeout}
	}
	return &ollamaTransportError{provider: c.provider, baseURL: c.baseURL, err: err}
}

func (c *openAIChatClient) retryBackoff(ctx context.Context, attempt int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if delay := c.retryDelay * time.Duration(attempt); delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	return nil
}

// openAIMessages converts the loop's transcript to the OpenAI message shape.
// Assistant tool-call echoes are ordinary assistant content, matching the loop's
// one-object protocol; no tool-role messages are synthesized.
func openAIMessages(messages []chatMessage) []openAIChatMessage {
	out := make([]openAIChatMessage, 0, len(messages))
	for _, m := range messages {
		om := openAIChatMessage{Role: m.Role, Content: m.Content, ToolCallID: m.ToolCallID}
		for _, tc := range m.NativeToolCalls {
			args, err := json.Marshal(tc.Args)
			if err != nil {
				args = []byte("{}")
			}
			om.ToolCalls = append(om.ToolCalls, openAIToolCallRequest{
				ID:       tc.ID,
				Type:     "function",
				Function: openAIFunctionCall{Name: tc.Name, Arguments: string(args)},
			})
		}
		out = append(out, om)
	}
	return out
}

// normalizeOpenAIToolCalls converts OpenAI's function tool_calls (arguments as a
// JSON string) into the loop's normalized form, dropping unnamed calls. The
// shared toolArguments helper handles the string/object and "args" wrapper shapes
// the models commonly emit.
func normalizeOpenAIToolCalls(raw []openAIToolCall) []toolCall {
	out := make([]toolCall, 0, len(raw))
	for _, rc := range raw {
		name := strings.TrimSpace(rc.Function.Name)
		if name == "" {
			continue
		}
		out = append(out, toolCall{ID: strings.TrimSpace(rc.ID), Name: name, Args: toolArguments(json.RawMessage(rc.Function.Arguments))})
	}
	return out
}

// toOpenAITools projects the provider-neutral specs into the OpenAI request shape.
func toOpenAITools(specs []nativeToolSpec) []openAIToolSpec {
	out := make([]openAIToolSpec, 0, len(specs))
	for _, spec := range specs {
		out = append(out, openAIToolSpec{
			Type: "function",
			Function: openAIFunctionSpec{
				Name:        spec.Name,
				Description: spec.Description,
				Parameters:  spec.Parameters,
			},
		})
	}
	return out
}
