// Package openai adapts an OpenAI-compatible HTTP endpoint to the provider
// interface. It is shared by the llama.cpp and MLX adapters, which differ only in
// their identity and default endpoint: both expose GET /v1/models and speak the
// chat-completions vocabulary.
//
// The adapter is read-only: it lists models and reports reachability. It never
// sends a completion and never chooses a model class.
package openai

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/model"
	"github.com/imhttran/agentic-sop/internal/provider"
	"github.com/imhttran/agentic-sop/internal/provider/httpx"
)

// Provider inspects an OpenAI-compatible endpoint under a given provider id.
type Provider struct {
	id      provider.ID
	baseURL string
	client  *http.Client
}

// New returns a provider for id at baseURL. An empty baseURL is retained and
// reported as unavailable rather than defaulted here, so the caller owns the
// default.
func New(id provider.ID, baseURL string, timeout time.Duration) *Provider {
	return &Provider{
		id:      id,
		baseURL: httpx.NormalizeBaseURL(baseURL),
		client:  httpx.NewClient(timeout),
	}
}

// ID returns the provider identity.
func (p *Provider) ID() provider.ID { return p.id }

// Health probes the model list, which every OpenAI-compatible server exposes.
func (p *Provider) Health(ctx context.Context) provider.HealthResult {
	if p.baseURL == "" {
		return provider.HealthResult{Status: provider.HealthUnavailable, Message: "no endpoint configured"}
	}
	status, _, err := httpx.Get(ctx, p.client, p.baseURL+"/v1/models")
	if err != nil {
		return provider.HealthResult{Status: provider.HealthUnavailable, Message: "unreachable"}
	}
	if status >= 200 && status < 300 {
		return provider.HealthResult{Status: provider.HealthHealthy, Message: "ok"}
	}
	return provider.HealthResult{Status: provider.HealthDegraded, Message: fmt.Sprintf("http %d", status)}
}

// Models lists the models reported by GET /v1/models. A server that does not
// answer the list reports ErrDiscoveryUnsupported so absence is never assumed.
func (p *Provider) Models(ctx context.Context) ([]provider.ModelInfo, error) {
	if p.baseURL == "" {
		return nil, provider.ErrDiscoveryUnsupported
	}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := httpx.GetJSON(ctx, p.client, p.baseURL+"/v1/models", &out); err != nil {
		return nil, provider.ErrDiscoveryUnsupported
	}
	infos := make([]provider.ModelInfo, 0, len(out.Data))
	seen := map[string]bool{}
	for _, d := range out.Data {
		name := strings.TrimSpace(d.ID)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		infos = append(infos, provider.ModelInfo{
			Name:         name,
			Provider:     p.id,
			Locality:     model.LocalityLocal,
			Capabilities: chatCapabilities(),
		})
	}
	return infos, nil
}

// Capabilities reports the capabilities an OpenAI-compatible chat server is
// known to provide. Only chat and streaming are reliable; everything else stays
// unknown rather than being fabricated.
func (p *Provider) Capabilities(_ context.Context, modelName string) (provider.Capabilities, error) {
	if strings.TrimSpace(modelName) == "" {
		return provider.Capabilities{}, fmt.Errorf("%s: model is required", p.id)
	}
	return chatCapabilities(), nil
}

// chatCapabilities is the capability set an OpenAI-compatible chat server is
// known to support.
func chatCapabilities() provider.Capabilities {
	return provider.Capabilities{
		Chat:      provider.CapYes,
		Streaming: provider.CapYes,
	}
}
