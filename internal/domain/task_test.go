package domain

import (
	"errors"
	"testing"
	"time"
)

func TestLegalTransitions(t *testing.T) {
	cases := []struct {
		from TaskStatus
		to   TaskStatus
	}{
		// Happy path
		{PLANNED, READY},
		{READY, BRANCH_CREATED},
		{BRANCH_CREATED, TESTS_WRITTEN},
		{TESTS_WRITTEN, RED_VERIFIED},
		{RED_VERIFIED, IMPLEMENTING},
		{IMPLEMENTING, LOCAL_TESTS_PASS},
		{LOCAL_TESTS_PASS, REVIEW},
		{REVIEW, REVIEW_PASS},
		{REVIEW_PASS, PR_OPEN},
		{REVIEW_PASS, LOCAL_DONE},
		{PR_OPEN, CI_RUNNING},
		{CI_RUNNING, CI_PASS},
		{CI_PASS, MERGED},
		{MERGED, DONE},
		// Remediation
		{IMPLEMENTING, FIX_REQUIRED},
		{LOCAL_TESTS_PASS, FIX_REQUIRED},
		{REVIEW, FIX_REQUIRED},
		{CI_RUNNING, FIX_REQUIRED},
		{FIX_REQUIRED, IMPLEMENTING},
	}

	for _, c := range cases {
		task := &Task{ID: "T1", Status: c.from}
		if !task.CanTransitionTo(c.to) {
			t.Errorf("%s should transition to %s", c.from, c.to)
		}
	}
}

func TestIllegalTransitions(t *testing.T) {
	cases := []struct {
		from TaskStatus
		to   TaskStatus
	}{
		{PLANNED, DONE},
		{READY, REVIEW},
		{BRANCH_CREATED, CI_RUNNING},
		{TESTS_WRITTEN, DONE},
		{REVIEW, CI_PASS},
		{CI_RUNNING, MERGED},
		{DONE, READY},
		{LOCAL_DONE, READY},
		{BLOCKED, IMPLEMENTING},
		// Skipping states on the happy path
		{LOCAL_TESTS_PASS, REVIEW_PASS},
		{PR_OPEN, CI_PASS},
		{CI_PASS, DONE},
		// BLOCKED is entered only through Block(), never Transition()
		{READY, BLOCKED},
		{IMPLEMENTING, BLOCKED},
		{FIX_REQUIRED, BLOCKED},
		// Backwards / arbitrary
		{FIX_REQUIRED, CI_RUNNING},
		{RED_VERIFIED, REVIEW},
		{PLANNED, PLANNED},
	}

	for _, c := range cases {
		task := &Task{ID: "T1", Status: c.from}
		if task.CanTransitionTo(c.to) {
			t.Errorf("%s should not transition to %s", c.from, c.to)
		}
	}
}

func TestTerminalStatesRejectEveryTransition(t *testing.T) {
	all := []TaskStatus{
		PLANNED, READY, BRANCH_CREATED, TESTS_WRITTEN, RED_VERIFIED, IMPLEMENTING,
		LOCAL_TESTS_PASS, REVIEW, REVIEW_PASS, PR_OPEN, CI_RUNNING, CI_PASS,
		FIX_REQUIRED, MERGED, DONE, LOCAL_DONE, BLOCKED,
	}

	for _, terminal := range []TaskStatus{BLOCKED, DONE, LOCAL_DONE} {
		task := &Task{ID: "T1", Status: terminal}
		for _, next := range all {
			if task.CanTransitionTo(next) {
				t.Errorf("terminal %s should not transition to %s", terminal, next)
			}
		}
	}
}

func TestTransitionAppliesAndUpdatesUpdatedAt(t *testing.T) {
	task := &Task{ID: "T1", Status: READY, UpdatedAt: time.Now().Add(-time.Hour)}
	before := task.UpdatedAt

	if err := task.Transition(BRANCH_CREATED); err != nil {
		t.Fatalf("Transition failed: %v", err)
	}
	if task.Status != BRANCH_CREATED {
		t.Errorf("Status = %s, want %s", task.Status, BRANCH_CREATED)
	}
	if !task.UpdatedAt.After(before) {
		t.Errorf("UpdatedAt not advanced: %v -> %v", before, task.UpdatedAt)
	}
}

