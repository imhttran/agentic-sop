package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/github"
	"github.com/imhttran/agentic-sop/internal/perf"
	"github.com/imhttran/agentic-sop/internal/review"
	"github.com/imhttran/agentic-sop/internal/testrunner"
)

// runBench runs the CLI with a working tree that only changes once the agent has
// implemented, so a verify-first task and a later implement task see different
// trees — as they would in a real repository.
func runBench(t *testing.T, dir string, a *benchAgent, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	d := deps{
		getwd:    func() (string, error) { return dir, nil },
		newAgent: func(string, string, string) (agent.Agent, error) { return a, nil },
		readDiff: func(context.Context, string) (string, error) {
			if a.implement == 0 {
				return "", nil // nothing has been implemented yet
			}
			return "diff\n", nil
		},
		commit:    func(context.Context, string, string) error { return nil },
		newGitHub: func(string) github.Client { return &fakeGitHub{} },
	}
	code := run(args, &out, &errOut, d)
	return code, out.String(), errOut.String()
}

// benchAgent counts agent calls per capability, so the benchmark asserts operation
// counts (stable on any machine) rather than wall-clock time.
type benchAgent struct {
	plan, implement, fix, review int
}

func (a *benchAgent) Generate(_ context.Context, r agent.Request) (agent.Response, error) {
	switch r.Capability {
	case agent.Plan:
		a.plan++
		return agent.Response{Content: validPlanJSON}, nil
	case agent.Implement:
		a.implement++
		return agent.Response{Content: "done"}, nil
	case agent.Fix:
		a.fix++
		return agent.Response{Content: "fixed"}, nil
	case agent.Review:
		a.review++
		return agent.Response{Content: `{"summary":"clean","findings":[]}`}, nil
	default:
		return agent.Response{Content: "{}"}, nil
	}
}

// TestPerformanceBenchmarkFixture is the deterministic performance fixture: a
// small multi-task plan with an already-satisfied verify-first task, a small
// implementation task, and a dependency between them. It asserts operation counts,
// never wall-clock, so a slow CI machine cannot make it fail.
func TestPerformanceBenchmarkFixture(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")
	// A plan document gives the run a stable identity so the run-level aggregate is
	// persisted; the tasks themselves are seeded.
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "docs/PLAN.md", "# Plan\n\n## S001 — Verify\n\n## S002 — Implement\n")
	seedTask(t, dir, &domain.Task{ID: "S001", Title: "Verify boundary", Status: domain.PLANNED, MaxAttempts: 3, ExecutionMode: domain.ExecutionVerifyFirst})
	seedTask(t, dir, &domain.Task{ID: "S002", Title: "Implement", Status: domain.PLANNED, MaxAttempts: 3, DependencyIDs: []string{"S001"}})

	a := &benchAgent{}
	code, stdout, stderr := runBench(t, dir, a, "run")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if a.fix != 0 {
		t.Errorf("the fixture should not need a fix cycle; got %d", a.fix)
	}
	// S001 is verify-first (no agent); S002 costs PLAN + IMPLEMENT + REVIEW.
	if a.plan != 1 || a.implement != 1 || a.review != 1 {
		t.Errorf("agent calls = plan:%d implement:%d review:%d, want 1/1/1", a.plan, a.implement, a.review)
	}

	s1 := mustTaskMetrics(t, dir, "S001")
	if s1.Counts.AgentCalls != 0 || s1.Counts.AgentCallsAvoided != 1 {
		t.Errorf("S001 counts = %+v, want one avoided agent call and none made", s1.Counts)
	}
	s2 := mustTaskMetrics(t, dir, "S002")
	if s2.Counts.AgentCalls != 2 || s2.Counts.ValidationRuns != 1 || s2.Counts.ReviewRuns != 1 {
		t.Errorf("S002 counts = %+v, want 2 agent calls, 1 validation, 1 review", s2.Counts)
	}

	run, ok := findRunAggregate(t, dir)
	if !ok {
		t.Fatal("the run-level metrics.json was not written")
	}
	if len(run.Tasks) != 2 || run.Counts.AgentCalls != 2 || run.Counts.AgentCallsAvoided != 1 {
		t.Errorf("run aggregate = %+v", run)
	}
}

// TestValidationReuseAcrossUnchangedVerifyFirstTasks is the regression check for
// PERF007: two already-satisfied verify-first tasks see the same tree and command
// set, so the second safely reuses the first's passing validation instead of
// re-running it.
func TestValidationReuseAcrossUnchangedVerifyFirstTasks(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")
	for _, id := range []string{"S001", "S002"} {
		seedTask(t, dir, &domain.Task{ID: id, Title: "Verify " + id, Status: domain.PLANNED, MaxAttempts: 3, ExecutionMode: domain.ExecutionVerifyFirst})
	}
	a := &countingAgent{}

	// An empty working-tree change means both tasks verify the identical tree.
	code, stdout, stderr := runInjectedCLI(t, dir, "", a, "run")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if a.calls != 0 {
		t.Errorf("verify-first tasks must not invoke the agent; got %d", a.calls)
	}
	s1 := mustTaskMetrics(t, dir, "S001")
	if s1.Counts.ValidationRuns != 1 || s1.Counts.ValidationReused != 0 {
		t.Errorf("S001 validation = %+v, want one run", s1.Counts)
	}
	s2 := mustTaskMetrics(t, dir, "S002")
	if s2.Counts.ValidationRuns != 0 || s2.Counts.ValidationReused != 1 {
		t.Errorf("S002 validation = %+v, want one safe reuse", s2.Counts)
	}
}

