// Package completion drives a plan to the end without supervision: it selects
// the next runnable task, drives it to CI, and completes validated work
// (merge -> refresh main -> DONE), repeating until nothing remains. It is
// control-plane and never lets a model decide whether work is complete.
package completion

import (
	"context"
	"errors"

	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/scheduler"
)

// Outcome is the terminal outcome of a completion step or run.
type Outcome string

const (
	// Progressed: one task was executed, merged, and marked DONE.
	Progressed Outcome = "PROGRESSED"
	// Waiting: work remains but none is runnable yet (in-flight or blocked deps).
	Waiting Outcome = "WAITING"
	// AllDone: every task is complete.
	AllDone Outcome = "ALL_DONE"
	// Blocked: progress cannot continue (a task is terminally blocked, or an
	// automatic recovery failed and the run stopped).
	Blocked Outcome = "BLOCKED"
)

// Result reports an outcome plus the task involved and how many tasks were
// completed during the run.
type Result struct {
	Outcome   Outcome
	TaskID    string
	Completed int
}

// Store is the persistence boundary the loop and scheduler need.
type Store interface {
	List() ([]*domain.Task, error)
	Get(id string) (*domain.Task, error)
	Save(task *domain.Task) error
}

// Executor drives one READY task through implementation and CI to CI_PASS, or
// blocks it. It owns the task's own transitions and never merges.
type Executor interface {
	Execute(ctx context.Context, task *domain.Task) error
}

// Merger merges a CI-passed task's pull request. It reports whether the merge
// happened; a merge the policy refuses is reported as (false, nil), not an
// error.
type Merger interface {
	Merge(ctx context.Context, task *domain.Task) (merged bool, err error)
}

// Refresher updates the local integration branch after a merge.
type Refresher interface {
	Refresh(ctx context.Context) error
}

// Handoff records a compact handoff for a completed task. It is best-effort:
// the loop calls it after a task is DONE and never lets its failure change task
// state or block progress.
type Handoff interface {
	Complete(ctx context.Context, task *domain.Task) error
}

// Loop completes tasks in dependency order until the plan is done or blocked.
type Loop struct {
	store     Store
	executor  Executor
	merger    Merger
	refresher Refresher
	handoff   Handoff
	schedule  *scheduler.Scheduler
}

// New builds a Loop. Readiness is delegated to a scheduler over store.
func New(store Store, executor Executor, merger Merger, refresher Refresher) *Loop {
	return &Loop{
		store:     store,
		executor:  executor,
		merger:    merger,
		refresher: refresher,
		schedule:  scheduler.New(store),
	}
}

// WithHandoff attaches a best-effort handoff recorder called after each task is
// marked DONE. It returns the Loop for chaining.
func (l *Loop) WithHandoff(h Handoff) *Loop {
	l.handoff = h
	return l
}

// Advance performs one task's lifecycle: select, execute, merge, refresh, and
// mark DONE. It returns Progressed when a task completed, or a terminal/waiting
// outcome when none could. Errors and cancellation propagate.
//
// Selection runs in the scheduler's precedence: normal runnable work first, then
// recovery of the earliest recoverable BLOCKED task. A recovery selection requeues
// the task to PLANNED without executing it; Advance re-runs selection so the
// requeued task is picked up as ordinary runnable work, keeping recovery a
// re-evaluation pass rather than a bypass of the PLANNED -> READY promotion. When
// a task the scheduler already recovered is BLOCKED again, the recovery failed:
// the scheduler reports FailedRecovery and Advance stops with the terminal
// Blocked outcome, naming the task, so no later task is executed.
func (l *Loop) Advance(ctx context.Context) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}

	// Select the next task to execute. Recovery selections (RecoveredTask) only
	// requeue a blocked task to PLANNED, so selection is repeated until it yields
	// runnable work or a terminal/waiting outcome. Each recovery is recorded by the
	// scheduler, which recovers any task at most once per invocation, so this loop
	// terminates: a task that re-blocks after its one recovery makes the scheduler
	// report FailedRecovery instead of a second RecoveredTask.
	scheduled, result, done, err := l.selectTask(ctx)
	if err != nil {
		return Result{}, err
	}
	if done {
		return result, nil
	}

	task := scheduled.Task
	if task == nil {
		return Result{}, errors.New("completion: scheduler returned no task for READY_TASK")
	}

	if err := l.executor.Execute(ctx, task); err != nil {
		return Result{TaskID: task.ID}, err
	}

	current, err := l.store.Get(task.ID)
	if err != nil {
		return Result{TaskID: task.ID}, err
	}
	if current.Status != domain.CI_PASS {
		// The executor did not reach CI (e.g. it blocked the task).
		return Result{TaskID: current.ID, Outcome: Blocked}, nil
	}

	merged, err := l.merger.Merge(ctx, current)
	if err != nil {
		return Result{TaskID: current.ID}, err
	}
	if !merged {
		return Result{TaskID: current.ID, Outcome: Blocked}, nil
	}

	if err := l.transition(current, domain.MERGED); err != nil {
		return Result{TaskID: current.ID}, err
	}
	if err := l.refresher.Refresh(ctx); err != nil {
		return Result{TaskID: current.ID}, err
	}
	if err := l.transition(current, domain.DONE); err != nil {
		return Result{TaskID: current.ID}, err
	}

	// Handoff is best-effort: a failure must never change completed work or
	// block progress, so its error is intentionally not propagated.
	if l.handoff != nil {
		_ = l.handoff.Complete(ctx, current)
	}

	return Result{Outcome: Progressed, TaskID: current.ID}, nil
}

