package ollamaagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/toolharness"
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
		MaxToolCalls:   4,
		CommandTimeout: 10 * time.Second,
		MaxOutputBytes: 1 << 20,
	}
}

func implementRequest() agent.Request {
	return agent.Request{Capability: agent.Implement, Task: "do it", Input: "context", OutputRequirements: "summarize"}
}

func fixRequest() agent.Request {
	return agent.Request{Capability: agent.Fix, Task: "fix it", Input: "validation failed", OutputRequirements: "summarize"}
}

func reviewRequest() agent.Request {
	return agent.Request{Capability: agent.Review, Task: "review it", Input: "diff", OutputRequirements: "json"}
}

func planRequest() agent.Request {
	return agent.Request{Capability: agent.Plan, Task: "make a plan", Input: "prd", OutputRequirements: "json"}
}

// distinctToolCalls returns n distinct read_file calls, so a model that never
// finishes performs genuinely different actions rather than a detected loop.
func distinctToolCalls(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = readToolCall(i)
	}
	return out
}

// --- Run: the stdin contract ---

func TestRunParsesRequestAndWritesOutcome(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	// The model changes a file, so its completed/changes_expected=true outcome is
	// consistent with the observed repository change and passes through unchanged.
	_, srv := newFakeOllama(t,
		`{"tool":"write_file","args":{"path":"out.txt","content":"x"}}`,
		`{"status":"completed","summary":"did it","changes_expected":true}`,
	)
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

func TestRunShowsRuntimeVisibilityAtStartup(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	// Just a simple model response to pass
	_, srv := newFakeOllama(t, `{"status":"completed","summary":"done","changes_expected":false}`)
	t.Setenv(agent.EnvOllamaBaseURL, srv.URL)
	t.Setenv(agent.EnvOllamaModel, "test-model:8b")
	t.Setenv(agent.EnvOllamaTimeout, "")

	body := `{"capability":"IMPLEMENT","task":"do it","input":"ctx","output_requirements":"out"}`
	var out, errOut bytes.Buffer
	if err := Run(context.Background(), strings.NewReader(body), &out, &errOut, func() (string, error) { return dir, nil }); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	stderr := errOut.String()
	// Verify runtime visibility output appears in stderr
	if !strings.Contains(stderr, "Harness: tool") {
		t.Errorf("stderr missing 'Harness: tool': %q", stderr)
	}
	if !strings.Contains(stderr, "Provider: ollama") {
		t.Errorf("stderr missing 'Provider: ollama': %q", stderr)
	}
	if !strings.Contains(stderr, "Model: test-model:8b") {
		t.Errorf("stderr missing 'Model: test-model:8b': %q", stderr)
	}
	if !strings.Contains(stderr, "Provider source: environment") {
		t.Errorf("stderr missing 'Provider source: environment': %q", stderr)
	}
}

func TestRunReconcilesClaimedChangeWithNoChange(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	// The model claims a change but the working tree is clean: observed reality wins.
	_, srv := newFakeOllama(t, `{"status":"completed","summary":"did it","changes_expected":true}`)
	t.Setenv(agent.EnvOllamaBaseURL, srv.URL)
	t.Setenv(agent.EnvOllamaModel, "")
	t.Setenv(agent.EnvOllamaTimeout, "")

	body := `{"capability":"IMPLEMENT","task":"do it","input":"ctx"}`
	var out, errOut bytes.Buffer
	if err := Run(context.Background(), strings.NewReader(body), &out, &errOut, func() (string, error) { return dir, nil }); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if !strings.Contains(out.String(), `"changes_expected":false`) {
		t.Errorf("out = %q, want changes_expected reconciled to false", out.String())
	}
	if !strings.Contains(out.String(), "reconciled to repository reality") {
		t.Errorf("out = %q, want the disagreement surfaced in the summary", out.String())
	}
	if !strings.Contains(errOut.String(), "reconciled") {
		t.Errorf("errOut = %q, want the mismatch surfaced", errOut.String())
	}
}

func TestRunReconcilesSilentChange(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	// The model changed a file but reported changes_expected=false: observed reality
	// still wins, so the outcome is corrected to true.
	_, srv := newFakeOllama(t,
		`{"tool":"write_file","args":{"path":"out.txt","content":"x"}}`,
		`{"status":"completed","summary":"no change needed","changes_expected":false}`,
	)
	t.Setenv(agent.EnvOllamaBaseURL, srv.URL)
	t.Setenv(agent.EnvOllamaModel, "")
	t.Setenv(agent.EnvOllamaTimeout, "")

	body := `{"capability":"IMPLEMENT","task":"do it"}`
	var out, errOut bytes.Buffer
	if err := Run(context.Background(), strings.NewReader(body), &out, &errOut, func() (string, error) { return dir, nil }); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if !strings.Contains(out.String(), `"changes_expected":true`) {
		t.Errorf("out = %q, want changes_expected reconciled to true", out.String())
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

// TestRunNoChangeCeilingReturnsNeedsHuman proves a phased run that never changes
// the repository surfaces as a retryable needs_human outcome, so SOP requeues it
// instead of blocking it.
func TestRunNoChangeCeilingReturnsNeedsHuman(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(agent.EnvOllamaBaseURL, "")
	t.Setenv(agent.EnvOllamaModel, "")
	t.Setenv(agent.EnvOllamaTimeout, "")
	_, srv := newFakeOllama(t, distinctToolCalls(maxIterationsImplement+4)...)
	t.Setenv(agent.EnvOllamaBaseURL, srv.URL)

	body := `{"capability":"IMPLEMENT","task":"do it","input":"ctx"}`
	var out, errOut bytes.Buffer
	if err := Run(context.Background(), strings.NewReader(body), &out, &errOut, func() (string, error) { return dir, nil }); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if !strings.Contains(out.String(), `"status":"needs_human"`) {
		t.Errorf("outcome = %q, want needs_human (retryable)", out.String())
	}
}

// TestRunTracesModelReportedFailure proves the trace is kept for a failure the
// model chose to report, not only for an infrastructure error — otherwise the
// turns that led to an AHV2010-style incident are lost.
func TestRunTracesModelReportedFailure(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	traceFile := filepath.Join(t.TempDir(), "trace.log")
	t.Setenv(agent.EnvOllamaBaseURL, "")
	t.Setenv(agent.EnvOllamaModel, "")
	t.Setenv(agent.EnvOllamaTimeout, "")
	t.Setenv("SOP_OLLAMA_TRACE_LOG", traceFile)
	_, srv := newFakeOllama(t, `{"status":"needs_human","reason":"stuck"}`)
	t.Setenv(agent.EnvOllamaBaseURL, srv.URL)

	body := `{"capability":"IMPLEMENT","task":"do it","input":"ctx"}`
	var out, errOut bytes.Buffer
	if err := Run(context.Background(), strings.NewReader(body), &out, &errOut, func() (string, error) { return dir, nil }); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	got, err := os.ReadFile(traceFile)
	if err != nil {
		t.Fatalf("trace file not written: %v", err)
	}
	if !strings.Contains(string(got), "IMPLEMENT") {
		t.Errorf("trace = %q, want the turn history", got)
	}
}

// TestRunNoChangeFailureIsRetryable proves a model-reported failure that changed
// nothing is surfaced as a retryable needs_human, so SOP requeues instead of
// blocking a flaky "explored and gave up" attempt.
func TestRunNoChangeFailureIsRetryable(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	t.Setenv(agent.EnvOllamaBaseURL, "")
	t.Setenv(agent.EnvOllamaModel, "")
	t.Setenv(agent.EnvOllamaTimeout, "")
	_, srv := newFakeOllama(t, `{"status":"failed","reason":"exploration consumed the budget"}`)
	t.Setenv(agent.EnvOllamaBaseURL, srv.URL)

	body := `{"capability":"IMPLEMENT","task":"do it","input":"ctx"}`
	var out, errOut bytes.Buffer
	if err := Run(context.Background(), strings.NewReader(body), &out, &errOut, func() (string, error) { return dir, nil }); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if !strings.Contains(out.String(), `"status":"needs_human"`) {
		t.Errorf("outcome = %q, want needs_human (retryable)", out.String())
	}
}

// TestRunChangedTreeKeepsFailure proves a failure that did change the repository is
// left as a hard failure, not silently made retryable.
func TestRunChangedTreeKeepsFailure(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	t.Setenv(agent.EnvOllamaBaseURL, "")
	t.Setenv(agent.EnvOllamaModel, "")
	t.Setenv(agent.EnvOllamaTimeout, "")
	// The mutation happens during the invocation (after the baseline is
	// captured), so it is attributed to this run, not treated as pre-existing
	// dirty work.
	_, srv := newFakeOllama(t,
		`{"tool":"write_file","args":{"path":"changed.txt","content":"an existing change"}}`,
		`{"status":"failed","reason":"could not finish"}`,
	)
	t.Setenv(agent.EnvOllamaBaseURL, srv.URL)

	body := `{"capability":"IMPLEMENT","task":"do it","input":"ctx"}`
	var out, errOut bytes.Buffer
	if err := Run(context.Background(), strings.NewReader(body), &out, &errOut, func() (string, error) { return dir, nil }); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if !strings.Contains(out.String(), `"status":"failed"`) {
		t.Errorf("outcome = %q, want failed", out.String())
	}
}

// TestRunPersistsFailedTraceToSink proves a failed run's per-turn trace reaches the
// operator-selected file, so it survives the command provider discarding stderr.
func TestRunPersistsFailedTraceToSink(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "notes.txt", "stable")
	traceFile := filepath.Join(t.TempDir(), "trace.log")
	t.Setenv(agent.EnvOllamaBaseURL, "")
	t.Setenv(agent.EnvOllamaModel, "")
	t.Setenv(agent.EnvOllamaTimeout, "")
	t.Setenv("SOP_OLLAMA_TRACE_LOG", traceFile)

	repeat := `{"tool":"read_file","args":{"path":"notes.txt"}}`
	_, srv := newFakeOllama(t, repeat, repeat, repeat, repeat, repeat, repeat, repeat, repeat)
	t.Setenv(agent.EnvOllamaBaseURL, srv.URL)

	body := `{"capability":"IMPLEMENT","task":"do it","input":"ctx"}`
	var out, errOut bytes.Buffer
	if err := Run(context.Background(), strings.NewReader(body), &out, &errOut, func() (string, error) { return dir, nil }); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	got, err := os.ReadFile(traceFile)
	if err != nil {
		t.Fatalf("trace file not written: %v", err)
	}
	if !strings.Contains(string(got), "IMPLEMENT") {
		t.Errorf("trace = %q, want the per-turn trail", got)
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
	if got := string(req.Format); got != `"json"` {
		t.Errorf("format = %s, want json", got)
	}
	if req.Stream {
		t.Error("stream = true, want false")
	}
	if req.Think == nil || *req.Think {
		t.Errorf("think = %v, want an explicit false to disable reasoning turns", req.Think)
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

func TestNativeToolCallIsExecuted(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "notes.txt", "hello from the repo")

	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			// Native Ollama tool call: empty content, the call in tool_calls.
			_, _ = io.WriteString(w, `{"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_1","function":{"name":"read_file","arguments":{"args":{"path":"notes.txt"}}}}]}}`)
			return
		}
		_ = json.NewEncoder(w).Encode(ollamaChatResponse{Message: chatMessage{Role: "assistant", Content: `{"status":"completed","summary":"read it","changes_expected":false}`}})
	}))
	defer srv.Close()

	content, err := New(testConfig(srv.URL), dir).Execute(context.Background(), implementRequest())
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(content, `"changes_expected":false`) {
		t.Errorf("content = %q", content)
	}
	if calls != 2 {
		t.Errorf("chat calls = %d, want 2 (the native call was executed, not treated as empty)", calls)
	}
}

