package completion

import (
	"context"
	"errors"
	"testing"

	"github.com/imhttran/agentic-sdlc/internal/domain"
)

// happyPath is the sequential status chain the fake executor walks from READY to
// CI_PASS.
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

type fakeStore struct {
	order []string
	tasks map[string]*domain.Task
}

func newStore(tasks ...*domain.Task) *fakeStore {
	s := &fakeStore{tasks: make(map[string]*domain.Task)}
	for _, t := range tasks {
		cp := *t
		s.tasks[t.ID] = &cp
		s.order = append(s.order, t.ID)
	}
	return s
}

func (s *fakeStore) List() ([]*domain.Task, error) {
	out := make([]*domain.Task, 0, len(s.order))
	for _, id := range s.order {
		cp := *s.tasks[id]
		out = append(out, &cp)
	}
	return out, nil
}

func (s *fakeStore) Get(id string) (*domain.Task, error) {
	t, ok := s.tasks[id]
	if !ok {
		return nil, errors.New("not found")
	}
	cp := *t
	return &cp, nil
}

func (s *fakeStore) Save(task *domain.Task) error {
	cp := *task
	s.tasks[task.ID] = &cp
	return nil
}

func (s *fakeStore) status(id string) domain.TaskStatus {
	return s.tasks[id].Status
}

type fakeExecutor struct {
	store *fakeStore
	block bool
	err   error
	calls []string
}

func (e *fakeExecutor) Execute(_ context.Context, task *domain.Task) error {
	e.calls = append(e.calls, task.ID)
	if e.err != nil {
		return e.err
	}
	current, err := e.store.Get(task.ID)
	if err != nil {
		return err
	}
	if e.block {
		if err := current.Block(domain.IMPLEMENTATION_TIMEOUT); err != nil {
			return err
		}
		return e.store.Save(current)
	}
	for _, status := range happyPath {
		if err := current.Transition(status); err != nil {
			return err
		}
	}
	return e.store.Save(current)
}

type fakeMerger struct {
	merged bool
	err    error
	calls  []string
}

func (m *fakeMerger) Merge(_ context.Context, task *domain.Task) (bool, error) {
	m.calls = append(m.calls, task.ID)
	if m.err != nil {
		return false, m.err
	}
	return m.merged, nil
}

type fakeRefresher struct {
	err   error
	calls int
}

func (r *fakeRefresher) Refresh(context.Context) error {
	r.calls++
	return r.err
}

func task(id string, deps ...string) *domain.Task {
	return &domain.Task{
		ID:            id,
		Title:         id,
		Status:        domain.PLANNED,
		MaxAttempts:   1,
		DependencyIDs: deps,
	}
}

func TestEmptyPlanIsAllDone(t *testing.T) {
	loop := New(newStore(), &fakeExecutor{}, &fakeMerger{}, &fakeRefresher{})
	result, err := loop.Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result.Outcome != AllDone || result.Completed != 0 {
		t.Errorf("result = %+v, want ALL_DONE/0", result)
	}
}

func TestSingleTaskCompletes(t *testing.T) {
	store := newStore(task("T1"))
	merger := &fakeMerger{merged: true}
	refresh := &fakeRefresher{}
	exec := &fakeExecutor{store: store}

	result, err := New(store, exec, merger, refresh).Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result.Outcome != AllDone || result.Completed != 1 {
		t.Errorf("result = %+v, want ALL_DONE/1", result)
	}
	if got := store.status("T1"); got != domain.DONE {
		t.Errorf("task status = %s, want DONE", got)
	}
	if len(merger.calls) != 1 || merger.calls[0] != "T1" {
		t.Errorf("merger calls = %v, want [T1]", merger.calls)
	}
	if refresh.calls != 1 {
		t.Errorf("refresh calls = %d, want 1", refresh.calls)
	}
}

// TestMultiTaskDependencyDAG is the "small multi-task demo": A has no deps; B and
// C depend on A; D depends on B and C. V1 is sequential, so it must complete in
// dependency order and end ALL_DONE.
func TestMultiTaskDependencyDAG(t *testing.T) {
	store := newStore(
		task("D", "B", "C"),
		task("A"),
		task("C", "A"),
		task("B", "A"),
	)
	exec := &fakeExecutor{store: store}
	merger := &fakeMerger{merged: true}
	refresh := &fakeRefresher{}

	result, err := New(store, exec, merger, refresh).Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result.Outcome != AllDone || result.Completed != 4 {
		t.Errorf("result = %+v, want ALL_DONE/4", result)
	}
	for _, id := range []string{"A", "B", "C", "D"} {
		if got := store.status(id); got != domain.DONE {
			t.Errorf("task %s status = %s, want DONE", id, got)
		}
	}
	wantOrder := []string{"A", "B", "C", "D"}
	for i, id := range wantOrder {
		if exec.calls[i] != id {
			t.Errorf("execution order = %v, want %v", exec.calls, wantOrder)
			break
		}
	}
	if refresh.calls != 4 {
		t.Errorf("refresh calls = %d, want 4", refresh.calls)
	}
}

