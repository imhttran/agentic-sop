package domain

import (
	"errors"
	"testing"
)

// allStatuses is every TaskStatus the domain declares. Selection-semantics tests
// assert a disposition for each of them, so adding a status without defining its
// selection behavior fails here.
func allStatuses() []TaskStatus {
	return []TaskStatus{
		PLANNED, READY, BRANCH_CREATED, TESTS_WRITTEN, RED_VERIFIED, IMPLEMENTING,
		LOCAL_TESTS_PASS, REVIEW, REVIEW_PASS, PR_OPEN, CI_RUNNING, CI_PASS,
		FIX_REQUIRED, MERGED, DONE, LOCAL_DONE, BLOCKED,
	}
}

// TestIsSatisfiedOnlyForCompletedWork pins the single definition of satisfied
// work to exactly the completed states.
func TestIsSatisfiedOnlyForCompletedWork(t *testing.T) {
	completed := map[TaskStatus]bool{MERGED: true, DONE: true, LOCAL_DONE: true}
	for _, status := range allStatuses() {
		task := &Task{ID: "T1", Status: status}
		want := completed[status]
		if got := task.IsSatisfied(); got != want {
			t.Errorf("IsSatisfied(%s) = %v, want %v", status, got, want)
		}
	}
}

// TestIsRunnableOnlyPlanned pins selection to PLANNED: completed work is never
// runnable, in-flight work is never runnable, and BLOCKED is never runnable.
func TestIsRunnableOnlyPlanned(t *testing.T) {
	for _, status := range allStatuses() {
		task := &Task{ID: "T1", Status: status}
		want := status == PLANNED
		if got := task.IsRunnable(); got != want {
			t.Errorf("IsRunnable(%s) = %v, want %v", status, got, want)
		}
	}
}

// TestBlockedRecoverableSemantics asserts BLOCKED is not permanently unrunnable
// while retry budget remains, and is terminally exhausted once it is spent.
func TestBlockedRecoverableSemantics(t *testing.T) {
	cases := []struct {
		name        string
		status      TaskStatus
		attempt     int
		maxAttempts int
		wantRecover bool
	}{
		{"blocked with budget", BLOCKED, 1, 3, true},
		{"blocked no budget cap", BLOCKED, 5, 0, true},
		{"blocked exhausted", BLOCKED, 3, 3, false},
		{"blocked over budget", BLOCKED, 4, 3, false},
		{"planned is not blocked", PLANNED, 1, 3, false},
		{"completed is not blocked", DONE, 0, 3, false},
		{"in-flight is not blocked", IMPLEMENTING, 1, 3, false},
	}
	for _, c := range cases {
		task := &Task{ID: "T1", Status: c.status, Attempt: c.attempt, MaxAttempts: c.maxAttempts}
		if got := task.IsBlockedRecoverable(); got != c.wantRecover {
			t.Errorf("%s: IsBlockedRecoverable = %v, want %v", c.name, got, c.wantRecover)
		}
		wantTerminal := c.status == BLOCKED && !c.wantRecover
		if got := task.IsTerminalBlocked(); got != wantTerminal {
			t.Errorf("%s: IsTerminalBlocked = %v, want %v", c.name, got, wantTerminal)
		}
	}
}

// TestBlockedRecoverableMatchesRequeue asserts the recoverability predicate agrees
// with the actual requeue path: recoverable BLOCKED yields PLANNED, exhausted
// BLOCKED returns ErrRetryExhausted.
func TestBlockedRecoverableMatchesRequeue(t *testing.T) {
	recoverable := &Task{ID: "T1", Status: BLOCKED, Attempt: 1, MaxAttempts: 3, BlockedReason: REVIEW_UNRESOLVED}
	if !recoverable.IsBlockedRecoverable() {
		t.Fatal("expected a recoverable BLOCKED task")
	}
	if err := recoverable.Requeue(); err != nil {
		t.Fatalf("Requeue of a recoverable task: %v", err)
	}
	if recoverable.Status != PLANNED {
		t.Errorf("status = %s, want PLANNED", recoverable.Status)
	}

	exhausted := &Task{ID: "T2", Status: BLOCKED, Attempt: 3, MaxAttempts: 3}
	if exhausted.IsBlockedRecoverable() {
		t.Fatal("a budget-exhausted BLOCKED task must not be recoverable")
	}
	if err := exhausted.Requeue(); !errors.Is(err, ErrRetryExhausted) {
		t.Fatalf("Requeue = %v, want ErrRetryExhausted", err)
	}
}

// TestResolveDependenciesNeverTreatsBlockedOrInflightAsSatisfied asserts
// dependency ordering is authoritative: only satisfied work satisfies a
// dependency.
func TestResolveDependenciesNeverTreatsBlockedOrInflightAsSatisfied(t *testing.T) {
	notSatisfying := []TaskStatus{
		BRANCH_CREATED, TESTS_WRITTEN, RED_VERIFIED, IMPLEMENTING,
		LOCAL_TESTS_PASS, REVIEW, REVIEW_PASS, PR_OPEN, CI_RUNNING, CI_PASS,
		FIX_REQUIRED, BLOCKED, PLANNED, READY,
	}
	for _, status := range notSatisfying {
		task := &Task{ID: "T2", DependencyIDs: []string{"T1"}}
		tasks := map[string]*Task{"T1": {ID: "T1", Status: status}}
		unmet, satisfied := task.ResolveDependencies(tasks)
		if satisfied {
			t.Errorf("dependency in %s must not satisfy", status)
		}
		if len(unmet) != 1 || unmet[0].TaskID != "T1" {
			t.Errorf("dependency in %s: unmet = %+v, want one entry for T1", status, unmet)
		}
	}

	for _, status := range []TaskStatus{MERGED, DONE, LOCAL_DONE} {
		task := &Task{ID: "T2", DependencyIDs: []string{"T1"}}
		tasks := map[string]*Task{"T1": {ID: "T1", Status: status}}
		if _, satisfied := task.ResolveDependencies(tasks); !satisfied {
			t.Errorf("dependency in %s must satisfy", status)
		}
	}
}

// TestRequeueCompletedTaskFails asserts completed work can never be returned to
// PLANNED.
func TestRequeueCompletedTaskFails(t *testing.T) {
	for _, status := range []TaskStatus{DONE, LOCAL_DONE, MERGED} {
		task := &Task{ID: "T1", Status: status}
		if err := task.Requeue(); err == nil {
			t.Errorf("Requeue of %s must fail", status)
		}
		if task.Status != status {
			t.Errorf("Requeue of %s mutated status to %s", status, task.Status)
		}
	}
}
