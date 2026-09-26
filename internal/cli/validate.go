package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/testrunner"
	"github.com/imhttran/agentic-sop/internal/validate"
)

// runValidate executes the configured build/test/lint commands in the project
// directory and reports deterministic results. It is the harness's validation
// stage: no model decides whether a change compiles or passes.
func runValidate(args []string, stdout, stderr io.Writer, getwd func() (string, error)) int {
	if len(args) != 0 {
		fmt.Fprintln(stderr, "usage: sop validate")
		return exitUsage
	}

	dir, ok := projectDir(getwd, stderr)
	if !ok {
		return exitError
	}

	cfg, err := config.LoadDir(dir)
	if err != nil {
		if errors.Is(err, config.ErrNotFound) {
			fmt.Fprintln(stderr, "no configuration found; run `sop init` first")
			return exitError
		}
		fmt.Fprintf(stderr, "validate: %v\n", err)
		return exitError
	}

	if !validate.Enabled(cfg.Validation) {
		fmt.Fprintln(stdout, "no validation commands configured")
		return exitOK
	}

	result := validate.Run(context.Background(), dir, cfg.Validation)
	for _, r := range result.Results {
		fmt.Fprintf(stdout, "%-6s %-12s %s (%.2fs)\n", r.Status, r.Category, r.Command, r.Duration.Seconds())
		if r.Status != testrunner.Pass {
			if detail := strings.TrimSpace(r.Stderr); detail != "" {
				fmt.Fprintf(stdout, "\n%s\n", detail)
			}
			break
		}
	}

	if result.Passed() {
		fmt.Fprintln(stdout, "validation: PASS")
		return exitOK
	}
	fmt.Fprintln(stdout, "validation: FAIL")
	return exitError
}
