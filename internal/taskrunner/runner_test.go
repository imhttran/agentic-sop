package taskrunner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/git"
	"github.com/imhttran/agentic-sop/internal/store"
	"github.com/imhttran/agentic-sop/internal/testrunner"
)

// --- fakes ---

type fakeStore struct {
	tasks      map[string]*domain.Task
	saves      int
	failSaveAt int // 1-based; 0 = never
	history    []domain.TaskStatus
}

func newFakeStore(tasks ...*domain.Task) *fakeStore {
	m := make(map[string]*domain.Task, len(tasks))
	for _, task := range tasks {
		m[task.ID] = task
	}
	return &fakeStore{tasks: m}
}

func (f *fakeStore) Get(id string) (*domain.Task, error) {
	task, ok := f.tasks[id]
	if !ok {
		return nil, fmt.Errorf("task %s not found", id)
	}
	return clone(task), nil
}

func (f *fakeStore) Save(task *domain.Task) error {
	f.saves++
	if f.failSaveAt != 0 && f.saves == f.failSaveAt {
		return errors.New("save failed")
	}
	f.tasks[task.ID] = clone(task)
	f.history = append(f.history, task.Status)
	return nil
}

func (f *fakeStore) status(id string) domain.TaskStatus { return f.tasks[id].Status }

type fakeGit struct {
	validateErr error
	createErr   error
	created     []string
}

func (f *fakeGit) ValidateRepository(context.Context) error { return f.validateErr }

func (f *fakeGit) CreateBranch(_ context.Context, branch string) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.created = append(f.created, branch)
	return nil
}

type fakeAgent struct {
	responses map[agent.Capability]string
	errs      map[agent.Capability]error
	requests  []agent.Request
}

func newFakeAgent() *fakeAgent {
	return &fakeAgent{responses: map[agent.Capability]string{}, errs: map[agent.Capability]error{}}
}

func (f *fakeAgent) Generate(_ context.Context, req agent.Request) (agent.Response, error) {
	f.requests = append(f.requests, req)
	if err := f.errs[req.Capability]; err != nil {
		return agent.Response{}, err
	}
	content := f.responses[req.Capability]
	if content == "" {
		content = string(req.Capability) + "-content"
	}
	return agent.Response{Content: content}, nil
}

func (f *fakeAgent) capabilities() []agent.Capability {
	caps := make([]agent.Capability, 0, len(f.requests))
	for _, req := range f.requests {
		caps = append(caps, req.Capability)
	}
	return caps
}

type fakeApplier struct {
	err     error
	applied []agent.Response
}

func (f *fakeApplier) Apply(_ context.Context, work agent.Response) error {
	if f.err != nil {
		return f.err
	}
	f.applied = append(f.applied, work)
	return nil
}

type fakeVerify struct {
	runResults   []testrunner.Result
	suiteResults []testrunner.SuiteResult
	runIdx       int
	suiteIdx     int
	runCalls     int
	suiteCalls   int
}

func (f *fakeVerify) Run(context.Context, testrunner.Check) testrunner.Result {
	f.runCalls++
	if f.runIdx < len(f.runResults) {
		result := f.runResults[f.runIdx]
		f.runIdx++
		return result
	}
	if len(f.runResults) > 0 {
		return f.runResults[len(f.runResults)-1]
	}
	return testrunner.Result{Status: testrunner.Pass}
}

func (f *fakeVerify) RunAll(context.Context, []testrunner.Check) testrunner.SuiteResult {
	f.suiteCalls++
	if f.suiteIdx < len(f.suiteResults) {
		result := f.suiteResults[f.suiteIdx]
		f.suiteIdx++
		return result
	}
	if len(f.suiteResults) > 0 {
		return f.suiteResults[len(f.suiteResults)-1]
	}
	return testrunner.SuiteResult{Status: testrunner.Pass}
}

// --- helpers ---

func res(status testrunner.Status) testrunner.Result { return testrunner.Result{Status: status} }

func suite(status testrunner.Status) testrunner.SuiteResult {
	return testrunner.SuiteResult{Status: status}
}

