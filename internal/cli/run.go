package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/planner"
	"github.com/imhttran/agentic-sop/internal/quality"
	"github.com/imhttran/agentic-sop/internal/review"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/taskfile"
	"github.com/imhttran/agentic-sop/internal/testrunner"
	"github.com/imhttran/agentic-sop/internal/validate"
)

// runRun drives one task from a Markdown file through the local lifecycle:
//
//	task → plan → implement → detect changes
//	     → { validate → review → gate → fix } → report
//
// The braces are a bounded fix loop: a blocking finding is sent back to the
// agent, then validation and review run again, at most quality.max_fix_cycles
// times. Exhausting the budget yields NEEDS_HUMAN, never an unbounded loop. The
// run stops at the human gate and never commits, pushes, or merges.
func runRun(args []string, stdout, stderr io.Writer, d deps) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: sop run TASK.md")
		return exitUsage
	}

	dir, ok := projectDir(d.getwd, stderr)
	if !ok {
		return exitError
	}

	spec, err := loadTaskFile(dir, args[0])
	if err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitError
	}

	cfg := config.Default()
	if loaded, err := config.LoadDir(dir); err == nil {
		cfg = *loaded
	} else if !errors.Is(err, config.ErrNotFound) {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitError
	}

	rn, err := runpkg.New(dir, runID(spec))
	if err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitError
	}
	_ = rn.Write("task.md", spec.Render())

	ctx := context.Background()
	a, err := d.newAgent(cfg.Agent.Provider)
	if err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitError
	}

	// Plan (must not mutate the repository).
	_ = rn.SetStage(runpkg.Planning)
	plan, err := planner.New(a).Generate(ctx, spec.Render())
	if err != nil {
		return failRun(rn, stderr, "plan", err)
	}
	_ = rn.Write("plan.md", plan.RenderMarkdown())

	// Implement: the agent edits the repository; its returned summary is recorded
	// but Git remains the authority on what changed.
	_ = rn.SetStage(runpkg.Implementing)
	impl, err := a.Generate(ctx, agent.Request{
		Capability:         agent.Implement,
		Task:               spec.Render(),
		Input:              plan.RenderMarkdown(),
		OutputRequirements: "Implement the plan in the working tree and summarize the changes.",
	})
	if err != nil {
		return failRun(rn, stderr, "implement", err)
	}
	_ = rn.Write("implementation.md", impl.Content)

	diff, err := d.readDiff(ctx, dir)
	if err != nil {
		return failRun(rn, stderr, "diff", err)
	}
	_ = rn.Write("diff.patch", diff)
	if strings.TrimSpace(diff) == "" {
		_ = rn.SetStage(runpkg.Failed)
		fmt.Fprintln(stdout, "run: no changes were produced")
		return exitError
	}

	maxCycles := cfg.Quality.MaxFixCycles
	cycles := 0
	var suite testrunner.SuiteResult
	var report review.Report
	var gate quality.Result

	for {
		// Validate: deterministic, fail-fast. Uncompilable changes never reach review.
		_ = rn.SetStage(runpkg.Validating)
		suite = validate.Run(ctx, dir, cfg.Validation)

		// Review only when validation passed.
		report = review.Report{}
		if suite.Passed() {
			_ = rn.SetStage(runpkg.Reviewing)
			provider, err := reviewProvider(cfg, d)
			if err != nil {
				return failRun(rn, stderr, "review", err)
			}
			report, err = provider.Review(ctx, review.Request{Task: spec.Render(), Diff: diff})
			if err != nil {
				return failRun(rn, stderr, "review", err)
			}
		}

		// Deterministic verdict; the fix budget turns an exhausted loop into a
		// human decision instead of looping forever.
		gate = quality.Evaluate(cfg.Quality, quality.Input{
			BuildPassed:  categoryPassed(suite, testrunner.Build),
			TestPassed:   categoryPassed(suite, testrunner.UnitTest),
			LintRequired: hasCategory(suite, testrunner.Lint),
			LintPassed:   categoryPassed(suite, testrunner.Lint),
			Unresolved:   report.Findings,
			FixCycles:    cycles,
		})

		actionable := quality.BlockingFindings(cfg.Quality.FailOn, report.Findings)
		if gate.Decision != quality.Fail || actionable == 0 || cycles >= maxCycles {
			break
		}

		// Fix, then re-validate and re-review (regression protection).
		_ = rn.SetStage(runpkg.Fixing)
		cycles++
		fix, err := a.Generate(ctx, agent.Request{
			Capability:         agent.Fix,
			Task:               spec.Render(),
			Input:              fixContext(plan.RenderMarkdown(), report, diff),
			OutputRequirements: "Fix the blocking findings in the working tree and summarize the changes.",
		})
		if err != nil {
			return failRun(rn, stderr, "fix", err)
		}
		_ = rn.Write(fmt.Sprintf("fix-%d.md", cycles), fix.Content)

		diff, err = d.readDiff(ctx, dir)
		if err != nil {
			return failRun(rn, stderr, "diff", err)
		}
		if strings.TrimSpace(diff) == "" {
			_ = rn.SetStage(runpkg.Failed)
			fmt.Fprintln(stdout, "run: fixes removed all changes")
			return exitError
		}
		_ = rn.Write("diff.patch", diff)
	}

	writeRunJSON(rn, "validation.json", suite)
	writeRunJSON(rn, "review.json", report)

	stage := runpkg.Passed
	switch gate.Decision {
	case quality.Fail:
		stage = runpkg.Failed
	case quality.NeedsHuman:
		stage = runpkg.WaitingForHuman
	}
	_ = rn.SetStage(stage)

	_ = rn.Write("report.md", buildRunReport(spec, cfg, suite, report, gate, stage, cycles))
	writeRunJSON(rn, "report.json", runReportDoc{
		ID:          rn.State().ID,
		Stage:       stage,
		Provider:    cfg.Agent.Provider,
		Engine:      cfg.Review.Engine,
		Decision:    gate.Decision,
		Reasons:     gate.Reasons,
		FixCycles:   cycles,
		Validation:  suite.Results,
		Findings:    report.Findings,
		GeneratedAt: time.Now().UTC(),
	})

	fmt.Fprintf(stdout, "run %s: %s\n", rn.State().ID, gate.Decision)
	for _, reason := range gate.Reasons {
		fmt.Fprintf(stdout, "  - %s\n", reason)
	}
	fmt.Fprintf(stdout, "fix cycles: %d/%d\n", cycles, maxCycles)
	if rel := relDir(dir, rn.Dir()); rel != "" {
		fmt.Fprintf(stdout, "report: %s/report.md\n", rel)
	}
	if gate.Decision == quality.Pass {
		fmt.Fprintln(stdout, "awaiting human approval; no commit was performed")
		return exitOK
	}
	return exitError
}

