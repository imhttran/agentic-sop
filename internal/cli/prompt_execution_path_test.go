package cli

// Execution-path verification for the independent design-review experiment
// (docs/reports/INDEPENDENT-DESIGN-REVIEW-EXECUTION-PATH-VERIFICATION.md).
//
// These tests drive `sop prompt --capability review --model-class medium --file
// request.md --json` through the PRODUCTION agent factory (defaultDeps) against
// synthetic local HTTP servers. No real provider, credential or model is used.
// They establish, at runtime, what the experiment protocol relies on: which model
// name reaches the provider, how usage is reported, whether repository tools are
// reachable, and how run directories are allocated.

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/perf"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
)

const verifyModel = "Qwen3-4B-4bit"

// verifyConfig selects provider/harness for the model class the request names.
func verifyConfig(provider, model string) string {
	class := func(name string) string {
		return "  " + name + ":\n    provider: " + provider + "\n    name: " + model + "\n    locality: local\n"
	}
	return "version: 1\nproject:\n  name: verify\n  integration_branch: main\nagent:\n  harness: tool\n  provider: " +
		provider + "\n  model: " + model + "\nmodels:\n  default_class: medium\n  fallback_class: medium\n" +
		class("small") + class("medium") + class("large")
}

// verifyProject creates a project with a config and a request file.
func verifyProject(t *testing.T, provider, model string) string {
	t.Helper()
	dir := t.TempDir()
	writeProjectConfig(t, dir, verifyConfig(provider, model))
	writeFile(t, dir, "request.md", "Review the synthetic plan below.\n\nPLAN: do the thing.\n")
	return dir
}

// writeProjectConfig writes .agent-sdlc/config.yaml in dir.
func writeProjectConfig(t *testing.T, dir, yaml string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, stateDirName), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join(stateDirName, "config.yaml"), yaml)
}

// capture records the JSON bodies a fake provider received.
type capture struct {
	mu     sync.Mutex
	bodies []map[string]any
}

func (c *capture) add(b map[string]any) { c.mu.Lock(); c.bodies = append(c.bodies, b); c.mu.Unlock() }

// record decodes a request body into the capture.
func (c *capture) record(r *http.Request) {
	raw, _ := io.ReadAll(r.Body)
	var body map[string]any
	_ = json.Unmarshal(raw, &body)
	c.add(body)
}

func (c *capture) count() int { c.mu.Lock(); defer c.mu.Unlock(); return len(c.bodies) }
func (c *capture) at(i int) map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.bodies[i]
}

// fakeMLX serves /v1/chat/completions (the text-only MLX/OpenAI-compatible path).
// With usage=false the response carries no usage object.
func fakeMLX(t *testing.T, reply string, usage bool) (*httptest.Server, *capture) {
	t.Helper()
	cap := &capture{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		cap.record(r)
		resp := map[string]any{"choices": []map[string]any{{
			"message": map[string]any{"role": "assistant", "content": reply}, "finish_reason": "stop"}}}
		if usage {
			resp["usage"] = map[string]any{"prompt_tokens": 120, "completion_tokens": 30}
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	return srv, cap
}

// fakeOllama serves /api/chat (the native tool-loop path) with scripted turns.
func fakeOllama(t *testing.T, turns ...string) (*httptest.Server, *capture) {
	t.Helper()
	cap := &capture{}
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}
		cap.record(r)
		mu.Lock()
		content := `{"findings":[],"summary":"default"}`
		if len(turns) > 0 {
			content, turns = turns[0], turns[1:]
		}
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"message":           map[string]any{"role": "assistant", "content": content},
			"done":              true,
			"done_reason":       "stop",
			"prompt_eval_count": 50,
			"eval_count":        20,
		})
	}))
	t.Cleanup(srv.Close)
	return srv, cap
}

// runRealPrompt runs the CLI with the production agent factory.
func runRealPrompt(t *testing.T, dir string, args ...string) (int, string, string) {
	t.Helper()
	d := defaultDeps()
	d.getwd = func() (string, error) { return dir, nil }
	var out, errOut bytes.Buffer
	code := run(args, &out, &errOut, d)
	return code, out.String(), errOut.String()
}

func reviewArgs() []string {
	return []string{"prompt", "--capability", "review", "--model-class", "medium", "--file", "request.md", "--json"}
}

func decodeDoc(t *testing.T, stdout string) promptResultDoc {
	t.Helper()
	var doc promptResultDoc
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("stdout is not a result document: %v\n%s", err, stdout)
	}
	return doc
}

func sentModel(c *capture, i int) string {
	m, _ := c.at(i)["model"].(string)
	return m
}

