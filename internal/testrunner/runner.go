// Package testrunner executes project-defined verification commands and
// reports deterministic, structured results. It is language-independent: SOP
// hard-codes no target-language commands; projects supply the commands.
//
// Security boundary: configured verification commands are trusted project
// configuration (executable code), much like a Makefile. The runner must never
// construct commands from untrusted input such as model output, task
// descriptions, acceptance criteria, branch names, or PR text.
package testrunner

import (
	"context"
	"errors"
	"strings"
	"time"
)

// Category identifies a kind of verification check.
type Category string

const (
	Build           Category = "BUILD"
	UnitTest        Category = "UNIT_TEST"
	IntegrationTest Category = "INTEGRATION_TEST"
	Lint            Category = "LINT"
	DockerBuild     Category = "DOCKER_BUILD"
)

// Status is the deterministic classification of a check.
type Status string

const (
	// Pass: the command started and exited with code 0.
	Pass Status = "PASS"
	// Fail: the command started and exited with a non-zero code.
	Fail Status = "FAIL"
	// Error: the runner could not execute the check normally.
	Error Status = "ERROR"
	// Canceled: the supplied context was canceled or its deadline expired.
	Canceled Status = "CANCELED"
)

// noExitCode is the sentinel ExitCode for checks that never started.
const noExitCode = -1

// Check is a single verification command to run.
type Check struct {
	Category Category
	Command  string
}

// Result is the structured outcome of running one check.
type Result struct {
	Category Category
	Command  string
	ExitCode int
	Stdout   string
	Stderr   string
	Duration time.Duration
	Status   Status
}

// SuiteResult is the ordered outcome of running several checks.
type SuiteResult struct {
	Results []Result
	Status  Status
}

// Passed reports whether every executed check passed.
func (s SuiteResult) Passed() bool { return s.Status == Pass }

// Commands is a project's verification configuration. Absent (empty) commands
// are optional and simply omitted.
type Commands struct {
	Build           string
	UnitTest        string
	IntegrationTest string
	Lint            string
	DockerBuild     string
}

// Checks returns the configured checks in a deterministic order, omitting
// unconfigured (empty) commands. An absent command is not a failure.
func (c Commands) Checks() []Check {
	ordered := []struct {
		category Category
		command  string
	}{
		{Build, c.Build},
		{UnitTest, c.UnitTest},
		{IntegrationTest, c.IntegrationTest},
		{Lint, c.Lint},
		{DockerBuild, c.DockerBuild},
	}

	checks := make([]Check, 0, len(ordered))
	for _, o := range ordered {
		if strings.TrimSpace(o.command) != "" {
			checks = append(checks, Check{Category: o.category, Command: o.command})
		}
	}
	return checks
}

// Runner executes verification checks in an explicit project directory.
//
// A Runner is not safe for concurrent mutation: its fields are read as-is on
// each Run, so callers must not modify them while runs are in flight.
type Runner struct {
	// Dir is the project directory every command runs in.
	Dir string
	// Env optionally overrides the child environment.
	Env map[string]string
}

// New returns a Runner that executes checks in dir.
func New(dir string) *Runner {
	return &Runner{Dir: dir}
}

// Run executes a single check and classifies the result. It never returns an
// error: infrastructure failures are reported as Error results.
func (r *Runner) Run(ctx context.Context, check Check) Result {
	result := Result{
		Category: check.Category,
		Command:  check.Command,
		ExitCode: noExitCode,
	}

	if strings.TrimSpace(check.Command) == "" {
		result.Status = Error
		result.Stderr = "empty command"
		return result
	}

	start := time.Now()
	exitCode, stdout, stderr, err := execute(ctx, r.Dir, r.Env, check.Command)
	result.Duration = time.Since(start)
	result.ExitCode = exitCode
	result.Stdout = stdout
	result.Stderr = stderr

	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		result.Status = Canceled
	case err != nil:
		result.Status = Error
		if result.Stderr == "" {
			result.Stderr = err.Error()
		}
	case exitCode == 0:
		result.Status = Pass
	default:
		result.Status = Fail
	}
	return result
}

// RunAll runs checks in order and stops at the first non-passing check
// (fail-fast). It preserves the ordered individual results.
func (r *Runner) RunAll(ctx context.Context, checks []Check) SuiteResult {
	results := make([]Result, 0, len(checks))
	for _, check := range checks {
		result := r.Run(ctx, check)
		results = append(results, result)
		if result.Status != Pass {
			return SuiteResult{Results: results, Status: result.Status}
		}
	}
	return SuiteResult{Results: results, Status: Pass}
}