// failRun marks the run failed, reports the stage error, and returns the error
// exit code.
func failRun(rn *runpkg.Run, stderr io.Writer, stage string, err error) int {
	_ = rn.SetStage(runpkg.Failed)
	fmt.Fprintf(stderr, "run: %s: %v\n", stage, err)
	return exitError
}

// fixContext renders the bounded context a fix is given: the plan, the blocking
// findings, and the current diff.
func fixContext(plan string, report review.Report, diff string) string {
	var b strings.Builder
	b.WriteString("# Plan\n\n")
	b.WriteString(strings.TrimSpace(plan))
	b.WriteString("\n\n# Blocking findings\n\n")
	for _, f := range report.Findings {
		location := f.File
		if f.Line > 0 {
			location = fmt.Sprintf("%s:%d", f.File, f.Line)
		}
		fmt.Fprintf(&b, "- %s %s — %s: %s\n", f.Severity, location, f.Title, f.Detail)
	}
	b.WriteString("\n# Current diff\n\n")
	b.WriteString(diff)
	return b.String()
}

// loadTaskFile reads a task file, resolving a relative path against the project
// directory.
func loadTaskFile(dir, arg string) (*taskfile.Spec, error) {
	path := arg
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, arg)
	}
	return taskfile.Load(path)
}

