// Package scheduler performs deterministic, dependency-aware task scheduling.
// It selects at most one legally runnable task and is part of SOP's control
// plane: it never consults a model.
//
// # Selection semantics
//
// Next inspects persisted tasks and reports exactly one Outcome for the state.
// Selection happens in a fixed precedence:
//
//  1. In-flight work (IsActive) occupies the single execution slot: Next returns
//     ActiveTask and never selects another task while it lasts.
//  2. Normal runnable work: PLANNED tasks whose dependencies are all satisfied
//     (ResolveDependencies) and which may legally transition to READY are
//     runnable; the lowest ID is promoted and returned as ReadyTask — one at a
//     time, deterministic order. Normal runnable work is always selected before
//     any recovery work.
//  3. Failed recovery: a BLOCKED task that this invocation already recovered
//     (requeued to PLANNED) and that is BLOCKED again has failed recovery. Next
//     fails closed: it returns FailedRecovery and selects no further task, so
//     later work never executes after a failed automatic recovery. The BLOCKED
//     task is left untouched, so its original failure diagnostic (BlockedReason
//     and recorded attempt signature) remains available.
//  4. Recovery work: only when no runnable work exists and no recovery has
//     failed, the earliest BLOCKED task that is safe to re-evaluate
//     (IsBlockedSelectable: BLOCKED, still recoverable via the requeue path,
//     with all dependencies satisfied, and not already recovered in this
//     invocation) is Requeued to PLANNED and returned as RecoveredTask. The
//     lowest ID among the eligible BLOCKED tasks is chosen, so a later blocked
//     task is never promoted ahead of an earlier eligible one, and no task is
//     skipped to reach later work. A task is recovered at most once per
//     Scheduler instance (per `sop run` invocation), so recovery is bounded and
//     cannot loop.
//  5. Otherwise Next reports why progress cannot continue: AllDone when every
//     task IsSatisfied, WaitingOnDependencies when only dependency-blocked work
//     remains, and the bare Blocked outcome only when BLOCKED tasks exist but
//     none is recoverable (all terminally blocked, dependency-blocked, or
//     already recovered in this invocation).
//
// Completed work (LOCAL_DONE, MERGED, DONE) is never selected: those statuses are
// not PLANNED and are not active. Completed work is never rerun. An empty plan
// has no outstanding work, so it is trivially complete and reported as AllDone.
package scheduler

import (
	"context"
	"fmt"
	"sort"

	"github.com/imhttran/agentic-sop/internal/domain"
)

// Outcome describes the result of a scheduling attempt.
type Outcome string

const (
	// ReadyTask: a PLANNED task was legally promoted to READY.
	ReadyTask Outcome = "READY_TASK"
	// RecoveredTask: no runnable work remained, so the earliest recoverable
	// BLOCKED task was requeued to PLANNED and is returned for re-evaluation.
	RecoveredTask Outcome = "RECOVERED_TASK"
	// ActiveTask: a task is already in progress; V1 schedules one task at a time.
	ActiveTask Outcome = "ACTIVE_TASK"
	// WaitingOnDependencies: PLANNED tasks remain but none are eligible yet.
	WaitingOnDependencies Outcome = "WAITING_ON_DEPENDENCIES"
	// AllDone: no runnable work remains because all tasks are complete.
	AllDone Outcome = "ALL_DONE"
	// Blocked: progress cannot continue because a task is blocked and no runnable
	// or recoverable work remains. A BLOCKED task alongside runnable work does not
	// produce this outcome: the runnable work is selected first, and a recoverable
	// BLOCKED task with satisfied dependencies is selected through recovery.
	Blocked Outcome = "BLOCKED"
	// FailedRecovery: a task this invocation already automatically recovered is
	// BLOCKED again, so the recovery failed. The scheduler fails closed: it stops
	// selecting work so no later task executes. Result.Task names the task whose
	// recovery failed, with its original failure diagnostic left intact.
	FailedRecovery Outcome = "FAILED_RECOVERY"
)

// Result is the outcome of a scheduling attempt. Task is set when the outcome is
// ReadyTask, RecoveredTask, or FailedRecovery.
type Result struct {
	Outcome Outcome
	Task    *domain.Task
}

