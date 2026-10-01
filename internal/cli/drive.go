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

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/autonomy"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/failure"
	"github.com/imhttran/agentic-sop/internal/perf"
	"github.com/imhttran/agentic-sop/internal/planflow"
	"github.com/imhttran/agentic-sop/internal/quality"
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
func runGraph(planArg string, stdout, stderr io.Writer, d deps) int {
	dir, ok := projectDir(d.getwd, stderr)
	if !ok {
		return exitError
	}

	// Equivalent to `sop init`: create state and the configuration template.
	if _, err := ensureProjectInitialized(dir); err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
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
	// Carry the resolved evidence down to each task run so it is recorded.
	d.routing = routing
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
	a, agentErr := d.newAgent(cfg.Agent.Harness, cfg.Agent.Provider, cfg.Agent.Model)

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
	if err := guardCapability(a, agent.Implement); err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitError
	}
	if !d.routingEnabled {
		// The run-level/default selection is what executes only when the automatic
		// per-task router is off. With the router on, each task's final selection is
		// validated in applyTaskRouting instead.
		if err := validateSelectedModel(ctx, cfg, routing); err != nil {
			fmt.Fprintf(stderr, "run: %v\n", err)
			return exitError
		}
	}

	sess := newRunSession()
	code := driveGraph(ctx, dir, cfg, a, d, st, prepared.PlanID, sess, stdout, stderr)
	if code == exitOK {
		if final, err := st.List(); err == nil && domain.AllSatisfied(final) {
			printCompletion(stdout, dir, cfg, prepared, final)
		}
	}
	return code
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
		done, ready, blocked := statusCounts(tasks)
		fmt.Fprintf(w, "Done: %d\nReady: %d\nBlocked: %d\n", done, ready, blocked)
	}

	if prepared.TasksCreated > 0 {
		fmt.Fprintf(w, "Created %d task(s).\n", prepared.TasksCreated)
	}
	fmt.Fprintln(w)
}

