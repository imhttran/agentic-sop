package cli

import (
	"context"
	"encoding/json"
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
	planArg, accept, listChanged, jsonOut, ok := parseReconcileArgs(args, stderr)
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

	// A listing is read-only: it reports what a reconciliation would change, so a
	// client can name every changed executed task before anything is applied. It
	// never mutates the graph, the machine plan, or the provenance.
	if listChanged {
		res, err := planflow.Inspect(context.Background(), planflow.ReconcileOptions{
			Dir:        dir,
			PlanSource: source,
			Agent:      a,
			Store:      st,
			OnRepair: func(attempt int, cause error) {
				fmt.Fprintf(stderr, "plan: invalid plan returned to the agent for correction (attempt %d): %v\n", attempt, cause)
			},
		})
		if err != nil {
			fmt.Fprintln(stderr, err)
			return exitError
		}
		if jsonOut {
			if err := writeReconcileListingJSON(stdout, res); err != nil {
				fmt.Fprintf(stderr, "reconcile: %v\n", err)
				return exitError
			}
			return exitOK
		}
		printReconcileListing(stdout, res)
		return exitOK
	}

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

// parseReconcileArgs splits the reconcile arguments into the required PLAN path,
// the repeatable --accept-changed approvals, and the read-only --list-changed and
// --json flags. It accepts both `--accept-changed ID` and `--accept-changed=ID`.
// A listing authorizes nothing, so combining it with an approval is a usage error
// rather than a silently ignored flag.
func parseReconcileArgs(args []string, stderr io.Writer) (plan string, accept []string, listChanged, jsonOut, ok bool) {
	const usage = "usage: sop reconcile <PLAN.md> [--accept-changed <TASK_ID>]... [--list-changed] [--json]"
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--accept-changed":
			if i+1 >= len(args) {
				fmt.Fprintln(stderr, "reconcile: --accept-changed requires a task id")
				fmt.Fprintln(stderr, usage)
				return "", nil, false, false, false
			}
			accept = append(accept, args[i+1])
			i++
		case strings.HasPrefix(a, "--accept-changed="):
			id := strings.TrimPrefix(a, "--accept-changed=")
			if id == "" {
				fmt.Fprintln(stderr, "reconcile: --accept-changed requires a task id")
				fmt.Fprintln(stderr, usage)
				return "", nil, false, false, false
			}
			accept = append(accept, id)
		case a == "--list-changed":
			listChanged = true
		case a == "--json":
			jsonOut = true
		case strings.HasPrefix(a, "-"):
			fmt.Fprintf(stderr, "reconcile: unknown flag %s\n", a)
			fmt.Fprintln(stderr, usage)
			return "", nil, false, false, false
		case plan == "":
			plan = a
		default:
			fmt.Fprintln(stderr, usage)
			return "", nil, false, false, false
		}
	}
	if listChanged && len(accept) > 0 {
		fmt.Fprintln(stderr, "reconcile: --list-changed only reports; drop --accept-changed (approve after you see the list)")
		fmt.Fprintln(stderr, usage)
		return "", nil, false, false, false
	}
	if jsonOut && !listChanged {
		fmt.Fprintln(stderr, "reconcile: --json is available with --list-changed")
		fmt.Fprintln(stderr, usage)
		return "", nil, false, false, false
	}
	if plan == "" {
		fmt.Fprintln(stderr, usage)
		return "", nil, false, false, false
	}
	return plan, accept, listChanged, jsonOut, true
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

// printReconcileListing renders the read-only preview: what a reconciliation would
// change, and which executed tasks would need an explicit human decision. It says
// plainly that nothing was applied, so a preview is never mistaken for a run.
func printReconcileListing(w io.Writer, res planflow.ReconcileResult) {
	fmt.Fprintf(w, "Reconcile preview for %s (nothing applied)\n", res.Source)
	fmt.Fprintf(w, "Plan ID: %s\n\n", res.PlanID)

	if res.PlanChanged {
		fmt.Fprintln(w, "The requested plan differs from the recorded plan.")
	} else {
		fmt.Fprintln(w, "The requested plan matches the recorded plan.")
	}

	fmt.Fprintf(w, "  unchanged: %d\n", len(res.Unchanged))
	printChangedIDs(w, "updated (never executed)", res.Updated)
	printChangedIDs(w, "added", res.Added)
	printChangedIDs(w, "removed (never executed)", res.Removed)
	printAutoReconciled(w, res.AutoReconciled)
	printChangedIDs(w, "changed (executed: approve each with --accept-changed)", res.ChangedExecuted)
	printChangedIDs(w, "removed (executed: cannot be approved; keep it or complete it first)", res.RemovedExecuted)
	if len(res.ChangedExecuted) == 0 && len(res.RemovedExecuted) == 0 {
		fmt.Fprintln(w, "  no executed task needs a human decision")
	}
}

// reconcileListingDoc is the machine-readable shape of a read-only reconciliation
// preview. It is stable data for a client (for example the controller) that must
// render what a reconciliation would change without diffing the plan itself.
// Every slice is non-nil so an empty category serializes as [] rather than null.
type reconcileListingDoc struct {
	Version         int                       `json:"version"`
	Source          string                    `json:"source"`
	PlanID          string                    `json:"plan_id"`
	PlanChanged     bool                      `json:"plan_changed"`
	Unchanged       []string                  `json:"unchanged"`
	Updated         []string                  `json:"updated"`
	Added           []string                  `json:"added"`
	Removed         []string                  `json:"removed"`
	ChangedExecuted []string                  `json:"changed_executed"`
	RemovedExecuted []string                  `json:"removed_executed"`
	AutoReconciled  []planflow.AutoReconciled `json:"auto_reconciled"`
}

// writeReconcileListingJSON renders the read-only preview as a JSON document.
func writeReconcileListingJSON(w io.Writer, res planflow.ReconcileResult) error {
	doc := reconcileListingDoc{
		Version:         1,
		Source:          res.Source,
		PlanID:          res.PlanID,
		PlanChanged:     res.PlanChanged,
		Unchanged:       idsOrEmpty(res.Unchanged),
		Updated:         idsOrEmpty(res.Updated),
		Added:           idsOrEmpty(res.Added),
		Removed:         idsOrEmpty(res.Removed),
		ChangedExecuted: idsOrEmpty(res.ChangedExecuted),
		RemovedExecuted: idsOrEmpty(res.RemovedExecuted),
		AutoReconciled:  res.AutoReconciled,
	}
	if doc.AutoReconciled == nil {
		doc.AutoReconciled = []planflow.AutoReconciled{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}

// idsOrEmpty returns a non-nil copy of ids, so an empty category renders as [] in
// JSON rather than null.
func idsOrEmpty(ids []string) []string {
	if ids == nil {
		return []string{}
	}
	return ids
}
