package ollamaagent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/redact"
	"github.com/imhttran/agentic-sop/internal/toolharness"
)

// fakeOMLX is a scripted OpenAI-compatible /v1/chat/completions server. It lets
// the governed oMLX path be tested end to end without a live model: the scripted
// turn is fed through the real OpenAI client, the real bounded tool loop, and the
// real controlled tool executor.
type fakeOMLX struct {
	mu        sync.Mutex
	responses []string
	requests  []openAIChatRequest
	auth      []string
	status    int
}

func newFakeOMLX(t *testing.T, responses ...string) (*fakeOMLX, *httptest.Server) {
	t.Helper()
	f := &fakeOMLX{responses: responses}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var req openAIChatRequest
		_ = json.Unmarshal(body, &req)

		f.mu.Lock()
		f.requests = append(f.requests, req)
		f.auth = append(f.auth, r.Header.Get("Authorization"))
		status := f.status
		content := `{"status":"completed","summary":"default","changes_expected":true}`
		if status == 0 || status == http.StatusOK {
			if len(f.responses) > 0 {
				content = f.responses[0]
				f.responses = f.responses[1:]
			}
		}
		f.mu.Unlock()

		if status != 0 && status != http.StatusOK {
			w.WriteHeader(status)
			_, _ = io.WriteString(w, `{"error":{"message":"scripted provider failure"}}`)
			return
		}
		var out openAIChatResponse
		out.Model = req.Model
		out.Choices = append(out.Choices, openAIChoice{})
		out.Choices[0].Message.Content = content
		out.Choices[0].FinishReason = "stop"
		_ = json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(srv.Close)
	return f, srv
}

func (f *fakeOMLX) request(i int) openAIChatRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests[i]
}

func (f *fakeOMLX) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

// omLXTestConfig is the shared harness configuration pointed at the fixture.
func omLXTestConfig(baseURL string) Config {
	cfg := testConfig(baseURL)
	cfg.Provider = "mlx"
	cfg.Model = "Qwen3-4B-4bit"
	cfg.MaxToolCalls = 50
	return cfg
}

// TestNativeOMLXAgentFromEnvResolvesModelPrecedence proves the oMLX agent does
// not inherit the Ollama default model: SOP_AGENT_MODEL wins, then SOP_MLX_MODEL,
// then the configured model passed by the composition root.
func TestNativeOMLXAgentFromEnvResolvesModelPrecedence(t *testing.T) {
	t.Setenv(agent.EnvAgentModel, "")
	t.Setenv(agent.EnvOllamaModel, "")
	t.Setenv(agent.EnvMLXModel, "mlx-model")
	t.Setenv(agent.EnvMLXAPIKey, "local-key")
	a, err := NativeOMLXAgentFromEnvWithPolicy("configured-model", false, redact.Policy{})
	if err != nil {
		t.Fatal(err)
	}
	if a.cfg.Model != "mlx-model" {
		t.Errorf("model = %q, want mlx-model", a.cfg.Model)
	}
	if a.cfg.APIKey != "local-key" {
		t.Errorf("api key = %q, want local-key", a.cfg.APIKey)
	}

	t.Setenv(agent.EnvMLXModel, "")
	a, err = NativeOMLXAgentFromEnvWithPolicy("configured-model", false, redact.Policy{})
	if err != nil {
		t.Fatal(err)
	}
	if a.cfg.Model != "configured-model" {
		t.Errorf("model = %q, want configured-model", a.cfg.Model)
	}

	t.Setenv(agent.EnvAgentModel, "agent-model")
	a, err = NativeOMLXAgentFromEnvWithPolicy("configured-model", false, redact.Policy{})
	if err != nil {
		t.Fatal(err)
	}
	if a.cfg.Model != "agent-model" {
		t.Errorf("model = %q, want agent-model", a.cfg.Model)
	}
}

// TestOMLXClientSendsConfiguredAPIKey proves the governed client uses the
// configured credential, so an authenticated local endpoint is not silently
// sent an anonymous request.
func TestOMLXClientSendsConfiguredAPIKey(t *testing.T) {
	dir := t.TempDir()
	f, srv := newFakeOMLX(t, `{"summary":"the plan","stages":[]}`)
	cfg := omLXTestConfig(srv.URL)
	cfg.APIKey = "local-secret"
	h := NewWithClient(cfg, dir, newOpenAIChatClient(cfg))

	if _, err := h.Execute(context.Background(), planRequest()); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.auth) != 1 || f.auth[0] != "Bearer local-secret" {
		t.Errorf("Authorization = %v, want Bearer local-secret", f.auth)
	}
}

