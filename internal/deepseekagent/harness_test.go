package deepseekagent

import (
	"bytes"
	"context"
	"encoding/json"
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
)

// fakeOllama is a scripted /api/chat server: each call returns the next scripted
// content, so the tool loop can be tested without a live model.
type fakeOllama struct {
	mu        sync.Mutex
	responses []string
	requests  []ollamaChatRequest
	status    int
}

func newFakeOllama(t *testing.T, responses ...string) (*fakeOllama, *httptest.Server) {
	t.Helper()
	f := &fakeOllama{responses: responses}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var req ollamaChatRequest
		_ = json.Unmarshal(body, &req)

		f.mu.Lock()
		f.requests = append(f.requests, req)
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
			_, _ = io.WriteString(w, `{"error":"server error"}`)
			return
		}
		_ = json.NewEncoder(w).Encode(ollamaChatResponse{Message: chatMessage{Role: "assistant", Content: content}})
	}))
	t.Cleanup(srv.Close)
	return f, srv
}

func (f *fakeOllama) request(i int) ollamaChatRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests[i]
}

func (f *fakeOllama) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

// messageText concatenates all message content of a recorded request, for
// asserting what the model was shown.
func messageText(req ollamaChatRequest) string {
	var b strings.Builder
	for _, m := range req.Messages {
		b.WriteString(m.Content)
		b.WriteString("\n")
	}
	return b.String()
}

func testConfig(baseURL string) Config {
	return Config{
		BaseURL:        baseURL,
		Model:          DefaultModel,
		Timeout:        5 * time.Second,
		MaxIterations:  6,
		MaxToolCalls:   4,
		CommandTimeout: 10 * time.Second,
		MaxOutputBytes: 1 << 20,
	}
}

func implementRequest() agent.Request {
	return agent.Request{Capability: agent.Implement, Task: "do it", Input: "context", OutputRequirements: "summarize"}
}

// --- Run: the stdin contract ---

func TestRunParsesRequestAndWritesOutcome(t *testing.T) {
	dir := t.TempDir()
	_, srv := newFakeOllama(t, `{"status":"completed","summary":"did it","changes_expected":true}`)
	t.Setenv(agent.EnvOllamaBaseURL, srv.URL)
	t.Setenv(agent.EnvOllamaModel, "")
	t.Setenv(agent.EnvOllamaTimeout, "")

	body := `{"capability":"IMPLEMENT","task":"do it","input":"ctx","output_requirements":"out"}`
	var out, errOut bytes.Buffer
	if err := Run(context.Background(), strings.NewReader(body), &out, &errOut, func() (string, error) { return dir, nil }); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if !strings.Contains(out.String(), `"status":"completed"`) || !strings.Contains(out.String(), `"changes_expected":true`) {
		t.Errorf("out = %q", out.String())
	}
}

func TestRunRejectsInvalidRequest(t *testing.T) {
	var out, errOut bytes.Buffer
	err := Run(context.Background(), strings.NewReader("not json"), &out, &errOut, os.Getwd)
	if err == nil || !strings.Contains(err.Error(), "parse request") {
		t.Fatalf("err = %v, want a request-parse error", err)
	}
}

func TestRunRejectsUnknownCapability(t *testing.T) {
	body := `{"capability":"NONSENSE","task":"x"}`
	var out, errOut bytes.Buffer
	err := Run(context.Background(), strings.NewReader(body), &out, &errOut, os.Getwd)
	if err == nil || !strings.Contains(err.Error(), "capability") {
		t.Fatalf("err = %v, want a capability error", err)
	}
}

// --- Ollama request shape ---

func TestChatSendsModelJSONFormatAndNoStream(t *testing.T) {
	dir := t.TempDir()
	fake, srv := newFakeOllama(t, `{"status":"completed","summary":"ok","changes_expected":true}`)

	if _, err := New(testConfig(srv.URL), dir).Execute(context.Background(), implementRequest()); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	req := fake.request(0)
	if req.Model != DefaultModel {
		t.Errorf("model = %q, want %q", req.Model, DefaultModel)
	}
	if req.Format != "json" {
		t.Errorf("format = %q, want json", req.Format)
	}
	if req.Stream {
		t.Error("stream = true, want false")
	}
	if len(req.Messages) == 0 || req.Messages[0].Role != "system" {
		t.Errorf("messages = %+v, want a system prompt first", req.Messages)
	}
}

