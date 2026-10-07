// Package continuation is the deterministic, read-only reconcile-before-continue
// gate for an already-active SOP plan. It answers one question: given the source
// plan, the recorded machine plan, and the persisted task lifecycle, is another
// governed `sop run` safe to attempt, and if not, why not?
//
// It is not a second SOP engine. It never mutates state, never runs a model, never
// schedules or executes a task, and never decides an approval. It reuses SOP's
// existing authority for each input it needs:
//
//   - reconciliation is planflow.Inspect, the read-only preview of the same
//     deterministic diff planflow.Reconcile applies;
//   - lifecycle meaning is the domain task predicates the scheduler already uses;
//   - human boundaries are the approval boundary the caller enumerates (the CLI
//     reads them through internal/approval).
//
// The classifier is deliberately conservative: any change that could alter plan
// meaning - a changed or removed executed task, an added/removed/updated task, or a
// capability the plan depends on whose owner the source does not determine - stops
// for human review rather than continuing. Over-stopping is safe; under-stopping is
// not, because this gate exists to make sure reconciliation never silently rewrites
// plan semantics merely to let execution proceed.
package continuation

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/planflow"
	"github.com/imhttran/agentic-sop/internal/planner"
)

// Classification is the single reconciliation outcome for a continuation attempt.
// The vocabulary is fixed; a caller relays it verbatim rather than inventing a
// sixth category.
type Classification string

const (
	// Clean: source, compiled plan, and persisted state are compatible and no
	// meaningful reconciliation mutation is required. Continuation may proceed.
	Clean Classification = "RECONCILE_CLEAN"
	// Changed: reconciliation would produce a deterministic, semantics-preserving
	// change (plan-level metadata, or an executed task whose descriptive text alone
	// changed). Continuation may proceed after validation because task identity,
	// lifecycle meaning, dependencies, and acceptance meaning are unchanged.
	Changed Classification = "RECONCILE_CHANGED"
	// SemanticChange: reconciliation would change plan meaning - a materially changed
	// or removed executed task, a task added/removed/updated, a task identity change,
	// or a new required capability with no determined owner. A human must review the
	// source plan before execution continues.
	SemanticChange Classification = "RECONCILE_SEMANTIC_CHANGE"
	// Failed: reconciliation cannot establish a valid continuation state (no active
	// plan, an unreadable or uncompilable source, or a store failure).
	Failed Classification = "RECONCILE_FAILED"
	// Nondeterministic: repeated reconciliation of the same source and state does not
	// agree, so the result cannot be trusted to authorize execution.
	Nondeterministic Classification = "RECONCILE_NONDETERMINISTIC"
)

// NextAction is the single recommended operator action for the current state.
type NextAction string

const (
	// ContinueSafe: the gate authorizes one governed continuation via `sop run`.
	ContinueSafe NextAction = "CONTINUE_SAFE"
	// RunAgain: a previous governed run requeued work; another bounded run is safe.
	// It is produced by observing a run, not by the pre-run Check, and is named here
	// so a report uses one vocabulary end to end.
	RunAgain NextAction = "RUN_AGAIN"
	// HumanReviewRequired: reconciliation needs a human decision before continuing.
	HumanReviewRequired NextAction = "HUMAN_REVIEW_REQUIRED"
	// ApprovalRequired: a pending human approval gate must be resolved first.
	ApprovalRequired NextAction = "APPROVAL_REQUIRED"
	// Blocked: progress cannot continue without an explicit operator action (for
	// example `sop retry` for a BLOCKED task).
	Blocked NextAction = "BLOCKED"
	// PlanComplete: every task is satisfied; there is nothing to continue.
	PlanComplete NextAction = "PLAN_COMPLETE"
)

// DefaultAttempts is the bounded number of read-only reconciliation passes used to
// confirm the result is stable. Two passes are enough to detect a compiler that
// does not return the same answer twice; the bound is small so a continuation
// check can never become an infinite reconciliation loop.
const DefaultAttempts = 2

// Inspector performs a read-only reconciliation preview. It matches
// planflow.Inspect, which never mutates the graph, the machine plan, or its
// provenance, and never calls a model. It is injectable so a caller (or a test) can
// supply a deterministic stand-in without changing the decision logic.
type Inspector func(ctx context.Context, opts planflow.ReconcileOptions) (planflow.ReconcileResult, error)