// TestValidationReuseInvalidatedByChangedInputs proves the reuse cache cannot
// return a stale result: any change to the diff or the command set misses, and a
// failing result is never cached.
func TestValidationReuseInvalidatedByChangedInputs(t *testing.T) {
	s := newRunSession()
	v := config.Validation{Build: []string{"go build ./..."}}

	key := validationIdentity(v, "diff-a")
	s.cacheValidation(key, testrunner.SuiteResult{Status: testrunner.Pass})
	if _, ok := s.cachedValidation(key); !ok {
		t.Fatal("identical inputs should hit")
	}
	if _, ok := s.cachedValidation(validationIdentity(v, "diff-b")); ok {
		t.Error("a changed working tree must not reuse a stale validation")
	}
	if _, ok := s.cachedValidation(validationIdentity(config.Validation{Build: []string{"go test ./..."}}, "diff-a")); ok {
		t.Error("a changed command set must not reuse a stale validation")
	}

	failKey := validationIdentity(v, "diff-fail")
	s.cacheValidation(failKey, testrunner.SuiteResult{Status: testrunner.Fail})
	if _, ok := s.cachedValidation(failKey); ok {
		t.Error("a failing validation must not be cached")
	}

	cleanKey := reviewIdentity("self", "task", "diff-a")
	s.cacheReview(cleanKey, review.Report{Summary: "clean"})
	if _, ok := s.cachedReview(cleanKey); !ok {
		t.Error("a clean review should hit for identical inputs")
	}
	if _, ok := s.cachedReview(reviewIdentity("self", "task", "diff-b")); ok {
		t.Error("a changed diff must not reuse a stale review")
	}
	if _, ok := s.cachedReview(reviewIdentity("self", "other", "diff-a")); ok {
		t.Error("a changed task must not reuse a stale review")
	}
	dirtyKey := reviewIdentity("self", "task2", "diff-a")
	s.cacheReview(dirtyKey, review.Report{Findings: []review.Finding{{Title: "x"}}})
	if _, ok := s.cachedReview(dirtyKey); ok {
		t.Error("a review with findings must not be cached")
	}
}

// TestReportShowsPerformance checks that `sop report` surfaces where time went.
func TestReportShowsPerformance(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")
	seedTask(t, dir, &domain.Task{ID: "S001", Title: "Implement", Status: domain.PLANNED, MaxAttempts: 3})
	if code, _, stderr := runInjectedCLI(t, dir, "diff\n", &recordingAgent{}, "run"); code != exitOK {
		t.Fatalf("run failed: %s", stderr)
	}

	code, stdout, stderr := runCLI(t, dir, "report")
	if code != exitOK {
		t.Fatalf("report: %s", stderr)
	}
	for _, want := range []string{"Performance", "Agent calls:", "Validation runs:"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("report missing %q:\n%s", want, stdout)
		}
	}
}

// invalidPlanAgent always returns an invalid PLAN document, so plan generation
// exhausts its bounded repair budget and the task fails.
type invalidPlanAgent struct{ plan int }

func (a *invalidPlanAgent) Generate(_ context.Context, r agent.Request) (agent.Response, error) {
	if r.Capability == agent.Plan {
		a.plan++
		return agent.Response{Content: `{"project":"p","summary":"s","stages":[{"id":"S1","title":"t","objective":"o","acceptance_criteria":["a"],"dependencies":["S1"]}]}`}, nil
	}
	return agent.Response{Content: "done"}, nil
}

// TestPlanRepairAccountingOnExhaustion proves the run's metrics count every PLAN
// agent call, including the initial one, when the bounded repair budget is
// exhausted and the task fails. It asserts the invariant counted == actually
// invoked rather than a fixed bound, so it catches an undercount on the failure
// path wherever the bound moves.
func TestPlanRepairAccountingOnExhaustion(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "docs/PLAN.md", "# Plan\n\n## S001 — Do the thing\n")
	seedTask(t, dir, &domain.Task{ID: "S001", Title: "Do the thing", Status: domain.PLANNED, MaxAttempts: 1})

	a := &invalidPlanAgent{}
	code, stdout, stderr := runInjectedCLI(t, dir, "", a, "run")
	if code == exitOK {
		t.Fatalf("expected the run to fail for a persistently invalid plan; stdout=%s", stdout)
	}
	if !strings.Contains(stderr, "depends on itself") {
		t.Errorf("stderr = %q, want the deterministic validation error", stderr)
	}

	m := mustTaskMetrics(t, dir, "S001")
	if m.Counts.AgentCalls != a.plan {
		t.Errorf("agent calls = %d, want %d (every PLAN invocation counted, including the initial one on failure)", m.Counts.AgentCalls, a.plan)
	}
	if a.plan < 2 || m.Counts.PlanRepairs != a.plan-1 {
		t.Errorf("agent invocations = %d, plan repairs = %d, want one repair per correction after the first", a.plan, m.Counts.PlanRepairs)
	}
}

func mustTaskMetrics(t *testing.T, dir, id string) perf.Task {
	t.Helper()
	m, ok := loadTaskMetrics(dir, id)
	if !ok {
		t.Fatalf("no metrics for %s", id)
	}
	return m
}

// findRunAggregate locates the run-level metrics.json among the run directories.
func findRunAggregate(t *testing.T, dir string) (perf.Run, bool) {
	t.Helper()
	root := filepath.Join(dir, stateDirName, "runs")
	entries, err := os.ReadDir(root)
	if err != nil {
		return perf.Run{}, false
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, e.Name(), "metrics.json"))
		if err != nil {
			continue
		}
		var run perf.Run
		if json.Unmarshal(data, &run) == nil && len(run.Tasks) > 0 {
			return run, true
		}
	}
	return perf.Run{}, false
}
