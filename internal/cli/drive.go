package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/autonomy"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/failure"
	"github.com/imhttran/agentic-sop/internal/perf"
	"github.com/imhttran/agentic-sop/internal/planflow"
	"github.com/imhttran/agentic-sop/internal/quality"
	"github.com/imhttran/agentic-sop/internal/recovery"
	"github.com/imhttran/agentic-sop/internal/resume"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/scheduler"
	"github.com/imhttran/agentic-sop/internal/store"
	"github.com/imhttran/agentic-sop/internal/taskfile"
)

// taskSaver is the persistence slice the graph driver needs to update state.
type taskSaver interface {
	Save(task *domain.Task) error
}

// graphStore is the persistence the graph driver reads and writes.
type graphStore interface {
	List() ([]*domain.Task, error)
	Save(task *domain.Task) error
}

// runGraph is `sop run`: the one-command workflow. It brings the project up to a
// runnable state — initializing state, compiling or generating the machine plan
// from a human PLAN/PRD, and creating tasks — then drives the task graph. planArg
// names an execution PLAN; when it is empty the project's normal plan is
// discovered. Each step is idempotent and safe to repeat.
func runGraph(planArg string, maxTasks int, stdout, stderr io.Writer, d deps) int {
	dir, ok := projectDir(d.getwd, stderr)
	if !ok {
		return exitError
	}

	if d.requireIsolation {
		if err := requirePlanIsolation(dir, planArg); err != nil {
			fmt.Fprintf(stderr, "run: %v\n", err)
			return exitError
		}
	}

	// Equivalent to `sop init`: create state and the configuration template.
	if _, warnings, err := ensureProjectInitialized(dir); err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitError
	} else {
		reportWarnings(stderr, warnings)
	}

	cfg, err := loadConfigOrDefault(dir)
	if err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitError
	}
	// Fail fast on an invalid enabled RAG configuration (RAG-SOP-003), before any
	// task is scheduled; required evidence itself is enforced per task attempt.
	if err := validateRAGConfig(cfg); err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitError
	}
	routing, err := applyModelRouting(context.Background(), &cfg, d.modelClass)
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
	if err := applyStageModels(context.Background(), &d, cfg); err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitError
	}
	// Carry the resolved evidence down to each task run so it is recorded.
	d.routing = routing
	// The tool-harness agent is built before this point in tests that inject a
	// factory; the production factory receives the project's independent Cargo
	// permission here so agent-tool authorization is wired, not only unit-tested.
	applyCargoAgentTool(d, cfg)
	if cfg.Workflow.Mode != "local" {
		fmt.Fprintf(stderr, "run: workflow.mode %q is not supported by the local run; set workflow.mode: local\n", cfg.Workflow.Mode)
		return exitError
	}

	stack := resolveExecutionStack(cfg)
	fmt.Fprintln(stdout, "SOP")
	fmt.Fprintln(stdout)
	printExecutionStack(stdout, dir, cfg, stack, routing)
	fmt.Fprintln(stdout)

	st, err := store.Open(statePath(dir))
	if err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitError
	}
	defer st.Close()

	// The agent is required to execute, but not to prepare. Build it lazily so a
	// PLAN.md can still be compiled and its tasks created when the agent is not
	// configured; execution then reports the missing agent clearly.
	a, agentErr := d.newAgent(cfg.Agent.Harness, cfg.Agent.Provider, cfg.Agent.Model, cfg.Agent.Class)

	var planSource string
	if planArg != "" {
		resolved, err := planflow.ResolvePlanPath(dir, planArg)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return exitError
		}
		planSource = resolved
	}

	ctx := context.Background()
	prepared, err := planflow.Prepare(ctx, planflow.Options{
		Dir:        dir,
		PlanSource: planSource,
		Agent:      a,
		Store:      st,
		OnRepair: func(attempt int, cause error) {
			fmt.Fprintf(stdout, "plan: invalid plan returned to the agent for correction (attempt %d): %v\n", attempt, cause)
		},
	})
	if err != nil {
		// Prepare errors are actionable blocks (validation, reconciliation);
		// print them without a prefix so they stand on their own.
		fmt.Fprintln(stderr, err)
		return exitError
	}

	tasks, err := st.List()
	if err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitError
	}
	printStartup(stdout, dir, cfg, stack, prepared, tasks)

	if agentErr != nil {
		fmt.Fprintf(stderr, "run: prepared the plan and tasks, but cannot execute: %v\n", agentErr)
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
		if err := validateSelectedModel(ctx, cfg, routing); err != nil {
			fmt.Fprintf(stderr, "run: %v\n", err)
			return exitError
		}
	}

	sess := newRunSession()
	sess.scoped = true
	code := driveGraph(ctx, dir, cfg, a, d, st, prepared.PlanID, sess, maxTasks, stdout, stderr)
	sess.warnEarlyJEVDown(stderr)
	if code == exitOK {
		if final, err := st.List(); err == nil && domain.AllSatisfied(final) {
			if !finalValidationGate(ctx, dir, cfg, d, sess, stderr) {
				return exitError
			}
			printCompletion(stdout, dir, cfg, prepared, final)
			historicalizeOnCompletion(dir, st, stdout, stderr)
		}
	}
	// A run that stopped at a human boundary names the gates it left behind, so an
	// operator can resolve them with `sop approvals` / `sop approve`. It prints
	// nothing when no gate is applicable, so a clean run's output is unchanged.
	printLeftoverGates(stdout, dir, st)
	return code
}

// finalValidationGate runs the full configured validation once before a plan
// whose tasks validated with verification.scoped tests reports completion. It
// keeps no state across invocations: re-running `sop run` on a satisfied plan
// retries it. The session cache reuses a pass only when the last task ran the
// full set on the same tree; after a scoped task the full set runs again.
func finalValidationGate(ctx context.Context, dir string, cfg config.Config, d deps, sess *runSession, stderr io.Writer) bool {
	if len(cfg.Verification.Scoped) == 0 {
		return true
	}
	diff, err := d.readDiff(ctx, dir)
	if err != nil {
		fmt.Fprintf(stderr, "run: final validation: diff: %v\n", err)
		return false
	}
	suite := sessionValidation(ctx, dir, cfg.Validation, diff, perf.NewRecorder("final-validation"), sess)
	if !suite.Passed() {
		fmt.Fprintf(stderr, "run: final full-suite validation failed; the plan is not complete\n%s", validationFailureContext(suite))
		return false
	}
	return true
}

// historicalizationOnCompletionEnv is the opt-in switch for `sop run`'s final
// lifecycle stage. It is OFF unless explicitly enabled, so an existing
// installation's behavior is unchanged.
const historicalizationOnCompletionEnv = "SOP_HISTORICALIZE_ON_COMPLETION"

