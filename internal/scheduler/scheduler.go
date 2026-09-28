// Package scheduler performs deterministic, dependency-aware task scheduling.
// It selects at most one legally runnable task and is part of SOP's control
// plane: it never consults a model.
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
	// ActiveTask: a task is already in progress; V1 schedules one task at a time.
	ActiveTask Outcome = "ACTIVE_TASK"
	// WaitingOnDependencies: PLANNED tasks remain but none are eligible yet.
	WaitingOnDependencies Outcome = "WAITING_ON_DEPENDENCIES"
	// AllDone: no runnable work remains because all tasks are complete.
	AllDone Outcome = "ALL_DONE"
	// Blocked: progress cannot continue because a task is terminally blocked.
	Blocked Outcome = "BLOCKED"
)

// Result is the outcome of a scheduling attempt. Task is set only when the
// outcome is ReadyTask.
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
type Scheduler struct {
	store TaskStore
}

// New returns a Scheduler backed by store.
func New(store TaskStore) *Scheduler {
	return &Scheduler{store: store}
}

// Next inspects persisted tasks and promotes at most one PLANNED task whose
// dependencies are complete to READY.
func (s *Scheduler) Next(_ context.Context) (Result, error) {
	tasks, err := s.store.List()
	if err != nil {
		return Result{}, err
	}

	if err := checkDependenciesExist(tasks); err != nil {
		return Result{}, err
	}

	// V1 runs one task at a time.
	for _, task := range tasks {
		if IsActive(task.Status) {
			return Result{Outcome: ActiveTask}, nil
		}
	}

	byID := index(tasks)
	var eligible []*domain.Task
	for _, task := range tasks {
		if task.Status != domain.PLANNED {
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

	// Nothing is eligible. Report why.
	plannedRemain := false
	blockedExists := false
	for _, task := range tasks {
		switch task.Status {
		case domain.PLANNED:
			plannedRemain = true
		case domain.BLOCKED:
			blockedExists = true
		}
	}

	if blockedExists {
		return Result{Outcome: Blocked}, nil
	}
	if plannedRemain {
		return Result{Outcome: WaitingOnDependencies}, nil
	}
	return Result{Outcome: AllDone}, nil
}

// IsActive reports whether a task represents in-flight work that prevents V1
// from scheduling another task. PLANNED and terminal states are not active. It is
// the single definition of "occupies the execution slot", shared by the scheduler
// and by `sop run` when it resumes an interrupted task.
func IsActive(status domain.TaskStatus) bool {
	switch status {
	case domain.PLANNED, domain.MERGED, domain.DONE, domain.LOCAL_DONE, domain.BLOCKED:
		return false
	default:
		return true
	}
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
