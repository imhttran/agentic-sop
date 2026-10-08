package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/git"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/store"
	"github.com/imhttran/agentic-sop/internal/validate"
)

// runTask prints the persisted details of a single task, records an external
// completion (`sop task complete`), or a NOT_REQUIRED disposition (`sop task skip`). Like status, the read path never creates state.
func runTask(args []string, stdout, stderr io.Writer, getwd func() (string, error)) int {
	if len(args) >= 1 && args[0] == "complete" {
		return runTaskComplete(args[1:], stdout, stderr, getwd)
	}
	if len(args) >= 1 && args[0] == "skip" {
		return runTaskSkip(args[1:], stdout, stderr, getwd)
	}
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: sop task <task-id>")
		fmt.Fprintln(stderr, "       sop task complete <task-id> --external [--commit <sha>]")
		fmt.Fprintln(stderr, "       "+strings.TrimPrefix(taskSkipUsage, "usage: "))
		return exitUsage
	}
	id := args[0]

	dir, ok := projectDir(getwd, stderr)
	if !ok {
		return exitError
	}

	path := statePath(dir)
	if !requireState(path, stderr) {
		return exitError
	}

	st, err := store.Open(path)
	if err != nil {
		fmt.Fprintf(stderr, "task: %v\n", err)
		return exitError
	}
	defer st.Close()

	task, err := st.Get(id)
	if store.IsNotFound(err) {
		fmt.Fprintf(stderr, "task %s not found\n", id)
		return exitError
	}
	if err != nil {
		fmt.Fprintf(stderr, "task: %v\n", err)
		return exitError
	}

	fmt.Fprintf(stdout, "Task: %s\n", task.ID)
	fmt.Fprintf(stdout, "Title: %s\n", task.Title)
	fmt.Fprintf(stdout, "Status: %s\n", task.Status)
	fmt.Fprintf(stdout, "Attempts: %d\n", len(task.Attempts))
	if n := len(task.Attempts); task.Status == domain.NOT_REQUIRED && n > 0 {
		fmt.Fprintf(stdout, "Not required: %s\n", task.Attempts[n-1].Reason)
	}
	if len(task.DependencyIDs) > 0 {
		fmt.Fprintln(stdout, "Dependencies:")
		for _, dep := range task.DependencyIDs {
			fmt.Fprintln(stdout, dep)
		}
	}
	return exitOK
}