// TestVerifyMLXPromptIdentityAndUsage: on the text-only MLX path the routed model is
// the model name sent to the provider, usage is recorded under that name, the
// reviewer has no tools, and the run leaves exactly one consistent run directory.
func TestVerifyMLXPromptIdentityAndUsage(t *testing.T) {
	clearProviderEnv(t)
	dir := verifyProject(t, "mlx", verifyModel)
	srv, cap := fakeMLX(t, "synthetic review", true)
	t.Setenv("SOP_MLX_BASE_URL", srv.URL)

	code, stdout, stderr := runRealPrompt(t, dir, reviewArgs()...)
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	doc := decodeDoc(t, stdout)
	if doc.Status != "completed" || doc.Routing == nil || doc.Routing.Model != verifyModel || doc.Routing.Provider != "mlx" {
		t.Fatalf("routing = %+v status=%s", doc.Routing, doc.Status)
	}
	if cap.count() != 1 || sentModel(cap, 0) != verifyModel {
		t.Fatalf("provider saw %d calls, model %q; want 1 call for %q", cap.count(), sentModel(cap, 0), verifyModel)
	}
	if _, hasTools := cap.at(0)["tools"]; hasTools {
		t.Error("text-only provider request must not advertise tools")
	}
	if got := doc.Tokens[verifyModel]; got != (perf.Tokens{Calls: 1, Input: 120, Output: 30}) {
		t.Errorf("tokens = %+v", doc.Tokens)
	}
	runs := promptRunDirs(t, dir)
	if len(runs) != 1 {
		t.Fatalf("run dirs = %v", runs)
	}
	meta, err := os.ReadFile(filepath.Join(runs[0], "metadata.json"))
	if err != nil || !strings.Contains(string(meta), doc.WorkItemID) || filepath.Base(runs[0]) != doc.WorkItemID {
		t.Errorf("metadata/work item id disagree with run dir %s: %v", runs[0], err)
	}
}

// TestVerifyMLXUnreportedUsageKeepsModelKey: a server that omits usage still produces
// a tokens entry under the model name, with zero tokens and unreported_calls = 1.
// A check that only looks for the key would therefore pass with no cost data.
func TestVerifyMLXUnreportedUsageKeepsModelKey(t *testing.T) {
	clearProviderEnv(t)
	dir := verifyProject(t, "mlx", verifyModel)
	srv, _ := fakeMLX(t, "synthetic review", false)
	t.Setenv("SOP_MLX_BASE_URL", srv.URL)

	code, stdout, stderr := runRealPrompt(t, dir, reviewArgs()...)
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	got, ok := decodeDoc(t, stdout).Tokens[verifyModel]
	if !ok || got != (perf.Tokens{Calls: 1, UnreportedCalls: 1}) {
		t.Errorf("tokens = %+v (present=%v), want one call, zero tokens, one unreported call", got, ok)
	}
}

// TestVerifyModelOverrideChangesRequestedModel: environment overrides change the
// model name SOP sends while metadata.routing.model still records the routed model.
// The experiment's requested-model check detects this through the tokens key.
func TestVerifyModelOverrideChangesRequestedModel(t *testing.T) {
	for _, env := range []string{agent.EnvAgentModel, agent.EnvMLXModel} {
		t.Run(env, func(t *testing.T) {
			clearProviderEnv(t)
			dir := verifyProject(t, "mlx", verifyModel)
			srv, cap := fakeMLX(t, "synthetic review", true)
			t.Setenv("SOP_MLX_BASE_URL", srv.URL)
			t.Setenv(env, "override-model")

			code, stdout, stderr := runRealPrompt(t, dir, reviewArgs()...)
			if code != exitOK {
				t.Fatalf("code=%d stderr=%s", code, stderr)
			}
			doc := decodeDoc(t, stdout)
			t.Logf("%s: routing.model=%q provider received %q tokens keys=%v", env, doc.Routing.Model, sentModel(cap, 0), keys(doc.Tokens))
			if doc.Routing.Model != verifyModel {
				t.Errorf("routing.model = %q, want the routed %q", doc.Routing.Model, verifyModel)
			}
			if sentModel(cap, 0) != "override-model" {
				t.Errorf("provider received %q, want the override", sentModel(cap, 0))
			}
			if _, ok := doc.Tokens["override-model"]; !ok || len(doc.Tokens) != 1 {
				t.Errorf("tokens keys = %v, want the sent model name", keys(doc.Tokens))
			}
		})
	}
}