// TestNativeOMLXAgentFromEnvParsesTimeoutAndThinking proves the operator
// settings are wired: SOP_MLX_TIMEOUT bounds calls and SOP_MLX_ENABLE_THINKING
// reaches the transport.
func TestNativeOMLXAgentFromEnvParsesTimeoutAndThinking(t *testing.T) {
	t.Setenv(agent.EnvMLXTimeout, "45s")
	t.Setenv(agent.EnvMLXEnableThinking, "true")
	t.Setenv(agent.EnvMLXModel, "m")
	a, err := NativeOMLXAgentFromEnvWithPolicy("configured", false, redact.Policy{})
	if err != nil {
		t.Fatal(err)
	}
	if a.cfg.Timeout != 45*time.Second {
		t.Errorf("timeout = %s, want 45s", a.cfg.Timeout)
	}
	if !a.cfg.EnableThinking {
		t.Error("EnableThinking = false, want true")
	}

	t.Setenv(agent.EnvMLXTimeout, "not-a-duration")
	if _, err := NativeOMLXAgentFromEnvWithPolicy("configured", false, redact.Policy{}); err == nil {
		t.Error("a malformed SOP_MLX_TIMEOUT must fail")
	}
	t.Setenv(agent.EnvMLXTimeout, "")
	t.Setenv(agent.EnvMLXEnableThinking, "maybe")
	if _, err := NativeOMLXAgentFromEnvWithPolicy("configured", false, redact.Policy{}); err == nil {
		t.Error("a malformed SOP_MLX_ENABLE_THINKING must fail")
	}

	// The text-only and governed paths must resolve an unset timeout to the same
	// default; otherwise the two MLX constructions would diverge.
	t.Setenv(agent.EnvMLXEnableThinking, "")
	defaultTimeout, err := agent.MLXTimeoutFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	a, err = NativeOMLXAgentFromEnvWithPolicy("configured", false, redact.Policy{})
	if err != nil {
		t.Fatal(err)
	}
	if a.cfg.Timeout != defaultTimeout {
		t.Errorf("timeout = %s, want the shared default %s", a.cfg.Timeout, defaultTimeout)
	}
}

// TestOllamaDiagnosticsKeepHistoricalWording proves the provider-aware
// diagnostics refactor did not change the default Ollama wording that existing
// callers and tests rely on.
func TestOllamaDiagnosticsKeepHistoricalWording(t *testing.T) {
	cfg := testConfig("http://127.0.0.1:1")
	h := NewWithClient(cfg, t.TempDir(), newOllamaClient(cfg))
	if got := h.agentName(); got != "Ollama agent" {
		t.Errorf("agentName = %q, want Ollama agent", got)
	}
	if got := h.providerName(); got != "ollama" {
		t.Errorf("providerName = %q, want ollama", got)
	}
	err := h.limitError(implementRequest(), policyFor(agent.Implement, cfg.EffectiveBudget()), "", "")
	if !strings.Contains(err.Error(), "the Ollama agent") {
		t.Errorf("limit error = %q, want the historical Ollama wording", err)
	}
}

