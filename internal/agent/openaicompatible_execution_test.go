package agent

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// clearOpenAICompatibleExecutionEnv unsets every variable the generic provider's
// execution path reads, so a developer's shell cannot leak into a wire assertion.
func clearOpenAICompatibleExecutionEnv(t *testing.T) {
	t.Helper()
	clearAgentEnv(t)
	for _, k := range []string{
		EnvOpenAICompatibleBaseURL,
		EnvOpenAICompatibleModel,
		EnvOpenAICompatibleTimeout,
		EnvOpenAICompatibleAPIKey,
	} {
		t.Setenv(k, "")
	}
}

// captureOpenAICompatibleServer serves POST /v1/chat/completions, records the
// request, and returns a minimal OpenAI-compatible response. It is the shared
// fixture for the execution-path tests; no external model service is required.
type capturedOpenAIRequest struct {
	method string
	path   string
	auth   string
	body   chatRequest
}

func captureOpenAICompatibleServer(t *testing.T, content string) (*httptest.Server, *capturedOpenAIRequest) {
	t.Helper()
	got := &capturedOpenAIRequest{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method = r.Method
		got.path = r.URL.Path
		got.auth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&got.body)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"` + content + `"}}]}`))
	}))
	t.Cleanup(srv.Close)
	return srv, got
}

// TestOpenAICompatibleExecutionUsesSharedTransport proves the generic provider
// executes through the same shared OpenAI-compatible transport as the specialized
// adapters: POST /v1/chat/completions with stream:false, selected by the endpoint
// resolved from the environment.
func TestOpenAICompatibleExecutionUsesSharedTransport(t *testing.T) {
	clearOpenAICompatibleExecutionEnv(t)
	srv, got := captureOpenAICompatibleServer(t, "hello")
	t.Setenv(EnvOpenAICompatibleBaseURL, srv.URL)
	t.Setenv(EnvOpenAICompatibleModel, "mlx-community/Qwen3-4B-4bit")

	// FromConfig must build the generic provider rather than reporting it as
	// not configured.
	a, err := FromConfig(ProviderOpenAICompatible, "")
	if err != nil {
		t.Fatalf("FromConfig: %v", err)
	}

	// A basic completion flows through the normal agent path.
	resp, err := a.Generate(context.Background(), validRequest(Plan))
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Content != "hello" {
		t.Errorf("content = %q, want hello", resp.Content)
	}

	// The endpoint is the configured one, at the chat-completions path.
	if got.method != http.MethodPost {
		t.Errorf("method = %q, want POST", got.method)
	}
	if got.path != "/v1/chat/completions" {
		t.Errorf("path = %q, want /v1/chat/completions", got.path)
	}
	// The model name is sent unchanged; the wire shape is the shared one.
	if got.body.Model != "mlx-community/Qwen3-4B-4bit" {
		t.Errorf("model = %q, want it unchanged", got.body.Model)
	}
	if got.body.Stream {
		t.Error("stream must be false on the shared transport")
	}
}

// TestOpenAICompatibleExecutionEndpointFromEnv proves the execution endpoint
// follows SOP_OPENAI_COMPATIBLE_BASE_URL, and never SOP_MLX_BASE_URL etc.
func TestOpenAICompatibleExecutionEndpointFromEnv(t *testing.T) {
	clearOpenAICompatibleExecutionEnv(t)
	srv, got := captureOpenAICompatibleServer(t, "ok")
	t.Setenv(EnvOpenAICompatibleBaseURL, srv.URL)
	// A decoy: the generic provider must not read another provider's endpoint.
	t.Setenv(EnvMLXBaseURL, "http://127.0.0.1:1")
	t.Setenv(EnvLlamaCppBaseURL, "http://127.0.0.1:1")

	a, err := NewOpenAICompatibleExecution(OpenAICompatibleEndpointFromEnv(), "m")
	if err != nil {
		t.Fatalf("NewOpenAICompatibleExecution: %v", err)
	}
	if _, err := a.Generate(context.Background(), validRequest(Review)); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if got.path != "/v1/chat/completions" {
		t.Fatalf("path = %q, want the generic provider's endpoint to be used", got.path)
	}
}

// TestOpenAICompatibleExecutionNoAuthRequired proves a local endpoint works with
// no credential configured.
func TestOpenAICompatibleExecutionNoAuthRequired(t *testing.T) {
	clearOpenAICompatibleExecutionEnv(t)
	srv, got := captureOpenAICompatibleServer(t, "ok")

	a, err := NewOpenAICompatibleExecution(srv.URL, "m")
	if err != nil {
		t.Fatalf("NewOpenAICompatibleExecution: %v", err)
	}
	if _, err := a.Generate(context.Background(), validRequest(Plan)); err != nil {
		t.Fatalf("Generate without a key: %v", err)
	}
	if got.auth != "" {
		t.Errorf("Authorization = %q, want empty for an unauthenticated local endpoint", got.auth)
	}
}

// TestOpenAICompatibleExecutionReusesAuthorization proves a configured key is
// carried on the existing Authorization header, without the key leaking into the
// request body or the identity.
func TestOpenAICompatibleExecutionReusesAuthorization(t *testing.T) {
	clearOpenAICompatibleExecutionEnv(t)
	srv, got := captureOpenAICompatibleServer(t, "ok")
	t.Setenv(EnvOpenAICompatibleAPIKey, "s3cret")

	a, err := NewOpenAICompatibleExecution(srv.URL, "m")
	if err != nil {
		t.Fatalf("NewOpenAICompatibleExecution: %v", err)
	}
	if _, err := a.Generate(context.Background(), validRequest(Plan)); err != nil {
		t.Fatalf("Generate with a key: %v", err)
	}
	if got.auth != "Bearer s3cret" {
		t.Errorf("Authorization = %q, want the reused bearer mechanism", got.auth)
	}
	oc, ok := a.(*OpenAICompatible)
	if !ok || oc.id != ProviderOpenAICompatible {
		t.Errorf("identity = %v, want %q", a, ProviderOpenAICompatible)
	}
}

// TestOpenAICompatibleExecutionCapabilityCeiling proves the generic provider
// claims exactly the shared transport's capability ceiling: the text
// capabilities only, never IMPLEMENT/FIX it cannot perform. It is not silently
// downgraded to text — it never claims tool execution it does not have.
func TestOpenAICompatibleExecutionCapabilityCeiling(t *testing.T) {
	clearOpenAICompatibleExecutionEnv(t)
	a, err := NewOpenAICompatibleExecution("http://127.0.0.1:8000", "m")
	if err != nil {
		t.Fatalf("NewOpenAICompatibleExecution: %v", err)
	}
	caps := CapabilitiesOf(a)
	for _, cap := range []Capability{Plan, DesignTests, DiagnoseFailure, Review} {
		if !caps.Supports(cap) {
			t.Errorf("the generic provider must support %s", cap)
		}
	}
	for _, cap := range []Capability{Implement, Fix} {
		if caps.Supports(cap) {
			t.Errorf("the text-only shared transport must not declare %s", cap)
		}
	}

	// The checked wrapper rejects a mutating capability rather than forwarding it.
	srv, _ := captureOpenAICompatibleServer(t, "ok")
	textOnly, err := NewOpenAICompatibleExecution(srv.URL, "m")
	if err != nil {
		t.Fatalf("NewOpenAICompatibleExecution: %v", err)
	}
	if _, err := NewChecked(textOnly).Generate(context.Background(), validRequest(Implement)); err == nil {
		t.Fatal("IMPLEMENT must be rejected for a text-only transport")
	}
}

// TestOpenAICompatibleExecutionDefaultEndpoint proves a blank endpoint falls back
// to the built-in default rather than failing.
func TestOpenAICompatibleExecutionDefaultEndpoint(t *testing.T) {
	clearOpenAICompatibleExecutionEnv(t)
	a, err := NewOpenAICompatibleExecution("   ", "m")
	if err != nil {
		t.Fatalf("NewOpenAICompatibleExecution: %v", err)
	}
	oc, ok := a.(*OpenAICompatible)
	if !ok {
		t.Fatalf("got %T, want *OpenAICompatible", a)
	}
	if oc.baseURL != defaultOpenAICompatibleBaseURL {
		t.Fatalf("baseURL = %q, want default %q", oc.baseURL, defaultOpenAICompatibleBaseURL)
	}
}

// TestOpenAICompatibleExecutionDoesNotAffectOtherProviders is the regression
// guard: building the generic provider leaves the specialized identities on the
// same shared transport, unchanged.
func TestOpenAICompatibleExecutionDoesNotAffectOtherProviders(t *testing.T) {
	clearOpenAICompatibleExecutionEnv(t)
	a, err := NewOpenAICompatibleExecution("http://127.0.0.1:8000", "m")
	if err != nil {
		t.Fatalf("NewOpenAICompatibleExecution: %v", err)
	}
	oc, ok := a.(*OpenAICompatible)
	if !ok || oc.id != ProviderOpenAICompatible {
		t.Fatalf("identity = %v, want %q", a, ProviderOpenAICompatible)
	}

	mlxAgent, err := NewMLX("http://127.0.0.1:8000", "m", "", 0)
	if err != nil {
		t.Fatalf("NewMLX: %v", err)
	}
	if mlxAgent.id != ProviderMLX {
		t.Fatalf("mlx identity = %q, want %q", mlxAgent.id, ProviderMLX)
	}
	llama, err := NewLlamaCpp("http://127.0.0.1:8080", "local", "", 0)
	if err != nil {
		t.Fatalf("NewLlamaCpp: %v", err)
	}
	if llama.id != ProviderLlamaCpp {
		t.Fatalf("llamacpp identity = %q, want %q", llama.id, ProviderLlamaCpp)
	}
}