// TestVerifyDotEnvCanInjectModelOverride: a project .env is applied for the prompt
// command, so a .env in an experiment project can change the model sent.
func TestVerifyDotEnvCanInjectModelOverride(t *testing.T) {
	clearProviderEnv(t)
	t.Cleanup(func() { _ = os.Unsetenv(agent.EnvAgentModel) }) // dotenv.Apply sets process env
	dir := verifyProject(t, "mlx", verifyModel)
	writeFile(t, dir, ".env", agent.EnvAgentModel+"=dotenv-model\n")
	srv, cap := fakeMLX(t, "synthetic review", true)
	t.Setenv("SOP_MLX_BASE_URL", srv.URL)

	code, stdout, stderr := runRealPrompt(t, dir, reviewArgs()...)
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if got := sentModel(cap, 0); got != "dotenv-model" {
		t.Errorf("provider received %q, want the .env override", got)
	}
	if m := decodeDoc(t, stdout).Routing.Model; m != verifyModel {
		t.Errorf("routing.model = %q, want the routed %q", m, verifyModel)
	}
}

func keys(m map[string]perf.Tokens) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

// nativeProject prepares a native-harness project (harness tool, provider ollama).
func nativeProject(t *testing.T) string {
	t.Helper()
	clearProviderEnv(t)
	dir := verifyProject(t, "ollama", "native-model")
	writeFile(t, dir, "notes.txt", "SYNTHETIC-MARKER-7f3a\n")
	t.Chdir(dir) // the native agent roots its tools at the process working directory
	return dir
}

// TestVerifyNativeReviewEarlyFinalIsOneCall: the native agent advertises read-only
// tools for REVIEW, but a first-turn final answer ends the invocation with exactly
// one model call and no inspection. The REVIEW system prompt tells the model to
// inspect only if the request is insufficient. The stored result is the
// re-serialized JSON object, not the raw reply.
func TestVerifyNativeReviewEarlyFinalIsOneCall(t *testing.T) {
	dir := nativeProject(t)
	srv, cap := fakeOllama(t, `{"summary":"clean","findings":[]}`)
	t.Setenv(agent.EnvOllamaBaseURL, srv.URL)

	code, stdout, stderr := runRealPrompt(t, dir, reviewArgs()...)
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	doc := decodeDoc(t, stdout)
	if cap.count() != 1 || doc.Tokens["native-model"].Calls != 1 {
		t.Fatalf("provider calls=%d tokens=%+v, want exactly one call", cap.count(), doc.Tokens)
	}
	req := cap.at(0)
	if req["format"] != "json" {
		t.Errorf("format = %v, want json", req["format"])
	}
	system := ""
	if msgs, ok := req["messages"].([]any); ok && len(msgs) > 0 {
		system, _ = msgs[0].(map[string]any)["content"].(string)
	}
	for _, want := range []string{"read_file", "inspect the", "Do not modify files"} {
		if !strings.Contains(system, want) {
			t.Errorf("REVIEW system prompt lacks %q", want)
		}
	}
	if !strings.HasPrefix(doc.Result, `{"findings":[],"summary":"clean"}`) {
		t.Errorf("result = %q, want the re-serialized (key-sorted) JSON object", doc.Result)
	}
}

// TestVerifyNativeReviewCanInspectRepository: when the model asks for a read-only
// tool, the production path executes it inside the project, returns the content to
// the model, and counts a second call. The file is not modified.
func TestVerifyNativeReviewCanInspectRepository(t *testing.T) {
	dir := nativeProject(t)
	srv, cap := fakeOllama(t, `{"tool":"read_file","args":{"path":"notes.txt"}}`, `{"summary":"inspected","findings":[]}`)
	t.Setenv(agent.EnvOllamaBaseURL, srv.URL)

	code, stdout, stderr := runRealPrompt(t, dir, reviewArgs()...)
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	doc := decodeDoc(t, stdout)
	if cap.count() != 2 || doc.Tokens["native-model"].Calls != 2 {
		t.Fatalf("provider calls=%d tokens=%+v, want two calls", cap.count(), doc.Tokens)
	}
	raw, _ := json.Marshal(cap.at(1))
	if !strings.Contains(string(raw), "SYNTHETIC-MARKER-7f3a") {
		t.Error("the tool result (file content) did not reach the model on the second call")
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "notes.txt")); string(data) != "SYNTHETIC-MARKER-7f3a\n" {
		t.Errorf("notes.txt changed: %q", data)
	}
}

