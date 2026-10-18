package cli

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/store"
)

// SOP-002 — preflight runs before any model call, reports one specific setup
// blocker, never executes a task command, and never spends continuation budget.

// preflightAgent records every model call on top of the scheduled-acceptance agent.
type preflightAgent struct {
	scheduledAcceptanceAgent
	calls int
}

func (a *preflightAgent) Generate(ctx context.Context, r agent.Request) (agent.Response, error) {
	a.calls++
	return a.scheduledAcceptanceAgent.Generate(ctx, r)
}

// runPreflightScheduled drives runScheduledTask once with a counting agent and
// returns the exit code, stdout, stderr, and the number of model calls.
func runPreflightScheduled(t *testing.T, dir string, task *domain.Task) (int, string, string, int) {
	t.Helper()
	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	cfg, err := loadConfigOrDefault(dir)
	if err != nil {
		t.Fatal(err)
	}
	a := &preflightAgent{scheduledAcceptanceAgent: scheduledAcceptanceAgent{dir: dir}}
	var out, errOut bytes.Buffer
	code := runScheduledTask(context.Background(), dir, cfg, a, scheduledAcceptanceDeps(dir, a), st, task, newRunSession(), &out, &errOut)
	return code, out.String(), errOut.String(), a.calls
}

func requireBlocker(t *testing.T, res PreflightResult, check, subject string) *PreflightBlocker {
	t.Helper()
	if res.OK() {
		t.Fatalf("preflight passed, want a %s blocker for %q", check, subject)
	}
	if res.Blocker.Check != check || res.Blocker.Subject != subject {
		t.Fatalf("blocker = %+v, want check=%s subject=%q", *res.Blocker, check, subject)
	}
	return res.Blocker
}

func TestPreflightPolicyMismatchBeforeModelCall(t *testing.T) {
	dir := scheduledAcceptanceFixture(t)
	planWithChecks(t, dir, "go run .")
	task := reconcilePlanTask(t, dir)

	code, _, stderr, calls := runPreflightScheduled(t, dir, task)
	if code == exitOK {
		t.Fatal("run passed despite an unauthorized acceptance check")
	}
	if calls != 0 {
		t.Errorf("model called %d time(s) before the setup blocker", calls)
	}
	if !strings.Contains(stderr, "setup blocker (command-policy): go run .") || !strings.Contains(stderr, "acceptance path") {
		t.Errorf("stderr missing the command-policy blocker: %q", stderr)
	}
}

func TestPreflightMissingTool(t *testing.T) {
	const tool = "sop-preflight-no-such-tool"
	b := requireBlocker(t, Preflight(t.TempDir(), PreflightDeclarations{Tools: []string{tool}}), "tool", tool)
	if !strings.Contains(b.Error(), "not found on PATH") {
		t.Errorf("blocker = %q", b.Error())
	}
	if _, err := exec.LookPath("go"); err == nil {
		if res := Preflight(t.TempDir(), PreflightDeclarations{Tools: []string{"go"}}); !res.OK() {
			t.Errorf("a tool on PATH blocked: %v", res.Blocker)
		}
	}
}

