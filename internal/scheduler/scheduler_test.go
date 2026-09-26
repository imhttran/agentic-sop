package scheduler

import (
	"context"
	"errors"
	"testing"

	"github.com/imhttran/agentic-sop/internal/domain"
)

// fakeStore is an in-memory TaskStore for deterministic tests.
type fakeStore struct {
	tasks   []*domain.Task
	saves   int
	saveErr error
}

func (f *fakeStore) List() ([]*domain.Task, error) { return f.tasks, nil }

func (f *fakeStore) Save(task *domain.Task) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.saves++
	return nil
}

func task(id string, status domain.TaskStatus, deps ...string) *domain.Task {
	return &domain.Task{ID: id, Status: status, DependencyIDs: deps}
}

func find(t *testing.T, tasks []*domain.Task, id string) *domain.Task {
	t.Helper()
	for _, tk := range tasks {
		if tk.ID == id {
			return tk
		}
	}
	t.Fatalf("task %s not found", id)
	return nil
}

func next(t *testing.T, store *fakeStore) Result {
	t.Helper()
	res, err := New(store).Next(context.Background())
	if err != nil {
		t.Fatalf("Next returned error: %v", err)
	}
	return res
}

func TestNextPromotesReadyTask(t *testing.T) {
	store := &fakeStore{tasks: []*domain.Task{task("T001", domain.PLANNED)}}

	res := next(t, store)
	if res.Outcome != ReadyTask {
		t.Fatalf("outcome = %s, want %s", res.Outcome, ReadyTask)
	}
	if res.Task == nil || res.Task.ID != "T001" {
		t.Fatalf("result task = %+v, want T001", res.Task)
	}
	if find(t, store.tasks, "T001").Status != domain.READY {
		t.Errorf("T001 status = %s, want READY", find(t, store.tasks, "T001").Status)
	}
	if store.saves != 1 {
		t.Errorf("saves = %d, want 1", store.saves)
	}
}

func TestNextLinearDependency(t *testing.T) {
	store := &fakeStore{tasks: []*domain.Task{
		task("T001", domain.DONE),
		task("T002", domain.PLANNED, "T001"),
	}}

	res := next(t, store)
	if res.Outcome != ReadyTask || res.Task.ID != "T002" {
		t.Fatalf("outcome = %s task = %+v, want READY_TASK T002", res.Outcome, res.Task)
	}
}

func TestNextDoesNotPromoteOnIncompleteDependency(t *testing.T) {
	// T001 is active, so V1 reports ACTIVE_TASK and leaves T002 PLANNED.
	store := &fakeStore{tasks: []*domain.Task{
		task("T001", domain.IMPLEMENTING),
		task("T002", domain.PLANNED, "T001"),
	}}

	res := next(t, store)
	if res.Outcome != ActiveTask {
		t.Errorf("outcome = %s, want %s", res.Outcome, ActiveTask)
	}
	if find(t, store.tasks, "T002").Status != domain.PLANNED {
		t.Errorf("T002 status = %s, want PLANNED", find(t, store.tasks, "T002").Status)
	}
}

func TestNextDoesNotTreatLocalTestsPassAsIntegrated(t *testing.T) {
	store := &fakeStore{tasks: []*domain.Task{
		task("T001", domain.LOCAL_TESTS_PASS),
		task("T002", domain.PLANNED, "T001"),
	}}

	res := next(t, store)
	if res.Outcome == ReadyTask {
		t.Errorf("T002 promoted although dependency is only LOCAL_TESTS_PASS")
	}
	if find(t, store.tasks, "T002").Status != domain.PLANNED {
		t.Errorf("T002 status = %s, want PLANNED", find(t, store.tasks, "T002").Status)
	}
}

func TestNextDoesNotTreatCIPassAsIntegrated(t *testing.T) {
	store := &fakeStore{tasks: []*domain.Task{
		task("T001", domain.CI_PASS),
		task("T002", domain.PLANNED, "T001"),
	}}

	res := next(t, store)
	if res.Outcome == ReadyTask {
		t.Errorf("T002 promoted although dependency is only CI_PASS")
	}
	if find(t, store.tasks, "T002").Status != domain.PLANNED {
		t.Errorf("T002 status = %s, want PLANNED", find(t, store.tasks, "T002").Status)
	}
}

func TestNextMergedDependencySatisfies(t *testing.T) {
	store := &fakeStore{tasks: []*domain.Task{
		task("T001", domain.MERGED),
		task("T002", domain.PLANNED, "T001"),
	}}

	res := next(t, store)
	if res.Outcome != ReadyTask || res.Task.ID != "T002" {
		t.Fatalf("outcome = %s task = %+v, want READY_TASK T002", res.Outcome, res.Task)
	}
}

func TestNextDiamondDependency(t *testing.T) {
	store := &fakeStore{tasks: []*domain.Task{
		task("T001", domain.DONE),
		task("T002", domain.DONE, "T001"),
		task("T003", domain.DONE, "T001"),
		task("T004", domain.PLANNED, "T002", "T003"),
	}}

	res := next(t, store)
	if res.Outcome != ReadyTask || res.Task.ID != "T004" {
		t.Fatalf("outcome = %s task = %+v, want READY_TASK T004", res.Outcome, res.Task)
	}
}