// runID derives a stable run id from the task file, falling back to a timestamp
// when the file has no id.
func runID(spec *taskfile.Spec) string {
	if id := strings.TrimSpace(spec.ID); id != "" {
		return id
	}
	return "run-" + time.Now().UTC().Format("20060102-150405")
}

// categoryPassed reports whether a category's check passed. A category that did
// not run (not configured, or not reached due to fail-fast) is not treated as a
// failure here; a build failure is what stops the suite.
func categoryPassed(suite testrunner.SuiteResult, category testrunner.Category) bool {
	for _, r := range suite.Results {
		if r.Category == category {
			return r.Status == testrunner.Pass
		}
	}
	return true
}

// hasCategory reports whether the suite contained a check of the category.
func hasCategory(suite testrunner.SuiteResult, category testrunner.Category) bool {
	for _, r := range suite.Results {
		if r.Category == category {
			return true
		}
	}
	return false
}

// writeRunJSON writes a JSON artifact best-effort; artifacts never change the
// run's outcome.
func writeRunJSON(rn *runpkg.Run, name string, v any) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return
	}
	_ = rn.Write(name, string(append(data, '\n')))
}

// relDir returns target relative to base when possible, else target.
func relDir(base, target string) string {
	if rel, err := filepath.Rel(base, target); err == nil {
		return rel
	}
	return target
}

// runReportDoc is the machine-readable run report.
type runReportDoc struct {
	ID          string              `json:"id"`
	Stage       runpkg.Stage        `json:"stage"`
	Provider    string              `json:"provider"`
	Engine      string              `json:"review_engine"`
	Decision    quality.Decision    `json:"decision"`
	Reasons     []string            `json:"reasons"`
	FixCycles   int                 `json:"fix_cycles"`
	Validation  []testrunner.Result `json:"validation"`
	Findings    []review.Finding    `json:"findings"`
	GeneratedAt time.Time           `json:"generated_at"`
}

// buildRunReport renders the human-readable report.
func buildRunReport(spec *taskfile.Spec, cfg config.Config, suite testrunner.SuiteResult, report review.Report, gate quality.Result, stage runpkg.Stage, cycles int) string {
	var b strings.Builder
	heading := strings.TrimSpace(strings.Trim(strings.Join([]string{spec.ID, spec.Title}, " — "), " —"))
	fmt.Fprintf(&b, "# Run: %s\n\n", heading)
	fmt.Fprintf(&b, "- Provider: `%s`\n", cfg.Agent.Provider)
	fmt.Fprintf(&b, "- Review engine: `%s`\n", cfg.Review.Engine)
	fmt.Fprintf(&b, "- Stage: `%s`\n", stage)
	fmt.Fprintf(&b, "- Fix cycles: %d/%d\n", cycles, cfg.Quality.MaxFixCycles)
	fmt.Fprintf(&b, "- Gate: `%s`\n\n", gate.Decision)

	b.WriteString("## Task\n\n")
	b.WriteString(strings.TrimSpace(spec.Render()))
	b.WriteString("\n\n## Validation\n\n")
	if len(suite.Results) == 0 {
		b.WriteString("No validation commands configured.\n")
	} else {
		for _, r := range suite.Results {
			fmt.Fprintf(&b, "- %s %s `%s`\n", r.Status, r.Category, r.Command)
		}
	}

	b.WriteString("\n## Review\n\n")
	if len(report.Findings) == 0 {
		if s := strings.TrimSpace(report.Summary); s != "" {
			b.WriteString(s + "\n")
		} else {
			b.WriteString("No findings.\n")
		}
	} else {
		for _, f := range report.Findings {
			location := f.File
			if f.Line > 0 {
				location = fmt.Sprintf("%s:%d", f.File, f.Line)
			}
			fmt.Fprintf(&b, "- %s %s — %s\n", f.Severity, location, f.Title)
		}
	}

	b.WriteString("\n## Gate\n\n")
	for _, reason := range gate.Reasons {
		fmt.Fprintf(&b, "- %s\n", reason)
	}
	return b.String()
}
