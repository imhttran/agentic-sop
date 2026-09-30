// Package ollama adapts a local or cloud Ollama server to the provider
// interface: it reports reachability (/api/version), the served models
// (/api/tags), and a model's capabilities (/api/show).
//
// It is read-only and never assumes a discovered model is local: Ollama's model
// list does not report locality, so ModelInfo.Locality is left empty and the
// authoritative locality stays the model-routing selection's.
package ollama

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/provider"
	"github.com/imhttran/agentic-sop/internal/provider/httpx"
)

// DefaultBaseURL is Ollama's default local address.
const DefaultBaseURL = "http://127.0.0.1:11434"

// EnvBaseURL is the environment variable that overrides the configured Ollama
// endpoint. It is the same setting internal/agent reads for the agent path, so
// there is one name for one endpoint.
const EnvBaseURL = "SOP_OLLAMA_BASE_URL"

// Provider inspects an Ollama server.
type Provider struct {
	baseURL string
	client  *http.Client
}

// New returns an Ollama provider for baseURL. An empty baseURL is retained and
// reported as unavailable rather than defaulted here, so the caller owns the
// default.
func New(baseURL string, timeout time.Duration) *Provider {
	return &Provider{
		baseURL: httpx.NormalizeBaseURL(baseURL),
		client:  httpx.NewClient(timeout),
	}
}

// ID returns the provider identity.
func (p *Provider) ID() provider.ID { return provider.Ollama }

// Health probes GET /api/version.
func (p *Provider) Health(ctx context.Context) provider.HealthResult {
	if p.baseURL == "" {
		return provider.HealthResult{Status: provider.HealthUnavailable, Message: "no endpoint configured"}
	}
	status, _, err := httpx.Get(ctx, p.client, p.baseURL+"/api/version")
	if err != nil {
		return provider.HealthResult{Status: provider.HealthUnavailable, Message: "unreachable"}
	}
	if status >= 200 && status < 300 {
		return provider.HealthResult{Status: provider.HealthHealthy, Message: "ok"}
	}
	return provider.HealthResult{Status: provider.HealthDegraded, Message: fmt.Sprintf("http %d", status)}
}

// Models lists GET /api/tags. A server that does not answer the list reports
// ErrDiscoveryUnsupported so absence is never assumed.
func (p *Provider) Models(ctx context.Context) ([]provider.ModelInfo, error) {
	if p.baseURL == "" {
		return nil, provider.ErrDiscoveryUnsupported
	}
	var out struct {
		Models []struct {
			Name  string `json:"name"`
			Model string `json:"model"`
		} `json:"models"`
	}
	if err := httpx.GetJSON(ctx, p.client, p.baseURL+"/api/tags", &out); err != nil {
		return nil, provider.ErrDiscoveryUnsupported
	}
	infos := make([]provider.ModelInfo, 0, len(out.Models))
	seen := map[string]bool{}
	for _, m := range out.Models {
		name := strings.TrimSpace(m.Name)
		if name == "" {
			name = strings.TrimSpace(m.Model)
		}
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		infos = append(infos, provider.ModelInfo{
			Name:         name,
			Provider:     provider.Ollama,
			Capabilities: streamingCapabilities(),
		})
	}
	return infos, nil
}

// Capabilities reads GET /api/show for the model's declared capabilities. A
// server that does not report them (older Ollama, or an unknown model) yields an
// honest CapUnknown set rather than a fabricated one.
func (p *Provider) Capabilities(ctx context.Context, modelName string) (provider.Capabilities, error) {
	name := strings.TrimSpace(modelName)
	if name == "" {
		return provider.Capabilities{}, fmt.Errorf("ollama: model is required")
	}
	caps := streamingCapabilities()
	if p.baseURL == "" {
		return caps, nil
	}
	var out struct {
		Capabilities []string `json:"capabilities"`
	}
	if err := httpx.PostJSON(ctx, p.client, p.baseURL+"/api/show", map[string]string{"model": name}, &out); err != nil {
		// The model may be unknown or the server may not implement /api/show;
		// either way the capabilities are undetermined, not absent.
		return caps, nil
	}
	for _, c := range out.Capabilities {
		switch strings.ToLower(strings.TrimSpace(c)) {
		case "completion":
			caps.Chat = provider.CapYes
		case "tools":
			caps.Tools = provider.CapYes
		case "vision":
			caps.Images = provider.CapYes
		case "embedding":
			caps.Embeddings = provider.CapYes
		case "thinking":
			caps.Reasoning = provider.CapYes
		}
	}
	return caps, nil
}

// streamingCapabilities is what an Ollama server is known to support regardless
// of model: its API always offers streaming for a completion-capable model.
func streamingCapabilities() provider.Capabilities {
	return provider.Capabilities{Streaming: provider.CapYes}
}
