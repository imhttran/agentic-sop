package parallel

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/domain"
)

func task(id string, status domain.TaskStatus, deps ...string) *domain.Task {
	return &domain.Task{ID: id, Title: id, Status: status, MaxAttempts: 1, DependencyIDs: deps}
}

type fakeStore struct {
	tasks []*domain.Task
	err   error
}

func (s *fakeStore) List() ([]*domain.Task, error) { return s.tasks, s.err }

type fakeWorkspaces struct {
	mu        sync.Mutex
	created   []string
	removed   []string
	createErr error
}

func (w *fakeWorkspaces) Create(_ context.Context, task *domain.Task) (string, error) {
	if w.createErr != nil {
		return "", w.createErr
	}
	dir := filepath.Join("/wt", task.ID)
	w.mu.Lock()
	w.created = append(w.created, dir)
	w.mu.Unlock()
	return dir, nil
}

func (w *fakeWorkspaces) Remove(_ context.Context, task *domain.Task) error {
	w.mu.Lock()
	w.removed = append(w.removed, filepath.Join("/wt", task.ID))
	w.mu.Unlock()
	return nil
}

type fakeExecutor struct {
	mu     sync.Mutex
	active int
	peak   int
	dirs   map[string]string
	calls  []string
	errs   map[string]error
	hold   time.Duration
}

func newFakeExecutor(errs map[string]error) *fakeExecutor {
	return &fakeExecutor{dirs: map[string]string{}, errs: errs, hold: 10 * time.Millisecond}
}

func (e *fakeExecutor) Execute(ctx context.Context, task *domain.Task, dir string) error {
	e.mu.Lock()
	e.active++
	if e.active > e.peak {
		e.peak = e.active
	}
	e.calls = append(e.calls, task.ID)
	e.dirs[task.ID] = dir
	e.mu.Unlock()

	time.Sleep(e.hold)

	e.mu.Lock()
	e.active--
	e.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}
	return e.errs[task.ID]
}

func TestSelectDeterministicAndBounded(t *testing.T) {
	tasks := []*domain.Task{
		task("C", domain.PLANNED),
		task("A", domain.PLANNED),
		task("B", domain.READY),
		task("D", domain.DONE),
	}
	got := Select(tasks, 2)
	if !reflect.DeepEqual(taskIDs(got), []string{"A", "B"}) {
		t.Errorf("Select = %v, want [A B]", taskIDs(got))
	}

	// max <= 0 selects nothing.
	if len(Select(tasks, 0)) != 0 {
		t.Error("Select(_, 0) should return nothing")
	}
}

func TestSelectExcludesUnmetDependencies(t *testing.T) {
	tasks := []*domain.Task{
		task("A", domain.PLANNED),
		task("B", domain.PLANNED, "A"), // A not done yet
		task("C", domain.PLANNED, "D"), // D done, so C is runnable
		task("D", domain.DONE),
	}
	got := taskIDs(Select(tasks, 10))
	want := []string{"A", "C"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Select = %v, want %v", got, want)
	}
}

func TestRunIndependentTasksConcurrently(t *testing.T) {
	store := &fakeStore{tasks: []*domain.Task{
		task("A", domain.PLANNED),
		task("B", domain.PLANNED),
		task("C", domain.PLANNED),
	}}
	workspaces := &fakeWorkspaces{}
	executor := newFakeExecutor(nil)

	result, err := New(store, executor, workspaces, 2).Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result.Failed() {
		t.Errorf("unexpected failures: %v", result.Errors)
	}
	if executor.peak != 2 {
		t.Errorf("peak concurrency = %d, want 2", executor.peak)
	}
	if len(result.TaskIDs) != 2 {
		t.Errorf("selected = %v, want 2 tasks", result.TaskIDs)
	}

	// Distinct working directory per task.
	if executor.dirs["A"] == executor.dirs["B"] || executor.dirs["B"] == executor.dirs["C"] {
		t.Errorf("tasks shared a directory: %v", executor.dirs)
	}

	// Every selected worktree removed (A and B, ordered by id).
	sort.Strings(workspaces.removed)
	if !reflect.DeepEqual(workspaces.removed, []string{"/wt/A", "/wt/B"}) {
		t.Errorf("removed = %v, want the two selected worktrees", workspaces.removed)
	}
}

