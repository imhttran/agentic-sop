// Package parallel runs dependency-independent tasks concurrently, each in its
// own isolated working directory, bounded by a maximum. It is control-plane and
// deterministic: no model decides what runs in parallel.
package parallel

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/imhttran/agentic-sdlc/internal/domain"
	"github.com/imhttran/agentic-sdlc/internal/git"
)

// Workspaces provisions an isolated working directory per task (for example a
// Git worktree) so parallel tasks never share a directory.
type Workspaces interface {
	Create(ctx context.Context, task *domain.Task) (dir string, err error)
	Remove(ctx context.Context, task *domain.Task) error
}

// Executor runs one task to completion in an isolated working directory.
type Executor interface {
	Execute(ctx context.Context, task *domain.Task, dir string) error
}

// Store loads task state.
type Store interface {
	List() ([]*domain.Task, error)
}

// Result reports the tasks attempted and any per-task error.
type Result struct {
	TaskIDs []string
	Errors  map[string]error
}

// Failed reports whether any task failed.
func (r Result) Failed() bool { return len(r.Errors) > 0 }

// Runner executes independent tasks concurrently.
type Runner struct {
	store      Store
	executor   Executor
	workspaces Workspaces
	max        int
}

// New returns a Runner. max is coerced to at least 1.
func New(store Store, executor Executor, workspaces Workspaces, max int) *Runner {
	if max < 1 {
		max = 1
	}
	return &Runner{store: store, executor: executor, workspaces: workspaces, max: max}
}

// Run selects up to max independent tasks and runs each in its own workspace,
// concurrently but bounded. Every workspace is removed afterwards, even on
// failure or cancellation.
func (r *Runner) Run(ctx context.Context) (Result, error) {
	tasks, err := r.store.List()
	if err != nil {
		return Result{Errors: map[string]error{}}, err
	}

	selected := Select(tasks, r.max)
	result := Result{TaskIDs: taskIDs(selected), Errors: map[string]error{}}
	if len(selected) == 0 {
		return result, ctx.Err()
	}

	sem := make(chan struct{}, r.max)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, task := range selected {
		wg.Add(1)
		go func(task *domain.Task) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			dir, err := r.workspaces.Create(ctx, task)
			if err != nil {
				mu.Lock()
				result.Errors[task.ID] = err
				mu.Unlock()
				return
			}

			execErr := r.executor.Execute(ctx, task, dir)
			// Clean up even when the context is canceled, so no worktree leaks.
			if removeErr := r.workspaces.Remove(context.WithoutCancel(ctx), task); removeErr != nil && execErr == nil {
				execErr = removeErr
			}
			if execErr != nil {
				mu.Lock()
				result.Errors[task.ID] = execErr
				mu.Unlock()
			}
		}(task)
	}
	wg.Wait()

	return result, ctx.Err()
}

// Select returns up to max runnable tasks, deterministically ordered by id. A
// runnable task is PLANNED or READY with all dependencies complete. Such tasks
// are mutually independent: if B depended on A, B could not be runnable while A
// is still unfinished, so inter-dependent tasks can never be co-selected.
func Select(tasks []*domain.Task, max int) []*domain.Task {
	if max < 1 {
		return nil
	}

	byID := make(map[string]*domain.Task, len(tasks))
	for _, task := range tasks {
		byID[task.ID] = task
	}

	var candidates []*domain.Task
	for _, task := range tasks {
		if task.Status != domain.PLANNED && task.Status != domain.READY {
			continue
		}
		if _, satisfied := task.ResolveDependencies(byID); !satisfied {
			continue
		}
		candidates = append(candidates, task)
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })

	if len(candidates) > max {
		candidates = candidates[:max]
	}
	return candidates
}

func taskIDs(tasks []*domain.Task) []string {
	ids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	return ids
}

// GitWorktrees is the narrow Git slice needed for isolated workspaces.
type GitWorktrees interface {
	AddWorktree(ctx context.Context, path, branch string) error
	RemoveWorktree(ctx context.Context, path string) error
}

// GitWorkspaces provisions one Git worktree per task under root, so parallel
// tasks never share a working directory and cannot corrupt each other's Git
// state.
type GitWorkspaces struct {
	repo GitWorktrees
	root string
}

// NewGitWorkspaces returns a Workspaces backed by Git worktrees under root.
func NewGitWorkspaces(repo GitWorktrees, root string) *GitWorkspaces {
	return &GitWorkspaces{repo: repo, root: root}
}

// Create adds a worktree for the task's branch and returns its directory.
func (w *GitWorkspaces) Create(ctx context.Context, task *domain.Task) (string, error) {
	dir, err := w.dir(task)
	if err != nil {
		return "", err
	}
	branch, err := git.TaskBranchName(task.ID, task.Title)
	if err != nil {
		return "", err
	}
	if err := w.repo.AddWorktree(ctx, dir, branch); err != nil {
		return "", err
	}
	return dir, nil
}

// Remove removes the task's worktree.
func (w *GitWorkspaces) Remove(ctx context.Context, task *domain.Task) error {
	dir, err := w.dir(task)
	if err != nil {
		return err
	}
	return w.repo.RemoveWorktree(ctx, dir)
}

// dir resolves the task's worktree directory, rejecting a task id that is not a
// single safe path element. Task ids originate in a plan and must never be able
// to escape the workspace root.
func (w *GitWorkspaces) dir(task *domain.Task) (string, error) {
	id := task.ID
	if strings.TrimSpace(id) == "" || id == "." || id == ".." || strings.ContainsAny(id, "/\\") {
		return "", fmt.Errorf("parallel: unsafe task id %q for a worktree path", id)
	}
	return filepath.Join(w.root, id), nil
}
