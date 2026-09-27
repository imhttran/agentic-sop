package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/planflow"
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

// runGraph is `sop run` with no argument: the one-command workflow. It brings the
// project up to a runnable state — initializing state, compiling or generating
// the machine plan from a human PLAN/PRD, and creating tasks — then drives the
// task graph. Each step is idempotent and safe to repeat.
func runGraph(stdout, stderr io.Writer, d deps) int {
	dir, ok := projectDir(d.getwd, stderr)
	if !ok {
		return exitError
	}

	stateExisted := stateDirExists(dir)

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

	a, err := d.newAgent(cfg.Agent.Provider)
	if err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitError
	}

	st, err := store.Open(statePath(dir))
	if err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitError
	}
	defer st.Close()

	ctx := context.Background()
	prepared, err := planflow.Prepare(ctx, planflow.Options{Dir: dir, Agent: a, Store: st})
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
	printStartup(stdout, dir, cfg, prepared, tasks, stateExisted)

	return driveGraph(ctx, dir, cfg, a, d, st, stdout, stderr)
}

// printStartup emits a concise summary of what SOP found and did.
func printStartup(w io.Writer, dir string, cfg config.Config, prepared planflow.Result, tasks []*domain.Task, stateExisted bool) {
	name := strings.TrimSpace(cfg.Project.Name)
	if name == "" {
		name = filepath.Base(dir)
	}
	source := prepared.Source
	if source == "" {
		source = "(existing plan.json)"
	}

	fmt.Fprintln(w, "SOP")
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Project: %s\n", name)
	fmt.Fprintf(w, "Source: %s\n", source)
	planState := "current"
	if prepared.PlanRebuilt {
		planState = "rebuilt"
	}
	fmt.Fprintf(w, "Plan: %s\n", planState)
	state := "existing"
	if !stateExisted {
		state = "initialized"
	}
	fmt.Fprintf(w, "State: %s\n", state)
	fmt.Fprintf(w, "Tasks: %d\n", len(tasks))

	var done, ready, blocked, pending int
	for _, task := range tasks {
		switch task.Status {
		case domain.LOCAL_DONE, domain.DONE, domain.MERGED:
			done++
		case domain.READY:
			ready++
		case domain.BLOCKED:
			blocked++
		case domain.PLANNED:
			pending++
		}
	}
	fmt.Fprintf(w, "Done: %d  Ready: %d  Blocked: %d  Pending: %d\n", done, ready, blocked, pending)

	if prepared.PlanRebuilt {
		if prepared.SourceKind == planflow.KindPRD {
			fmt.Fprintln(w, "Generated plan.")
		} else {
			fmt.Fprintln(w, "Compiled plan.")
		}
	}
	if prepared.TasksCreated > 0 {
		fmt.Fprintf(w, "Created task graph (%d task(s)).\n", prepared.TasksCreated)
	}
}

// stateDirExists reports whether the project state directory already exists.
func stateDirExists(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, stateDirName))
	return err == nil && info.IsDir()
}

// driveGraph selects ready tasks with the scheduler and runs the local lifecycle
// for each, completing a task (gate passed) or blocking it, until no runnable
// work remains.
//
// Completion is local: it records LOCAL_DONE through the local lifecycle
// (BRANCH_CREATED … REVIEW_PASS → LOCAL_DONE). It never fabricates PR_OPEN,
// CI_RUNNING, CI_PASS, or MERGED, because no PR was opened and no CI ran. The
// persisted state therefore describes what actually happened.
func driveGraph(ctx context.Context, dir string, cfg config.Config, a agent.Agent, d deps, st graphStore, stdout, stderr io.Writer) int {
	tasks, err := st.List()
	if err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitError
	}
	if len(tasks) == 0 {
		fmt.Fprintln(stderr, "run: no tasks to execute")
		return exitError
	}

	sch := scheduler.New(st)
	completed := 0

	// Each iteration either completes a task (→ DONE) or blocks one (→ BLOCKED),
	// so the loop is bounded by the task count; +1 is defensive.
	for i := 0; i <= len(tasks); i++ {
		res, err := sch.Next(ctx)
		if err != nil {
			fmt.Fprintf(stderr, "run: %v\n", err)
			return exitError
		}

		switch res.Outcome {
		case scheduler.ReadyTask:
			if code := runScheduledTask(ctx, dir, cfg, a, d, st, res.Task, stdout, stderr); code != exitOK {
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

// runScheduledTask runs the lifecycle for one scheduled task and updates its
// persisted state.
func runScheduledTask(ctx context.Context, dir string, cfg config.Config, a agent.Agent, d deps, saver taskSaver, task *domain.Task, stdout, stderr io.Writer) int {
	spec := specFromTask(task)
	fmt.Fprintf(stdout, "Running: %s\n", task.ID)
	rn, err := runpkg.New(dir, task.ID)
	if err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitError
	}
	_ = rn.Write("task.md", spec.Render())

	res, err := executeLifecycle(ctx, dir, cfg, a, d, spec, rn)
	if err != nil {
		_ = rn.SetStage(runpkg.Failed)
		_ = blockTask(saver, task, domain.RETRIES_EXHAUSTED)
		fmt.Fprintf(stderr, "%s: %v\n", task.ID, err)
		return exitError
	}

	if code := emitRunSummary(stdout, dir, cfg, rn, res); code != exitOK {
		if err := blockTask(saver, task, domain.REVIEW_UNRESOLVED); err != nil {
			fmt.Fprintf(stderr, "run: %v\n", err)
		}
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

// completeTask advances a task to LOCAL_DONE on a copy, saves once, and publishes
// the change back — so a failed save cannot leave in-memory state that was never
// persisted.
func completeTask(saver taskSaver, task *domain.Task) error {
	staged := *task
	for _, status := range localCompletionPath {
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
