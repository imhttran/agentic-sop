// Package resume recovers safely from process interruption. It reconciles a
// task's persisted status with the resources that actually exist (branch, pull
// request) and reports the next legal action, correcting a lost state write
// without duplicating the branch or PR. It is control-plane: no model decides
// what to do next.
package resume

import (
	"context"
	"fmt"

	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/github"
)

// Action is the next legal step for a task.
type Action string

const (
	// None: the task is complete or terminally blocked; nothing to resume.
	None Action = "NONE"
	// CreateBranch: the task is planned/ready and has no branch yet.
	CreateBranch Action = "CREATE_BRANCH"
	// WriteTests: the branch exists; write the failing tests.
	WriteTests Action = "WRITE_TESTS"
	// VerifyRed: tests are written; confirm they fail.
	VerifyRed Action = "VERIFY_RED"
	// Implement: implement (or re-implement after a fix) until tests pass.
	Implement Action = "IMPLEMENT"
	// Review: local tests pass; run the review loop.
	Review Action = "REVIEW"
	// OpenPR: review passed; open the pull request.
	OpenPR Action = "OPEN_PR"
	// PollCI: the PR is open; observe CI until it settles.
	PollCI Action = "POLL_CI"
	// Merge: CI passed; merge through the merge gate.
	Merge Action = "MERGE"
	// Finish: the PR is merged; refresh main and mark the task DONE.
	Finish Action = "FINISH"
)

// Observation is the externally observed state of a task's resources.
type Observation struct {
	// BranchExists reports whether the task's branch exists locally.
	BranchExists bool
	// PR is the task's pull request, or nil when none exists.
	PR *github.PullRequest
}

// Observer reports the resources a resume decision depends on.
type Observer interface {
	Observe(ctx context.Context, task *domain.Task) (Observation, error)
}

// Decision is the resolved next action, with any recovered status.
type Decision struct {
	TaskID    string
	Action    Action
	Status    domain.TaskStatus
	Recovered bool

	// path is the sequence of legal transitions used to recover a lost write.
	path []domain.TaskStatus
}

// Store is the persistence boundary resume needs.
type Store interface {
	List() ([]*domain.Task, error)
	Get(id string) (*domain.Task, error)
	Save(task *domain.Task) error
}

// Resumer determines the next legal action for interrupted work.
type Resumer struct {
	store    Store
	observer Observer
}

// New returns a Resumer backed by store and observer.
func New(store Store, observer Observer) *Resumer {
	return &Resumer{store: store, observer: observer}
}

// Resolve determines the next legal action for one task. When persisted state is
// inconsistent with observed resources it recovers to a consistent status and
// persists it before returning.
func (r *Resumer) Resolve(ctx context.Context, taskID string) (Decision, error) {
	if err := ctx.Err(); err != nil {
		return Decision{}, err
	}

	task, err := r.store.Get(taskID)
	if err != nil {
		return Decision{}, err
	}

	obs, err := r.observer.Observe(ctx, task)
	if err != nil {
		return Decision{}, err
	}

	decision, err := resolve(task, obs)
	if err != nil {
		return Decision{}, err
	}
	if len(decision.path) == 0 {
		return decision, nil
	}

	// Stage the recovery on a copy so a failed save changes nothing.
	staged := *task
	for _, status := range decision.path {
		if err := staged.Transition(status); err != nil {
			return Decision{}, fmt.Errorf("resume: apply recovery for %s: %w", task.ID, err)
		}
	}
	if err := r.store.Save(&staged); err != nil {
		return Decision{}, err
	}
	decision.Status = staged.Status
	return decision, nil
}

// ResolveActive resolves the single in-flight task. It reports None when there
// is nothing to resume and an error when several tasks are in flight (the caller
// must name one).
func (r *Resumer) ResolveActive(ctx context.Context) (Decision, error) {
	if err := ctx.Err(); err != nil {
		return Decision{}, err
	}

	tasks, err := r.store.List()
	if err != nil {
		return Decision{}, err
	}
	switch active := Active(tasks); len(active) {
	case 0:
		return Decision{Action: None}, nil
	case 1:
		return r.Resolve(ctx, active[0].ID)
	default:
		return Decision{}, fmt.Errorf("resume: %d in-flight tasks; specify a task id", len(active))
	}
}

