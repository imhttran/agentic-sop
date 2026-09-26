package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/imhttran/agentic-sdlc/internal/store"
)

// runInit creates the project state directory and database. It is idempotent:
// re-running it never deletes or resets existing state.
func runInit(args []string, stdout, stderr io.Writer, getwd func() (string, error)) int {
	if len(args) != 0 {
		fmt.Fprintln(stderr, "usage: sop init")
		return exitUsage
	}

	dir, ok := projectDir(getwd, stderr)
	if !ok {
		return exitError
	}

	if err := os.MkdirAll(filepath.Join(dir, stateDirName), 0o755); err != nil {
		fmt.Fprintf(stderr, "init: %v\n", err)
		return exitError
	}

	// Opening the store runs schema migration; existing data is preserved.
	st, err := store.Open(statePath(dir))
	if err != nil {
		fmt.Fprintf(stderr, "init: %v\n", err)
		return exitError
	}
	if err := st.Close(); err != nil {
		fmt.Fprintf(stderr, "init: %v\n", err)
		return exitError
	}

	fmt.Fprintln(stdout, "initialized .agent-sdlc/state.db")
	return exitOK
}