// TaskStore is the smallest persistence boundary the scheduler needs.
type TaskStore interface {
	List() ([]*domain.Task, error)
	Save(task *domain.Task) error
}

// Scheduler selects the next runnable task from persisted state.
//
// A Scheduler is not safe for concurrent use: Next assumes it is called
// serially. Parallel scheduling is deferred to a later task.
//
// A Scheduler also carries the per-invocation automatic-recovery guard: the set
// of blocked tasks it has already recovered (recovered). It is created empty by
// New and lives only as long as the Scheduler, so it is discarded when one
// `sop run` invocation ends and never leaks into another. It is never persisted.
// The guard is the single source of truth for "has this invocation already
// recovered this task"; callers that need to gate their own re-selection on it
// ask RecoveredAlready rather than tracking a parallel set.
type Scheduler struct {
	store     TaskStore
	recovered map[string]bool
}

// New returns a Scheduler backed by store, with an empty per-invocation recovery
// guard.
func New(store TaskStore) *Scheduler {
	return &Scheduler{store: store, recovered: map[string]bool{}}
}

// RecoveredAlready reports whether taskID has already been automatically
// recovered by this Scheduler instance (this invocation). It exposes the same
// per-invocation guard Next uses internally, so a caller's re-selection decision
// is gated on the scheduler's own recovery set rather than a parallel copy that
// could drift from it.
func (s *Scheduler) RecoveredAlready(taskID string) bool {
	return s.recovered[taskID]
}

// Next inspects persisted tasks and promotes at most one task: a PLANNED task
// whose dependencies are complete is promoted to READY, and when no runnable
// work remains the earliest recoverable BLOCKED task is requeued to PLANNED. It
// never selects completed work (LOCAL_DONE/MERGED/DONE), never promotes a PLANNED
// task with unsatisfied dependencies, never selects a blocked task with
// unsatisfied dependencies or an exhausted retry budget, and treats a BLOCKED
// task as non-fatal rather than terminal as long as runnable or recoverable work
// remains. Each blocked task is recovered at most once per Scheduler instance;
// when a task it already recovered is BLOCKED again, Next fails closed with
// FailedRecovery and selects nothing further.
func (s *Scheduler) Next(_ context.Context) (Result, error) {
	tasks, err := s.store.List()
	if err != nil {
		return Result{}, err
	}

	if err := checkDependenciesExist(tasks); err != nil {
		return Result{}, err
	}

	// An empty plan has no outstanding work: it is trivially complete.
	if len(tasks) == 0 {
		return Result{Outcome: AllDone}, nil
	}

	// V1 runs one task at a time. An in-flight task occupies the execution slot.
	for _, task := range tasks {
		if IsActive(task.Status) {
			return Result{Outcome: ActiveTask}, nil
		}
	}

	byID := index(tasks)

	// Fail closed on the first failed automatic recovery: a task already recovered
	// in this invocation that is BLOCKED again did not recover. Stop selecting
	// work so no later task executes; the task is left untouched, preserving its
	// original failure diagnostic.
	if failed := failedRecovery(tasks, s.recovered); failed != nil {
		return Result{Outcome: FailedRecovery, Task: failed}, nil
	}

	// Tier 1: normal runnable work is always selected before recovery work.
	var eligible []*domain.Task
	for _, task := range tasks {
		if !task.IsRunnable() {
			continue
		}
		if _, satisfied := task.ResolveDependencies(byID); !satisfied {
			continue
		}
		if !task.CanTransitionTo(domain.READY) {
			continue
		}
		eligible = append(eligible, task)
	}

	if len(eligible) > 0 {
		sort.Slice(eligible, func(i, j int) bool { return eligible[i].ID < eligible[j].ID })
		next := eligible[0]

		// Stage the transition on a copy so a failed save cannot leave the
		// in-memory task in a state that was never persisted.
		staged := *next
		if err := staged.Transition(domain.READY); err != nil {
			return Result{}, err
		}
		if err := s.store.Save(&staged); err != nil {
			return Result{}, err
		}

		// Persistence succeeded: reflect the change in the in-memory view.
		next.Status = staged.Status
		next.UpdatedAt = staged.UpdatedAt
		return Result{Outcome: ReadyTask, Task: next}, nil
	}

	// Tier 2: no runnable work exists, so select the earliest recoverable blocked
	// task that is safe to re-evaluate and requeue it to PLANNED. The lowest ID is
	// chosen so a later blocked task is never promoted ahead of an earlier
	// eligible one. A task already recovered in this invocation is skipped, so the
	// same blocked task is never recovered twice and no unbounded loop is possible.
	var recoverable []*domain.Task
	for _, task := range tasks {
		if s.RecoveredAlready(task.ID) {
			continue
		}
		if task.IsBlockedSelectable(byID) {
			recoverable = append(recoverable, task)
		}
	}

	if len(recoverable) > 0 {
		sort.Slice(recoverable, func(i, j int) bool { return recoverable[i].ID < recoverable[j].ID })
		next := recoverable[0]

		// Stage the requeue on a copy so a failed save cannot leave the in-memory
		// task in a state that was never persisted. Requeue is the existing budget
		// accounting path; a budget that is actually exhausted is surfaced rather
		// than silently selected.
		staged := *next
		if err := staged.Requeue(); err != nil {
			return Result{}, err
		}
		if err := s.store.Save(&staged); err != nil {
			return Result{}, err
		}

		// Persistence succeeded: reflect the change in the in-memory view and mark
		// the task recovered for the rest of this invocation.
		s.recovered[next.ID] = true
		next.Status = staged.Status
		next.BlockedReason = staged.BlockedReason
		next.Attempt = staged.Attempt
		next.Attempts = staged.Attempts
		next.UpdatedAt = staged.UpdatedAt
		return Result{Outcome: RecoveredTask, Task: next}, nil
	}

	// Nothing runnable or recoverable remains. Report why, distinguishing a plan
	// that is fully complete from one that is merely waiting or blocked.
	if domain.AllSatisfied(tasks) {
		return Result{Outcome: AllDone}, nil
	}

	// A BLOCKED task only stops the plan when no runnable or recoverable work
	// remains; otherwise that work above would already have been selected. A plan
	// that is not all-satisfied and has no BLOCKED task is waiting on dependency
	// ordering, not complete, so it is reported as WaitingOnDependencies rather
	// than claiming completion.
	for _, task := range tasks {
		if task.IsBlocked() {
			return Result{Outcome: Blocked}, nil
		}
	}
	return Result{Outcome: WaitingOnDependencies}, nil
}