func TestIllegalTransitionIsAtomic(t *testing.T) {
	task := &Task{ID: "T1", Status: READY, UpdatedAt: time.Now()}
	before := task.UpdatedAt

	err := task.Transition(DONE)
	if err == nil {
		t.Fatal("expected error for illegal transition READY -> DONE")
	}
	if task.Status != READY {
		t.Errorf("Status mutated to %s, want READY", task.Status)
	}
	if !task.UpdatedAt.Equal(before) {
		t.Errorf("UpdatedAt mutated on failed transition: %v -> %v", before, task.UpdatedAt)
	}
}

func TestCanTransitionToIsSideEffectFree(t *testing.T) {
	task := &Task{ID: "T1", Status: READY, UpdatedAt: time.Now()}
	beforeStatus, beforeUpdated := task.Status, task.UpdatedAt

	task.CanTransitionTo(BRANCH_CREATED)
	task.CanTransitionTo(DONE)

	if task.Status != beforeStatus || !task.UpdatedAt.Equal(beforeUpdated) {
		t.Errorf("CanTransitionTo mutated the Task")
	}
}

func TestBlockRequiresReason(t *testing.T) {
	task := &Task{ID: "T1", Status: IMPLEMENTING, UpdatedAt: time.Now()}

	err := task.Block(NO_REASON)
	if err == nil {
		t.Fatal("expected error when blocking without a reason")
	}
	if task.Status != IMPLEMENTING {
		t.Errorf("Status mutated to %s, want IMPLEMENTING", task.Status)
	}
}

func TestBlockSetsTerminalState(t *testing.T) {
	task := &Task{ID: "T1", Status: IMPLEMENTING, UpdatedAt: time.Now().Add(-time.Hour)}
	before := task.UpdatedAt

	if err := task.Block(RETRIES_EXHAUSTED); err != nil {
		t.Fatalf("Block failed: %v", err)
	}
	if task.Status != BLOCKED {
		t.Errorf("Status = %s, want BLOCKED", task.Status)
	}
	if task.BlockedReason != RETRIES_EXHAUSTED {
		t.Errorf("BlockedReason = %s, want %s", task.BlockedReason, RETRIES_EXHAUSTED)
	}
	if !task.UpdatedAt.After(before) {
		t.Errorf("UpdatedAt not advanced: %v -> %v", before, task.UpdatedAt)
	}
	if task.CanTransitionTo(IMPLEMENTING) {
		t.Errorf("BLOCKED task should be terminal")
	}
}

func TestRequeue(t *testing.T) {
	for _, from := range []TaskStatus{PLANNED, READY, BRANCH_CREATED, IMPLEMENTING, BLOCKED} {
		task := &Task{ID: "T1", Status: from, BlockedReason: REVIEW_UNRESOLVED}
		if err := task.Requeue(); err != nil {
			t.Errorf("Requeue from %s: %v", from, err)
			continue
		}
		if task.Status != PLANNED {
			t.Errorf("Requeue from %s: status = %s, want PLANNED", from, task.Status)
		}
		if task.BlockedReason != NO_REASON {
			t.Errorf("Requeue from %s: blocked reason = %q, want empty", from, task.BlockedReason)
		}
	}

	for _, from := range []TaskStatus{DONE, LOCAL_DONE, MERGED} {
		task := &Task{ID: "T1", Status: from}
		if err := task.Requeue(); err == nil {
			t.Errorf("Requeue from %s should fail", from)
		}
		if task.Status != from {
			t.Errorf("Requeue from %s mutated status to %s", from, task.Status)
		}
	}
}