// historicalizationOnCompletionEnabled reports whether the operator opted in.
func historicalizationOnCompletionEnabled() bool {
	v := strings.TrimSpace(os.Getenv(historicalizationOnCompletionEnv))
	if v == "" {
		return false
	}
	on, err := strconv.ParseBool(v)
	return err == nil && on
}

// historicalizeOnCompletion is the opt-in final lifecycle stage of `sop run`. It
// runs only after a run finished with every task satisfied, and only when the
// operator enabled it. It historicalizes the plan only when readiness says the
// plan is eligible: a pending human approval gate (or any other blocker) is never
// crossed, so automatic completion preserves the existing approval semantics and
// leaves the plan ACTIVE when a gate remains. It never executes a task and never
// runs a model; a historicalization problem is reported and never fails the run.
func historicalizeOnCompletion(dir string, st *store.Store, stdout, stderr io.Writer) {
	if !historicalizationOnCompletionEnabled() {
		return
	}
	ready, err := planflow.EvaluateHistoricalization(dir, st, "", "")
	if err != nil {
		fmt.Fprintf(stderr, "run: historicalization readiness: %v\n", err)
		return
	}
	if !ready.Eligible {
		fmt.Fprintf(stdout, "Historicalization: skipped (%s)\n", ready.Reason)
		return
	}
	res, err := planflow.Historicalize(planflow.HistoricalizeOptions{Dir: dir, Store: st})
	if err != nil {
		fmt.Fprintf(stderr, "run: historicalization: %v\n", err)
		return
	}
	fmt.Fprintf(stdout, "Historicalized (%s): %s\n", res.Readiness.Disposition, res.Archived)
}

// projectName is the configured project name, or the directory's base name.
func projectName(dir string, cfg config.Config) string {
	if name := strings.TrimSpace(cfg.Project.Name); name != "" {
		return name
	}
	return filepath.Base(dir)
}

// printStartup emits a concise summary of the plan being executed. The execution
// stack is printed earlier before plan preparation.
func printStartup(w io.Writer, dir string, cfg config.Config, stack executionStack, prepared planflow.Result, tasks []*domain.Task) {
	source := prepared.Source
	if source == "" {
		source = "(existing plan.json)"
	}

	fmt.Fprintf(w, "Source: %s\n", source)

	if prepared.PlanRebuilt {
		if prepared.PlanID != "" {
			fmt.Fprintf(w, "Plan ID: %s\n", prepared.PlanID)
		}
		fmt.Fprintln(w)
		if prepared.SourceKind == planflow.KindPRD {
			fmt.Fprintln(w, "Generating plan...")
		} else {
			fmt.Fprintln(w, "Compiling plan...")
		}
		fmt.Fprintf(w, "Validated %d tasks.\n", len(tasks))
		if prepared.PlanDoc != "" {
			fmt.Fprintf(w, "Wrote %s.\n", prepared.PlanDoc)
		}
	} else {
		fmt.Fprintln(w, "Plan: current")
		fmt.Fprintln(w)
		fmt.Fprintf(w, "Tasks: %d\n", len(tasks))
		done, notRequired, ready, blocked := statusCounts(tasks)
		fmt.Fprintf(w, "Done: %d\nNot required: %d\nReady: %d\nBlocked: %d\n", done, notRequired, ready, blocked)
	}

	if prepared.TasksCreated > 0 {
		fmt.Fprintf(w, "Created %d task(s).\n", prepared.TasksCreated)
		// A stage the plan declares done is recorded as already satisfied rather than
		// executed, so say so: the run's summary must never look like work happened.
		if n := countDeclaredDone(tasks); n > 0 {
			fmt.Fprintf(w, "Declared done in the plan: %d task(s) (recorded complete, no work run).\n", n)
		}
	}
	fmt.Fprintln(w)
}

// countDeclaredDone tallies the tasks whose plan stage declared the work already
// exists (execution_mode: done), so a declared completion is visible in the run
// summary instead of being indistinguishable from work the run performed.
func countDeclaredDone(tasks []*domain.Task) int {
	n := 0
	for _, task := range tasks {
		if task.ExecutionMode.Done() && task.IsCompleted() {
			n++
		}
	}
	return n
}

// statusCounts tallies completed, not-required, ready, and blocked tasks.
func statusCounts(tasks []*domain.Task) (done, notRequired, ready, blocked int) {
	for _, task := range tasks {
		switch {
		case task.IsCompleted():
			done++
		case task.Status == domain.NOT_REQUIRED:
			notRequired++
		case task.Status == domain.READY:
			ready++
		case task.IsBlocked():
			blocked++
		}
	}
	return done, notRequired, ready, blocked
}

// guardCapability rejects a provider that cannot serve a capability the run
// needs, before any work is attempted.
func guardCapability(a agent.Agent, capability agent.Capability) error {
	caps := agent.CapabilitiesOf(a)
	if caps.Supports(capability) {
		return nil
	}
	return fmt.Errorf("provider cannot %s (supported: %s); configure a provider that supports it", capability, caps.String())
}

// outcomeSignature identifies an attempt's outcome so a repeat can be detected.
// It is human-readable, because the recorded signature also becomes the context a
// retried task is given for why the previous attempt stopped.
func outcomeSignature(gate quality.Result) string {
	return string(gate.Decision) + ": " + strings.Join(gate.Reasons, "; ")
}

// printCompletion emits the completion summary once every task is done.
func printCompletion(w io.Writer, dir string, cfg config.Config, prepared planflow.Result, tasks []*domain.Task) {
	fmt.Fprintln(w, "SOP COMPLETE")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Project: %s\n", projectName(dir, cfg))
	if prepared.Source != "" {
		fmt.Fprintf(w, "Source: %s\n", prepared.Source)
	}
	done, notRequired, _, _ := statusCounts(tasks)
	fmt.Fprintf(w, "Tasks: %d/%d complete", done, len(tasks))
	if notRequired > 0 {
		fmt.Fprintf(w, ", %d not required", notRequired)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Final gate: PASS")
	if report := latestReportPath(dir); report != "" {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Report:")
		fmt.Fprintln(w, report)
	}
}

// printFailedRecovery reports that a blocked task could not be recovered and the
// run has stopped. It names the task and its original failure diagnostic
// (BlockedReason), and points at the run report when one exists — never claiming
// a report location that is not present. It performs no cleanup or state reset.
func printFailedRecovery(w io.Writer, dir string, task *domain.Task) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "SOP STOPPED: automatic recovery failed")
	fmt.Fprintln(w)
	if task == nil {
		fmt.Fprintln(w, "A task that was automatically recovered is blocked again.")
	} else {
		fmt.Fprintf(w, "Task: %s", task.ID)
		if task.Title != "" {
			fmt.Fprintf(w, " %s", task.Title)
		}
		fmt.Fprintln(w)
		if task.BlockedReason != domain.NO_REASON {
			fmt.Fprintf(w, "Reason: %s\n", task.BlockedReason)
			if task.BlockedDetail != "" {
				fmt.Fprintf(w, "Missing: %s\n", task.BlockedDetail)
			}
		}
		fmt.Fprintf(w, "Status: %s\n", task.Status)
	}
	fmt.Fprintln(w, "No later task was executed.")

	report := latestReportPath(dir)
	if report != "" {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Report:")
		fmt.Fprintln(w, report)
	}
}

