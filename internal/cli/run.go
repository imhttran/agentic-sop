package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/imhttran/agentic-sop/internal/activity"
	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/autonomy"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/failure"
	"github.com/imhttran/agentic-sop/internal/git"
	"github.com/imhttran/agentic-sop/internal/model"
	"github.com/imhttran/agentic-sop/internal/perf"
	"github.com/imhttran/agentic-sop/internal/planner"
	"github.com/imhttran/agentic-sop/internal/quality"
	"github.com/imhttran/agentic-sop/internal/review"
	"github.com/imhttran/agentic-sop/internal/router"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/runtrace"
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
	opts, ok := parseRunArgs(args, stderr)
	if !ok {
		return exitUsage
	}
	d.modelClass = opts.modelClass
	var err error
	d.taskInputs, err = loadTaskInputs()
	if err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitError
	}
	if opts.taskArg != "" {
		return runSingleTask(opts.taskArg, stdout, stderr, d)
	}
	return runGraph(opts.planArg, stdout, stderr, d)
}

// runUsage is the one-line usage for `sop run`.
const runUsage = "usage: sop run [--model-class small|medium|large] [PLAN.md | --task TASK.md]"

// runOptions are the parsed arguments of `sop run`.
type runOptions struct {
	planArg    string
	taskArg    string
	modelClass string
}

// parseRunArgs parses "sop run [--model-class CLASS] [PLAN.md | --task TASK.md]":
// no arguments discovers the project plan, one argument names an execution PLAN,
// --task names a single task file, and --model-class overrides the model-routing
// class for this invocation.
func parseRunArgs(args []string, stderr io.Writer) (runOptions, bool) {
	var opts runOptions
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--task":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, runUsage)
				return runOptions{}, false
			}
			opts.taskArg = args[i+1]
			i++
		case strings.HasPrefix(a, "--task="):
			opts.taskArg = strings.TrimPrefix(a, "--task=")
		case a == "--model-class":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, runUsage)
				return runOptions{}, false
			}
			opts.modelClass = args[i+1]
			i++
		case strings.HasPrefix(a, "--model-class="):
			opts.modelClass = strings.TrimPrefix(a, "--model-class=")
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "unknown flag %s\n%s\n", a, runUsage)
			return runOptions{}, false
		default:
			if opts.planArg != "" {
				fmt.Fprintln(stderr, runUsage)
				return runOptions{}, false
			}
			opts.planArg = a
		}
	}
	if opts.planArg != "" && opts.taskArg != "" {
		fmt.Fprintln(stderr, runUsage)
		return runOptions{}, false
	}
	return opts, true
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
	// execution_mode done declares a plan stage whose work already exists, so its
	// task is recorded as satisfied rather than executed. A single --task invocation
	// has nothing to record against, so the mode is rejected here instead of being
	// silently ignored (which would implement work the operator declared done).
	if spec.ExecutionMode.Done() {
		fmt.Fprintln(stderr, "run: `--task` always executes a task; execution_mode `done` applies to plan stages")
		return exitError
	}

	cfg, err := loadConfigOrDefault(dir)
	if err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitError
	}
	routing, err := applyModelRouting(&cfg, d.modelClass)
	if err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitError
	}
	if err := applyRoutingEnabled(&d, cfg); err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitError
	}
	if err := applyEscalationEnabled(&d, cfg); err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitError
	}
	if err := applyReplanPolicy(&d, cfg); err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitError
	}
	// Carry the resolved evidence down to the lifecycle so the run records it.
	d.routing = routing

	stack := resolveExecutionStack(cfg)
	printExecutionStack(stdout, dir, cfg, stack, routing)
	fmt.Fprintln(stdout)

	id := runID(spec)
	priorStage, _ := runpkg.Load(dir, id)
	rn, err := runpkg.New(dir, id)
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
	if !d.routingEnabled {
		// The run-level/default selection is what executes only when the automatic
		// per-task router is off. With the router on, each task's final selection is
		// built, validated, and capability-guarded in applyTaskRouting instead, so the
		// default agent MUST NOT be rejected here (Phase 5.4 §36).
		if err := guardCapability(a, agent.Implement); err != nil {
			fmt.Fprintf(stderr, "run: %v\n", err)
			return exitError
		}
		if err := validateSelectedModel(context.Background(), cfg, routing); err != nil {
			fmt.Fprintf(stderr, "run: %v\n", err)
			return exitError
		}
	}

	ctx := taskActivityContext(context.Background(), rn.Dir(), stdout, rn.State().ID, spec.Title)

	// Optional early JEV task-triage checkpoint (Phase 3). Single-task mode has no
	// scheduler store to requeue into, so a policy escalation stops with a clear
	// human boundary message instead of silently running the task. A disabled gate
	// is a strict no-op.
	tri := runEarlyGate(ctx, cfg, d, spec, rn, runpkg.CheckpointTaskTriage)
	if tri.Escalate {
		fmt.Fprintf(stdout, "%s NEEDS_HUMAN (early JEV triage)\n  %s\n", rn.State().ID, tri.Reason)
		printParkedHumanGate(stdout, rn.State().ID, string(runpkg.WaitingForHuman), tri.Reason, "sop run --task "+file, false)
		return exitError
	}

	res, err := runAttempts(ctx, dir, cfg, a, d, spec, rn, newRunSession(), currentApprovalBoundary(rn, priorStage), tri, stdout)
	emitClassificationActivity(ctx, res.classification, res.decision)
	if err != nil {
		return failRun(rn, stderr, err)
	}
	code := emitRunSummary(stdout, dir, cfg, rn, res)
	// An ad-hoc `--task` run that stops at a human boundary names it too, but it has no
	// stored task, so there is no resolvable approval gate: it prints the boundary and
	// how to continue instead of a command that would fail.
	if humanBoundary(res.stage, res.classification, res.decision) {
		printParkedHumanGate(stdout, rn.State().ID, string(res.stage),
			firstNonBlank(firstReason(res.gate), res.classification.Reason), "sop run --task "+file, false)
	}
	return code
}

// lifeResult is the outcome of one local lifecycle.
type lifeResult struct {
	gate          quality.Result
	cycles        int
	stage         runpkg.Stage
	suite         testrunner.SuiteResult
	report        review.Report
	verifiedFirst bool
	satisfaction  *agent.CompletionEvidence
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
	// decision is the risk-based autonomy decision applied to the classification:
	// the single authority on whether the failure is handled automatically or needs
	// a human. It is zero when the run passed. It is diagnostic evidence the driver
	// acts on; the classification alone never implies human approval.
	decision autonomy.Decision
	// modelSelection is the resolved, non-secret model-routing evidence for the
	// run, or nil when routing is inactive. It is diagnostic: it records which
	// class, provider, model, and layer the run used and why, and never feeds a
	// decision. When per-task routing applied, this is the task's own selection.
	modelSelection *model.Selection
	// routing is the per-task routing decision (Phase 3.5), or nil when no routing
	// applied. It records the class, the deterministic reasons, the resolved
	// selection, and the typed signals used. It is diagnostic evidence; it never
	// feeds a decision beyond the model choice it already recorded.
	routing *taskRouting
}

