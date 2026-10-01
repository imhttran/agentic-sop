package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/model"
	"github.com/imhttran/agentic-sop/internal/provider"
	"github.com/imhttran/agentic-sop/internal/provider/llamacpp"
	"github.com/imhttran/agentic-sop/internal/provider/mlx"
	"github.com/imhttran/agentic-sop/internal/provider/ollama"
)

// clearAgentEnv unsets the provider-selection environment so the developer's shell
// cannot leak into a resolution assertion.
func clearAgentEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		EnvAgentProvider, EnvAgentModel, EnvAgentHarness, EnvAgentCommand,
		EnvOllamaBaseURL, EnvOllamaModel, EnvOllamaTimeout,
		EnvLlamaCppBaseURL, EnvLlamaCppModel, EnvLlamaCppTimeout, EnvLlamaCppAPIKey,
		EnvMLXBaseURL, EnvMLXModel, EnvMLXTimeout, EnvMLXAPIKey,
	} {
		t.Setenv(k, "")
	}
}

// mlxTestServer serves the OpenAI-compatible surface an MLX / oMLX runtime
// exposes: GET /v1/models for discovery and POST /v1/chat/completions for
// generation. No external model service is required.
func mlxTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"mlx-community/Qwen3-4B-4bit"}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/chat/completions":
			_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"mlx output"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestMLXDiscoverValidateGenerate proves an MLX/oMLX model can be discovered,
// validated, and generated against — all through local HTTP, offline.
func TestMLXDiscoverValidateGenerate(t *testing.T) {
	clearAgentEnv(t)
	srv := mlxTestServer(t)
	ctx := context.Background()
	const mlxModel = "mlx-community/Qwen3-4B-4bit"

	// Discover + validate through the provider layer.
	reg := provider.NewRegistry()
	if err := reg.Register(mlx.New(srv.URL, time.Second)); err != nil {
		t.Fatalf("register mlx: %v", err)
	}
	p, err := reg.Get(provider.MLX)
	if err != nil {
		t.Fatalf("get mlx: %v", err)
	}
	models, err := p.Models(ctx)
	if err != nil {
		t.Fatalf("models: %v", err)
	}
	if len(models) != 1 || models[0].Name != mlxModel {
		t.Fatalf("models = %+v, want [%s]", models, mlxModel)
	}
	if err := provider.ValidateSelection(ctx, reg, model.Selection{Provider: string(provider.MLX), Model: mlxModel}); err != nil {
		t.Fatalf("validate mlx selection: %v", err)
	}

	// Generate through the agent (execution) layer.
	a, err := NewMLX(srv.URL, mlxModel, "", time.Second)
	if err != nil {
		t.Fatalf("NewMLX: %v", err)
	}
	resp, err := a.Generate(ctx, validRequest(Plan))
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Content != "mlx output" {
		t.Errorf("content = %q, want %q", resp.Content, "mlx output")
	}
}

// TestMLXCannotImplement proves the MLX transport is honest about its capability
// ceiling: it is a text-only provider, so IMPLEMENT is not declared (and the
// checked wrapper rejects it) rather than being silently forwarded.
func TestMLXCannotImplement(t *testing.T) {
	clearAgentEnv(t)
	srv := mlxTestServer(t)
	a, err := NewMLX(srv.URL, "m", "", time.Second)
	if err != nil {
		t.Fatalf("NewMLX: %v", err)
	}
	if CapabilitiesOf(a).Supports(Implement) {
		t.Fatal("an MLX text-only transport must not declare IMPLEMENT")
	}
	if _, err := NewChecked(a).Generate(context.Background(), validRequest(Implement)); err == nil {
		t.Fatal("IMPLEMENT must be rejected for a text-only MLX transport")
	}
}

// TestNewMLXValidation rejects the misconfigurations that must fail clearly.
func TestNewMLXValidation(t *testing.T) {
	clearAgentEnv(t)
	if _, err := NewMLX("", "m", "", time.Second); err == nil {
		t.Error("empty base URL must fail")
	}
	if _, err := NewMLX("http://x", "", "", time.Second); err == nil {
		t.Error("empty model must fail")
	}
}