// latestReportPath returns the newest run report path, or "".
func latestReportPath(dir string) string {
	runsRoot := filepath.Join(dir, stateDirName, "runs")
	id, err := latestRun(runsRoot)
	if err != nil || id == "" {
		return ""
	}
	candidate := filepath.Join(runsRoot, id, "report.md")
	present, err := exists(candidate)
	if err != nil || !present {
		return ""
	}
	return filepath.Join(stateDirName, "runs", id, "report.md")
}

// driveGraph selects ready tasks with the scheduler and runs the local lifecycle
// for each, completing a task (gate passed) or blocking it, until no runnable
// work remains. When no runnable work remains but a recoverable BLOCKED task
// exists, the scheduler requeues it (the same legal transition an explicit
// `sop retry` performs) and the driver re-selects it as runnable work, so the
// blocked task is automatically re-evaluated inside one invocation. The
// scheduler recovers each blocked task at most once per invocation, and its
// recovery guard lives only as long as this invocation, so no unbounded retry
// loop is possible and no recovery state leaks between runs.
//
// Recovery is fail-closed: when a task the scheduler already recovered is BLOCKED
// again, the scheduler reports FailedRecovery and the driver stops the run with a
// non-zero status, naming the failed task and its original failure diagnostic.
// No later task is executed, and nothing is cleaned up or reset.
//
// Completion is local: it records LOCAL_DONE through the local lifecycle
// (BRANCH_CREATED … REVIEW_PASS → LOCAL_DONE). It never fabricates PR_OPEN,
// CI_RUNNING, CI_PASS, or MERGED, because no PR was opened and no CI ran. The
// persisted state therefore describes what actually happened.
func driveGraph(ctx context.Context, dir string, cfg config.Config, a agent.Agent, d deps, st graphStore, planID string, sess *runSession, maxTasks int, stdout, stderr io.Writer) int {
	tasks, err := st.List()
	if err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitError
	}
	if len(tasks) == 0 {
		fmt.Fprintln(stderr, "run: no tasks to execute")
		return exitError
	}

	// The run's performance aggregate. It is diagnostic metadata only: it is
	// written beside the task runs and never read back to drive a decision.
	run := perf.NewRun(time.Now())
	defer func() {
		run.Finish(time.Now())
		writeRunMetrics(dir, planID, run)
	}()

	// The scheduler carries this invocation's automatic-recovery guard: it is
	// created here, used only here, and discarded when driveGraph returns.
	sch := scheduler.New(st)
	completed := 0
	tasksEntered := 0
	sawBlock := false
	var blockedIDs []string

	// Each iteration either completes a task (→ LOCAL_DONE), resumes and completes
	// an interrupted one, blocks one, or requeues one blocked task for recovery
	// (which the next iteration then selects as runnable work). The driver does not
	// keep a parallel recovery set: whether a task was already recovered is asked
	// of the scheduler (sch.RecoveredAlready), so the driver's re-selection is gated
	// on exactly the same guard the scheduler uses to fail closed. The scheduler
	// recovers each task at most once, and the driver's re-selection after a
	// recovered task re-blocks consumes the scheduler's guard, so each task
	// contributes a bounded number of iterations. The hard bound below is a backstop
	// that fails closed rather than silently reporting success.
	const iterationsPerTask = 3
	maxIterations := iterationsPerTask*len(tasks) + 2

	for i := 0; i < maxIterations; i++ {
		// Operator execution bound (--max-tasks): stop before selecting the next
		// task, so a bounded invocation never enters (or even promotes to READY)
		// task N+1. The bound counts tasks entered, not successful completions, and
		// only trims successful continuation: a failure, block, or approval boundary
		// has already returned through the existing path above.
		if maxTasks > 0 && tasksEntered >= maxTasks {
			fmt.Fprintf(stdout, "Bounded run: execution bound reached after %d task(s); stopping before the next task.\n", tasksEntered)
			fmt.Fprintf(stdout, "Remaining tasks are unchanged and normally runnable; inspect with sop status and continue with sop run.\n")
			return exitOK
		}
		// ONESHOT-005: when this invocation has parked a task-local block and no
		// independent runnable task remains, stop WITHOUT calling Next, so the
		// scheduler does not requeue (and thereby overwrite) the blocked task.
		if len(blockedIDs) > 0 && !sch.HasRunnable() {
			return reportBlockedTasks(stderr, blockedIDs)
		}

		res, err := sch.Next(ctx)
		if err != nil {
			fmt.Fprintf(stderr, "run: %v\n", err)
			return exitError
		}

		switch res.Outcome {
		case scheduler.ReadyTask:
			id := res.Task.ID
			attempt := res.Task.Attempt
			tasksEntered++
			code := runScheduledTask(ctx, dir, cfg, a, d, st, res.Task, sess, stdout, stderr)
			addTaskMetrics(run, dir, res.Task.ID)
			if code != exitOK {
				if code == exitSetupBlocker || code == exitApprovalBlocked {
					// A setup or approval gate leaves the task runnable by design and
					// cannot be resolved by immediately trying it again: stop now.
					return exitError
				}
				if sch.RecoveredAlready(id) {
					// The task was auto-recovered earlier in this invocation and is
					// BLOCKED again: re-select so the scheduler reports the failed
					// recovery fail-closed, naming the task. Because the scheduler
					// has already recovered this task, the next Next call cannot
					// recover it again; it must return FailedRecovery (or another
					// terminal outcome), so this re-selection happens at most once
					// per recovered task. A normal task that blocks is not
					// re-selected, so no later work runs after any block.
					continue
				}
				if stage, ok := runpkg.Load(dir, id); ok && (stage == runpkg.WaitingForHuman || stage == runpkg.WaitingForPlanner) {
					return code
				}
				if retriedInRun(dir, st, id, attempt) {
					// Recovery requeued the task as a RETRY and spent an attempt on it:
					// select it again now instead of ending the run, since a fresh
					// attempt can clear a stalled or empty model response. The attempt
					// budget bounds this, and an exhausted budget blocks the task. A
					// requeue that spent nothing (an unchanged repeat, a continuation)
					// still stops below, so this can never loop without spending budget.
					fmt.Fprintf(stdout, "Retrying: %s (attempt %d of its budget)\n", id, attempt+1)
					tasksEntered-- // a retry is the same task, not a new one for --max-tasks
					maxIterations++
					continue
				}
				if !taskIsBlocked(st, id) {
					return code
				}
				sawBlock = true
				blockedIDs = append(blockedIDs, id)
				fmt.Fprintf(stdout, "Blocked: %s (task-local failure); continuing with independent tasks\n", id)
				continue
			}
			completed++
		case scheduler.RecoveredTask:
			// The scheduler requeued a BLOCKED task to PLANNED via the existing
			// retry transition (the same one `sop retry` uses). Report it and
			// re-select so it is picked up as ordinary runnable work; the
			// scheduler never recovers it again in this invocation.
			fmt.Fprintf(stdout, "Recovering: %s was blocked; re-evaluating it (automatic recovery)\n", res.Task.ID)
			continue
		case scheduler.FailedRecovery:
			// A task already recovered in this invocation is BLOCKED again: the
			// automatic recovery failed. Stop the run, name the task and its
			// original failure diagnostic, and point at the run report if one
			// exists. Nothing is cleaned up or reset.
			printFailedRecovery(stderr, dir, res.Task)
			return exitError
		case scheduler.ActiveTask:
			tasksEntered++
			// A task is mid-lifecycle (typically selected but interrupted). SOP has one
			// interpretation of that state: resume the same task, never start another.
			code, id := resumeActiveTask(ctx, dir, cfg, a, d, st, sess, stdout, stderr)
			if id != "" {
				addTaskMetrics(run, dir, id)
			}
			if code != exitOK {
				return code
			}
			completed++
		case scheduler.AllDone:
			fmt.Fprintf(stdout, "all tasks done (%d completed this run)\n", completed)
			if sawBlock {
				return reportBlockedTasks(stderr, blockedIDs)
			}
			return exitOK
		default:
			fmt.Fprintf(stdout, "no runnable task (%s)\n", res.Outcome)
			if sawBlock {
				return reportBlockedTasks(stderr, blockedIDs)
			}
			return exitOK
		}
	}

	// The bound is a backstop: if the driver ever exhausts it without a terminal
	// scheduler outcome, fail closed instead of claiming success, so an unbounded
	// or mis-bounded loop can never be reported as a completed run.
	fmt.Fprintf(stderr, "run: made no progress after %d scheduling steps; stopping without reporting success\n", maxIterations)
	return exitError
}