// executeLifecycle runs the lifecycle for spec, writing artifacts (including the
// report) into rn. It returns an error only for infrastructure failures
// (planner/agent/validation/review), which the caller records as a failed run; a
// deterministic gate failure is a normal result.
func executeLifecycle(ctx context.Context, dir string, cfg config.Config, a agent.Agent, d deps, spec *taskfile.Spec, rn *runpkg.Run, sess *runSession, approval failure.ApprovalBoundary, tri earlyGateResult, stdout io.Writer) (lifeResult, error) {
	// Persist the non-secret model-routing evidence before the lifecycle runs, so
	// the selected class/provider/model and why remain inspectable even when the
	// lifecycle stops early. It is diagnostic: nothing reads it back.
	if d.routing.Active {
		writeModelSelectionArtifact(rn, d.routing.Selection)
	}
	res, err := runStages(ctx, dir, cfg, a, d, spec, rn, sess, approval, tri, stdout)
	// A per-task routing decision (Phase 3.5) supersedes the run-level selection:
	// the task's own resolved class/provider/model is recorded, and a routing
	// artifact records the class, reasons, and typed signals. Both are diagnostic.
	if res.routing != nil {
		res.modelSelection = &res.routing.Selection
		writeModelSelectionArtifact(rn, res.routing.Selection)
		// Persist the routing decision and surface any contract violation (an
		// unknown version or source that failed closed at the store boundary) rather
		// than discarding it. The artifact is diagnostic only, so this never changes
		// the run's already-fixed decision; it is reported so a failed write is not
		// mistaken for success.
		//
		// An escalated attempt (Phase 5) never overwrites it: routing.json records what
		// the router FIRST chose, and an escalation is a different fact, recorded in
		// its own attempt record.
		if res.routing.Source != runpkg.RoutingSourceEscalation {
			if werr := writeRoutingDecisionArtifact(rn, rn.State().ID, res.routing); werr != nil {
				fmt.Fprintf(stdout, "warning: routing decision artifact not persisted: %v\n", werr)
			}
		}
	} else {
		res.modelSelection = modelSelectionDoc(d.routing)
	}
	// Persist the failure classification and the autonomy decision beside the other
	// run artifacts before any early return, so a failure that stopped the lifecycle
	// is still recorded. They are diagnostic evidence: nothing reads them back to
	// drive a decision.
	if res.classification.Disposition != "" {
		writeClassificationArtifact(rn, res.classification, res.decision)
	}
	if err != nil {
		// Persist the trace for a stopped run too: its trajectory and the attached
		// classification are exactly what a reader needs. Best-effort observation.
		writeRunTrace(rn, res, cfg, runtrace.CollectorFromContext(ctx))
		// Return the partial result alongside the error so a caller can still act
		// on any classification the lifecycle attached before it stopped.
		return res, err
	}

	_ = rn.Write("report.md", buildRunReport(spec, cfg, res))
	completion := ""
	var mutations *int
	if res.satisfaction != nil {
		completion = agent.AlreadySatisfied
		zero := 0
		mutations = &zero
	}
	writeRunJSON(rn, "report.json", runReportDoc{
		ID:                  rn.State().ID,
		Stage:               res.stage,
		Provider:            cfg.Agent.Provider,
		Engine:              cfg.Review.Engine,
		Decision:            res.gate.Decision,
		Reasons:             res.gate.Reasons,
		FixCycles:           res.cycles,
		ExecutionMode:       string(spec.ExecutionMode),
		VerifiedFirst:       res.verifiedFirst,
		Validation:          res.suite.Results,
		AlreadySatisfied:    res.satisfaction,
		Completion:          completion,
		RepositoryMutations: mutations,
		Findings:            res.report.Findings,
		// JEV is diagnostic evidence, reported in its own section and pointing at
		// the persisted artifact. It is separate from the validation and review
		// sections, which remain authoritative.
		JEV: jevReportSection(res.jevDoc, res.jevPath),
		// ModelSelection is the non-secret model-routing evidence: the selected
		// class, provider, model, locality, layer, whether a fallback was used, and
		// why. It is omitted when model routing is inactive.
		ModelSelection: res.modelSelection,
		Routing:        routingDocFor(res.routing),
		Classification: classificationDoc(res.classification),
		Performance:    res.perf,
		GeneratedAt:    time.Now().UTC(),
	})
	// Persist the canonical structured run trace beside the report. It composes the
	// evidence already produced above; it is best-effort observation and never
	// changes the run outcome.
	writeRunTrace(rn, res, cfg, runtrace.CollectorFromContext(ctx))
	return res, nil
}

