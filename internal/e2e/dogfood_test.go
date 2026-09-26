// Package e2e exercises SOP's control plane end to end. The real components
// (planner, task builder, store, scheduler/completion, merge gate, CI
// remediation, review, parallelism, handoff) are composed here; only the
// external boundaries (model, Git, GitHub) are faked, so the run is
// deterministic and needs no network.
package e2e

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/imhttran/agentic-sdlc/internal/agent"
	"github.com/imhttran/agentic-sdlc/internal/ciremediation"
	"github.com/imhttran/agentic-sdlc/internal/completion"
	"github.com/imhttran/agentic-sdlc/internal/domain"
	"github.com/imhttran/agentic-sdlc/internal/github"
	"github.com/imhttran/agentic-sdlc/internal/handoff"
	"github.com/imhttran/agentic-sdlc/internal/mergegate"
	"github.com/imhttran/agentic-sdlc/internal/parallel"
	"github.com/imhttran/agentic-sdlc/internal/planner"
	"github.com/imhttran/agentic-sdlc/internal/review"
	"github.com/imhttran/agentic-sdlc/internal/store"
	"github.com/imhttran/agentic-sdlc/internal/taskbuilder"
	"github.com/imhttran/agentic-sdlc/internal/testrunner"
)

// happyPath is the sequential status chain from READY to a mergeable task.
var happyPath = []domain.TaskStatus{
	domain.BRANCH_CREATED,
	domain.TESTS_WRITTEN,
	domain.RED_VERIFIED,
	domain.IMPLEMENTING,
	domain.LOCAL_TESTS_PASS,
	domain.REVIEW,
	domain.REVIEW_PASS,
	domain.PR_OPEN,
	domain.CI_RUNNING,
	domain.CI_PASS,
}

func TestDogfoodPipeline(t *testing.T) {
	ctx := context.Background()

	// 1. PRD -> Plan (fake model only).
	prd := readFixture(t, "PRD.md")
	planJSON := readFixture(t, "plan.json")
	plan, err := planner.New(&fakeAgent{content: planJSON}).Generate(ctx, prd)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if len(plan.Stages) < 5 {
		t.Fatalf("plan has %d stages, want >= 5", len(plan.Stages))
	}
	if !hasDependencyChain(plan) {
		t.Error("plan has no dependency chain")
	}
	if !planHasDeliverable(plan, "integration tests") || !planHasDeliverable(plan, "Docker image") || !planHasDeliverable(plan, "GitHub Actions workflow") {
		t.Error("plan is missing integration tests, Docker, or CI deliverables")
	}

	// 2. Plan -> persisted tasks (real SQLite).
	st, err := store.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	tasks, err := taskbuilder.CreateTasksFromPlan(plan, st.SaveTasks)
	if err != nil {
		t.Fatalf("create tasks: %v", err)
	}
	if len(tasks) != 5 {
		t.Fatalf("built %d tasks, want 5", len(tasks))
	}
	// The environment bootstrap stage is an implicit dependency of every feature.
	for _, task := range tasks {
		if task.ID == "S000" {
			continue
		}
		if !dependsOn(task, "S000") {
			t.Errorf("feature %s does not depend on the environment task", task.ID)
		}
	}

	// 3. Dependency-aware parallel selection: with S000 and S001 done, S002 and
	// S003 (both depending only on S001) are runnable together.
	view := cloneTasks(tasks)
	for _, task := range view {
		if task.ID == "S000" || task.ID == "S001" {
			task.Status = domain.DONE
		}
	}
	selected := parallel.Select(view, 2)
	if ids := taskIDs(selected); !equalStrings(ids, []string{"S002", "S003"}) {
		t.Errorf("parallel selection = %v, want [S002 S003]", ids)
	}

	// 4. Review passes when there are no blocking findings.
	reviewLoop := review.NewLoop(&fakeReviewer{}, nil, &fakeTester{}, review.High, 2)
	if _, err := reviewLoop.Run(ctx, review.Request{Task: "S004"}); err != nil {
		t.Fatalf("review: %v", err)
	}

	// 5. CI fails once and is remediated, then passes (the intentional failure).
	ci := &fakeCI{sequences: [][]github.Check{
		{{Name: "ci", State: github.CheckFail}},
		{{Name: "ci", State: github.CheckFail}},
		{{Name: "ci", State: github.CheckPass}},
	}, logs: "compile error"}
	remediation, err := ciremediation.NewLoop(ci, &fakeRemediator{}, 3, nil).Run(ctx)
	if err != nil {
		t.Fatalf("remediation: %v", err)
	}
	if remediation.Outcome != ciremediation.Pass || remediation.Attempts != 2 {
		t.Errorf("remediation = %+v, want PASS/2", remediation)
	}

	// 6. Merge gate merges only when every condition holds.
	merged, err := mergegate.New(&fakeMergeGateMerger{}, "squash").Merge(ctx, mergegate.Input{
		PR:           github.PullRequest{Number: 1, Mergeable: "MERGEABLE"},
		Checks:       []github.Check{{Name: "ci", State: github.CheckPass}},
		TestsPassed:  true,
		ReviewPassed: true,
	})
	if err != nil {
		t.Fatalf("merge gate: %v", err)
	}
	if merged.Outcome != mergegate.Merged {
		t.Errorf("merge gate = %+v, want MERGED", merged)
	}

	// 7. Completion loop drives every task to DONE, recording a handoff each.
	manager := handoff.New(st, handoff.NoOpCompressor{}, 0)
	manager.Env = nil
	loop := completion.New(st, &fakeExecutor{store: st}, &fakeMerger{}, &fakeRefresher{}).
		WithHandoff(handoffAdapter{manager: manager})

	result, err := loop.Run(ctx)
	if err != nil {
		t.Fatalf("completion: %v", err)
	}
	if result.Outcome != completion.AllDone || result.Completed != 5 {
		t.Fatalf("completion = %+v, want ALL_DONE/5", result)
	}

	persisted, err := st.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, task := range persisted {
		if task.Status != domain.DONE {
			t.Errorf("task %s = %s, want DONE", task.ID, task.Status)
		}
	}

	records, err := st.Handoffs()
	if err != nil {
		t.Fatalf("handoffs: %v", err)
	}
	if len(records) != 5 {
		t.Errorf("persisted %d handoffs, want one per task (5)", len(records))
	}
}