// TestMLXFromEnv proves the environment constructor honors the MLX variables and
// requires a model.
func TestMLXFromEnv(t *testing.T) {
	clearAgentEnv(t)
	srv := mlxTestServer(t)
	t.Setenv(EnvMLXBaseURL, srv.URL)
	t.Setenv(EnvMLXModel, "env-model")

	a, err := NewMLXFromEnv("")
	if err != nil {
		t.Fatalf("NewMLXFromEnv: %v", err)
	}
	m, ok := a.(*MLX)
	if !ok {
		t.Fatalf("got %T, want *MLX", a)
	}
	if m.baseURL != srv.URL || m.model != "env-model" {
		t.Errorf("resolved (%q, %q), want (%q, env-model)", m.baseURL, m.model, srv.URL)
	}

	t.Setenv(EnvMLXModel, "")
	t.Setenv(EnvAgentModel, "")
	if _, err := NewMLXFromEnv(""); err == nil {
		t.Error("a missing model must fail clearly")
	}
}

// TestFromConfigMLX proves MLX is a first-class execution provider in the
// provider-selection helpers.
func TestFromConfigMLX(t *testing.T) {
	clearAgentEnv(t)
	srv := mlxTestServer(t)
	t.Setenv(EnvMLXBaseURL, srv.URL)
	t.Setenv(EnvMLXModel, "mlx-model")

	a, err := FromConfig(ProviderMLX, "")
	if err != nil {
		t.Fatalf("FromConfig: %v", err)
	}
	if _, ok := a.(*MLX); !ok {
		t.Fatalf("got %T, want *MLX", a)
	}

	h, err := HarnessFromConfig(HarnessTool, ProviderMLX, "mlx-model")
	if err != nil {
		t.Fatalf("HarnessFromConfig: %v", err)
	}
	if _, err := h.Execute(context.Background(), validRequest(Review)); err != nil {
		t.Fatalf("tool harness with MLX provider: %v", err)
	}
}

// TestFromConfigUnknownProviderNamesEveryProvider proves the unknown-provider
// error names the canonical provider set, including MLX.
func TestFromConfigUnknownProviderNamesEveryProvider(t *testing.T) {
	clearAgentEnv(t)
	_, err := FromConfig("skynet", "")
	if err == nil {
		t.Fatal("expected an error for an unknown provider")
	}
	for _, want := range []string{"command", "llamacpp", "ollama", "mlx"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing provider %q", err, want)
		}
	}
}

// TestDefaultEndpointsMatchProviderAdapters proves each execution provider's
// built-in endpoint is the same one the provider/runtime adapter probes, so the
// agent path and the inspection path cannot diverge.
func TestDefaultEndpointsMatchProviderAdapters(t *testing.T) {
	pairs := []struct {
		what    string
		agent   string
		adapter string
	}{
		{"ollama", defaultOllamaBaseURL, ollama.DefaultBaseURL},
		{"llamacpp", defaultLlamaCppBaseURL, llamacpp.DefaultBaseURL},
		{"mlx", defaultMLXBaseURL, mlx.DefaultBaseURL},
	}
	for _, p := range pairs {
		if p.agent != p.adapter {
			t.Errorf("%s default endpoint: agent %q != adapter %q", p.what, p.agent, p.adapter)
		}
	}
}

// TestMLXUsesSharedTransport checks the MLX identity uses the same
// OpenAI-compatible wire shape as llama.cpp (model + stream:false), i.e. the two
// providers genuinely share the transport rather than duplicating it.
func TestMLXUsesSharedTransport(t *testing.T) {
	clearAgentEnv(t)
	var got chatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	t.Cleanup(srv.Close)

	a, err := NewMLX(srv.URL, "mlx-model", "", time.Second)
	if err != nil {
		t.Fatalf("NewMLX: %v", err)
	}
	if _, err := a.Generate(context.Background(), validRequest(Review)); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got.Model != "mlx-model" || got.Stream {
		t.Errorf("wire body = %+v, want model mlx-model and stream false", got)
	}
	if a.id != ProviderMLX {
		t.Errorf("identity = %q, want %q", a.id, ProviderMLX)
	}
}
