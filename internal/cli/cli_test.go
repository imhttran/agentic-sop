package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/github"
	"github.com/imhttran/agentic-sop/internal/resume"
	"github.com/imhttran/agentic-sop/internal/review"
	"github.com/imhttran/agentic-sop/internal/store"
)

// stateExists reports whether a file exists, for test assertions.
func stateExists(path string) bool {
	ok, _ := exists(path)
	return ok
}

// runCLI executes the dispatcher against dir as the project root and returns
// the exit code with captured stdout and stderr.
func runCLI(t *testing.T, dir string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	return runCLIWithAgent(t, dir, nil, args...)
}

// runCLIWithAgent is runCLI with an injected agent; a nil agent simulates an
// unconfigured agent.
func runCLIWithAgent(t *testing.T, dir string, a agent.Agent, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	d := deps{
		getwd: func() (string, error) { return dir, nil },
		newAgent: func(_ string) (agent.Agent, error) {
			if a == nil {
				return nil, errors.New("no agent configured")
			}
			return a, nil
		},
		readDiff:  func(context.Context, string) (string, error) { return "", nil },
		commit:    func(context.Context, string, string) error { return nil },
		newGitHub: func(string) github.Client { return &fakeGitHub{} },
	}
	code = run(args, &out, &errOut, d)
	return code, out.String(), errOut.String()
}

func initProject(t *testing.T, dir string) {
	t.Helper()
	if code, _, stderr := runCLI(t, dir, "init"); code != 0 {
		t.Fatalf("init failed: code=%d stderr=%s", code, stderr)
	}
}

func seedTask(t *testing.T, dir string, task *domain.Task) {
	t.Helper()
	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	if err := st.Save(task); err != nil {
		t.Fatalf("save task: %v", err)
	}
}

func TestRunHelp(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"-h"}, {"help"}, {}} {
		code, stdout, stderr := runCLI(t, t.TempDir(), args...)
		if code != exitOK {
			t.Errorf("%v: code=%d, want %d", args, code, exitOK)
		}
		for _, want := range []string{"init", "status", "task", "version"} {
			if !strings.Contains(stdout, want) {
				t.Errorf("%v: help missing %q:\n%s", args, want, stdout)
			}
		}
		if stderr != "" {
			t.Errorf("%v: unexpected stderr: %s", args, stderr)
		}
	}
}

func TestRunVersion(t *testing.T) {
	code, stdout, stderr := runCLI(t, t.TempDir(), "version")
	if code != exitOK {
		t.Fatalf("code=%d, want %d", code, exitOK)
	}
	if !strings.Contains(stdout, Version) {
		t.Errorf("version output %q missing %q", stdout, Version)
	}
	if stderr != "" {
		t.Errorf("unexpected stderr: %s", stderr)
	}
}

func TestRunUnknownCommand(t *testing.T) {
	code, stdout, stderr := runCLI(t, t.TempDir(), "banana")
	if code != exitUsage {
		t.Errorf("code=%d, want %d", code, exitUsage)
	}
	if stdout != "" {
		t.Errorf("expected empty stdout, got %q", stdout)
	}
	if !strings.Contains(stderr, "unknown command") {
		t.Errorf("stderr missing unknown-command message: %s", stderr)
	}
}

func TestRunInitCreatesState(t *testing.T) {
	dir := t.TempDir()
	code, stdout, stderr := runCLI(t, dir, "init")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if !stateExists(statePath(dir)) {
		t.Fatalf("state database not created at %s", statePath(dir))
	}
	if !strings.Contains(stdout, "initialized") {
		t.Errorf("unexpected stdout: %q", stdout)
	}
}

func TestRunInitWritesConfigTemplate(t *testing.T) {
	dir := t.TempDir()
	if code, _, stderr := runCLI(t, dir, "init"); code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	cfg, err := config.LoadDir(dir)
	if err != nil {
		t.Fatalf("generated config does not load: %v", err)
	}
	if cfg.Project.Name != filepath.Base(dir) {
		t.Errorf("project name = %q, want %q", cfg.Project.Name, filepath.Base(dir))
	}
}

func TestRunInitPreservesExistingConfig(t *testing.T) {
	dir := t.TempDir()
	if code, _, stderr := runCLI(t, dir, "init"); code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	custom := []byte("project:\n  name: my-custom-name\n")
	if err := os.WriteFile(config.Path(dir), custom, 0o644); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runCLI(t, dir, "init"); code != exitOK {
		t.Fatalf("second init failed: code=%d stderr=%s", code, stderr)
	}
	got, err := os.ReadFile(config.Path(dir))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, custom) {
		t.Errorf("init overwrote an existing config:\n%s", got)
	}
}

func TestRunInitIsIdempotentAndPreservesTasks(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)

	now := time.Now().UTC()
	seedTask(t, dir, &domain.Task{
		ID:        "T001",
		Title:     "Persisted",
		Status:    domain.DONE,
		CreatedAt: now,
		UpdatedAt: now,
	})

	// Second init must succeed and must not wipe existing state.
	initProject(t, dir)

	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	if _, err := st.Get("T001"); err != nil {
		t.Errorf("task lost after second init: %v", err)
	}
}

func TestRunStatusUninitialized(t *testing.T) {
	dir := t.TempDir()
	code, stdout, stderr := runCLI(t, dir, "status")
	if code != exitError {
		t.Errorf("code=%d, want %d", code, exitError)
	}
	if stdout != "" {
		t.Errorf("expected empty stdout, got %q", stdout)
	}
	if !strings.Contains(stderr, "not initialized") {
		t.Errorf("stderr missing not-initialized message: %s", stderr)
	}
	if stateExists(statePath(dir)) {
		t.Errorf("status must not create the state database")
	}
}

func TestRunTaskUninitialized(t *testing.T) {
	dir := t.TempDir()
	code, stdout, stderr := runCLI(t, dir, "task", "T001")
	if code != exitError {
		t.Errorf("code=%d, want %d", code, exitError)
	}
	if stdout != "" {
		t.Errorf("expected empty stdout, got %q", stdout)
	}
	if !strings.Contains(stderr, "not initialized") {
		t.Errorf("stderr missing not-initialized message: %s", stderr)
	}
	if stateExists(statePath(dir)) {
		t.Errorf("task must not create the state database")
	}
}