// failFirst serves the first n requests with HTTP 500, then answers with ok(). It
// records every request, including the failed ones.
func failFirst(t *testing.T, path string, n int, ok func() any) (*httptest.Server, *capture) {
	t.Helper()
	cap := &capture{}
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			http.NotFound(w, r)
			return
		}
		cap.record(r)
		mu.Lock()
		fail := n > 0
		n--
		mu.Unlock()
		if fail {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, `{"error":"synthetic failure"}`)
			return
		}
		_ = json.NewEncoder(w).Encode(ok())
	}))
	t.Cleanup(srv.Close)
	return srv, cap
}

// TestVerifyMLXProviderErrorIsNotRetried: the text-only provider makes one attempt. A
// provider error fails the prompt, leaves a failed result document, and is not retried.
func TestVerifyMLXProviderErrorIsNotRetried(t *testing.T) {
	clearProviderEnv(t)
	dir := verifyProject(t, "mlx", verifyModel)
	srv, cap := failFirst(t, "/v1/chat/completions", 1, func() any {
		return map[string]any{"choices": []map[string]any{{"message": map[string]any{"content": "late"}}}}
	})
	t.Setenv("SOP_MLX_BASE_URL", srv.URL)

	code, _, stderr := runRealPrompt(t, dir, reviewArgs()...)
	if code == exitOK || cap.count() != 1 {
		t.Fatalf("code=%d calls=%d stderr=%s; want a failure after exactly one attempt", code, cap.count(), stderr)
	}
}

// TestVerifyNativeProviderRetryIsInvisibleInTokens: the native client retries a failed
// chat turn inside the provider boundary. The retry succeeds, the prompt completes,
// and tokens.calls counts only the call that reported usage, so a retried invocation
// is not distinguishable from a clean one in metadata.json.
func TestVerifyNativeProviderRetryIsInvisibleInTokens(t *testing.T) {
	dir := nativeProject(t)
	srv, cap := failFirst(t, "/api/chat", 1, func() any {
		return map[string]any{
			"message": map[string]any{"role": "assistant", "content": `{"summary":"ok","findings":[]}`},
			"done":    true, "done_reason": "stop", "prompt_eval_count": 50, "eval_count": 20,
		}
	})
	t.Setenv(agent.EnvOllamaBaseURL, srv.URL)

	code, stdout, stderr := runRealPrompt(t, dir, reviewArgs()...)
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if got := decodeDoc(t, stdout).Tokens["native-model"]; cap.count() != 2 || got.Calls != 1 {
		t.Errorf("provider attempts=%d tokens=%+v; want 2 attempts but one counted call", cap.count(), got)
	}
}

// TestVerifyModelClassWithoutModelsBlockRoutesToBuiltInDefault: --model-class with no
// models block in the project routes to the built-in class default (provider ollama),
// not to the configured agent provider. Nothing is sent to the configured endpoint.
// The built-in default can be a cloud model, so an experiment project must define
// models.<class> for every class it can name. Both endpoints here are synthetic.
func TestVerifyModelClassWithoutModelsBlockRoutesToBuiltInDefault(t *testing.T) {
	clearProviderEnv(t)
	dir := t.TempDir()
	writeFile(t, dir, "request.md", "synthetic\n")
	writeProjectConfig(t, dir,
		"version: 1\nproject:\n  name: verify\n  integration_branch: main\nagent:\n  harness: tool\n  provider: mlx\n  model: "+verifyModel+"\n")
	t.Chdir(dir)
	ollamaSrv, ollamaCap := fakeOllama(t, `{"summary":"ok","findings":[]}`)
	mlxSrv, mlxCap := fakeMLX(t, "must not be called", true)
	t.Setenv(agent.EnvOllamaBaseURL, ollamaSrv.URL)
	t.Setenv("SOP_MLX_BASE_URL", mlxSrv.URL)

	code, stdout, stderr := runRealPrompt(t, dir, reviewArgs()...)
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	doc := decodeDoc(t, stdout)
	t.Logf("routed to provider=%q model=%q (configured agent: mlx %q)", doc.Routing.Provider, doc.Routing.Model, verifyModel)
	if doc.Routing.Provider != "ollama" || doc.Routing.Model == verifyModel {
		t.Errorf("routing = %+v, want the built-in ollama class default", doc.Routing)
	}
	if ollamaCap.count() == 0 || mlxCap.count() != 0 {
		t.Errorf("ollama calls=%d configured mlx calls=%d; want the class default used and the configured provider untouched", ollamaCap.count(), mlxCap.count())
	}
}

