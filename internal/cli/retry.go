package cli

import (
	"fmt"
	"io"

	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/store"
)

// runRetry requeues a BLOCKED task to PLANNED so the next `sop run` retries it.
// It is the explicit, safe counterpart to the automatic requeue on a needs_human
// outcome: it refuses a completed task, only acts on a BLOCKED task, and preserves
// the task's history so the retry resumes the same task.
func runRetry(args []string, stdout, stderr io.Writer, getwd func() (string, error)) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: sop retry <task-id>")
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

	id := args[0]
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

	// Stage the change on a copy and save once, so a failed save cannot leave
	// in-memory state that was never persisted.
	staged := *task
	if err := staged.Requeue(); err != nil {
		fmt.Fprintf(stderr, "retry: %v\n", err)
		return exitError
	}
	if err := st.Save(&staged); err != nil {
		fmt.Fprintf(stderr, "retry: %v\n", err)
		return exitError
	}

	fmt.Fprintf(stdout, "requeued %s (BLOCKED -> PLANNED)\n", id)
	return exitOK
}