func TestRunStatusListsTasks(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)

	now := time.Now().UTC()
	seedTask(t, dir, &domain.Task{ID: "T002", Title: "SQLite persistence", Status: domain.READY, CreatedAt: now, UpdatedAt: now})
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "Domain model", Status: domain.DONE, CreatedAt: now, UpdatedAt: now})

	code, stdout, stderr := runCLI(t, dir, "status")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}

	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	want := []string{
		"T001 DONE Domain model",
		"T002 READY SQLite persistence",
	}
	if len(lines) != len(want) {
		t.Fatalf("got %d line(s), want %d:\n%s", len(lines), len(want), stdout)
	}
	for i, w := range want {
		if lines[i] != w {
			t.Errorf("line %d: got %q, want %q", i, lines[i], w)
		}
	}
}

func TestRunTaskShowsDetails(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)

	now := time.Now().UTC()
	seedTask(t, dir, &domain.Task{ID: "T000", Title: "Root", Status: domain.DONE, CreatedAt: now, UpdatedAt: now})
	seedTask(t, dir, &domain.Task{
		ID:            "T001",
		Title:         "CLI foundation",
		Status:        domain.IMPLEMENTING,
		DependencyIDs: []string{"T000"},
		Attempts:      []domain.Attempt{{Number: 1, Status: domain.IMPLEMENTING, Timestamp: now}},
		CreatedAt:     now,
		UpdatedAt:     now,
	})

	code, stdout, stderr := runCLI(t, dir, "task", "T001")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	for _, want := range []string{"Task: T001", "Title: CLI foundation", "Status: IMPLEMENTING", "Attempts: 1", "Dependencies:", "T000"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("output missing %q:\n%s", want, stdout)
		}
	}
}

func TestRunTaskMissingID(t *testing.T) {
	code, stdout, stderr := runCLI(t, t.TempDir(), "task")
	if code != exitUsage {
		t.Errorf("code=%d, want %d", code, exitUsage)
	}
	if stdout != "" {
		t.Errorf("expected empty stdout, got %q", stdout)
	}
	if !strings.Contains(stderr, "usage") {
		t.Errorf("stderr missing usage: %s", stderr)
	}
}