// addTaskMetrics folds a task's persisted performance record into the run
// aggregate. Metrics are diagnostic only; a missing record is skipped.
func addTaskMetrics(run *perf.Run, dir, taskID string) {
	if t, ok := loadTaskMetrics(dir, taskID); ok {
		run.Add(t)
	}
}

// loadTaskMetrics reads a task's persisted performance record, if any.
func loadTaskMetrics(dir, taskID string) (perf.Task, bool) {
	data, err := os.ReadFile(filepath.Join(dir, stateDirName, "runs", taskID, "metrics.json"))
	if err != nil {
		return perf.Task{}, false
	}
	var t perf.Task
	if err := json.Unmarshal(data, &t); err != nil {
		return perf.Task{}, false
	}
	return t, true
}

// writeRunMetrics persists the run-level aggregate beside the task runs, under the
// plan's identity. It is a diagnostic artifact, never workflow state.
func writeRunMetrics(dir, planID string, run *perf.Run) {
	if strings.TrimSpace(planID) == "" || len(run.Tasks) == 0 {
		return
	}
	runDir := filepath.Join(dir, stateDirName, "runs", planID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return
	}
	data, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(runDir, "metrics.json"), append(data, '\n'), 0o644)
}

// resumeActiveTask resumes the single in-flight task instead of refusing to run.
// It reuses the notions the rest of SOP uses: the scheduler decides which status
// occupies the execution slot (scheduler.IsActive), and resume decides the next
// legal action for that status (resume.ActionFor) — the same decision `sop resume`
// reports. A local run only performs the local lifecycle, so a task parked in the
// remote lifecycle is reported rather than guessed at.
func resumeActiveTask(ctx context.Context, dir string, cfg config.Config, a agent.Agent, d deps, st graphStore, sess *runSession, stdout, stderr io.Writer) (int, string) {
	tasks, err := st.List()
	if err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitError, ""
	}

	var active []*domain.Task
	for _, task := range tasks {
		if scheduler.IsActive(task.Status) {
			active = append(active, task)
		}
	}
	switch len(active) {
	case 0:
		fmt.Fprintln(stderr, "Cannot safely resume: a task was scheduled as active but none was found.\n\nInspect state with:\n\n  sop resume")
		return exitError, ""
	case 1:
	default:
		ids := make([]string, len(active))
		for i, t := range active {
			ids[i] = t.ID
		}
		fmt.Fprintf(stderr, "Cannot safely resume: %d tasks are in flight (%s).\n\nResolve them explicitly, for example with `sop resume <task-id>`.\n", len(active), strings.Join(ids, ", "))
		return exitError, ""
	}

	task := active[0]
	stage, hasStage := runpkg.Load(dir, task.ID)
	action, ok := resume.ActionFor(task.Status)
	if !ok {
		printCannotResume(stderr, task, stage, hasStage, "", fmt.Sprintf("status %s has no defined next action", task.Status))
		return exitError, task.ID
	}

	// The run's lifecycle already passed its gates, or the task is parked after the
	// local gates: finish it without restarting implementation.
	if (hasStage && stage == runpkg.Passed) || action == resume.Review || action == resume.OpenPR {
		if err := completeTask(st, task); err != nil {
			fmt.Fprintf(stderr, "run: %v\n", err)
			return exitError, task.ID
		}
		fmt.Fprintf(stdout, "Resumed: %s %s (already past the local gates; completed locally)\n", task.ID, task.Title)
		return exitOK, task.ID
	}

	if action == resume.CreateBranch || action == resume.WriteTests || action == resume.VerifyRed || action == resume.Implement {
		return runScheduledTask(ctx, dir, cfg, a, d, st, task, sess, stdout, stderr), task.ID
	}

	// The remaining actions (PollCI, Merge, Finish) are the remote lifecycle. From
	// PR_OPEN/CI_RUNNING/CI_PASS the state machine only reaches a terminal through
	// MERGED, and a local run opens no PR and runs no CI, so it genuinely cannot
	// finish such a task. Report it precisely instead of guessing.
	printRemoteParked(stderr, task, stage, hasStage, action)
	return exitError, task.ID
}