func TestConfigDefaultsToDeepSeek(t *testing.T) {
	t.Setenv(agent.EnvOllamaBaseURL, "")
	t.Setenv(agent.EnvOllamaModel, "")
	t.Setenv(agent.EnvOllamaTimeout, "")
	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "deepseek-v4.1-flash:cloud" {
		t.Errorf("model = %q, want the DeepSeek default", cfg.Model)
	}
	if cfg.BaseURL != "http://127.0.0.1:11434" {
		t.Errorf("baseURL = %q", cfg.BaseURL)
	}
}

func TestConfigModelEnvOverride(t *testing.T) {
	t.Setenv(agent.EnvOllamaModel, "glm-5.3-flash:cloud")
	cfg, err := ConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "glm-5.3-flash:cloud" {
		t.Errorf("model = %q, want the environment override", cfg.Model)
	}
}

func TestConfigRejectsBadTimeout(t *testing.T) {
	t.Setenv(agent.EnvOllamaTimeout, "soon")
	if _, err := ConfigFromEnv(); err == nil {
		t.Error("expected an error for an invalid timeout")
	}
}

// --- Tool loop ---

func TestToolCallReadsFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "notes.txt", "hello from the repo")
	fake, srv := newFakeOllama(t,
		`{"tool":"read_file","args":{"path":"notes.txt"}}`,
		`{"status":"completed","summary":"read it","changes_expected":false}`,
	)

	content, err := New(testConfig(srv.URL), dir).Execute(context.Background(), implementRequest())
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(content, `"changes_expected":false`) {
		t.Errorf("content = %q", content)
	}
	if fake.count() != 2 {
		t.Fatalf("chat calls = %d, want 2", fake.count())
	}
	if !strings.Contains(messageText(fake.request(1)), "hello from the repo") {
		t.Errorf("the tool result was not fed back to the model:\n%s", messageText(fake.request(1)))
	}
}

func TestToolCallWritesFile(t *testing.T) {
	dir := t.TempDir()
	_, srv := newFakeOllama(t,
		`{"tool":"write_file","args":{"path":"pkg/new.go","content":"package pkg\n"}}`,
		`{"status":"completed","summary":"wrote it","changes_expected":true}`,
	)
	if _, err := New(testConfig(srv.URL), dir).Execute(context.Background(), implementRequest()); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "pkg", "new.go"))
	if err != nil {
		t.Fatalf("file not written: %v", err)
	}
	if string(got) != "package pkg\n" {
		t.Errorf("content = %q", got)
	}
}

func TestToolCallCreateFileRefusesExisting(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "exists.txt", "original")
	fake, srv := newFakeOllama(t,
		`{"tool":"create_file","args":{"path":"exists.txt","content":"overwritten"}}`,
		`{"status":"completed","summary":"done","changes_expected":false}`,
	)
	if _, err := New(testConfig(srv.URL), dir).Execute(context.Background(), implementRequest()); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "exists.txt"))
	if string(got) != "original" {
		t.Errorf("create_file overwrote an existing file: %q", got)
	}
	if !strings.Contains(messageText(fake.request(1)), "already exists") {
		t.Errorf("the refusal was not reported to the model:\n%s", messageText(fake.request(1)))
	}
}

func TestUnsupportedToolIsReportedNotFatal(t *testing.T) {
	dir := t.TempDir()
	fake, srv := newFakeOllama(t,
		`{"tool":"launch_missiles","args":{}}`,
		`{"status":"completed","summary":"ok","changes_expected":false}`,
	)
	if _, err := New(testConfig(srv.URL), dir).Execute(context.Background(), implementRequest()); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(messageText(fake.request(1)), "unsupported tool") {
		t.Errorf("tool error not reported:\n%s", messageText(fake.request(1)))
	}
}

