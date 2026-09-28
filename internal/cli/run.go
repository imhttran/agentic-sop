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

// runRun drives the local lifecycle. With a task file it runs that one task; with
// no arguments it drives the persisted task graph in dependency order.
//
//	task → plan → implement → detect changes
//	     → { validate → review → gate → fix } → report
//
// The braces are a bounded fix loop (≤ quality.max_fix_cycles; exhaustion yields
// NEEDS_HUMAN). A run never commits, pushes, or merges.
func runRun(args []string, stdout, stderr io.Writer, d deps) int {
	planArg, taskArg, ok := parseRunArgs(args, stderr)
	if !ok {
		return exitUsage
	}
	if taskArg != "" {
		return runSingleTask(taskArg, stdout, stderr, d)
	}
	return runGraph(planArg, stdout, stderr, d)
}

// runUsage is the one-line usage for `sop run`.
const runUsage = "usage: sop run [PLAN.md | --task TASK.md]"

// parseRunArgs parses "sop run [PLAN.md | --task TASK.md]": no arguments
// discovers the project plan, one argument names an execution PLAN, and --task
// names a single task file.
func parseRunArgs(args []string, stderr io.Writer) (planArg, taskArg string, ok bool) {
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--task":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, runUsage)
				return "", "", false
			}
			taskArg = args[i+1]
			i++
		case strings.HasPrefix(a, "--task="):
			taskArg = strings.TrimPrefix(a, "--task=")
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "unknown flag %s\n%s\n", a, runUsage)
			return "", "", false
		default:
			if planArg != "" {
				fmt.Fprintln(stderr, runUsage)
				return "", "", false
			}
			planArg = a
		}
	}
	if planArg != "" && taskArg != "" {
		fmt.Fprintln(stderr, runUsage)
		return "", "", false
	}
	return planArg, taskArg, true
}

// runSingleTask runs one task file through the local lifecycle.
func runSingleTask(file string, stdout, stderr io.Writer, d deps) int {
	dir, ok := projectDir(d.getwd, stderr)
	if !ok {
		return exitError
	}

	spec, err := loadTaskFile(dir, file)
	if err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitError
	}

	cfg, err := loadConfigOrDefault(dir)
	if err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitError
	}

	rn, err := runpkg.New(dir, runID(spec))
	if err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitError
	}
	_ = rn.Write("task.md", spec.Render())

	a, err := d.newAgent(cfg.Agent.Provider, cfg.Agent.Model)
	if err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitError
	}
	if err := guardCapability(a, agent.Implement); err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitError
	}

	res, err := executeLifecycle(context.Background(), dir, cfg, a, d, spec, rn)
	if err != nil {
		return failRun(rn, stderr, err)
	}
	return emitRunSummary(stdout, dir, cfg, rn, res)
}

// lifeResult is the outcome of one local lifecycle.
type lifeResult struct {
	gate          quality.Result
	cycles        int
	stage         runpkg.Stage
	suite         testrunner.SuiteResult
	report        review.Report
	verifiedFirst bool
}

// executeLifecycle runs the lifecycle for spec, writing artifacts (including the
// report) into rn. It returns an error only for infrastructure failures
// (planner/agent/validation/review), which the caller records as a failed run; a
// deterministic gate failure is a normal result.
func executeLifecycle(ctx context.Context, dir string, cfg config.Config, a agent.Agent, d deps, spec *taskfile.Spec, rn *runpkg.Run) (lifeResult, error) {
	res, err := runStages(ctx, dir, cfg, a, d, spec, rn)
	if err != nil {
		return lifeResult{}, err
	}

	_ = rn.Write("report.md", buildRunReport(spec, cfg, res))
	writeRunJSON(rn, "report.json", runReportDoc{
		ID:            rn.State().ID,
		Stage:         res.stage,
		Provider:      cfg.Agent.Provider,
		Engine:        cfg.Review.Engine,
		Decision:      res.gate.Decision,
		Reasons:       res.gate.Reasons,
		FixCycles:     res.cycles,
		ExecutionMode: string(spec.ExecutionMode),
		VerifiedFirst: res.verifiedFirst,
		Validation:    res.suite.Results,
		Findings:      res.report.Findings,
		GeneratedAt:   time.Now().UTC(),
	})
	return res, nil
}

