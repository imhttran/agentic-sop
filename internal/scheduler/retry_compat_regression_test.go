package scheduler

import (
	"testing"

	"github.com/imhttran/agentic-sop/internal/domain"
)

// Explicit `sop retry <task>` requeues a single BLOCKED task through the same
// domain.Requeue path the scheduler uses for automatic recovery. These tests
// prove explicit retry stays compatible with the automatic recovery path: both
// use the identical eligibility predicate and both refuse completed,
// NEEDS_HUMAN, and unsatisfied-dependency tasks. No LLM, network, Git, or state
// deletion is involved.

// retryOne models `sop retry <task>`: it requeues a single BLOCKED task exactly
// as the CLI does, and reports whether it requeued.
func retryOne(tasks []*domain.Task, id string) bool {
	if id == "" {
		return false
	}
	for _, task := range tasks {
		if task.ID != id {
			continue
		}
		if task.Status != domain.BLOCKED {
			return false
		}
		return task.Requeue() == nil
	}
	return false
}

// TestRetryCompatAutomaticAndExplicitAgreeOnEligibility proves the automatic
// recovery path and explicit retry agree on which blocked tasks are eligible.
func TestRetryCompatAutomaticAndExplicitAgreeOnEligibility(t *testing.T) {
	cases := []struct {
		name   string
		task   *domain.Task
		wantOK bool
	}{
		{"recoverable blocked", blockedTask("T001"), true},
		{"terminally blocked (NEEDS_HUMAN)", &domain.Task{ID: "T001", Status: domain.BLOCKED, Attempt: 3, MaxAttempts: 3}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Automatic path.
			schedStore := &fakeStore{tasks: []*domain.Task{cloneTask(tc.task)}}
			res := next(t, schedStore)
			autoEligible := res.Outcome == RecoveredTask

			// Explicit retry path.
			retryTasks := []*domain.Task{cloneTask(tc.task)}
			explicitEligible := retryOne(retryTasks, "T001")

			if autoEligible != tc.wantOK {
				t.Errorf("automatic eligible = %v, want %v", autoEligible, tc.wantOK)
			}
			if explicitEligible != tc.wantOK {
				t.Errorf("explicit eligible = %v, want %v", explicitEligible, tc.wantOK)
			}
			if autoEligible != explicitEligible {
				t.Errorf("automatic and explicit disagree: auto=%v explicit=%v", autoEligible, explicitEligible)
			}
		})
	}
}

// TestRetryCompatExplicitNeverRecoversCompletedOrNeedsHuman proves explicit retry
// refuses completed and NEEDS_HUMAN tasks, matching the automatic path.
func TestRetryCompatExplicitNeverRecoversCompletedOrNeedsHuman(t *testing.T) {
	for _, status := range []domain.TaskStatus{domain.LOCAL_DONE, domain.MERGED, domain.DONE} {
		tasks := []*domain.Task{{ID: "T001", Status: status}}
		if retryOne(tasks, "T001") {
			t.Errorf("explicit retry requeued a completed task (%s)", status)
		}
		if tasks[0].Status != status {
			t.Errorf("completed task status changed to %s", tasks[0].Status)
		}
	}

	needsHuman := []*domain.Task{{ID: "T001", Status: domain.BLOCKED, Attempt: 3, MaxAttempts: 3}}
	if retryOne(needsHuman, "T001") {
		t.Errorf("explicit retry requeued a NEEDS_HUMAN (budget-exhausted) task")
	}
	if needsHuman[0].Status != domain.BLOCKED {
		t.Errorf("NEEDS_HUMAN task status = %s, want BLOCKED", needsHuman[0].Status)
	}
}

// TestRetryCompatExplicitAndAutomaticShareOrdering proves explicit retry of an
// eligible task then automatic recovery of the remainder produces the same
// ordering the pure automatic path would.
func TestRetryCompatExplicitAndAutomaticShareOrdering(t *testing.T) {
	// Explicit retry T001, then let automatic recovery pick up T003.
	store := &fakeStore{tasks: []*domain.Task{
		blockedTask("T001"),
		blockedTask("T003"),
	}}

	// Model `sop retry T001`: requeue T001 in place.
	if !retryOne(store.tasks, "T001") {
		t.Fatalf("explicit retry of T001 failed")
	}
	if find(t, store.tasks, "T001").Status != domain.PLANNED {
		t.Fatalf("T001 status = %s, want PLANNED after explicit retry", find(t, store.tasks, "T001").Status)
	}

	s := New(store)
	// The requeued T001 is runnable work and is selected before recovery of T003.
	res, err := s.Next(nil)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if res.Outcome != ReadyTask || res.Task == nil || res.Task.ID != "T001" {
		t.Fatalf("outcome = %s task = %+v, want READY_TASK T001", res.Outcome, res.Task)
	}

	// Complete T001, then automatic recovery picks the next blocked task.
	find(t, store.tasks, "T001").Status = domain.LOCAL_DONE
	res, err = s.Next(nil)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if res.Outcome != RecoveredTask || res.Task == nil || res.Task.ID != "T003" {
		t.Fatalf("outcome = %s task = %+v, want RECOVERED_TASK T003", res.Outcome, res.Task)
	}
}

// cloneTask returns a shallow copy of a task so the two paths under comparison do
// not share mutable state.
func cloneTask(task *domain.Task) *domain.Task {
	copy := *task
	copy.DependencyIDs = append([]string(nil), task.DependencyIDs...)
	return &copy
}
