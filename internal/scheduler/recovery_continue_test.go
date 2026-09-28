package scheduler

import (
	"context"
	"testing"

	"github.com/imhttran/agentic-sop/internal/domain"
)

// TestNextRecoversMultipleBlockedSequentially proves multiple historical blocked
// tasks are recovered sequentially by one Scheduler instance (one invocation),
// each promoted through the same requeue transition an explicit `sop retry`
// performs, and each recovered at most once.
func TestNextRecoversMultipleBlockedSequentially(t *testing.T) {
	store := &fakeStore{tasks: []*domain.Task{
		blockedTask("T001"),
		blockedTask("T003"),
		blockedTask("T005"),
	}}
	s := New(store)

	// Each of the three blocked tasks is recovered once, in ascending id order.
	for _, want := range []string{"T001", "T003", "T005"} {
		// Drain the requeued task through READY so the scheduler moves on.
		res, err := s.Next(context.Background())
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		if res.Outcome != RecoveredTask || res.Task == nil || res.Task.ID != want {
			t.Fatalf("outcome = %s task = %+v, want RECOVERED_TASK %s", res.Outcome, res.Task, want)
		}
		if find(t, store.tasks, want).Status != domain.PLANNED {
			t.Errorf("%s status = %s, want PLANNED", want, find(t, store.tasks, want).Status)
		}
		// The requeued task is promoted to READY, then the invocation moves on.
		ready, err := s.Next(context.Background())
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		if ready.Outcome != ReadyTask || ready.Task == nil || ready.Task.ID != want {
			t.Fatalf("outcome = %s task = %+v, want READY_TASK %s", ready.Outcome, ready.Task, want)
		}
		// Simulate the task completing locally, so it is no longer selected.
		find(t, store.tasks, want).Status = domain.LOCAL_DONE
	}
}

// TestNextDoesNotRecoverSameTaskTwice proves the per-invocation one-attempt
// bound: once a task has been recovered and is BLOCKED again, Next fails closed
// with FailedRecovery rather than recovering it a second time.
func TestNextDoesNotRecoverSameTaskTwice(t *testing.T) {
	store := &fakeStore{tasks: []*domain.Task{blockedTask("T001")}}
	s := New(store)

	res, err := s.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if res.Outcome != RecoveredTask {
		t.Fatalf("outcome = %s, want %s", res.Outcome, RecoveredTask)
	}

	// The recovered task is blocked again: recovery failed.
	find(t, store.tasks, "T001").Status = domain.BLOCKED
	res, err = s.Next(context.Background())
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if res.Outcome != FailedRecovery {
		t.Fatalf("outcome = %s, want %s", res.Outcome, FailedRecovery)
	}
	if res.Task == nil || res.Task.ID != "T001" {
		t.Fatalf("result task = %+v, want T001", res.Task)
	}
}