func TestIterationLimitIsBounded(t *testing.T) {
	dir := t.TempDir()
	tool := `{"tool":"git_status","args":{}}`
	_, srv := newFakeOllama(t, tool, tool, tool, tool, tool, tool)
	cfg := testConfig(srv.URL)
	cfg.MaxIterations = 3

	_, err := New(cfg, dir).Execute(context.Background(), implementRequest())
	if err == nil || !strings.Contains(err.Error(), "iteration limit") {
		t.Fatalf("err = %v, want an iteration-limit error", err)
	}
}

func TestToolCallLimitIsBounded(t *testing.T) {
	dir := t.TempDir()
	tool := `{"tool":"git_status","args":{}}`
	_, srv := newFakeOllama(t, tool, tool, tool, tool, tool, tool)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 1

	_, err := New(cfg, dir).Execute(context.Background(), implementRequest())
	if err == nil || !strings.Contains(err.Error(), "tool-call limit") {
		t.Fatalf("err = %v, want a tool-call-limit error", err)
	}
}

// --- Outcomes ---

func TestEmptyResponseIsRetried(t *testing.T) {
	dir := t.TempDir()
	fake, srv := newFakeOllama(t, "", `{"status":"completed","summary":"ok","changes_expected":false}`)
	content, err := New(testConfig(srv.URL), dir).Execute(context.Background(), implementRequest())
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(content, `"status":"completed"`) {
		t.Errorf("content = %q", content)
	}
	if fake.count() != 2 {
		t.Errorf("chat calls = %d, want 2 (an empty turn is re-requested)", fake.count())
	}
}

func TestEmptyResponseExhaustsRetries(t *testing.T) {
	dir := t.TempDir()
	_, srv := newFakeOllama(t, "", "", "")
	_, err := New(testConfig(srv.URL), dir).Execute(context.Background(), implementRequest())
	if err == nil || !strings.Contains(err.Error(), "empty response") {
		t.Fatalf("err = %v, want an empty-response error", err)
	}
}

