package domain

import (
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

// Requeue returns a Task to PLANNED so the scheduler may select it again. It is
// the inverse of Block: a human boundary (NEEDS_HUMAN) is not terminal, so the
// work is not lost and a later run retries it. Like Block, it is an explicit
// domain operation rather than a Transition, and a completed Task cannot be
// requeued.
func (t *Task) Requeue() error {
	switch t.Status {
	case DONE, LOCAL_DONE, MERGED:
		return fmt.Errorf("cannot requeue a completed task")
	}
	t.Status = PLANNED
	t.BlockedReason = NO_REASON
	t.UpdatedAt = time.Now()
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

func (t *Task) CanRetry() bool {
	return t.Attempt < t.MaxAttempts
}

func (t *Task) HasRetries() bool {
	return t.MaxAttempts > 1
}

func (t *Task) CurrentAttempt() int {
	return t.Attempt
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
	// local tests, review, or CI may still live on an unmerged branch.
	return depTask.Status == MERGED || depTask.Status == DONE || depTask.Status == LOCAL_DONE
}