func readyTask() *domain.Task {
	return &domain.Task{ID: "T001", Title: "Add greeting", Status: domain.READY, MaxAttempts: 3}
}

func testConfig() Config {
	return Config{TestCheck: testrunner.Check{Category: testrunner.UnitTest, Command: "test"}}
}

func newTestRunner(s *fakeStore, g *fakeGit, a *fakeAgent, ap *fakeApplier, v *fakeVerify) *Runner {
	return New(s, g, a, ap, v, testConfig())
}

func containsInOrder(haystack []domain.TaskStatus, needles ...domain.TaskStatus) bool {
	i := 0
	for _, h := range haystack {
		if i < len(needles) && h == needles[i] {
			i++
		}
	}
	return i == len(needles)
}

// --- tests ---

func TestRunHappyPath(t *testing.T) {
	st := newFakeStore(readyTask())
	g := &fakeGit{}
	a := newFakeAgent()
	ap := &fakeApplier{}
	v := &fakeVerify{runResults: []testrunner.Result{res(testrunner.Fail), res(testrunner.Pass)}, suiteResults: []testrunner.SuiteResult{suite(testrunner.Pass)}}

	result, err := newTestRunner(st, g, a, ap, v).Run(context.Background(), "T001")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result.Outcome != LocalTestsPass {
		t.Errorf("outcome = %s, want LOCAL_TESTS_PASS", result.Outcome)
	}
	if st.status("T001") != domain.LOCAL_TESTS_PASS {
		t.Errorf("persisted status = %s, want LOCAL_TESTS_PASS", st.status("T001"))
	}
	if !containsInOrder(st.history, domain.BRANCH_CREATED, domain.TESTS_WRITTEN, domain.RED_VERIFIED, domain.IMPLEMENTING, domain.LOCAL_TESTS_PASS) {
		t.Errorf("transitions not in legal order: %v", st.history)
	}
	if len(g.created) != 1 || g.created[0] != "task/T001-add-greeting" {
		t.Errorf("branch = %v, want [task/T001-add-greeting]", g.created)
	}
	if v.suiteCalls == 0 {
		t.Error("final suite was not run")
	}
	for _, cap := range a.capabilities() {
		if cap == agent.Review {
			t.Error("REVIEW must not be requested in T010")
		}
	}
}

func TestRunRejectsWrongInitialState(t *testing.T) {
	for _, status := range []domain.TaskStatus{domain.PLANNED, domain.BRANCH_CREATED, domain.TESTS_WRITTEN, domain.RED_VERIFIED, domain.IMPLEMENTING, domain.LOCAL_TESTS_PASS, domain.BLOCKED, domain.DONE} {
		task := readyTask()
		task.Status = status
		st := newFakeStore(task)
		g := &fakeGit{}
		a := newFakeAgent()

		_, err := newTestRunner(st, g, a, &fakeApplier{}, &fakeVerify{}).Run(context.Background(), "T001")
		if err == nil {
			t.Errorf("status %s: expected error", status)
		}
		if len(g.created) != 0 {
			t.Errorf("status %s: git must not be called", status)
		}
		if len(a.requests) != 0 {
			t.Errorf("status %s: agent must not be called", status)
		}
		if st.status("T001") != status {
			t.Errorf("status %s: state mutated to %s", status, st.status("T001"))
		}
	}
}

func TestRunBranchFailureLeavesReady(t *testing.T) {
	st := newFakeStore(readyTask())
	g := &fakeGit{createErr: errors.New("boom")}
	a := newFakeAgent()
	v := &fakeVerify{}

	_, err := newTestRunner(st, g, a, &fakeApplier{}, v).Run(context.Background(), "T001")
	if err == nil {
		t.Fatal("expected error when branch creation fails")
	}
	if st.status("T001") != domain.READY {
		t.Errorf("status = %s, want READY", st.status("T001"))
	}
	if len(a.requests) != 0 {
		t.Error("agent must not be invoked when branch creation fails")
	}
	if v.runCalls != 0 {
		t.Error("tests must not run when branch creation fails")
	}
}