// failedRecovery returns the earliest task that this invocation already recovered
// (recovered) and that is BLOCKED again, or nil when no automatic recovery has
// failed. It never mutates a task, so the failed task keeps its BlockedReason and
// recorded attempt context as of the moment it re-blocked (including the attempt
// the failed automatic recovery spent).
func failedRecovery(tasks []*domain.Task, recovered map[string]bool) *domain.Task {
	var failed []*domain.Task
	for _, task := range tasks {
		if task.IsBlocked() && recovered[task.ID] {
			failed = append(failed, task)
		}
	}
	if len(failed) == 0 {
		return nil
	}
	sort.Slice(failed, func(i, j int) bool { return failed[i].ID < failed[j].ID })
	return failed[0]
}

// IsActive reports whether a task represents in-flight work that prevents V1
// from scheduling another task. PLANNED and terminal states are not active. It is
// the single definition of "occupies the execution slot", shared by the scheduler
// and by `sop run` when it resumes an interrupted task.
func IsActive(status domain.TaskStatus) bool {
	return status != domain.PLANNED && status != domain.MERGED && !status.IsTerminal()
}

// checkDependenciesExist rejects persisted state where a task references a
// dependency that cannot be loaded.
func checkDependenciesExist(tasks []*domain.Task) error {
	known := make(map[string]bool, len(tasks))
	for _, task := range tasks {
		known[task.ID] = true
	}
	for _, task := range tasks {
		for _, dep := range task.DependencyIDs {
			if !known[dep] {
				return fmt.Errorf("task %s depends on missing task %s", task.ID, dep)
			}
		}
	}
	return nil
}

func index(tasks []*domain.Task) map[string]*domain.Task {
	byID := make(map[string]*domain.Task, len(tasks))
	for _, task := range tasks {
		byID[task.ID] = task
	}
	return byID
}