// Options configures Check. Dir is the project root. PlanSource is the requested
// plan (absolute), or empty to use the recorded active plan's own source. Tasks is
// the persisted graph; when it is empty and Store is set, Check reads the graph
// from Store. Store is the persistence slot planflow.Inspect reads the active graph
// from — Inspect only ever calls List, so no mutation is possible. PendingApprovals
// is the set of task IDs with an applicable pending approval gate, enumerated by
// the caller through SOP's approval boundary.
type Options struct {
	Dir              string
	PlanSource       string
	Tasks            []*domain.Task
	Store            planflow.GraphStore
	PendingApprovals []string
	Inspect          Inspector
	Attempts         int
}

// Result is the classification and the evidence behind it. Every slice is non-nil
// so an empty category renders as [] rather than null.
type Result struct {
	Classification Classification
	NextAction     NextAction
	Reason         string

	Source      string
	PlanID      string
	PlanChanged bool

	Deterministic bool
	CapabilityGap bool

	Unchanged         []string
	Updated           []string
	Added             []string
	Removed           []string
	ChangedExecuted   []string
	ChangedEquivalent []string
	ChangedMaterial   []string
	RemovedExecuted   []string

	PendingApprovals []string
	BlockedTasks     []string

	CurrentTask       string
	CurrentTaskStatus domain.TaskStatus
	RunnableTask      string
	TasksTotal        int
	TasksSatisfied    int
	PlanComplete      bool
}

// Check performs the read-only reconcile-before-continue decision. It never
// returns an operational error: an unreadable source, a missing plan, or a store
// problem is reported as Classification Failed with an actionable Reason, so a
// caller always has a decision to relay. The only thing that stops it early is a
// canceled context.
func Check(ctx context.Context, opts Options) Result {
	inspect := opts.Inspect
	if inspect == nil {
		inspect = planflow.Inspect
	}
	attempts := opts.Attempts
	if attempts <= 0 {
		attempts = DefaultAttempts
	}

	tasks := opts.Tasks
	if len(tasks) == 0 && opts.Store != nil {
		got, err := opts.Store.List()
		if err != nil {
			res := newResult(nil, opts.PendingApprovals)
			res.Reason = fmt.Sprintf("cannot read the persisted task graph: %v", err)
			return res
		}
		tasks = got
	}

	res := newResult(tasks, opts.PendingApprovals)
	source, reason := resolveSource(opts.Dir, opts.PlanSource)
	if source == "" {
		res.Reason = reason
		return res
	}

	// Stability: reconcile the same source and state a bounded number of times. A
	// disagreement is reported as nondeterministic rather than resolved by retrying
	// until it agrees, which would hide the instability the gate exists to catch.
	var first planflow.ReconcileResult
	var firstErr error
	for i := 0; i < attempts; i++ {
		got, err := inspect(ctx, planflow.ReconcileOptions{Dir: opts.Dir, PlanSource: source, Store: opts.Store})
		if i == 0 {
			first, firstErr = got, err
			continue
		}
		if !sameOutcome(first, firstErr, got, err) {
			res.Deterministic = false
			res.Source = first.Source
			res.PlanID = first.PlanID
			res.Classification = Nondeterministic
			res.NextAction = HumanReviewRequired
			res.Reason = "repeated reconciliation of the same source and state disagreed; the compiler is not stable, so the result cannot authorize a run"
			return res
		}
	}
	res.Deterministic = true

	if firstErr != nil {
		return classifyError(res, firstErr)
	}

	res.Source = first.Source
	res.PlanID = first.PlanID
	res.PlanChanged = first.PlanChanged
	res.Unchanged = idsOrEmpty(first.Unchanged)
	res.Updated = idsOrEmpty(first.Updated)
	res.Added = idsOrEmpty(first.Added)
	res.Removed = idsOrEmpty(first.Removed)
	res.ChangedExecuted = idsOrEmpty(first.ChangedExecuted)
	res.ChangedEquivalent = idsOrEmpty(first.ChangedExecutedEquivalent)
	res.ChangedMaterial = idsOrEmpty(first.ChangedExecutedMaterial)
	res.RemovedExecuted = idsOrEmpty(first.RemovedExecuted)

	res.Classification = classify(first)
	return decide(res)
}

