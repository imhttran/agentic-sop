package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/store"
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

	configWritten, err := ensureProjectInitialized(dir)
	if err != nil {
		fmt.Fprintf(stderr, "init: %v\n", err)
		return exitError
	}

	fmt.Fprintln(stdout, "initialized .agent-sdlc/state.db")
	if configWritten {
		fmt.Fprintln(stdout, "wrote .agent-sdlc/config.yaml")
	}
	return exitOK
}

// ensureProjectInitialized creates the per-project state directory, migrates the
// database, and writes the configuration template when absent. It is idempotent
// and never overwrites existing state or a hand-edited configuration. It reports
// whether the configuration template was written.
func ensureProjectInitialized(dir string) (configWritten bool, err error) {
	if err := os.MkdirAll(filepath.Join(dir, stateDirName), 0o755); err != nil {
		return false, err
	}

	// Opening the store runs schema migration; existing data is preserved.
	st, err := store.Open(statePath(dir))
	if err != nil {
		return false, err
	}
	if err := st.Close(); err != nil {
		return false, err
	}

	configPath := config.Path(dir)
	present, err := exists(configPath)
	if err != nil {
		return false, err
	}
	configWritten = false
	if !present {
		if err := os.WriteFile(configPath, []byte(config.Template(filepath.Base(dir))), 0o644); err != nil {
			return false, err
		}
		configWritten = true
	}

	// SOP runtime state must not make an otherwise clean source tree dirty.
	if err := ensureRuntimeIgnored(dir); err != nil {
		return configWritten, err
	}
	return configWritten, nil
}

// ensureRuntimeIgnored makes SOP-owned runtime state invisible to Git without
// touching the project's own .gitignore: a .agent-sdlc/.gitignore containing "*"
// ignores the whole state directory (including itself). It is idempotent.
func ensureRuntimeIgnored(dir string) error {
	path := filepath.Join(dir, stateDirName, ".gitignore")
	if data, err := os.ReadFile(path); err == nil {
		if strings.TrimSpace(string(data)) == "*" {
			return nil
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return os.WriteFile(path, []byte("*\n"), 0o644)
}
