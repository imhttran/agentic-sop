package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/imhttran/agentic-sop/internal/continuation"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/planflow"
	"github.com/imhttran/agentic-sop/internal/store"
)

// continueUsage is the one-line usage for `sop continue`.
const continueUsage = "usage: sop continue --check [PLAN.md] [--json]"

// runContinue is the read-only reconcile-before-continue gate. It classifies the
// active plan's reconciliation (via internal/continuation, which reuses
// planflow.Inspect) and reports whether one governed continuation is safe. It never
// executes a task, mutates state, runs a model, decides an approval, or duplicates
// the run loop; a caller continues with `sop run` only when the gate authorizes it.
//
// `--check` is required: this command has no mutating mode, so an operator cannot
// mistake it for execution. It exits zero only when the next action is
// CONTINUE_SAFE; every stop boundary exits non-zero after printing the report, so a
// script can gate a run on it.
func runContinue(args []string, stdout, stderr io.Writer, d deps) int {
	planArg, check, jsonOut, ok := parseContinueArgs(args, stderr)
	if !ok {
		return exitUsage
	}
	if !check {
		fmt.Fprintln(stderr, "continue: --check is required; this command only reports, it never executes")
		fmt.Fprintln(stderr, "continue: to continue execution, run `sop run`")
		fmt.Fprintln(stderr, continueUsage)
		return exitUsage
	}

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
		fmt.Fprintf(stderr, "continue: %v\n", err)
		return exitError
	}
	defer st.Close()

	tasks, err := st.List()
	if err != nil {
		fmt.Fprintf(stderr, "continue: %v\n", err)
		return exitError
	}

	// Pending approvals are read through SOP's approval boundary, exactly as
	// `sop approvals` does, so a gate is never inferred from a task status.
	pending, err := pendingApprovalTaskIDs(st, dir, tasks)
	if err != nil {
		fmt.Fprintf(stderr, "continue: %v\n", err)
		return exitError
	}

	var source string
	if strings.TrimSpace(planArg) != "" {
		resolved, err := planflow.ResolvePlanPath(dir, planArg)
		if err != nil {
			fmt.Fprintln(stderr, err)
			return exitError
		}
		source = resolved
	}

	res := continuation.Check(context.Background(), continuation.Options{
		Dir:              dir,
		PlanSource:       source,
		Tasks:            tasks,
		Store:            st,
		PendingApprovals: pending,
	})

	if jsonOut {
		if err := writeContinueCheckJSON(stdout, res); err != nil {
			fmt.Fprintf(stderr, "continue: %v\n", err)
			return exitError
		}
	} else {
		printContinueCheck(stdout, res)
	}

	if res.NextAction == continuation.ContinueSafe {
		return exitOK
	}
	fmt.Fprintf(stderr, "continue: continuation is not authorized (%s: %s)\n", res.NextAction, res.Reason)
	return exitError
}

// parseContinueArgs parses `sop continue --check [PLAN.md] [--json]`. `--check` is
// the only mode; any other flag or a second positional argument is a usage error.
func parseContinueArgs(args []string, stderr io.Writer) (plan string, check, jsonOut, ok bool) {
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--check":
			check = true
		case a == "--json":
			jsonOut = true
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "continue: unknown flag %s\n", a)
			fmt.Fprintln(stderr, continueUsage)
			return "", false, false, false
		case plan == "":
			plan = a
		default:
			fmt.Fprintln(stderr, continueUsage)
			return "", false, false, false
		}
	}
	return plan, check, jsonOut, true
}

// pendingApprovalTaskIDs returns the IDs of tasks with an applicable pending
// approval gate, using the same boundary `sop approvals` uses. A read failure is
// surfaced rather than silently dropping a gate the operator needs to see.
func pendingApprovalTaskIDs(st *store.Store, dir string, tasks []*domain.Task) ([]string, error) {
	svc := approvalService(st, dir)
	var out []string
	for _, task := range tasks {
		view, err := svc.Approval(task.ID)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", task.ID, err)
		}
		if view.Applicable {
			out = append(out, task.ID)
		}
	}
	return out, nil
}