// printRemoteParked explains that an active task is mid remote lifecycle, which a
// local run cannot complete.
func printRemoteParked(stderr io.Writer, task *domain.Task, stage runpkg.Stage, hasStage bool, action resume.Action) {
	fmt.Fprintf(stderr, "Cannot resume active task %s in a local run.\n\n", task.ID)
	fmt.Fprintf(stderr, "Persisted status: %s\n", task.Status)
	if hasStage {
		fmt.Fprintf(stderr, "Persisted stage: %s\n", stage)
	}
	fmt.Fprintf(stderr, "Next action: %s (remote: a local run opens no PR and runs no CI)\n", action)
	fmt.Fprintf(stderr, "\n%s is mid remote lifecycle. From %s the state machine reaches a terminal only\nthrough MERGED, so a local run cannot complete it. Finish it with the remote\n(GitHub/PR) flow, or reconcile it explicitly:\n\n  sop resume %s\n", task.ID, task.Status, task.ID)
}

// printCannotResume reports an active task that cannot safely be resumed, naming
// the task, what is known about it, and the recovery command.
func printCannotResume(stderr io.Writer, task *domain.Task, stage runpkg.Stage, hasStage bool, action resume.Action, reason string) {
	fmt.Fprintf(stderr, "Cannot safely resume active task %s.\n\n", task.ID)
	fmt.Fprintf(stderr, "Persisted status: %s\n", task.Status)
	if hasStage {
		fmt.Fprintf(stderr, "Persisted stage: %s\n", stage)
	}
	fmt.Fprintf(stderr, "Reason: %s.\n", reason)
	if action != "" {
		fmt.Fprintf(stderr, "Next action: %s\n", action)
	}
	fmt.Fprintf(stderr, "\nInspect and resume explicitly:\n\n  sop resume %s\n", task.ID)
}

// parkRunAtHumanBoundary parks a run at the EXISTING WAITING_FOR_HUMAN stage, the
// same stage the lifecycle's own human boundary sets, before a caller records an
// approval request. It keeps the persisted run stage consistent with the reported
// human boundary without introducing a new task state. The write is best-effort
// (stage persistence is diagnostic and must never fail the run), but a failure is
// surfaced as a diagnostic so the persisted stage cannot silently disagree with
// the boundary the operator was told about.
func parkRunAtHumanBoundary(rn *runpkg.Run, taskID string, stderr io.Writer) {
	if err := rn.SetStage(runpkg.WaitingForHuman); err != nil {
		fmt.Fprintf(stderr, "%s: could not persist the WAITING_FOR_HUMAN run stage: %v\n", taskID, err)
	}
}