func TestToolArgumentsUnwrapsArgsWrapper(t *testing.T) {
	wrapped := toolArguments(json.RawMessage(`{"args":{"path":"a.go"}}`))
	if wrapped["path"] != "a.go" {
		t.Errorf("wrapped args = %v, want path=a.go", wrapped)
	}
	direct := toolArguments(json.RawMessage(`{"command":"go test ./..."}`))
	if direct["command"] != "go test ./..." {
		t.Errorf("direct args = %v, want command", direct)
	}
}

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

func TestToolCallWithRawControlCharactersIsParsed(t *testing.T) {
	dir := t.TempDir()
	// A tool call whose "content" holds literal newlines and tabs — invalid strict
	// JSON, as a model emits when writing a multi-line file body.
	call := "{\"tool\": \"write_file\", \"args\": {\"path\": \"pkg/new.go\", \"content\": \"package pkg\n\nfunc F() {}\n\"}}"
	_, srv := newFakeOllama(t,
		call,
		`{"status":"completed","summary":"wrote it","changes_expected":true}`,
	)
	if _, err := New(testConfig(srv.URL), dir).Execute(context.Background(), implementRequest()); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "pkg", "new.go"))
	if err != nil {
		t.Fatalf("file not written: %v", err)
	}
	if string(got) != "package pkg\n\nfunc F() {}\n" {
		t.Errorf("content = %q", got)
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

// TestImplementWithoutMutationStopsWithNoProgress covers a model that keeps making
// distinct tool calls without ever changing the repository. It must not run to the
// ceiling: the repository no-progress guard stops it early with a retryable
// IMPLEMENT_NO_PROGRESS result rather than a hard failure.
func TestImplementWithoutMutationStopsWithNoProgress(t *testing.T) {
	dir := t.TempDir()
	_, srv := newFakeOllama(t, distinctToolCalls(maxIterationsImplement+4)...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 100

	h := New(cfg, dir)
	_, err := h.Execute(context.Background(), implementRequest())
	if err == nil || !strings.Contains(err.Error(), "IMPLEMENT_NO_PROGRESS") {
		t.Fatalf("err = %v, want an IMPLEMENT_NO_PROGRESS termination", err)
	}
	if !strings.Contains(err.Error(), "termination=no_progress") {
		t.Errorf("err = %v, want termination=no_progress", err)
	}
	if hasEvent(h.TraceRecords(), implementFinalizeEvent) {
		t.Error("an unmutated run must be stopped by the no-progress guard, not finalized")
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

// --- Capability policy ---

func TestCapabilityBudgets(t *testing.T) {
	// PLAN is two-phase (see the PLAN phase tests); its MaxIterations is only a
	// total ceiling, so it is covered separately.
	cases := []struct {
		cap  agent.Capability
		want int
	}{
		{agent.DesignTests, 12},
		{agent.Implement, 32},
		{agent.Fix, 24},
		{agent.Review, 10},
	}
	for _, tc := range cases {
		if got := PolicyFor(tc.cap).MaxIterations; got != tc.want {
			t.Errorf("PolicyFor(%s).MaxIterations = %d, want %d", tc.cap, got, tc.want)
		}
	}
}

func TestCapabilityToolRestrictions(t *testing.T) {
	plan := PolicyFor(agent.Plan)
	if !plan.ReadOnly {
		t.Error("PLAN should be read-only")
	}
	for _, tool := range []string{toolharness.ToolWriteFile, toolharness.ToolCreateFile, toolharness.ToolRunCommand} {
		if plan.Allows(tool) {
			t.Errorf("PLAN allows %s, want a read-only surface", tool)
		}
	}
	for _, tool := range []string{toolharness.ToolReadFile, toolharness.ToolListFiles, toolharness.ToolSearchFiles, toolharness.ToolGitStatus, toolharness.ToolGitDiff} {
		if !plan.Allows(tool) {
			t.Errorf("PLAN should allow %s", tool)
		}
	}
	impl := PolicyFor(agent.Implement)
	if impl.ReadOnly {
		t.Error("IMPLEMENT should not be read-only")
	}
	if !impl.Allows(toolharness.ToolWriteFile) || !impl.Allows(toolharness.ToolRunCommand) {
		t.Error("IMPLEMENT should allow the mutation tools")
	}
}

func TestPlanNormalCompletion(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "notes.txt", "context")
	fake, srv := newFakeOllama(t,
		`{"tool":"list_files","args":{"path":"."}}`,
		`{"tool":"read_file","args":{"path":"notes.txt"}}`,
		`{"project":"p","summary":"s","stages":[]}`,
	)
	content, err := New(testConfig(srv.URL), dir).Execute(context.Background(), planRequest())
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(content, "stages") {
		t.Errorf("content = %q, want the plan document", content)
	}
	if fake.count() != 3 {
		t.Errorf("chat calls = %d, want 3 (finished well before the 8-iteration budget)", fake.count())
	}
}

func TestPlanCannotMutateRepository(t *testing.T) {
	dir := t.TempDir()
	fake, srv := newFakeOllama(t,
		`{"tool":"write_file","args":{"path":"should-not-exist.txt","content":"nope"}}`,
		`{"project":"p","summary":"s","stages":[]}`,
	)
	h := New(testConfig(srv.URL), dir)
	content, err := h.Execute(context.Background(), planRequest())
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "should-not-exist.txt")); statErr == nil {
		t.Error("PLAN wrote a file: the read-only policy was bypassed")
	}
	if !strings.Contains(messageText(fake.request(1)), "not available for PLAN") {
		t.Errorf("the refusal was not reported to the model:\n%s", messageText(fake.request(1)))
	}
	denied := 0
	for _, r := range h.AuditRecords() {
		if r.Action == toolharness.ActionDeny {
			denied++
		}
	}
	if denied != 1 {
		t.Errorf("denied audit records = %d, want 1", denied)
	}
	if !strings.Contains(content, "stages") {
		t.Errorf("content = %q, want the plan document", content)
	}
}

// --- PLAN phases: DISCOVERY → SYNTHESIS ---

// readToolCall builds a distinct read_file tool call, so discovery turns are not
// detected as a repeated no-progress action.
func readToolCall(i int) string {
	return fmt.Sprintf(`{"tool":"read_file","args":{"path":"pkg/f%d.go"}}`, i)
}

func countPhase(records []TraceRecord, phase string) int {
	n := 0
	for _, r := range records {
		if r.Phase == phase {
			n++
		}
	}
	return n
}

func hasEvent(records []TraceRecord, event string) bool {
	for _, r := range records {
		if r.Event == event {
			return true
		}
	}
	return false
}

func TestPlanEarlyFinalCompletesImmediately(t *testing.T) {
	dir := t.TempDir()
	fake, srv := newFakeOllama(t, `{"project":"p","summary":"s","stages":[]}`)
	h := New(testConfig(srv.URL), dir)
	content, err := h.Execute(context.Background(), planRequest())
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(content, "stages") {
		t.Errorf("content = %q", content)
	}
	if fake.count() != 1 {
		t.Errorf("chat calls = %d, want 1 (early final, no discovery)", fake.count())
	}
	records := h.TraceRecords()
	if len(records) != 1 || records[0].Phase != "DISCOVERY" || records[0].Tool != "final" {
		t.Errorf("trace = %+v, want one DISCOVERY final turn", records)
	}
	if hasEvent(records, "→ "+planTwoPhase.synthLabel) {
		t.Errorf("early final must not enter synthesis: %+v", records)
	}
}

func TestPlanForcedSynthesisAfterDiscoveryLimit(t *testing.T) {
	dir := t.TempDir()
	responses := make([]string, 0, planDiscoveryTurns+1)
	for i := 0; i < planDiscoveryTurns; i++ {
		responses = append(responses, readToolCall(i))
	}
	responses = append(responses, `{"project":"p","summary":"s","stages":[]}`)
	fake, srv := newFakeOllama(t, responses...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 100

	h := New(cfg, dir)
	content, err := h.Execute(context.Background(), planRequest())
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(content, "stages") {
		t.Errorf("content = %q", content)
	}
	if fake.count() != planDiscoveryTurns+1 {
		t.Errorf("chat calls = %d, want %d discovery + 1 synthesis", fake.count(), planDiscoveryTurns)
	}
	if got := len(h.AuditRecords()); got != planDiscoveryTurns {
		t.Errorf("executed tools = %d, want %d", got, planDiscoveryTurns)
	}

	records := h.TraceRecords()
	if got := countPhase(records, "DISCOVERY"); got != planDiscoveryTurns {
		t.Errorf("discovery turns = %d, want %d", got, planDiscoveryTurns)
	}
	if !hasEvent(records, "→ "+planTwoPhase.synthLabel) {
		t.Errorf("trace does not record the synthesis transition: %+v", records)
	}
	if got := countPhase(records, "SYNTHESIS"); got != 1 {
		t.Errorf("synthesis turns = %d, want 1", got)
	}
	if !strings.Contains(messageText(fake.request(fake.count()-1)), "Exploration is complete") {
		t.Error("the forced-synthesis instruction was not sent")
	}
}

func TestPlanSynthesisDeniesTools(t *testing.T) {
	dir := t.TempDir()
	responses := make([]string, 0, planDiscoveryTurns+2)
	for i := 0; i < planDiscoveryTurns; i++ {
		responses = append(responses, readToolCall(i))
	}
	// During synthesis the model requests a read-only tool; it must be denied.
	// On the next synthesis turn it returns the required final PLAN.
	responses = append(responses,
		`{"tool":"read_file","args":{"path":"notes.txt"}}`,
		`{"project":"p","summary":"plan complete","stages":[{"id":"stage-1","title":"Implement change"}]}`,
	)
	fake, srv := newFakeOllama(t, responses...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 100

	h := New(cfg, dir)
	if _, err := h.Execute(context.Background(), planRequest()); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	denied := 0
	for _, r := range h.AuditRecords() {
		if r.Action == toolharness.ActionDeny {
			denied++
		}
	}
	if denied != 1 {
		t.Errorf("denied records = %d, want 1 (the synthesis tool request)", denied)
	}
	if got := len(h.AuditRecords()); got != planDiscoveryTurns+1 {
		t.Errorf("audit records = %d, want %d executed + 1 denied", got, planDiscoveryTurns)
	}
	if !strings.Contains(messageText(fake.request(fake.count()-1)), "No additional tools are available") {
		t.Error("the synthesis correction was not sent")
	}
}

func TestPlanSynthesisExhaustion(t *testing.T) {
	dir := t.TempDir()
	responses := make([]string, 0, planDiscoveryTurns+planSynthesisTurns)
	for i := 0; i < planDiscoveryTurns; i++ {
		responses = append(responses, readToolCall(i))
	}
	responses = append(responses,
		`{"tool":"read_file","args":{"path":"pkg/a.go"}}`,
		`{"tool":"read_file","args":{"path":"pkg/b.go"}}`,
	)
	fake, srv := newFakeOllama(t, responses...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 100

	_, err := New(cfg, dir).Execute(context.Background(), planRequest())
	if err == nil || !strings.Contains(err.Error(), "termination=synthesis_limit") {
		t.Fatalf("err = %v, want a synthesis_limit termination", err)
	}
	if strings.Contains(err.Error(), "iteration_limit") {
		t.Errorf("err = %v, must not masquerade as the generic iteration limit", err)
	}
	if !strings.Contains(err.Error(), "during synthesis") ||
		!strings.Contains(err.Error(), "discovery_tool_calls=8") ||
		!strings.Contains(err.Error(), "synthesis_turns=2") {
		t.Errorf("err = %v, want phase and counts", err)
	}
	if fake.count() != planDiscoveryTurns+planSynthesisTurns {
		t.Errorf("chat calls = %d, want %d", fake.count(), planDiscoveryTurns+planSynthesisTurns)
	}
}

// TestPlanAHV2006ShapeFixture approximates AHV2006: several useful discovery
// reads/searches consume the discovery budget, then the model synthesizes the
// document instead of continuing to explore. It uses no live provider and never
// touches real SOP state.
func TestPlanAHV2006ShapeFixture(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "internal/agent/agent.go", "package agent\n// Capability marker\n")
	writeFile(t, dir, "internal/ollamaagent/policy.go", "package ollamaagent\nfunc PolicyFor() {}\n")
	responses := []string{
		`{"tool":"list_files","args":{"path":"."}}`,
		`{"tool":"search_files","args":{"pattern":"Capability"}}`,
		`{"tool":"read_file","args":{"path":"internal/agent/agent.go"}}`,
		`{"tool":"search_files","args":{"pattern":"PolicyFor"}}`,
		`{"tool":"read_file","args":{"path":"internal/ollamaagent/policy.go"}}`,
		`{"tool":"list_files","args":{"path":"internal"}}`,
		`{"tool":"read_file","args":{"path":"internal/agent/agent.go"}}`,
		`{"tool":"git_status","args":{}}`,
		`{"project":"sop","summary":"discovered then synthesized","stages":[{"id":"S1","title":"t"}]}`,
	}
	fake, srv := newFakeOllama(t, responses...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 100

	h := New(cfg, dir)
	content, err := h.Execute(context.Background(), planRequest())
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(content, "discovered then synthesized") {
		t.Errorf("content = %q", content)
	}
	if fake.count() != planDiscoveryTurns+1 {
		t.Errorf("chat calls = %d, want %d discovery + 1 synthesis", fake.count(), planDiscoveryTurns)
	}
	records := h.TraceRecords()
	if !hasEvent(records, "→ "+planTwoPhase.synthLabel) {
		t.Errorf("trace does not record the transition: %+v", records)
	}
	// The safe trace never carries file contents or prompts.
	for _, r := range records {
		if strings.Contains(r.Request, "Capability marker") || strings.Contains(r.Request, "package ") {
			t.Errorf("trace leaked file content: %+v", r)
		}
	}
}

func TestPlanTraceRendering(t *testing.T) {
	dir := t.TempDir()
	responses := make([]string, 0, planDiscoveryTurns+1)
	for i := 0; i < planDiscoveryTurns; i++ {
		responses = append(responses, readToolCall(i))
	}
	responses = append(responses, `{"project":"p","summary":"s","stages":[]}`)
	_, srv := newFakeOllama(t, responses...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 100

	h := New(cfg, dir)
	if _, err := h.Execute(context.Background(), planRequest()); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	var b strings.Builder
	h.FlushTrace(&b)
	out := b.String()
	t.Logf("trace:\n%s", out)
	for _, want := range []string{
		"PLAN DISCOVERY #1 read_file path=pkg/f0.go [ok]",
		"PLAN DISCOVERY #8 read_file path=pkg/f7.go [ok]",
		"PLAN → SYNTHESIS",
		"PLAN SYNTHESIS #1 final [ok]",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("trace missing %q:\n%s", want, out)
		}
	}
}

func TestPlanTraceShowsSynthesisTermination(t *testing.T) {
	dir := t.TempDir()
	responses := make([]string, 0, planDiscoveryTurns+planSynthesisTurns)
	for i := 0; i < planDiscoveryTurns; i++ {
		responses = append(responses, readToolCall(i))
	}
	responses = append(responses,
		`{"tool":"read_file","args":{"path":"pkg/a.go"}}`,
		`{"tool":"read_file","args":{"path":"pkg/b.go"}}`,
	)
	_, srv := newFakeOllama(t, responses...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 100

	h := New(cfg, dir)
	if _, err := h.Execute(context.Background(), planRequest()); err == nil {
		t.Fatal("expected a synthesis-limit failure")
	}
	var b strings.Builder
	h.FlushTrace(&b)
	if !strings.Contains(b.String(), "termination=synthesis_limit") {
		t.Errorf("trace does not show the termination reason:\n%s", b.String())
	}
}

func TestImplementNormalCompletion(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "notes.txt", "hello")
	_, srv := newFakeOllama(t,
		`{"tool":"read_file","args":{"path":"notes.txt"}}`,
		`{"tool":"write_file","args":{"path":"out.txt","content":"written"}}`,
		`{"tool":"run_command","args":{"command":"go vet ./..."}}`,
		`{"status":"completed","summary":"did it","changes_expected":true}`,
	)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 10
	content, err := New(cfg, dir).Execute(context.Background(), implementRequest())
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(content, `"changes_expected":true`) {
		t.Errorf("content = %q", content)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "out.txt")); string(got) != "written" {
		t.Errorf("out.txt = %q", got)
	}
}

// TestReviewForcedSynthesisAfterDiscoveryLimit proves REVIEW is bounded like PLAN:
// after its short discovery budget it must synthesize the verdict with no tools,
// rather than reading until the iteration limit and erroring the whole run (the
// AHV2010 failure).
func TestReviewForcedSynthesisAfterDiscoveryLimit(t *testing.T) {
	dir := t.TempDir()
	seedToolFiles(t, dir, "pkg", reviewInspectTurns)
	responses := make([]string, 0, reviewInspectTurns+2)
	for i := 0; i < reviewInspectTurns; i++ {
		responses = append(responses, readToolCall(i))
	}
	responses = append(responses,
		`{"tool":"read_file","args":{"path":"pkg/extra.go"}}`, // refused: synthesis has no tools
		`{"summary":"clean","findings":[]}`,
	)
	fake, srv := newFakeOllama(t, responses...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 100

	h := New(cfg, dir)
	content, err := h.Execute(context.Background(), reviewRequest())
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(content, "findings") {
		t.Errorf("content = %q", content)
	}
	records := h.TraceRecords()
	if !hasEvent(records, "→ "+reviewTwoPhase.synthLabel) {
		t.Errorf("REVIEW did not enter synthesis: %+v", records)
	}
	if got := countPhase(records, "INSPECT"); got != reviewInspectTurns {
		t.Errorf("inspect turns = %d, want %d", got, reviewInspectTurns)
	}
	if got := countPhase(records, "SYNTHESIZE"); got != 2 {
		t.Errorf("synthesize turns = %d, want 2 (the denied tool call plus the final response)", got)
	}
	denied := 0
	for _, r := range h.AuditRecords() {
		if r.Action == toolharness.ActionDeny {
			denied++
		}
	}
	if denied != 1 {
		t.Errorf("denied tools = %d, want 1 (synthesis has no tools)", denied)
	}
	if !strings.Contains(messageText(fake.request(reviewInspectTurns)), "Inspection is complete") {
		t.Error("the forced-synthesis instruction was not sent")
	}
	if !strings.Contains(messageText(fake.request(fake.count()-1)), "No additional tools are available") {
		t.Error("the synthesis correction was not sent")
	}
}

func TestReviewCannotMutateRepository(t *testing.T) {
	dir := t.TempDir()
	fake, srv := newFakeOllama(t,
		`{"tool":"write_file","args":{"path":"should-not-exist.txt","content":"nope"}}`,
		`{"summary":"clean","findings":[]}`,
	)
	h := New(testConfig(srv.URL), dir)
	content, err := h.Execute(context.Background(), reviewRequest())
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "should-not-exist.txt")); statErr == nil {
		t.Error("REVIEW wrote a file: the read-only policy was bypassed")
	}
	if !strings.Contains(messageText(fake.request(1)), "not available for REVIEW") {
		t.Errorf("the refusal was not reported to the model:\n%s", messageText(fake.request(1)))
	}
	denied := 0
	for _, r := range h.AuditRecords() {
		if r.Action == toolharness.ActionDeny {
			denied++
		}
	}
	if denied != 1 {
		t.Errorf("denied audit records = %d, want 1", denied)
	}
	if !strings.Contains(content, "findings") {
		t.Errorf("content = %q, want the review document", content)
	}
}

func TestReviewEarlyFinalCompletesImmediately(t *testing.T) {
	dir := t.TempDir()
	fake, srv := newFakeOllama(t, `{"summary":"clean","findings":[]}`)
	h := New(testConfig(srv.URL), dir)
	content, err := h.Execute(context.Background(), reviewRequest())
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(content, "findings") {
		t.Errorf("content = %q", content)
	}
	if fake.count() != 1 {
		t.Errorf("chat calls = %d, want 1 (early final, no inspection)", fake.count())
	}
	records := h.TraceRecords()
	if len(records) != 1 || records[0].Phase != "INSPECT" || records[0].Tool != "final" {
		t.Errorf("trace = %+v, want one INSPECT final turn", records)
	}
	if hasEvent(records, "→ "+reviewTwoPhase.synthLabel) {
		t.Errorf("early final must not enter synthesis: %+v", records)
	}
}

func TestReviewInspectNudgeAtSixthCall(t *testing.T) {
	dir := t.TempDir()
	seedToolFiles(t, dir, "pkg", reviewInspectTurns)
	responses := make([]string, 0, reviewInspectTurns+1)
	for i := 0; i < reviewInspectTurns; i++ {
		responses = append(responses, readToolCall(i))
	}
	responses = append(responses, `{"summary":"clean","findings":[]}`)
	fake, srv := newFakeOllama(t, responses...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 100

	h := New(cfg, dir)
	content, err := h.Execute(context.Background(), reviewRequest())
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(content, "findings") {
		t.Errorf("content = %q", content)
	}

	// Messages accumulate across turns, so the nudge stays in every later
	// request's history once sent; only the newly appended (last) message of
	// each request shows when it was actually attached.
	nudgeCount, nudgeIndex := 0, -1
	for i := 0; i < fake.count(); i++ {
		msgs := fake.request(i).Messages
		if len(msgs) == 0 {
			continue
		}
		if strings.Contains(msgs[len(msgs)-1].Content, reviewTwoPhase.nudge) {
			nudgeCount++
			nudgeIndex = i
		}
	}
	if nudgeCount != 1 {
		t.Errorf("nudge appeared %d times, want exactly 1", nudgeCount)
	}
	if nudgeIndex != reviewInspectNudgeAfter {
		t.Errorf("nudge appeared at request %d, want %d (attached to inspection call 6's result)", nudgeIndex, reviewInspectNudgeAfter)
	}

	denied := 0
	for _, r := range h.AuditRecords() {
		if r.Action == toolharness.ActionDeny {
			denied++
		}
	}
	if denied != 0 {
		t.Errorf("denied tools = %d, want 0: the nudge must not deny or withdraw any tool", denied)
	}
	if got := len(h.AuditRecords()); got != reviewInspectTurns {
		t.Errorf("executed tools = %d, want %d (calls 7 and 8 still execute)", got, reviewInspectTurns)
	}
}

func TestReviewSynthesisExhaustion(t *testing.T) {
	dir := t.TempDir()
	seedToolFiles(t, dir, "pkg", reviewInspectTurns+2)
	responses := make([]string, 0, reviewInspectTurns+reviewSynthesizeTurns)
	for i := 0; i < reviewInspectTurns; i++ {
		responses = append(responses, readToolCall(i))
	}
	responses = append(responses,
		`{"tool":"read_file","args":{"path":"pkg/extra1.go"}}`,
		`{"tool":"read_file","args":{"path":"pkg/extra2.go"}}`,
	)
	fake, srv := newFakeOllama(t, responses...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 100

	_, err := New(cfg, dir).Execute(context.Background(), reviewRequest())
	if err == nil || !strings.Contains(err.Error(), "termination=synthesis_limit") {
		t.Fatalf("err = %v, want a synthesis_limit termination", err)
	}
	if strings.Contains(err.Error(), "iteration_limit") {
		t.Errorf("err = %v, must not masquerade as the generic iteration limit", err)
	}
	if !strings.Contains(err.Error(), "during synthesize") ||
		!strings.Contains(err.Error(), "inspect_tool_calls=8") ||
		!strings.Contains(err.Error(), "synthesize_turns=2") {
		t.Errorf("err = %v, want phase and counts", err)
	}
	if fake.count() != reviewInspectTurns+reviewSynthesizeTurns {
		t.Errorf("chat calls = %d, want %d", fake.count(), reviewInspectTurns+reviewSynthesizeTurns)
	}
}

func TestReviewTraceRendering(t *testing.T) {
	dir := t.TempDir()
	seedToolFiles(t, dir, "pkg", reviewInspectTurns)
	responses := make([]string, 0, reviewInspectTurns+1)
	for i := 0; i < reviewInspectTurns; i++ {
		responses = append(responses, readToolCall(i))
	}
	responses = append(responses, `{"summary":"clean","findings":[]}`)
	_, srv := newFakeOllama(t, responses...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 100

	h := New(cfg, dir)
	if _, err := h.Execute(context.Background(), reviewRequest()); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	var b strings.Builder
	h.FlushTrace(&b)
	out := b.String()
	t.Logf("trace:\n%s", out)
	for _, want := range []string{
		"REVIEW INSPECT #1 read_file path=pkg/f0.go [ok]",
		"REVIEW INSPECT #8 read_file path=pkg/f7.go [ok]",
		"REVIEW → SYNTHESIZE",
		"REVIEW SYNTHESIZE #1 final [ok]",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("trace missing %q:\n%s", want, out)
		}
	}
}

func TestReviewTraceShowsSynthesisTermination(t *testing.T) {
	dir := t.TempDir()
	seedToolFiles(t, dir, "pkg", reviewInspectTurns+2)
	responses := make([]string, 0, reviewInspectTurns+reviewSynthesizeTurns)
	for i := 0; i < reviewInspectTurns; i++ {
		responses = append(responses, readToolCall(i))
	}
	responses = append(responses,
		`{"tool":"read_file","args":{"path":"pkg/extra1.go"}}`,
		`{"tool":"read_file","args":{"path":"pkg/extra2.go"}}`,
	)
	_, srv := newFakeOllama(t, responses...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 100

	h := New(cfg, dir)
	if _, err := h.Execute(context.Background(), reviewRequest()); err == nil {
		t.Fatal("expected a synthesis-limit failure")
	}
	var b strings.Builder
	h.FlushTrace(&b)
	if !strings.Contains(b.String(), "termination=synthesis_limit") {
		t.Errorf("trace does not show the termination reason:\n%s", b.String())
	}
}

// --- IMPLEMENT phases: DISCOVER → CHANGE → FINALIZE ---

// seedToolFiles creates n distinct files so the harness's read calls succeed,
// making a discovery fixture representative rather than error-driven.
func seedToolFiles(t *testing.T, dir, prefix string, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		writeFile(t, dir, fmt.Sprintf("%s/f%d.go", prefix, i), "package pkg\n")
	}
}

func TestImplementEarlyCompletionSkipsFinalization(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "notes.txt", "hello")
	_, srv := newFakeOllama(t,
		`{"tool":"read_file","args":{"path":"notes.txt"}}`,
		`{"tool":"write_file","args":{"path":"out.txt","content":"x"}}`,
		`{"status":"completed","summary":"done","changes_expected":true}`,
	)
	h := New(testConfig(srv.URL), dir)
	content, err := h.Execute(context.Background(), implementRequest())
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(content, `"status":"completed"`) {
		t.Errorf("content = %q", content)
	}
	records := h.TraceRecords()
	if hasEvent(records, implementFinalizeEvent) {
		t.Errorf("an early completion must not force finalization: %+v", records)
	}
	if got := countPhase(records, "FINALIZE"); got != 0 {
		t.Errorf("finalize turns = %d, want 0", got)
	}
	if got := countPhase(records, "CHANGE"); got != 2 {
		t.Errorf("change turns = %d, want 2 (write then outcome)", got)
	}
}

// TestImplementUnmutatedRunStopsBeforeDiscoveryNudge proves the discovery nudge
// (which fires past implementNudgeAfter) is no longer reached by a run that never
// changes the repository: the no-progress guard stops it first. The nudge remains
// reachable for a run that has mutated.
func TestImplementUnmutatedRunStopsBeforeDiscoveryNudge(t *testing.T) {
	dir := t.TempDir()
	seedToolFiles(t, dir, "pkg", implementNudgeAfter)
	responses := distinctToolCalls(implementNudgeAfter)
	responses = append(responses, `{"status":"completed","summary":"done","changes_expected":false}`)
	fake, srv := newFakeOllama(t, responses...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 100

	h := New(cfg, dir)
	_, err := h.Execute(context.Background(), implementRequest())
	var stalled *noProgressError
	if !errors.As(err, &stalled) {
		t.Fatalf("err = %v, want *noProgressError", err)
	}
	for i := 0; i < fake.count(); i++ {
		if strings.Contains(messageText(fake.request(i)), "Begin making the requested change now") {
			t.Errorf("the discovery nudge was sent to a run stopped for no progress:\n%s", messageText(fake.request(i)))
		}
	}
	if hasEvent(h.TraceRecords(), implementChangeEvent) {
		t.Error("plain discovery is not a mutation and must not enter CHANGE")
	}
}

func TestImplementMutationTransitionsToChange(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "notes.txt", "hello")
	fake, srv := newFakeOllama(t,
		`{"tool":"read_file","args":{"path":"notes.txt"}}`,
		`{"tool":"write_file","args":{"path":"out.txt","content":"x"}}`,
		`{"status":"completed","summary":"done","changes_expected":true}`,
	)
	h := New(testConfig(srv.URL), dir)
	if _, err := h.Execute(context.Background(), implementRequest()); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	records := h.TraceRecords()
	if !hasEvent(records, implementChangeEvent) {
		t.Errorf("a successful write must move to CHANGE: %+v", records)
	}
	if got := countPhase(records, "DISCOVER"); got != 1 {
		t.Errorf("discovery turns = %d, want 1", got)
	}
	if text := messageText(fake.request(2)); !strings.Contains(text, "The requested change has begun") {
		t.Errorf("the change guidance was not sent:\n%s", text)
	}
}

func TestImplementFailedMutationNotCounted(t *testing.T) {
	dir := t.TempDir()
	_, srv := newFakeOllama(t,
		`{"tool":"write_file","args":{"path":"../escape.txt","content":"x"}}`,
		`{"status":"completed","summary":"nothing written","changes_expected":false}`,
	)
	h := New(testConfig(srv.URL), dir)
	if _, err := h.Execute(context.Background(), implementRequest()); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if hasEvent(h.TraceRecords(), implementChangeEvent) {
		t.Error("a failed write is not a mutation")
	}
}

func TestImplementDeniedMutationNotCounted(t *testing.T) {
	dir := t.TempDir()
	_, srv := newFakeOllama(t,
		`{"tool":"write_file","args":{"path":".agent-sdlc/state.db","content":"x"}}`,
		`{"status":"completed","summary":"denied","changes_expected":false}`,
	)
	h := New(testConfig(srv.URL), dir)
	if _, err := h.Execute(context.Background(), implementRequest()); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if hasEvent(h.TraceRecords(), implementChangeEvent) {
		t.Error("a denied write is not a mutation")
	}
	denied := 0
	for _, r := range h.AuditRecords() {
		if r.Action == toolharness.ActionDeny {
			denied++
		}
	}
	if denied != 1 {
		t.Errorf("denied audit records = %d, want 1", denied)
	}
}

func TestImplementForcedFinalization(t *testing.T) {
	dir := t.TempDir()
	// A mutation must be observed before the threshold withdraws the tools, so the
	// fixture writes once, then continues targeted reads to the threshold.
	responses := []string{`{"tool":"write_file","args":{"path":"out.txt","content":"x"}}`}
	responses = append(responses, distinctToolCalls(implementFinalizeAfter-1)...)
	responses = append(responses, `{"status":"completed","summary":"done","changes_expected":true}`)
	fake, srv := newFakeOllama(t, responses...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 100

	h := New(cfg, dir)
	if _, err := h.Execute(context.Background(), implementRequest()); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	records := h.TraceRecords()
	if !hasEvent(records, implementFinalizeEvent) {
		t.Errorf("finalization was not entered: %+v", records)
	}
	if got := countPhase(records, "FINALIZE"); got != 1 {
		t.Errorf("finalize turns = %d, want 1 (the outcome)", got)
	}
	if text := messageText(fake.request(implementFinalizeAfter)); !strings.Contains(text, "Work on the requested change is complete for this invocation") {
		t.Errorf("the finalize instruction was not sent:\n%s", text)
	}
	if got := len(h.AuditRecords()); got != implementFinalizeAfter {
		t.Errorf("executed tools = %d, want %d (tools withdrawn during FINALIZE)", got, implementFinalizeAfter)
	}
}

func TestImplementToolRequestDuringFinalizationIsDenied(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "notes.txt", "hello")
	responses := []string{`{"tool":"write_file","args":{"path":"out.txt","content":"x"}}`}
	responses = append(responses, distinctToolCalls(implementFinalizeAfter-1)...)
	responses = append(responses,
		`{"tool":"read_file","args":{"path":"notes.txt"}}`,
		`{"status":"completed","summary":"done","changes_expected":true}`,
	)
	fake, srv := newFakeOllama(t, responses...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 100

	h := New(cfg, dir)
	content, err := h.Execute(context.Background(), implementRequest())
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(content, `"status":"completed"`) {
		t.Errorf("content = %q", content)
	}
	denied := 0
	for _, r := range h.AuditRecords() {
		if r.Action == toolharness.ActionDeny {
			denied++
		}
	}
	if denied != 1 {
		t.Errorf("denied records = %d, want 1 (the finalize tool request)", denied)
	}
	if got := len(h.AuditRecords()); got != implementFinalizeAfter+1 {
		t.Errorf("audit records = %d, want %d executed + 1 denied", got, implementFinalizeAfter)
	}
	if text := messageText(fake.request(implementFinalizeAfter + 1)); !strings.Contains(text, "No additional tools are available") {
		t.Errorf("the finalize correction was not sent:\n%s", text)
	}
	sawDenied := false
	for _, r := range h.TraceRecords() {
		if r.Progress == progressDenied {
			sawDenied = true
		}
	}
	if !sawDenied {
		t.Error("the trace does not show a denied finalize turn")
	}
}

// TestImplementUnmutatedRunStopsWithNoProgress is the AHV2008 shape minus the
// mutation: substantial discovery with no change yet no longer reaches the finalize
// threshold or receives the implement-now steering — the repository no-progress
// guard stops it early instead. The steering below is reachable only once a run has
// changed the repository.
func TestImplementUnmutatedRunStopsWithNoProgress(t *testing.T) {
	dir := t.TempDir()
	seedToolFiles(t, dir, "pkg", implementFinalizeAfter+2)
	responses := distinctToolCalls(implementFinalizeAfter) // discovery reads, no mutation
	fake, srv := newFakeOllama(t, responses...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 100

	_, err := New(cfg, dir).Execute(context.Background(), implementRequest())
	var stalled *noProgressError
	if !errors.As(err, &stalled) {
		t.Fatalf("err = %v, want *noProgressError", err)
	}
	if got := fake.count(); got >= implementFinalizeAfter {
		t.Errorf("chat calls = %d, want well under the finalize threshold %d", got, implementFinalizeAfter)
	}
}

// TestImplementFinalizationExhaustionAfterMutation covers a model that mutates,
// reaches the threshold, then keeps asking for tools during FINALIZE: it is
// stopped with a phase-specific reason rather than the generic iteration limit.
func TestImplementFinalizationExhaustionAfterMutation(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "notes.txt", "hello")
	responses := []string{`{"tool":"write_file","args":{"path":"out.txt","content":"x"}}`}
	responses = append(responses, distinctToolCalls(implementFinalizeAfter-1)...)
	responses = append(responses,
		`{"tool":"read_file","args":{"path":"notes.txt"}}`,
		`{"tool":"read_file","args":{"path":"notes.txt"}}`,
		`{"tool":"read_file","args":{"path":"notes.txt"}}`,
	)
	_, srv := newFakeOllama(t, responses...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 100

	_, err := New(cfg, dir).Execute(context.Background(), implementRequest())
	if err == nil || !strings.Contains(err.Error(), "termination=finalization_limit") {
		t.Fatalf("err = %v, want a finalization_limit termination", err)
	}
	if !strings.Contains(err.Error(), "mutation_observed=true") ||
		!strings.Contains(err.Error(), "finalization_turns=3") {
		t.Errorf("err = %v, want the mutation and finalization counts", err)
	}
}

// TestImplementWritingPastThresholdIsNotFinalized covers the AHV2009 shape: a model
// that keeps writing a multi-file change past the finalize threshold must keep its
// tools, because an invocation that is still mutating has not finished. It is only
// finalized once it stops writing (here, by returning its outcome). Writes are
// interleaved with reads so the run never spends more than the repository
// no-progress bound without changing the repository.
func TestImplementWritingPastThresholdIsNotFinalized(t *testing.T) {
	dir := t.TempDir()
	var responses []string
	for i := 0; i < 12; i++ {
		responses = append(responses,
			readToolCall(i),
			fmt.Sprintf(`{"tool":"write_file","args":{"path":"w%d.txt","content":"x"}}`, i),
		)
	}
	responses = append(responses, `{"status":"completed","summary":"implemented it all","changes_expected":true}`)
	_, srv := newFakeOllama(t, responses...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 100

	h := New(cfg, dir)
	content, err := h.Execute(context.Background(), implementRequest())
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(content, `"status":"completed"`) {
		t.Errorf("content = %q", content)
	}
	records := h.TraceRecords()
	if hasEvent(records, implementFinalizeEvent) {
		t.Errorf("a model still writing must not be finalized: %+v", records)
	}
	// Every write executed; none was refused as a finalize turn.
	if got := len(h.AuditRecords()); got != 24 {
		t.Errorf("executed tools = %d, want 24", got)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "w11.txt")); string(got) != "x" {
		t.Errorf("w11.txt = %q, want the final write applied", got)
	}
}

// TestImplementMutationDuringFinalizationIsDenied covers the other half: once the
// run has been finalized its tools are withdrawn, so a mutation requested during
// FINALIZE is denied rather than honoured, and the invocation does not resume
// CHANGE. The model still returns its structured outcome.
func TestImplementMutationDuringFinalizationIsDenied(t *testing.T) {
	dir := t.TempDir()

	responses := []string{
		`{"tool":"write_file","args":{"path":"out.txt","content":"x"}}`,
	}
	responses = append(responses, distinctToolCalls(17)...)
	responses = append(responses,
		`{"tool":"write_file","args":{"path":"more.txt","content":"y"}}`,
		`{"status":"completed","summary":"finished the change","changes_expected":true}`,
	)

	_, srv := newFakeOllama(t, responses...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 100

	h := New(cfg, dir)

	content, err := h.Execute(
		context.Background(),
		implementRequest(),
	)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !strings.Contains(content, `"status":"completed"`) {
		t.Errorf("content = %q", content)
	}

	denied := false
	for _, r := range h.AuditRecords() {
		if r.Action == toolharness.ActionDeny &&
			r.Tool == toolharness.ToolWriteFile {
			denied = true
		}
	}

	if !denied {
		t.Error("write_file during FINALIZE was not denied")
	}

	if _, err := os.Stat(filepath.Join(dir, "more.txt")); !os.IsNotExist(err) {
		t.Errorf("more.txt should not have been created during FINALIZE")
	}

	for _, r := range h.TraceRecords() {
		if r.Event == implementChangeEvent &&
			r.Detail == "resumed for a further mutation" {
			t.Error("FINALIZE must not transition back to CHANGE")
		}
	}
}

// TestImplementNonCompletedOutcomesNeedNoMutation verifies that truthful
// escalation or failure outcomes are accepted without any repository change: the
// mutation requirement applies to claiming success, not to honest non-completion.
func TestImplementNonCompletedOutcomesNeedNoMutation(t *testing.T) {
	for _, tc := range []struct{ name, response string }{
		{"needs_human", `{"status":"needs_human","reason":"needs authorization"}`},
		{"failed", `{"status":"failed","reason":"cannot complete"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			_, srv := newFakeOllama(t, tc.response)
			content, err := New(testConfig(srv.URL), dir).Execute(context.Background(), implementRequest())
			if err != nil {
				t.Fatalf("Execute failed: %v", err)
			}
			if !strings.Contains(content, `"status":"`+tc.name+`"`) {
				t.Errorf("content = %q, want a %s outcome", content, tc.name)
			}
		})
	}
}

// TestImplementEarlyMutationBeforeThresholdSucceeds covers a model that changes
// the repository well before the finalize threshold and then returns an outcome:
// the threshold is a maximum, not a minimum amount of work.
func TestImplementEarlyMutationBeforeThresholdSucceeds(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "notes.txt", "hello")
	_, srv := newFakeOllama(t,
		`{"tool":"read_file","args":{"path":"notes.txt"}}`,
		`{"tool":"write_file","args":{"path":"out.txt","content":"x"}}`,
		`{"tool":"git_diff","args":{}}`,
		`{"status":"completed","summary":"done","changes_expected":true}`,
	)
	h := New(testConfig(srv.URL), dir)
	content, err := h.Execute(context.Background(), implementRequest())
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(content, `"status":"completed"`) {
		t.Errorf("content = %q", content)
	}
	if hasEvent(h.TraceRecords(), implementFinalizeEvent) {
		t.Error("an early completion must not be forced to finalize")
	}
}

func TestImplementInvocationIsolation(t *testing.T) {
	dir := t.TempDir()
	_, srv := newFakeOllama(t,
		`{"tool":"write_file","args":{"path":"out.txt","content":"x"}}`,
		`{"status":"completed","summary":"mutated","changes_expected":true}`,
		`{"tool":"read_file","args":{"path":"out.txt"}}`,
		`{"status":"completed","summary":"no change","changes_expected":false}`,
	)
	h := New(testConfig(srv.URL), dir)
	if _, err := h.Execute(context.Background(), implementRequest()); err != nil {
		t.Fatalf("first Execute failed: %v", err)
	}
	first := h.TraceRecords()
	if !hasEvent(first, implementChangeEvent) {
		t.Error("the first invocation mutated and should enter CHANGE")
	}
	if _, err := h.Execute(context.Background(), implementRequest()); err != nil {
		t.Fatalf("second Execute failed: %v", err)
	}
	second := h.TraceRecords()[len(first):]
	if hasEvent(second, implementChangeEvent) {
		t.Error("the second invocation inherited mutation state from the first")
	}
}

func TestImplementTraceRendering(t *testing.T) {
	dir := t.TempDir()
	responses := []string{readToolCall(0), readToolCall(1)}
	responses = append(responses, `{"tool":"write_file","args":{"path":"out.txt","content":"x"}}`)
	responses = append(responses, distinctToolCalls(implementFinalizeAfter-3)...)
	responses = append(responses, `{"status":"completed","summary":"done","changes_expected":true}`)
	_, srv := newFakeOllama(t, responses...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 100

	h := New(cfg, dir)
	if _, err := h.Execute(context.Background(), implementRequest()); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	var b strings.Builder
	h.FlushTrace(&b)
	out := b.String()
	t.Logf("trace:\n%s", out)
	for _, want := range []string{
		"IMPLEMENT DISCOVER #1 read_file path=pkg/f0.go [ok]",
		"IMPLEMENT → CHANGE",
		"IMPLEMENT CHANGE #3 write_file path=out.txt [ok]",
		"IMPLEMENT → FINALIZE",
		"IMPLEMENT FINALIZE #19 outcome [ok]",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("trace missing %q:\n%s", want, out)
		}
	}
}

// TestImplementAHV2008ShapeFixture approximates AHV2008: IMPLEMENT inspects the
// repository, makes a change, keeps doing targeted inspection, enters FINALIZE,
// and returns a structured outcome — so SOP regains control instead of receiving
// "IMPLEMENT did not complete after 24 iterations". No live provider is used and
// real SOP state is never touched.
func TestImplementAHV2008ShapeFixture(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "internal/cli/cli.go", "package cli\n// DefaultAgent\n")
	writeFile(t, dir, "internal/agent/agent.go", "package agent\n// Capability\n")
	responses := []string{
		`{"tool":"list_files","args":{"path":"."}}`,
		`{"tool":"search_files","args":{"pattern":"DefaultAgent"}}`,
		`{"tool":"read_file","args":{"path":"internal/cli/cli.go"}}`,
		`{"tool":"read_file","args":{"path":"internal/agent/agent.go"}}`,
		`{"tool":"write_file","args":{"path":"internal/cli/cli.go","content":"package cli\n// DefaultAgent selected\n"}}`,
	}
	responses = append(responses, distinctToolCalls(implementFinalizeAfter-5)...)
	responses = append(responses, `{"status":"completed","summary":"Implemented default agent selection.","changes_expected":true}`)
	_, srv := newFakeOllama(t, responses...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 100

	h := New(cfg, dir)
	content, err := h.Execute(context.Background(), implementRequest())
	if err != nil {
		t.Fatalf("IMPLEMENT did not return control to SOP: %v", err)
	}
	if !strings.Contains(content, `"status":"completed"`) {
		t.Errorf("content = %q, want a completed outcome", content)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "internal/cli/cli.go")); !strings.Contains(string(got), "DefaultAgent selected") {
		t.Errorf("cli.go = %q, want the mutation applied", got)
	}
	records := h.TraceRecords()
	if !hasEvent(records, implementChangeEvent) || !hasEvent(records, implementFinalizeEvent) {
		t.Errorf("trace missing phase transitions: %+v", records)
	}
}

// TestFixUsesPhasedCompletion proves FIX shares the phased loop: a fix that makes
// the change and then keeps verifying must still finalize and hand back before the
// ceiling, instead of running to iteration_limit (the AHV2009 fix failure).
func TestFixUsesPhasedCompletion(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "notes.txt", "hello")
	responses := []string{
		`{"tool":"read_file","args":{"path":"notes.txt"}}`,
		`{"tool":"write_file","args":{"path":"out.txt","content":"x"}}`,
	}
	responses = append(responses, distinctToolCalls(implementFinalizeAfter-2)...)
	responses = append(responses, `{"status":"completed","summary":"fixed it","changes_expected":true}`)
	_, srv := newFakeOllama(t, responses...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 100

	h := New(cfg, dir)
	content, err := h.Execute(context.Background(), fixRequest())
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(content, `"status":"completed"`) {
		t.Errorf("content = %q", content)
	}
	records := h.TraceRecords()
	for _, r := range records {
		if r.Capability != string(agent.Fix) {
			t.Errorf("trace record capability = %q, want FIX", r.Capability)
		}
	}
	if !hasEvent(records, implementFinalizeEvent) {
		t.Errorf("FIX must finalize before the ceiling: %+v", records)
	}
}

func TestFixForceFinalizesBeforeIterationLimit(t *testing.T) {
	dir := t.TempDir()

	// Keep FIX actively mutating so normal completion-window finalization
	// never becomes eligible. The capability-specific force-finalize threshold
	// must still withdraw tools before maxIterationsFix is reached.
	responses := make([]string, 0, fixForceFinalizeAfter+3)

	for i := 0; i < fixForceFinalizeAfter; i++ {
		responses = append(responses, fmt.Sprintf(
			`{"tool":"write_file","args":{"path":"fix-%d.txt","content":"mutation-%d"}}`,
			i,
			i,
		))
	}

	// Once forced into FINALIZE, mutation tools must be denied.
	responses = append(responses,
		`{"tool":"write_file","args":{"path":"too-late.txt","content":"must not be written"}}`,
		`{"status":"completed","summary":"fix complete","changes_expected":true}`,
	)

	fake, srv := newFakeOllama(t, responses...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 100

	h := New(cfg, dir)
	content, err := h.Execute(context.Background(), fixRequest())
	if err != nil {
		t.Fatalf("FIX did not return control before iteration limit: %v", err)
	}

	if !strings.Contains(content, `"status":"completed"`) {
		t.Errorf("content = %q, want completed outcome", content)
	}

	if fake.count() >= maxIterationsFix {
		t.Errorf(
			"FIX used %d model turns, want fewer than maxIterationsFix=%d",
			fake.count(),
			maxIterationsFix,
		)
	}

	records := h.TraceRecords()
	if !hasEvent(records, implementFinalizeEvent) {
		t.Errorf("FIX never entered FINALIZE: %+v", records)
	}

	denied := false
	for _, r := range h.AuditRecords() {
		if r.Action == toolharness.ActionDeny &&
			r.Tool == toolharness.ToolWriteFile {
			denied = true
			break
		}
	}

	if !denied {
		t.Error("write_file requested during FIX FINALIZE was not denied")
	}

	if _, err := os.Stat(filepath.Join(dir, "too-late.txt")); !os.IsNotExist(err) {
		t.Error("too-late.txt should not exist; mutation during FINALIZE must be denied")
	}
}

// --- No-progress detection ---

func TestRepeatedToolActionStopsEarly(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "notes.txt", "stable")
	repeat := `{"tool":"read_file","args":{"path":"notes.txt"}}`
	fake, srv := newFakeOllama(t, repeat, repeat, repeat, repeat, repeat, repeat, repeat, repeat)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 100

	h := New(cfg, dir)
	_, err := h.Execute(context.Background(), implementRequest())
	if err == nil || !strings.Contains(err.Error(), "termination=no_progress") {
		t.Fatalf("err = %v, want a no-progress termination", err)
	}
	if fake.count() >= maxIterationsImplement {
		t.Errorf("chat calls = %d, want an early stop well under the 24-iteration budget", fake.count())
	}
	// The recovery instruction reached the model before it was stopped.
	if last := fake.request(fake.count() - 1); !strings.Contains(messageText(last), "repeating actions without making progress") {
		t.Errorf("the recovery instruction was not sent:\n%s", messageText(last))
	}
}

func TestProgressAfterRepetitionContinues(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.txt", "one")
	writeFile(t, dir, "b.txt", "two")
	fake, srv := newFakeOllama(t,
		`{"tool":"read_file","args":{"path":"a.txt"}}`,
		`{"tool":"read_file","args":{"path":"a.txt"}}`,
		`{"tool":"read_file","args":{"path":"b.txt"}}`,
		`{"status":"completed","summary":"done","changes_expected":false}`,
	)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 10
	content, err := New(cfg, dir).Execute(context.Background(), implementRequest())
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(content, "completed") {
		t.Errorf("content = %q", content)
	}
	if fake.count() != 4 {
		t.Errorf("chat calls = %d, want 4 (a single repeat is not a loop)", fake.count())
	}
}

func TestTraceIsSafeAndRecordsTurns(t *testing.T) {
	dir := t.TempDir()
	secret := "SUPER_SECRET_VALUE"
	_, srv := newFakeOllama(t,
		fmt.Sprintf(`{"tool":"write_file","args":{"path":"out.txt","content":%q}}`, secret),
		`{"status":"completed","summary":"done","changes_expected":true}`,
	)
	h := New(testConfig(srv.URL), dir)
	if _, err := h.Execute(context.Background(), implementRequest()); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	records := h.TraceRecords()
	if len(records) != 3 {
		t.Fatalf("trace records = %d, want 3 (change transition, write, outcome): %+v", len(records), records)
	}
	if !hasEvent(records, implementChangeEvent) {
		t.Errorf("trace missing the change transition: %+v", records)
	}
	var write *TraceRecord
	for i := range records {
		if records[i].Tool == toolharness.ToolWriteFile {
			write = &records[i]
		}
	}
	if write == nil || write.Phase != "CHANGE" || write.Iteration != 1 || write.Progress != progressOK {
		t.Errorf("write trace record = %+v", write)
	}
	last := records[len(records)-1]
	if last.Tool != "outcome" || last.Phase != "CHANGE" {
		t.Errorf("final trace record = %+v, want the outcome under CHANGE", last)
	}
	for _, r := range records {
		if strings.Contains(r.Request, secret) {
			t.Errorf("trace leaked file content: %+v", r)
		}
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "out.txt")); string(got) != secret {
		t.Errorf("out.txt = %q", got)
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

func TestNarrationIsNudgedNotFatal(t *testing.T) {
	dir := t.TempDir()
	fake, srv := newFakeOllama(t,
		"I am not JSON at all",
		`{"status":"completed","summary":"ok","changes_expected":false}`,
	)
	content, err := New(testConfig(srv.URL), dir).Execute(context.Background(), implementRequest())
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(content, `"status":"completed"`) {
		t.Errorf("content = %q", content)
	}
	if fake.count() != 2 {
		t.Errorf("chat calls = %d, want 2 (the narration was nudged)", fake.count())
	}
}

func TestRepeatedNarrationStopsEarly(t *testing.T) {
	dir := t.TempDir()
	narration := "I will think about this some more."
	_, srv := newFakeOllama(t, narration, narration, narration, narration, narration, narration, narration, narration)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 100

	_, err := New(cfg, dir).Execute(context.Background(), implementRequest())
	if err == nil || !strings.Contains(err.Error(), "termination=no_progress") {
		t.Fatalf("err = %v, want an early no-progress termination", err)
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

func TestRunEnsuresProseBecomeStructuredOutcome(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	writeFile(t, dir, "file.txt", "initial")
	// The model returns JSON that is not an outcome (not recognized), which looks like prose.
	// ensureStructuredOutcome should wrap it in a completed outcome.
	_, srv := newFakeOllama(t,
		`{"tool":"write_file","args":{"path":"file.txt","content":"changed"}}`,
		`{"plan":"Step 1: Modify the file"}`,
	)
	t.Setenv(agent.EnvOllamaBaseURL, srv.URL)
	t.Setenv(agent.EnvOllamaModel, "")
	t.Setenv(agent.EnvOllamaTimeout, "")

	body := `{"capability":"IMPLEMENT","task":"do it","input":"ctx"}`
	var out, errOut bytes.Buffer
	if err := Run(context.Background(), strings.NewReader(body), &out, &errOut, func() (string, error) { return dir, nil }); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Non-outcome JSON (or prose) should be wrapped as a structured outcome
	outStr := out.String()
	if !strings.Contains(outStr, `"status":"completed"`) {
		t.Errorf("out = %q, want status=completed", outStr)
	}
}

func TestRunWrapsProseInOutcomeWithNoChanges(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	// The model returns prose but makes no changes: wraps as completed with changes_expected=false
	_, srv := newFakeOllama(t, `Nothing needed to be changed.`)
	t.Setenv(agent.EnvOllamaBaseURL, srv.URL)
	t.Setenv(agent.EnvOllamaModel, "")
	t.Setenv(agent.EnvOllamaTimeout, "")

	body := `{"capability":"IMPLEMENT","task":"do it","input":"ctx"}`
	var out, errOut bytes.Buffer
	if err := Run(context.Background(), strings.NewReader(body), &out, &errOut, func() (string, error) { return dir, nil }); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	outStr := out.String()
	if !strings.Contains(outStr, `"status":"completed"`) {
		t.Errorf("out = %q, want status=completed", outStr)
	}
	if !strings.Contains(outStr, `"changes_expected":false`) {
		t.Errorf("out = %q, want changes_expected=false since no change was made", outStr)
	}
}

// gitInit makes dir a git repository, so the harness's working-tree observation
// (used to reconcile a claimed change) has something to inspect.
func gitInit(t *testing.T, dir string) {
	t.Helper()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, out)
		}
	}
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v (%s)", err, out)
	}
	run("-c", "user.email=test@example.com", "-c", "user.name=test", "commit", "--allow-empty", "-q", "-m", "initial")
}