func TestOutcomesPassThrough(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"completed with changes", `{"status":"completed","summary":"did it","changes_expected":true}`},
		{"completed without changes", `{"status":"completed","summary":"already satisfied","changes_expected":false}`},
		{"needs human", `{"status":"needs_human","reason":"needs a decision"}`},
		{"failed", `{"status":"failed","reason":"cannot complete"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			_, srv := newFakeOllama(t, tc.body)
			content, err := New(testConfig(srv.URL), dir).Execute(context.Background(), implementRequest())
			if err != nil {
				t.Fatalf("Execute failed: %v", err)
			}
			var got map[string]any
			if err := json.Unmarshal([]byte(content), &got); err != nil {
				t.Fatalf("content is not JSON: %v (%q)", err, content)
			}
			var want map[string]any
			_ = json.Unmarshal([]byte(tc.body), &want)
			if got["status"] != want["status"] {
				t.Errorf("status = %v, want %v", got["status"], want["status"])
			}
		})
	}
}

func TestMalformedModelResponse(t *testing.T) {
	dir := t.TempDir()
	_, srv := newFakeOllama(t, "I am not JSON at all")
	_, err := New(testConfig(srv.URL), dir).Execute(context.Background(), implementRequest())
	if err == nil || !strings.Contains(err.Error(), "malformed model response") {
		t.Fatalf("err = %v, want a malformed-response error", err)
	}
}

func TestMalformedToolRequest(t *testing.T) {
	dir := t.TempDir()
	_, srv := newFakeOllama(t, `{"tool":123,"args":{}}`)
	_, err := New(testConfig(srv.URL), dir).Execute(context.Background(), implementRequest())
	if err == nil || !strings.Contains(err.Error(), "malformed tool request") {
		t.Fatalf("err = %v, want a malformed-tool error", err)
	}
}

// --- Ollama failures ---

func TestOllamaUnavailable(t *testing.T) {
	dir := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.Close() // now unreachable

	_, err := New(testConfig(srv.URL), dir).Execute(context.Background(), implementRequest())
	if err == nil || !strings.Contains(err.Error(), "ollama unavailable") {
		t.Fatalf("err = %v, want an unavailable error", err)
	}
}

func TestOllamaHTTPError(t *testing.T) {
	dir := t.TempDir()
	fake, srv := newFakeOllama(t)
	fake.status = http.StatusInternalServerError

	_, err := New(testConfig(srv.URL), dir).Execute(context.Background(), implementRequest())
	if err == nil || !strings.Contains(err.Error(), "HTTP 500") {
		t.Fatalf("err = %v, want an HTTP error", err)
	}
}

// --- PLAN and REVIEW keep their own schema ---

func TestPlanResponseIsNotConvertedToOutcome(t *testing.T) {
	dir := t.TempDir()
	plan := `{"project":"p","summary":"s","stages":[{"id":"S001","title":"t","objective":"o","dependencies":[],"deliverables":["d"],"acceptance_criteria":["a"]}]}`
	_, srv := newFakeOllama(t, plan)
	t.Setenv(agent.EnvOllamaBaseURL, srv.URL)
	t.Setenv(agent.EnvOllamaModel, "")
	t.Setenv(agent.EnvOllamaTimeout, "")

	body := `{"capability":"PLAN","task":"make a plan","input":"prd","output_requirements":"json"}`
	var out, errOut bytes.Buffer
	if err := Run(context.Background(), strings.NewReader(body), &out, &errOut, func() (string, error) { return dir, nil }); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if !strings.Contains(out.String(), "stages") || strings.Contains(out.String(), "changes_expected") {
		t.Errorf("out = %q, want the plan document unchanged", out.String())
	}
}

func TestReviewResponseIsNotConvertedToOutcome(t *testing.T) {
	dir := t.TempDir()
	report := `{"summary":"looks fine","findings":[]}`
	_, srv := newFakeOllama(t, report)
	t.Setenv(agent.EnvOllamaBaseURL, srv.URL)
	t.Setenv(agent.EnvOllamaModel, "")
	t.Setenv(agent.EnvOllamaTimeout, "")

	body := `{"capability":"REVIEW","task":"review","input":"diff","output_requirements":"json"}`
	var out, errOut bytes.Buffer
	if err := Run(context.Background(), strings.NewReader(body), &out, &errOut, func() (string, error) { return dir, nil }); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if !strings.Contains(out.String(), "findings") || strings.Contains(out.String(), "changes_expected") {
		t.Errorf("out = %q, want the review document unchanged", out.String())
	}
}

func TestHarnessErrorBecomesFailedOutcomeForMutatingCapability(t *testing.T) {
	dir := t.TempDir()
	fake, srv := newFakeOllama(t)
	fake.status = http.StatusInternalServerError
	t.Setenv(agent.EnvOllamaBaseURL, srv.URL)
	t.Setenv(agent.EnvOllamaModel, "")
	t.Setenv(agent.EnvOllamaTimeout, "")

	body := `{"capability":"IMPLEMENT","task":"do it"}`
	var out, errOut bytes.Buffer
	if err := Run(context.Background(), strings.NewReader(body), &out, &errOut, func() (string, error) { return dir, nil }); err != nil {
		t.Fatalf("Run returned an error, want a failed outcome: %v", err)
	}
	if !strings.Contains(out.String(), `"status":"failed"`) {
		t.Errorf("out = %q, want a failed outcome", out.String())
	}
	if !strings.Contains(errOut.String(), "HTTP 500") {
		t.Errorf("errOut = %q, want the underlying reason", errOut.String())
	}
}

func TestHarnessErrorIsAnErrorForSchemaCapability(t *testing.T) {
	dir := t.TempDir()
	fake, srv := newFakeOllama(t)
	fake.status = http.StatusInternalServerError
	t.Setenv(agent.EnvOllamaBaseURL, srv.URL)
	t.Setenv(agent.EnvOllamaModel, "")
	t.Setenv(agent.EnvOllamaTimeout, "")

	body := `{"capability":"PLAN","task":"plan"}`
	var out, errOut bytes.Buffer
	if err := Run(context.Background(), strings.NewReader(body), &out, &errOut, func() (string, error) { return dir, nil }); err == nil {
		t.Fatal("Run returned nil, want an error for a schema capability")
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(name)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