// newResult builds the zero-state result for a task graph: it sets every slice
// non-nil, counts the satisfied and blocked tasks, and selects the current and
// runnable task. Classification and NextAction start at the fail-closed stop so an
// early return can never accidentally authorize a run.
func newResult(tasks []*domain.Task, pending []string) Result {
	res := Result{
		Classification:    Failed,
		NextAction:        HumanReviewRequired,
		Unchanged:         []string{},
		Updated:           []string{},
		Added:             []string{},
		Removed:           []string{},
		ChangedExecuted:   []string{},
		ChangedEquivalent: []string{},
		ChangedMaterial:   []string{},
		RemovedExecuted:   []string{},
		PendingApprovals:  normalizeIDs(pending),
		BlockedTasks:      []string{},
		TasksTotal:        len(tasks),
		PlanComplete:      len(tasks) > 0 && domain.AllSatisfied(tasks),
	}
	for _, t := range tasks {
		if t.IsSatisfied() {
			res.TasksSatisfied++
		}
		if t.IsBlocked() {
			res.BlockedTasks = append(res.BlockedTasks, t.ID)
		}
	}
	sort.Strings(res.BlockedTasks)
	res.CurrentTask, res.CurrentTaskStatus, res.RunnableTask = selection(tasks)
	return res
}

// classifyError maps a reconciliation failure to the classification and next
// action. A capability the plan requires whose owner the source does not determine
// is a planning ambiguity a human must resolve (a new prerequisite), so it is a
// semantic change. An uncompilable source and any other failure cannot establish a
// continuation state, so they are Failed. Both stop for human review.
func classifyError(res Result, err error) Result {
	switch {
	case planner.IsNeedsHuman(err):
		res.Classification = SemanticChange
		res.CapabilityGap = true
	case errors.Is(err, planflow.ErrUnsupportedReconcileSource):
		res.Classification = Failed
	default:
		res.Classification = Failed
	}
	res.NextAction = HumanReviewRequired
	res.Reason = err.Error()
	return res
}

// classify maps a successful read-only reconciliation to one classification. It is
// conservative by construction: only a change that provably preserves executable
// meaning (plan-level metadata, or descriptive text on an executed task) is
// Changed; any task-level membership or executable-semantics change is a semantic
// change.
func classify(res planflow.ReconcileResult) Classification {
	switch {
	case len(res.RemovedExecuted) > 0 || len(res.ChangedExecutedMaterial) > 0:
		return SemanticChange
	case len(res.Added) > 0 || len(res.Removed) > 0 || len(res.Updated) > 0:
		return SemanticChange
	case res.PlanChanged || len(res.ChangedExecutedEquivalent) > 0:
		return Changed
	default:
		return Clean
	}
}

// decide turns a compatible classification plus the persisted lifecycle into the
// single recommended action. A reconciliation that needs a human always wins; only
// a Clean or Changed result can reach a run, and even then a completed plan, a
// genuinely blocked plan, or a pending approval gate stops it.
func decide(res Result) Result {
	switch res.Classification {
	case SemanticChange:
		res.NextAction = HumanReviewRequired
		res.Reason = "the requested plan would change plan meaning; a human must review the source plan before execution continues"
		return res
	case Failed:
		res.NextAction = HumanReviewRequired
		res.Reason = "reconciliation could not establish a valid continuation state"
		return res
	case Nondeterministic:
		res.NextAction = HumanReviewRequired
		return res
	}

	switch {
	case res.PlanComplete:
		res.NextAction = PlanComplete
		res.Reason = "every task is satisfied; there is nothing to continue"
	// Runnable work is always selected first, so a BLOCKED task alongside it does
	// not stop the run; only a plan with no runnable work and a BLOCKED task is the
	// operator-action boundary this gate reports. A plan merely waiting on
	// dependency ordering is not blocked, so it stays eligible for a run.
	case res.RunnableTask == "" && len(res.BlockedTasks) > 0:
		res.NextAction = Blocked
		res.Reason = "no runnable or recoverable work remains and a task is BLOCKED; resolve it explicitly (for example `sop retry`)"
	case len(res.PendingApprovals) > 0:
		res.NextAction = ApprovalRequired
		res.Reason = "a pending human approval gate must be resolved before continuing"
	default:
		res.NextAction = ContinueSafe
		res.Reason = "reconciliation is compatible with the persisted state; the governed run may continue"
	}
	return res
}

