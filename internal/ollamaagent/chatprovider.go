package ollamaagent

import (
	"context"
	"fmt"
	"strings"
)

// chatProvider adapts the package's existing ollamaClient to the single-turn chat
// entry point the JEV adapter needs. It reuses newOllamaClient, so the JEV
// adapter drives the same Ollama provider path (base URL, model, timeout, and
// bounded response size) rather than introducing a second provider stack.
type chatProvider struct {
	client *ollamaClient
}

// NewChatProvider returns a single-turn Ollama chat provider built from the
// reused configuration. It reuses newOllamaClient, so the base URL, model,
// timeout, and bounded response size all come from Config.
func NewChatProvider(cfg Config) (interface {
	Chat(ctx context.Context, model, prompt string) (string, error)
}, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, fmt.Errorf("ollama: base URL is required")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, fmt.Errorf("ollama: model is required")
	}
	return &chatProvider{client: newOllamaClient(cfg)}, nil
}

// Chat sends a single user message to Ollama's /api/chat endpoint and returns the
// assistant content. Empty content is an error: the JEV adapter fails closed
// rather than treating an empty response as a pass. The model argument allows
// the caller to select the model; when empty the client's configured model is
// used, so no model value is required to be hardcoded.
func (p *chatProvider) Chat(ctx context.Context, model, prompt string) (string, error) {
	c := *p.client
	if m := strings.TrimSpace(model); m != "" {
		c.model = m
	}
	content, _, err := c.chat(ctx, []chatMessage{{Role: "user", Content: prompt}})
	if err != nil {
		return "", err
	}
	return content, nil
}