func TestRequeueWithoutSpending(t *testing.T) {
	task := &Task{ID: "T1", Status: BLOCKED, Attempt: 2, MaxAttempts: 3, BlockedReason: REVIEW_UNRESOLVED}
	if err := task.RequeueWithoutSpending(); err != nil {
		t.Fatalf("RequeueWithoutSpending: %v", err)
	}
	if task.Status != PLANNED {
		t.Errorf("status = %s, want PLANNED", task.Status)
	}
	if task.Attempt != 2 {
		t.Errorf("attempt = %d, want 2 (budget must not be spent)", task.Attempt)
	}
	if task.BlockedReason != NO_REASON {
		t.Errorf("blocked reason = %q, want empty", task.BlockedReason)
	}

	done := &Task{ID: "T2", Status: DONE}
	if err := done.RequeueWithoutSpending(); err == nil {
		t.Error("expected refusal for a completed task")
	}
}

func TestRequeueBudgetExhausted(t *testing.T) {
	task := &Task{ID: "T1", Status: BLOCKED, Attempt: 3, MaxAttempts: 3}
	if err := task.Requeue(); !errors.Is(err, ErrRetryExhausted) {
		t.Fatalf("Requeue = %v, want ErrRetryExhausted", err)
	}
	if task.Status != BLOCKED || task.Attempt != 3 {
		t.Errorf("task mutated: status=%s attempt=%d", task.Status, task.Attempt)
	}
}

func TestBlockRejectsCompletedTask(t *testing.T) {
	task := &Task{ID: "T1", Status: DONE}
	if err := task.Block(RETRIES_EXHAUSTED); err == nil {
		t.Fatal("expected error when blocking a completed task")
	}
	if task.Status != DONE {
		t.Errorf("Status mutated to %s, want DONE", task.Status)
	}
}

func TestDependency_all_met(t *testing.T) {
	task := &Task{ID: "T2", DependencyIDs: []string{"T1"}}
	depTask := &Task{ID: "T1", Status: DONE}
	tasks := map[string]*Task{"T1": depTask}

	unmet, satisfied := task.ResolveDependencies(tasks)
	if len(unmet) != 0 || !satisfied {
		t.Errorf("All dependencies satisfied, expected unmet=0, satisfied=true; got %d, %v", len(unmet), satisfied)
	}
}

func TestDependency_one_missing(t *testing.T) {
	task := &Task{ID: "T2", DependencyIDs: []string{"T1"}}
	depTask := &Task{ID: "T1", Status: READY}
	tasks := map[string]*Task{"T1": depTask}

	unmet, satisfied := task.ResolveDependencies(tasks)
	if len(unmet) != 1 || satisfied {
		t.Errorf("One dependency unsatisfied, expected unmet=1, satisfied=false; got %d, %v", len(unmet), satisfied)
	}
}

func TestDependency_partial_completion(t *testing.T) {
	task := &Task{ID: "T3", DependencyIDs: []string{"T1", "T2"}}
	tasks := map[string]*Task{
		"T1": {ID: "T1", Status: MERGED},
		"T2": {ID: "T2", Status: READY},
	}

	unmet, satisfied := task.ResolveDependencies(tasks)
	if len(unmet) != 1 || satisfied {
		t.Errorf("One dependency unsatisfied, expected unmet=1, satisfied=false; got %d, %v", len(unmet), satisfied)
	}
}

func TestDependencyRequiresIntegratedWork(t *testing.T) {
	cases := map[TaskStatus]bool{
		DONE:             true,
		MERGED:           true,
		LOCAL_DONE:       true,
		CI_PASS:          false,
		PR_OPEN:          false,
		REVIEW_PASS:      false,
		LOCAL_TESTS_PASS: false,
		IMPLEMENTING:     false,
		READY:            false,
		PLANNED:          false,
	}

	for status, wantSatisfied := range cases {
		task := &Task{ID: "T2", DependencyIDs: []string{"T1"}}
		tasks := map[string]*Task{"T1": {ID: "T1", Status: status}}

		_, satisfied := task.ResolveDependencies(tasks)
		if satisfied != wantSatisfied {
			t.Errorf("dependency in %s: satisfied=%v, want %v", status, satisfied, wantSatisfied)
		}
	}
}

func TestDependency_empty(t *testing.T) {
	task := &Task{ID: "T1", DependencyIDs: []string{}}
	unmet, satisfied := task.ResolveDependencies(map[string]*Task{})
	if len(unmet) != 0 || !satisfied {
		t.Errorf("No dependencies, expected unmet=0, satisfied=true; got %d, %v", len(unmet), satisfied)
	}
}

