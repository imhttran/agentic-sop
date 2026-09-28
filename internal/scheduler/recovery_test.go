package scheduler

import (
	"context"
	"errors"
	"testing"

	"github.com/imhttran/agentic-sop/internal/domain"
)

// blockedTask builds a recoverable BLOCKED task (retry budget left).
func blockedTask(id string, deps ...string) *domain.Task {
	return &domain.Task{ID: id, Status: domain.BLOCKED, Attempt: 1, MaxAttempts: 3, DependencyIDs: deps}
}

// TestNextRunnableWinsOverRecoverableBlocked asserts normal runnable work is
// selected before recovery work whenever runnable work exists.
func TestNextRunnableWinsOverRecoverableBlocked(t *testing.T) {
	store := &fakeStore{tasks: []*domain.Task{
		blockedTask("T001"),
		task("T002", domain.PLANNED),
	}}

	res := next(t, store)
	if res.Outcome != ReadyTask || res.Task == nil || res.Task.ID != "T002" {
		t.Fatalf("outcome = %s task = %+v, want READY_TASK T002", res.Outcome, res.Task)
	}
	if find(t, store.tasks, "T001").Status != domain.BLOCKED {
		t.Errorf("T001 status = %s, want BLOCKED (no speculative requeue)", find(t, store.tasks, "T001").Status)
	}
}

// TestNextSelectsEarliestRecoverableBlocked asserts that with no runnable work
// the earliest recoverable blocked task is requeued and returned.
func TestNextSelectsEarliestRecoverableBlocked(t *testing.T) {
	store := &fakeStore{tasks: []*domain.Task{
		blockedTask("T002"),
		blockedTask("T001"),
	}}

	res := next(t, store)
	if res.Outcome != RecoveredTask {
		t.Fatalf("outcome = %s, want %s", res.Outcome, RecoveredTask)
	}
	if res.Task == nil || res.Task.ID != "T001" {
		t.Fatalf("result task = %+v, want T001", res.Task)
	}
	if find(t, store.tasks, "T001").Status != domain.PLANNED {
		t.Errorf("T001 status = %s, want PLANNED", find(t, store.tasks, "T001").Status)
	}
	if find(t, store.tasks, "T002").Status != domain.BLOCKED {
		t.Errorf("T002 status = %s, want BLOCKED (not selected)", find(t, store.tasks, "T002").Status)
	}
	if store.saves != 1 {
		t.Errorf("saves = %d, want 1", store.saves)
	}
}

// TestNextDoesNotSelectBlockedWithUnsatisfiedDependency asserts a blocked task
// with unsatisfied dependencies is not selected.
func TestNextDoesNotSelectBlockedWithUnsatisfiedDependency(t *testing.T) {
	store := &fakeStore{tasks: []*domain.Task{
		task("T001", domain.PLANNED, "T002"),
		blockedTask("T002", "T001"),
	}}

	res := next(t, store)
	if res.Outcome == RecoveredTask {
		t.Fatalf("outcome = %s, must not recover a dependency-blocked task", res.Outcome)
	}
	if find(t, store.tasks, "T002").Status != domain.BLOCKED {
		t.Errorf("T002 status = %s, want BLOCKED", find(t, store.tasks, "T002").Status)
	}
}

// TestNextDoesNotSelectTerminallyBlocked asserts a budget-exhausted blocked task
// is not selected and the bare Blocked outcome is preserved.
func TestNextDoesNotSelectTerminallyBlocked(t *testing.T) {
	exhausted := &domain.Task{ID: "T001", Status: domain.BLOCKED, Attempt: 3, MaxAttempts: 3}
	store := &fakeStore{tasks: []*domain.Task{exhausted}}

	res := next(t, store)
	if res.Outcome != Blocked {
		t.Fatalf("outcome = %s, want %s", res.Outcome, Blocked)
	}
	if exhausted.Status != domain.BLOCKED {
		t.Errorf("status = %s, want BLOCKED", exhausted.Status)
	}
	if store.saves != 0 {
		t.Errorf("saves = %d, want 0", store.saves)
	}
}

