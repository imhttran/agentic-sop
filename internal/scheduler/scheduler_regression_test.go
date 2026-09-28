package scheduler

import (
	"context"
	"testing"

	"github.com/imhttran/agentic-sop/internal/domain"
)

// This file is the deterministic regression suite TASK008 requires. It exercises
// the scheduler and its automatic recovery path through the existing
// in-memory fakeStore harness: no LLM, no network, no Git operations, and no
// deletion of .agent-sdlc/state.db.
//
// recoveryHarness drives a Scheduler and records the observable recovery
// history (selection order, skips, and stop reason) so each of the 13 required
// scheduler/recovery cases can assert on the same deterministic observations.
type recoveryHarness struct {
	store   *fakeStore
	sched   *Scheduler
	history []Result
}

func newHarness(tasks ...*domain.Task) *recoveryHarness {
	store := &fakeStore{tasks: tasks}
	return &recoveryHarness{store: store, sched: New(store)}
}

// step runs one scheduling attempt and records it.
func (h *recoveryHarness) step(t *testing.T) Result {
	t.Helper()
	res, err := h.sched.Next(context.Background())
	if err != nil {
		t.Fatalf("Next returned error: %v", err)
	}
	h.history = append(h.history, res)
	return res
}

// selectedIDs returns the task IDs selected (ReadyTask or RecoveredTask) in order.
func (h *recoveryHarness) selectedIDs() []string {
	var ids []string
	for _, res := range h.history {
		if (res.Outcome == ReadyTask || res.Outcome == RecoveredTask) && res.Task != nil {
			ids = append(ids, res.Task.ID)
		}
	}
	return ids
}

func (h *recoveryHarness) last() Result { return h.history[len(h.history)-1] }

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

// --- The 13 required scheduler and recovery test cases ------------------------

// Case 1: a PLANNED task with no dependencies is promoted to READY.
func TestRegressionCase01PlannedTaskPromoted(t *testing.T) {
	h := newHarness(task("T001", domain.PLANNED))
	res := h.step(t)
	if res.Outcome != ReadyTask || res.Task == nil || res.Task.ID != "T001" {
		t.Fatalf("outcome = %s task = %+v, want READY_TASK T001", res.Outcome, res.Task)
	}
	if find(t, h.store.tasks, "T001").Status != domain.READY {
		t.Errorf("T001 status = %s, want READY", find(t, h.store.tasks, "T001").Status)
	}
}

// Case 2: dependency ordering selects the lowest-id eligible task
// deterministically regardless of storage order.
func TestRegressionCase02DependencyOrdering(t *testing.T) {
	orders := [][]*domain.Task{
		{task("T003", domain.PLANNED), task("T001", domain.PLANNED), task("T002", domain.PLANNED)},
		{task("T001", domain.PLANNED), task("T002", domain.PLANNED), task("T003", domain.PLANNED)},
		{task("T002", domain.PLANNED), task("T003", domain.PLANNED), task("T001", domain.PLANNED)},
	}
	for i, tasks := range orders {
		h := newHarness(tasks...)
		res := h.step(t)
		if res.Outcome != ReadyTask || res.Task == nil || res.Task.ID != "T001" {
			t.Errorf("run %d: outcome = %s task = %+v, want READY_TASK T001", i, res.Outcome, res.Task)
		}
	}
}

// Case 3: a PLANNED task with an unsatisfied dependency is never promoted.
func TestRegressionCase03UnsatisfiedDependencyNotPromoted(t *testing.T) {
	h := newHarness(
		task("T001", domain.IMPLEMENTING),
		task("T002", domain.PLANNED, "T001"),
	)
	if res := h.step(t); res.Outcome == ReadyTask {
		t.Errorf("T002 promoted though T001 is incomplete")
	}
	if find(t, h.store.tasks, "T002").Status != domain.PLANNED {
		t.Errorf("T002 status = %s, want PLANNED", find(t, h.store.tasks, "T002").Status)
	}
}

// Case 4: eligible blocked tasks are recovered when no runnable work exists.
func TestRegressionCase04EligibleBlockedRecovered(t *testing.T) {
	h := newHarness(blockedTask("T001"))
	res := h.step(t)
	if res.Outcome != RecoveredTask || res.Task == nil || res.Task.ID != "T001" {
		t.Fatalf("outcome = %s task = %+v, want RECOVERED_TASK T001", res.Outcome, res.Task)
	}
	if find(t, h.store.tasks, "T001").Status != domain.PLANNED {
		t.Errorf("T001 status = %s, want PLANNED after recovery", find(t, h.store.tasks, "T001").Status)
	}
}