// runStages performs the lifecycle for one task. An ordinary task runs plan →
// implement → (validate → review → gate → fix)*. A verify-first task runs the
// configured deterministic validation first and invokes the implementation agent
// only when that validation fails (or when there is nothing configured to
// verify). It returns the final result and writes the intermediate artifacts.
func runStages(ctx context.Context, dir string, cfg config.Config, a agent.Agent, d deps, spec *taskfile.Spec, rn *runpkg.Run) (lifeResult, error) {
	var (
		plan       *planner.Plan
		diff       string
		sealed     *testrunner.SuiteResult // validation already run for a verify-first task
		failureCtx string
	)

	// Verification-first: run the configured deterministic validation before any
	// agent is invoked. A pass needs no agent at all; a failure is handed to the
	// implementation agent below; with nothing configured to run there is nothing
	// to verify, so the task takes the ordinary implementation path.
	if spec.ExecutionMode.VerifyFirst() {
		_ = rn.SetStage(runpkg.Validating)
		suite := validate.Run(ctx, dir, cfg.Validation)
		switch {
		case len(suite.Results) == 0:
		case suite.Passed():
			sealed = &suite
		default:
			failureCtx = validationFailureContext(suite)
		}
	}

	if sealed == nil {
		// Plan (must not mutate the repository).
		_ = rn.SetStage(runpkg.Planning)
		var err error
		plan, err = planner.New(a).Generate(ctx, spec.Render())
		if err != nil {
			return lifeResult{}, fmt.Errorf("plan: %w", err)
		}
		_ = rn.Write("plan.md", plan.RenderMarkdown())

		// Implement: the agent edits the repository; its summary is recorded but Git
		// remains the authority on what changed. The input carries the deterministic
		// failure that made the agent necessary, and the previous attempt's outcome,
		// so it can address the blocker rather than repeat the request that stopped it.
		_ = rn.SetStage(runpkg.Implementing)
		input := plan.RenderMarkdown()
		if failureCtx != "" {
			input = failureCtx + "\n" + input
		}
		if sig, had := rn.ReadAttempt(); had {
			input = "# Previous attempt\n\nA previous attempt at this task did not complete:\n\n" + sig + "\n\n" + input
		}
		impl, err := a.Generate(ctx, agent.Request{
			Capability:         agent.Implement,
			Task:               spec.Render(),
			Input:              input,
			OutputRequirements: "Implement the plan in the working tree and summarize the changes.",
		})
		if err != nil {
			return lifeResult{}, fmt.Errorf("implement: %w", err)
		}
		_ = rn.Write("implementation.md", impl.Content)
		if impl.Outcome != nil && impl.Outcome.Status != agent.OutcomeCompleted {
			return outcomeResult(rn, impl.Outcome), nil
		}

		diff, err = d.readDiff(ctx, dir)
		if err != nil {
			return lifeResult{}, fmt.Errorf("diff: %w", err)
		}
		_ = rn.Write("diff.patch", diff)

		// A claimed change with none produced is a failure. A legitimate no-change
		// completion is allowed, but still runs the configured validation below
		// before it can pass.
		changesExpected := impl.Outcome == nil || impl.Outcome.ChangesExpected
		if strings.TrimSpace(diff) == "" && changesExpected {
			_ = rn.SetStage(runpkg.Failed)
			return lifeResult{gate: fail("agent reported successful implementation but produced no repository changes"), stage: runpkg.Failed}, nil
		}
	} else {
		// Verification-first pass: no agent produced a change, so there is nothing
		// for this task to review. Record the working tree for the report.
		var err error
		diff, err = d.readDiff(ctx, dir)
		if err != nil {
			return lifeResult{}, fmt.Errorf("diff: %w", err)
		}
		_ = rn.Write("diff.patch", diff)
	}

	// verifiedFirst marks the fast path: the deterministic validation passed and no
	// implementation agent ran.
	verifiedFirst := sealed != nil

	maxCycles := cfg.Quality.MaxFixCycles
	cycles := 0
	var suite testrunner.SuiteResult
	var report review.Report
	var gate quality.Result

	for {
		if sealed != nil {
			// The verification-first pre-check already ran the validation.
			suite = *sealed
			sealed = nil
		} else {
			// Validate: deterministic, fail-fast. Uncompilable changes never reach review.
			_ = rn.SetStage(runpkg.Validating)
			suite = validate.Run(ctx, dir, cfg.Validation)
		}

		// Review only when validation passed, there is something to review, and the
		// change came from an implementation: a verify-first pass made no change of
		// its own, so there is nothing for this task to review.
		report = review.Report{}
		if suite.Passed() && !verifiedFirst && strings.TrimSpace(diff) != "" {
			_ = rn.SetStage(runpkg.Reviewing)
			provider, err := reviewProvider(cfg, d)
			if err != nil {
				return lifeResult{}, fmt.Errorf("review: %w", err)
			}
			report, err = provider.Review(ctx, review.Request{Task: spec.Render(), Diff: diff})
			if err != nil {
				return lifeResult{}, fmt.Errorf("review: %w", err)
			}
		}

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
			return lifeResult{}, fmt.Errorf("fix: %w", err)
		}
		_ = rn.Write(fmt.Sprintf("fix-%d.md", cycles), fix.Content)
		if fix.Outcome != nil && fix.Outcome.Status != agent.OutcomeCompleted {
			return outcomeResult(rn, fix.Outcome), nil
		}

		diff, err = d.readDiff(ctx, dir)
		if err != nil {
			return lifeResult{}, fmt.Errorf("diff: %w", err)
		}
		if strings.TrimSpace(diff) == "" {
			_ = rn.SetStage(runpkg.Failed)
			return lifeResult{gate: fail("agent reported a fix but produced no repository changes"), stage: runpkg.Failed}, nil
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
	return lifeResult{gate: gate, cycles: cycles, stage: stage, suite: suite, report: report, verifiedFirst: verifiedFirst}, nil
}

// emitRunSummary prints the run's outcome and returns the process exit code.
func emitRunSummary(stdout io.Writer, dir string, cfg config.Config, rn *runpkg.Run, res lifeResult) int {
	fmt.Fprintf(stdout, "run %s: %s\n", rn.State().ID, res.gate.Decision)
	if res.verifiedFirst {
		fmt.Fprintln(stdout, "verified first: the configured validation passed; no implementation agent was invoked")
	}
	for _, reason := range res.gate.Reasons {
		fmt.Fprintf(stdout, "  - %s\n", reason)
	}
	fmt.Fprintf(stdout, "fix cycles: %d/%d\n", res.cycles, cfg.Quality.MaxFixCycles)
	if rel := relDir(dir, rn.Dir()); rel != "" {
		fmt.Fprintf(stdout, "report: %s/report.md\n", rel)
	}
	if res.gate.Decision == quality.Pass {
		fmt.Fprintln(stdout, "human approval required before commit; completed locally without committing.")
		return exitOK
	}
	return exitError
}

// fail builds a FAIL result carrying a single reason.
func fail(reason string) quality.Result {
	return quality.Result{Decision: quality.Fail, Reasons: []string{reason}}
}

// outcomeResult maps a non-completed agent outcome to a run result: a human
// boundary becomes NEEDS_HUMAN, any other reported failure becomes FAIL. A
// reported outcome is never treated as success.
func outcomeResult(rn *runpkg.Run, outcome *agent.Outcome) lifeResult {
	reason := outcome.Reason
	if reason == "" {
		reason = outcome.Summary
	}
	if reason == "" {
		reason = string(outcome.Status)
	}
	if outcome.Status == agent.OutcomeNeedsHuman {
		_ = rn.SetStage(runpkg.WaitingForHuman)
		return lifeResult{gate: quality.Result{Decision: quality.NeedsHuman, Reasons: []string{reason}}, stage: runpkg.WaitingForHuman}
	}
	_ = rn.SetStage(runpkg.Failed)
	return lifeResult{gate: quality.Result{Decision: quality.Fail, Reasons: []string{reason}}, stage: runpkg.Failed}
}

// failRun marks the run failed, reports the stage error, and returns the error
// exit code.
func failRun(rn *runpkg.Run, stderr io.Writer, err error) int {
	_ = rn.SetStage(runpkg.Failed)
	fmt.Fprintf(stderr, "run: %v\n", err)
	return exitError
}

// loadConfigOrDefault returns the project configuration, falling back to the
// built-in defaults when no configuration file exists.
func loadConfigOrDefault(dir string) (config.Config, error) {
	loaded, err := config.LoadDir(dir)
	if err == nil {
		return *loaded, nil
	}
	if errors.Is(err, config.ErrNotFound) {
		return config.Default(), nil
	}
	return config.Config{}, err
}

// validationFailureContext renders the deterministic validation failure that made
// an implementation agent necessary, so a verify-first task's agent starts from
// the actual failure instead of rediscovering it.
func validationFailureContext(suite testrunner.SuiteResult) string {
	var b strings.Builder
	b.WriteString("# Validation failed\n\nThe configured validation did not pass, so an implementation is required.\n")
	for _, r := range suite.Results {
		if r.Status == testrunner.Pass {
			continue
		}
		fmt.Fprintf(&b, "\n- %s %s `%s`\n", r.Status, r.Category, r.Command)
		for _, stream := range []string{r.Stdout, r.Stderr} {
			if out := strings.TrimSpace(stream); out != "" {
				fmt.Fprintf(&b, "\n```\n%s\n```\n", out)
			}
		}
	}
	return b.String()
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
	ID            string              `json:"id"`
	Stage         runpkg.Stage        `json:"stage"`
	Provider      string              `json:"provider"`
	Engine        string              `json:"review_engine"`
	Decision      quality.Decision    `json:"decision"`
	Reasons       []string            `json:"reasons"`
	FixCycles     int                 `json:"fix_cycles"`
	ExecutionMode string              `json:"execution_mode"`
	VerifiedFirst bool                `json:"verified_first"`
	Validation    []testrunner.Result `json:"validation"`
	Findings      []review.Finding    `json:"findings"`
	GeneratedAt   time.Time           `json:"generated_at"`
}

// buildRunReport renders the human-readable report.
func buildRunReport(spec *taskfile.Spec, cfg config.Config, res lifeResult) string {
	var b strings.Builder
	heading := strings.TrimSpace(strings.Trim(strings.Join([]string{spec.ID, spec.Title}, " — "), " — "))
	fmt.Fprintf(&b, "# Run: %s\n\n", heading)
	fmt.Fprintf(&b, "- Provider: `%s`\n", cfg.Agent.Provider)
	fmt.Fprintf(&b, "- Review engine: `%s`\n", cfg.Review.Engine)
	fmt.Fprintf(&b, "- Stage: `%s`\n", res.stage)
	fmt.Fprintf(&b, "- Fix cycles: %d/%d\n", res.cycles, cfg.Quality.MaxFixCycles)
	if res.verifiedFirst {
		b.WriteString("- Execution: `verify-first` (validation passed; no implementation agent invoked)\n")
	}
	fmt.Fprintf(&b, "- Gate: `%s`\n\n", res.gate.Decision)

	b.WriteString("## Task\n\n")
	b.WriteString(strings.TrimSpace(spec.Render()))
	b.WriteString("\n\n## Validation\n\n")
	if len(res.suite.Results) == 0 {
		b.WriteString("No validation commands configured.\n")
	} else {
		for _, r := range res.suite.Results {
			fmt.Fprintf(&b, "- %s %s `%s`\n", r.Status, r.Category, r.Command)
		}
	}

	b.WriteString("\n## Review\n\n")
	report := res.report
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
	for _, reason := range res.gate.Reasons {
		fmt.Fprintf(&b, "- %s\n", reason)
	}
	return b.String()
}