func TestRunBranchPersistenceFailure(t *testing.T) {
	st := newFakeStore(readyTask())
	st.failSaveAt = 1 // first save is BRANCH_CREATED
	g := &fakeGit{}
	a := newFakeAgent()

	_, err := newTestRunner(st, g, a, &fakeApplier{}, &fakeVerify{}).Run(context.Background(), "T001")
	if err == nil {
		t.Fatal("expected consistency error")
	}
	if !strings.Contains(err.Error(), "BRANCH_CREATED") {
		t.Errorf("error = %q, want it to mention BRANCH_CREATED", err)
	}
	if st.status("T001") != domain.READY {
		t.Errorf("status = %s, want READY", st.status("T001"))
	}
	if len(a.requests) != 0 {
		t.Error("later steps must not run after a persistence failure")
	}
}

func TestRunTestDesignFailure(t *testing.T) {
	st := newFakeStore(readyTask())
	a := newFakeAgent()
	a.errs[agent.DesignTests] = errors.New("agent down")

	_, err := newTestRunner(st, &fakeGit{}, a, &fakeApplier{}, &fakeVerify{}).Run(context.Background(), "T001")
	if err == nil {
		t.Fatal("expected error")
	}
	if st.status("T001") != domain.BRANCH_CREATED {
		t.Errorf("status = %s, want BRANCH_CREATED", st.status("T001"))
	}
	for _, cap := range a.capabilities() {
		if cap == agent.Implement {
			t.Error("IMPLEMENT must not be invoked when test design fails")
		}
	}
}

func TestRunApplyTestFailure(t *testing.T) {
	st := newFakeStore(readyTask())
	ap := &fakeApplier{err: errors.New("cannot write")}

	_, err := newTestRunner(st, &fakeGit{}, newFakeAgent(), ap, &fakeVerify{}).Run(context.Background(), "T001")
	if err == nil {
		t.Fatal("expected error")
	}
	if st.status("T001") != domain.BRANCH_CREATED {
		t.Errorf("status = %s, want BRANCH_CREATED", st.status("T001"))
	}
}

func TestRunFalseREDInfrastructureError(t *testing.T) {
	st := newFakeStore(readyTask())
	a := newFakeAgent()
	v := &fakeVerify{runResults: []testrunner.Result{res(testrunner.Error)}}

	_, err := newTestRunner(st, &fakeGit{}, a, &fakeApplier{}, v).Run(context.Background(), "T001")
	if err == nil {
		t.Fatal("expected error for infrastructure failure during RED")
	}
	if st.status("T001") != domain.TESTS_WRITTEN {
		t.Errorf("status = %s, want TESTS_WRITTEN", st.status("T001"))
	}
	if containsInOrder(st.history, domain.RED_VERIFIED) {
		t.Error("ERROR must not establish RED_VERIFIED")
	}
	for _, cap := range a.capabilities() {
		if cap == agent.Implement {
			t.Error("IMPLEMENT must not run without RED")
		}
	}
}

func TestRunUnexpectedlyGreenBlocks(t *testing.T) {
	st := newFakeStore(readyTask())
	a := newFakeAgent()
	v := &fakeVerify{runResults: []testrunner.Result{res(testrunner.Pass)}} // always pass

	result, err := newTestRunner(st, &fakeGit{}, a, &fakeApplier{}, v).Run(context.Background(), "T001")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Outcome != BlockedOutcome {
		t.Errorf("outcome = %s, want BLOCKED", result.Outcome)
	}
	if st.status("T001") != domain.BLOCKED {
		t.Errorf("status = %s, want BLOCKED", st.status("T001"))
	}
	if st.tasks["T001"].Attempt != 3 {
		t.Errorf("attempt = %d, want 3", st.tasks["T001"].Attempt)
	}
	if st.tasks["T001"].BlockedReason != domain.TEST_DESIGN_FAILED {
		t.Errorf("blocked reason = %s, want TEST_DESIGN_FAILED", st.tasks["T001"].BlockedReason)
	}
	for _, cap := range a.capabilities() {
		if cap == agent.Implement {
			t.Error("IMPLEMENT must not run when RED cannot be established")
		}
	}
}

