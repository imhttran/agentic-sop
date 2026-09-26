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

	"github.com/imhttran/agentic-sdlc/internal/agent"
	"github.com/imhttran/agentic-sdlc/internal/domain"
	"github.com/imhttran/agentic-sdlc/internal/resume"
	"github.com/imhttran/agentic-sdlc/internal/store"
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
		newAgent: func() (agent.Agent, error) {
			if a == nil {
				return nil, errors.New("no agent configured")
			}
			return a, nil
		},
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
	code, _, stderr := runCLI(t, dir, "plan", "extra")
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
