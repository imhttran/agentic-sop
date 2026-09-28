package scheduler

import (
	"testing"

	"github.com/imhttran/agentic-sop/internal/domain"
)

// The integration scenarios drive the same deterministic, in-memory recovery
// harness used by the unit coverage: no LLM, no network, no Git operations, and
// no deletion of the real state database. They prove the end-to-end shape of the
// automatic recovery path rather than any single decision.

// drive runs the scheduler until it stops making progress and returns the
// scheduling history. It models a `sop run` loop: when a task is selected it is
// executed through the supplied callback. A false return means the task failed
// and the run must stop at that point.
func drive(t *testing.T, store *fakeStore, s *Scheduler, execute func(task *domain.Task) bool) []Result {
	t.Helper()
	var history []Result
	for i := 0; i < 100; i++ {
		res, err := s.Next(nil)
		if err != nil {
			t.Fatalf("Next returned error: %v", err)
		}
		history = append(history, res)
		switch res.Outcome {
		case ReadyTask:
			if !execute(res.Task) {
				return history
			}
		case RecoveredTask:
			// Nothing to do; the next iteration re-selects the requeued task.
		case ActiveTask, WaitingOnDependencies, AllDone, Blocked, FailedRecovery:
			return history
		}
	}
	t.Fatal("scheduler did not terminate within the iteration bound")
	return history
}

// TestIntegrationSuccessRecoveryReachesPlanCompletion drives a blocked plan
// through automatic recovery to plan completion, asserting eligible blocked
// tasks are recovered in dependency order and that completed tasks stay skipped.
func TestIntegrationSuccessRecoveryReachesPlanCompletion(t *testing.T) {
	// T001 and T003 are historical blocks. T002 depends on T001, T004 depends on
	// T003. Recovery must respect the ordering and the plan must complete.
	store := &fakeStore{tasks: []*domain.Task{
		blockedTask("T001"),
		task("T002", domain.PLANNED, "T001"),
		blockedTask("T003"),
		task("T004", domain.PLANNED, "T003"),
	}}
	s := New(store)

	var executed []string
	history := drive(t, store, s, func(task *domain.Task) bool {
		executed = append(executed, task.ID)
		// A successful attempt completes the task locally.
		find(t, store.tasks, task.ID).Status = domain.LOCAL_DONE
		return true
	})

	// Every task ran exactly once, in dependency order.
	if !equalStrings(executed, []string{"T001", "T002", "T003", "T004"}) {
		t.Fatalf("executed = %v, want T001 T002 T003 T004", executed)
	}

	// The plan reached completion.
	if last := history[len(history)-1]; last.Outcome != AllDone {
		t.Fatalf("final outcome = %s, want ALL_DONE", last.Outcome)
	}
	if !domain.AllSatisfied(store.tasks) {
		t.Errorf("plan is not fully satisfied: %+v", store.tasks)
	}

	// Completed tasks were never rerun: each id appears exactly once.
	seen := map[string]int{}
	for _, id := range executed {
		seen[id]++
	}
	for _, id := range []string{"T001", "T002", "T003", "T004"} {
		if seen[id] != 1 {
			t.Errorf("%s executed %d times, want 1", id, seen[id])
		}
	}
}