func TestNextMultipleDependenciesOneIncomplete(t *testing.T) {
	store := &fakeStore{tasks: []*domain.Task{
		task("T001", domain.DONE),
		task("T002", domain.REVIEW),
		task("T003", domain.PLANNED, "T001", "T002"),
	}}

	res := next(t, store)
	if res.Outcome == ReadyTask {
		t.Errorf("T003 promoted although T002 is incomplete")
	}
	if find(t, store.tasks, "T003").Status != domain.PLANNED {
		t.Errorf("T003 status = %s, want PLANNED", find(t, store.tasks, "T003").Status)
	}
}

func TestNextDeterministicOrdering(t *testing.T) {
	store := &fakeStore{tasks: []*domain.Task{
		task("T003", domain.PLANNED),
		task("T001", domain.PLANNED),
		task("T002", domain.PLANNED),
	}}

	res := next(t, store)
	if res.Outcome != ReadyTask || res.Task.ID != "T001" {
		t.Fatalf("outcome = %s task = %+v, want READY_TASK T001", res.Outcome, res.Task)
	}
}

func TestNextExistingReadyTaskIsActive(t *testing.T) {
	store := &fakeStore{tasks: []*domain.Task{
		task("T001", domain.READY),
		task("T002", domain.PLANNED),
	}}

	res := next(t, store)
	if res.Outcome != ActiveTask {
		t.Errorf("outcome = %s, want %s", res.Outcome, ActiveTask)
	}
	if find(t, store.tasks, "T002").Status != domain.PLANNED {
		t.Errorf("T002 status = %s, want PLANNED", find(t, store.tasks, "T002").Status)
	}
}

func TestNextExistingActiveTask(t *testing.T) {
	store := &fakeStore{tasks: []*domain.Task{
		task("T001", domain.IMPLEMENTING),
		task("T002", domain.PLANNED),
	}}

	if res := next(t, store); res.Outcome != ActiveTask {
		t.Errorf("outcome = %s, want %s", res.Outcome, ActiveTask)
	}
}

func TestNextBlockedDependency(t *testing.T) {
	store := &fakeStore{tasks: []*domain.Task{
		task("T001", domain.BLOCKED),
		task("T002", domain.PLANNED, "T001"),
	}}

	res := next(t, store)
	if res.Outcome != Blocked {
		t.Errorf("outcome = %s, want %s", res.Outcome, Blocked)
	}
	if find(t, store.tasks, "T002").Status != domain.PLANNED {
		t.Errorf("T002 status = %s, want PLANNED (must not be mutated)", find(t, store.tasks, "T002").Status)
	}
}

func TestNextMissingDependencyIsError(t *testing.T) {
	store := &fakeStore{tasks: []*domain.Task{
		task("T002", domain.PLANNED, "T999"),
	}}

	_, err := New(store).Next(context.Background())
	if err == nil {
		t.Fatal("expected error for a missing dependency")
	}
	if find(t, store.tasks, "T002").Status != domain.PLANNED {
		t.Errorf("T002 status = %s, want PLANNED", find(t, store.tasks, "T002").Status)
	}
	if store.saves != 0 {
		t.Errorf("saves = %d, want 0", store.saves)
	}
}

func TestNextAllDone(t *testing.T) {
	store := &fakeStore{tasks: []*domain.Task{
		task("T001", domain.DONE),
		task("T002", domain.DONE),
	}}

	if res := next(t, store); res.Outcome != AllDone {
		t.Errorf("outcome = %s, want %s", res.Outcome, AllDone)
	}
}

func TestNextRepeatedCallIsIdempotent(t *testing.T) {
	store := &fakeStore{tasks: []*domain.Task{
		task("T001", domain.PLANNED),
		task("T002", domain.PLANNED, "T001"),
	}}
	s := New(store)

	first, err := s.Next(context.Background())
	if err != nil {
		t.Fatalf("first Next: %v", err)
	}
	if first.Outcome != ReadyTask || first.Task.ID != "T001" {
		t.Fatalf("first outcome = %s task = %+v, want READY_TASK T001", first.Outcome, first.Task)
	}

	second, err := s.Next(context.Background())
	if err != nil {
		t.Fatalf("second Next: %v", err)
	}
	if second.Outcome != ActiveTask {
		t.Errorf("second outcome = %s, want %s", second.Outcome, ActiveTask)
	}
	if find(t, store.tasks, "T002").Status != domain.PLANNED {
		t.Errorf("T002 status = %s, want PLANNED", find(t, store.tasks, "T002").Status)
	}
}

func TestNextPersistenceFailureDoesNotReportReady(t *testing.T) {
	store := &fakeStore{
		tasks:   []*domain.Task{task("T001", domain.PLANNED)},
		saveErr: errors.New("disk on fire"),
	}

	res, err := New(store).Next(context.Background())
	if err == nil {
		t.Fatal("expected error when persistence fails")
	}
	if res.Outcome == ReadyTask {
		t.Errorf("outcome = %s, must not report READY_TASK on persistence failure", res.Outcome)
	}
	if find(t, store.tasks, "T001").Status != domain.PLANNED {
		t.Errorf("T001 status = %s, want PLANNED (original must be unchanged)", find(t, store.tasks, "T001").Status)
	}
}