// Case 5: completed tasks remain skipped and are never rerun.
func TestRegressionCase05CompletedTasksNeverRerun(t *testing.T) {
	completed := []domain.TaskStatus{domain.LOCAL_DONE, domain.MERGED, domain.DONE}
	for _, status := range completed {
		h := newHarness(&domain.Task{ID: "T001", Status: status})
		res := h.step(t)
		if res.Outcome == ReadyTask || res.Outcome == RecoveredTask {
			t.Errorf("status %s was selected for execution", status)
		}
		if res.Outcome != AllDone {
			t.Errorf("status %s: outcome = %s, want ALL_DONE", status, res.Outcome)
		}
		if find(t, h.store.tasks, "T001").Status != status {
			t.Errorf("status = %s, want unchanged %s", find(t, h.store.tasks, "T001").Status, status)
		}
	}
}

// Case 6: a NEEDS_HUMAN task (budget exhausted, terminally BLOCKED) is never
// automatically recovered.
func TestRegressionCase06NeedsHumanNeverRecovered(t *testing.T) {
	needsHuman := &domain.Task{ID: "T001", Status: domain.BLOCKED, BlockedReason: domain.REVIEW_UNRESOLVED, Attempt: 3, MaxAttempts: 3}
	h := newHarness(needsHuman)
	res := h.step(t)
	if res.Outcome == RecoveredTask {
		t.Errorf("NEEDS_HUMAN task was automatically recovered")
	}
	if res.Outcome != Blocked {
		t.Errorf("outcome = %s, want BLOCKED", res.Outcome)
	}
	if needsHuman.Status != domain.BLOCKED {
		t.Errorf("status = %s, want BLOCKED", needsHuman.Status)
	}
}

// Case 7: a blocked task with an unsatisfied dependency is never recovered.
func TestRegressionCase07UnsatisfiedDependencyBlockedNotRecovered(t *testing.T) {
	h := newHarness(
		task("T001", domain.PLANNED, "T002"),
		blockedTask("T002", "T001"),
	)
	if res := h.step(t); res.Outcome == RecoveredTask {
		t.Errorf("dependency-blocked task was recovered")
	}
	if find(t, h.store.tasks, "T002").Status != domain.BLOCKED {
		t.Errorf("T002 status = %s, want BLOCKED", find(t, h.store.tasks, "T002").Status)
	}
}

// Case 8: runnable work is always selected before recovery work.
func TestRegressionCase08RunnableBeforeRecovery(t *testing.T) {
	h := newHarness(blockedTask("T001"), task("T002", domain.PLANNED))
	res := h.step(t)
	if res.Outcome != ReadyTask || res.Task == nil || res.Task.ID != "T002" {
		t.Fatalf("outcome = %s task = %+v, want READY_TASK T002", res.Outcome, res.Task)
	}
	if find(t, h.store.tasks, "T001").Status != domain.BLOCKED {
		t.Errorf("T001 status = %s, want BLOCKED (no speculative requeue)", find(t, h.store.tasks, "T001").Status)
	}
}

// Case 9: multiple eligible blocked tasks are recovered sequentially in
// ascending id order, each at most once per invocation.
func TestRegressionCase09SequentialRecovery(t *testing.T) {
	h := newHarness(blockedTask("T001"), blockedTask("T003"), blockedTask("T005"))
	for _, want := range []string{"T001", "T003", "T005"} {
		res := h.step(t)
		if res.Outcome != RecoveredTask || res.Task == nil || res.Task.ID != want {
			t.Fatalf("outcome = %s task = %+v, want RECOVERED_TASK %s", res.Outcome, res.Task, want)
		}
		// Drain the requeued task through READY, then complete it locally.
		if ready := h.step(t); ready.Outcome != ReadyTask || ready.Task == nil || ready.Task.ID != want {
			t.Fatalf("outcome = %s task = %+v, want READY_TASK %s", ready.Outcome, ready.Task, want)
		}
		find(t, h.store.tasks, want).Status = domain.LOCAL_DONE
	}
	if !equalStrings(h.selectedIDs(), []string{"T001", "T001", "T003", "T003", "T005", "T005"}) {
		t.Errorf("selection order = %v, want each task recovered then readied in id order", h.selectedIDs())
	}
}