// TestNativeOMLXAgentFromEnvSendsAPIKey proves the env-resolved credential is
// carried all the way to the HTTP request, not just stored on Config.
func TestNativeOMLXAgentFromEnvSendsAPIKey(t *testing.T) {
	f, srv := newFakeOMLX(t, `{"summary":"the plan","stages":[]}`)
	t.Setenv(agent.EnvAgentModel, "")
	t.Setenv(agent.EnvOllamaModel, "")
	t.Setenv(agent.EnvMLXBaseURL, srv.URL)
	t.Setenv(agent.EnvMLXModel, "Qwen3-4B-4bit")
	t.Setenv(agent.EnvMLXAPIKey, "env-key")
	a, err := NativeOMLXAgentFromEnvWithPolicy("configured", false, redact.Policy{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Generate(context.Background(), planRequest()); err != nil {
		t.Fatalf("plan through the env-built oMLX agent: %v", err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.auth) != 1 || f.auth[0] != "Bearer env-key" {
		t.Errorf("Authorization = %v, want Bearer env-key", f.auth)
	}
}

// TestOMLXClientRejectsMalformedAndErrorResponses covers the client's parse and
// provider-error paths directly, so a broken endpoint cannot be mistaken for a
// completed outcome.
func TestOMLXClientRejectsMalformedAndErrorResponses(t *testing.T) {
	cases := []struct {
		name    string
		status  int
		body    string
		wantMsg string
	}{
		{"malformed json", http.StatusOK, `not-json`, "parse response"},
		{"provider error object", http.StatusOK, `{"error":{"message":"bad model"}}`, "bad model"},
		{"empty response", http.StatusOK, `{"choices":[{"message":{"content":""},"finish_reason":"stop"}]}`, errEmptyResponse.Error()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer srv.Close()

			cfg := omLXTestConfig(srv.URL)
			h := NewWithClient(cfg, t.TempDir(), newOpenAIChatClient(cfg))
			_, err := h.Execute(context.Background(), planRequest())
			if err == nil || !strings.Contains(err.Error(), tc.wantMsg) {
				t.Fatalf("err = %v, want %q", err, tc.wantMsg)
			}
		})
	}
}

// TestOMLXClientRetriesTransientFailure proves the oMLX transport absorbs a
// transient 5xx at the provider boundary before the loop sees it.
func TestOMLXClientRetriesTransientFailure(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		if n == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(w, `{"error":{"message":"busy"}}`)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{
				"message":       map[string]any{"role": "assistant", "content": `{"summary":"the plan","stages":[]}`},
				"finish_reason": "stop",
			}},
		})
	}))
	defer srv.Close()

	cfg := omLXTestConfig(srv.URL)
	h := NewWithClient(cfg, t.TempDir(), newOpenAIChatClient(cfg))
	if _, err := h.Execute(context.Background(), planRequest()); err != nil {
		t.Fatalf("transient failure was not recovered: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 2 {
		t.Errorf("calls = %d, want 2 (one retry)", calls)
	}
}

// TestOMLXClientReportsTimeoutNamesProvider proves a timeout is typed and names
// the oMLX provider and its timeout setting.
func TestOMLXClientReportsTimeoutNamesProvider(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = io.WriteString(w, `{}`)
	}))
	defer srv.Close()

	cfg := omLXTestConfig(srv.URL)
	cfg.Timeout = 50 * time.Millisecond
	h := NewWithClient(cfg, t.TempDir(), newOpenAIChatClient(cfg))
	_, err := h.Execute(context.Background(), planRequest())
	var timeoutErr *ollamaTimeoutError
	if !errors.As(err, &timeoutErr) {
		t.Fatalf("err = %v, want *ollamaTimeoutError", err)
	}
	if timeoutErr.provider != "mlx" {
		t.Errorf("provider = %q, want mlx", timeoutErr.provider)
	}
	if timeoutErr.env != agent.EnvMLXTimeout {
		t.Errorf("timeout hint = %q, want %s", timeoutErr.env, agent.EnvMLXTimeout)
	}
}

// TestOMLXClientReportsTruncation proves a length-limited turn is typed and
// never accepted as a completed outcome.
func TestOMLXClientReportsTruncation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []any{map[string]any{
				"message":       map[string]any{"role": "assistant", "content": "partial"},
				"finish_reason": "length",
			}},
		})
	}))
	defer srv.Close()

	cfg := omLXTestConfig(srv.URL)
	h := NewWithClient(cfg, t.TempDir(), newOpenAIChatClient(cfg))
	_, err := h.Execute(context.Background(), planRequest())
	var trunc *agent.TruncationError
	if !errors.As(err, &trunc) {
		t.Fatalf("err = %v, want *agent.TruncationError", err)
	}
	if trunc.Provider != "mlx" {
		t.Errorf("provider = %q, want mlx", trunc.Provider)
	}
}

