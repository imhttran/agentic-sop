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
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
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
	if cfg.Workflow.Mode != "local" {
		fmt.Fprintf(stderr, "run: workflow.mode %q is not supported by the local run; set workflow.mode: local\n", cfg.Workflow.Mode)
		return exitError
	}

	providerName, providerSource := agent.EffectiveProvider(cfg.Agent.Provider)

	st, err := store.Open(statePath(dir))
	if err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitError
	}
	defer st.Close()

	// The agent is required to execute, but not to prepare. Build it lazily so a
	// PLAN.md can still be compiled and its tasks created when the agent is not
	// configured; execution then reports the missing agent clearly.
	a, agentErr := d.newAgent(cfg.Agent.Provider, cfg.Agent.Model)

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
	prepared, err := planflow.Prepare(ctx, planflow.Options{Dir: dir, PlanSource: planSource, Agent: a, Store: st})
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
	printStartup(stdout, dir, cfg, providerName, string(providerSource), prepared, tasks)

	if agentErr != nil {
		fmt.Fprintf(stderr, "run: prepared the plan and tasks, but cannot execute: %v\n", agentErr)
		return exitError
	}
	if err := guardCapability(a, agent.Implement); err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitError
	}

	sess := newRunSession()
	code := driveGraph(ctx, dir, cfg, a, d, st, prepared.PlanID, sess, stdout, stderr)
	if code == exitOK {
		if final, err := st.List(); err == nil && allComplete(final) {
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

// printStartup emits a concise summary of the plan being executed.
func printStartup(w io.Writer, dir string, cfg config.Config, providerName, providerSource string, prepared planflow.Result, tasks []*domain.Task) {
	source := prepared.Source
	if source == "" {
		source = "(existing plan.json)"
	}

	fmt.Fprintln(w, "SOP")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Project: %s\n", projectName(dir, cfg))
	fmt.Fprintf(w, "Provider: %s (%s)\n", providerName, providerSource)
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

// allComplete reports whether every task reached a success terminal state.
func allComplete(tasks []*domain.Task) bool {
	if len(tasks) == 0 {
		return false
	}
	for _, task := range tasks {
		switch task.Status {
		case domain.LOCAL_DONE, domain.DONE, domain.MERGED:
		default:
			return false
		}
	}
	return true
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
// work remains.
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

	sch := scheduler.New(st)
	completed := 0

	// Each iteration either completes a task (→ LOCAL_DONE), resumes and completes an
	// interrupted one, or blocks one, so the loop is bounded by the task count;
	// +1 is defensive.
	for i := 0; i <= len(tasks); i++ {
		res, err := sch.Next(ctx)
		if err != nil {
			fmt.Fprintf(stderr, "run: %v\n", err)
			return exitError
		}

		switch res.Outcome {
		case scheduler.ReadyTask:
			code := runScheduledTask(ctx, dir, cfg, a, d, st, res.Task, sess, stdout, stderr)
			addTaskMetrics(run, dir, res.Task.ID)
			if code != exitOK {
				return code
			}
			completed++
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

	fmt.Fprintf(stdout, "all tasks done (%d completed this run)\n", completed)
	return exitOK
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

// runScheduledTask runs the lifecycle for one scheduled task and updates its
// persisted state.
func runScheduledTask(ctx context.Context, dir string, cfg config.Config, a agent.Agent, d deps, saver taskSaver, task *domain.Task, sess *runSession, stdout, stderr io.Writer) int {
	spec := specFromTask(task)
	fmt.Fprintf(stdout, "Running: %s %s\n", task.ID, task.Title)
	rn, err := runpkg.New(dir, task.ID)
	if err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitError
	}
	_ = rn.Write("task.md", spec.Render())

	res, err := executeLifecycle(ctx, dir, cfg, a, d, spec, rn, sess)
	if err != nil {
		_ = rn.SetStage(runpkg.Failed)
		_ = blockTask(saver, task, domain.RETRIES_EXHAUSTED)
		fmt.Fprintf(stderr, "%s: %v\n", task.ID, err)
		return exitError
	}

	if code := emitRunSummary(stdout, dir, cfg, rn, res); code != exitOK {
		if res.stage == runpkg.WaitingForHuman {
			// A human boundary is not terminal: return the task to PLANNED so a
			// later run retries it. A repeat that changed nothing does not spend
			// the budget; a progressing attempt does, bounded by max_attempts.
			sig := outcomeSignature(res.gate)
			prev, hadPrev := rn.ReadAttempt()
			_ = rn.RecordAttempt(sig)

			if hadPrev && prev == sig {
				if err := requeueTask(saver, task, false); err != nil {
					fmt.Fprintf(stderr, "run: %v\n", err)
					return exitError
				}
				fmt.Fprintf(stdout, "%s NEEDS_HUMAN (no change since the previous attempt: %s)\n", task.ID, res.gate.Reasons[0])
				return exitError
			}

			if err := requeueTask(saver, task, true); err != nil {
				if errors.Is(err, domain.ErrRetryExhausted) {
					if berr := blockTask(saver, task, domain.RETRIES_EXHAUSTED); berr != nil {
						fmt.Fprintf(stderr, "run: %v\n", berr)
					}
					fmt.Fprintf(stdout, "%s NEEDS_HUMAN (retry budget exhausted; BLOCKED)\n", task.ID)
					return exitError
				}
				fmt.Fprintf(stderr, "run: %v\n", err)
				return exitError
			}
			fmt.Fprintf(stdout, "%s NEEDS_HUMAN (requeued)\n", task.ID)
			return exitError
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