// --- fakes (external boundaries only) ---

type fakeAgent struct {
	content string
	err     error
}

func (f *fakeAgent) Generate(context.Context, agent.Request) (agent.Response, error) {
	return agent.Response{Content: f.content}, f.err
}

type fakeExecutor struct {
	store *store.Store
}

func (e *fakeExecutor) Execute(_ context.Context, task *domain.Task) error {
	current, err := e.store.Get(task.ID)
	if err != nil {
		return err
	}
	for _, status := range happyPath {
		if err := current.Transition(status); err != nil {
			return err
		}
	}
	return e.store.Save(current)
}

type fakeMerger struct{}

func (fakeMerger) Merge(context.Context, *domain.Task) (bool, error) { return true, nil }

type fakeRefresher struct{}

func (fakeRefresher) Refresh(context.Context) error { return nil }

type handoffAdapter struct{ manager *handoff.Manager }

func (a handoffAdapter) Complete(ctx context.Context, task *domain.Task) error {
	_, err := a.manager.Complete(ctx, task, handoff.Facts{Summary: task.Title}, nil)
	return err
}

type fakeReviewer struct{}

func (fakeReviewer) Review(context.Context, review.Request) (review.Report, error) {
	return review.Report{Summary: "looks good"}, nil
}

type fakeTester struct{}

func (fakeTester) Run(context.Context) testrunner.Result {
	return testrunner.Result{Status: testrunner.Pass}
}

type fakeCI struct {
	sequences [][]github.Check
	idx       int
	logs      string
}

func (f *fakeCI) Checks(context.Context) ([]github.Check, error) {
	if f.idx < len(f.sequences) {
		checks := f.sequences[f.idx]
		f.idx++
		return checks, nil
	}
	return f.sequences[len(f.sequences)-1], nil
}

func (f *fakeCI) FailureLogs(context.Context) (string, error) { return f.logs, nil }

type fakeRemediator struct{}

func (fakeRemediator) Attempt(context.Context, ciremediation.Failure) error { return nil }

type fakeMergeGateMerger struct{}

func (fakeMergeGateMerger) Merge(context.Context, int, string) error { return nil }

// --- helpers ---

func readFixture(t *testing.T, name string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("testdata", "dogfood", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return string(content)
}

func cloneTasks(tasks []*domain.Task) []*domain.Task {
	clones := make([]*domain.Task, 0, len(tasks))
	for _, task := range tasks {
		clone := *task
		clone.DependencyIDs = append([]string{}, task.DependencyIDs...)
		clones = append(clones, &clone)
	}
	return clones
}

func taskIDs(tasks []*domain.Task) []string {
	ids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	return ids
}

func dependsOn(task *domain.Task, id string) bool {
	for _, dep := range task.DependencyIDs {
		if dep == id {
			return true
		}
	}
	return false
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// hasDependencyChain reports whether the plan has a chain of at least three
// stages (a -> b -> c).
func hasDependencyChain(plan *planner.Plan) bool {
	deps := make(map[string][]string, len(plan.Stages))
	for _, stage := range plan.Stages {
		deps[stage.ID] = stage.Dependencies
	}
	for _, stage := range plan.Stages {
		for _, mid := range stage.Dependencies {
			if len(deps[mid]) > 0 {
				return true
			}
		}
	}
	return false
}

func planHasDeliverable(plan *planner.Plan, want string) bool {
	for _, stage := range plan.Stages {
		for _, deliverable := range stage.Deliverables {
			if deliverable == want {
				return true
			}
		}
	}
	return false
}
