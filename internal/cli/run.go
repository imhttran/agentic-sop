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

	"github.com/imhttran/agentic-sop/internal/activity"
	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/failure"
	"github.com/imhttran/agentic-sop/internal/perf"
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

	stack := resolveExecutionStack(cfg)
	printExecutionStack(stdout, dir, cfg, stack)
	fmt.Fprintln(stdout)

	rn, err := runpkg.New(dir, runID(spec))
	if err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitError
	}
	_ = rn.Write("task.md", spec.Render())

	a, err := d.newAgent(cfg.Agent.Harness, cfg.Agent.Provider, cfg.Agent.Model)
	if err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitError
	}
	if err := guardCapability(a, agent.Implement); err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitError
	}

	ctx := taskActivityContext(context.Background(), rn.Dir(), stdout, rn.State().ID, spec.Title)
	res, err := executeLifecycle(ctx, dir, cfg, a, d, spec, rn, newRunSession(), stdout)
	emitClassificationActivity(ctx, res.classification)
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
	perf          perf.Task
	// jevDoc is the JEV run artifact written during the lifecycle, or nil when
	// JEV did not run. It is diagnostic evidence referenced by the report; it is
	// never workflow state and never feeds a decision.
	jevDoc *jevRunDoc
	// jevPath is the repository-relative path of the persisted JEV artifact, so
	// the report can point a reader at it. Empty when JEV did not run.
	jevPath string
	// classification is the failure-fixability classification for a run that did
	// not pass. It is zero when the run passed (or before it has been computed).
	// It is diagnostic: it records WHY the lifecycle stopped and what disposition
	// it applied, and never overrides the gate's own decision.
	classification failure.Classification
}

// executeLifecycle runs the lifecycle for spec, writing artifacts (including the
// report) into rn. It returns an error only for infrastructure failures
// (planner/agent/validation/review), which the caller records as a failed run; a
// deterministic gate failure is a normal result.
func executeLifecycle(ctx context.Context, dir string, cfg config.Config, a agent.Agent, d deps, spec *taskfile.Spec, rn *runpkg.Run, sess *runSession, stdout io.Writer) (lifeResult, error) {
	res, err := runStages(ctx, dir, cfg, a, d, spec, rn, sess, stdout)
	// Persist the failure classification beside the other run artifacts before any
	// early return, so a failure that stopped the lifecycle is still recorded. It
	// is diagnostic evidence: nothing reads it back to drive a decision.
	if res.classification.Disposition != "" {
		writeClassificationArtifact(rn, res.classification)
	}
	if err != nil {
		// Return the partial result alongside the error so a caller can still act
		// on any classification the lifecycle attached before it stopped.
		return res, err
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
		// JEV is diagnostic evidence, reported in its own section and pointing at
		// the persisted artifact. It is separate from the validation and review
		// sections, which remain authoritative.
		JEV:            jevReportSection(res.jevDoc, res.jevPath),
		Classification: classificationDoc(res.classification),
		Performance:    res.perf,
		GeneratedAt:    time.Now().UTC(),
	})
	return res, nil
}

// classificationDoc returns a pointer to the classification for the report, or
// nil when the run passed (nothing failed to classify). The report omits a nil
// classification, so an existing PASS report is unchanged.
func classificationDoc(cls failure.Classification) *failure.Classification {
	if cls.Disposition == "" {
		return nil
	}
	return &cls
}

// writeClassificationArtifact persists the failure classification as its own run
// artifact. It is best-effort: a write failure never changes the run outcome.
func writeClassificationArtifact(rn *runpkg.Run, cls failure.Classification) {
	data, err := json.MarshalIndent(cls, "", "  ")
	if err != nil {
		return
	}
	_ = rn.Write("classification.json", string(append(data, '\n')))
}

