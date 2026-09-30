package openai_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/imhttran/agentic-sop/internal/model"
	"github.com/imhttran/agentic-sop/internal/provider"
	"github.com/imhttran/agentic-sop/internal/provider/openai"
)

func servers(t *testing.T) (*httptest.Server, *httptest.Server) {
	t.Helper()
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"local-model"},{"id":"other"}]}`))
	}))
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	}))
	t.Cleanup(func() { ok.Close(); bad.Close() })
	return ok, bad
}

func TestOpenAIHealthAndModels(t *testing.T) {
	ok, _ := servers(t)
	p := openai.New(provider.LlamaCPP, ok.URL, 0)
	if h := p.Health(context.Background()); h.Status != provider.HealthHealthy {
		t.Fatalf("health = %+v", h)
	}
	infos, err := p.Models(context.Background())
	if err != nil {
		t.Fatalf("Models: %v", err)
	}
	if len(infos) != 2 || infos[0].Name != "local-model" {
		t.Fatalf("models = %+v", infos)
	}
	if infos[0].Locality != model.LocalityLocal {
		t.Fatalf("locality = %q, want local", infos[0].Locality)
	}
	if !infos[0].Capabilities.Chat.Present() {
		t.Fatalf("chat capability = %v", infos[0].Capabilities.Chat)
	}
}

func TestOpenAIModelsUnsupported(t *testing.T) {
	_, bad := servers(t)
	_, err := openai.New(provider.MLX, bad.URL, 0).Models(context.Background())
	if !errors.Is(err, provider.ErrDiscoveryUnsupported) {
		t.Fatalf("err = %v, want ErrDiscoveryUnsupported", err)
	}
}

func TestOpenAIHealthDegraded(t *testing.T) {
	_, bad := servers(t)
	if h := openai.New(provider.MLX, bad.URL, 0).Health(context.Background()); h.Status != provider.HealthDegraded {
		t.Fatalf("health = %+v, want degraded", h)
	}
}

func TestOpenAICapabilities(t *testing.T) {
	p := openai.New(provider.LlamaCPP, "http://127.0.0.1:1", 0)
	caps, err := p.Capabilities(context.Background(), "local-model")
	if err != nil {
		t.Fatalf("Capabilities: %v", err)
	}
	if !caps.Chat.Present() || !caps.Streaming.Present() {
		t.Fatalf("caps = %+v", caps)
	}
	if _, err := p.Capabilities(context.Background(), ""); err == nil {
		t.Fatal("an empty model must be rejected")
	}
}

func TestOpenAINoEndpoint(t *testing.T) {
	p := openai.New(provider.MLX, "", 0)
	if h := p.Health(context.Background()); h.Status != provider.HealthUnavailable {
		t.Fatalf("health = %+v", h)
	}
	if _, err := p.Models(context.Background()); !errors.Is(err, provider.ErrDiscoveryUnsupported) {
		t.Fatalf("err = %v", err)
	}
}

// TestOpenAIConfiguredModelFallback proves a single-model endpoint that cannot be
// enumerated reports its configured identity rather than claiming nothing is
// known — and that with no configured model the failure stays "unknown".
func TestOpenAIConfiguredModelFallback(t *testing.T) {
	_, bad := servers(t)

	infos, err := openai.New(provider.LlamaCPP, bad.URL, 0).WithConfiguredModel("only-model").Models(context.Background())
	if err != nil || len(infos) != 1 || infos[0].Name != "only-model" {
		t.Fatalf("configured fallback: infos=%+v err=%v", infos, err)
	}
	if infos[0].Provider != provider.LlamaCPP {
		t.Fatalf("provider = %q", infos[0].Provider)
	}

	if _, err := openai.New(provider.LlamaCPP, bad.URL, 0).Models(context.Background()); !errors.Is(err, provider.ErrDiscoveryUnsupported) {
		t.Fatalf("without a configured model, err = %v, want ErrDiscoveryUnsupported", err)
	}
}
