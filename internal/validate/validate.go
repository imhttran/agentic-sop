// Package validate is the harness's validation stage: it runs a project's
// configured verification commands in a deterministic order and returns
// structured results. No model decides whether a change compiles or passes.
package validate

import (
	"context"
	"strings"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/testrunner"
)

// Checks maps the configured validation commands to ordered checks: build,
// then test, then lint, preserving each list's order and omitting blanks.
func Checks(v config.Validation) []testrunner.Check {
	var checks []testrunner.Check
	add := func(category testrunner.Category, commands []string) {
		for _, command := range commands {
			if strings.TrimSpace(command) != "" {
				checks = append(checks, testrunner.Check{Category: category, Command: command})
			}
		}
	}
	add(testrunner.Build, v.Build)
	add(testrunner.UnitTest, v.Test)
	add(testrunner.Lint, v.Lint)
	return checks
}

// Run executes the configured checks in dir. It fails fast: the first
// non-passing check stops the suite, so uncompilable changes are not carried
// forward to expensive review.
func Run(ctx context.Context, dir string, v config.Validation) testrunner.SuiteResult {
	return testrunner.New(dir).RunAll(ctx, Checks(v))
}

// Enabled reports whether any validation command is configured.
func Enabled(v config.Validation) bool {
	return len(Checks(v)) > 0
}