// runTaskComplete records the completion of a task whose work was performed outside the
// active SOP execution: an explicit, model-free operator action backed by deterministic
// repository and validation evidence. It never fabricates an attempt, a model run, or an
// approval, and it fails closed unless the evidence establishes completion. It is the
// supported counterpart to a task the SOP runtime cannot complete because the work already
// exists externally.
func runTaskComplete(args []string, stdout, stderr io.Writer, getwd func() (string, error)) int {
	id, commit, ok := parseTaskCompleteArgs(args, stderr)
	if !ok {
		return exitUsage
	}

	dir, ok := projectDir(getwd, stderr)
	if !ok {
		return exitError
	}
	path := statePath(dir)
	if !requireState(path, stderr) {
		return exitError
	}

	cfg, err := config.LoadDir(dir)
	if err != nil {
		fmt.Fprintf(stderr, "task complete: %v\n", err)
		return exitError
	}

	st, err := store.Open(path)
	if err != nil {
		fmt.Fprintf(stderr, "task complete: %v\n", err)
		return exitError
	}
	defer st.Close()

	task, err := st.Get(id)
	if store.IsNotFound(err) {
		fmt.Fprintf(stderr, "task complete: task %s not found\n", id)
		return exitError
	}
	if err != nil {
		fmt.Fprintf(stderr, "task complete: %v\n", err)
		return exitError
	}
	if task.IsSatisfied() {
		fmt.Fprintf(stderr, "task complete: %s is already complete (%s)\n", id, task.Status)
		return exitError
	}

	// Dependencies must already satisfy existing policy: external completion never
	// advances a task past its dependency ordering.
	if err := requireDependenciesSatisfied(st, task); err != nil {
		fmt.Fprintf(stderr, "task complete: %v\n", err)
		return exitError
	}

	// Repository evidence: the claimed implementation commit must be contained by HEAD,
	// so the repository genuinely holds the work the completion asserts.
	g := git.New(dir)
	if err := g.ValidateRepository(context.Background()); err != nil {
		fmt.Fprintf(stderr, "task complete: %v\n", err)
		return exitError
	}
	head, err := g.Head(context.Background())
	if err != nil {
		fmt.Fprintf(stderr, "task complete: %v\n", err)
		return exitError
	}
	if strings.TrimSpace(commit) == "" {
		commit = head
	}
	contained, err := g.IsAncestor(context.Background(), commit, "HEAD")
	if err != nil {
		fmt.Fprintf(stderr, "task complete: %v\n", err)
		return exitError
	}
	if !contained {
		fmt.Fprintf(stderr, "task complete: implementation commit %s is not contained by HEAD %s\n", commit, head)
		return exitError
	}

	// Validation evidence: the required deterministic gates must pass. This reuses the
	// harness's validation stage rather than a weaker second verifier.
	if validate.Enabled(cfg.Validation) {
		res := validate.Run(context.Background(), dir, cfg.Validation)
		if !res.Passed() {
			fmt.Fprintln(stderr, "task complete: validation FAILED; external completion requires passing validation")
			return exitError
		}
	}

	// Invariant I1: resolve any PENDING approval for the task BEFORE the completion
	// is recorded, so an external completion can never leave an actionable PENDING
	// head. This is not best-effort: if the approval cannot be persisted the command
	// fails and reports no successful completion, and because the task is not yet
	// marked complete the operator can retry without being blocked as already done.
	// It reads and writes only the task's own run directory via run.At, so it never
	// resets the task's run state.json or attempts evidence.
	approvalRun := runpkg.At(runpkg.Dir(dir, id))
	if _, derr := runpkg.ResolvePendingApproval(approvalRun); derr != nil {
		fmt.Fprintf(stderr, "task complete: pending approval not resolved: %v\n", derr)
		return exitError
	}
	if head, ok := approvalRun.Approval(); ok && head.Status == domain.ApprovalPending {
		fmt.Fprintf(stderr, "task complete: a PENDING approval remains for %s after resolution\n", id)
		return exitError
	}

	// Record the transition. The domain method is the one authority for it, so the status
	// is never set by hand.
	if err := task.CompleteExternally(); err != nil {
		fmt.Fprintf(stderr, "task complete: %v\n", err)
		return exitError
	}
	if err := st.Save(task); err != nil {
		fmt.Fprintf(stderr, "task complete: %v\n", err)
		return exitError
	}

	rec := runpkg.ExternalCompletion{
		Version:              runpkg.ExternalCompletionVersion,
		TaskID:               id,
		CompletionSource:     runpkg.CompletionSourceExternal,
		RepositoryHead:       head,
		ImplementationCommit: commit,
		Verification:         "PASS",
		RecordedBy:           runpkg.RecordedByOperatorAction,
		RecordedAt:           time.Now().UTC(),
	}
	// Best-effort provenance: a failed artifact write is reported but never changes the
	// recorded completion. Open (not New) binds the task's EXISTING run directory, so
	// recording provenance never resets state.json or clears attempts/ evidence.
	if rn, rerr := runpkg.Open(dir, id); rerr == nil {
		if werr := rn.WriteExternalCompletion(rec); werr != nil {
			fmt.Fprintf(stdout, "warning: external-completion artifact not persisted: %v\n", werr)
		}
	}

	fmt.Fprintf(stdout, "task %s: recorded external completion (%s)\n", id, task.Status)
	fmt.Fprintf(stdout, "  source:     %s\n", rec.CompletionSource)
	fmt.Fprintf(stdout, "  commit:     %s\n", rec.ImplementationCommit)
	fmt.Fprintf(stdout, "  repository: %s\n", rec.RepositoryHead)
	fmt.Fprintf(stdout, "  validation: %s\n", rec.Verification)
	return exitOK
}

// requireDependenciesSatisfied fails unless every dependency of task is satisfied, so an
// operator transition never advances a task past its dependency ordering.
func requireDependenciesSatisfied(st *store.Store, task *domain.Task) error {
	all, err := st.List()
	if err != nil {
		return err
	}
	byID := make(map[string]*domain.Task, len(all))
	for _, t := range all {
		byID[t.ID] = t
	}
	unmet, satisfied := task.ResolveDependencies(byID)
	if satisfied {
		return nil
	}
	names := make([]string, 0, len(unmet))
	for _, d := range unmet {
		names = append(names, d.TaskID)
	}
	return fmt.Errorf("%s has unsatisfied dependencies: %s", task.ID, strings.Join(names, ", "))
}

// parseTaskCompleteArgs parses
// `sop task complete <task-id> --external [--commit <sha>]`. An explicit --external is
// required: a task completed by the SOP lifecycle records its own completion, and external
// completion is an explicit operator decision, never an implicit fallback.
func parseTaskCompleteArgs(args []string, stderr io.Writer) (id, commit string, ok bool) {
	const usage = "usage: sop task complete <task-id> --external [--commit <sha>]"
	external := false
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--external":
			external = true
		case a == "--commit":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, usage)
				return "", "", false
			}
			commit = args[i+1]
			i++
		case strings.HasPrefix(a, "--commit="):
			commit = strings.TrimPrefix(a, "--commit=")
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "task complete: unknown flag %s\n%s\n", a, usage)
			return "", "", false
		case id == "":
			id = a
		default:
			fmt.Fprintln(stderr, usage)
			return "", "", false
		}
	}
	if id == "" {
		fmt.Fprintln(stderr, usage)
		return "", "", false
	}
	if !external {
		fmt.Fprintln(stderr, "task complete: an explicit --external completion is required")
		fmt.Fprintln(stderr, usage)
		return "", "", false
	}
	return id, commit, true
}