func TestRetry_attempt_tracking(t *testing.T) {
	task := &Task{ID: "T1"}
	task.AddAttempt(IMPLEMENTING, "first attempt")
	task.AddAttempt(FIX_REQUIRED, "test failure")

	if len(task.Attempts) != 2 {
		t.Errorf("Expected 2 attempts, got %d", len(task.Attempts))
	}
	if task.Attempts[0].Status != IMPLEMENTING || task.Attempts[1].Status != FIX_REQUIRED {
		t.Errorf("Attempts not tracked correctly")
	}
}

func TestTaskHelpers_IsReady(t *testing.T) {
	task := &Task{ID: "T1", Status: READY}
	if !task.IsReady() {
		t.Errorf("Task with READY status should return true for IsReady()")
	}
}

func TestTaskHelpers_IsBlocked(t *testing.T) {
	task := &Task{ID: "T1", Status: BLOCKED}
	if !task.IsBlocked() {
		t.Errorf("Task with BLOCKED status should return true for IsBlocked()")
	}
}

func TestTaskHelpers_IsDone(t *testing.T) {
	task := &Task{ID: "T1", Status: DONE}
	if !task.IsDone() {
		t.Errorf("Task with DONE status should return true for IsDone()")
	}
}

func TestTaskIsSatisfied(t *testing.T) {
	satisfied := []TaskStatus{MERGED, DONE, LOCAL_DONE}
	unsatisfied := []TaskStatus{PLANNED, READY, BRANCH_CREATED, TESTS_WRITTEN, RED_VERIFIED,
		IMPLEMENTING, LOCAL_TESTS_PASS, REVIEW, REVIEW_PASS, PR_OPEN, CI_RUNNING, CI_PASS,
		FIX_REQUIRED, BLOCKED}

	for _, status := range satisfied {
		if !(&Task{Status: status}).IsSatisfied() {
			t.Errorf("%s should be satisfied", status)
		}
	}
	for _, status := range unsatisfied {
		if (&Task{Status: status}).IsSatisfied() {
			t.Errorf("%s should not be satisfied", status)
		}
	}
}

func TestAllSatisfied(t *testing.T) {
	if AllSatisfied(nil) {
		t.Error("an empty task set is not a completed plan")
	}
	if !AllSatisfied([]*Task{{Status: LOCAL_DONE}, {Status: DONE}, {Status: MERGED}}) {
		t.Error("all satisfied states should be complete")
	}
	if AllSatisfied([]*Task{{Status: LOCAL_DONE}, {Status: BLOCKED}}) {
		t.Error("one unresolved task must make the plan incomplete")
	}
}

// TestContinuationRequeueDoesNotSpendAttempt documents the plan-run/task-store
// separation behind "sop task <id>" showing Attempts: 0 after a run reached a
// nonzero continuation number: a CONTINUE returns the task to PLANNED without
// spending a retry attempt (the bounded continuation budget is separate and lives
// in the run), whereas a retry requeue increments the attempt counter. This is
// intentional, not stale state.
func TestContinuationRequeueDoesNotSpendAttempt(t *testing.T) {
	base := &Task{ID: "T", Status: READY, MaxAttempts: 3}

	cont := *base
	if err := cont.RequeueWithoutSpending(); err != nil {
		t.Fatalf("RequeueWithoutSpending: %v", err)
	}
	if cont.Status != PLANNED {
		t.Errorf("status = %s, want PLANNED", cont.Status)
	}
	if cont.Attempt != 0 {
		t.Errorf("attempt = %d, want 0 (a continuation must not spend a retry attempt)", cont.Attempt)
	}

	retry := *base
	if err := retry.Requeue(); err != nil {
		t.Fatalf("Requeue: %v", err)
	}
	if retry.Status != PLANNED {
		t.Errorf("status = %s, want PLANNED", retry.Status)
	}
	if retry.Attempt != 1 {
		t.Errorf("attempt = %d, want 1 (a retry spends an attempt)", retry.Attempt)
	}
}