// continueCheckDoc is the stable machine-readable shape of `sop continue --check
// --json`. Every slice is non-nil so an empty category serializes as [].
type continueCheckDoc struct {
	Version           int      `json:"version"`
	Classification    string   `json:"classification"`
	NextAction        string   `json:"next_action"`
	Reason            string   `json:"reason,omitempty"`
	Source            string   `json:"source,omitempty"`
	PlanID            string   `json:"plan_id,omitempty"`
	PlanChanged       bool     `json:"plan_changed"`
	Deterministic     bool     `json:"deterministic"`
	CapabilityGap     bool     `json:"capability_gap"`
	Unchanged         []string `json:"unchanged"`
	Updated           []string `json:"updated"`
	Added             []string `json:"added"`
	Removed           []string `json:"removed"`
	ChangedExecuted   []string `json:"changed_executed"`
	ChangedEquivalent []string `json:"changed_executed_equivalent"`
	ChangedMaterial   []string `json:"changed_executed_material"`
	RemovedExecuted   []string `json:"removed_executed"`
	PendingApprovals  []string `json:"pending_approvals"`
	BlockedTasks      []string `json:"blocked_tasks"`
	CurrentTask       string   `json:"current_task,omitempty"`
	CurrentTaskStatus string   `json:"current_task_status,omitempty"`
	RunnableTask      string   `json:"runnable_task,omitempty"`
	TasksTotal        int      `json:"tasks_total"`
	TasksSatisfied    int      `json:"tasks_satisfied"`
	PlanComplete      bool     `json:"plan_complete"`
}

// writeContinueCheckJSON renders a continuation check as a JSON document.
func writeContinueCheckJSON(w io.Writer, res continuation.Result) error {
	doc := continueCheckDoc{
		Version:           1,
		Classification:    string(res.Classification),
		NextAction:        string(res.NextAction),
		Reason:            res.Reason,
		Source:            res.Source,
		PlanID:            res.PlanID,
		PlanChanged:       res.PlanChanged,
		Deterministic:     res.Deterministic,
		CapabilityGap:     res.CapabilityGap,
		Unchanged:         res.Unchanged,
		Updated:           res.Updated,
		Added:             res.Added,
		Removed:           res.Removed,
		ChangedExecuted:   res.ChangedExecuted,
		ChangedEquivalent: res.ChangedEquivalent,
		ChangedMaterial:   res.ChangedMaterial,
		RemovedExecuted:   res.RemovedExecuted,
		PendingApprovals:  res.PendingApprovals,
		BlockedTasks:      res.BlockedTasks,
		CurrentTask:       res.CurrentTask,
		CurrentTaskStatus: string(res.CurrentTaskStatus),
		RunnableTask:      res.RunnableTask,
		TasksTotal:        res.TasksTotal,
		TasksSatisfied:    res.TasksSatisfied,
		PlanComplete:      res.PlanComplete,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

// printContinueCheck renders a continuation check for an operator. It states the
// classification, whether the result was stable, the reconciled source, the next
// action, and the gate or boundary that decided it.
func printContinueCheck(w io.Writer, res continuation.Result) {
	fmt.Fprintln(w, "Reconciliation check (read-only; nothing was applied)")
	if res.Source != "" {
		fmt.Fprintf(w, "Source: %s\n", res.Source)
	}
	if res.PlanID != "" {
		fmt.Fprintf(w, "Plan ID: %s\n", res.PlanID)
	}
	fmt.Fprintf(w, "Classification: %s\n", res.Classification)
	fmt.Fprintf(w, "Deterministic: %s\n", yesNo(res.Deterministic))
	fmt.Fprintf(w, "Plan changed: %s\n", yesNo(res.PlanChanged))
	printContinueIDs(w, "unchanged", res.Unchanged)
	printContinueIDs(w, "updated (never executed)", res.Updated)
	printContinueIDs(w, "added", res.Added)
	printContinueIDs(w, "removed (never executed)", res.Removed)
	printContinueIDs(w, "changed (executed, semantics preserved)", res.ChangedEquivalent)
	printContinueIDs(w, "changed (executed, needs review)", res.ChangedMaterial)
	printContinueIDs(w, "removed (executed, needs review)", res.RemovedExecuted)
	printContinueIDs(w, "pending approvals", res.PendingApprovals)
	printContinueIDs(w, "blocked tasks", res.BlockedTasks)
	fmt.Fprintf(w, "Tasks: %d total, %d satisfied\n", res.TasksTotal, res.TasksSatisfied)
	if res.CurrentTask != "" {
		fmt.Fprintf(w, "Current task: %s\n", res.CurrentTask)
	}
	if res.RunnableTask != "" {
		fmt.Fprintf(w, "Runnable task: %s\n", res.RunnableTask)
	}
	fmt.Fprintf(w, "Next action: %s\n", res.NextAction)
	if res.Reason != "" {
		fmt.Fprintf(w, "Reason: %s\n", res.Reason)
	}
}

func printContinueIDs(w io.Writer, label string, ids []string) {
	if len(ids) == 0 {
		return
	}
	fmt.Fprintf(w, "  %s: %s\n", label, strings.Join(ids, ", "))
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