// TestNextSkipsIneligibleEarlierBlocked asserts an ineligible earlier blocked task
// does not block selection of the next earliest eligible blocked task, and that
// eligible-but-not-selected tasks stay BLOCKED.
func TestNextSkipsIneligibleEarlierBlocked(t *testing.T) {
	exhausted := &domain.Task{ID: "T001", Status: domain.BLOCKED, Attempt: 3, MaxAttempts: 3}
	store := &fakeStore{tasks: []*domain.Task{
		exhausted,
		blockedTask("T002"),
		blockedTask("T003"),
	}}

	res := next(t, store)
	if res.Outcome != RecoveredTask || res.Task == nil || res.Task.ID != "T002" {
		t.Fatalf("outcome = %s task = %+v, want RECOVERED_TASK T002", res.Outcome, res.Task)
	}
	if exhausted.Status != domain.BLOCKED {
		t.Errorf("T001 status = %s, want BLOCKED", exhausted.Status)
	}
	if find(t, store.tasks, "T003").Status != domain.BLOCKED {
		t.Errorf("T003 status = %s, want BLOCKED (no speculative requeue)", find(t, store.tasks, "T003").Status)
	}
}

// TestNextRecoverySelectionDeterministic asserts recovery selection is
// deterministic across equivalent runs and permuted storage order.
func TestNextRecoverySelectionDeterministic(t *testing.T) {
	orders := [][]*domain.Task{
		{blockedTask("T001"), blockedTask("T002"), blockedTask("T003")},
		{blockedTask("T003"), blockedTask("T001"), blockedTask("T002")},
		{blockedTask("T002"), blockedTask("T003"), blockedTask("T001")},
	}

	for i, tasks := range orders {
		store := &fakeStore{tasks: tasks}
		res := next(t, store)
		if res.Outcome != RecoveredTask || res.Task == nil || res.Task.ID != "T001" {
			t.Errorf("run %d: outcome = %s task = %+v, want RECOVERED_TASK T001", i, res.Outcome, res.Task)
		}
	}
}

// TestNextRecoveryPersistenceFailureDoesNotReportRecovered asserts a failed save
// leaves in-memory state unchanged.
func TestNextRecoveryPersistenceFailureDoesNotReportRecovered(t *testing.T) {
	store := &fakeStore{
		tasks:   []*domain.Task{blockedTask("T001")},
		saveErr: errors.New("disk on fire"),
	}

	res, err := New(store).Next(context.Background())
	if err == nil {
		t.Fatal("expected error when persistence fails")
	}
	if res.Outcome == RecoveredTask {
		t.Errorf("outcome = %s, must not report RECOVERED_TASK on persistence failure", res.Outcome)
	}
	if find(t, store.tasks, "T001").Status != domain.BLOCKED {
		t.Errorf("T001 status = %s, want BLOCKED (original must be unchanged)", find(t, store.tasks, "T001").Status)
	}
}

// TestNextRecoveryWaitingOnDependenciesPreserved asserts the bare Blocked outcome
// only appears when blocked tasks exist but none is recoverable; dependency-only
// work still reports WaitingOnDependencies.
func TestNextRecoveryWaitingOnDependenciesPreserved(t *testing.T) {
	store := &fakeStore{tasks: []*domain.Task{
		task("T001", domain.IMPLEMENTING),
		task("T002", domain.PLANNED, "T001"),
	}}

	// An active task occupies the slot, so this is ActiveTask rather than waiting.
	if res := next(t, store); res.Outcome != ActiveTask {
		t.Fatalf("outcome = %s, want %s", res.Outcome, ActiveTask)
	}

	store = &fakeStore{tasks: []*domain.Task{
		task("T001", domain.PLANNED, "T002"),
		task("T002", domain.PLANNED, "T001"),
	}}
	if res := next(t, store); res.Outcome != WaitingOnDependencies {
		t.Fatalf("outcome = %s, want %s", res.Outcome, WaitingOnDependencies)
	}
}

// TestNextRecoveryAllDonePreserved asserts a fully satisfied plan reports AllDone
// rather than recovery.
func TestNextRecoveryAllDonePreserved(t *testing.T) {
	store := &fakeStore{tasks: []*domain.Task{
		task("T001", domain.DONE),
		task("T002", domain.MERGED),
	}}

	if res := next(t, store); res.Outcome != AllDone {
		t.Fatalf("outcome = %s, want %s", res.Outcome, AllDone)
	}
}
