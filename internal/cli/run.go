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
//	task → plan → implement → detect changes → validate → review → quality gate → report
//
// It stops at the human gate. It never commits, pushes, or merges. Every stage
// writes an artifact under .agent-sdlc/runs/<id>/ so the run stays inspectable.
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

	// Detect changes: Git is authoritative, never the agent's summary.
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

	// Validate: deterministic, fail-fast. Uncompilable changes never reach review.
	_ = rn.SetStage(runpkg.Validating)
	suite := validate.Run(ctx, dir, cfg.Validation)
	writeRunJSON(rn, "validation.json", suite)

	// Review only when validation passed.
	var report review.Report
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
	writeRunJSON(rn, "review.json", report)

	// Quality gate: deterministic verdict from the evidence, not the model.
	gate := quality.Evaluate(cfg.Quality, quality.Input{
		BuildPassed:  categoryPassed(suite, testrunner.Build),
		TestPassed:   categoryPassed(suite, testrunner.UnitTest),
		LintRequired: hasCategory(suite, testrunner.Lint),
		LintPassed:   categoryPassed(suite, testrunner.Lint),
		Unresolved:   report.Findings,
	})

	stage := runpkg.Passed
	switch gate.Decision {
	case quality.Fail:
		stage = runpkg.Failed
	case quality.NeedsHuman:
		stage = runpkg.WaitingForHuman
	}
	_ = rn.SetStage(stage)

	_ = rn.Write("report.md", buildRunReport(spec, cfg, suite, report, gate, stage))
	writeRunJSON(rn, "report.json", runReportDoc{
		ID:          rn.State().ID,
		Stage:       stage,
		Provider:    cfg.Agent.Provider,
		Engine:      cfg.Review.Engine,
		Decision:    gate.Decision,
		Reasons:     gate.Reasons,
		Validation:  suite.Results,
		Findings:    report.Findings,
		GeneratedAt: time.Now().UTC(),
	})

	fmt.Fprintf(stdout, "run %s: %s\n", rn.State().ID, gate.Decision)
	for _, reason := range gate.Reasons {
		fmt.Fprintf(stdout, "  - %s\n", reason)
	}
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
// not run (not configured, or not reached due to fail-fast) is treated as not
// failing here; a build failure is what stops the suite.
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
	Validation  []testrunner.Result `json:"validation"`
	Findings    []review.Finding    `json:"findings"`
	GeneratedAt time.Time           `json:"generated_at"`
}

// buildRunReport renders the human-readable report.
func buildRunReport(spec *taskfile.Spec, cfg config.Config, suite testrunner.SuiteResult, report review.Report, gate quality.Result, stage runpkg.Stage) string {
	var b strings.Builder
	heading := strings.TrimSpace(strings.Trim(strings.Join([]string{spec.ID, spec.Title}, " — "), " —"))
	fmt.Fprintf(&b, "# Run: %s\n\n", heading)
	fmt.Fprintf(&b, "- Provider: `%s`\n", cfg.Agent.Provider)
	fmt.Fprintf(&b, "- Review engine: `%s`\n", cfg.Review.Engine)
	fmt.Fprintf(&b, "- Stage: `%s`\n", stage)
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