// runScheduledTask runs the lifecycle for one scheduled task and updates its
// persisted state.
func runScheduledTask(ctx context.Context, dir string, cfg config.Config, a agent.Agent, d deps, saver taskSaver, task *domain.Task, sess *runSession, stdout, stderr io.Writer) int {
	spec := specFromTask(task)
	spec.Deliverables = planTaskDeliverables(dir, task)
	spec.DeliverableGroups = deliverableGroups(spec)
	// Plan-declared acceptance checks are recovered through the same plan-stage
	// reconciliation as the deliverables and projected into the spec, so the
	// existing deriveOutcome/authorizeAcceptanceCommand/runAcceptanceArgv seam
	// evaluates them. A missing/mismatched plan yields nil, and a task with no
	// declared checks behaves exactly as before.
	spec.AcceptanceChecks = planTaskAcceptanceChecks(dir, task)
	spec.AcceptanceMapping = planTaskAcceptanceMapping(dir, task)
	fmt.Fprintf(stdout, "Running: %s %s\n", task.ID, task.Title)
	// SOP-002 preflight: a setup blocker stops here, before the required-evidence
	// retrieval and before any model call, so it changes neither the task's attempt
	// count nor its continuation budget. It runs first because a policy/prerequisite
	// mismatch must not trigger retrieval or any other task work.
	if code := runPreflight(dir, PreflightDeclarationsFromConfig(cfg, spec.AcceptanceChecks), stdout, stderr); code != exitOK {
		return exitSetupBlocker
	}
	// Approval-safe recovery: a persisted PENDING or DECLINED decision blocks
	// execution before retrieval, a run reset, or any model work. An APPROVED
	// decision proceeds; approving never edits state.db or resets a budget.
	if code := approvalExecutionGate(dir, task, stdout, stderr); code != exitOK {
		return code
	}
	// Required evidence (RAG-SOP-003 4B-2A2b): ONE attempt-scoped retrieval, validated
	// before the task consumes any evidence or runs its agent. A task with no declared
	// required evidence is a no-op, so existing behavior is unchanged.
	blocked, code, attempt := enforceRequiredEvidenceOnce(ctx, cfg, saver, task, spec, d, stdout, stderr)
	if blocked {
		return code
	}
	// Carry the verified attempt through the execution chain on the attempt-local
	// deps copy (never a global). implementContext reads it in Phase 4B-2A3.
	d.ragAttempt = attempt
	d.attemptNumber = task.Attempt
	// Planner gate: a task parked at WAITING_FOR_PLANNER is resumed, not restarted.
	// It runs BEFORE the destructive run setup so a parked run is neither reset nor
	// advanced into a new attempt.
	if handled, code := plannerLifecycleGate(ctx, dir, cfg, a, d, saver, task, sess, stdout, stderr); handled {
		return code
	}
	// The previous invocation's run stage is read before New resets state.json, so
	// a task parked at WAITING_FOR_HUMAN stays a human gate on re-entry.
	priorStage, _ := runpkg.Load(dir, task.ID)
	rn, err := runpkg.New(dir, task.ID)
	if err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitError
	}
	// The current run's structured approval boundary: an active approval request
	// recorded for this task, or a persisted WAITING_FOR_HUMAN stage. It is the only
	// input that can produce APPROVAL_REQUIRED; agent prose never can.
	approval := currentApprovalBoundary(rn, priorStage)
	// A task parked at a human boundary names its own gate on the way out. The
	// deferred print reads the request SOP recorded (never a task status or prose) and
	// prints the commands that resolve it, so the run summary comes first. Presentation
	// only: it changes neither the task state nor the run's outcome.
	parked := false
	defer func() {
		if !parked {
			return
		}
		if v, ok := rn.Approval(); ok && v.Status == domain.ApprovalPending {
			printParkedHumanGate(stdout, v.TaskID, v.Stage, v.Reason, "sop run", true)
		}
	}()
	// Attach this task's activity stream (a no-op when reporting is disabled) so
	// the lifecycle and the in-process agent report what they are doing while it
	// runs. The recorder carries the task id, so events stay attributed even
	// though the agent is shared across tasks.
	ctx = taskActivityContext(ctx, rn.Dir(), stdout, task.ID, task.Title)
	_ = rn.Write("task.md", spec.Render())

	// Optional early JEV task-triage checkpoint (Phase 3): a deterministic policy
	// check on the already-chosen task, run before the lifecycle begins. It is a
	// strict no-op unless early_jev.gates.task_triage is enabled. A policy
	// escalation stops through the existing human boundary and introduces no new
	// task state.
	tri := runEarlyGate(ctx, cfg, d, dir, spec, rn, runpkg.CheckpointTaskTriage)
	sess.noteEarlyJEV(tri)
	if tri.Escalate {
		fmt.Fprintf(stdout, "%s NEEDS_HUMAN (early JEV triage)\n  %s\n", task.ID, tri.Reason)
		// Park the run at the existing WAITING_FOR_HUMAN stage before recording the
		// approval request, so the persisted run state matches the boundary — the
		// same stage the lifecycle's own human boundary sets. No new task state is
		// introduced.
		parkRunAtHumanBoundary(rn, task.ID, stderr)
		recordHumanApprovalRequest(ctx, dir, rn, task, runpkg.WaitingForHuman, failure.NeedsHuman, tri.Reason, tri.Reason)
		parked = true
		return recoverTask(saver, task, rn, "TRIAGE|NEEDS_HUMAN", "NEEDS_HUMAN", failure.NeedsHuman, failure.Classification{Disposition: failure.NeedsHuman, Reason: tri.Reason}, cfg.AutonomyPolicy(), tri.Reason, stdout, stderr)
	}

	// Between-task context compression (RBV-011): include the handoff capsules of the
	// task's direct dependencies when opted in. It reads only the store; a store without
	// handoffs yields nothing, so a disabled feature is a strict no-op.
	if cfg.ContextEfficiency.Handoff {
		d.handoffItems = dependencyHandoffItems(task, saver)
	}
	res, err := runAttempts(ctx, dir, cfg, a, d, spec, rn, sess, approval, tri, stdout)
	emitClassificationActivity(ctx, res.classification, res.decision)

	// A structured planner request parks THIS run at WAITING_FOR_PLANNER. It is not a
	// human gate and not a failure: the run stops and waits for a planner answer.
	if res.stage == runpkg.WaitingForPlanner {
		fmt.Fprintf(stdout, "%s PAUSED: WAITING_FOR_PLANNER (awaiting a planner answer)\n", task.ID)
		return exitError
	}

	// A genuine human boundary (the classifier's human disposition, the autonomy
	// policy's human decision, or the WAITING_FOR_HUMAN stage) records an explicit,
	// resolvable approval request, so a client resolves the gate through SOP rather
	// than inferring one from a task status. It is provenance only: it never changes
	// the task status or the run outcome.
	if err == nil && humanBoundary(res.stage, res.classification, res.decision) {
		recordHumanApprovalRequest(ctx, dir, rn, task, res.stage, res.classification.Disposition,
			firstNonBlank(firstReason(res.gate), res.classification.Reason), res.classification.Reason)
		parked = true
	}

	// An error is an agent/infrastructure failure. Classify it and apply the
	// autonomy policy (the lifecycle may not have had enough evidence): a transient
	// failure retries (bounded), a policy-required human boundary requeues, and
	// anything else is a terminal automation block.
	if err != nil {
		cls := res.classification
		if cls.Disposition == "" {
			cls = failure.Classify(failure.Evidence{Source: "run", Err: err, Approval: approval})
			decision := decideAutonomy(cfg, cls)
			writeClassificationArtifact(rn, cls, decision)
			emitClassificationActivity(ctx, cls, decision)
			res.decision = decision
		}
		fmt.Fprintf(stderr, "%s: %v\n", task.ID, err)
		if humanBoundary(res.stage, cls, res.decision) {
			recordHumanApprovalRequest(ctx, dir, rn, task, res.stage, cls.Disposition,
				firstNonBlank(cls.Reason, err.Error()), cls.Reason)
			parked = true
		}
		if cls.Retryable() {
			return recoverTask(saver, task, rn, "ERR|"+err.Error()+"|"+string(cls.Disposition), string(cls.Disposition), cls.Disposition, cls, cfg.AutonomyPolicy(), err.Error(), stdout, stderr)
		}
		if policyForcesHuman(cls, res.decision) {
			return recoverTask(saver, task, rn, "ERR|NEEDS_HUMAN|"+err.Error(), "NEEDS_HUMAN", failure.NeedsHuman, cls, cfg.AutonomyPolicy(), err.Error(), stdout, stderr)
		}
		_ = rn.SetStage(runpkg.Failed)
		_ = blockTask(saver, task, domain.RETRIES_EXHAUSTED)
		return exitError
	}

	if code := emitRunSummary(stdout, dir, cfg, rn, res); code != exitOK {
		// The autonomy decision — not the raw disposition or the run stage — decides
		// how a non-pass outcome is recovered. A terminal decision (bounded automation
		// exhausted under the high level) wins over the stage, so an exhausted fix
		// loop stops as an automation failure instead of parking for a human.
		decision := res.decision
		if decision.Action == "" {
			decision = decideAutonomy(cfg, res.classification)
		}
		if decision.Action == autonomy.ActionTerminal {
			// The persisted block reason follows the classification: a bounded no-progress
			// stop and a missing required validation set each name their own terminal
			// operator-intervention state, so the stored diagnostic is not misreported as
			// an exhausted retry budget. Both stay terminal (no requeue here).
			if res.classification.Kind == failure.NoProgress {
				// The bounded checkpoint recovery was already spent. Record the single
				// actionable blocker report and block with the preserved diagnostic.
				max := cfg.AutonomyPolicy().MaxContinuations
				if max <= 0 {
					max = autonomy.DefaultMaxContinuations
				}
				budgets := recovery.Budgets{Continuations: recovery.Budget{Used: rn.ReadContinuations(), Max: max}}
				rec := recovery.Recovery{Attempt: task.Attempt, Class: recovery.ImplementationNoProgress, Action: recovery.ActionHuman, Reason: recovery.ReasonNoMaterialChange}
				if err := applyNoProgressBlock(saver, task, rn, rec, budgets, stdout); err != nil {
					fmt.Fprintf(stderr, "run: %v\n", err)
				}
				_ = rn.RecordAttempt(outcomeSignature(res.gate))
				return exitError
			}
			reason := domain.RETRIES_EXHAUSTED
			switch res.classification.Kind {
			case failure.ValidationNotConfigured:
				reason = domain.VALIDATION_NOT_CONFIGURED
			}
			if err := blockTask(saver, task, reason); err != nil {
				fmt.Fprintf(stderr, "run: %v\n", err)
			}
			// One failed event for the terminal failure transition. A BLOCKED task with
			// no approval request is a failure, never a human gate.
			_, _, _ = emitTransitionFailed(dir, task.ID, "", task.Attempt, true,
				string(reason), "resolve the failure reason, then retry with sop retry")
			_ = rn.RecordAttempt(outcomeSignature(res.gate))
			fmt.Fprintf(stdout, "%s BLOCKED (%s exhausted)\n", task.ID, res.classification.Kind)
			return exitError
		}
		if res.stage == runpkg.WaitingForHuman || policyForcesHuman(res.classification, decision) {
			// A human boundary is not terminal: return the task to PLANNED so a
			// later run retries it. A repeat that changed nothing does not spend
			// the budget; a progressing attempt does, bounded by max_attempts.
			return recoverTask(saver, task, rn, outcomeSignature(res.gate)+"|NEEDS_HUMAN", "NEEDS_HUMAN", failure.NeedsHuman, res.classification, cfg.AutonomyPolicy(), firstReason(res.gate), stdout, stderr)
		}
		// A retryable disposition (CONTINUE/RETRY) means required work remains and
		// no human decision is required: RETRY reuses the bounded retry budget,
		// while CONTINUE consumes the separate bounded continuation budget.
		if res.classification.Retryable() {
			disposition := res.classification.Disposition
			return recoverTask(saver, task, rn, outcomeSignature(res.gate)+"|"+string(disposition), string(disposition), disposition, res.classification, cfg.AutonomyPolicy(), firstReason(res.gate), stdout, stderr)
		}
		if err := blockTask(saver, task, domain.REVIEW_UNRESOLVED); err != nil {
			fmt.Fprintf(stderr, "run: %v\n", err)
		}
		// Record why the attempt failed so an explicit `sop retry` gives the next
		// attempt that context instead of a stale or missing note.
		_ = rn.RecordAttempt(outcomeSignature(res.gate))
		fmt.Fprintf(stdout, "%s BLOCKED\n", task.ID)
		return exitError
	}

	// SOP-003: a mapped stage whose required acceptance evidence is unmapped, failed,
	// unexecuted, or unaccepted stops at the human boundary, so no dependent task
	// starts. The boundary is recorded as an explicit approval request and the run is
	// parked at WAITING_FOR_HUMAN BEFORE the task is requeued; a second run therefore
	// sees PENDING and refuses to execute until the request is approved. A DECLINED
	// request is terminal for automatic execution.
	if res.outcome.blocksAdvancement() {
		reason := "required acceptance evidence is missing or failed: " + strings.Join(res.outcome.Reasons, "; ")
		parkRunAtHumanBoundary(rn, task.ID, stderr)
		recordHumanApprovalRequest(ctx, dir, rn, task, runpkg.WaitingForHuman, failure.NeedsHuman, reason, reason)
		parked = true
		return recoverTask(saver, task, rn, string(res.outcome.Status)+"|ACCEPTANCE|NEEDS_HUMAN", "NEEDS_HUMAN", failure.NeedsHuman, failure.Classification{Disposition: failure.NeedsHuman}, cfg.AutonomyPolicy(), reason, stdout, stderr)
	}

	if err := completeTask(saver, task); err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitError
	}
	// One completed event for the terminal success transition. The emitter dedups
	// on the deterministic event id, so a repeated drain or a replayed stage does
	// not enqueue a second event, while a new attempt does.
	_, _, _ = emitTransitionCompleted(dir, task.ID, "", task.Attempt,
		string(task.Status), "task completed locally; inspect with sop status")
	// Persist this task's handoff capsule so a dependent task's context can be
	// compressed between tasks (RBV-011). Best-effort; metadata only.
	if cfg.ContextEfficiency.Handoff {
		saveTaskHandoff(saver, task, rn, res)
	}
	fmt.Fprintf(stdout, "%s %s\n", task.ID, task.Status)
	return exitOK
}

