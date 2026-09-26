package cli

import (
	"fmt"
	"io"

	"github.com/imhttran/agentic-sop/internal/store"
)

// runTask prints the persisted details of a single task. Like status, it never
// creates state.
func runTask(args []string, stdout, stderr io.Writer, getwd func() (string, error)) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: sop task <task-id>")
		return exitUsage
	}
	id := args[0]

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
		fmt.Fprintf(stderr, "task: %v\n", err)
		return exitError
	}
	defer st.Close()

	task, err := st.Get(id)
	if store.IsNotFound(err) {
		fmt.Fprintf(stderr, "task %s not found\n", id)
		return exitError
	}
	if err != nil {
		fmt.Fprintf(stderr, "task: %v\n", err)
		return exitError
	}

	fmt.Fprintf(stdout, "Task: %s\n", task.ID)
	fmt.Fprintf(stdout, "Title: %s\n", task.Title)
	fmt.Fprintf(stdout, "Status: %s\n", task.Status)
	fmt.Fprintf(stdout, "Attempts: %d\n", len(task.Attempts))
	if len(task.DependencyIDs) > 0 {
		fmt.Fprintln(stdout, "Dependencies:")
		for _, dep := range task.DependencyIDs {
			fmt.Fprintln(stdout, dep)
		}
	}
	return exitOK
}