// Active returns the tasks that may need resuming: everything that is neither
// planned nor terminal.
func Active(tasks []*domain.Task) []*domain.Task {
	var active []*domain.Task
	for _, task := range tasks {
		if task.Status != domain.PLANNED && !task.Status.IsTerminal() {
			active = append(active, task)
		}
	}
	return active
}

// actionFor maps a status to its next legal action.
var actionFor = map[domain.TaskStatus]Action{
	domain.PLANNED:          CreateBranch,
	domain.READY:            CreateBranch,
	domain.BRANCH_CREATED:   WriteTests,
	domain.TESTS_WRITTEN:    VerifyRed,
	domain.RED_VERIFIED:     Implement,
	domain.IMPLEMENTING:     Implement,
	domain.FIX_REQUIRED:     Implement,
	domain.LOCAL_TESTS_PASS: Review,
	domain.REVIEW:           Review,
	domain.REVIEW_PASS:      OpenPR,
	domain.PR_OPEN:          PollCI,
	domain.CI_RUNNING:       PollCI,
	domain.CI_PASS:          Merge,
	domain.MERGED:           Finish,
}

// ActionFor returns the next legal action for a task status, and whether the
// status has one. It is the single interpreted status→action decision, shared by
// `sop resume` (which also reconciles observed branch/PR resources) and by
// `sop run` when it resumes an interrupted task.
func ActionFor(status domain.TaskStatus) (Action, bool) {
	a, ok := actionFor[status]
	return a, ok
}

// needsBranch reports whether a status implies the task branch must already
// exist. A merged task's branch is normally deleted, so MERGED does not require
// one.
func needsBranch(status domain.TaskStatus) bool {
	switch status {
	case domain.PLANNED, domain.READY, domain.MERGED:
		return false
	}
	return !status.IsTerminal()
}

// needsPR reports whether a status implies the pull request must already exist.
func needsPR(status domain.TaskStatus) bool {
	switch status {
	case domain.PR_OPEN, domain.CI_RUNNING, domain.CI_PASS:
		return true
	default:
		return false
	}
}

// recoveryPath returns the transitions that reconcile a persisted status with
// observed resources, or nil when no recovery is needed. It never plans a
// duplicate branch or PR.
func recoveryPath(status domain.TaskStatus, obs Observation) []domain.TaskStatus {
	if obs.BranchExists {
		switch status {
		case domain.READY:
			return []domain.TaskStatus{domain.BRANCH_CREATED}
		case domain.PLANNED:
			return []domain.TaskStatus{domain.READY, domain.BRANCH_CREATED}
		}
	}
	if status == domain.REVIEW_PASS && obs.PR != nil {
		return []domain.TaskStatus{domain.PR_OPEN}
	}
	return nil
}

// resolve is the pure decision function: given a task and observed resources it
// returns the next action, a recovery path, or a consistency error.
func resolve(task *domain.Task, obs Observation) (Decision, error) {
	decision := Decision{TaskID: task.ID, Status: task.Status}

	if task.Status.IsTerminal() {
		decision.Action = None
		return decision, nil
	}

	if path := recoveryPath(task.Status, obs); len(path) > 0 {
		decision.Action = actionFor[path[len(path)-1]]
		decision.Recovered = true
		decision.path = path
		return decision, nil
	}

	if needsBranch(task.Status) && !obs.BranchExists {
		return Decision{}, fmt.Errorf("resume: task %s is %s but its branch does not exist", task.ID, task.Status)
	}
	if needsPR(task.Status) && obs.PR == nil {
		return Decision{}, fmt.Errorf("resume: task %s is %s but its pull request does not exist", task.ID, task.Status)
	}

	action, ok := actionFor[task.Status]
	if !ok {
		return Decision{}, fmt.Errorf("resume: task %s has unknown status %s", task.ID, task.Status)
	}
	decision.Action = action
	return decision, nil
}
