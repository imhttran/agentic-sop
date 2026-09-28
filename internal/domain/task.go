package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type Dependency struct {
	TaskID         string
	RequiredStatus TaskStatus
}

type Attempt struct {
	Number    int
	Status    TaskStatus
	Reason    string
	Output    string
	Duration  time.Duration
	Timestamp time.Time
}

type Task struct {
	ID                 string
	Title              string
	Objective          string
	AcceptanceCriteria string
	Status             TaskStatus
	BlockedReason      BlockedReason
	ExecutionMode      ExecutionMode
	Attempt            int
	MaxAttempts        int
	DependencyIDs      []string
	Attempts           []Attempt
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// transitions is the authoritative state machine. Every workflow decision
// lives here; callers never compare states by hand.
var transitions = map[TaskStatus][]TaskStatus{
	PLANNED:          {READY},
	READY:            {BRANCH_CREATED},
	BRANCH_CREATED:   {TESTS_WRITTEN},
	TESTS_WRITTEN:    {RED_VERIFIED},
	RED_VERIFIED:     {IMPLEMENTING},
	IMPLEMENTING:     {LOCAL_TESTS_PASS, FIX_REQUIRED},
	LOCAL_TESTS_PASS: {REVIEW, FIX_REQUIRED},
	REVIEW:           {REVIEW_PASS, FIX_REQUIRED},
	REVIEW_PASS:      {PR_OPEN, LOCAL_DONE},
	PR_OPEN:          {CI_RUNNING},
	CI_RUNNING:       {CI_PASS, FIX_REQUIRED},
	CI_PASS:          {MERGED},
	FIX_REQUIRED:     {IMPLEMENTING},
	MERGED:           {DONE},
	DONE:             {},
	LOCAL_DONE:       {},
	BLOCKED:          {},
}

// CanTransitionTo reports whether the Task may move to nextStatus. It is a pure
// query with no side effects.
func (t *Task) CanTransitionTo(nextStatus TaskStatus) bool {
	for _, allowed := range transitions[t.Status] {
		if allowed == nextStatus {
			return true
		}
	}
	return false
}

// Transition validates nextStatus and applies it atomically: Status is mutated
// only after the transition is known to be legal, and UpdatedAt advances on
// success. An illegal transition returns an error and leaves the Task untouched.
func (t *Task) Transition(nextStatus TaskStatus) error {
	if !t.CanTransitionTo(nextStatus) {
		return fmt.Errorf("invalid transition: %s -> %s", t.Status, nextStatus)
	}
	t.Status = nextStatus
	t.UpdatedAt = time.Now()
	return nil
}

// Block moves the Task into the terminal BLOCKED state. A meaningful reason is
// required, and a completed Task cannot be blocked.
func (t *Task) Block(reason BlockedReason) error {
	if strings.TrimSpace(string(reason)) == "" {
		return fmt.Errorf("blocked reason must not be empty")
	}
	if t.Status == DONE || t.Status == LOCAL_DONE {
		return fmt.Errorf("cannot block a completed task")
	}
	t.Status = BLOCKED
	t.BlockedReason = reason
	t.UpdatedAt = time.Now()
	return nil
}

// ErrRetryExhausted is returned by Requeue when a task's retry budget
// (MaxAttempts) is spent, so a requeue loop cannot run forever.
var ErrRetryExhausted = errors.New("retry budget exhausted")

// Requeue returns a Task to PLANNED so the scheduler may select it again. It is
// the inverse of Block: a human boundary (NEEDS_HUMAN) is not terminal, so the
// work is not lost and a later run retries it. Like Block, it is an explicit
// domain operation rather than a Transition. A completed Task cannot be
// requeued, and a requeue spends one attempt against MaxAttempts; once the
// budget is spent it returns ErrRetryExhausted.
func (t *Task) Requeue() error {
	if err := t.canRequeue(); err != nil {
		return err
	}
	if t.MaxAttempts > 0 && t.Attempt >= t.MaxAttempts {
		return fmt.Errorf("%w (%d/%d)", ErrRetryExhausted, t.Attempt, t.MaxAttempts)
	}
	_ = t.AddAttempt(PLANNED, "requeued")
	t.Status = PLANNED
	t.BlockedReason = NO_REASON
	t.UpdatedAt = time.Now()
	return nil
}

// RequeueWithoutSpending returns a Task to PLANNED without spending an attempt.
// It is used when a retry reproduced the same outcome (no progress): the task
// stays runnable, but a repeat that changed nothing does not consume the budget.
func (t *Task) RequeueWithoutSpending() error {
	if err := t.canRequeue(); err != nil {
		return err
	}
	t.Status = PLANNED
	t.BlockedReason = NO_REASON
	t.UpdatedAt = time.Now()
	return nil
}

// canRequeue rejects requeuing a completed task.
func (t *Task) canRequeue() error {
	switch t.Status {
	case DONE, LOCAL_DONE, MERGED:
		return fmt.Errorf("cannot requeue a completed task")
	}
	return nil
}

func (t *Task) IsReady() bool {
	return t.Status == READY
}

func (t *Task) IsBlocked() bool {
	return t.Status == BLOCKED
}

func (t *Task) IsDone() bool {
	return t.Status == DONE
}

// IsSatisfied reports whether the task has reached a satisfied terminal state:
// its work is finished locally (LOCAL_DONE), integrated into shared history
// (MERGED), or fully done (DONE). It is the single definition of "complete",
// shared by dependency resolution, the run summary, and completed-plan handoff.
func (t *Task) IsSatisfied() bool {
	switch t.Status {
	case MERGED, DONE, LOCAL_DONE:
		return true
	}
	return false
}

// IsRunnable reports whether the task is in the one state `sop run` may pick up
// and execute: PLANNED. A PLANNED task still has to satisfy its dependencies
// (see ResolveDependencies) before the scheduler will promote it. Every other
// status is either complete, in-flight elsewhere, or BLOCKED, and so is not
// runnable. It is the single definition of "may be selected for execution".
func (t *Task) IsRunnable() bool {
	return t.Status == PLANNED
}

// IsBlockedRecoverable reports whether a BLOCKED task may be returned to PLANNED
// by the existing requeue path rather than being terminally stuck. It is the
// single definition of "BLOCKED is not permanent": a task is recoverable while
// it is BLOCKED, is not completed, and still has retry budget left. Once the
// budget is spent the task is terminally exhausted and Requeue returns
// ErrRetryExhausted. A task that is not BLOCKED is not recoverable in this sense.
func (t *Task) IsBlockedRecoverable() bool {
	if t.Status != BLOCKED {
		return false
	}
	if t.MaxAttempts > 0 && t.Attempt >= t.MaxAttempts {
		return false
	}
	return true
}

// IsTerminalBlocked reports whether a BLOCKED task has spent its retry budget, so
// it can no longer be returned to PLANNED by requeue. It is the complement of
// IsBlockedRecoverable for BLOCKED tasks.
func (t *Task) IsTerminalBlocked() bool {
	return t.Status == BLOCKED && !t.IsBlockedRecoverable()
}

// AllSatisfied reports whether tasks is non-empty and every task has reached a
// satisfied terminal state. It is the plan-completion predicate: an empty plan
// is not complete, and so is not eligible for completed-plan handoff.
func AllSatisfied(tasks []*Task) bool {
	if len(tasks) == 0 {
		return false
	}
	for _, task := range tasks {
		if !task.IsSatisfied() {
			return false
		}
	}
	return true
}

func (t *Task) AddAttempt(status TaskStatus, reason string) error {
	t.Attempt++
	attempt := Attempt{
		Number:    t.Attempt,
		Status:    status,
		Reason:    reason,
		Timestamp: time.Now(),
	}
	t.Attempts = append(t.Attempts, attempt)
	return nil
}

func (t *Task) ResolveDependencies(tasks map[string]*Task) (unmet []Dependency, satisfied bool) {
	unmet = []Dependency{}

	for _, depID := range t.DependencyIDs {
		depTask, ok := tasks[depID]
		if !ok {
			unmet = append(unmet, Dependency{TaskID: depID, RequiredStatus: DONE})
			continue
		}

		if !t.dependencySatisfied(depTask) {
			unmet = append(unmet, Dependency{
				TaskID:         depID,
				RequiredStatus: DONE,
			})
		}
	}

	return unmet, len(unmet) == 0
}

func (t *Task) dependencySatisfied(depTask *Task) bool {
	// A dependency is complete once its work is finished: locally (LOCAL_DONE) or
	// integrated into shared history (MERGED/DONE). Work that has merely passed
	// local tests, review, or CI may still live on an unmerged branch. BLOCKED and
	// in-flight work never satisfy a dependency.
	return depTask.IsSatisfied()
}