// runStages performs the lifecycle for one task. An ordinary task runs plan →
// implement → (validate → review → gate → fix)*. A verify-first task runs the
// configured deterministic validation first and invokes the implementation agent
// only when that validation fails (or when there is nothing configured to
// verify). It returns the final result and writes the intermediate artifacts.
func runStages(ctx context.Context, dir string, cfg config.Config, a agent.Agent, d deps, spec *taskfile.Spec, rn *runpkg.Run, sess *runSession, stdout io.Writer) (res lifeResult, err error) {
	rec := perf.NewRecorder(rn.State().ID)
	defer func() {
		res.perf = rec.Task()
		_ = writeMetrics(rn, res.perf)
	}()

	// ar observes the lifecycle for the activity stream. It is nil when reporting
	// is disabled (or no consumer is registered), and every emit on it is a no-op,
	// so it never affects execution.
	ar := activity.FromContext(ctx)

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
		// The working-tree change identifies the validation inputs, so it is read
		// before validation: a reuse hit is then decided against the same tree the
		// suite would run against.
		diff, err = d.readDiff(ctx, dir)
		if err != nil {
			return lifeResult{}, fmt.Errorf("diff: %w", err)
		}
		// The task's change evidence is recorded before the agent-less validation,
		// so a verify-first pass still contributes to the task's accumulated
		// implementation evidence.
		recordTaskChanges(rn, changedFiles(diff))
		suite := sessionValidation(ctx, dir, cfg, diff, rec, sess)
		switch {
		case len(suite.Results) == 0:
		case suite.Passed():
			sealed = &suite
			rec.AgentCallAvoided()
		default:
			failureCtx = validationFailureContext(suite)
		}
	}

	if sealed == nil {
		// Plan (must not mutate the repository). An invalid execution graph is
		// returned to the agent for correction rather than failing the task; each
		// repair is a real agent call and is reported in the run metrics.
		_ = rn.SetStage(runpkg.Planning)
		ar.Emit(activity.StagePlan, "generating plan", "")
		stop := rec.Measure(perf.StagePlan)
		plan, err = planner.New(a).OnRepair(func(attempt int, cause error) {
			rec.PlanRepair()
			rec.AgentCall()
			fmt.Fprintf(stdout, "plan: invalid plan returned to the agent for correction (attempt %d): %v\n", attempt, cause)
		}).Generate(ctx, spec.Render())
		stop()
		rec.AgentCall()
		if err != nil {
			return lifeResult{}, fmt.Errorf("plan: %w", err)
		}
		_ = rn.Write("plan.md", plan.RenderMarkdown())

		// Implement: the agent edits the repository; its summary is recorded but Git
		// remains the authority on what changed. The input carries the deterministic
		// failure that made the agent necessary, and the previous attempt's outcome,
		// so it can address the blocker rather than repeat the request that stopped it.
		_ = rn.SetStage(runpkg.Implementing)
		ar.Emit(activity.StageImplement, "implementing", "")
		input := plan.RenderMarkdown()
		if failureCtx != "" {
			input = failureCtx + "\n" + input
		}
		if sig, had := rn.ReadAttempt(); had {
			input = "# Previous attempt\n\nA previous attempt at this task did not complete:\n\n" + sig + "\n\n" + input
		}
		implStop := rec.Measure(perf.StageImplement)
		impl, err := a.Generate(ctx, agent.Request{
			Capability:         agent.Implement,
			Task:               spec.Render(),
			Input:              input,
			OutputRequirements: "Implement the plan in the working tree and summarize the changes.",
		})
		implStop()
		rec.AgentCall()
		if err != nil {
			return lifeResult{}, fmt.Errorf("implement: %w", err)
		}
		_ = rn.Write("implementation.md", impl.Content)
		// Record the invocation's observed changes as task-scoped evidence even when
		// it did not complete, so the next bounded invocation (and JEV) keeps the
		// implementation the task already produced.
		recordTaskChanges(rn, impl.ChangedFiles)
		if impl.Outcome != nil && impl.Outcome.Status != agent.OutcomeCompleted {
			return outcomeResult(ctx, rn, "IMPLEMENT", impl.Outcome), nil
		}

		diff, err = d.readDiff(ctx, dir)
		if err != nil {
			return lifeResult{}, fmt.Errorf("diff: %w", err)
		}
		recordTaskChanges(rn, taskInvocationChanges(impl.ChangedFiles, diff))
		_ = rn.Write("diff.patch", diff)

		// A claimed change with none produced is a failure. A legitimate no-change
		// completion is allowed, but still runs the configured validation below
		// before it can pass.
		changesExpected := impl.Outcome == nil || impl.Outcome.ChangesExpected
		if strings.TrimSpace(diff) == "" && changesExpected {
			_ = rn.SetStage(runpkg.Failed)
			ar.Emit(activity.StageFailed, "FAIL", "no repository changes")
			return lifeResult{gate: fail("agent reported successful implementation but produced no repository changes"), stage: runpkg.Failed}, nil
		}
	} else {
		// Verification-first pass: no agent produced a change, so there is nothing
		// for this task to review. The working tree was already read for the
		// validation identity; record it for the report.
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
	var jevEv *jevRunEvidence
	// jevDoc holds the persisted JEV artifact for the final iteration so the
	// report can reference it. It is written at the quality seam below, where JEV
	// evidence is produced, and is always the latest result of the run.
	var jevDoc *jevRunDoc

	for {
		if sealed != nil {
			// The verification-first pre-check already ran the validation.
			suite = *sealed
			sealed = nil
		} else {
			// Validate: deterministic, fail-fast. Uncompilable changes never reach review.
			_ = rn.SetStage(runpkg.Validating)
			suite = sessionValidation(ctx, dir, cfg, diff, rec, sess)
		}

		// Review only when validation passed, there is something to review, and the
		// change came from an implementation: a verify-first pass made no change of
		// its own, so there is nothing for this task to review.
		report = review.Report{}
		if suite.Passed() && !verifiedFirst && strings.TrimSpace(diff) != "" {
			reviewKey := reviewIdentity(cfg.Review.Engine, spec.Render(), diff)
			if cached, ok := sess.cachedReview(reviewKey); ok {
				rec.ReviewReused()
				report = cached
			} else {
				_ = rn.SetStage(runpkg.Reviewing)
				ar.Emit(activity.StageReview, "reviewing changes", "")
				provider, perr := reviewProvider(cfg, d)
				if perr != nil {
					return lifeResult{}, fmt.Errorf("review: %w", perr)
				}
				revStop := rec.Measure(perf.StageReview)
				report, err = provider.Review(ctx, review.Request{Task: spec.Render(), Diff: diff})
				revStop()
				rec.ReviewRun()
				if err != nil {
					return lifeResult{}, fmt.Errorf("review: %w", err)
				}
				sess.cacheReview(reviewKey, report)
			}
		}

		// Optional JEV analysis runs at the quality seam: after validation and
		// review have produced their evidence and before the gate decides. It is
		// read-only and add-only — with JEV disabled it is a no-op, so the gate
		// sees exactly what it saw before. A blocking JEV finding fails the gate
		// and enters the same bounded fix loop below; after a fix, validation,
		// review, and JEV all rerun before the gate is evaluated again. JEV
		// invocation metrics are recorded on rec as diagnostics only; they never
		// influence the gate.
		// The task-scoped change evidence accumulated across this task's invocations
		// (not only this one) is handed to JEV, so a no-change final invocation still
		// reviews the implementation the task produced earlier.
		jevEv = runOptionalJEV(ctx, cfg, d, spec, rn.State().ID, diff, suite, report, rn.ChangedFiles(), dir, rec)

		// Persist the JEV result as a run artifact beside the other diagnostics,
		// so results are available after the run. Persistence is best-effort and
		// never changes the run outcome; the same document is referenced by the
		// report. It is diagnostic evidence only: nothing here is read back to
		// drive a decision.
		jevDoc = persistedJEVDoc(rn, jevEv)

		gate = quality.Evaluate(cfg.Quality, quality.Input{
			BuildPassed:  categoryPassed(suite, testrunner.Build),
			TestPassed:   categoryPassed(suite, testrunner.UnitTest),
			LintRequired: hasCategory(suite, testrunner.Lint),
			LintPassed:   categoryPassed(suite, testrunner.Lint),
			Unresolved:   report.Findings,
			FixCycles:    cycles,
			JEV:          jevEv.gateEvidence(),
		})
		ar.Emit(activity.StageQuality, string(gate.Decision), "")

		// A failing check is actionable in its own right: the bounded repair loop must
		// run for it, not only for blocking review findings. A blocking JEV finding
		// is actionable the same way and enters this same loop (it is not a second
		// fix lifecycle: same stage, same budget, same counter). Review is skipped
		// when validation fails, so without this a broken build or test would never
		// reach a fix at all. The loop still stops when the gate passes, the fix
		// budget is spent, or nothing is left to act on.
		validationFailed := !suite.Passed()
		actionable := quality.BlockingFindings(cfg.Quality.FailOn, report.Findings) +
			quality.JEVBlockingFindings(cfg.Quality.JEVFailOn(), jevEv.gateEvidence())
		if gate.Decision != quality.Fail || cycles >= maxCycles || (!validationFailed && actionable == 0) {
			break
		}

		// Fix, then re-validate and re-review (regression protection).
		_ = rn.SetStage(runpkg.Fixing)
		ar.Emit(activity.StageFix, "applying fix", "")
		cycles++
		rec.FixCycle()
		fixStop := rec.Measure(perf.StageFix)
		fix, err := a.Generate(ctx, agent.Request{
			Capability:         agent.Fix,
			Task:               spec.Render(),
			Input:              fixContext(plan.RenderMarkdown(), report, suite, diff, jevEv, cfg.Quality.JEVFailOn()),
			OutputRequirements: "Fix the failing checks and blocking findings in the working tree and summarize the changes.",
		})
		fixStop()
		rec.AgentCall()
		if err != nil {
			return lifeResult{}, fmt.Errorf("fix: %w", err)
		}
		_ = rn.Write(fmt.Sprintf("fix-%d.md", cycles), fix.Content)
		recordTaskChanges(rn, fix.ChangedFiles)
		if fix.Outcome != nil && fix.Outcome.Status != agent.OutcomeCompleted {
			return outcomeResult(ctx, rn, "FIX", fix.Outcome), nil
		}

		diff, err = d.readDiff(ctx, dir)
		if err != nil {
			return lifeResult{}, fmt.Errorf("diff: %w", err)
		}
		recordTaskChanges(rn, taskInvocationChanges(fix.ChangedFiles, diff))
		if strings.TrimSpace(diff) == "" {
			_ = rn.SetStage(runpkg.Failed)
			ar.Emit(activity.StageFailed, "FAIL", "no repository changes")
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
		ar.Emit(activity.StageFailed, string(gate.Decision), firstReason(gate))
	case quality.NeedsHuman:
		stage = runpkg.WaitingForHuman
		ar.Emit(activity.StageBlocked, string(gate.Decision), firstReason(gate))
	case quality.Continue:
		// Never produced by Evaluate, but a continuation must not be mistaken for a
		// pass if a future gate verdict returns one.
		stage = runpkg.Failed
		ar.Emit(activity.StageClassify, string(gate.Decision), firstReason(gate))
	default:
		ar.Emit(activity.StageComplete, string(gate.Decision), "")
	}
	_ = rn.SetStage(stage)

	// Classify the gate failure so the driver's recovery decision (and the run
	// report) records why the lifecycle stopped and what disposition it applied. A
	// pass has no failure to classify.
	var class failure.Classification
	if gate.Decision != quality.Pass {
		class = failure.Classify(verificationEvidence(cfg, suite, report, cycles, jevEv))
	}
	return lifeResult{
		gate:           gate,
		cycles:         cycles,
		stage:          stage,
		suite:          suite,
		report:         report,
		verifiedFirst:  verifiedFirst,
		jevDoc:         jevDoc,
		jevPath:        jevArtifactRef(dir, rn),
		classification: class,
	}, nil
}

// verificationEvidence builds the structured evidence for a deterministic gate
// failure: which configured check failed, the blocking findings that remain, the
// fix budget, and whether JEV failed closed, so the classifier can decide whether
// the failure is safely auto-fixable, a bounded continuation, or has exhausted its
// bounded budget. A JEV analysis failure is a bounded, non-human signal: it is
// recorded so the classification preserves its reason instead of falling through
// to an "unknown" human boundary.
func verificationEvidence(cfg config.Config, suite testrunner.SuiteResult, report review.Report, cycles int, jevEv *jevRunEvidence) failure.Evidence {
	buildFailed := hasCategory(suite, testrunner.Build) && !categoryPassed(suite, testrunner.Build)
	testFailed := hasCategory(suite, testrunner.UnitTest) && !categoryPassed(suite, testrunner.UnitTest)
	lintFailed := hasCategory(suite, testrunner.Lint) && !categoryPassed(suite, testrunner.Lint)
	ev := failure.Evidence{
		Source:           "VALIDATE",
		BuildFailed:      buildFailed,
		TestFailed:       testFailed,
		LintFailed:       lintFailed,
		BlockingFindings: quality.BlockingFindings(cfg.Quality.FailOn, report.Findings),
		FixCycles:        cycles,
		MaxFixCycles:     cfg.Quality.MaxFixCycles,
	}
	// A fail-closed JEV is an analysis/provider failure, not a human decision. Its
	// authoritative reason is carried through so the classification and the report
	// say the task is blocked because JEV could not produce a valid result.
	if jevEvidence := jevEv.gateEvidence(); jevEvidence != nil && jevEvidence.FailClosed {
		ev.JEVFailed = true
		ev.JEVReason = jevEvidence.Reason
	}
	// Required test coverage is only "missing" when nothing else failed: a build
	// failure that skipped the tests is a compiler error, not a coverage gap.
	if cfg.Quality.RequiresTests() && !hasCategory(suite, testrunner.UnitTest) && !buildFailed && !testFailed && !lintFailed {
		ev.TestsMissing = true
	}
	return ev
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
	writeClassification(stdout, res.classification)
	if res.perf.Measured() {
		fmt.Fprintf(stdout, "performance: %s\n", res.perf.Line())
	}

	// JEV reporting: a concise read-only projection of the persisted JEV result,
	// so a user can tell whether JEV executed, see blocking findings with their
	// severity and source location, reach non-blocking findings, and locate the
	// report. It renders nothing when JEV did not run (res.jevDoc == nil), so a
	// disabled/absent JEV leaves the CLI output exactly as it was before. It is
	// diagnostic output only and never workflow state.
	writeJEVReport(stdout, res.jevDoc, cfg.Quality.JEVFailOn(), res.jevPath)

	if rel := relDir(dir, rn.Dir()); rel != "" {
		fmt.Fprintf(stdout, "report: %s/report.md\n", rel)
	}
	if res.gate.Decision == quality.Pass {
		fmt.Fprintln(stdout, "human approval required before commit; completed locally without committing.")
		return exitOK
	}
	return exitError
}

// writeClassification prints the failure classification for a run that did not
// pass: the disposition SOP applied, the failure kind, the confidence, and the
// reason. It renders nothing for a passing run (an empty classification), so a
// PASS leaves the CLI output unchanged. It is diagnostic output: the disposition
// describes what the lifecycle already decided, it never decides anything.
func writeClassification(w io.Writer, cls failure.Classification) {
	if cls.Disposition == "" {
		return
	}
	fmt.Fprintf(w, "classification: %s (%s, %s)\n", cls.Disposition, cls.Kind, cls.Confidence)
	if reason := strings.TrimSpace(cls.Reason); reason != "" {
		fmt.Fprintf(w, "  reason: %s\n", reason)
	}
}

// fail builds a FAIL result carrying a single reason.
func fail(reason string) quality.Result {
	return quality.Result{Decision: quality.Fail, Reasons: []string{reason}}
}

// firstReason returns the gate's first reason, or "" when there is none. It is
// the short, single-line detail the activity stream attaches to a terminal
// event; the full reasons remain in the run report.
func firstReason(gate quality.Result) string {
	if len(gate.Reasons) == 0 {
		return ""
	}
	return gate.Reasons[0]
}

// emitOutcomeActivity reports a non-completed agent outcome on the activity
// stream. A resumable continuation (a retryable disposition) is reported as a
// CLASSIFY event carrying the disposition, so it is visibly distinct from a human
// boundary (BLOCKED) and a hard failure (FAILED): the operator sees SOP will
// continue on its own.
func emitOutcomeActivity(ar *activity.Recorder, outcome *agent.Outcome, class failure.Classification) {
	if class.Retryable() {
		ar.Emit(activity.StageClassify, string(class.Disposition), string(class.Kind))
		return
	}
	if outcome.Status == agent.OutcomeNeedsHuman {
		ar.Emit(activity.StageBlocked, string(outcome.Status), "")
		return
	}
	ar.Emit(activity.StageFailed, string(outcome.Status), "")
}

// emitClassificationActivity reports a non-empty failure classification on the
// activity stream carried by ctx. It is a no-op when reporting is disabled or the
// run passed (an empty classification).
func emitClassificationActivity(ctx context.Context, cls failure.Classification) {
	if cls.Disposition == "" {
		return
	}
	activity.FromContext(ctx).Emit(activity.StageClassify, string(cls.Disposition), string(cls.Kind))
}

// outcomeResult maps a non-completed agent outcome to a run result and reports it
// on the activity stream. SOP owns the lifecycle, so the failure classifier's
// evidence decides the disposition rather than the status label alone: an outcome
// that is a resumable continuation (budget/no-change exhaustion, a transient
// provider failure) is NOT a human boundary even though the harness labels it
// needs_human to request a requeue. Such an outcome is reported as CONTINUE so the
// operator sees SOP will continue on its own, and the driver requeues it through
// the existing bounded recovery path. Only a genuine human boundary (approval,
// safety, ambiguity) becomes NEEDS_HUMAN; any other reported failure becomes FAIL.
// A reported outcome is never treated as success. It also attaches the
// classification, so the driver applies the same disposition logic it applies to a
// deterministic gate failure.
func outcomeResult(ctx context.Context, rn *runpkg.Run, source string, outcome *agent.Outcome) lifeResult {
	reason := outcome.Reason
	if reason == "" {
		reason = outcome.Summary
	}
	if reason == "" {
		reason = string(outcome.Status)
	}
	class := failure.Classify(failure.Evidence{Source: source, Outcome: outcome})
	emitOutcomeActivity(activity.FromContext(ctx), outcome, class)

	if outcome.Status == agent.OutcomeNeedsHuman && class.Retryable() {
		// A resumable continuation: the invocation did not finish, but no human
		// decision is required. Report CONTINUE (never NEEDS_HUMAN), mirroring the
		// gate-failure path where a retryable disposition is a non-pass result.
		_ = rn.SetStage(runpkg.Failed)
		return lifeResult{gate: quality.Result{Decision: quality.Continue, Reasons: []string{reason}}, stage: runpkg.Failed, classification: class}
	}
	if outcome.Status == agent.OutcomeNeedsHuman {
		_ = rn.SetStage(runpkg.WaitingForHuman)
		return lifeResult{gate: quality.Result{Decision: quality.NeedsHuman, Reasons: []string{reason}}, stage: runpkg.WaitingForHuman, classification: class}
	}
	_ = rn.SetStage(runpkg.Failed)
	return lifeResult{gate: quality.Result{Decision: quality.Fail, Reasons: []string{reason}}, stage: runpkg.Failed, classification: class}
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

// fixContext renders the bounded context a fix is given: the plan, the
// deterministic validation failure that still stands (when a check failed), the
// blocking review findings, the actionable JEV findings (when JEV ran), and the
// current diff. The existing sections keep their order; the JEV section is a
// distinct block appended after the failure/review evidence and before the diff.
func fixContext(plan string, report review.Report, suite testrunner.SuiteResult, diff string, jev *jevRunEvidence, jevFailOn []string) string {
	var b strings.Builder
	b.WriteString("# Plan\n\n")
	b.WriteString(strings.TrimSpace(plan))
	if !suite.Passed() {
		b.WriteString("\n\n")
		b.WriteString(validationFailureContext(suite))
	}
	b.WriteString("\n\n# Blocking findings\n\n")
	for _, f := range report.Findings {
		location := f.File
		if f.Line > 0 {
			location = fmt.Sprintf("%s:%d", f.File, f.Line)
		}
		fmt.Fprintf(&b, "- %s %s — %s: %s\n", f.Severity, location, f.Title, f.Detail)
	}
	if section := jev.findingsSection(jevFailOn); section != "" {
		b.WriteString("\n")
		b.WriteString(section)
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

// timedValidation runs the configured validation and records its stage time, its
// per-category durations, and one validation execution. It is the single place a
// suite is measured, so counts and timings cannot drift from where validation runs.
func timedValidation(ctx context.Context, dir string, cfg config.Config, rec *perf.Recorder) testrunner.SuiteResult {
	// Announce the validation commands before running them, so a slow suite is
	// visible as it runs rather than only after it returns.
	ar := activity.FromContext(ctx)
	for _, check := range validate.Checks(cfg.Validation) {
		ar.Emit(activity.StageValidate, check.Command, "")
	}
	stop := rec.Measure(perf.StageValidation)
	suite := validate.Run(ctx, dir, cfg.Validation)
	stop()
	rec.ValidationRun()
	for _, r := range suite.Results {
		rec.AddValidation(validationCategory(r.Category), r.Duration)
	}
	return suite
}

// sessionValidation runs the configured validation, reusing a prior passing result
// when the command set and working-tree change are unchanged. It is the single
// place the reuse decision is made, so the cache and the run cannot drift.
func sessionValidation(ctx context.Context, dir string, cfg config.Config, diff string, rec *perf.Recorder, sess *runSession) testrunner.SuiteResult {
	key := validationIdentity(cfg.Validation, diff)
	if suite, ok := sess.cachedValidation(key); ok {
		rec.ValidationReused()
		return suite
	}
	suite := timedValidation(ctx, dir, cfg, rec)
	sess.cacheValidation(key, suite)
	return suite
}

// validationCategory maps a runner category to the performance model's categories.
func validationCategory(c testrunner.Category) string {
	switch c {
	case testrunner.UnitTest, testrunner.IntegrationTest:
		return perf.CategoryTest
	case testrunner.Lint:
		return perf.CategoryLint
	default:
		return perf.CategoryBuild
	}
}

// writeMetrics persists a task's performance record beside its other run
// artifacts. Metrics are diagnostic only and are never read back to drive a
// decision.
func writeMetrics(rn *runpkg.Run, t perf.Task) error {
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	return rn.Write("metrics.json", string(append(data, '\n')))
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
	// JEV is the JEV diagnostic evidence section: a distinct, attributed summary
	// of the JEV result that points at the persisted artifact. It is separate
	// from Validation and Findings, which remain authoritative, and is omitted
	// when JEV did not run.
	JEV *jevReportDoc `json:"jev,omitempty"`
	// Performance is the task's timing/count record: diagnostic metadata only,
	// never an input to a decision.
	Performance perf.Task `json:"performance"`
	// Classification is the failure-fixability classification for a run that did
	// not pass: the kind, the disposition SOP applied, and the reason. It is
	// omitted for a passing run, so an existing PASS report is unchanged.
	Classification *failure.Classification `json:"classification,omitempty"`
	GeneratedAt    time.Time               `json:"generated_at"`
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

	// JEV is reported in its own section, after Review and before Gate, so a
	// reader can tell the JEV diagnostic evidence apart from validation and
	// review. It is rendered only when JEV ran.
	if section := renderJEVSection(res.jevDoc, res.jevPath); section != "" {
		b.WriteString("\n")
		b.WriteString(section)
	}

	b.WriteString("\n## Gate\n\n")
	for _, reason := range res.gate.Reasons {
		fmt.Fprintf(&b, "- %s\n", reason)
	}
	writeClassificationReport(&b, res.classification)
	return b.String()
}

// writeClassificationReport renders the failure classification in the run report,
// so the reason the lifecycle stopped and the disposition it applied are durable
// and auditable. It renders nothing for a passing run.
func writeClassificationReport(b *strings.Builder, cls failure.Classification) {
	if cls.Disposition == "" {
		return
	}
	b.WriteString("\n## Classification\n\n")
	fmt.Fprintf(b, "- Kind: `%s`\n", cls.Kind)
	fmt.Fprintf(b, "- Disposition: `%s`\n", cls.Disposition)
	fmt.Fprintf(b, "- Confidence: `%s`\n", cls.Confidence)
	if reason := strings.TrimSpace(cls.Reason); reason != "" {
		fmt.Fprintf(b, "- Reason: %s\n", reason)
	}
}
