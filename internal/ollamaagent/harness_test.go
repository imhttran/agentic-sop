package ollamaagent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

func planRequest() agent.Request {
	return agent.Request{Capability: agent.Plan, Task: "make a plan", Input: "prd", OutputRequirements: "json"}
}

// distinctToolCalls returns n distinct read_file calls, so a model that never
// finishes performs genuinely different actions rather than a detected loop.
func distinctToolCalls(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf(`{"tool":"read_file","args":{"path":"pkg/f%d.go"}}`, i)
	}
	return out
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

func TestPlanStopsAtCapabilityBudget(t *testing.T) {
	dir := t.TempDir()
	_, srv := newFakeOllama(t, distinctToolCalls(12)...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 100

	_, err := New(cfg, dir).Execute(context.Background(), planRequest())
	if err == nil || !strings.Contains(err.Error(), "termination=iteration_limit") {
		t.Fatalf("err = %v, want an iteration_limit termination", err)
	}
	if !strings.Contains(err.Error(), "after 8 iterations") {
		t.Errorf("err = %v, want PLAN capped at its 8-iteration budget", err)
	}
}

func TestImplementStopsAtCapabilityBudget(t *testing.T) {
	dir := t.TempDir()
	_, srv := newFakeOllama(t, distinctToolCalls(maxIterationsImplement+4)...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 100

	_, err := New(cfg, dir).Execute(context.Background(), implementRequest())
	if err == nil || !strings.Contains(err.Error(), "termination=iteration_limit") {
		t.Fatalf("err = %v, want an iteration_limit termination", err)
	}
	if !strings.Contains(err.Error(), "after 24 iterations") {
		t.Errorf("err = %v, want IMPLEMENT capped at 24, not the old universal 48", err)
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
	cases := []struct {
		cap  agent.Capability
		want int
	}{
		{agent.Plan, 8},
		{agent.DesignTests, 12},
		{agent.Implement, 24},
		{agent.Fix, 24},
		{agent.Review, 12},
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
	if len(records) != 1 {
		t.Fatalf("trace records = %d, want 1", len(records))
	}
	if records[0].Tool != toolharness.ToolWriteFile || records[0].Iteration != 1 || records[0].Progress != progressOK {
		t.Errorf("trace record = %+v", records[0])
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