// TestVerifyNativeModelOverrideChangesRequestedModel: the native tool-loop agent honors
// the same overrides, so the requested-model check applies to it as well.
func TestVerifyNativeModelOverrideChangesRequestedModel(t *testing.T) {
	for _, env := range []string{agent.EnvAgentModel, agent.EnvOllamaModel} {
		t.Run(env, func(t *testing.T) {
			dir := nativeProject(t)
			srv, cap := fakeOllama(t, `{"summary":"clean","findings":[]}`)
			t.Setenv(agent.EnvOllamaBaseURL, srv.URL)
			t.Setenv(env, "override-model")

			code, stdout, stderr := runRealPrompt(t, dir, reviewArgs()...)
			if code != exitOK {
				t.Fatalf("code=%d stderr=%s", code, stderr)
			}
			doc := decodeDoc(t, stdout)
			t.Logf("%s: routing.model=%q provider received %q tokens keys=%v", env, doc.Routing.Model, sentModel(cap, 0), keys(doc.Tokens))
			if doc.Routing.Model != "native-model" || sentModel(cap, 0) != "override-model" {
				t.Errorf("routing.model=%q provider received %q", doc.Routing.Model, sentModel(cap, 0))
			}
			if _, ok := doc.Tokens["override-model"]; !ok || len(doc.Tokens) != 1 {
				t.Errorf("tokens keys = %v", keys(doc.Tokens))
			}
		})
	}
}

// TestVerifyPromptRunIDReservationGap characterizes how prompt run directories are
// allocated. promptRunID only checks that a directory does not exist yet; it does not
// create it, and runpkg.New (MkdirAll) does not fail when the directory exists. Two
// callers that both choose an id before either creates its directory therefore get
// the same id and the same directory. This documents a gap; when run-id allocation is
// made atomic, update or delete this test.
func TestVerifyPromptRunIDReservationGap(t *testing.T) {
	dir := t.TempDir()
	var a, b string
	for range 5 { // ids carry second resolution; retry across a boundary
		a, b = promptRunID(dir), promptRunID(dir)
		if a == b {
			break
		}
	}
	if a != b {
		t.Skip("could not draw two ids within one second")
	}
	first, err := runpkg.New(dir, promptRunDir(a))
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Write("result.md", "first"); err != nil {
		t.Fatal(err)
	}
	second, err := runpkg.New(dir, promptRunDir(b)) // same directory; must not fail today
	if err != nil {
		t.Fatalf("second New on the same id failed: %v", err)
	}
	if err := second.Write("result.md", "second"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, stateDirName, "runs", promptRunDir(a), "result.md"))
	if string(data) != "second" {
		t.Errorf("result.md = %q; expected the later writer to overwrite the earlier one today", data)
	}
	if n := len(promptRunDirs(t, dir)); n != 1 {
		t.Errorf("run dirs = %d, want 1 shared directory for two logical runs", n)
	}
}

// TestVerifyConcurrentPromptsObservation runs prompts concurrently in one project and
// reports how many distinct run directories and surviving results there are. It is an
// observation, not a gate: it only runs when SOP_VERIFY_CONCURRENT=1, and fails on a
// collision only when SOP_VERIFY_CONCURRENT_STRICT=1.
func TestVerifyConcurrentPromptsObservation(t *testing.T) {
	if os.Getenv("SOP_VERIFY_CONCURRENT") != "1" {
		t.Skip("observation only; set SOP_VERIFY_CONCURRENT=1")
	}
	clearProviderEnv(t)
	dir := t.TempDir()
	writeFile(t, dir, "request.md", "synthetic\n")
	const n = 8
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			a := &fakeCapabilityAgent{review: "reply-" + string(rune('A'+i))}
			d := defaultDeps()
			d.getwd = func() (string, error) { return dir, nil }
			d.newAgent = func(string, string, string, string) (agent.Agent, error) { return a, nil }
			var out, errOut bytes.Buffer
			<-start
			run([]string{"prompt", "--capability", "review", "--file", "request.md", "--json"}, &out, &errOut, d)
		}(i)
	}
	close(start)
	wg.Wait()
	runs := promptRunDirs(t, dir)
	replies := map[string]bool{}
	for _, r := range runs {
		if data, err := os.ReadFile(filepath.Join(r, "result.md")); err == nil {
			replies[strings.TrimSpace(string(data))] = true
		}
	}
	t.Logf("concurrent prompts=%d distinct run dirs=%d surviving distinct results=%d", n, len(runs), len(replies))
	if os.Getenv("SOP_VERIFY_CONCURRENT_STRICT") == "1" && (len(runs) != n || len(replies) != n) {
		t.Errorf("collision: %d runs produced %d run dirs and %d surviving results", n, len(runs), len(replies))
	}
}
