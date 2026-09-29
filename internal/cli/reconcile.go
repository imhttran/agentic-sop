package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/imhttran/agentic-sop/internal/autonomy"
	"github.com/imhttran/agentic-sop/internal/planflow"
	"github.com/imhttran/agentic-sop/internal/store"
)

// runReconcile applies an intentional PLAN change to the persisted active plan
// explicitly. It compiles and validates the requested PLAN, diffs it against the
// active graph, and updates only tasks that have never executed. A task with
// execution history whose definition changed, or that the plan removes, stops
// with NEEDS_HUMAN rather than being silently overwritten or discarded, unless
// the human names it with --accept-changed to approve the replacement of its
// definition explicitly (its lifecycle state and history are preserved).
//
// This is the supported counterpart to the guard `sop run` raises when the plan
// changed: the user never needs to delete state.db or hand-edit plan.meta.json.
func runReconcile(args []string, stdout, stderr io.Writer, d deps) int {
	planArg, accept, ok := parseReconcileArgs(args, stderr)
	if !ok {
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

	source, err := planflow.ResolvePlanPath(dir, planArg)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitError
	}

	cfg, err := loadConfigOrDefault(dir)
	if err != nil {
		fmt.Fprintf(stderr, "reconcile: %v\n", err)
		return exitError
	}

	st, err := store.Open(path)
	if err != nil {
		fmt.Fprintf(stderr, "reconcile: %v\n", err)
		return exitError
	}
	defer st.Close()

	// The agent is only needed to normalize a PLAN that is not recognizable. If it
	// cannot be built, reconciliation still proceeds for a well-formed plan and
	// reports the missing agent clearly only when normalization is required.
	a, _ := d.newAgent(cfg.Agent.Harness, cfg.Agent.Provider, cfg.Agent.Model)

	res, err := planflow.Reconcile(context.Background(), planflow.ReconcileOptions{
		Dir:           dir,
		PlanSource:    source,
		Agent:         a,
		Store:         st,
		AcceptChanged: accept,
		// The autonomy policy, not the CLI, decides whether a safe plan change is
		// reconciled automatically. Under a low/balanced policy this authorizes
		// nothing and the explicit-approval behavior is unchanged.
		AutoAcceptExecuted: reconcileAutoAccept(cfg.AutonomyPolicy()),
		OnRepair: func(attempt int, cause error) {
			fmt.Fprintf(stderr, "plan: invalid plan returned to the agent for correction (attempt %d): %v\n", attempt, cause)
		},
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return exitError
	}

	printReconcileSummary(stdout, res)
	return exitOK
}

// reconcileAutoAccept builds the reconciliation callback the planflow layer
// consults for an executed task whose definition changed. It classifies the change
// deterministically (equivalent vs material) and asks the autonomy policy; only an
// AUTO_RECONCILE decision authorizes a silent, history-preserving refresh.
func reconcileAutoAccept(policy autonomy.Policy) func(planflow.ExecutedChange) autonomy.Decision {
	return func(change planflow.ExecutedChange) autonomy.Decision {
		kind := autonomy.PlanChangeExecutedMaterial
		if change.Equivalent {
			kind = autonomy.PlanChangeExecutedEquivalent
		}
		return autonomy.DecidePlanChange(kind, policy)
	}
}

// parseReconcileArgs splits the reconcile arguments into the required PLAN path
// and the repeatable --accept-changed approvals. It accepts both
// `--accept-changed ID` and `--accept-changed=ID`.
func parseReconcileArgs(args []string, stderr io.Writer) (plan string, accept []string, ok bool) {
	const usage = "usage: sop reconcile <PLAN.md> [--accept-changed <TASK_ID>]..."
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--accept-changed":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "reconcile: --accept-changed requires a task id")
				fmt.Fprintln(stderr, usage)
				return "", nil, false
			}
			accept = append(accept, args[i+1])
			i++
		case strings.HasPrefix(a, "--accept-changed="):
			id := strings.TrimPrefix(a, "--accept-changed=")
			if id == "" {
				fmt.Fprintln(stderr, "reconcile: --accept-changed requires a task id")
				fmt.Fprintln(stderr, usage)
				return "", nil, false
			}
			accept = append(accept, id)
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "reconcile: unknown flag %s\n", a)
			fmt.Fprintln(stderr, usage)
			return "", nil, false
		case plan == "":
			plan = a
		default:
			fmt.Fprintln(stderr, usage)
			return "", nil, false
		}
	}
	if plan == "" {
		fmt.Fprintln(stderr, usage)
		return "", nil, false
	}
	return plan, accept, true
}

// printReconcileSummary reports what the reconciliation changed, naming the task
// IDs in each category so the operator can see exactly what moved.
func printReconcileSummary(w io.Writer, res planflow.ReconcileResult) {
	fmt.Fprintf(w, "Reconciled %s\n", res.Source)
	fmt.Fprintf(w, "Plan ID: %s\n\n", res.PlanID)

	if res.PlanChanged {
		fmt.Fprintln(w, "The requested plan differs from the recorded plan.")
	} else {
		fmt.Fprintln(w, "The requested plan matches the recorded plan.")
	}

	fmt.Fprintf(w, "  unchanged: %d\n", len(res.Unchanged))
	printChangedIDs(w, "updated", res.Updated)
	printChangedIDs(w, "accepted (executed)", res.Accepted)
	printChangedIDs(w, "added", res.Added)
	printChangedIDs(w, "removed", res.Removed)
	printAutoReconciled(w, res.AutoReconciled)
}

// printAutoReconciled reports each task reconciled automatically, with its
// provenance (the definition hashes, the classification, the risk, and the
// reason), so a silent refresh is auditable and never loses its history.
func printAutoReconciled(w io.Writer, recs []planflow.AutoReconciled) {
	if len(recs) == 0 {
		return
	}
	fmt.Fprintln(w, "  auto-reconciled (executed, history preserved):")
	for _, r := range recs {
		fmt.Fprintf(w, "    %s: %s -> %s [%s risk=%s]\n", r.TaskID, r.Before, r.After, r.Classification, r.Risk)
		if reason := strings.TrimSpace(r.Reason); reason != "" {
			fmt.Fprintf(w, "      reason: %s\n", reason)
		}
	}
}

func printChangedIDs(w io.Writer, label string, ids []string) {
	if len(ids) == 0 {
		return
	}
	fmt.Fprintf(w, "  %s: %v\n", label, ids)
}