func TestRunImplementationOrdering(t *testing.T) {
	st := newFakeStore(readyTask())
	a := newFakeAgent()
	v := &fakeVerify{runResults: []testrunner.Result{res(testrunner.Fail), res(testrunner.Pass)}, suiteResults: []testrunner.SuiteResult{suite(testrunner.Pass)}}

	if _, err := newTestRunner(st, &fakeGit{}, a, &fakeApplier{}, v).Run(context.Background(), "T001"); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if caps := a.capabilities(); !containsCapsInOrder(caps, agent.DesignTests, agent.Implement) {
		t.Errorf("capabilities = %v, want DESIGN_TESTS before IMPLEMENT", caps)
	}
}

func TestRunRetrySuccess(t *testing.T) {
	st := newFakeStore(readyTask())
	a := newFakeAgent()
	v := &fakeVerify{
		runResults:   []testrunner.Result{res(testrunner.Fail), res(testrunner.Fail), res(testrunner.Pass)},
		suiteResults: []testrunner.SuiteResult{suite(testrunner.Pass)},
	}

	result, err := newTestRunner(st, &fakeGit{}, a, &fakeApplier{}, v).Run(context.Background(), "T001")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result.Outcome != LocalTestsPass || st.status("T001") != domain.LOCAL_TESTS_PASS {
		t.Errorf("outcome=%s status=%s, want LOCAL_TESTS_PASS", result.Outcome, st.status("T001"))
	}
	if st.tasks["T001"].Attempt != 2 {
		t.Errorf("attempt = %d, want 2", st.tasks["T001"].Attempt)
	}
	if !containsCapsInOrder(a.capabilities(), agent.DiagnoseFailure, agent.Fix) {
		t.Errorf("capabilities = %v, want DIAGNOSE_FAILURE then FIX", a.capabilities())
	}
}

func TestRunRetryExhaustion(t *testing.T) {
	st := newFakeStore(readyTask())
	a := newFakeAgent()
	v := &fakeVerify{runResults: []testrunner.Result{res(testrunner.Fail)}} // always fail

	result, err := newTestRunner(st, &fakeGit{}, a, &fakeApplier{}, v).Run(context.Background(), "T001")
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.Outcome != BlockedOutcome {
		t.Errorf("outcome = %s, want BLOCKED", result.Outcome)
	}
	if st.status("T001") != domain.BLOCKED {
		t.Errorf("status = %s, want BLOCKED", st.status("T001"))
	}
	if st.tasks["T001"].Attempt != 3 || st.tasks["T001"].Attempt != st.tasks["T001"].MaxAttempts {
		t.Errorf("attempt = %d, want 3 (== MaxAttempts)", st.tasks["T001"].Attempt)
	}
	if st.tasks["T001"].BlockedReason != domain.RETRIES_EXHAUSTED {
		t.Errorf("blocked reason = %s, want RETRIES_EXHAUSTED", st.tasks["T001"].BlockedReason)
	}
}

func TestRunFinalSuiteFailureThenFix(t *testing.T) {
	st := newFakeStore(readyTask())
	a := newFakeAgent()
	v := &fakeVerify{
		runResults:   []testrunner.Result{res(testrunner.Fail), res(testrunner.Pass), res(testrunner.Pass)},
		suiteResults: []testrunner.SuiteResult{suite(testrunner.Fail), suite(testrunner.Pass)},
	}

	result, err := newTestRunner(st, &fakeGit{}, a, &fakeApplier{}, v).Run(context.Background(), "T001")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result.Outcome != LocalTestsPass {
		t.Errorf("outcome = %s, want LOCAL_TESTS_PASS", result.Outcome)
	}
	if v.suiteCalls < 2 {
		t.Errorf("final suite calls = %d, want >= 2", v.suiteCalls)
	}
}

func TestRunCancellationIsNotBlocked(t *testing.T) {
	st := newFakeStore(readyTask())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := newTestRunner(st, &fakeGit{}, newFakeAgent(), &fakeApplier{}, &fakeVerify{}).Run(ctx, "T001")
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if st.status("T001") == domain.BLOCKED {
		t.Error("cancellation must not block the task")
	}
}

