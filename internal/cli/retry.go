package cli

import (
	"errors"
	"fmt"
	"io"

	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/store"
)

// runRetry requeues a BLOCKED task to PLANNED so the next `sop run` retries it,
// or with --all requeues every retryable BLOCKED task at once. It is the explicit,
// safe counterpart to the automatic requeue on a needs_human outcome: it never
// touches a completed task, only acts on BLOCKED tasks, and preserves each task's
// history so a retry resumes the same task.
//
// --force raises an exhausted retry budget so a task that hit its max_attempts can
// be retried again, without hand-editing state or starting fresh.
func runRetry(args []string, stdout, stderr io.Writer, getwd func() (string, error)) int {
	id := ""
	force := false
	for _, a := range args {
		switch {
		case a == "--force" || a == "-f":
			force = true
		case id == "":
			id = a
		default:
			fmt.Fprintln(stderr, "usage: sop retry <task-id> | sop retry --all [--force]")
			return exitUsage
		}
	}
	if id == "" {
		fmt.Fprintln(stderr, "usage: sop retry <task-id> | sop retry --all [--force]")
		return exitUsage
	}

	dir, ok := projectDir(getwd, stderr)
	if !ok {
		return exitError
	}

	path := statePath(dir)
	if !requireState(path, stderr) {
		return exitError
	}

	st, err := store.Open(path)
	if err != nil {
		fmt.Fprintf(stderr, "retry: %v\n", err)
		return exitError
	}
	defer st.Close()

	if id == "--all" {
		return retryAll(st, stdout, stderr, force)
	}
	return retryOne(st, id, stdout, stderr, force)
}

// retryOne requeues a single BLOCKED task.
func retryOne(st *store.Store, id string, stdout, stderr io.Writer, force bool) int {
	task, err := st.Get(id)
	if err != nil {
		fmt.Fprintf(stderr, "retry: %v\n", err)
		return exitError
	}

	switch task.Status {
	case domain.DONE, domain.LOCAL_DONE, domain.MERGED:
		fmt.Fprintf(stderr, "retry: task %s is %s; nothing to retry\n", id, task.Status)
		return exitError
	case domain.BLOCKED:
	default:
		fmt.Fprintf(stderr, "retry: task %s is %s, not BLOCKED; nothing to retry\n", id, task.Status)
		return exitError
	}

	if err := requeuePersisted(st, task, force); err != nil {
		if errors.Is(err, domain.ErrRetryExhausted) {
			fmt.Fprintf(stderr, "retry: task %s %v; raise max_attempts with --force or start fresh\n", id, err)
			return exitError
		}
		fmt.Fprintf(stderr, "retry: %v\n", err)
		return exitError
	}

	fmt.Fprintf(stdout, "requeued %s (BLOCKED -> PLANNED)\n", id)
	return exitOK
}

// retryAll requeues every BLOCKED task with retry budget remaining. A task whose
// budget is spent is reported and left BLOCKED rather than silently skipped.
func retryAll(st *store.Store, stdout, stderr io.Writer, force bool) int {
	tasks, err := st.List()
	if err != nil {
		fmt.Fprintf(stderr, "retry: %v\n", err)
		return exitError
	}

	requeued := 0
	for _, task := range tasks {
		if task.Status != domain.BLOCKED {
			continue
		}
		if err := requeuePersisted(st, task, force); err != nil {
			if errors.Is(err, domain.ErrRetryExhausted) {
				fmt.Fprintf(stderr, "retry: task %s %v; raise max_attempts with --force or start fresh\n", task.ID, err)
				continue
			}
			fmt.Fprintf(stderr, "retry: %v\n", err)
			return exitError
		}
		fmt.Fprintf(stdout, "requeued %s (BLOCKED -> PLANNED)\n", task.ID)
		requeued++
	}

	if requeued == 0 {
		fmt.Fprintln(stdout, "no BLOCKED tasks to retry")
	} else {
		fmt.Fprintf(stdout, "requeued %d task(s)\n", requeued)
	}
	return exitOK
}

// requeuePersisted returns a BLOCKED task to PLANNED on a copy and saves once, so
// a failed save cannot leave in-memory state that was never persisted. It returns
// the task's requeue error (including ErrRetryExhausted) so callers can report it.
// When force is set and the retry budget is spent, the budget is raised before the
// requeue, so an operator can deliberately retry past max_attempts.
func requeuePersisted(st *store.Store, task *domain.Task, force bool) error {
	staged := *task
	if err := staged.Requeue(); err != nil {
		if !force || !errors.Is(err, domain.ErrRetryExhausted) {
			return err
		}
		staged.MaxAttempts = staged.Attempt + domain.DefaultRetryPolicy().MaxAttempts
		if err := staged.Requeue(); err != nil {
			return err
		}
	}
	return st.Save(&staged)
}
