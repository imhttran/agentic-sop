package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/scheduler"
	"github.com/imhttran/agentic-sop/internal/store"
	"github.com/imhttran/agentic-sop/internal/taskfile"
)

// taskSaver is the persistence slice the graph driver needs to update state.
type taskSaver interface {
	Save(task *domain.Task) error
}

// runGraph drives the persisted task graph: the scheduler selects the next ready
// task, the local lifecycle runs for it, and the task is completed (gate passed)
// or blocked, repeating until no runnable work remains.
//
// Completion is local: no remote PR/CI/merge runs, so a passed lifecycle
// advances the task through the remaining legal transitions to DONE (documented
// in the command output). Dependents then unblock, which is what lets graph
// execution progress without a remote.
func runGraph(stdout, stderr io.Writer, d deps) int {
	dir, ok := projectDir(d.getwd, stderr)
	if !ok {
		return exitError
	}

	path := statePath(dir)
	if !requireState(path, stderr) {
		return exitError
	}

	st, err := store.Open(path)
	if err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitError
	}
	defer st.Close()

	tasks, err := st.List()
	if err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitError
	}
	if len(tasks) == 0 {
		fmt.Fprintln(stderr, "run: no tasks; run `sop tasks` first")
		return exitError
	}

	cfg, err := loadConfigOrDefault(dir)
	if err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitError
	}

	a, err := d.newAgent(cfg.Agent.Provider)
	if err != nil {
		fmt.Fprintf(stderr, "run: %v\n", err)
		return exitError
	}

	ctx := context.Background()
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
	fmt.Fprintf(stdout, "%s DONE\n", task.ID)
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

// localCompletionPath advances a task from READY to DONE. Local execution has no
// remote PR/CI/merge, so the intermediate remote states are synthesized; the
// dependency rule (a dependency is complete only at DONE) then holds.
var localCompletionPath = []domain.TaskStatus{
	domain.BRANCH_CREATED,
	domain.TESTS_WRITTEN,
	domain.RED_VERIFIED,
	domain.IMPLEMENTING,
	domain.LOCAL_TESTS_PASS,
	domain.REVIEW,
	domain.REVIEW_PASS,
	domain.PR_OPEN,
	domain.CI_RUNNING,
	domain.CI_PASS,
	domain.MERGED,
	domain.DONE,
}

// completeTask advances a task to DONE on a copy, saves once, and publishes the
// change back — so a failed save cannot leave in-memory state that was never
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
