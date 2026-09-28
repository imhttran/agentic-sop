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
)

// chatMessage is one message in an Ollama chat conversation. ToolCalls is
// populated on a response when the model used Ollama's native tool calling — in
// that case content is empty. It is never set on a request.
type chatMessage struct {
	Role      string           `json:"role"`
	Content   string           `json:"content"`
	ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`
}

// ollamaToolCall is one native tool call in a response. Arguments is kept raw
// because Ollama returns it as an object (not the string OpenAI uses).
type ollamaToolCall struct {
	Function struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	} `json:"function"`
}

// toolCall is a normalized tool invocation, whatever protocol expressed it.
type toolCall struct {
	Name string
	Args map[string]any
}

// ollamaChatRequest is the request body for POST /api/chat. Format "json"
// constrains the model to emit a single JSON object, which is what the tool
// protocol and the final outcome both require. Think disables the model's
// reasoning channel (see chat).
type ollamaChatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
	Format   string        `json:"format,omitempty"`
	Think    *bool         `json:"think,omitempty"`
}

type ollamaChatResponse struct {
	Message chatMessage     `json:"message"`
	Error   json.RawMessage `json:"error"`
}

// errEmptyResponse is returned when the model produces no message content. It is
// retry-able: a model can occasionally emit an empty turn, and re-asking usually
// produces the answer. Reasoning is disabled (see chat) so this is rare.
var errEmptyResponse = errors.New("ollama returned an empty response")

// ollamaClient is a small client for Ollama's /api/chat endpoint. It is
// deliberately independent of SOP's Ollama provider: it needs multi-turn
// messages and a forced JSON format, which the provider's single-prompt boundary
// does not expose.
type ollamaClient struct {
	baseURL  string
	model    string
	http     *http.Client
	maxBytes int
}

func newOllamaClient(cfg Config) *ollamaClient {
	return &ollamaClient{
		baseURL:  strings.TrimRight(cfg.BaseURL, "/"),
		model:    cfg.Model,
		http:     &http.Client{Timeout: cfg.Timeout},
		maxBytes: maxHTTPResponseBytes,
	}
}

// chat sends one non-streaming chat completion and returns the assistant
// message's content and any native tool calls. A turn with neither is
// errEmptyResponse, which the caller retries; every other failure is actionable
// and never silently defaulted.
func (c *ollamaClient) chat(ctx context.Context, messages []chatMessage) (string, []toolCall, error) {
	// Thinking models (for example deepseek-v4.1-flash) emit some turns with an
	// empty content field and the reasoning in "thinking", and split a turn
	// across several tool calls. The harness only consumes content and speaks a
	// one-object-per-turn protocol, so reasoning is disabled: it is discarded
	// anyway and otherwise produces unusable empty turns.
	think := false
	body, err := json.Marshal(ollamaChatRequest{
		Model:    c.model,
		Messages: messages,
		Stream:   false,
		Format:   "json",
		Think:    &think,
	})
	if err != nil {
		return "", nil, fmt.Errorf("encode ollama request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return "", nil, fmt.Errorf("build ollama request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", nil, fmt.Errorf("ollama timed out after %s", c.http.Timeout)
		}
		return "", nil, fmt.Errorf("ollama unavailable at %s: %w", c.baseURL, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, int64(c.maxBytes)))
	if err != nil {
		return "", nil, fmt.Errorf("read ollama response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("ollama returned HTTP %d at %s: %s", resp.StatusCode, c.baseURL, truncate(strings.TrimSpace(string(data)), 512))
	}

	var out ollamaChatResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return "", nil, fmt.Errorf("parse ollama response: %w", err)
	}
	if detail := rawErrorMessage(out.Error); detail != "" {
		return "", nil, fmt.Errorf("ollama: %s", detail)
	}
	calls := normalizeToolCalls(out.Message.ToolCalls)
	if strings.TrimSpace(out.Message.Content) == "" && len(calls) == 0 {
		return "", nil, errEmptyResponse
	}
	return out.Message.Content, calls, nil
}

// normalizeToolCalls converts Ollama's native tool calls into the harness's
// (name, args) form, dropping any without a name.
func normalizeToolCalls(raw []ollamaToolCall) []toolCall {
	out := make([]toolCall, 0, len(raw))
	for _, rc := range raw {
		name := strings.TrimSpace(rc.Function.Name)
		if name == "" {
			continue
		}
		out = append(out, toolCall{Name: name, Args: toolArguments(rc.Function.Arguments)})
	}
	return out
}

// toolArguments decodes a native call's arguments. The model mirrors the prompt's
// {"tool":...,"args":...} shape, so it often wraps the real arguments under a
// single "args" key; that wrapper is unwrapped here.
func toolArguments(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			_ = json.Unmarshal([]byte(s), &m)
		}
	}
	if m == nil {
		return map[string]any{}
	}
	if inner, ok := m["args"].(map[string]any); ok {
		return inner
	}
	return m
}

// rawErrorMessage extracts a message from Ollama's "error" field, which may be a
// plain string or an object with a "message".
func rawErrorMessage(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s)
	}
	var obj struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil {
		return strings.TrimSpace(obj.Message)
	}
	return ""
}
