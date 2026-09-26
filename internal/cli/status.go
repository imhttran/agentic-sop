package cli

import (
	"fmt"
	"io"

	"github.com/imhttran/agentic-sop/internal/store"
)

// notInitializedMsg is shown by read commands when project state is missing.
const notInitializedMsg = "project is not initialized; run `sop init`"

// runStatus lists persisted tasks. It never creates state: the database must
// already exist.
func runStatus(args []string, stdout, stderr io.Writer, getwd func() (string, error)) int {
	if len(args) != 0 {
		fmt.Fprintln(stderr, "usage: sop status")
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
		fmt.Fprintf(stderr, "status: %v\n", err)
		return exitError
	}
	defer st.Close()

	tasks, err := st.List()
	if err != nil {
		fmt.Fprintf(stderr, "status: %v\n", err)
		return exitError
	}

	for _, task := range tasks {
		fmt.Fprintf(stdout, "%s %s %s\n", task.ID, task.Status, task.Title)
	}
	return exitOK
}

// requireState reports whether the state database exists, printing a helpful
// message when it does not. It distinguishes a missing database (project not
// initialized) from an unexpected filesystem error, and returns false when the
// caller should exit with a runtime failure.
func requireState(path string, stderr io.Writer) bool {
	found, err := exists(path)
	switch {
	case err != nil:
		fmt.Fprintf(stderr, "cannot access state database: %v\n", err)
		return false
	case !found:
		fmt.Fprintln(stderr, notInitializedMsg)
		return false
	default:
		return true
	}
}