func TestRunCanceledDuringRed(t *testing.T) {
	st := newFakeStore(readyTask())
	v := &fakeVerify{runResults: []testrunner.Result{res(testrunner.Canceled)}}

	_, err := newTestRunner(st, &fakeGit{}, newFakeAgent(), &fakeApplier{}, v).Run(context.Background(), "T001")
	if err == nil || !strings.Contains(err.Error(), "canceled") {
		t.Errorf("err = %v, want a cancellation error", err)
	}
	if st.status("T001") == domain.BLOCKED {
		t.Error("cancellation must not block the task")
	}
}

func TestRunPersistenceFailureDoesNotAdvanceState(t *testing.T) {
	st := newFakeStore(readyTask())
	st.failSaveAt = 2 // BRANCH_CREATED ok, TESTS_WRITTEN fails

	_, err := newTestRunner(st, &fakeGit{}, newFakeAgent(), &fakeApplier{}, &fakeVerify{}).Run(context.Background(), "T001")
	if err == nil {
		t.Fatal("expected persistence error")
	}
	if st.status("T001") != domain.BRANCH_CREATED {
		t.Errorf("status = %s, want BRANCH_CREATED", st.status("T001"))
	}
}

func containsCapsInOrder(caps []agent.Capability, want ...agent.Capability) bool {
	i := 0
	for _, c := range caps {
		if i < len(want) && c == want[i] {
			i++
		}
	}
	return i == len(want)
}

// --- integration test (real git + real store + real test runner, fake agent) ---

type scriptedApplier struct {
	dir string
}

func (a *scriptedApplier) Apply(_ context.Context, work agent.Response) error {
	switch {
	case strings.Contains(work.Content, "design"):
		return os.WriteFile(a.path("check.sh"), []byte("#!/bin/sh\n[ -f impl.txt ]\n"), 0o644)
	case strings.Contains(work.Content, "impl"):
		return os.WriteFile(a.path("impl.txt"), []byte("hello\n"), 0o644)
	}
	return nil
}

func (a *scriptedApplier) path(name string) string { return filepath.Join(a.dir, name) }

func TestRunIntegration(t *testing.T) {
	repo := initRepo(t)

	statePath := filepath.Join(t.TempDir(), "state.db")
	st, err := store.Open(statePath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	task := &domain.Task{ID: "T001", Title: "Greeting", Objective: "Create impl.txt", AcceptanceCriteria: "impl.txt exists", Status: domain.READY, MaxAttempts: 3}
	if err := st.Save(task); err != nil {
		t.Fatalf("save task: %v", err)
	}

	a := newFakeAgent()
	a.responses[agent.DesignTests] = "design content"
	a.responses[agent.Implement] = "impl content"

	applier := &scriptedApplier{dir: repo}
	v := testrunner.New(repo)
	cfg := Config{
		TestCheck:   testrunner.Check{Category: testrunner.UnitTest, Command: "sh check.sh"},
		FinalChecks: []testrunner.Check{{Category: testrunner.Build, Command: "true"}},
	}

	runner := New(st, git.New(repo), a, applier, v, cfg)
	result, err := runner.Run(context.Background(), "T001")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result.Outcome != LocalTestsPass {
		t.Fatalf("outcome = %s, want LOCAL_TESTS_PASS", result.Outcome)
	}

	loaded, err := st.Get("T001")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if loaded.Status != domain.LOCAL_TESTS_PASS {
		t.Errorf("persisted status = %s, want LOCAL_TESTS_PASS", loaded.Status)
	}

	current := currentBranch(t, repo)
	if current != "task/T001-greeting" {
		t.Errorf("current branch = %s, want task/T001-greeting", current)
	}
}

func initRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	t.Setenv("GIT_AUTHOR_NAME", "SOP Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "SOP Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")

	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "initial")
	return dir
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func currentBranch(t *testing.T, dir string) string {
	t.Helper()
	return strings.TrimSpace(runGit(t, dir, "rev-parse", "--abbrev-ref", "HEAD"))
}