// autonomyDoc returns a pointer to the autonomy decision for the report, or nil
// when no decision was made (a passing run), so an existing PASS report is
// unchanged.
func autonomyDoc(d autonomy.Decision) *autonomy.Decision {
	if d.Action == "" {
		return nil
	}
	return &d
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

// writeClassificationArtifact persists the failure classification and the
// autonomy decision as the run's classification artifact. It embeds the
// classification at the top level, so existing readers of classification.json are
// unaffected, and adds the decision as provenance. It is best-effort: a write
// failure never changes the run outcome.
func writeClassificationArtifact(rn *runpkg.Run, cls failure.Classification, decision autonomy.Decision) {
	artifact := classificationArtifact{Classification: cls, Autonomy: autonomyDoc(decision)}
	data, err := json.MarshalIndent(artifact, "", "  ")
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
func runStages(ctx context.Context, dir string, cfg config.Config, a agent.Agent, d deps, spec *taskfile.Spec, rn *runpkg.Run, sess *runSession, approval failure.ApprovalBoundary, tri earlyGateResult, stdout io.Writer) (res lifeResult, err error) {
	reports := taskReportDeliverables(spec)
	if rawOutputEvidence(spec) {
		_ = os.Setenv(envCommandEvidenceLog, filepath.Join(rn.Dir(), "command-evidence.jsonl"))
		defer func() { _ = os.Unsetenv(envCommandEvidenceLog) }()
	}
	if len(reports) > 0 {
		readDiff := d.readDiff
		d.readDiff = func(ctx context.Context, dir string) (string, error) {
			diff, err := readDiff(ctx, dir)
			if err != nil {
				return "", err
			}
			declared, err := git.New(dir).DiffFiles(ctx, reports...)
			return diff + declared, err
		}
	}
	rec := perf.NewRecorder(rn.State().ID)
	// trouting holds the per-task routing decision once it is made, so the deferred
	// assignment records it even when the lifecycle returns through a literal.
	var trouting *taskRouting
	defer func() {
		res.perf = rec.Task()
		_ = writeMetrics(rn, res.perf)
		res.routing = trouting
	}()

	// ar observes the lifecycle for the activity stream. It is nil when reporting
	// is disabled (or no consumer is registered), and every emit on it is a no-op,
	// so it never affects execution.
	ar := activity.FromContext(ctx)

	// Ordinary IMPLEMENT tasks need governed mutation or verified completion
	// evidence. That evidence may span earlier invocations of the same task.
	// A verify-first task proves acceptance with the configured validation (no
	// implementation agent of its own), and an execution_mode done stage's work is
	// declared already present, so neither requires a change here. The rule reads
	// only the task's declared execution mode; a model claim alone cannot waive it.
	changeRequired := !spec.ExecutionMode.VerifyFirst() && !spec.ExecutionMode.Done()

	var (
		plan       *planner.Plan
		diff       string
		sealed     *testrunner.SuiteResult // validation already run for a verify-first task
		failureCtx string

		// cycles counts the bounded fix cycles taken in this invocation. The gate,
		// the report, and the performance record read the same counter, so
		// "fix cycles: n/max" can never disagree with the recorded FIX count.
		cycles int
		// implMutation is SOP's OBSERVED mutation evidence for the ordinary
		// IMPLEMENT/FIX invocations, separate from accumulated task evidence.
		// Only harness-observed paths or invocation-scoped content changes count.
		// Narration, pre-existing dirty work, tool intent, a write
		// request, a test run, and a model-guessed git status are never evidence.
		implMutation     bool
		alreadySatisfied bool
		satisfaction     *agent.CompletionEvidence
		// Legacy providers may explicitly finish without changes. That completion
		// still needs actual, successful configured validation and the quality gate;
		// neither an empty suite nor an unverified ALREADY_SATISFIED claim suffices.
		legacyNoChange bool
		// pendingOutcome is a non-completed mutating (IMPLEMENT/FIX) outcome whose
		// disposition is deferred until SOP's own deterministic validation has run,
		// so structured build/test evidence outranks the agent's free-form summary.
		// It is cleared once the evidence is consumed.
		pendingOutcome *agent.Outcome
		pendingSource  string
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
		recordTaskChanges(rn, changedFiles(diff), reports...)
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
		}).Generate(ctx, d.taskInput(spec.ID, spec.Render()))
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
		// Optional early JEV pre-execution checkpoint (Phase 3): analysis of the
		// proposed execution context immediately before implementation, after the
		// precheck and planning. It is analysis-only and a strict no-op unless
		// early_jev.gates.pre_execution is enabled. A policy escalation stops the
		// task at the existing human boundary; the lifecycle itself is unchanged.
		if pre := runEarlyGate(ctx, cfg, d, spec, rn, runpkg.CheckpointPreExecution); pre.Escalate {
			_ = rn.SetStage(runpkg.WaitingForHuman)
			return lifeResult{
				gate:           fail(pre.Reason),
				stage:          runpkg.WaitingForHuman,
				classification: pre.Decision.Classification,
				decision:       pre.Decision,
			}, nil
		} else {
			// Deterministic per-task model routing (Phase 3.5). It runs immediately
			// before implementation, using the freshest evidence. It only selects the
			// model for the bounded implementation work: it never approves, blocks, or
			// bypasses a gate. A manual --model-class override always wins. Routing is
			// OFF by default, so this is a no-op unless enabled.
			ra, tr, rerr := applyTaskRouting(ctx, cfg, d, spec, tri, pre, a, stdout)
			if rerr != nil {
				return lifeResult{}, fmt.Errorf("model routing: %w", rerr)
			}
			if tr != nil {
				a, trouting = ra, tr
			}
		}

		_ = rn.SetStage(runpkg.Implementing)
		ar.Emit(activity.StageImplement, "implementing", "")
		input := plan.RenderMarkdown()
		if failureCtx != "" {
			input = failureCtx + "\n" + input
		}
		// An escalated attempt (Phase 5) is handed the previous attempt's bounded
		// context, so a stronger model starts from the actual failure instead of
		// repeating the change that did not pass.
		if d.attempt != nil && d.attempt.FailureContext != "" {
			input = d.attempt.FailureContext + "\n" + input
		}
		if sig, had := rn.ReadAttempt(); had {
			input = "# Previous attempt\n\nA previous attempt at this task did not complete:\n\n" + sig + "\n\n" + input
		}
		var before map[string]string
		if d.snapshotRepository != nil {
			before, err = d.snapshotRepository(ctx, dir)
			if err != nil {
				return lifeResult{}, fmt.Errorf("implement mutation baseline: %w", err)
			}
		}
		implStop := rec.Measure(perf.StageImplement)
		impl, err := a.Generate(ctx, agent.Request{
			Capability:         agent.Implement,
			AcceptanceCriteria: spec.AcceptanceCriteria,
			ValidationCommands: completionValidationCommands(cfg),
			Task:               spec.Render(),
			Input:              d.taskInput(spec.ID, input),
			Deliverables:       reports,
			RawOutputEvidence:  rawOutputEvidence(spec),
			OutputRequirements: "Implement the plan in the working tree and summarize the changes.",
		})
		implStop()
		rec.AgentCall()
		if err != nil {
			return lifeResult{}, fmt.Errorf("implement: %w", err)
		}
		_ = rn.Write("implementation.md", impl.Content)
		alreadySatisfied = impl.VerifiedAlreadySatisfied && impl.Outcome != nil &&
			impl.Outcome.Status == agent.OutcomeCompleted && impl.Outcome.Completion == agent.AlreadySatisfied
		if alreadySatisfied {
			satisfaction = impl.Outcome.Evidence
		}
		// Record the invocation's observed changes as task-scoped evidence even when
		// it did not complete, so the next bounded invocation (and JEV) keeps the
		// implementation the task already produced.
		recordTaskChanges(rn, impl.ChangedFiles, reports...)

		// The working tree is read before a non-completed outcome is classified. SOP's
		// own deterministic validation runs against it below, and a structured build
		// or test result outranks the agent's free-form summary: an in-progress edit
		// that broke the build is a compiler error, not an unexplained human boundary.
		diff, err = d.readDiff(ctx, dir)
		if err != nil {
			return lifeResult{}, fmt.Errorf("diff: %w", err)
		}
		paths, observed, err := invocationChanges(ctx, dir, d, before, impl.ChangedFiles, diff, reports...)
		if err != nil {
			return lifeResult{}, fmt.Errorf("implement mutation observation: %w", err)
		}
		recordTaskChanges(rn, paths, reports...)
		_ = rn.Write("diff.patch", diff)

		implMutation = observed
		legacyNoChange = impl.Outcome != nil && impl.Outcome.Status == agent.OutcomeCompleted &&
			!impl.Outcome.ChangesExpected && impl.Outcome.Completion == ""

		if impl.Outcome != nil && impl.Outcome.Status != agent.OutcomeCompleted {
			if implMutation {
				// Changes were left behind: defer the disposition until the configured
				// validation has run, so a broken build the invocation introduced is
				// classified from evidence rather than from prose.
				pendingOutcome, pendingSource = impl.Outcome, "IMPLEMENT"
			} else {
				// Nothing changed, so there is nothing to validate: the outcome's own
				// signal stands (a budget/no-change exhaustion is a continuation).
				return outcomeResult(ctx, cfg, rn, "IMPLEMENT", impl.Outcome, cycles, approval), nil
			}
		}

		// A claimed change with no current or accumulated task evidence is a failure.
		// A final invocation may instead verify earlier governed work; leave that
		// evidence available to validation/review/JEV without calling it a new change.
		changesExpected := impl.Outcome == nil || impl.Outcome.ChangesExpected
		if !implMutation && changesExpected && !alreadySatisfied && len(taskChangeEvidence(rn, reports...)) == 0 {
			_ = rn.SetStage(runpkg.Failed)
			ar.Emit(activity.StageFailed, "FAIL", "no repository changes")
			return noChangesFailure(cfg, "IMPLEMENT", "agent reported successful implementation but produced no repository changes"), nil
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
	// The autonomy policy governs whether the bounded fix loop may run at all: a
	// conservative level that withholds automatic fixes hands the failure to the
	// driver (which applies the same policy) instead of mutating the repository.
	autoPolicy := cfg.AutonomyPolicy()
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

		// A non-completed mutating outcome is classified on the deterministic
		// validation evidence just produced, never on the agent's prose alone. When
		// SOP's own build/test result confirms a fixable failure, the ambiguous
		// outcome is dropped and the existing fix loop repairs it (the gate below is
		// red, so the loop runs and review/JEV are skipped). Otherwise the outcome's
		// disposition — a continuation, a retry, or a genuine human boundary — is
		// reported, with the deterministic evidence folded in.
		if pendingOutcome != nil {
			class := pendingClassification(cfg, suite, pendingSource, pendingOutcome, cycles, approval)
			if class.Disposition == failure.AutoFix {
				pendingOutcome = nil
			} else {
				return pendingResult(ctx, cfg, rn, pendingOutcome, class, cycles), nil
			}
		}

		// Review only when validation passed, there is something to review, and the
		// change came from an implementation: a verify-first pass made no change of
		// its own, so there is nothing for this task to review.
		report = review.Report{}
		if rawOutputEvidence(spec) {
			if ierr := injectCapturedEvidence(dir, reports, rn.Dir()); ierr != nil {
				return lifeResult{}, fmt.Errorf("captured evidence: %w", ierr)
			}
		}
		if suite.Passed() && !verifiedFirst && strings.TrimSpace(diff) != "" {
			reviewTask := d.taskInput(spec.ID, spec.Render())
			reviewKey := reviewIdentity(cfg.Review.Engine, reviewTask, diff)
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
				report, err = provider.Review(ctx, review.Request{Task: reviewTask, Diff: diff})
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
		// read-only and add-only — with JEV disabled it is a no-op, so the gate sees
		// exactly what it saw before. A blocking JEV finding fails the gate and enters
		// the same bounded fix loop below; after a fix, validation, review, and JEV
		// all rerun before the gate is evaluated again. JEV invocation metrics are
		// recorded on rec as diagnostics only; they never influence the gate.
		//
		// JEV is a POST-validation engineering review: it runs only once the
		// deterministic basic validation is green. While a build/test/lint check is
		// red, the failure is deterministic and auto-fixable; running the (expensive)
		// model analysis against a known-broken tree would only add latency and noise,
		// and the pending fix would invalidate it. After the fix reruns validation,
		// JEV runs then. The evidence is cleared each iteration so a red validation
		// never leaves the gate weighing a stale JEV result.
		//
		// The task-scoped change evidence accumulated across this task's invocations
		// (not only this one) is handed to JEV, so a no-change final invocation still
		// reviews the implementation the task produced earlier.
		jevEv = nil
		if suite.Passed() {
			jevEv = runOptionalJEV(ctx, cfg, d, spec, rn, diff, suite, report, dir, rec)

			// Persist the JEV result as a run artifact beside the other diagnostics,
			// so results are available after the run. Persistence is best-effort and
			// never changes the run outcome; the same document is referenced by the
			// report. It is diagnostic evidence only: nothing here is read back to
			// drive a decision.
			jevDoc = persistedJEVDoc(rn, jevEv)
		}

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

		// Persist validation, review, and gate evidence as soon as the gate is
		// evaluated, not only after the loop. A later FIX cycle that fails before it
		// re-validates must not erase the evidence explaining why the gate failed:
		// the blocking review findings and the gate's own reasons must survive so
		// the failure is diagnosable. These are diagnostics; nothing reads them back.
		writeRunJSON(rn, "validation.json", suite)
		writeRunJSON(rn, "review.json", report)
		writeRunJSON(rn, "gate.json", gate)

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
		if !autoPolicy.AutoFix {
			// The configured autonomy level withholds automatic fixes: stop here and
			// let the driver's autonomy decision apply (it will require a human).
			break
		}

		// Fix, then re-validate and re-review (regression protection).
		_ = rn.SetStage(runpkg.Fixing)
		ar.Emit(activity.StageFix, "applying fix", "")
		cycles++
		rec.FixCycle()
		var before map[string]string
		if d.snapshotRepository != nil {
			before, err = d.snapshotRepository(ctx, dir)
			if err != nil {
				return lifeResult{}, fmt.Errorf("fix mutation baseline: %w", err)
			}
		}
		fixStop := rec.Measure(perf.StageFix)
		fix, err := a.Generate(ctx, agent.Request{
			Capability:         agent.Fix,
			AcceptanceCriteria: spec.AcceptanceCriteria,
			ValidationCommands: completionValidationCommands(cfg),
			Task:               spec.Render(),
			Input:              d.taskInput(spec.ID, fixContext(plan.RenderMarkdown(), report, suite, diff, jevEv, cfg.Quality.JEVFailOn())),
			Deliverables:       reports,
			RawOutputEvidence:  rawOutputEvidence(spec),
			OutputRequirements: "Fix the failing checks and blocking findings in the working tree and summarize the changes.",
		})
		fixStop()
		rec.AgentCall()
		if err != nil {
			return lifeResult{}, fmt.Errorf("fix: %w", err)
		}
		_ = rn.Write(fmt.Sprintf("fix-%d.md", cycles), fix.Content)
		// A later fix invalidates an earlier already-satisfied verdict. Only the
		// current proof can be used; a run that implemented changes is not a
		// zero-mutation completion merely because its final FIX needed no edit.
		alreadySatisfied = false
		satisfaction = nil
		if !implMutation && fix.VerifiedAlreadySatisfied && fix.Outcome != nil && fix.Outcome.Status == agent.OutcomeCompleted && fix.Outcome.Completion == agent.AlreadySatisfied {
			alreadySatisfied = true
			satisfaction = fix.Outcome.Evidence
		}
		legacyNoChange = fix.Outcome != nil && fix.Outcome.Status == agent.OutcomeCompleted &&
			!fix.Outcome.ChangesExpected && fix.Outcome.Completion == ""
		recordTaskChanges(rn, fix.ChangedFiles, reports...)

		// Re-read the working tree after the fix so the next classification sees what
		// the fix actually produced.
		diff, err = d.readDiff(ctx, dir)
		if err != nil {
			return lifeResult{}, fmt.Errorf("diff: %w", err)
		}
		paths, fixMutation, err := invocationChanges(ctx, dir, d, before, fix.ChangedFiles, diff, reports...)
		if err != nil {
			return lifeResult{}, fmt.Errorf("fix mutation observation: %w", err)
		}
		recordTaskChanges(rn, paths, reports...)
		implMutation = implMutation || fixMutation

		if fix.Outcome != nil && fix.Outcome.Status != agent.OutcomeCompleted {
			if fixMutation {
				// A fix that did not complete is not trusted on its own prose: re-validate
				// the tree and let structured evidence decide. The pending check at the top
				// of the loop then either continues the deterministic repair or reports the
				// outcome's disposition (a tool-budget exhaustion is a continuation).
				pendingOutcome, pendingSource = fix.Outcome, "FIX"
				continue
			}
			return outcomeResult(ctx, cfg, rn, "FIX", fix.Outcome, cycles, approval), nil
		}

		if !fixMutation && !alreadySatisfied && !legacyNoChange {
			_ = rn.SetStage(runpkg.Failed)
			ar.Emit(activity.StageFailed, "FAIL", "no repository changes")
			return noChangesFailure(cfg, "FIX", "agent reported a fix but produced no repository changes"), nil
		}
		_ = rn.Write("diff.patch", diff)
	}

	// Completion is task-scoped; mutation attribution is invocation-scoped. Prior
	// governed work remains available to the independent quality/JEV gate. Legacy
	// no-change completion requires actual green validation, not just model prose
	// or a vacuously passing empty suite. Explicit ALREADY_SATISFIED still requires
	// the trusted harness proof above. Missing evidence uses the existing bounded
	// IMPLEMENT_NO_CHANGES continuation, never an artificial mutation or PASS.
	validatedNoChange := legacyNoChange && len(suite.Results) > 0 && suite.Passed()
	if gate.Decision == quality.Pass && changeRequired && !verifiedFirst && !implMutation && !alreadySatisfied &&
		len(taskChangeEvidence(rn, reports...)) == 0 && !validatedNoChange {
		class := failure.Classify(failure.Evidence{Source: "IMPLEMENT", ChangeRequired: true, MutationObserved: false})
		return noChangesCompletion(cfg, rn, class), nil
	}

	// Classify the gate failure and apply the configured autonomy policy before the
	// stage is finalized, so the run report and the driver's recovery decision
	// record why the lifecycle stopped, what disposition it applied, and whether a
	// human is required. A pass has no failure to classify or decide.
	var class failure.Classification
	var decision autonomy.Decision
	if gate.Decision != quality.Pass {
		class = failure.Classify(verificationEvidence(cfg, suite, report, cycles, jevEv, approval))
		decision = decideAutonomy(cfg, class)
	}

	stage := runpkg.Passed
	switch {
	case decision.Action == autonomy.ActionTerminal:
		// Bounded automation exhausted: a terminal automation state, not a human
		// boundary, even though the quality gate labeled the exhausted fix loop a
		// human decision. It reports the effective action.
		stage = runpkg.Failed
		ar.Emit(activity.StageFailed, string(decision.Action), firstReason(gate))
	default:
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
	}
	if gate.Decision != quality.Pass {
		satisfaction = nil
	}
	_ = rn.SetStage(stage)
	return lifeResult{
		gate:           gate,
		cycles:         cycles,
		stage:          stage,
		suite:          suite,
		report:         report,
		verifiedFirst:  verifiedFirst,
		satisfaction:   satisfaction,
		jevDoc:         jevDoc,
		jevPath:        jevArtifactRef(dir, rn),
		classification: class,
		decision:       decision,
	}, nil
}

// verificationEvidence builds the structured evidence for a deterministic gate
// failure: which configured check failed, the blocking findings that remain, the
// fix budget, and whether JEV failed closed, so the classifier can decide whether
// the failure is safely auto-fixable, a bounded continuation, or has exhausted its
// bounded budget. A JEV analysis failure is a bounded, non-human signal: it is
// recorded so the classification preserves its reason instead of falling through
// to an "unknown" human boundary.
func verificationEvidence(cfg config.Config, suite testrunner.SuiteResult, report review.Report, cycles int, jevEv *jevRunEvidence, approval failure.ApprovalBoundary) failure.Evidence {
	buildFailed := hasCategory(suite, testrunner.Build) && !categoryPassed(suite, testrunner.Build)
	testFailed := hasCategory(suite, testrunner.UnitTest) && !categoryPassed(suite, testrunner.UnitTest)
	lintFailed := hasCategory(suite, testrunner.Lint) && !categoryPassed(suite, testrunner.Lint)
	ev := failure.Evidence{
		Source:           "VALIDATE",
		Approval:         approval,
		BuildFailed:      buildFailed,
		TestFailed:       testFailed,
		LintFailed:       lintFailed,
		Detail:           verificationDetail(suite),
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
	// Only the blocking ones, the same rule the gate and the fix loop already use
	// (quality.JEVSeverityPriority over the fail_on severities). A fail-closed JEV is
	// handled above as an analysis failure, not a finding.
	ev.BlockingFindings += quality.JEVBlockingFindings(cfg.Quality.JEVFailOn(), jevEv.gateEvidence())
	return ev
}

// pendingClassification classifies a non-completed mutating (IMPLEMENT/FIX)
// outcome against SOP's own deterministic validation result, so a structured build
// or test failure outranks the agent's free-form summary. It reuses the classifier's
// fixed evidence precedence: a red `go build` is a compiler error even when the
// agent's summary is empty, and the bounded fix budget still applies.
func pendingClassification(cfg config.Config, suite testrunner.SuiteResult, source string, outcome *agent.Outcome, cycles int, approval failure.ApprovalBoundary) failure.Classification {
	return failure.Classify(failure.Evidence{
		Source:       source,
		Outcome:      outcome,
		Approval:     approval,
		BuildFailed:  hasCategory(suite, testrunner.Build) && !categoryPassed(suite, testrunner.Build),
		TestFailed:   hasCategory(suite, testrunner.UnitTest) && !categoryPassed(suite, testrunner.UnitTest),
		LintFailed:   hasCategory(suite, testrunner.Lint) && !categoryPassed(suite, testrunner.Lint),
		Detail:       verificationDetail(suite),
		FixCycles:    cycles,
		MaxFixCycles: cfg.Quality.MaxFixCycles,
	})
}

// verificationDetail returns a short, bounded diagnostic for the first failing
// validation check (for example the compiler error line from a red `go build`), so
// a report states the concrete failure instead of relying on the agent's summary.
// It returns "" when every check passed or produced no output.
func verificationDetail(suite testrunner.SuiteResult) string {
	for _, r := range suite.Results {
		if r.Status == testrunner.Pass {
			continue
		}
		if out := strings.TrimSpace(firstNonBlank(r.Stderr, r.Stdout)); out != "" {
			return fmt.Sprintf("%s `%s`: %s", r.Category, r.Command, boundedDetail(out))
		}
		return fmt.Sprintf("%s `%s`: %s", r.Category, r.Command, r.Status)
	}
	return ""
}

// boundedDetail keeps a validation diagnostic short enough for a one-line reason.
func boundedDetail(out string) string {
	const maxLen = 400
	out = strings.Join(strings.Fields(out), " ")
	if len(out) <= maxLen {
		return out
	}
	// Trim to a rune boundary so the diagnostic never ends mid-character.
	cut := maxLen
	for cut > 0 && !utf8.RuneStart(out[cut]) {
		cut--
	}
	return out[:cut] + "…"
}

// firstNonBlank returns the first argument that carries non-whitespace text.
func firstNonBlank(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// emitRunSummary prints the run's outcome and returns the process exit code.
func emitRunSummary(stdout io.Writer, dir string, cfg config.Config, rn *runpkg.Run, res lifeResult) int {
	// The displayed outcome is the effective one: a terminal autonomy decision
	// (bounded automation exhausted) is reported as a failure, not as the gate's
	// human label, so an operator is never told a human is required when none is.
	outcome := res.gate.Decision
	if res.decision.Action == autonomy.ActionTerminal {
		outcome = quality.Fail
	}
	fmt.Fprintf(stdout, "run %s: %s\n", rn.State().ID, outcome)
	if res.verifiedFirst {
		fmt.Fprintln(stdout, "verified first: the configured validation passed; no implementation agent was invoked")
	}
	for _, reason := range res.gate.Reasons {
		fmt.Fprintf(stdout, "  - %s\n", reason)
	}
	fmt.Fprintf(stdout, "fix cycles: %d/%d\n", res.cycles, cfg.Quality.MaxFixCycles)
	writeClassification(stdout, res.classification, res.decision)
	writeAutonomySummary(stdout, res.decision)
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
func writeClassification(w io.Writer, cls failure.Classification, d autonomy.Decision) {
	if cls.Disposition == "" {
		return
	}
	// The autonomy decision is authoritative over the classifier's conservative
	// label, so the disposition shown is the effective one: a bounded automation
	// exhaustion reads TERMINAL rather than the classifier's NEEDS_HUMAN.
	disposition := string(cls.Disposition)
	if d.Action == autonomy.ActionTerminal {
		disposition = string(autonomy.ActionTerminal)
	}
	fmt.Fprintf(w, "classification: %s (%s, %s)\n", disposition, cls.Kind, cls.Confidence)
	if reason := strings.TrimSpace(cls.Reason); reason != "" {
		fmt.Fprintf(w, "  reason: %s\n", reason)
	}
}

// writeAutonomySummary prints the autonomy decision beneath the classification, so
// a reader sees the risk, the level, the resulting action, and why. It renders
// nothing when no decision was made (a passing run).
func writeAutonomySummary(w io.Writer, d autonomy.Decision) {
	if d.Action == "" {
		return
	}
	fmt.Fprintf(w, "autonomy: %s risk=%s decision=%s\n", strings.ToUpper(string(d.Level)), d.Risk, d.Action)
	if reason := strings.TrimSpace(d.Reason); reason != "" {
		fmt.Fprintf(w, "  reason: %s\n", reason)
	}
}

// fail builds a FAIL result carrying a single reason.
func fail(reason string) quality.Result {
	return quality.Result{Decision: quality.Fail, Reasons: []string{reason}}
}

// noChangesFailure is the terminal result for SOP's own deterministic verdict that a
// mutating invocation claimed success while producing no repository change. The
// verdict is CLASSIFIED (failure.NoChangesProduced) rather than left empty, so the
// failure is reported like any other and the bounded recovery policy (Phase 5) may
// escalate it to a stronger model. The AUTO_FIX disposition is an automatically
// actionable failure, so with recovery off the existing lifecycle path is unchanged:
// the task still stops and is blocked at the existing boundary.
func noChangesFailure(cfg config.Config, source, reason string) lifeResult {
	cls := failure.Classify(failure.Evidence{Source: source, NoChangesProduced: true})
	return lifeResult{gate: fail(reason), stage: runpkg.Failed, classification: cls, decision: decideAutonomy(cfg, cls)}
}

// noChangesCompletion is the authoritative finalization for a change-requiring
// IMPLEMENT task whose invocation completed against an unchanged repository with
// the configured validation green. It is NOT a pass and NOT a hard failure: the
// verdict is a retryable continuation, reported with the existing vocabulary as
// termination=no_changes, diagnostic=IMPLEMENT_NO_CHANGES, outcome=CONTINUE, so
// the driver's existing bounded requeue path continues the work on a fresh
// invocation. It is deliberately distinct from the no-progress guard's
// IMPLEMENT_NO_PROGRESS (five consecutive non-mutating turns) and shares no
// second mechanism with it; the model's changes_expected is recorded as evidence
// and is never consulted here.
func noChangesCompletion(cfg config.Config, rn *runpkg.Run, cls failure.Classification) lifeResult {
	decision := decideAutonomy(cfg, cls)
	reason := cls.Reason
	// The existing continuation path reports a retryable disposition as CONTINUE at
	// the FAILED stage, exactly as an agent-requested requeue does, so the driver
	// requeues through the same bounded recovery path rather than inventing a new
	// one.
	_ = rn.SetStage(runpkg.Failed)
	return lifeResult{
		gate:           quality.Result{Decision: quality.Continue, Reasons: []string{reason}},
		stage:          runpkg.Failed,
		classification: cls,
		decision:       decision,
	}
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

// emitClassificationActivity reports a non-empty failure classification and the
// autonomy decision on the activity stream carried by ctx. It is a no-op when
// reporting is disabled or the run passed (an empty classification).
func emitClassificationActivity(ctx context.Context, cls failure.Classification, decision autonomy.Decision) {
	if cls.Disposition == "" {
		return
	}
	activity.FromContext(ctx).Emit(activity.StageClassify, string(cls.Disposition), string(cls.Kind))
	emitAutonomyActivity(ctx, decision)
}

// The outcome-result helpers map a non-completed agent outcome to a run result and
// report it on the activity stream. SOP owns the lifecycle, so the failure
// classifier's evidence decides the disposition rather than the status label alone:
// an outcome that is a resumable continuation (budget/no-change exhaustion, a
// transient provider failure) is NOT a human boundary even though the harness
// labels it needs_human to request a requeue. Such an outcome is reported as
// CONTINUE so the operator sees SOP will continue on its own, and the driver
// requeues it through the existing bounded recovery path. Only a genuine human
// boundary (approval, safety, ambiguity) becomes NEEDS_HUMAN; any other reported
// failure becomes FAIL. A reported outcome is never treated as success. The
// classification is attached to the result, so the driver applies the same
// disposition logic it applies to a deterministic gate failure.

// outcomeReason is the short human-readable reason for a non-completed outcome:
// its explicit reason, then its summary, then its status label, so a report always
// explains why the invocation stopped.
func outcomeReason(outcome *agent.Outcome) string {
	reason := outcome.Reason
	if reason == "" {
		reason = outcome.Summary
	}
	if reason == "" {
		reason = string(outcome.Status)
	}
	return reason
}

// outcomeResult classifies a non-completed agent outcome from its own evidence
// alone and maps it to a run result. Use it when no deterministic validation
// evidence exists (for example a no-change invocation).
func outcomeResult(ctx context.Context, cfg config.Config, rn *runpkg.Run, source string, outcome *agent.Outcome, cycles int, approval failure.ApprovalBoundary) lifeResult {
	class := failure.Classify(failure.Evidence{Source: source, Outcome: outcome, Approval: approval})
	emitOutcomeActivity(activity.FromContext(ctx), outcome, class)
	return classifiedOutcome(cfg, rn, outcome, class, cycles)
}

// pendingResult maps a non-completed mutating outcome that SOP has re-classified
// with its own deterministic validation evidence. A structured, exhausted
// deterministic failure is a genuine boundary regardless of the agent's own status
// label, so it takes the same stage the gate-failure path uses; every other
// disposition keeps the outcome's own status mapping (a continuation, a retry, or
// a hard failure).
func pendingResult(ctx context.Context, cfg config.Config, rn *runpkg.Run, outcome *agent.Outcome, class failure.Classification, cycles int) lifeResult {
	emitOutcomeActivity(activity.FromContext(ctx), outcome, class)
	if class.Kind == failure.AutoFixExhausted {
		decision := decideAutonomy(cfg, class)
		reason := outcomeReason(outcome)
		stage := runpkg.WaitingForHuman
		gate := quality.Result{Decision: quality.NeedsHuman, Reasons: []string{reason}}
		if decision.Action == autonomy.ActionTerminal {
			stage = runpkg.Failed
			gate = quality.Result{Decision: quality.Fail, Reasons: []string{reason}}
		}
		_ = rn.SetStage(stage)
		return lifeResult{gate: gate, cycles: cycles, stage: stage, classification: class, decision: decision}
	}
	return classifiedOutcome(cfg, rn, outcome, class, cycles)
}

// classifiedOutcome maps an already-classified non-completed outcome to a run
// result. The autonomy policy — not the raw disposition or the harness's status
// label — decides whether a human is required, so the classification carries the
// disposition the driver acts on. A resumable continuation of a needs_human
// outcome is reported as CONTINUE (never NEEDS_HUMAN); a genuine human boundary is
// NEEDS_HUMAN; any other reported failure becomes FAIL.
func classifiedOutcome(cfg config.Config, rn *runpkg.Run, outcome *agent.Outcome, class failure.Classification, cycles int) lifeResult {
	reason := outcomeReason(outcome)
	decision := decideAutonomy(cfg, class)

	if outcome.Status == agent.OutcomeNeedsHuman && class.Retryable() {
		// A resumable continuation: the invocation did not finish, but no human
		// decision is required. Report CONTINUE (never NEEDS_HUMAN), mirroring the
		// gate-failure path where a retryable disposition is a non-pass result.
		_ = rn.SetStage(runpkg.Failed)
		return lifeResult{gate: quality.Result{Decision: quality.Continue, Reasons: []string{reason}}, cycles: cycles, stage: runpkg.Failed, classification: class, decision: decision}
	}
	// A BLOCK disposition (a bounded no-progress stop) is neither a resumable
	// continuation nor a human decision: it must NOT park the run at
	// WAITING_FOR_HUMAN, because the approval subsystem treats that stage as a
	// genuine human boundary and would create an approve/decline gate.
	if outcome.Status == agent.OutcomeNeedsHuman && class.Disposition != failure.Block {
		_ = rn.SetStage(runpkg.WaitingForHuman)
		return lifeResult{gate: quality.Result{Decision: quality.NeedsHuman, Reasons: []string{reason}}, cycles: cycles, stage: runpkg.WaitingForHuman, classification: class, decision: decision}
	}
	_ = rn.SetStage(runpkg.Failed)
	return lifeResult{gate: quality.Result{Decision: quality.Fail, Reasons: []string{reason}}, cycles: cycles, stage: runpkg.Failed, classification: class, decision: decision}
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
		if _, err := applyModelRouting(loaded, ""); err != nil {
			return config.Config{}, err
		}
		return *loaded, nil
	}
	if errors.Is(err, config.ErrNotFound) {
		cfg := config.Default()
		if _, err := applyModelRouting(&cfg, ""); err != nil {
			return config.Config{}, err
		}
		return cfg, nil
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

// rawOutputEvidence reports whether a task's contract explicitly requires raw
// command output in its deliverable. It reads the declared acceptance criteria and
// deliverables — task data — never the artifact content, so it is not a prose
// heuristic over the produced report.
// envCommandEvidenceLog names the file the agent appends captured command evidence
// to (JSONL). SOP sets it for a task that requires raw output, to a path inside the
// task's run directory, so the captured evidence is SOP-owned; it is unset for every
// other task, so nothing is captured needlessly.
const envCommandEvidenceLog = "SOP_COMMAND_EVIDENCE_LOG"

// rawOutputEvidence reports whether a task's contract explicitly requires raw
// command output in its deliverable.
func rawOutputEvidence(spec *taskfile.Spec) bool {
	for _, item := range append(append([]string{}, spec.AcceptanceCriteria...), spec.Deliverables...) {
		if strings.Contains(strings.ToLower(item), "raw output") {
			return true
		}
	}
	return false
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
	// Announce the validation commands before they run, so a slow suite is
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

// modelSelectionDoc returns the persistable, non-secret model-routing evidence
// for a run, or nil when routing is inactive. model.Selection carries no
// credential, so the artifact never records a secret.
func modelSelectionDoc(routing model.Result) *model.Selection {
	if !routing.Active {
		return nil
	}
	sel := routing.Selection
	return &sel
}

// writeModelSelectionArtifact persists the resolved model-routing evidence as a
// run artifact beside report.json, so the selected class/provider/model, the
// layer that chose it, whether a fallback was used, and why are all durable.
// It is written before the lifecycle so it survives a run that stops early. It
// is evidence only: nothing reads it back to drive a decision.
func writeModelSelectionArtifact(rn *runpkg.Run, sel model.Selection) {
	writeRunJSON(rn, "model-selection.json", sel)
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
	Completion          string                    `json:"completion,omitempty"`
	RepositoryMutations *int                      `json:"repository_mutations,omitempty"`
	AlreadySatisfied    *agent.CompletionEvidence `json:"already_satisfied,omitempty"`
	ID                  string                    `json:"id"`
	Stage               runpkg.Stage              `json:"stage"`
	Provider            string                    `json:"provider"`
	Engine              string                    `json:"review_engine"`
	Decision            quality.Decision          `json:"decision"`
	Reasons             []string                  `json:"reasons"`
	FixCycles           int                       `json:"fix_cycles"`
	ExecutionMode       string                    `json:"execution_mode"`
	VerifiedFirst       bool                      `json:"verified_first"`
	Validation          []testrunner.Result       `json:"validation"`
	Findings            []review.Finding          `json:"findings"`
	// JEV is the JEV diagnostic evidence section: a distinct, attributed summary
	// of the JEV result that points at the persisted artifact. It is separate
	// from Validation and Findings, which remain authoritative, and is omitted
	// when JEV did not run.
	JEV *jevReportDoc `json:"jev,omitempty"`
	// Performance is the task's timing/count record: diagnostic metadata only,
	// never an input to a decision.
	Performance perf.Task `json:"performance"`
	// ModelSelection is the non-secret model-routing evidence for the run: the
	// selected class, provider, model, locality, the layer that chose them,
	// whether a fallback was used, and why. It is omitted when model routing is
	// inactive, so a run without routing is unchanged.
	ModelSelection *model.Selection `json:"model_selection,omitempty"`
	// Routing is the per-task routing decision (Phase 3.5): the class, source,
	// deterministic reasons, the resolved provider/model, and the typed signals
	// used. It is omitted when no routing decision was recorded.
	Routing *routingDoc `json:"routing,omitempty"`
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
	if res.satisfaction != nil {
		b.WriteString("\n- Completion: ALREADY_SATISFIED\n- Repository mutations: 0\n")
		for _, item := range res.satisfaction.Acceptance {
			fmt.Fprintf(&b, "- Acceptance: %s — inspected %s\n", item.Criterion, strings.Join(item.Paths, ", "))
		}
	}
	if res.verifiedFirst {
		b.WriteString("- Execution: `verify-first` (validation passed; no implementation agent invoked)\n")
	}
	fmt.Fprintf(&b, "- Gate: `%s`\n\n", res.gate.Decision)
	writeModelSelectionReport(&b, res.modelSelection)
	writeRoutingReport(&b, routingDocFor(res.routing))

	b.WriteString("## Task\n\n")
	b.WriteString(strings.TrimSpace(spec.Render()))
	b.WriteString("\n\n## Validation\n\n")
	if len(res.suite.Results) == 0 {
		writeNoValidationResults(&b, cfg)
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
	writeAutonomyReport(&b, res.decision)
	return b.String()
}

// writeNoValidationResults renders the Validation section when the suite produced
// no results. It distinguishes a suite with configured checks that did not run
// (the lifecycle stopped before validation) from a project with no checks
// configured at all, so a report never claims "no validation commands
// configured" when checks were configured. The executed-results path is
// unaffected.
func writeNoValidationResults(b *strings.Builder, cfg config.Config) {
	checks := validate.Checks(cfg.Validation)
	if len(checks) == 0 {
		b.WriteString("No validation commands configured.\n")
		return
	}
	b.WriteString("Validation checks configured but NOT RUN: the lifecycle stopped before validation.\n\n")
	for _, check := range checks {
		fmt.Fprintf(b, "- NOT RUN %s `%s`\n", check.Category, check.Command)
	}
}

// writeModelSelectionReport renders the resolved model-routing evidence in the
// run report, so the class/provider/model and why they were chosen are durable.
// It renders nothing when routing is inactive.
func writeModelSelectionReport(b *strings.Builder, sel *model.Selection) {
	if sel == nil {
		return
	}
	b.WriteString("## Model selection\n\n")
	fmt.Fprintf(b, "- Class: `%s`\n", sel.Class)
	fmt.Fprintf(b, "- Provider: `%s`\n", sel.Provider)
	fmt.Fprintf(b, "- Model: `%s`\n", sel.Model)
	fmt.Fprintf(b, "- Locality: `%s`\n", sel.Locality)
	fmt.Fprintf(b, "- Source: `%s`\n", sel.Source)
	fmt.Fprintf(b, "- Reason: %s\n", sel.Reason)
	if sel.Fallback {
		b.WriteString("- Fallback: `true`\n")
	}
	b.WriteString("\n")
}

// writeRoutingReport renders the per-task routing decision in the run report, so
// the class, the resolved model, the source, the reasons, and the typed evidence
// that informed it are durable. It renders nothing when no routing decision was
// recorded.
func writeRoutingReport(b *strings.Builder, r *routingDoc) {
	if r == nil {
		return
	}
	b.WriteString("## Model routing\n\n")
	fmt.Fprintf(b, "- Class: `%s`\n", r.Class)
	fmt.Fprintf(b, "- Provider: `%s`\n", r.Provider)
	fmt.Fprintf(b, "- Model: `%s`\n", r.Model)
	fmt.Fprintf(b, "- Locality: `%s`\n", r.Locality)
	fmt.Fprintf(b, "- Source: `%s`\n", r.Source)
	if len(r.Reasons) > 0 {
		fmt.Fprintf(b, "- Reasons: %s\n", router.ReasonsText(r.Reasons))
	}
	if cps := routingCheckpointsText(r.Checkpoints); cps != "" {
		fmt.Fprintf(b, "- Checkpoints: %s\n", cps)
	}
	if ev := routingEvidenceMarkdown(r.Signals); ev != "" {
		fmt.Fprintf(b, "- Evidence: %s\n", ev)
	}
	b.WriteString("\n")
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

// completionValidationCommands is caller-owned policy, not model-selected proof.
func completionValidationCommands(cfg config.Config) []string {
	var commands []string
	for _, check := range validate.Checks(cfg.Validation) {
		commands = append(commands, check.Command)
	}
	return commands
}