func TestRunTaskExtraArgs(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	code, _, stderr := runCLI(t, dir, "task", "T001", "extra")
	if code != exitUsage {
		t.Errorf("code=%d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr, "usage") {
		t.Errorf("stderr missing usage: %s", stderr)
	}
}

func TestRunTaskNotFound(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	code, stdout, stderr := runCLI(t, dir, "task", "T999")
	if code != exitError {
		t.Errorf("code=%d, want %d", code, exitError)
	}
	if stdout != "" {
		t.Errorf("expected empty stdout, got %q", stdout)
	}
	if !strings.Contains(stderr, "not found") {
		t.Errorf("stderr missing not-found message: %s", stderr)
	}
}

// fakeAgent is a deterministic agent for tests.
type fakeAgent struct {
	content string
	err     error
}

func (f *fakeAgent) Generate(_ context.Context, _ agent.Request) (agent.Response, error) {
	if f.err != nil {
		return agent.Response{}, f.err
	}
	return agent.Response{Content: f.content}, nil
}

const validPlanJSON = `{
  "project": "Book RAG",
  "summary": "Build a retrieval augmented generation app.",
  "stages": [
    {
      "id": "S001",
      "title": "Application skeleton",
      "objective": "Create the Go application skeleton.",
      "dependencies": [],
      "deliverables": ["Go application"],
      "acceptance_criteria": ["Application starts successfully"]
    },
    {
      "id": "S002",
      "title": "Ingestion",
      "objective": "Ingest books into the index.",
      "dependencies": ["S001"],
      "deliverables": ["Ingester"],
      "acceptance_criteria": ["A book can be ingested"]
    }
  ]
}`

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestRunPlanWritesMarkdown(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "PRD.md", "# Book RAG\n\nAn app that answers questions over books.\n")

	code, stdout, stderr := runCLIWithAgent(t, dir, &fakeAgent{content: validPlanJSON}, "plan")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "wrote PLAN.md") {
		t.Errorf("stdout = %q, want mention of PLAN.md", stdout)
	}

	plan, err := os.ReadFile(filepath.Join(dir, "PLAN.md"))
	if err != nil {
		t.Fatalf("PLAN.md not written: %v", err)
	}
	for _, want := range []string{
		"# Implementation Plan",
		"## Project\n\nBook RAG",
		"## S001 — Application skeleton",
		"## S002 — Ingestion",
		"- Application starts successfully",
		"- S001",
	} {
		if !strings.Contains(string(plan), want) {
			t.Errorf("PLAN.md missing %q:\n%s", want, plan)
		}
	}

	// The machine representation is written alongside the human one.
	raw, err := os.ReadFile(filepath.Join(dir, stateDirName, "plan.json"))
	if err != nil {
		t.Fatalf("plan.json not written: %v", err)
	}
	var decoded struct {
		Project string `json:"project"`
		Stages  []struct {
			ID string `json:"id"`
		} `json:"stages"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("plan.json is not valid JSON: %v", err)
	}
	if decoded.Project != "Book RAG" || len(decoded.Stages) != 2 {
		t.Errorf("plan.json mismatch: %+v", decoded)
	}
}

func TestRunPlanUsesConfiguredProvider(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "PRD.md", "# Book RAG\n")
	initProject(t, dir)
	if err := os.WriteFile(config.Path(dir), []byte("project:\n  name: x\nagent:\n  provider: ollama\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(agent.EnvAgentProvider, "")

	var got string
	var out, errOut bytes.Buffer
	d := deps{
		getwd: func() (string, error) { return dir, nil },
		newAgent: func(provider string) (agent.Agent, error) {
			got = provider
			return &fakeAgent{content: validPlanJSON}, nil
		},
	}
	if code := run([]string{"plan"}, &out, &errOut, d); code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, errOut.String())
	}
	if got != "ollama" {
		t.Errorf("provider = %q, want ollama", got)
	}
}

func TestRunPlanRejectsInvalidConfig(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "PRD.md", "# Book RAG\n")
	if err := os.MkdirAll(filepath.Join(dir, config.DirName), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join(config.DirName, config.FileName), "project:\n  name: x\nagent:\n  provider: skynet\n")

	code, _, stderr := runCLIWithAgent(t, dir, &fakeAgent{content: validPlanJSON}, "plan")
	if code != exitError {
		t.Errorf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "agent.provider") {
		t.Errorf("stderr missing config error: %s", stderr)
	}
}

func TestRunPlanMissingPRD(t *testing.T) {
	dir := t.TempDir()
	code, stdout, stderr := runCLIWithAgent(t, dir, &fakeAgent{content: validPlanJSON}, "plan")
	if code != exitError {
		t.Errorf("code=%d, want %d", code, exitError)
	}
	if stdout != "" {
		t.Errorf("expected empty stdout, got %q", stdout)
	}
	if !strings.Contains(stderr, "PRD.md") {
		t.Errorf("stderr missing PRD message: %s", stderr)
	}
	if stateExists(filepath.Join(dir, "PLAN.md")) {
		t.Errorf("PLAN.md must not be created when PRD.md is missing")
	}
}

func TestRunPlanEmptyPRD(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "PRD.md", "   \n")
	code, _, stderr := runCLIWithAgent(t, dir, &fakeAgent{content: validPlanJSON}, "plan")
	if code != exitError {
		t.Errorf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "empty") {
		t.Errorf("stderr missing empty-PRD message: %s", stderr)
	}
	if stateExists(filepath.Join(dir, "PLAN.md")) {
		t.Errorf("PLAN.md must not be created for an empty PRD")
	}
}

func TestRunPlanProtectsExistingPlan(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "PRD.md", "# Book RAG\n")
	const existing = "hand-edited plan, do not touch\n"
	writeFile(t, dir, "PLAN.md", existing)

	code, _, stderr := runCLIWithAgent(t, dir, &fakeAgent{content: validPlanJSON}, "plan")
	if code != exitError {
		t.Errorf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "already exists") {
		t.Errorf("stderr missing overwrite protection: %s", stderr)
	}

	got, err := os.ReadFile(filepath.Join(dir, "PLAN.md"))
	if err != nil {
		t.Fatalf("read PLAN.md: %v", err)
	}
	if string(got) != existing {
		t.Errorf("existing PLAN.md was modified: %q", got)
	}
}

func TestRunPlanInvalidAgentOutput(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "PRD.md", "# Book RAG\n")
	code, _, stderr := runCLIWithAgent(t, dir, &fakeAgent{content: "not json"}, "plan")
	if code != exitError {
		t.Errorf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "parse agent response") {
		t.Errorf("stderr missing parse error: %s", stderr)
	}
	if stateExists(filepath.Join(dir, "PLAN.md")) {
		t.Errorf("PLAN.md must not be created for invalid agent output")
	}
}

func TestRunPlanAgentError(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "PRD.md", "# Book RAG\n")
	code, _, stderr := runCLIWithAgent(t, dir, &fakeAgent{err: errors.New("boom")}, "plan")
	if code != exitError {
		t.Errorf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "boom") {
		t.Errorf("stderr missing agent error: %s", stderr)
	}
}

func TestRunPlanUnconfiguredAgent(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "PRD.md", "# Book RAG\n")
	code, _, stderr := runCLI(t, dir, "plan")
	if code != exitError {
		t.Errorf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "no agent configured") {
		t.Errorf("stderr missing agent-config message: %s", stderr)
	}
}

func TestRunPlanBadArgs(t *testing.T) {
	dir := t.TempDir()
	code, _, stderr := runCLI(t, dir, "plan", "one.md", "two.md")
	if code != exitUsage {
		t.Errorf("code=%d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr, "usage") {
		t.Errorf("stderr missing usage: %s", stderr)
	}
}

// writePlanJSON places a machine plan under .agent-sdlc/plan.json.
func writePlanJSON(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, stateDirName), 0o755); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	writeFile(t, dir, filepath.Join(stateDirName, "plan.json"), content)
}

// capturingAgent records the request it received, for asserting what the CLI
// sent to the agent.
type capturingAgent struct {
	content string
	got     agent.Request
}

func (c *capturingAgent) Generate(_ context.Context, r agent.Request) (agent.Response, error) {
	c.got = r
	return agent.Response{Content: c.content}, nil
}

func TestRunPlanFromTaskFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", "# T042 --- Add widget\n\n## Objective\n\nAdd the widget.\n\n## Acceptance Criteria\n\n- [ ] widget works\n")

	a := &capturingAgent{content: validPlanJSON}
	code, stdout, stderr := runCLIWithAgent(t, dir, a, "plan", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "wrote PLAN.md") {
		t.Errorf("stdout = %q", stdout)
	}
	if a.got.Capability != agent.Plan {
		t.Errorf("capability = %q, want PLAN", a.got.Capability)
	}
	if !strings.Contains(a.got.Input, "Add widget") || !strings.Contains(a.got.Input, "widget works") {
		t.Errorf("agent input missing task content:\n%s", a.got.Input)
	}
}

func TestRunPlanTaskFileMissing(t *testing.T) {
	dir := t.TempDir()
	code, _, stderr := runCLIWithAgent(t, dir, &fakeAgent{content: validPlanJSON}, "plan", "NOPE.md")
	if code != exitError {
		t.Errorf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "NOPE.md") {
		t.Errorf("stderr missing file name: %s", stderr)
	}
	if stateExists(filepath.Join(dir, "PLAN.md")) {
		t.Error("PLAN.md must not be created when the task file is missing")
	}
}

// writeConfig writes a project configuration under .agent-sdlc/.
func writeConfig(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, config.DirName), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join(config.DirName, config.FileName), content)
}

func TestRunValidatePass(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
	code, stdout, stderr := runCLI(t, dir, "validate")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "validation: PASS") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRunValidateFailIsFailFast(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"echo boom >&2; exit 1\"\n  lint:\n    - \"true\"\n")
	code, stdout, _ := runCLI(t, dir, "validate")
	if code != exitError {
		t.Errorf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stdout, "validation: FAIL") {
		t.Errorf("stdout missing FAIL: %q", stdout)
	}
	if !strings.Contains(stdout, "boom") {
		t.Errorf("stdout missing diagnostics: %q", stdout)
	}
	if !strings.Contains(stdout, "UNIT_TEST") {
		t.Errorf("stdout missing the failing category: %q", stdout)
	}
	if strings.Contains(stdout, "LINT") {
		t.Errorf("lint should not run after a failure (fail-fast): %q", stdout)
	}
}

func TestRunValidateNoConfig(t *testing.T) {
	dir := t.TempDir()
	code, _, stderr := runCLI(t, dir, "validate")
	if code != exitError {
		t.Errorf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "no configuration") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestRunValidateNoCommands(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "project:\n  name: x\n")
	code, stdout, stderr := runCLI(t, dir, "validate")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "no validation commands") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRunValidateBadArgs(t *testing.T) {
	dir := t.TempDir()
	code, _, stderr := runCLI(t, dir, "validate", "extra")
	if code != exitUsage {
		t.Errorf("code=%d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr, "usage") {
		t.Errorf("stderr missing usage: %s", stderr)
	}
}

// runInjectedCLI runs a command with an injected working-tree diff and agent.
func runInjectedCLI(t *testing.T, dir, diff string, a agent.Agent, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	d := deps{
		getwd: func() (string, error) { return dir, nil },
		newAgent: func(string) (agent.Agent, error) {
			if a == nil {
				return nil, errors.New("no agent configured")
			}
			return a, nil
		},
		readDiff:  func(context.Context, string) (string, error) { return diff, nil },
		commit:    func(context.Context, string, string) error { return nil },
		newGitHub: func(string) github.Client { return &fakeGitHub{} },
	}
	code = run(args, &out, &errOut, d)
	return code, out.String(), errOut.String()
}

func TestRunReviewNoChanges(t *testing.T) {
	dir := t.TempDir()
	code, stdout, stderr := runInjectedCLI(t, dir, "  \n", &fakeAgent{content: "{}"}, "review")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "no changes to review") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRunReviewBlockingFinding(t *testing.T) {
	dir := t.TempDir()
	findings := `{"summary":"looks fine","findings":[{"severity":"HIGH","title":"nil deref","file":"a.go","line":3}]}`
	code, stdout, _ := runInjectedCLI(t, dir, "diff --git a/a.go b/a.go\n", &fakeAgent{content: findings}, "review")
	if code != exitError {
		t.Errorf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stdout, "nil deref") {
		t.Errorf("stdout missing finding: %q", stdout)
	}
	if !strings.Contains(stdout, "1 blocking") {
		t.Errorf("stdout missing blocking count: %q", stdout)
	}
}

func TestRunReviewNonBlockingFinding(t *testing.T) {
	dir := t.TempDir()
	findings := `{"summary":"minor","findings":[{"severity":"MEDIUM","title":"style","file":"a.go","line":1}]}`
	code, stdout, _ := runInjectedCLI(t, dir, "diff --git a/a.go b/a.go\n", &fakeAgent{content: findings}, "review")
	if code != exitOK {
		t.Errorf("code=%d, want %d", code, exitOK)
	}
	if !strings.Contains(stdout, "0 blocking") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRunReviewOCRNotConfigured(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "project:\n  name: x\nreview:\n  engine: open-code-review\n")
	t.Setenv(review.EnvReviewCommand, "")
	code, _, stderr := runInjectedCLI(t, dir, "diff\n", &fakeAgent{content: "{}"}, "review")
	if code != exitError {
		t.Errorf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, review.EnvReviewCommand) {
		t.Errorf("stderr should name the missing variable: %q", stderr)
	}
}

func TestRunReviewBadArgs(t *testing.T) {
	dir := t.TempDir()
	code, _, stderr := runInjectedCLI(t, dir, "diff\n", &fakeAgent{content: "{}"}, "review", "extra")
	if code != exitUsage {
		t.Errorf("code=%d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr, "usage") {
		t.Errorf("stderr = %q", stderr)
	}
}

// fakeCapabilityAgent returns different content per agent capability. reviews,
// if set, is returned in order (the last repeats), so a fix loop can change its
// verdict across cycles.
type fakeCapabilityAgent struct {
	plan, impl, review, fix string
	reviews                 []string
	reviewIdx               int
}

func (f *fakeCapabilityAgent) Generate(_ context.Context, r agent.Request) (agent.Response, error) {
	switch r.Capability {
	case agent.Plan:
		return agent.Response{Content: f.plan}, nil
	case agent.Implement:
		return agent.Response{Content: f.impl}, nil
	case agent.Fix:
		return agent.Response{Content: f.fix}, nil
	case agent.Review:
		if len(f.reviews) > 0 {
			i := f.reviewIdx
			if i >= len(f.reviews) {
				i = len(f.reviews) - 1
			}
			f.reviewIdx++
			return agent.Response{Content: f.reviews[i]}, nil
		}
		return agent.Response{Content: f.review}, nil
	default:
		return agent.Response{Content: "{}"}, nil
	}
}

const runTaskFile = "# T001 --- Add widget\n\n## Objective\n\nAdd the widget.\n"

func TestRunEndToEndPass(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
	a := &fakeCapabilityAgent{plan: validPlanJSON, impl: "changed files", review: `{"summary":"clean","findings":[]}`}

	code, stdout, stderr := runInjectedCLI(t, dir, "diff --git a/x b/x\n", a, "run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "PASS") {
		t.Errorf("stdout = %q", stdout)
	}
	if !strings.Contains(stdout, "awaiting human approval") {
		t.Errorf("stdout missing human-gate message: %q", stdout)
	}

	runDir := filepath.Join(dir, stateDirName, "runs", "T001")
	for _, name := range []string{
		"state.json", "task.md", "plan.md", "implementation.md",
		"diff.patch", "validation.json", "review.json", "report.md", "report.json",
	} {
		if !stateExists(filepath.Join(runDir, name)) {
			t.Errorf("missing run artifact %s", name)
		}
	}
}

func TestRunNoChanges(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")
	a := &fakeCapabilityAgent{plan: validPlanJSON, impl: "nothing"}

	code, stdout, _ := runInjectedCLI(t, dir, "   \n", a, "run", "--task", "TASK.md")
	if code != exitError {
		t.Errorf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stdout, "no changes") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRunValidationFailureFailsGate(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"false\"\n  test:\n    - \"true\"\n")
	a := &fakeCapabilityAgent{plan: validPlanJSON, impl: "x", review: `{"summary":"clean","findings":[]}`}

	code, stdout, _ := runInjectedCLI(t, dir, "diff\n", a, "run", "--task", "TASK.md")
	if code != exitError {
		t.Errorf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stdout, "FAIL") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRunBlockingFindingNeedsHuman(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")
	a := &fakeCapabilityAgent{
		plan:   validPlanJSON,
		impl:   "x",
		review: `{"summary":"issue","findings":[{"severity":"CRITICAL","title":"boom","file":"a.go","line":1}]}`,
	}

	// A finding that never clears exhausts the fix budget and needs a human.
	code, stdout, _ := runInjectedCLI(t, dir, "diff\n", a, "run", "--task", "TASK.md")
	if code != exitError {
		t.Errorf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stdout, "NEEDS_HUMAN") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRunMissingTaskFile(t *testing.T) {
	dir := t.TempDir()
	code, _, stderr := runInjectedCLI(t, dir, "diff\n", &fakeAgent{content: "{}"}, "run", "--task", "NOPE.md")
	if code != exitError {
		t.Errorf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "NOPE.md") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestRunBadArgs(t *testing.T) {
	dir := t.TempDir()
	code, _, stderr := runInjectedCLI(t, dir, "diff\n", &fakeAgent{content: "{}"}, "run", "a.md", "b.md")
	if code != exitUsage {
		t.Errorf("code=%d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr, "usage") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestRunGraphNoSource(t *testing.T) {
	dir := t.TempDir()
	code, _, stderr := runInjectedCLI(t, dir, "diff\n", &fakeAgent{content: "{}"}, "run")
	if code != exitError {
		t.Errorf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "PLAN.md") {
		t.Errorf("stderr should name the missing sources: %q", stderr)
	}
	if !stateExists(statePath(dir)) {
		t.Error("sop run should initialize state")
	}
}

func TestRunGraphFromExistingPlanJSON(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
	writePlanJSON(t, dir, validPlanJSON)
	a := &fakeCapabilityAgent{plan: validPlanJSON, impl: "x", review: `{"summary":"clean","findings":[]}`}

	code, stdout, stderr := runInjectedCLI(t, dir, "diff\n", a, "run")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "Created 2 task(s).") {
		t.Errorf("stdout = %q", stdout)
	}
	if !strings.Contains(stdout, "all tasks done") {
		t.Errorf("stdout = %q", stdout)
	}
}

const autoPlanDoc = "# Implementation Plan\n\n## Project\n\nWidget\n\n## Summary\n\nAdd a widget.\n\n## S001 — Application skeleton\n\nCreate the skeleton.\n\n### Dependencies\n\nNone\n\n### Deliverables\n\n- Go application\n\n### Acceptance Criteria\n\n- Application starts\n"

func TestRunAutoCompilesPlanFromMarkdown(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join("docs", "PLAN.md"), autoPlanDoc)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
	a := &fakeCapabilityAgent{plan: validPlanJSON, impl: "x", review: `{"summary":"clean","findings":[]}`}

	code, stdout, stderr := runInjectedCLI(t, dir, "diff\n", a, "run")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	for _, want := range []string{"Source: " + filepath.Join("docs", "PLAN.md"), "Created 1 task(s).", "S001 LOCAL_DONE", "all tasks done"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout)
		}
	}
	if !stateExists(filepath.Join(dir, stateDirName, "plan.json")) {
		t.Error("plan.json not written")
	}
}

func TestRunAutoGeneratesPlanFromPRD(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join("docs", "PRD.md"), "# Widget\n\nAdd a widget.\n")
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
	a := &fakeCapabilityAgent{plan: validPlanJSON, impl: "x", review: `{"summary":"clean","findings":[]}`}

	code, stdout, stderr := runInjectedCLI(t, dir, "diff\n", a, "run")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "Source: "+filepath.Join("docs", "PRD.md")) {
		t.Errorf("stdout = %q", stdout)
	}
	if !strings.Contains(stdout, "all tasks done") {
		t.Errorf("stdout = %q", stdout)
	}
	if !stateExists(filepath.Join(dir, "docs", "reports", "prd.md")) {
		t.Error("generated plan doc must be written under docs/reports/")
	}
	if stateExists(filepath.Join(dir, "PLAN.md")) {
		t.Error("generated plan must not be written to the project root")
	}
	if stateExists(filepath.Join(dir, ".gitignore")) {
		t.Error("runtime state must not require a root .gitignore")
	}
}

func TestRunGraphAllDone(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "already done", Status: domain.DONE, MaxAttempts: 3})
	code, stdout, stderr := runInjectedCLI(t, dir, "diff\n", &fakeAgent{content: "{}"}, "run")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "all tasks done") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRunGraphExecutesReadyTask(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
	seedTask(t, dir, &domain.Task{
		ID: "T001", Title: "Add widget", Objective: "Add the widget.",
		AcceptanceCriteria: "widget works", Status: domain.PLANNED, MaxAttempts: 3,
	})
	a := &fakeCapabilityAgent{plan: validPlanJSON, impl: "x", review: `{"summary":"clean","findings":[]}`}

	code, stdout, stderr := runInjectedCLI(t, dir, "diff\n", a, "run")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "T001 LOCAL_DONE") {
		t.Errorf("stdout missing completion: %q", stdout)
	}

	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	got, err := st.Get("T001")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.Status != domain.LOCAL_DONE {
		t.Errorf("persisted status = %s, want LOCAL_DONE", got.Status)
	}
	if got.Status == domain.DONE || got.Status == domain.MERGED {
		t.Error("local completion must not claim remote states")
	}
}

func TestRunGraphBlocksOnGateFailure(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"false\"\n")
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "Add widget", Status: domain.PLANNED, MaxAttempts: 3})
	a := &fakeCapabilityAgent{plan: validPlanJSON, impl: "x", fix: "x"}

	code, stdout, _ := runInjectedCLI(t, dir, "diff\n", a, "run")
	if code != exitError {
		t.Errorf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stdout, "T001 BLOCKED") {
		t.Errorf("stdout = %q", stdout)
	}

	st, _ := store.Open(statePath(dir))
	defer st.Close()
	got, _ := st.Get("T001")
	if got.Status != domain.BLOCKED {
		t.Errorf("persisted status = %s, want BLOCKED", got.Status)
	}
}

func TestRunFixLoopResolves(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
	a := &fakeCapabilityAgent{
		plan: validPlanJSON, impl: "impl", fix: "fixed",
		reviews: []string{
			`{"summary":"issue","findings":[{"severity":"HIGH","title":"bug","file":"a.go","line":1}]}`,
			`{"summary":"clean","findings":[]}`,
		},
	}

	code, stdout, stderr := runInjectedCLI(t, dir, "diff\n", a, "run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "fix cycles: 1/3") {
		t.Errorf("stdout = %q", stdout)
	}
	if !stateExists(filepath.Join(dir, stateDirName, "runs", "T001", "fix-1.md")) {
		t.Error("fix artifact not written")
	}
}

func TestRunFixLoopExhausted(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")
	a := &fakeCapabilityAgent{
		plan: validPlanJSON, impl: "impl", fix: "still broken",
		reviews: []string{`{"summary":"issue","findings":[{"severity":"CRITICAL","title":"bug","file":"a.go","line":1}]}`},
	}

	code, stdout, _ := runInjectedCLI(t, dir, "diff\n", a, "run", "--task", "TASK.md")
	if code != exitError {
		t.Errorf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stdout, "NEEDS_HUMAN") {
		t.Errorf("stdout = %q", stdout)
	}
	if !strings.Contains(stdout, "fix cycles: 3/3") {
		t.Errorf("stdout = %q", stdout)
	}
}

// fakeGitHub records the remote operations a command performs.
type fakeGitHub struct {
	pushed  string
	created github.CreateRequest
}

func (f *fakeGitHub) PushBranch(_ context.Context, branch string) error {
	f.pushed = branch
	return nil
}

func (f *fakeGitHub) CreatePullRequest(_ context.Context, req github.CreateRequest) (github.PullRequest, error) {
	f.created = req
	return github.PullRequest{Number: 1, URL: "https://example.com/pr/1"}, nil
}

func (f *fakeGitHub) GetPullRequest(context.Context, string) (github.PullRequest, error) {
	return github.PullRequest{}, nil
}

func (f *fakeGitHub) Checks(context.Context, int) ([]github.Check, error) { return nil, nil }

func (f *fakeGitHub) Merge(context.Context, int, string) error { return nil }

func TestRunCommitRequiresApproval(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeFile(t, dir, "TASK.md", runTaskFile)
	var out, errOut bytes.Buffer
	d := deps{
		getwd: func() (string, error) { return dir, nil },
		commit: func(context.Context, string, string) error {
			t.Error("commit must not run without approval")
			return nil
		},
	}
	if code := run([]string{"commit", "TASK.md"}, &out, &errOut, d); code != exitError {
		t.Fatalf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(errOut.String(), "approval required") {
		t.Errorf("stderr = %q", errOut.String())
	}
}

func TestRunCommitWithApproval(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeFile(t, dir, "TASK.md", runTaskFile)
	var got string
	var out, errOut bytes.Buffer
	d := deps{
		getwd:  func() (string, error) { return dir, nil },
		commit: func(_ context.Context, _, message string) error { got = message; return nil },
	}
	if code := run([]string{"commit", "TASK.md", "--yes"}, &out, &errOut, d); code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, errOut.String())
	}
	if got != "task(T001): Add widget" {
		t.Errorf("message = %q", got)
	}
}

func TestRunCommitNoApprovalWhenDisabled(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "project:\n  name: x\nhuman:\n  approval_before_commit: false\n")
	var got string
	var out, errOut bytes.Buffer
	d := deps{
		getwd:  func() (string, error) { return dir, nil },
		commit: func(_ context.Context, _, message string) error { got = message; return nil },
	}
	if code := run([]string{"commit"}, &out, &errOut, d); code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, errOut.String())
	}
	if got != "sop: apply changes" {
		t.Errorf("message = %q", got)
	}
}

func TestRunPr(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeFile(t, dir, "TASK.md", runTaskFile)
	gh := &fakeGitHub{}
	var out, errOut bytes.Buffer
	d := deps{
		getwd:     func() (string, error) { return dir, nil },
		newGitHub: func(string) github.Client { return gh },
	}
	if code := run([]string{"pr", "TASK.md", "--yes"}, &out, &errOut, d); code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, errOut.String())
	}
	if gh.pushed != "task/T001-add-widget" {
		t.Errorf("pushed = %q", gh.pushed)
	}
	if gh.created.Base != "main" || gh.created.Head != "task/T001-add-widget" {
		t.Errorf("create = %+v", gh.created)
	}
	if !strings.Contains(out.String(), "opened") {
		t.Errorf("stdout = %q", out.String())
	}
}

func TestRunPrRequiresApproval(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeFile(t, dir, "TASK.md", runTaskFile)
	var out, errOut bytes.Buffer
	d := deps{
		getwd:     func() (string, error) { return dir, nil },
		newGitHub: func(string) github.Client { t.Error("github must not be used without approval"); return &fakeGitHub{} },
	}
	if code := run([]string{"pr", "TASK.md"}, &out, &errOut, d); code != exitError {
		t.Fatalf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(errOut.String(), "approval required") {
		t.Errorf("stderr = %q", errOut.String())
	}
}

func TestRunPrRequiresTaskFile(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	var out, errOut bytes.Buffer
	d := deps{getwd: func() (string, error) { return dir, nil }}
	if code := run([]string{"pr", "--yes"}, &out, &errOut, d); code != exitUsage {
		t.Errorf("code=%d, want %d", code, exitUsage)
	}
}

func TestRunReport(t *testing.T) {
	dir := t.TempDir()
	runDir := filepath.Join(dir, stateDirName, "runs", "T001")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	report := `{"id":"T001","stage":"PASSED","provider":"command","review_engine":"self","decision":"PASS","fix_cycles":0,"validation":[{"Category":"BUILD","Command":"go build ./...","Status":"PASS"}],"findings":[{"Severity":"MEDIUM","Title":"x","File":"a.go","Line":1}]}`
	if err := os.WriteFile(filepath.Join(runDir, "report.json"), []byte(report), 0o644); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := runCLI(t, dir, "report")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	for _, want := range []string{"Run: T001", "Gate: PASS", "BUILD", "MEDIUM"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout)
		}
	}
}

func TestRunReportNoRuns(t *testing.T) {
	dir := t.TempDir()
	code, _, stderr := runCLI(t, dir, "report")
	if code != exitError {
		t.Errorf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "no runs") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestRunEval(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
	corpus := filepath.Join(dir, "corpus")
	if err := os.MkdirAll(corpus, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(corpus, "case1.md"), []byte(runTaskFile), 0o644); err != nil {
		t.Fatal(err)
	}
	a := &fakeCapabilityAgent{plan: validPlanJSON, impl: "x", review: `{"summary":"clean","findings":[]}`}

	code, stdout, stderr := runInjectedCLI(t, dir, "diff\n", a, "eval", "corpus")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "passed: 1") {
		t.Errorf("stdout = %q", stdout)
	}
	if !strings.Contains(stdout, "success: 100%") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRunEvalNoFiles(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	code, _, stderr := runInjectedCLI(t, dir, "diff\n", &fakeAgent{content: "{}"}, "eval", "empty")
	if code != exitError {
		t.Errorf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "no .md task files") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestRunEvalBadArgs(t *testing.T) {
	dir := t.TempDir()
	code, _, stderr := runCLI(t, dir, "eval")
	if code != exitUsage {
		t.Errorf("code=%d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr, "usage") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestRunRepeatedRunIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join("docs", "PLAN.md"), autoPlanDoc)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
	a := &fakeCapabilityAgent{plan: validPlanJSON, impl: "x", review: `{"summary":"clean","findings":[]}`}

	if code, out, errs := runInjectedCLI(t, dir, "diff\n", a, "run"); code != exitOK {
		t.Fatalf("first run: code=%d stderr=%s stdout=%s", code, errs, out)
	} else if !strings.Contains(out, "Created 1 task(s).") {
		t.Errorf("first run should create tasks: %q", out)
	}

	code, out, errs := runInjectedCLI(t, dir, "diff\n", a, "run")
	if code != exitOK {
		t.Fatalf("second run: code=%d stderr=%s stdout=%s", code, errs, out)
	}
	if strings.Contains(out, "Created ") {
		t.Errorf("second run must not recreate tasks: %q", out)
	}
	if !strings.Contains(out, "all tasks done") {
		t.Errorf("second run should report completion: %q", out)
	}
}

func TestRunValidateSubdirectoryCommand(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "backend"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - cd backend && true\n")

	code, stdout, stderr := runCLI(t, dir, "validate")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "validation: PASS") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRunBootstrapsWithoutAgent(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join("docs", "PLAN.md"), autoPlanDoc)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")

	// A nil agent makes newAgent return "no agent configured": preparation must
	// still discover the PLAN, compile plan.json, and create the tasks.
	code, stdout, stderr := runInjectedCLI(t, dir, "diff\n", nil, "run")
	if code != exitError {
		t.Fatalf("code=%d, want %d", code, exitError)
	}
	if !stateExists(filepath.Join(dir, stateDirName, "plan.json")) {
		t.Error("plan.json was not created without an agent")
	}
	if !strings.Contains(stdout, "Created 1 task(s).") {
		t.Errorf("stdout = %q, want a created task graph", stdout)
	}
	if !strings.Contains(stderr, "cannot execute") {
		t.Errorf("stderr = %q, want the missing-agent message", stderr)
	}
}

func TestRunNamedPlan(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join("docs", "PLAN-Hardening.md"), autoPlanDoc)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
	a := &fakeCapabilityAgent{plan: validPlanJSON, impl: "x", review: `{"summary":"clean","findings":[]}`}

	code, stdout, stderr := runInjectedCLI(t, dir, "diff\n", a, "run", filepath.Join("docs", "PLAN-Hardening.md"))
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	for _, want := range []string{
		"Source: " + filepath.Join("docs", "PLAN-Hardening.md"),
		"Plan ID: plan-hardening",
		"Created 1 task(s).",
		"S001 LOCAL_DONE",
		"SOP COMPLETE",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout)
		}
	}
}

func TestRunNamedPlanResolvesToDocs(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join("docs", "PLAN-Hardening.md"), autoPlanDoc)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")
	a := &fakeCapabilityAgent{plan: validPlanJSON, impl: "x", review: `{"summary":"clean","findings":[]}`}

	code, stdout, stderr := runInjectedCLI(t, dir, "diff\n", a, "run", "PLAN-Hardening.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "Source: "+filepath.Join("docs", "PLAN-Hardening.md")) {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestRunNamedPlanAmbiguous(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "PLAN-Hardening.md", autoPlanDoc)
	writeFile(t, dir, filepath.Join("docs", "PLAN-Hardening.md"), autoPlanDoc)

	code, _, stderr := runInjectedCLI(t, dir, "diff\n", &fakeAgent{content: "{}"}, "run", "PLAN-Hardening.md")
	if code != exitError {
		t.Fatalf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "multiple plans match") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestRunNamedPlanMissing(t *testing.T) {
	dir := t.TempDir()
	code, _, stderr := runInjectedCLI(t, dir, "diff\n", &fakeAgent{content: "{}"}, "run", "NOPE.md")
	if code != exitError {
		t.Fatalf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "PLAN not found") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestRunDifferentPlanStops(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join("docs", "PLAN-Hardening.md"), autoPlanDoc)
	writeFile(t, dir, filepath.Join("docs", "PLAN.md"), autoPlanDoc2)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
	a := &fakeCapabilityAgent{plan: validPlanJSON, impl: "x", review: `{"summary":"clean","findings":[]}`}

	if code, _, errs := runInjectedCLI(t, dir, "diff\n", a, "run", filepath.Join("docs", "PLAN-Hardening.md")); code != exitOK {
		t.Fatalf("first plan: code=%d stderr=%s", code, errs)
	}

	code, _, stderr := runInjectedCLI(t, dir, "diff\n", a, "run", filepath.Join("docs", "PLAN.md"))
	if code != exitError {
		t.Fatalf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "different plan is already active") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestRunChangedPlanStops(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	plan := filepath.Join("docs", "PLAN-Hardening.md")
	writeFile(t, dir, plan, autoPlanDoc)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
	a := &fakeCapabilityAgent{plan: validPlanJSON, impl: "x", review: `{"summary":"clean","findings":[]}`}

	if code, _, errs := runInjectedCLI(t, dir, "diff\n", a, "run", plan); code != exitOK {
		t.Fatalf("first run: code=%d stderr=%s", code, errs)
	}

	writeFile(t, dir, plan, strings.ReplaceAll(autoPlanDoc, "Application skeleton", "Renamed"))
	code, _, stderr := runInjectedCLI(t, dir, "diff\n", a, "run", plan)
	if code != exitError {
		t.Fatalf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "plan changed") {
		t.Errorf("stderr = %q", stderr)
	}
}

const autoPlanDoc2 = "# Implementation Plan\n\n## Project\n\nWidget\n\n## Summary\n\nOther plan.\n\n## SC-001 — Other work\n\nDo it.\n\n### Acceptance Criteria\n\n- ok\n"

func TestRunTasksBuildsFromPlan(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writePlanJSON(t, dir, validPlanJSON)

	code, stdout, stderr := runCLI(t, dir, "tasks")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "created 2 task(s)") {
		t.Errorf("stdout = %q", stdout)
	}

	code, out, _ := runCLI(t, dir, "status")
	if code != exitOK {
		t.Fatalf("status code=%d", code)
	}
	for _, want := range []string{"S001 PLANNED Application skeleton", "S002 PLANNED Ingestion"} {
		if !strings.Contains(out, want) {
			t.Errorf("status missing %q:\n%s", want, out)
		}
	}
}

func TestRunTasksNotInitialized(t *testing.T) {
	dir := t.TempDir()
	code, _, stderr := runCLI(t, dir, "tasks")
	if code != exitError {
		t.Errorf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "not initialized") {
		t.Errorf("stderr = %s", stderr)
	}
}

func TestRunTasksMissingPlan(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	code, _, stderr := runCLI(t, dir, "tasks")
	if code != exitError {
		t.Errorf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "plan.json") {
		t.Errorf("stderr = %s", stderr)
	}
}

func TestRunTasksMalformedPlan(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writePlanJSON(t, dir, "not json")
	code, _, stderr := runCLI(t, dir, "tasks")
	if code != exitError {
		t.Errorf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "parse plan.json") {
		t.Errorf("stderr = %s", stderr)
	}
}

func TestRunTasksInvalidPlan(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writePlanJSON(t, dir, `{"project":"p","summary":"s","stages":[]}`)
	code, _, stderr := runCLI(t, dir, "tasks")
	if code != exitError {
		t.Errorf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "no stages") {
		t.Errorf("stderr = %s", stderr)
	}
}

func TestRunTasksRejectsCyclicPlan(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writePlanJSON(t, dir, `{"project":"p","summary":"s","stages":[`+
		`{"id":"S1","title":"t","objective":"o","acceptance_criteria":["a"],"dependencies":["S2"]},`+
		`{"id":"S2","title":"t","objective":"o","acceptance_criteria":["a"],"dependencies":["S1"]}]}`)
	code, _, stderr := runCLI(t, dir, "tasks")
	if code != exitError {
		t.Errorf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "cycle") {
		t.Errorf("stderr = %s", stderr)
	}
}

func TestRunTasksRejectsExistingTasks(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writePlanJSON(t, dir, validPlanJSON)

	if code, _, stderr := runCLI(t, dir, "tasks"); code != exitOK {
		t.Fatalf("first tasks failed: code=%d stderr=%s", code, stderr)
	}

	code, _, stderr := runCLI(t, dir, "tasks")
	if code != exitError {
		t.Errorf("second run code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "already exists") {
		t.Errorf("stderr = %s", stderr)
	}
}

func TestRunTasksBadArgs(t *testing.T) {
	dir := t.TempDir()
	code, _, stderr := runCLI(t, dir, "tasks", "extra")
	if code != exitUsage {
		t.Errorf("code=%d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr, "usage") {
		t.Errorf("stderr = %s", stderr)
	}
}

func TestRunPlanThenTasksFlow(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "PRD.md", "# Book RAG\n\nAnswer questions over books.\n")

	code, _, stderr := runCLIWithAgent(t, dir, &fakeAgent{content: validPlanJSON}, "plan")
	if code != exitOK {
		t.Fatalf("plan code=%d stderr=%s", code, stderr)
	}

	initProject(t, dir)

	if code, _, stderr := runCLI(t, dir, "tasks"); code != exitOK {
		t.Fatalf("tasks code=%d stderr=%s", code, stderr)
	}

	code, out, stderr := runCLI(t, dir, "status")
	if code != exitOK {
		t.Fatalf("status code=%d stderr=%s", code, stderr)
	}
	for _, want := range []string{"S001", "S002"} {
		if !strings.Contains(out, want) {
			t.Errorf("status missing %q:\n%s", want, out)
		}
	}
}

// fakeResources is a resume observer for CLI tests.
type fakeResources struct {
	obs resume.Observation
	err error
}

func (f *fakeResources) Observe(context.Context, *domain.Task) (resume.Observation, error) {
	return f.obs, f.err
}

// runCLIWithResources runs the dispatcher with an injected resume observer.
func runCLIWithResources(t *testing.T, dir string, res resume.Observer, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	d := deps{
		getwd: func() (string, error) { return dir, nil },
		newResources: func(string) (resume.Observer, error) {
			return res, nil
		},
	}
	code = run(args, &out, &errOut, d)
	return code, out.String(), errOut.String()
}

func TestRunResumeReportsNextAction(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "T001", Status: domain.IMPLEMENTING, MaxAttempts: 1})

	res := &fakeResources{obs: resume.Observation{BranchExists: true}}
	code, out, stderr := runCLIWithResources(t, dir, res, "resume", "T001")
	if code != exitOK {
		t.Fatalf("resume code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(out, "IMPLEMENT") {
		t.Errorf("stdout = %q, want IMPLEMENT", out)
	}
}

func TestRunResumeRecoversExistingBranch(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "T001", Status: domain.READY, MaxAttempts: 1})

	res := &fakeResources{obs: resume.Observation{BranchExists: true}}
	code, out, stderr := runCLIWithResources(t, dir, res, "resume")
	if code != exitOK {
		t.Fatalf("resume code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(out, "WRITE_TESTS") || !strings.Contains(out, "recovered=BRANCH_CREATED") {
		t.Errorf("stdout = %q, want recovered WRITE_TESTS", out)
	}

	// The recovery is persisted, so a second resume continues from BRANCH_CREATED.
	code, out, stderr = runCLIWithResources(t, dir, res, "resume")
	if code != exitOK {
		t.Fatalf("second resume code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(out, "WRITE_TESTS") {
		t.Errorf("stdout = %q, want WRITE_TESTS", out)
	}
}

func TestRunResumeNothingToResume(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "T001", Status: domain.DONE, MaxAttempts: 1})

	code, out, stderr := runCLIWithResources(t, dir, &fakeResources{}, "resume")
	if code != exitOK {
		t.Fatalf("resume code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(out, "nothing to resume") {
		t.Errorf("stdout = %q, want nothing to resume", out)
	}
}

func TestRunResumeConsistencyError(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "T001", Status: domain.BRANCH_CREATED, MaxAttempts: 1})

	code, _, stderr := runCLIWithResources(t, dir, &fakeResources{}, "resume", "T001")
	if code != exitError {
		t.Errorf("code = %d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "branch does not exist") {
		t.Errorf("stderr = %q, want branch-missing message", stderr)
	}
}

func TestRunResumeBadArgs(t *testing.T) {
	dir := t.TempDir()
	code, _, stderr := runCLIWithResources(t, dir, &fakeResources{}, "resume", "a", "b")
	if code != exitUsage {
		t.Errorf("code=%d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr, "usage") {
		t.Errorf("stderr = %s", stderr)
	}
}

func TestRunResumeUninitialized(t *testing.T) {
	dir := t.TempDir()
	code, _, stderr := runCLIWithResources(t, dir, &fakeResources{}, "resume", "T001")
	if code != exitError {
		t.Errorf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, notInitializedMsg) {
		t.Errorf("stderr = %q, want %q", stderr, notInitializedMsg)
	}
}