// selectTask repeatedly asks the scheduler for work until it yields a READY task
// or a terminal/waiting outcome. A RecoveredTask only requeues a blocked task to
// PLANNED, so selection is retried; a FailedRecovery stops the loop with the
// terminal Blocked outcome, naming the task, so no later task is executed. When
// the third return is true, selection is finished and result is the outcome to
// return verbatim; otherwise scheduled names the READY task to execute.
//
// The loop is bounded by the scheduler's per-invocation recovery guard: the
// scheduler recovers each task at most once, so the number of RecoveredTask
// iterations is at most the number of tasks, after which the scheduler reports a
// terminal outcome. Cancellation is honored on every iteration.
func (l *Loop) selectTask(ctx context.Context) (scheduled scheduler.Result, result Result, done bool, err error) {
	for {
		if err := ctx.Err(); err != nil {
			return scheduler.Result{}, Result{}, false, err
		}

		scheduled, err = l.schedule.Next(ctx)
		if err != nil {
			return scheduler.Result{}, Result{}, false, err
		}

		switch scheduled.Outcome {
		case scheduler.ReadyTask:
			// Selected runnable work: return it for execution.
			return scheduled, Result{}, false, nil
		case scheduler.RecoveredTask:
			// A recoverable BLOCKED task was requeued to PLANNED; re-select so it
			// is promoted to READY through the normal path.
			continue
		case scheduler.FailedRecovery:
			// A recovered task is BLOCKED again: the recovery failed. Stop with the
			// terminal Blocked outcome, naming the task; do not select later work.
			id := ""
			if scheduled.Task != nil {
				id = scheduled.Task.ID
			}
			return scheduler.Result{}, Result{Outcome: Blocked, TaskID: id}, true, nil
		case scheduler.ActiveTask, scheduler.WaitingOnDependencies:
			return scheduler.Result{}, Result{Outcome: Waiting}, true, nil
		case scheduler.AllDone:
			return scheduler.Result{}, Result{Outcome: AllDone}, true, nil
		default: // scheduler.Blocked
			return scheduler.Result{}, Result{Outcome: Blocked}, true, nil
		}
	}
}

// Run advances until the plan is done, blocked, or waiting, counting completed
// tasks. It terminates: every Progressed step completes exactly one task.
func (l *Loop) Run(ctx context.Context) (Result, error) {
	completed := 0
	for {
		result, err := l.Advance(ctx)
		if err != nil {
			result.Completed = completed
			return result, err
		}
		if result.Outcome == Progressed {
			completed++
			continue
		}
		result.Completed = completed
		return result, nil
	}
}

// transition stages a state change on a copy and persists it, so a failed save
// cannot leave the in-memory task in a state that was never stored.
func (l *Loop) transition(task *domain.Task, status domain.TaskStatus) error {
	staged := *task
	if err := staged.Transition(status); err != nil {
		return err
	}
	if err := l.store.Save(&staged); err != nil {
		return err
	}
	task.Status = staged.Status
	task.UpdatedAt = staged.UpdatedAt
	return nil
}