// TestIntegrationFailureStopsAtFirstFailedRecovery injects a failure at the
// first recovery and asserts recovery halts immediately with no later tasks
// executed. T003 depends on T001 so it only becomes runnable after T001
// completes; because T001's recovery fails, T003 is never reached.
func TestIntegrationFailureStopsAtFirstFailedRecovery(t *testing.T) {
	store := &fakeStore{tasks: []*domain.Task{
		blockedTask("T001"),
		task("T003", domain.PLANNED, "T001"),
		blockedTask("T005", "T003"),
	}}
	s := New(store)

	var executed []string
	history := drive(t, store, s, func(task *domain.Task) bool {
		executed = append(executed, task.ID)
		if task.ID == "T001" {
			// The attempt failed: the task blocks again, which is a failed recovery.
			find(t, store.tasks, task.ID).Status = domain.BLOCKED
			return true
		}
		find(t, store.tasks, task.ID).Status = domain.LOCAL_DONE
		return true
	})

	// Only T001 executed before the run stopped.
	if !equalStrings(executed, []string{"T001"}) {
		t.Fatalf("executed = %v, want only T001", executed)
	}
	last := history[len(history)-1]
	if last.Outcome != FailedRecovery || last.Task == nil || last.Task.ID != "T001" {
		t.Fatalf("final outcome = %s task = %+v, want FAILED_RECOVERY T001", last.Outcome, last.Task)
	}

	// No later task was reached: T003 stays PLANNED and T005 stays BLOCKED.
	if got := find(t, store.tasks, "T003").Status; got != domain.PLANNED {
		t.Errorf("T003 status = %s, want untouched PLANNED", got)
	}
	if got := find(t, store.tasks, "T005").Status; got != domain.BLOCKED {
		t.Errorf("T005 status = %s, want untouched BLOCKED", got)
	}
}

// TestIntegrationCompletedTasksRemainSkipped proves completed tasks are never
// recovered or rerun while the automatic path makes progress on other work.
func TestIntegrationCompletedTasksRemainSkipped(t *testing.T) {
	store := &fakeStore{tasks: []*domain.Task{
		task("T001", domain.LOCAL_DONE),
		task("T002", domain.MERGED),
		task("T003", domain.DONE),
		blockedTask("T004"),
	}}
	s := New(store)

	var executed []string
	history := drive(t, store, s, func(task *domain.Task) bool {
		executed = append(executed, task.ID)
		find(t, store.tasks, task.ID).Status = domain.LOCAL_DONE
		return true
	})

	if !equalStrings(executed, []string{"T004"}) {
		t.Fatalf("executed = %v, want only T004", executed)
	}
	if last := history[len(history)-1]; last.Outcome != AllDone {
		t.Fatalf("final outcome = %s, want ALL_DONE", last.Outcome)
	}
}

// TestIntegrationNeedsHumanAndDependencyBlockedNeverRecovered proves the
// automatic path never recovers a NEEDS_HUMAN task or a task with an unsatisfied
// dependency, even when other work exists.
func TestIntegrationNeedsHumanAndDependencyBlockedNeverRecovered(t *testing.T) {
	needsHuman := &domain.Task{ID: "T001", Status: domain.BLOCKED, BlockedReason: domain.REVIEW_UNRESOLVED, Attempt: 3, MaxAttempts: 3}
	depBlocked := &domain.Task{ID: "T003", Status: domain.BLOCKED, BlockedReason: domain.REVIEW_UNRESOLVED, Attempt: 1, MaxAttempts: 3, DependencyIDs: []string{"T002"}}
	store := &fakeStore{tasks: []*domain.Task{
		needsHuman,
		task("T002", domain.IMPLEMENTING),
		depBlocked,
	}}
	s := New(store)

	// The in-flight task occupies the slot, so no recovery happens.
	history := drive(t, store, s, func(task *domain.Task) bool {
		t.Errorf("no task should be selected: %s", task.ID)
		return false
	})
	if last := history[len(history)-1]; last.Outcome != ActiveTask {
		t.Fatalf("final outcome = %s, want ACTIVE_TASK", last.Outcome)
	}
	if needsHuman.Status != domain.BLOCKED {
		t.Errorf("NEEDS_HUMAN task status = %s, want BLOCKED", needsHuman.Status)
	}
	if s.RecoveredAlready("T001") {
		t.Errorf("NEEDS_HUMAN task was automatically recovered")
	}
	if depBlocked.Status != domain.BLOCKED {
		t.Errorf("dependency-blocked task status = %s, want BLOCKED", depBlocked.Status)
	}
	if s.RecoveredAlready("T003") {
		t.Errorf("dependency-blocked task was automatically recovered")
	}
}
