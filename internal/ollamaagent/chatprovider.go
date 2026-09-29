package ollamaagent

import (
	"context"
	"encoding/json"
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
// timeout, and bounded response size all come from Config. The provider exposes
// both the generic JSON chat and a schema-constrained variant, so a caller that
// needs a structured contract (JEV) shares exactly this provider path rather than
// introducing a second one.
func NewChatProvider(cfg Config) (interface {
	Chat(ctx context.Context, model, prompt string) (string, error)
	ChatStructured(ctx context.Context, model, prompt string, schema json.RawMessage) (string, error)
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
	return p.converse(ctx, model, prompt, nil)
}

// ChatStructured sends a single user message constrained to schema, reusing the
// same client, transport, timeout, response bound, model selection, and provider
// retry as Chat. It is the structured-operation the JEV adapter drives so the
// provider enforces the output schema instead of the model inferring it.
func (p *chatProvider) ChatStructured(ctx context.Context, model, prompt string, schema json.RawMessage) (string, error) {
	return p.converse(ctx, model, prompt, schema)
}

// converse is the shared body of Chat and ChatStructured: it selects the model
// and issues exactly one logical chat turn through the reused client. A nil
// schema selects the generic JSON mode.
func (p *chatProvider) converse(ctx context.Context, model, prompt string, schema json.RawMessage) (string, error) {
	c := *p.client
	if m := strings.TrimSpace(model); m != "" {
		c.model = m
	}
	content, _, err := c.chatStructured(ctx, []chatMessage{{Role: "user", Content: prompt}}, schema)
	if err != nil {
		return "", err
	}
	return content, nil
}