func TestTwoIndependentTasksDoNotShareDirectory(t *testing.T) {
	store := &fakeStore{tasks: []*domain.Task{task("A", domain.PLANNED), task("B", domain.PLANNED)}}
	workspaces := &fakeWorkspaces{}
	executor := newFakeExecutor(nil)

	if _, err := New(store, executor, workspaces, 2).Run(context.Background()); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if executor.dirs["A"] == executor.dirs["B"] {
		t.Errorf("both tasks used %q", executor.dirs["A"])
	}
}

func TestRunReportsPerTaskErrors(t *testing.T) {
	store := &fakeStore{tasks: []*domain.Task{task("A", domain.PLANNED), task("B", domain.PLANNED)}}
	workspaces := &fakeWorkspaces{}
	executor := newFakeExecutor(map[string]error{"A": errors.New("boom")})

	result, err := New(store, executor, workspaces, 2).Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if !result.Failed() || result.Errors["A"] == nil || result.Errors["B"] != nil {
		t.Errorf("errors = %v, want only A to fail", result.Errors)
	}
	// Worktrees are still cleaned up.
	if len(workspaces.removed) != 2 {
		t.Errorf("removed = %v, want both", workspaces.removed)
	}
}

func TestRunRemovesWorkspaceWhenCreateFails(t *testing.T) {
	store := &fakeStore{tasks: []*domain.Task{task("A", domain.PLANNED)}}
	workspaces := &fakeWorkspaces{createErr: errors.New("no space")}
	executor := newFakeExecutor(nil)

	result, _ := New(store, executor, workspaces, 2).Run(context.Background())
	if result.Errors["A"] == nil {
		t.Error("expected a create error for A")
	}
	if len(workspaces.removed) != 0 {
		t.Errorf("nothing to remove, got %v", workspaces.removed)
	}
}

func TestRunCleansUpOnCancellation(t *testing.T) {
	store := &fakeStore{tasks: []*domain.Task{task("A", domain.PLANNED)}}
	workspaces := &fakeWorkspaces{}
	executor := newFakeExecutor(nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := New(store, executor, workspaces, 2).Run(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if len(workspaces.removed) != 1 {
		t.Errorf("workspace not cleaned up on cancellation: %v", workspaces.removed)
	}
}

func TestRunNoWork(t *testing.T) {
	store := &fakeStore{tasks: []*domain.Task{task("A", domain.DONE)}}
	result, err := New(store, newFakeExecutor(nil), &fakeWorkspaces{}, 2).Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(result.TaskIDs) != 0 || result.Failed() {
		t.Errorf("result = %+v, want no work", result)
	}
}

type fakeGitWorktrees struct {
	added   []string
	removed []string
}

func (g *fakeGitWorktrees) AddWorktree(_ context.Context, path, branch string) error {
	g.added = append(g.added, path+"@"+branch)
	return nil
}

func (g *fakeGitWorktrees) RemoveWorktree(_ context.Context, path string) error {
	g.removed = append(g.removed, path)
	return nil
}

func TestGitWorkspacesUsesTaskBranch(t *testing.T) {
	repo := &fakeGitWorktrees{}
	workspaces := NewGitWorkspaces(repo, "/root")

	task := &domain.Task{ID: "T1", Title: "Add feature"}
	dir, err := workspaces.Create(context.Background(), task)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if dir != filepath.Join("/root", "T1") {
		t.Errorf("dir = %q, want /root/T1", dir)
	}
	if !reflect.DeepEqual(repo.added, []string{"/root/T1@task/T1-add-feature"}) {
		t.Errorf("added = %v, want task branch worktree", repo.added)
	}

	if err := workspaces.Remove(context.Background(), task); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
	if !reflect.DeepEqual(repo.removed, []string{"/root/T1"}) {
		t.Errorf("removed = %v, want [/root/T1]", repo.removed)
	}
}

func TestGitWorkspacesRejectsUnsafeTaskID(t *testing.T) {
	repo := &fakeGitWorktrees{}
	workspaces := NewGitWorkspaces(repo, "/root")

	for _, id := range []string{"../evil", "a/b", `a\\b`, "..", ""} {
		task := &domain.Task{ID: id, Title: "t"}
		if _, err := workspaces.Create(context.Background(), task); err == nil {
			t.Errorf("Create with id %q = nil error, want rejection", id)
		}
		if err := workspaces.Remove(context.Background(), task); err == nil {
			t.Errorf("Remove with id %q = nil error, want rejection", id)
		}
	}
	if len(repo.added) != 0 || len(repo.removed) != 0 {
		t.Errorf("unsafe ids must not touch the repo: added=%v removed=%v", repo.added, repo.removed)
	}
}
