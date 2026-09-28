package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/store"
)

// runRetry requeues a BLOCKED task to PLANNED so the next `sop run` retries it,
// or with --all requeues every retryable BLOCKED task at once. It is the explicit,
// safe counterpart to the automatic requeue on a needs_human outcome: it never
// touches a completed task, only acts on BLOCKED tasks, and preserves each task's
// history so a retry resumes the same task.
func runRetry(args []string, stdout, stderr io.Writer, getwd func() (string, error)) int {
	all, id, ok := parseRetryArgs(args, stderr)
	if !ok {
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

	if all {
		return retryAll(st, stdout, stderr)
	}
	return retryOne(st, id, stdout, stderr)
}

// retryUsage is the one-line usage for `sop retry`.
const retryUsage = "usage: sop retry <task-id> | sop retry --all"

// parseRetryArgs accepts either a single task id or --all.
func parseRetryArgs(args []string, stderr io.Writer) (all bool, id string, ok bool) {
	switch {
	case len(args) == 1 && args[0] == "--all":
		return true, "", true
	case len(args) == 1 && !strings.HasPrefix(args[0], "-"):
		return false, args[0], true
	default:
		fmt.Fprintln(stderr, retryUsage)
		return false, "", false
	}
}

// retryOne requeues a single BLOCKED task.
func retryOne(st *store.Store, id string, stdout, stderr io.Writer) int {
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

	if err := requeuePersisted(st, task); err != nil {
		if errors.Is(err, domain.ErrRetryExhausted) {
			fmt.Fprintf(stderr, "retry: task %s %v; raise max_attempts or start fresh\n", id, err)
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
func retryAll(st *store.Store, stdout, stderr io.Writer) int {
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
		if err := requeuePersisted(st, task); err != nil {
			if errors.Is(err, domain.ErrRetryExhausted) {
				fmt.Fprintf(stderr, "retry: task %s %v; raise max_attempts or start fresh\n", task.ID, err)
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
func requeuePersisted(st *store.Store, task *domain.Task) error {
	staged := *task
	if err := staged.Requeue(); err != nil {
		return err
	}
	return st.Save(&staged)
}
