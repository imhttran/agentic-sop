package sopclient

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ErrDecisionRejected: SOP refused an approve/decline (e.g. a stale gate).
// Callers render it as an actionable conflict, not an infrastructure failure.
var ErrDecisionRejected = errors.New("SOP rejected the decision")

// DecisionRejection carries SOP's refusal message verbatim and the task id.
type DecisionRejection struct {
	Verb    string
	TaskID  string
	Message string
	Err     error
}

func (e *DecisionRejection) Error() string {
	msg := e.Message
	if msg == "" {
		msg = "no message"
	}
	return fmt.Sprintf("sop %s %s rejected by SOP: %s", e.Verb, e.TaskID, msg)
}

func (e *DecisionRejection) Unwrap() []error { return []error{ErrDecisionRejected, e.Err} }

// ErrReconcileRejected: SOP refused a reconcile or accept-changed. Distinct from
// the precondition sentinels, which are checked before any SOP call.
var ErrReconcileRejected = errors.New("SOP rejected the reconciliation")

// ReconcileRejection carries SOP's refusal message verbatim and the task ids.
type ReconcileRejection struct {
	Verb string
	// TaskIDs is empty for a plain reconcile.
	TaskIDs []string
	Message string
	Err     error
}

func (e *ReconcileRejection) Error() string {
	msg := e.Message
	if msg == "" {
		msg = "no message"
	}
	if len(e.TaskIDs) > 0 {
		return fmt.Sprintf("sop %s (accept-changed %s) rejected by SOP: %s", e.Verb, strings.Join(e.TaskIDs, ", "), msg)
	}
	return fmt.Sprintf("sop %s rejected by SOP: %s", e.Verb, msg)
}

func (e *ReconcileRejection) Unwrap() []error { return []error{ErrReconcileRejected, e.Err} }

// DecisionOptions are the optional `--by` / `--note` values; empty omits the flag.
type DecisionOptions struct {
	By   string
	Note string
}

// decisionArgs never emits --run: recording a decision must not start work.
func decisionArgs(taskID string, opts DecisionOptions) []string {
	args := []string{taskID}
	if opts.By != "" {
		args = append(args, "--by", opts.By)
	}
	if opts.Note != "" {
		args = append(args, "--note", opts.Note)
	}
	return args
}

// ApproveTask delegates a gate decision to `sop approve <task-id> [--by] [--note]`.
// SOP validates the gate; a refusal comes back as *DecisionRejection. No --run is
// passed, so approving never starts execution.
func (c *Client) ApproveTask(ctx context.Context, projectID, taskID string, opts DecisionOptions) error {
	return c.decide(ctx, projectID, "approve", taskID, opts)
}

// DeclineTask delegates to `sop decline <task-id> [--by] [--note]`.
func (c *Client) DeclineTask(ctx context.Context, projectID, taskID string, opts DecisionOptions) error {
	return c.decide(ctx, projectID, "decline", taskID, opts)
}

// AcceptChangedTask accepts one changed executed task; see AcceptChangedTasks.
func (c *Client) AcceptChangedTask(ctx context.Context, projectID, taskID string) error {
	return c.AcceptChangedTasks(ctx, projectID, []string{taskID})
}

// AcceptChangedTasks accepts the explicitly selected changed tasks in ONE
// `sop reconcile <PLAN.md> --accept-changed <id>...` invocation, so SOP's atomic
// reconcile semantics hold. Every precondition (project, active plan, reported
// changed set, each id in that set) is checked before any SOP call, so a batch
// is all-or-nothing. A SOP refusal comes back as *ReconcileRejection.
func (c *Client) AcceptChangedTasks(ctx context.Context, projectID string, taskIDs []string) error {
	st, ok := c.stores[projectID]
	if !ok {
		return ErrProjectNotFound
	}
	planPath, ok := c.PlanSource(projectID)
	if !ok {
		return ErrNoActivePlan
	}
	changed, err := c.ChangedTasks(ctx, projectID)
	if err != nil {
		return err
	}
	if !changed.Reported {
		return ErrChangedTasksNotReported
	}

	ids := normalizeTaskIDs(taskIDs)
	if len(ids) == 0 {
		return ErrNoExplicitTaskIDs
	}
	for _, id := range ids {
		if !changed.Has(id) {
			return ErrTaskNotInChangedSet
		}
	}

	out, err := c.cmd.Exec(ctx, st.root, "reconcile", acceptChangedArgs(planPath, ids)...)
	if err != nil {
		return &ReconcileRejection{Verb: "reconcile", TaskIDs: ids, Message: decisionMessage(out, err), Err: err}
	}
	return nil
}

func acceptChangedArgs(planPath string, ids []string) []string {
	args := []string{planPath}
	for _, id := range ids {
		args = append(args, "--accept-changed", id)
	}
	return args
}

// normalizeTaskIDs trims, drops blanks, and de-duplicates, keeping first-seen order.
func normalizeTaskIDs(taskIDs []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, raw := range taskIDs {
		id := strings.TrimSpace(raw)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}