// TestOMLXTypedErrorsShareRetryClassification proves the oMLX transport returns
// the same typed errors the retry boundary understands, so a transient oMLX
// failure is retried and a deterministic one is not.
func TestOMLXTypedErrorsShareRetryClassification(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"5xx", &ollamaHTTPError{provider: "mlx", statusCode: http.StatusBadGateway}, true},
		{"429", &ollamaHTTPError{provider: "mlx", statusCode: http.StatusTooManyRequests}, true},
		{"400", &ollamaHTTPError{provider: "mlx", statusCode: http.StatusBadRequest}, false},
		{"timeout", &ollamaTimeoutError{provider: "mlx", timeout: time.Second, env: agent.EnvMLXTimeout}, true},
		{"transport", &ollamaTransportError{provider: "mlx", baseURL: "x", err: errors.New("reset")}, true},
		{"truncation", &agent.TruncationError{Provider: "mlx", Reason: "length"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := retryableProviderError(tc.err); got != tc.want {
				t.Errorf("retryable = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestOMLXGovernedImplementWritesFile is the complete deterministic path: a
// scripted provider succeeds, the loop parses its tool call, the existing
// controlled tool executor writes the file, edit tracking records the mutation,
// and the structured outcome is grounded in observed reality.
func TestOMLXGovernedImplementWritesFile(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)

	f, srv := newFakeOMLX(t,
		`{"tool":"write_file","args":{"path":"out.txt","content":"hello oMLX"}}`,
		`{"status":"completed","summary":"wrote out.txt","changes_expected":true}`,
	)
	cfg := omLXTestConfig(srv.URL)
	h := NewWithClient(cfg, dir, newOpenAIChatClient(cfg))
	ev := &mutationEvidence{}

	content, _, err := h.CompleteWithEvidence(context.Background(), implementRequest(), ev)
	if err != nil {
		t.Fatalf("CompleteWithEvidence failed: %v", err)
	}
	if !strings.Contains(content, `"status":"completed"`) {
		t.Fatalf("content = %q, want a completed outcome", content)
	}
	got, err := os.ReadFile(filepath.Join(dir, "out.txt"))
	if err != nil {
		t.Fatalf("the scripted tool call did not write the file: %v", err)
	}
	if string(got) != "hello oMLX" {
		t.Errorf("out.txt = %q, want %q", string(got), "hello oMLX")
	}
	if paths := ev.mutationPaths(); len(paths) != 1 || paths[0] != "out.txt" {
		t.Errorf("mutation evidence = %v, want [out.txt]", paths)
	}

	audit := h.AuditRecords()
	wrote := false
	for _, rec := range audit {
		if rec.Tool == toolharness.ToolWriteFile && rec.Action == toolharness.ActionAllow && rec.Outcome == toolharness.OutcomeOK {
			wrote = true
		}
	}
	if !wrote {
		t.Errorf("audit = %+v, want an allowed, successful write_file", audit)
	}

	req := f.request(0)
	if req.Model != "Qwen3-4B-4bit" {
		t.Errorf("request model = %q, want the exact oMLX model id", req.Model)
	}
	if req.ResponseFormat == nil || req.ResponseFormat.Type != "json_object" {
		t.Errorf("request response_format = %+v, want json_object", req.ResponseFormat)
	}
	if req.ChatTemplateKwargs == nil || req.ChatTemplateKwargs["enable_thinking"] != false {
		t.Errorf("request chat_template_kwargs = %+v, want enable_thinking=false", req.ChatTemplateKwargs)
	}
	if f.count() != 2 {
		t.Errorf("provider calls = %d, want 2 (tool call + outcome)", f.count())
	}
}

// TestOMLXAgentAdvertisesGovernedCapabilities proves the governed oMLX agent
// advertises IMPLEMENT/FIX while the unchanged text-only MLX transport does not.
func TestOMLXAgentAdvertisesGovernedCapabilities(t *testing.T) {
	cfg := omLXTestConfig("http://127.0.0.1:1")
	cfg.MaxOutputTokens = 64
	governed := NewNativeOMLXAgent(cfg, t.TempDir(), nil)
	if caps := governed.Capabilities(); !caps.Supports(agent.Implement) || !caps.Supports(agent.Fix) {
		t.Errorf("governed oMLX capabilities = %s, want IMPLEMENT and FIX", caps)
	}

	text, err := agent.NewMLX("http://127.0.0.1:1", "m", "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if caps := text.Capabilities(); caps.Supports(agent.Implement) || caps.Supports(agent.Fix) {
		t.Errorf("text-only MLX capabilities = %s, must not advertise IMPLEMENT/FIX", caps)
	}
}

// TestOMLXTextCapabilityStillWorks proves the opt-in path keeps the existing
// text-only capabilities working: a PLAN turn returns its document through the
// same loop.
func TestOMLXTextCapabilityStillWorks(t *testing.T) {
	dir := t.TempDir()
	plan := `{"summary":"the plan","stages":[]}`
	_, srv := newFakeOMLX(t, plan)
	cfg := omLXTestConfig(srv.URL)
	h := NewWithClient(cfg, dir, newOpenAIChatClient(cfg))

	content, err := h.Execute(context.Background(), planRequest())
	if err != nil {
		t.Fatalf("PLAN through the oMLX loop failed: %v", err)
	}
	if !strings.Contains(content, `"summary":"the plan"`) {
		t.Errorf("plan content = %q", content)
	}
}

// TestOMLXProviderFailureNamesProvider proves a provider failure is reported
// accurately: it names mlx, not Ollama, and never claims a completed outcome.
func TestOMLXProviderFailureNamesProvider(t *testing.T) {
	dir := t.TempDir()
	f, srv := newFakeOMLX(t)
	f.status = http.StatusBadGateway
	cfg := omLXTestConfig(srv.URL)
	h := NewWithClient(cfg, dir, newOpenAIChatClient(cfg))

	content, err := h.Execute(context.Background(), implementRequest())
	if err == nil {
		t.Fatalf("a provider failure must not return content: %q", content)
	}
	msg := err.Error()
	var httpErr *ollamaHTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("error = %T %q, want *ollamaHTTPError", err, msg)
	}
	if httpErr.provider != "mlx" {
		t.Errorf("error provider = %q, want mlx", httpErr.provider)
	}
	if httpErr.statusCode != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", httpErr.statusCode)
	}
	if strings.Contains(msg, "ollama") {
		t.Errorf("error = %q, must not attribute the failure to Ollama", msg)
	}
	if strings.Contains(content, `"status":"completed"`) {
		t.Errorf("content = %q, must not claim completion", content)
	}
}

// TestOMLXReusesToolPermissionChecks proves the oMLX path does not bypass the
// existing tool policy: a write to SOP state is denied and audited, and the file
// is never created.
func TestOMLXReusesToolPermissionChecks(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	_, srv := newFakeOMLX(t,
		`{"tool":"write_file","args":{"path":".agent-sdlc/state.db","content":"x"}}`,
	)
	cfg := omLXTestConfig(srv.URL)
	h := NewWithClient(cfg, dir, newOpenAIChatClient(cfg))

	_, _ = h.Execute(context.Background(), implementRequest())

	denied := false
	for _, rec := range h.AuditRecords() {
		if rec.Tool == toolharness.ToolWriteFile && rec.Action == toolharness.ActionDeny {
			denied = true
		}
	}
	if !denied {
		t.Errorf("audit = %+v, want the SOP-state write denied", h.AuditRecords())
	}
	if _, err := os.Stat(filepath.Join(dir, ".agent-sdlc", "state.db")); err == nil {
		t.Error("a denied write must not create the SOP state file")
	}
}

// TestOMLXClientClassifiesBodyReadTimeout proves a timeout after the response
// headers arrive is classified as the typed oMLX timeout (and remains
// retryable), not a generic read-response error.
func TestOMLXClientClassifiesBodyReadTimeout(t *testing.T) {
	headers := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		close(headers)
		time.Sleep(300 * time.Millisecond)
		_, _ = io.WriteString(w, `{"choices":[]}`)
	}))
	defer srv.Close()

	cfg := omLXTestConfig(srv.URL)
	cfg.Timeout = 80 * time.Millisecond
	client := newOpenAIChatClient(cfg).(*openAIChatClient)
	_, _, err := client.chatOnce(context.Background(), []chatMessage{{Role: "user", Content: "x"}})
	var timeoutErr *ollamaTimeoutError
	if !errors.As(err, &timeoutErr) {
		t.Fatalf("err = %v, want *ollamaTimeoutError after headers arrived", err)
	}
	if timeoutErr.provider != "mlx" {
		t.Errorf("provider = %q, want mlx", timeoutErr.provider)
	}
	if timeoutErr.env != agent.EnvMLXTimeout {
		t.Errorf("timeout hint = %q, want %s", timeoutErr.env, agent.EnvMLXTimeout)
	}
	if !retryableProviderError(err) {
		t.Errorf("body-read timeout must remain retryable")
	}
	select {
	case <-headers:
	default:
		t.Fatal("server did not send headers before the failure")
	}
}
