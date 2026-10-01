package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/github"
	"github.com/imhttran/agentic-sop/internal/model"
)

// Phase 5.4 hardening regressions: the capability guard must apply to the agent
// that will actually EXECUTE (the routed final selection), a --file must not escape
// the project through a symlink, and prompt input must stay data.

// cleanReviewJSON is a minimal non-blocking review response for the test agents.
const cleanReviewJSON = `{"summary":"clean","findings":[]}`

// planOnlyAgent plans and reviews but cannot mutate the repository. It is the shape
// of a text-only default provider used while routing selects a tool-capable one.
type planOnlyAgent struct{ *fakeCapabilityAgent }

func (planOnlyAgent) Capabilities() agent.Capabilities {
	return agent.NewCapabilities(agent.Plan, agent.DesignTests, agent.DiagnoseFailure, agent.Review)
}

// runPromptWithFactory runs `sop prompt ...` with a provider-aware agent factory, so
// a test can make different providers declare different capabilities.
func runPromptWithFactory(t *testing.T, dir, diff string, newAgent func(harness, provider, model string) (agent.Agent, error), args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	d := deps{
		getwd:     func() (string, error) { return dir, nil },
		newAgent:  newAgent,
		readDiff:  func(context.Context, string) (string, error) { return diff, nil },
		commit:    func(context.Context, string, string) error { return nil },
		newGitHub: func(string) github.Client { return &fakeGitHub{} },
	}
	code = run(args, &out, &errOut, d)
	return code, out.String(), errOut.String()
}

