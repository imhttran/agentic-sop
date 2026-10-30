// Package ollama adapts a local or cloud Ollama server to the provider
// interface: it reports reachability (/api/version), the served models
// (/api/tags), a model's capabilities (/api/show), and a model's locality from
// the same /api/show evidence.
//
// It is read-only and it never guesses locality: the runtime itself reports the
// evidence. A /api/tags entry carries a remote_host field when the model is
// served by Ollama Cloud, and an entry without one is served by this local
// server. The same remote_host field in /api/show carries the evidence when a
// caller asks for one model's locality. A missing or empty remote_host in
// /api/show stays undetermined — it is never read as local or cloud from the
// model name, the endpoint host, or an absence of ":cloud" — and a failed
// request or an absent model reports no locality either, so callers never
// mistake an assumption for evidence.
package ollama

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

// Models lists GET /api/tags, including each model's reported metadata (family,
// parameter size, quantization, context length, size) and locality: an entry
// with a non-empty remote_host is served by Ollama Cloud, and an entry without
// one is served by this local server. A server that does not answer the list
// reports ErrDiscoveryUnsupported so absence is never assumed.
func (p *Provider) Models(ctx context.Context) ([]provider.ModelInfo, error) {
	if p.baseURL == "" {
		return nil, provider.ErrDiscoveryUnsupported
	}
	var out struct {
		Models []struct {
			Name       string `json:"name"`
			Model      string `json:"model"`
			Size       int64  `json:"size"`
			RemoteHost string `json:"remote_host"`
			Details    struct {
				Family        string `json:"family"`
				ParameterSize string `json:"parameter_size"`
				Quantization  string `json:"quantization_level"`
				ContextLength int    `json:"context_length"`
			} `json:"details"`
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
		// remote_host is the runtime's own locality evidence: a non-empty value
		// means Ollama Cloud serves the model, and a listed entry without one is
		// served by this local server. Nothing else — not the name, not the
		// endpoint host — is locality evidence.
		locality := model.LocalityLocal
		if strings.TrimSpace(m.RemoteHost) != "" {
			locality = model.LocalityCloud
		}
		infos = append(infos, provider.ModelInfo{
			Name:          name,
			Provider:      provider.Ollama,
			Locality:      locality,
			Capabilities:  streamingCapabilities(),
			Family:        strings.TrimSpace(m.Details.Family),
			ParameterSize: strings.TrimSpace(m.Details.ParameterSize),
			Quantization:  strings.TrimSpace(m.Details.Quantization),
			ContextWindow: m.Details.ContextLength,
			SizeBytes:     m.Size,
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

// Locality reports a named model's locality from Ollama's /api/show
// remote_host evidence, implementing provider.LocalityReporter. A non-empty
// remote_host means Ollama Cloud serves the model. A missing or empty
// remote_host — and equally a failed request, an unsupported discovery (no
// endpoint configured), or an absent model — reports the zero model.Locality
// ("not determined"): it is never read as local or cloud from the model name.
// It is read-only: it sends no generation request, changes no SOP state, and
// reports the evidence without printing endpoints or credentials.
func (p *Provider) Locality(ctx context.Context, modelName string) model.Locality {
	name := strings.TrimSpace(modelName)
	if name == "" || p.baseURL == "" {
		return ""
	}
	var out struct {
		RemoteHost string `json:"remote_host"`
	}
	if err := httpx.PostJSON(ctx, p.client, p.baseURL+"/api/show", map[string]string{"model": name}, &out); err != nil {
		// The model may be unknown or the request may have failed; either way
		// the locality is undetermined, not an assumption.
		return ""
	}
	if strings.TrimSpace(out.RemoteHost) == "" {
		return ""
	}
	return model.LocalityCloud
}

// streamingCapabilities is what an Ollama server is known to support regardless
// of model: its API always offers streaming for a completion-capable model.
func streamingCapabilities() provider.Capabilities {
	return provider.Capabilities{Streaming: provider.CapYes}
}
