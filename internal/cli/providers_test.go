package cli

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/model"
	"github.com/imhttran/agentic-sop/internal/provider"
	"github.com/imhttran/agentic-sop/internal/provider/llamacpp"
	"github.com/imhttran/agentic-sop/internal/provider/mlx"
	"github.com/imhttran/agentic-sop/internal/provider/ollama"
)

// providerTestServer answers the provider probe endpoints: Ollama's
// /api/version, /api/tags, and /api/show, and the OpenAI-compatible /v1/models.
func providerTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/version":
			_, _ = w.Write([]byte(`{"version":"0.1"}`))
		case "/api/tags":
			_, _ = w.Write([]byte(`{"models":[{"name":"qwen3:4b"},{"name":"glm-5.3-flash:cloud"}]}`))
		case "/api/show":
			_, _ = w.Write([]byte(`{"capabilities":["completion","tools"]}`))
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"local-model"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// clearProviderEnv unsets the agent/provider/model environment so the developer's
// shell cannot leak into a provider resolution assertion.
func clearProviderEnv(t *testing.T) {
	t.Helper()
	clearModelEnv(t)
	for _, k := range []string{
		agent.EnvAgentProvider, agent.EnvAgentModel, agent.EnvAgentHarness,
		agent.EnvOllamaModel, agent.EnvLlamaCppModel,
		ollama.EnvBaseURL, llamacpp.EnvBaseURL, mlx.EnvBaseURL,
	} {
		if v, ok := os.LookupEnv(k); ok {
			t.Cleanup(func() { _ = os.Setenv(k, v) })
			_ = os.Unsetenv(k)
		}
	}
}

func TestProvidersCommandListsRuntimes(t *testing.T) {
	clearProviderEnv(t)
	srv := providerTestServer(t)
	t.Setenv(ollama.EnvBaseURL, srv.URL)
	t.Setenv(llamacpp.EnvBaseURL, srv.URL)
	t.Setenv(mlx.EnvBaseURL, srv.URL)

	dir := t.TempDir()
	code, out, errOut := runCLI(t, dir, "providers")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, errOut)
	}
	for _, want := range []string{"PROVIDER", "ollama", "healthy", "llamacpp", "mlx", "command", "unknown"} {
		if !strings.Contains(out, want) {
			t.Errorf("providers output missing %q:\n%s", want, out)
		}
	}
	// Inspection is read-only: it must not create SOP state.
	if stateExists(filepath.Join(dir, stateDirName, stateFileName)) {
		t.Error("sop providers created state")
	}
}

func TestProvidersCommandModels(t *testing.T) {
	clearProviderEnv(t)
	srv := providerTestServer(t)
	t.Setenv(ollama.EnvBaseURL, srv.URL)
	t.Setenv(llamacpp.EnvBaseURL, srv.URL)
	t.Setenv(mlx.EnvBaseURL, srv.URL)

	code, out, errOut := runCLI(t, t.TempDir(), "providers", "--models")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, errOut)
	}
	for _, want := range []string{"qwen3:4b", "glm-5.3-flash:cloud", "local-model", "capabilities:"} {
		if !strings.Contains(out, want) {
			t.Errorf("providers --models output missing %q:\n%s", want, out)
		}
	}
}

func TestProvidersCommandUnknownFlag(t *testing.T) {
	if code, _, _ := runCLI(t, t.TempDir(), "providers", "--bogus"); code != exitUsage {
		t.Fatalf("code = %d, want usage error", code)
	}
}

func TestProviderEndpointsPrecedence(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv(ollama.EnvBaseURL, "http://env:1")
	cfg := config.Default()
	cfg.Providers.Ollama.Endpoint = "http://config:1"
	if got := endpointOr(cfg.Providers.Ollama.Endpoint, ollama.EnvBaseURL, ollama.DefaultBaseURL); got != "http://env:1" {
		t.Fatalf("env should win: %q", got)
	}
	_ = os.Unsetenv(ollama.EnvBaseURL)
	if got := endpointOr(cfg.Providers.Ollama.Endpoint, ollama.EnvBaseURL, ollama.DefaultBaseURL); got != "http://config:1" {
		t.Fatalf("config should win when env unset: %q", got)
	}
	if got := endpointOr("", ollama.EnvBaseURL, ollama.DefaultBaseURL); got != ollama.DefaultBaseURL {
		t.Fatalf("default fallback = %q", got)
	}
}

