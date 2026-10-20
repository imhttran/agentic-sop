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
	"github.com/imhttran/agentic-sop/internal/projectdetect"
	"github.com/imhttran/agentic-sop/internal/store"
)

// runInit creates the project state directory and database. It is idempotent:
// re-running it never deletes or resets existing state and never overwrites an
// existing configuration. --refresh-validation is the explicit opt-in that
// replaces only the generated build/test/lint lists with the project's detected
// commands, preserving every other policy key.
func runInit(args []string, stdout, stderr io.Writer, getwd func() (string, error)) int {
	refreshValidation := false
	for _, arg := range args {
		switch arg {
		case "--refresh-validation":
			refreshValidation = true
		default:
			fmt.Fprintln(stderr, "usage: sop init [--refresh-validation]")
			return exitUsage
		}
	}

	dir, ok := projectDir(getwd, stderr)
	if !ok {
		return exitError
	}

	if refreshValidation {
		present, err := exists(config.Path(dir))
		if err != nil {
			fmt.Fprintf(stderr, "init: %v\n", err)
			return exitError
		}
		if present {
			detection := projectdetect.Detect(dir)
			if detection.Kind == projectdetect.KindUnknown || detection.Empty() {
				// Never replace an operator's commands with nothing.
				fmt.Fprintf(stderr, "init: no validation commands were detected for this project; .agent-sdlc/config.yaml was not changed\n")
				reportDetectionWarnings(stderr, detection)
				return exitError
			}
			if err := config.UpdateValidation(config.Path(dir), config.Validation{
				Build: detection.Validation.Build,
				Test:  detection.Validation.Test,
				Lint:  detection.Validation.Lint,
			}); err != nil {
				fmt.Fprintf(stderr, "init: %v\n", err)
				return exitError
			}
			fmt.Fprintf(stdout, "updated .agent-sdlc/config.yaml validation for %s (%s)\n", detection.Kind, detection.LanguageText())
			reportDetectionWarnings(stderr, detection)
			if err := ensureRuntimeIgnored(dir); err != nil {
				fmt.Fprintf(stderr, "init: %v\n", err)
				return exitError
			}
			return exitOK
		}
	}

	configWritten, warnings, err := ensureProjectInitialized(dir)
	if err != nil {
		fmt.Fprintf(stderr, "init: %v\n", err)
		return exitError
	}
	reportWarnings(stderr, warnings)

	fmt.Fprintln(stdout, "initialized .agent-sdlc/state.db")
	if configWritten {
		fmt.Fprintln(stdout, "wrote .agent-sdlc/config.yaml")
	}
	return exitOK
}

// ensureProjectInitialized creates the per-project state directory, migrates the
// database, and writes the configuration template when absent. It is idempotent
// and never overwrites existing state or a hand-edited configuration. It reports
// whether the configuration template was written and any detection warnings.
func ensureProjectInitialized(dir string) (configWritten bool, warnings []string, err error) {
	if err := os.MkdirAll(filepath.Join(dir, stateDirName), 0o755); err != nil {
		return false, nil, err
	}

	// Opening the store runs schema migration; existing data is preserved.
	st, err := store.Open(statePath(dir))
	if err != nil {
		return false, nil, err
	}
	if err := st.Close(); err != nil {
		return false, nil, err
	}

	return ensureConfigTemplate(dir)
}

// ensureConfigTemplate writes a configuration derived from the project actually
// present when none exists, and keeps SOP runtime state out of Git. It is the
// config-only part of project initialization, for a caller that needs the
// configuration but not the task store: a single `sop run --task` is an ordinary
// project run and must be able to verify its change, yet it has no scheduler
// graph to initialize. It is idempotent and never overwrites an existing
// configuration, so an operator's explicit validation policy always wins.
func ensureConfigTemplate(dir string) (configWritten bool, warnings []string, err error) {
	configPath := config.Path(dir)
	present, err := exists(configPath)
	if err != nil {
		return false, nil, err
	}
	if !present {
		if err := os.MkdirAll(filepath.Join(dir, stateDirName), 0o755); err != nil {
			return false, nil, err
		}
		template, detection := config.TemplateForDir(filepath.Base(dir), dir)
		if err := os.WriteFile(configPath, []byte(template), 0o644); err != nil {
			return false, nil, err
		}
		warnings = append(warnings, detection.Warnings...)
		warnings = append(warnings, detection.Conflicts...)
		configWritten = true
	}

	// SOP runtime state must not make an otherwise clean source tree dirty.
	if err := ensureRuntimeIgnored(dir); err != nil {
		return configWritten, warnings, err
	}
	return configWritten, warnings, nil
}

// reportDetectionWarnings prints the detection facts a bootstrap caller must see,
// so an unresolved conflict is reported before any command can run.
func reportDetectionWarnings(w io.Writer, detection projectdetect.Detection) {
	reportWarnings(w, append(append([]string{}, detection.Warnings...), detection.Conflicts...))
}

func reportWarnings(w io.Writer, warnings []string) {
	for _, warning := range warnings {
		fmt.Fprintf(w, "init: detection: %s\n", warning)
	}
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