// recoverTask returns a task to PLANNED through the existing bounded requeue
// path so a later run continues from where it stopped. It is the same mechanism
// the needs_human boundary already used: a repeat that produced the same outcome
// (signature) does not spend the attempt budget, and once max_attempts is spent
// the task is terminally BLOCKED rather than looping. label names the disposal
// for the operator; reason is a short diagnostic for the no-progress message.
//
// A CONTINUE disposition is routed to the separate continuation budget
// (continueTask) so resuming productive-but-unfinished work never consumes the
// retry budget, which is reserved for repeating a failed attempt.
func recoverTask(saver taskSaver, task *domain.Task, rn *runpkg.Run, signature, label string, disp failure.Disposition, cls failure.Classification, policy autonomy.Policy, reason string, stdout, stderr io.Writer) int {
	prev, hadPrev := rn.ReadAttempt()
	prevSignature := ""
	if hadPrev {
		prevSignature = prev
	}

	// SOP-004 production wiring: classify the failed attempt and refuse a blind
	// retry of an unchanged or discovery-only failure. The pure recovery package owns
	// the decision; this seam records its reason/change and applies the terminal
	// no-progress outcome. A classification without a kind (a pure human boundary)
	// keeps the existing path unchanged. A CONTINUE is owned by the separate
	// continuation budget (including repeated discovery-only continuations), so it is
	// not converted into a terminal failure here.
	if cls.Kind != "" && disp != failure.Continue {
		if cls.Disposition == "" {
			cls.Disposition = disp
		}
		maxContinuations := policy.MaxContinuations
		if maxContinuations <= 0 {
			maxContinuations = autonomy.DefaultMaxContinuations
		}
		budgets := recovery.Budgets{Continuations: recovery.Budget{Used: rn.ReadContinuations(), Max: maxContinuations}}
		base := recovery.Evidence{Kind: cls.Kind, Disposition: cls.Disposition, Attempt: task.Attempt}
		dec, rec := recovery.Recover(recovery.Policy{}, recoveryEvidence(rn, cls, signature, priorNoProgressRecoveries(rn)), prevSignature, base, budgets.Continuations)
		if recoverRefusesRetry(dec) {
			if err := applyNoProgressBlock(saver, task, rn, rec, budgets, stdout); err != nil {
				fmt.Fprintf(stderr, "run: %v\n", err)
			}
			return exitError
		}
		if dec.Action != recovery.ActionNone {
			if err := recordRecovery(rn, rec); err != nil {
				fmt.Fprintf(stderr, "run: %v\n", err)
			}
		}
	}

	if disp == failure.Continue {
		return continueTask(saver, task, rn, signature, reason, policy, stdout, stderr)
	}

	_ = rn.RecordAttempt(signature)

	if hadPrev && prev == signature {
		if err := requeueTask(saver, task, false); err != nil {
			fmt.Fprintf(stderr, "run: %v\n", err)
			return exitError
		}
		fmt.Fprintf(stdout, "%s %s (no change since the previous attempt: %s)\n", task.ID, label, reason)
		return exitError
	}

	if err := requeueTask(saver, task, true); err != nil {
		if errors.Is(err, domain.ErrRetryExhausted) {
			reason := domain.RETRIES_EXHAUSTED
			if cls.Kind == failure.NoProgress {
				reason = domain.NO_PROGRESS
			}
			if berr := blockTask(saver, task, reason); berr != nil {
				fmt.Fprintf(stderr, "run: %v\n", berr)
			}
			fmt.Fprintf(stdout, "%s %s (retry budget exhausted; BLOCKED: %s)\n", task.ID, label, reason)
			return exitError
		}
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitError
	}
	fmt.Fprintf(stdout, "%s %s (requeued)\n", task.ID, label)
	return exitError
}

