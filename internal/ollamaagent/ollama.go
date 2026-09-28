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

// maxProviderAttempts bounds the provider-call retry budget for one logical chat
// turn: the initial attempt plus at most two retries. It is small and fixed so a
// transient provider failure is absorbed at the provider boundary instead of
// consuming the task's retry or FIX-cycle budget, and can never become an
// unbounded loop.
const maxProviderAttempts = 3

// defaultProviderRetryDelay is the base delay before a provider retry. The delay
// grows linearly per attempt and is capped by maxProviderAttempts, so total retry
// latency stays small. It is a field on ollamaClient so a caller (or a test) can
// change or eliminate the wait.
const defaultProviderRetryDelay = 200 * time.Millisecond

// ollamaHTTPError is a non-2xx response from Ollama. It carries the status code so
// the retry boundary decides from the type rather than by parsing the message,
// which is preserved unchanged for compatibility with failure classification.
type ollamaHTTPError struct {
	statusCode int
	baseURL    string
	body       string
}

func (e *ollamaHTTPError) Error() string {
	return fmt.Sprintf("ollama returned HTTP %d at %s: %s", e.statusCode, e.baseURL, truncate(strings.TrimSpace(e.body), 512))
}

// ollamaTimeoutError is a provider call that exceeded the client timeout. It is a
// transient condition and is retried at the provider boundary.
type ollamaTimeoutError struct{ timeout time.Duration }

func (e *ollamaTimeoutError) Error() string {
	return fmt.Sprintf("ollama timed out after %s", e.timeout)
}

// ollamaTransportError is a failure to reach Ollama (connection refused, reset,
// unavailable). It is transient and retryable, and it wraps the underlying error
// so a caller can still inspect it — in particular a context.Canceled that must
// never be retried.
type ollamaTransportError struct {
	baseURL string
	err     error
}

func (e *ollamaTransportError) Error() string {
	return fmt.Sprintf("ollama unavailable at %s: %v", e.baseURL, e.err)
}

func (e *ollamaTransportError) Unwrap() error { return e.err }

// ollamaClient is a small client for Ollama's /api/chat endpoint. It is
// deliberately independent of SOP's Ollama provider: it needs multi-turn
// messages and a forced JSON format, which the provider's single-prompt boundary
// does not expose.
type ollamaClient struct {
	baseURL  string
	model    string
	http     *http.Client
	maxBytes int
	// retryDelay is the base delay between provider retries. It is bounded by
	// maxProviderAttempts, and a non-positive value disables the wait entirely.
	retryDelay time.Duration
}

func newOllamaClient(cfg Config) *ollamaClient {
	return &ollamaClient{
		baseURL:    strings.TrimRight(cfg.BaseURL, "/"),
		model:      cfg.Model,
		http:       &http.Client{Timeout: cfg.Timeout},
		maxBytes:   maxHTTPResponseBytes,
		retryDelay: defaultProviderRetryDelay,
	}
}

// chat sends one non-streaming chat completion, retrying transient provider
// failures at the provider boundary. It returns the assistant message's content
// and any native tool calls. A retry re-sends the SAME request/messages and
// happens before any returned tool call is executed, so a transient failure can
// never duplicate a tool invocation.
func (c *ollamaClient) chat(ctx context.Context, messages []chatMessage) (string, []toolCall, error) {
	var lastErr error
	for attempt := 1; attempt <= maxProviderAttempts; attempt++ {
		content, calls, err := c.chatOnce(ctx, messages)
		if err == nil {
			return content, calls, nil
		}
		lastErr = err
		if attempt == maxProviderAttempts || !retryableProviderError(err) {
			return "", nil, err
		}
		if err := c.retryBackoff(ctx, attempt); err != nil {
			// The context ended while waiting to retry: stop immediately.
			return "", nil, err
		}
	}
	return "", nil, lastErr
}

// chatOnce performs exactly one HTTP request to Ollama's /api/chat. It never
// retries itself; chat is the bounded retry wrapper. It is a single-attempt
// operation so the retry policy lives in exactly one place.
func (c *ollamaClient) chatOnce(ctx context.Context, messages []chatMessage) (string, []toolCall, error) {
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
			return "", nil, &ollamaTimeoutError{timeout: c.http.Timeout}
		}
		return "", nil, &ollamaTransportError{baseURL: c.baseURL, err: err}
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, int64(c.maxBytes)))
	if err != nil {
		return "", nil, fmt.Errorf("read ollama response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", nil, &ollamaHTTPError{statusCode: resp.StatusCode, baseURL: c.baseURL, body: string(data)}
	}

	var out ollamaChatResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return "", nil, fmt.Errorf("parse ollama response: %w", err)
	}
	if detail := rawErrorMessage(out.Error); detail != "" {
		return "", nil, fmt.Errorf("ollama: %s", detail)
	}
	// A native tool call with empty content is a valid turn, never an empty
	// response: only content == "" with no tool calls is errEmptyResponse.
	calls := normalizeToolCalls(out.Message.ToolCalls)
	if strings.TrimSpace(out.Message.Content) == "" && len(calls) == 0 {
		return "", nil, errEmptyResponse
	}
	return out.Message.Content, calls, nil
}

// retryableProviderError reports whether a provider failure is transient and
// worth retrying at the provider boundary. Deterministic failures are never
// retried: a bad request, auth/permission, not-found, a request encoding error,
// invalid configuration, a malformed successful response, or cancellation.
func retryableProviderError(err error) bool {
	if err == nil {
		return false
	}
	// A cancelled or expired context is terminal whatever wrapped it: retrying a
	// request whose context is gone cannot succeed.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	if errors.Is(err, errEmptyResponse) {
		return true
	}
	var httpErr *ollamaHTTPError
	if errors.As(err, &httpErr) {
		// 429 (rate limited) and 5xx (server) are transient; 4xx other than 429 is
		// a deterministic client/protocol error.
		return httpErr.statusCode == http.StatusTooManyRequests || httpErr.statusCode >= http.StatusInternalServerError
	}
	var timeoutErr *ollamaTimeoutError
	if errors.As(err, &timeoutErr) {
		return true
	}
	var transportErr *ollamaTransportError
	if errors.As(err, &transportErr) {
		return true
	}
	return false
}

// retryBackoff waits before the next provider attempt. It is small and bounded
// (the delay grows linearly and never exceeds maxProviderAttempts steps), and it
// is context-aware: if the context ends first it returns the context error so the
// retry loop stops immediately.
func (c *ollamaClient) retryBackoff(ctx context.Context, attempt int) error {
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