// selection reports the task occupying the execution slot (the in-flight task, or
// the lowest-ID runnable task when none is active), and the lowest-ID runnable
// task. It reuses the domain predicates the scheduler composes, so a continuation
// check and a run agree on what is runnable without duplicating the scheduler.
func selection(tasks []*domain.Task) (current string, status domain.TaskStatus, runnable string) {
	byID := index(tasks)
	for _, t := range tasks {
		if t.Status != domain.PLANNED && t.Status != domain.MERGED && !t.Status.IsTerminal() {
			current = t.ID
			status = t.Status
			break
		}
	}
	var eligible []*domain.Task
	for _, t := range tasks {
		if !t.IsRunnable() {
			continue
		}
		if _, satisfied := t.ResolveDependencies(byID); !satisfied {
			continue
		}
		if !t.CanTransitionTo(domain.READY) {
			continue
		}
		eligible = append(eligible, t)
	}
	if len(eligible) > 0 {
		sort.Slice(eligible, func(i, j int) bool { return eligible[i].ID < eligible[j].ID })
		runnable = eligible[0].ID
		if current == "" {
			current = runnable
		}
	}
	return current, status, runnable
}

// resolveSource returns the absolute source plan to reconcile, or an empty string
// and an actionable reason. An explicit source is resolved against Dir; otherwise
// the recorded active plan's own source is used, so continuation always reconciles
// the plan SOP already owns rather than discovering an unrelated document.
func resolveSource(dir, explicit string) (path, reason string) {
	if s := strings.TrimSpace(explicit); s != "" {
		if !filepath.IsAbs(s) {
			s = filepath.Join(dir, s)
		}
		return s, ""
	}
	_, recorded, ok := planflow.ActivePlanID(dir)
	if !ok || strings.TrimSpace(recorded) == "" {
		return "", "no active plan: SOP has no recorded plan to continue; run `sop run` to establish one"
	}
	if filepath.IsAbs(recorded) {
		return recorded, ""
	}
	return filepath.Join(dir, recorded), ""
}

// sameOutcome reports whether two reconciliation passes agree on everything that
// could change the classification. Comparing the classified fields (not the whole
// struct) keeps the stability check tied to the decision, so an incidental
// unexported field cannot make a stable result look nondeterministic.
func sameOutcome(a planflow.ReconcileResult, aerr error, b planflow.ReconcileResult, berr error) bool {
	if (aerr == nil) != (berr == nil) {
		return false
	}
	if aerr != nil {
		return aerr.Error() == berr.Error()
	}
	return a.Source == b.Source &&
		a.PlanID == b.PlanID &&
		a.PlanChanged == b.PlanChanged &&
		equalIDs(a.Unchanged, b.Unchanged) &&
		equalIDs(a.Updated, b.Updated) &&
		equalIDs(a.Added, b.Added) &&
		equalIDs(a.Removed, b.Removed) &&
		equalIDs(a.ChangedExecuted, b.ChangedExecuted) &&
		equalIDs(a.ChangedExecutedEquivalent, b.ChangedExecutedEquivalent) &&
		equalIDs(a.ChangedExecutedMaterial, b.ChangedExecutedMaterial) &&
		equalIDs(a.RemovedExecuted, b.RemovedExecuted)
}

func equalIDs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func normalizeIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if s := strings.TrimSpace(id); s != "" {
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func idsOrEmpty(ids []string) []string {
	if ids == nil {
		return []string{}
	}
	return ids
}

func index(tasks []*domain.Task) map[string]*domain.Task {
	byID := make(map[string]*domain.Task, len(tasks))
	for _, t := range tasks {
		byID[t.ID] = t
	}
	return byID
}

// String renders a one-line summary; it is used by tests and diagnostics.
func (r Result) String() string {
	return fmt.Sprintf("%s/%s: %s", r.Classification, r.NextAction, r.Reason)
}