// continueTask resumes productive-but-unfinished work against the bounded
// CONTINUATION budget, which is deliberately separate from the retry budget: a
// continuation is not a failed attempt, so it returns the task to PLANNED without
// spending a retry attempt. Every continuation counts against the budget (a
// continuation that makes no progress must not loop forever); when the budget is
// spent the task is terminally stuck (CONTINUATION_EXHAUSTED) — a bounded
// automation failure, never a human decision.
func continueTask(saver taskSaver, task *domain.Task, rn *runpkg.Run, signature, reason string, policy autonomy.Policy, stdout, stderr io.Writer) int {
	_ = rn.RecordAttempt(signature)

	max := policy.MaxContinuations
	if max <= 0 {
		max = autonomy.DefaultMaxContinuations
	}
	n := rn.ReadContinuations() + 1
	if n > max {
		if err := blockTask(saver, task, domain.CONTINUATION_EXHAUSTED); err != nil {
			fmt.Fprintf(stderr, "run: %v\n", err)
			return exitError
		}
		fmt.Fprintf(stdout, "%s CONTINUE (continuation budget exhausted %d/%d; BLOCKED: %s)\n", task.ID, max, max, reason)
		fmt.Fprintf(stdout, "continuation: %d/%d\n", max, max)
		return exitError
	}
	if err := rn.RecordContinuations(n); err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitError
	}
	if err := requeueTask(saver, task, false); err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitError
	}
	fmt.Fprintf(stdout, "%s CONTINUE (requeued)\n", task.ID)
	fmt.Fprintf(stdout, "continuation: %d/%d\n", n, max)
	return exitError
}

// specFromTask builds a task-file spec from a persisted task; acceptance criteria
// are newline-delimited (the domain's single representation).
func specFromTask(task *domain.Task) *taskfile.Spec {
	var criteria []string
	for _, line := range strings.Split(task.AcceptanceCriteria, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			criteria = append(criteria, trimmed)
		}
	}
	return &taskfile.Spec{
		ID:                  task.ID,
		Title:               task.Title,
		Description:         task.Objective,
		AcceptanceCriteria:  criteria,
		Attempt:             task.Attempt,
		ExecutionMode:       task.ExecutionMode,
		DiscoveryIterations: task.DiscoveryIterations,
	}
}

// localCompletionPath advances a task from READY to LOCAL_DONE through the local
// lifecycle only. It stops at LOCAL_DONE and never enters the remote states
// (PR_OPEN, CI_RUNNING, CI_PASS, MERGED, DONE), which would misrepresent what
// happened. A dependency is satisfied by LOCAL_DONE (local) or MERGED/DONE
// (remote).
var localCompletionPath = []domain.TaskStatus{
	domain.BRANCH_CREATED,
	domain.TESTS_WRITTEN,
	domain.RED_VERIFIED,
	domain.IMPLEMENTING,
	domain.LOCAL_TESTS_PASS,
	domain.REVIEW,
	domain.REVIEW_PASS,
	domain.LOCAL_DONE,
}

// completeTask advances a task to LOCAL_DONE along the local path on a copy,
// saved once. It continues from wherever the task already is on that path, so a
// resumed task that already passed some local steps is not sent backwards. A task that
// has not been promoted to READY yet (a caller that bypassed the scheduler) is promoted
// first, so completing it is always legal.
func completeTask(saver taskSaver, task *domain.Task) error {
	start := 0
	for i, status := range localCompletionPath {
		if status == task.Status {
			start = i + 1
			break
		}
	}
	staged := *task
	if staged.Status == domain.PLANNED {
		if err := staged.Transition(domain.READY); err != nil {
			return err
		}
	}
	for _, status := range localCompletionPath[start:] {
		if err := staged.Transition(status); err != nil {
			return err
		}
	}
	if err := saver.Save(&staged); err != nil {
		return err
	}
	task.Status = staged.Status
	task.UpdatedAt = staged.UpdatedAt
	return nil
}

// blockTask moves a task to the terminal BLOCKED state with a reason.
func blockTask(saver taskSaver, task *domain.Task, reason domain.BlockedReason) error {
	staged := *task
	if err := staged.Block(reason); err != nil {
		return err
	}
	if err := saver.Save(&staged); err != nil {
		return err
	}
	task.Status = staged.Status
	task.UpdatedAt = staged.UpdatedAt
	return nil
}

// requeueTask returns a task to PLANNED on a copy, saves once, and publishes the
// change back — so a failed save cannot leave in-memory state that was never
// persisted. When spend is true it consumes one attempt (bounded by
// max_attempts); when false it keeps the budget for a repeat that changed nothing.
func requeueTask(saver taskSaver, task *domain.Task, spend bool) error {
	staged := *task
	var err error
	if spend {
		err = staged.Requeue()
	} else {
		err = staged.RequeueWithoutSpending()
	}
	if err != nil {
		return err
	}
	if err := saver.Save(&staged); err != nil {
		return err
	}
	task.Status = staged.Status
	task.UpdatedAt = staged.UpdatedAt
	return nil
}

func reportBlockedTasks(stderr io.Writer, ids []string) int {
	if len(ids) == 0 {
		return exitError
	}
	fmt.Fprintf(stderr, "run: %d task(s) blocked and not recovered: %s\n", len(ids), strings.Join(ids, ", "))
	return exitError
}

// retriedInRun reports whether a task that just failed was requeued to PLANNED
// as a RETRY with an attempt spent, so it can be retried within the same run. An
// unreachable provider (TRANSIENT_PROVIDER) is left for a later run: retrying it
// at once would only spend the whole attempt budget in seconds.
func retriedInRun(dir string, st graphStore, id string, attemptBefore int) bool {
	data, err := os.ReadFile(filepath.Join(dir, stateDirName, "runs", id, "classification.json"))
	if err != nil {
		return false
	}
	var cls failure.Classification
	if json.Unmarshal(data, &cls) != nil || cls.Disposition != failure.Retry || cls.Kind == failure.TransientProvider {
		return false
	}
	tasks, err := st.List()
	if err != nil {
		return false
	}
	for _, t := range tasks {
		if t.ID == id {
			return t.Status == domain.PLANNED && t.Attempt > attemptBefore
		}
	}
	return false
}

func taskIsBlocked(st graphStore, id string) bool {
	tasks, err := st.List()
	if err != nil {
		return false
	}
	for _, t := range tasks {
		if t.ID == id {
			return t.IsBlocked()
		}
	}
	return false
}

// blockedThisRun reports whether id blocked earlier in this invocation.