// TestPromptImplementGuardsFinalRoutedAgent is the Phase 5.4 §36 regression: with the
// router on, an IMPLEMENT prompt MUST NOT be rejected because the DEFAULT agent lacks
// IMPLEMENT when routing builds a different, tool-capable agent to execute. The
// capability guard applies to the routed final selection, not the default.
func TestPromptImplementGuardsFinalRoutedAgent(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	dir := t.TempDir()
	// The default execution stack is a text-only provider; routing resolves MEDIUM to
	// the built-in ollama class, which the injected factory makes tool-capable.
	writeConfig(t, dir, "project:\n  name: x\nagent:\n  harness: tool\n  provider: mlx\n  model: mlx-community/Qwen3-4B-4bit\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")

	var providers []string
	routed := &recordingAgent{}
	factory := func(_, provider, _ string) (agent.Agent, error) {
		providers = append(providers, provider)
		if provider == "mlx" {
			return planOnlyAgent{&fakeCapabilityAgent{plan: validPlanJSON, review: cleanReviewJSON}}, nil
		}
		return routed, nil
	}

	code, stdout, stderr := runPromptWithFactory(t, dir, "diff --git a/x b/x\n", factory,
		"prompt", "--capability", "implement", "Add caching to provider discovery")
	if code != exitOK {
		t.Fatalf("the routed tool-capable agent must execute; code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "Task routing: medium") {
		t.Errorf("routing block missing from output: %q", stdout)
	}
	if !strings.Contains(stdout, "PASS") {
		t.Errorf("governed lifecycle should pass the gate: %q", stdout)
	}
	if !containsString(providers, "ollama") {
		t.Errorf("the routed provider must be built, got %v", providers)
	}
	if len(routed.inputs) != 1 {
		t.Errorf("the ROUTED agent must run the implementation, got %d calls", len(routed.inputs))
	}
}

// TestPromptImplementRejectsIncapableRoutedSelection proves the complementary
// direction: when the ROUTED selection cannot IMPLEMENT, the prompt stops at the
// routing seam before the implementation step, even though the default agent could
// have run it. The routed selection is never silently substituted.
func TestPromptImplementRejectsIncapableRoutedSelection(t *testing.T) {
	clearProviderEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	dir := t.TempDir()
	// The default class (large) is tool-capable; the routed class (medium, chosen with
	// no evidence) is a text-only provider.
	writeConfig(t, dir, "project:\n  name: x\nagent:\n  harness: tool\n  provider: ollama\nmodels:\n  default_class: large\n  small:\n    provider: mlx\n    name: mlx-small\n    locality: local\n  medium:\n    provider: mlx\n    name: mlx-medium\n    locality: local\n  large:\n    provider: ollama\n    name: glm-large\n    locality: cloud\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")

	factory := func(_, provider, _ string) (agent.Agent, error) {
		if provider == "mlx" {
			return planOnlyAgent{&fakeCapabilityAgent{plan: validPlanJSON, review: cleanReviewJSON}}, nil
		}
		return &recordingAgent{}, nil
	}

	code, _, stderr := runPromptWithFactory(t, dir, "diff --git a/x b/x\n", factory,
		"prompt", "--capability", "implement", "Add caching to provider discovery")
	if code == exitOK {
		t.Fatalf("a routed selection that cannot IMPLEMENT must stop the prompt")
	}
	if !strings.Contains(stderr, "cannot IMPLEMENT") {
		t.Errorf("stderr should name the rejected capability: %q", stderr)
	}
	dirs := promptRunDirs(t, dir)
	if len(dirs) != 1 {
		t.Fatalf("want one prompt run dir, got %v", dirs)
	}
	if stateExists(filepath.Join(dirs[0], "implementation.md")) {
		t.Error("no implementation work may run on an incapable routed selection")
	}
}

// TestPromptFileSymlinkEscapeRejected is the Phase 5.4 §37 regression: a project-local
// symlink pointing outside the project must be rejected, not just a lexical `..` path.
func TestPromptFileSymlinkEscapeRejected(t *testing.T) {
	clearProviderEnv(t)
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret.md")
	if err := os.WriteFile(outside, []byte("secret"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "prompts"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "prompts", "input.md")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	a := &fakeCapabilityAgent{review: "x"}

	code, _, stderr := runCLIWithAgent(t, dir, a, "prompt", "--capability", "review", "--file", "prompts/input.md")
	if code == exitOK {
		t.Fatalf("a symlink escaping the project must be rejected")
	}
	if !strings.Contains(stderr, "outside the project directory") {
		t.Errorf("stderr = %q", stderr)
	}
	// A legitimate project-local symlink target inside the project is still allowed.
	if err := os.WriteFile(filepath.Join(dir, "real.md"), []byte("in project"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "real.md"), filepath.Join(dir, "prompts", "ok.md")); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runCLIWithAgent(t, dir, a, "prompt", "--capability", "review", "--file", "prompts/ok.md"); code != exitOK {
		t.Errorf("an in-project symlink must be allowed: code=%d stderr=%s", code, stderr)
	}
}

// TestPromptArgsAreDataNotShell proves prompt input is data: shell metacharacters are
// preserved verbatim in the prompt artifact and are never executed.
func TestPromptArgsAreDataNotShell(t *testing.T) {
	clearProviderEnv(t)
	dir := t.TempDir()
	a := &fakeCapabilityAgent{review: "ok"}
	const hostile = "review this; rm -rf . $(whoami) `id` \"quoted\" 'x' | tee /tmp/pwned"

	code, _, stderr := runCLIWithAgent(t, dir, a, "prompt", "--capability", "review", hostile)
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	dirs := promptRunDirs(t, dir)
	if len(dirs) != 1 {
		t.Fatalf("want one prompt run dir, got %v", dirs)
	}
	data, err := os.ReadFile(filepath.Join(dirs[0], "prompt.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), hostile) {
		t.Errorf("prompt content must be preserved verbatim:\n%s", data)
	}
	if stateExists("/tmp/pwned") {
		t.Fatal("prompt text must never reach a shell")
	}
}

// TestPromptProjectsAreIsolated proves a prompt run is confined to the project it was
// submitted in: two projects keep independent .agent-sdlc state.
func TestPromptProjectsAreIsolated(t *testing.T) {
	clearProviderEnv(t)
	dirA, dirB := t.TempDir(), t.TempDir()
	a := &fakeCapabilityAgent{review: "ok"}

	if code, _, stderr := runCLIWithAgent(t, dirA, a, "prompt", "--capability", "review", "review project A"); code != exitOK {
		t.Fatalf("A: code=%d stderr=%s", code, stderr)
	}
	for i := 0; i < 2; i++ {
		if code, _, stderr := runCLIWithAgent(t, dirB, a, "prompt", "--capability", "review", "review project B"); code != exitOK {
			t.Fatalf("B: code=%d stderr=%s", code, stderr)
		}
	}
	pa, pb := promptRunDirs(t, dirA), promptRunDirs(t, dirB)
	if len(pa) != 1 || len(pb) != 2 {
		t.Fatalf("runs must stay in their project: A=%v B=%v", pa, pb)
	}
	promptOf := func(runDir string) string {
		data, err := os.ReadFile(filepath.Join(runDir, "prompt.md"))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	if !strings.Contains(promptOf(pa[0]), "review project A") {
		t.Error("project A lost its own prompt")
	}
	if strings.Contains(promptOf(pa[0]), "project B") {
		t.Error("project A must not see project B's work")
	}
}

func containsString(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