// statusCounts tallies completed, ready, and blocked tasks.
func statusCounts(tasks []*domain.Task) (done, ready, blocked int) {
	for _, task := range tasks {
		switch task.Status {
		case domain.LOCAL_DONE, domain.DONE, domain.MERGED:
			done++
		case domain.READY:
			ready++
		case domain.BLOCKED:
			blocked++
		}
	}
	return done, ready, blocked
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
	fmt.Fprintf(w, "Tasks: %d/%d complete\n", len(tasks), len(tasks))
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
func driveGraph(ctx context.Context, dir string, cfg config.Config, a agent.Agent, d deps, st graphStore, planID string, sess *runSession, stdout, stderr io.Writer) int {
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
		res, err := sch.Next(ctx)
		if err != nil {
			fmt.Fprintf(stderr, "run: %v\n", err)
			return exitError
		}

		switch res.Outcome {
		case scheduler.ReadyTask:
			id := res.Task.ID
			code := runScheduledTask(ctx, dir, cfg, a, d, st, res.Task, sess, stdout, stderr)
			addTaskMetrics(run, dir, res.Task.ID)
			if code != exitOK {
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
				return code
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
			return exitOK
		default:
			fmt.Fprintf(stdout, "no runnable task (%s)\n", res.Outcome)
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
	fmt.Fprintf(stdout, "Running: %s %s\n", task.ID, task.Title)
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
	tri := runEarlyGate(ctx, cfg, d, spec, rn, runpkg.CheckpointTaskTriage)
	if tri.Escalate {
		fmt.Fprintf(stdout, "%s NEEDS_HUMAN (early JEV triage)\n  %s\n", task.ID, tri.Reason)
		// Park the run at the existing WAITING_FOR_HUMAN stage before recording the
		// approval request, so the persisted run state matches the boundary — the
		// same stage the lifecycle's own human boundary sets. No new task state is
		// introduced.
		parkRunAtHumanBoundary(rn, task.ID, stderr)
		recordHumanApprovalRequest(ctx, rn, task, runpkg.WaitingForHuman, failure.NeedsHuman, tri.Reason, tri.Reason)
		return recoverTask(saver, task, rn, "TRIAGE|NEEDS_HUMAN", "NEEDS_HUMAN", failure.NeedsHuman, cfg.AutonomyPolicy(), tri.Reason, stdout, stderr)
	}

	res, err := runAttempts(ctx, dir, cfg, a, d, spec, rn, sess, approval, tri, stdout)
	emitClassificationActivity(ctx, res.classification, res.decision)

	// A genuine human boundary (the classifier's human disposition, the autonomy
	// policy's human decision, or the WAITING_FOR_HUMAN stage) records an explicit,
	// resolvable approval request, so a client resolves the gate through SOP rather
	// than inferring one from a task status. It is provenance only: it never changes
	// the task status or the run outcome.
	if err == nil && humanBoundary(res.stage, res.classification, res.decision) {
		recordHumanApprovalRequest(ctx, rn, task, res.stage, res.classification.Disposition,
			firstNonBlank(firstReason(res.gate), res.classification.Reason), res.classification.Reason)
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
			recordHumanApprovalRequest(ctx, rn, task, res.stage, cls.Disposition,
				firstNonBlank(cls.Reason, err.Error()), cls.Reason)
		}
		if cls.Retryable() {
			return recoverTask(saver, task, rn, "ERR|"+err.Error()+"|"+string(cls.Disposition), string(cls.Disposition), cls.Disposition, cfg.AutonomyPolicy(), err.Error(), stdout, stderr)
		}
		if policyForcesHuman(cls, res.decision) {
			return recoverTask(saver, task, rn, "ERR|NEEDS_HUMAN|"+err.Error(), "NEEDS_HUMAN", failure.NeedsHuman, cfg.AutonomyPolicy(), err.Error(), stdout, stderr)
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
			if err := blockTask(saver, task, domain.RETRIES_EXHAUSTED); err != nil {
				fmt.Fprintf(stderr, "run: %v\n", err)
			}
			_ = rn.RecordAttempt(outcomeSignature(res.gate))
			fmt.Fprintf(stdout, "%s BLOCKED (%s exhausted)\n", task.ID, res.classification.Kind)
			return exitError
		}
		if res.stage == runpkg.WaitingForHuman || policyForcesHuman(res.classification, decision) {
			// A human boundary is not terminal: return the task to PLANNED so a
			// later run retries it. A repeat that changed nothing does not spend
			// the budget; a progressing attempt does, bounded by max_attempts.
			return recoverTask(saver, task, rn, outcomeSignature(res.gate)+"|NEEDS_HUMAN", "NEEDS_HUMAN", failure.NeedsHuman, cfg.AutonomyPolicy(), firstReason(res.gate), stdout, stderr)
		}
		// A retryable disposition (CONTINUE/RETRY) means required work remains and
		// no human decision is required: RETRY reuses the bounded retry budget,
		// while CONTINUE consumes the separate bounded continuation budget.
		if res.classification.Retryable() {
			disposition := res.classification.Disposition
			return recoverTask(saver, task, rn, outcomeSignature(res.gate)+"|"+string(disposition), string(disposition), disposition, cfg.AutonomyPolicy(), firstReason(res.gate), stdout, stderr)
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

	if err := completeTask(saver, task); err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitError
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
func recoverTask(saver taskSaver, task *domain.Task, rn *runpkg.Run, signature, label string, disp failure.Disposition, policy autonomy.Policy, reason string, stdout, stderr io.Writer) int {
	if disp == failure.Continue {
		return continueTask(saver, task, rn, signature, reason, policy, stdout, stderr)
	}

	prev, hadPrev := rn.ReadAttempt()
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
			if berr := blockTask(saver, task, domain.RETRIES_EXHAUSTED); berr != nil {
				fmt.Fprintf(stderr, "run: %v\n", berr)
			}
			fmt.Fprintf(stdout, "%s %s (retry budget exhausted; BLOCKED)\n", task.ID, label)
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
		ID:                 task.ID,
		Title:              task.Title,
		Description:        task.Objective,
		AcceptanceCriteria: criteria,
		ExecutionMode:      task.ExecutionMode,
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
// resumed task that already passed some local steps is not sent backwards.
func completeTask(saver taskSaver, task *domain.Task) error {
	start := 0
	for i, status := range localCompletionPath {
		if status == task.Status {
			start = i + 1
			break
		}
	}
	staged := *task
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
