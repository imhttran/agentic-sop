package deepseekagent

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

// chatMessage is one message in an Ollama chat conversation.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ollamaChatRequest is the request body for POST /api/chat. Format "json"
// constrains the model to emit a single JSON object, which is what the tool
// protocol and the final outcome both require.
type ollamaChatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
	Format   string        `json:"format,omitempty"`
}

type ollamaChatResponse struct {
	Message chatMessage     `json:"message"`
	Error   json.RawMessage `json:"error"`
}

// errEmptyResponse is returned when the model produces no message content. It is
// retry-able: cloud models occasionally emit only reasoning (a "thinking" turn),
// and re-asking usually produces the answer.
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
// message content. Every failure is actionable and never silently defaulted.
func (c *ollamaClient) chat(ctx context.Context, messages []chatMessage) (string, error) {
	body, err := json.Marshal(ollamaChatRequest{
		Model:    c.model,
		Messages: messages,
		Stream:   false,
		Format:   "json",
	})
	if err != nil {
		return "", fmt.Errorf("encode ollama request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("build ollama request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("ollama timed out after %s", c.http.Timeout)
		}
		return "", fmt.Errorf("ollama unavailable at %s: %w", c.baseURL, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, int64(c.maxBytes)))
	if err != nil {
		return "", fmt.Errorf("read ollama response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama returned HTTP %d at %s: %s", resp.StatusCode, c.baseURL, truncate(strings.TrimSpace(string(data)), 512))
	}

	var out ollamaChatResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("parse ollama response: %w", err)
	}
	if detail := rawErrorMessage(out.Error); detail != "" {
		return "", fmt.Errorf("ollama: %s", detail)
	}
	if strings.TrimSpace(out.Message.Content) == "" {
		return "", errEmptyResponse
	}
	return out.Message.Content, nil
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
