package cli

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/model"
)

// tierCapture records what one OpenAI-compatible server actually received.
type tierCapture struct {
	mu    sync.Mutex
	path  string
	model string
	auth  string
}

func (c *tierCapture) record(r *http.Request) {
	var body struct {
		Model string `json:"model"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.path = r.URL.Path
	c.model = body.Model
	c.auth = r.Header.Get("Authorization")
}

func (c *tierCapture) snapshot() (path, modelName, auth string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.path, c.model, c.auth
}

// tierChatServer answers the chat-completions path and records the request.
func tierChatServer(t *testing.T, capture *tierCapture) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/v1/chat/completions"):
			capture.record(r)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
		case strings.HasSuffix(r.URL.Path, "/v1/models"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":"model"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestOpenAICompatiblePerTierRequestPathAndAuth runs two tiers in one process and
// proves each agent sends its request to the endpoint for the model it selected,
// with the credential scoped to that destination.
func TestOpenAICompatiblePerTierRequestPathAndAuth(t *testing.T) {
	clearModelEnv(t)
	for _, k := range []string{agent.EnvAgentProvider, agent.EnvAgentHarness, agent.EnvOpenAICompatibleAPIKey,
		agent.EnvOpenAICompatibleTimeout, config.EnvOpenAICompatibleBaseURL} {
		t.Setenv(k, "")
	}

	localCapture, cloudCapture := &tierCapture{}, &tierCapture{}
	local := tierChatServer(t, localCapture)
	cloud := tierChatServer(t, cloudCapture)

	// The acceptance example: SMALL on local oMLX, MEDIUM/LARGE on OpenRouter. The
	// cloud test server stands in for OpenRouter; only its origin matters here.
	t.Setenv(config.EnvOpenAICompatibleBaseURL, cloud.URL+"/api")
	t.Setenv(agent.EnvOpenAICompatibleAPIKey, "shared-openrouter-key")

	t.Setenv(model.ClassEnvKey(model.ClassSmall, "PROVIDER"), agent.ProviderOpenAICompatible)
	t.Setenv(model.ClassEnvKey(model.ClassSmall, "NAME"), "Qwen3-4B-4bit")
	t.Setenv(model.ClassEnvKey(model.ClassSmall, "BASE_URL"), local.URL)
	t.Setenv(model.ClassEnvKey(model.ClassSmall, "LOCALITY"), "local")

	t.Setenv(model.ClassEnvKey(model.ClassMedium, "PROVIDER"), agent.ProviderOpenAICompatible)
	t.Setenv(model.ClassEnvKey(model.ClassMedium, "NAME"), "openrouter-medium-model-id")

	t.Chdir(t.TempDir())
	d := defaultDeps()

	type tier struct {
		class    string
		model    string
		capture  *tierCapture
		auth     string
		wantPath string
	}
	rows := []tier{
		{class: "small", model: "Qwen3-4B-4bit", capture: localCapture, auth: "", wantPath: "/v1/chat/completions"},
		{class: "medium", model: "openrouter-medium-model-id", capture: cloudCapture, auth: "Bearer shared-openrouter-key", wantPath: "/api/v1/chat/completions"},
	}
	for _, row := range rows {
		a, err := d.newAgent(agent.HarnessTool, agent.ProviderOpenAICompatible, row.model, row.class)
		if err != nil {
			t.Fatalf("%s: build agent: %v", row.class, err)
		}
		resp, err := a.Generate(context.Background(), agent.Request{Capability: agent.Review, Task: "ping"})
		if err != nil {
			t.Fatalf("%s: generate: %v", row.class, err)
		}
		if resp.Content == "" {
			t.Fatalf("%s: empty response", row.class)
		}
		path, gotModel, gotAuth := row.capture.snapshot()
		if path != row.wantPath {
			t.Errorf("%s request path = %q, want %q", row.class, path, row.wantPath)
		}
		if gotModel != row.model {
			t.Errorf("%s model sent = %q, want %q", row.class, gotModel, row.model)
		}
		if gotAuth != row.auth {
			t.Errorf("%s Authorization = %q, want %q", row.class, gotAuth, row.auth)
		}
	}
}

// TestModelInputsInfersOpenRouterCloud proves the CLI's routing inputs feed the
// effective endpoint into locality resolution: the exact OpenRouter origin infers
// cloud, and a lookalike host supplies no evidence and fails closed.
func TestModelInputsInfersOpenRouterCloud(t *testing.T) {
	clearModelEnv(t)
	t.Setenv(model.ClassEnvKey(model.ClassMedium, "PROVIDER"), agent.ProviderOpenAICompatible)
	t.Setenv(model.ClassEnvKey(model.ClassMedium, "NAME"), "openrouter-medium-model-id")
	cfg := config.Default()

	t.Setenv(config.EnvOpenAICompatibleBaseURL, "https://openrouter.ai/api")
	res, err := resolveModelLocality(context.Background(), modelInputs(cfg, "medium"), nil)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got, want := res.Selection.Locality, model.LocalityCloud; got != want {
		t.Errorf("locality = %q, want %q", got, want)
	}
	if got, want := res.Selection.LocalitySource, model.LocalitySourceInferred; got != want {
		t.Errorf("locality source = %q, want %q", got, want)
	}

	t.Setenv(config.EnvOpenAICompatibleBaseURL, "https://openrouter.ai.example.com/api")
	_, err = resolveModelLocality(context.Background(), modelInputs(cfg, "medium"), nil)
	var unknown *model.LocalityUnknownError
	if !errors.As(err, &unknown) {
		t.Fatalf("lookalike host: err = %v, want *model.LocalityUnknownError", err)
	}
}

// TestOpenAICompatibleRouteForClassScopesKey proves the composition-root resolver
// used by the agent factory returns the tier endpoint and a destination-scoped
// key.
func TestOpenAICompatibleRouteForClassScopesKey(t *testing.T) {
	clearModelEnv(t)
	t.Setenv(config.EnvOpenAICompatibleBaseURL, "https://openrouter.ai/api")
	t.Setenv(agent.EnvOpenAICompatibleAPIKey, "shared-key")
	t.Setenv(model.ClassEnvKey(model.ClassSmall, "BASE_URL"), "http://127.0.0.1:8000")

	dir := t.TempDir()
	if endpoint, key := openAICompatibleRouteFor(dir, "small"); endpoint != "http://127.0.0.1:8000" || key != "" {
		t.Errorf("small route = (%q, %q), want local endpoint and no key", endpoint, key)
	}
	if endpoint, key := openAICompatibleRouteFor(dir, "medium"); endpoint != "https://openrouter.ai/api" || key != "shared-key" {
		t.Errorf("medium route = (%q, %q), want shared endpoint and key", endpoint, key)
	}
}