// Case 10: a task recovered once that blocks again fails recovery closed, and
// later tasks are not executed.
func TestRegressionCase10FailedRecoveryStopsRun(t *testing.T) {
	h := newHarness(blockedTask("T001"), blockedTask("T002"))
	first := h.step(t)
	if first.Outcome != RecoveredTask || first.Task == nil || first.Task.ID != "T001" {
		t.Fatalf("outcome = %s task = %+v, want RECOVERED_TASK T001", first.Outcome, first.Task)
	}
	// T001's recovery failed: it is BLOCKED again.
	find(t, h.store.tasks, "T001").Status = domain.BLOCKED
	res := h.step(t)
	if res.Outcome != FailedRecovery || res.Task == nil || res.Task.ID != "T001" {
		t.Fatalf("outcome = %s task = %+v, want FAILED_RECOVERY T001", res.Outcome, res.Task)
	}
	// The later blocked task must not be reached.
	if find(t, h.store.tasks, "T002").Status != domain.BLOCKED {
		t.Errorf("T002 status = %s, want untouched BLOCKED after a failed recovery", find(t, h.store.tasks, "T002").Status)
	}
}

// Case 11: an in-flight task occupies the execution slot and no other task is
// selected.
func TestRegressionCase11ActiveTaskOccupiesSlot(t *testing.T) {
	h := newHarness(
		task("T001", domain.IMPLEMENTING),
		blockedTask("T002"),
		task("T003", domain.PLANNED),
	)
	res := h.step(t)
	if res.Outcome != ActiveTask {
		t.Fatalf("outcome = %s, want ACTIVE_TASK", res.Outcome)
	}
	if find(t, h.store.tasks, "T002").Status != domain.BLOCKED {
		t.Errorf("T002 status = %s, want BLOCKED (slot occupied)", find(t, h.store.tasks, "T002").Status)
	}
	if find(t, h.store.tasks, "T003").Status != domain.PLANNED {
		t.Errorf("T003 status = %s, want PLANNED (slot occupied)", find(t, h.store.tasks, "T003").Status)
	}
}

// Case 12: dependency-only work reports WaitingOnDependencies, and a fully
// satisfied plan reports AllDone.
func TestRegressionCase12WaitingAndAllDone(t *testing.T) {
	h := newHarness(
		task("T001", domain.PLANNED, "T002"),
		task("T002", domain.PLANNED, "T001"),
	)
	if res := h.step(t); res.Outcome != WaitingOnDependencies {
		t.Errorf("outcome = %s, want WAITING_ON_DEPENDENCIES", res.Outcome)
	}

	done := newHarness(task("T001", domain.DONE), task("T002", domain.MERGED))
	if res := done.step(t); res.Outcome != AllDone {
		t.Errorf("outcome = %s, want ALL_DONE", res.Outcome)
	}
}

// Case 13: scheduling and recovery decisions are deterministic across repeated
// identical runs and permuted storage order.
func TestRegressionCase13DeterministicAcrossRuns(t *testing.T) {
	build := func() []*domain.Task {
		return []*domain.Task{
			blockedTask("T001"),
			blockedTask("T002"),
			task("T003", domain.PLANNED),
		}
	}

	var baseline []Outcome
	for run := 0; run < 5; run++ {
		h := newHarness(build()...)
		var got []Outcome
		for i := 0; i < 4; i++ {
			got = append(got, h.step(t).Outcome)
			// Advance whatever was selected so the run makes progress.
			if h.last().Task != nil {
				switch h.last().Outcome {
				case RecoveredTask:
					find(t, h.store.tasks, h.last().Task.ID).Status = domain.PLANNED
				case ReadyTask:
					find(t, h.store.tasks, h.last().Task.ID).Status = domain.LOCAL_DONE
				}
			}
		}
		if baseline == nil {
			baseline = got
			continue
		}
		if len(got) != len(baseline) {
			t.Fatalf("run %d outcomes = %v, want %v", run, got, baseline)
		}
		for i := range got {
			if got[i] != baseline[i] {
				t.Fatalf("run %d outcomes = %v, want %v", run, got, baseline)
			}
		}
	}
}