func TestPreflightMissingManifest(t *testing.T) {
	dir := t.TempDir()
	requireBlocker(t, Preflight(dir, PreflightDeclarations{Manifests: []string{"Cargo.toml"}}), "manifest", "Cargo.toml")
	requireBlocker(t, Preflight(dir, PreflightDeclarations{Manifests: []string{"../outside.toml"}}), "manifest", "../outside.toml")

	if err := os.WriteFile(filepath.Join(dir, "Cargo.toml"), []byte("[package]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if res := Preflight(dir, PreflightDeclarations{Manifests: []string{"Cargo.toml"}}); !res.OK() {
		t.Errorf("present manifest blocked: %v", res.Blocker)
	}
}

func TestPreflightMissingEnvVarNeverPrintsValue(t *testing.T) {
	const name, secret = "SOP_PREFLIGHT_TOKEN", "s3cr3t-sentinel-value"
	const missing = "SOP_PREFLIGHT_MISSING"
	t.Setenv(name, secret)
	os.Unsetenv(missing)

	decl := PreflightDeclarations{EnvVars: []string{name, missing}}
	var out, errOut bytes.Buffer
	if code := runPreflight(t.TempDir(), decl, &out, &errOut); code == exitOK {
		t.Fatal("preflight passed with a missing environment variable")
	}
	b := requireBlocker(t, Preflight(t.TempDir(), decl), "env", missing)
	for _, text := range []string{out.String(), errOut.String(), b.Error()} {
		if strings.Contains(text, secret) {
			t.Fatalf("secret value printed: %q", text)
		}
	}
	if !strings.Contains(errOut.String(), missing) {
		t.Errorf("stderr does not name the missing variable: %q", errOut.String())
	}
}

func TestPreflightConflictingPermission(t *testing.T) {
	// The two Cargo permissions are independent: acceptance-only Cargo must pass
	// preflight even when the agent-tool permission is off, and the reverse must
	// also pass. This is the configuration SOP-001 promises; a "conflict" blocker
	// would make the independent settings unusable.
	dir := t.TempDir()
	cfg := config.Config{Cargo: config.Cargo{AcceptanceCheck: boolPtr(true), AgentTool: boolPtr(false)}}
	if res := Preflight(dir, PreflightDeclarationsFromConfig(cfg, []string{"cargo test"})); !res.OK() {
		t.Errorf("acceptance-only Cargo permissions blocked: %v", res.Blocker)
	}

	cfg.Cargo.AcceptanceCheck = boolPtr(false)
	cfg.Cargo.AgentTool = boolPtr(true)
	decl := PreflightDeclarationsFromConfig(cfg, nil)
	decl.Commands = []PreflightCommand{{Command: "cargo test", Path: PreflightAgentTool}}
	if res := Preflight(dir, decl); !res.OK() {
		t.Errorf("agent-tool-only Cargo permissions blocked: %v", res.Blocker)
	}

	// A command declared on a path whose permission is off is still refused.
	cfg.Cargo.AcceptanceCheck = boolPtr(false)
	cfg.Cargo.AgentTool = boolPtr(true)
	if res := Preflight(dir, PreflightDeclarationsFromConfig(cfg, []string{"cargo test"})); res.OK() {
		t.Fatal("acceptance Cargo check passed with acceptance_check off")
	}
}

func TestPreflightFailureKeepsContinuationBudget(t *testing.T) {
	dir := scheduledAcceptanceFixture(t)
	planWithChecks(t, dir, "git push")
	task := reconcilePlanTask(t, dir)
	rn, err := runpkg.New(dir, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := rn.RecordContinuations(1); err != nil {
		t.Fatal(err)
	}
	attempt := task.Attempt

	if code, _, _, calls := runPreflightScheduled(t, dir, task); code == exitOK || calls != 0 {
		t.Fatalf("code=%d calls=%d, want a setup blocker before any model call", code, calls)
	}

	reopened, err := runpkg.Open(dir, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.ReadContinuations(); got != 1 {
		t.Errorf("continuations = %d after a setup failure, want 1", got)
	}
	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	stored, err := st.Get(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if task.Attempt != attempt || stored.Attempt != attempt {
		t.Errorf("attempt changed: task=%d stored=%d, want %d", task.Attempt, stored.Attempt, attempt)
	}
}

func TestPreflightExecutesNoTaskCommand(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker")
	decl := PreflightDeclarations{Commands: []PreflightCommand{
		{Command: "touch marker", Path: PreflightAcceptance},
		{Command: "go vet ./...", Path: PreflightAgentTool},
	}}
	requireBlocker(t, Preflight(dir, decl), "command-policy", "touch marker")
	if res := Preflight(dir, PreflightDeclarations{Commands: decl.Commands[1:]}); !res.OK() {
		t.Errorf("SAFE agent-tool command blocked: %v", res.Blocker)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("preflight executed a declared task command")
	}
}