func TestValidateSelectedModelDisabledIsNoOp(t *testing.T) {
	clearProviderEnv(t)
	// A dead endpoint must not be probed when validation is off (the default).
	t.Setenv(ollama.EnvBaseURL, "http://127.0.0.1:1")
	cfg := config.Default()
	cfg.Agent.Provider = "ollama"
	cfg.Agent.Model = "qwen3:4b"
	if err := validateSelectedModel(context.Background(), cfg, model.Result{}); err != nil {
		t.Fatalf("disabled validation must be a no-op: %v", err)
	}
}

func TestValidateSelectedModelEnabledUnreachableFails(t *testing.T) {
	clearProviderEnv(t)
	dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := dead.URL
	dead.Close()
	t.Setenv(ollama.EnvBaseURL, url)

	yes := true
	cfg := config.Default()
	cfg.Agent.Provider = "ollama"
	cfg.Agent.Model = "qwen3:4b"
	cfg.Providers.Validate = &yes
	err := validateSelectedModel(context.Background(), cfg, model.Result{})
	if !errors.Is(err, provider.ErrProviderUnavailable) {
		t.Fatalf("err = %v, want ErrProviderUnavailable", err)
	}
}

func TestValidateSelectedModelEnabledHealthyPasses(t *testing.T) {
	clearProviderEnv(t)
	srv := providerTestServer(t)
	t.Setenv(ollama.EnvBaseURL, srv.URL)

	yes := true
	cfg := config.Default()
	cfg.Agent.Provider = "ollama"
	cfg.Agent.Model = "qwen3:4b"
	cfg.Providers.Validate = &yes
	if err := validateSelectedModel(context.Background(), cfg, model.Result{}); err != nil {
		t.Fatalf("healthy selection must pass: %v", err)
	}
}

func TestValidateSelectedModelEnabledAbsentModelFails(t *testing.T) {
	clearProviderEnv(t)
	srv := providerTestServer(t)
	t.Setenv(ollama.EnvBaseURL, srv.URL)

	yes := true
	cfg := config.Default()
	cfg.Agent.Provider = "ollama"
	cfg.Agent.Model = "not-installed:1b"
	cfg.Providers.Validate = &yes
	err := validateSelectedModel(context.Background(), cfg, model.Result{})
	if !errors.Is(err, provider.ErrModelNotFound) {
		t.Fatalf("err = %v, want ErrModelNotFound", err)
	}
}

func TestValidateSelectedModelSkipsCommandProvider(t *testing.T) {
	clearProviderEnv(t)
	yes := true
	cfg := config.Default()
	cfg.Agent.Provider = "command"
	cfg.Agent.Model = ""
	cfg.Providers.Validate = &yes
	if err := validateSelectedModel(context.Background(), cfg, model.Result{}); err != nil {
		t.Fatalf("the command provider must be skipped: %v", err)
	}
}

func TestValidateSelectedModelUsesRoutedSelection(t *testing.T) {
	clearProviderEnv(t)
	srv := providerTestServer(t)
	t.Setenv(ollama.EnvBaseURL, srv.URL)

	yes := true
	cfg := config.Default()
	cfg.Providers.Validate = &yes
	routing := model.Result{Active: true, Selection: model.Selection{Provider: "ollama", Model: "not-installed:1b"}}
	err := validateSelectedModel(context.Background(), cfg, routing)
	if !errors.Is(err, provider.ErrModelNotFound) {
		t.Fatalf("err = %v, want ErrModelNotFound for the routed model", err)
	}
}
