package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/github"
	"github.com/imhttran/agentic-sop/internal/ollamaagent"
)

// scriptedOllama is a minimal scripted /api/chat server, independent of
// ollamaagent's unexported wire types, so this package can drive the real
// NativeAgent tool loop without a live Ollama server.
func scriptedOllama(t *testing.T, responses ...string) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		content := `{"status":"completed","summary":"default","changes_expected":true}`
		if len(responses) > 0 {
			content = responses[0]
			responses = responses[1:]
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		body, _ := json.Marshal(map[string]any{
			"message": map[string]any{"role": "assistant", "content": content},
		})
		_, _ = w.Write(body)
	}))
}

// gitInitCommit makes dir a git repository with one commit, so the tool
// harness's git-diff-based fingerprint (used to attribute a mutation to this
// invocation) has a HEAD to diff against.
func gitInitCommit(t *testing.T, dir string) {
	t.Helper()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, out)
		}
	}
	run("init", "-q")
	run("-c", "user.email=test@example.com", "-c", "user.name=test", "commit", "--allow-empty", "-q", "-m", "initial")
}

// hybridAgent routes IMPLEMENT to the real Ollama tool-loop harness (backed by
// a scripted server) and answers PLAN/REVIEW with static canned content, so the
// end-to-end run exercises the actual DISCOVER->CHANGE->FINALIZE completion
// logic without needing a live model for every capability.
type hybridAgent struct{ native *ollamaagent.NativeAgent }

func (a *hybridAgent) Generate(ctx context.Context, req agent.Request) (agent.Response, error) {
	switch req.Capability {
	case agent.Implement:
		return a.native.Generate(ctx, req)
	case agent.Plan:
		return agent.Response{Content: validPlanJSON}, nil
	case agent.Review:
		return agent.Response{Content: `{"summary":"clean","findings":[]}`}, nil
	default:
		return agent.Response{Content: "{}"}, nil
	}
}

// TestImplementEndToEndProducesValidationRuns proves the full PREJEV002 chain:
// DISCOVER -> CHANGE (a real mutation through the controlled tools) ->
// mutation-aware completion -> FINALIZE -> structured outcome, and that the
// resulting IMPLEMENT run causes SOP to record a validation run - not a fake
// agent standing in for the phased loop, but the actual NativeAgent harness
// against a scripted Ollama server.
func TestImplementEndToEndProducesValidationRuns(t *testing.T) {
	dir := t.TempDir()
	gitInitCommit(t, dir)
	initProject(t, dir)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "docs/PLAN.md", "# Plan\n\n## S002 - Implement\n")
	seedTask(t, dir, &domain.Task{ID: "S002", Title: "Implement", Status: domain.PLANNED, MaxAttempts: 3})

	srv := scriptedOllama(t,
		`{"tool":"write_file","args":{"path":"impl.txt","content":"done"}}`,
		`{"status":"completed","summary":"implemented","changes_expected":true}`,
	)
	defer srv.Close()

	cfg := ollamaagent.Config{
		BaseURL:        srv.URL,
		Model:          ollamaagent.DefaultModel,
		Timeout:        5 * time.Second,
		MaxToolCalls:   10,
		CommandTimeout: 10 * time.Second,
		MaxOutputBytes: 1 << 20,
	}
	a := &hybridAgent{native: ollamaagent.NewNativeAgent(cfg, dir, io.Discard)}

	var out, errOut bytes.Buffer
	d := deps{
		getwd:     func() (string, error) { return dir, nil },
		newAgent:  func(string, string, string) (agent.Agent, error) { return a, nil },
		readDiff:  func(context.Context, string) (string, error) { return "impl.txt diff\n", nil },
		commit:    func(context.Context, string, string) error { return nil },
		newGitHub: func(string) github.Client { return &fakeGitHub{} },
	}
	code := run([]string{"run"}, &out, &errOut, d)
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, errOut.String(), out.String())
	}

	if got, err := os.ReadFile(filepath.Join(dir, "impl.txt")); err != nil || string(got) != "done" {
		t.Errorf("impl.txt = %q, err=%v; the mutation did not reach the repository", got, err)
	}

	s2 := mustTaskMetrics(t, dir, "S002")
	if s2.Counts.ValidationRuns == 0 {
		t.Errorf("S002 counts = %+v, want validation runs > 0", s2.Counts)
	}
}