func TestExecutorBlockedTaskYieldsBlocked(t *testing.T) {
	store := newStore(task("T1"))
	merger := &fakeMerger{merged: true}
	refresh := &fakeRefresher{}
	exec := &fakeExecutor{store: store, block: true}

	result, err := New(store, exec, merger, refresh).Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result.Outcome != Blocked || result.Completed != 0 {
		t.Errorf("result = %+v, want BLOCKED/0", result)
	}
	if got := store.status("T1"); got != domain.BLOCKED {
		t.Errorf("task status = %s, want BLOCKED", got)
	}
	if len(merger.calls) != 0 || refresh.calls != 0 {
		t.Errorf("blocked task must not merge or refresh (merge=%v refresh=%d)", merger.calls, refresh.calls)
	}
}

func TestBlockedMergeYieldsBlocked(t *testing.T) {
	store := newStore(task("T1"))
	merger := &fakeMerger{merged: false}
	refresh := &fakeRefresher{}
	exec := &fakeExecutor{store: store}

	result, err := New(store, exec, merger, refresh).Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result.Outcome != Blocked {
		t.Errorf("outcome = %s, want BLOCKED", result.Outcome)
	}
	if got := store.status("T1"); got != domain.CI_PASS {
		t.Errorf("task status = %s, want CI_PASS (not DONE)", got)
	}
	if refresh.calls != 0 {
		t.Errorf("refresh calls = %d, want 0", refresh.calls)
	}
}

func TestRefreshErrorLeavesTaskMerged(t *testing.T) {
	store := newStore(task("T1"))
	merger := &fakeMerger{merged: true}
	refresh := &fakeRefresher{err: errors.New("fetch failed")}
	exec := &fakeExecutor{store: store}

	loop := New(store, exec, merger, refresh)
	_, err := loop.Advance(context.Background())
	if err == nil {
		t.Fatal("expected refresh error to propagate")
	}
	if got := store.status("T1"); got != domain.MERGED {
		t.Errorf("task status = %s, want MERGED", got)
	}
}

func TestExecutorErrorPropagates(t *testing.T) {
	store := newStore(task("T1"))
	exec := &fakeExecutor{store: store, err: errors.New("boom")}
	if _, err := New(store, exec, &fakeMerger{merged: true}, &fakeRefresher{}).Run(context.Background()); err == nil {
		t.Error("expected executor error to propagate")
	}
}

func TestMergerErrorPropagates(t *testing.T) {
	store := newStore(task("T1"))
	merger := &fakeMerger{err: errors.New("gh down")}
	exec := &fakeExecutor{store: store}
	if _, err := New(store, exec, merger, &fakeRefresher{}).Run(context.Background()); err == nil {
		t.Error("expected merger error to propagate")
	}
}

func TestCancellationStopsLoop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	store := newStore(task("T1"))
	loop := New(store, &fakeExecutor{store: store}, &fakeMerger{merged: true}, &fakeRefresher{})
	if _, err := loop.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

type fakeHandoff struct {
	err   error
	calls []string
}

func (h *fakeHandoff) Complete(_ context.Context, task *domain.Task) error {
	h.calls = append(h.calls, task.ID)
	return h.err
}

func TestHandoffRunsAfterDone(t *testing.T) {
	store := newStore(task("T1"))
	h := &fakeHandoff{}
	loop := New(store, &fakeExecutor{store: store}, &fakeMerger{merged: true}, &fakeRefresher{}).WithHandoff(h)

	if _, err := loop.Run(context.Background()); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if len(h.calls) != 1 || h.calls[0] != "T1" {
		t.Errorf("handoff calls = %v, want [T1]", h.calls)
	}
}

func TestHandoffFailureDoesNotBlock(t *testing.T) {
	store := newStore(task("T1"))
	h := &fakeHandoff{err: errors.New("compression failed")}
	loop := New(store, &fakeExecutor{store: store}, &fakeMerger{merged: true}, &fakeRefresher{}).WithHandoff(h)

	result, err := loop.Run(context.Background())
	if err != nil {
		t.Fatalf("handoff failure must not surface as an error, got: %v", err)
	}
	if result.Outcome != AllDone || result.Completed != 1 {
		t.Errorf("result = %+v, want ALL_DONE/1", result)
	}
	if got := store.status("T1"); got != domain.DONE {
		t.Errorf("task status = %s, want DONE", got)
	}
}
