package continuation

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/planflow"
	"github.com/imhttran/agentic-sop/internal/planner"
)

// These tests exercise the deterministic, offline reconcile-before-continue
// decision. They inject a fake Inspector so the classification and the continuation
// validation can be tested without a model, a provider, or a repository, and they
// use a source path that is never read (the injected inspector ignores it).

const testSource = "/project/docs/plans/PLAN-Example.md"

// fixedInspector returns the same reconciliation result (or error) every call, so
// the stability check is satisfied and the classification is deterministic.
func fixedInspector(res planflow.ReconcileResult, err error) Inspector {
	return func(context.Context, planflow.ReconcileOptions) (planflow.ReconcileResult, error) {
		return res, err
	}
}

// task builds a persisted task for a state scenario.
func task(id string, status domain.TaskStatus, deps ...string) *domain.Task {
	return &domain.Task{ID: id, Title: id, Status: status, DependencyIDs: deps}
}

func check(t *testing.T, opts Options, inspect Inspector) Result {
	t.Helper()
	if opts.Dir == "" {
		opts.Dir = t.TempDir()
	}
	if opts.PlanSource == "" {
		opts.PlanSource = testSource
	}
	opts.Inspect = inspect
	return Check(context.Background(), opts)
}

// Scenario 1: a clean reconciliation authorizes continuation.
func TestCheckCleanAllowsContinuation(t *testing.T) {
	res := check(t, Options{Tasks: []*domain.Task{task("T001", domain.PLANNED)}},
		fixedInspector(planflow.ReconcileResult{PlanChanged: false, Unchanged: []string{"T001"}}, nil))
	if res.Classification != Clean {
		t.Fatalf("classification = %s, want %s", res.Classification, Clean)
	}
	if res.NextAction != ContinueSafe {
		t.Fatalf("next action = %s, want %s (%s)", res.NextAction, ContinueSafe, res.Reason)
	}
	if !res.Deterministic {
		t.Error("a stable result must be reported deterministic")
	}
}

// Scenario 2: a semantics-preserving plan-level change authorizes continuation
// after validation.
func TestCheckMetadataChangeIsChangedAndAllowsContinuation(t *testing.T) {
	res := check(t, Options{Tasks: []*domain.Task{task("T001", domain.PLANNED)}},
		fixedInspector(planflow.ReconcileResult{PlanChanged: true, Unchanged: []string{"T001"}}, nil))
	if res.Classification != Changed {
		t.Fatalf("classification = %s, want %s", res.Classification, Changed)
	}
	if res.NextAction != ContinueSafe {
		t.Fatalf("next action = %s, want %s (%s)", res.NextAction, ContinueSafe, res.Reason)
	}
}

// A semantics-preserving executed change (descriptive text only) is Changed, not a
// semantic change.
func TestCheckEquivalentExecutedChangeIsChanged(t *testing.T) {
	res := check(t, Options{Tasks: []*domain.Task{
		task("T001", domain.LOCAL_DONE),
		task("T002", domain.PLANNED),
	}}, fixedInspector(planflow.ReconcileResult{
		PlanChanged:               true,
		ChangedExecuted:           []string{"T001"},
		ChangedExecutedEquivalent: []string{"T001"},
	}, nil))
	if res.Classification != Changed {
		t.Fatalf("classification = %s, want %s", res.Classification, Changed)
	}
	if res.NextAction != ContinueSafe {
		t.Fatalf("next action = %s, want %s (%s)", res.NextAction, ContinueSafe, res.Reason)
	}
}

// Scenario 3: a dependency change is a semantic change and stops.
func TestCheckDependencyChangeStops(t *testing.T) {
	res := check(t, Options{Tasks: []*domain.Task{
		task("T001", domain.LOCAL_DONE),
		task("T002", domain.PLANNED, "T001"),
	}}, fixedInspector(planflow.ReconcileResult{PlanChanged: true, Updated: []string{"T002"}}, nil))
	if res.Classification != SemanticChange {
		t.Fatalf("classification = %s, want %s", res.Classification, SemanticChange)
	}
	if res.NextAction != HumanReviewRequired {
		t.Fatalf("next action = %s, want %s", res.NextAction, HumanReviewRequired)
	}
}

// Scenario 4: a task identity change (an ID rename is a remove plus an add) stops.
func TestCheckTaskIDChangeStops(t *testing.T) {
	res := check(t, Options{Tasks: []*domain.Task{task("T001", domain.PLANNED)}},
		fixedInspector(planflow.ReconcileResult{PlanChanged: true, Added: []string{"T002"}, Removed: []string{"T001"}}, nil))
	if res.Classification != SemanticChange {
		t.Fatalf("classification = %s, want %s", res.Classification, SemanticChange)
	}
	if res.NextAction != HumanReviewRequired {
		t.Fatalf("next action = %s, want %s", res.NextAction, HumanReviewRequired)
	}
}

// Scenario 5: a material acceptance-semantics change on an executed task stops.
func TestCheckAcceptanceChangeStops(t *testing.T) {
	res := check(t, Options{Tasks: []*domain.Task{
		task("T001", domain.LOCAL_DONE),
		task("T002", domain.PLANNED),
	}}, fixedInspector(planflow.ReconcileResult{
		PlanChanged:             true,
		ChangedExecuted:         []string{"T001"},
		ChangedExecutedMaterial: []string{"T001"},
	}, nil))
	if res.Classification != SemanticChange {
		t.Fatalf("classification = %s, want %s", res.Classification, SemanticChange)
	}
	if res.NextAction != HumanReviewRequired {
		t.Fatalf("next action = %s, want %s", res.NextAction, HumanReviewRequired)
	}
}

// Scenarios 6 and 7: a newly required capability with no determined owner is a
// semantic change; the check must not add a prerequisite, and it mutates nothing.
func TestCheckCapabilityGapStopsWithoutAddingPrerequisite(t *testing.T) {
	gap := &planner.CapabilityGapError{Capabilities: []planner.Capability{
		{Name: "External Tool", Status: planner.CapabilityMissing, Gap: "absent"},
	}}
	tasks := []*domain.Task{task("T001", domain.PLANNED)}
	before := len(tasks[0].DependencyIDs)
	res := check(t, Options{Tasks: tasks}, fixedInspector(planflow.ReconcileResult{}, gap))
	if res.Classification != SemanticChange {
		t.Fatalf("classification = %s, want %s", res.Classification, SemanticChange)
	}
	if !res.CapabilityGap {
		t.Error("capability_gap must be reported for a capability-gap stop")
	}
	if res.NextAction != HumanReviewRequired {
		t.Fatalf("next action = %s, want %s", res.NextAction, HumanReviewRequired)
	}
	// No silent prerequisite: the check never mutates a task or adds a dependency.
	if len(tasks[0].DependencyIDs) != before {
		t.Error("the check added a dependency; a capability must never be silently adopted")
	}
	if tasks[0].Status != domain.PLANNED {
		t.Errorf("task status changed to %s", tasks[0].Status)
	}
}

// Scenario 8: a compiler that returns different results for the same source and
// state is nondeterministic and stops, so it never reaches `sop run`.
func TestCheckRecurringRepairIsNondeterministic(t *testing.T) {
	calls := 0
	flaky := func(context.Context, planflow.ReconcileOptions) (planflow.ReconcileResult, error) {
		calls++
		if calls == 1 {
			return planflow.ReconcileResult{PlanChanged: true, Updated: []string{"T001"}}, nil
		}
		return planflow.ReconcileResult{PlanChanged: true, Added: []string{"T009"}}, nil
	}
	res := check(t, Options{Tasks: []*domain.Task{task("T001", domain.PLANNED)}}, flaky)
	if res.Classification != Nondeterministic {
		t.Fatalf("classification = %s, want %s", res.Classification, Nondeterministic)
	}
	if res.NextAction != HumanReviewRequired {
		t.Fatalf("next action = %s, want %s", res.NextAction, HumanReviewRequired)
	}
	if res.Deterministic {
		t.Error("nondeterministic result must not be reported deterministic")
	}
	if calls != DefaultAttempts {
		t.Errorf("inspector called %d times, want the bounded %d", calls, DefaultAttempts)
	}
}

// A compile failure is Failed (cannot establish a continuation state), and an
// uncompilable changed source is the same stop.
func TestCheckCompileFailureIsFailed(t *testing.T) {
	for _, err := range []error{
		errors.New("NEEDS_HUMAN: no persisted plan to reconcile against"),
		planflow.ErrUnsupportedReconcileSource,
	} {
		res := check(t, Options{Tasks: []*domain.Task{task("T001", domain.PLANNED)}}, fixedInspector(planflow.ReconcileResult{}, err))
		if res.Classification != Failed {
			t.Errorf("err %v: classification = %s, want %s", err, res.Classification, Failed)
		}
		if res.NextAction != HumanReviewRequired {
			t.Errorf("err %v: next action = %s, want %s", err, res.NextAction, HumanReviewRequired)
		}
	}
}

// No active plan and no explicit source is a stop, not a fresh graph.
func TestCheckNoActivePlanStops(t *testing.T) {
	res := Check(context.Background(), Options{
		Dir:     t.TempDir(),
		Inspect: fixedInspector(planflow.ReconcileResult{}, nil),
	})
	if res.Classification != Failed {
		t.Fatalf("classification = %s, want %s", res.Classification, Failed)
	}
	if !strings.Contains(res.Reason, "no active plan") {
		t.Errorf("reason %q should name the missing active plan", res.Reason)
	}
}

// Scenarios 9 and 10: persisted LOCAL_DONE and NOT_REQUIRED are preserved, and a
// plan whose unfinished work is still runnable continues.
func TestCheckPreservesLocalDoneAndNotRequired(t *testing.T) {
	tasks := []*domain.Task{
		task("T001", domain.LOCAL_DONE),
		task("T002", domain.NOT_REQUIRED),
		task("T003", domain.PLANNED),
	}
	res := check(t, Options{Tasks: tasks},
		fixedInspector(planflow.ReconcileResult{Unchanged: []string{"T001", "T002", "T003"}}, nil))
	if res.NextAction != ContinueSafe {
		t.Fatalf("next action = %s, want %s (%s)", res.NextAction, ContinueSafe, res.Reason)
	}
	if tasks[0].Status != domain.LOCAL_DONE {
		t.Errorf("LOCAL_DONE task changed to %s", tasks[0].Status)
	}
	if tasks[1].Status != domain.NOT_REQUIRED {
		t.Errorf("NOT_REQUIRED task changed to %s", tasks[1].Status)
	}
}

// Scenario 12: a task requeued to PLANNED after a CONTINUE retains its history and
// its artifact is not fabricated as complete; the gate still allows the run so SOP's
// own acceptance verification can decide.
func TestCheckRequeuePreservesHistoryAndRunnable(t *testing.T) {
	requeued := &domain.Task{
		ID:          "T001",
		Status:      domain.PLANNED,
		Attempt:     1,
		Attempts:    []domain.Attempt{{Number: 1, Status: domain.IMPLEMENTING, Reason: "CONTINUE"}},
		MaxAttempts: 3,
	}
	res := check(t, Options{Tasks: []*domain.Task{requeued}},
		fixedInspector(planflow.ReconcileResult{Unchanged: []string{"T001"}}, nil))
	if res.NextAction != ContinueSafe {
		t.Fatalf("next action = %s, want %s (%s)", res.NextAction, ContinueSafe, res.Reason)
	}
	if len(requeued.Attempts) != 1 || requeued.Attempts[0].Reason != "CONTINUE" {
		t.Errorf("requeued task history was not preserved: %+v", requeued.Attempts)
	}
}

// Scenario 13: a pending approval gate prevents continuation.
func TestCheckPendingApprovalStops(t *testing.T) {
	res := check(t, Options{
		Tasks:            []*domain.Task{task("T001", domain.PLANNED)},
		PendingApprovals: []string{"T001"},
	}, fixedInspector(planflow.ReconcileResult{Unchanged: []string{"T001"}}, nil))
	if res.NextAction != ApprovalRequired {
		t.Fatalf("next action = %s, want %s (%s)", res.NextAction, ApprovalRequired, res.Reason)
	}
	if len(res.PendingApprovals) != 1 || res.PendingApprovals[0] != "T001" {
		t.Errorf("pending approvals = %v, want [T001]", res.PendingApprovals)
	}
}

// Scenario 14: a genuinely blocked plan with no runnable work prevents continuation.
func TestCheckBlockedStops(t *testing.T) {
	res := check(t, Options{Tasks: []*domain.Task{
		task("T001", domain.LOCAL_DONE),
		{ID: "T002", Status: domain.BLOCKED, BlockedReason: domain.RETRIES_EXHAUSTED},
	}}, fixedInspector(planflow.ReconcileResult{Unchanged: []string{"T001", "T002"}}, nil))
	if res.NextAction != Blocked {
		t.Fatalf("next action = %s, want %s (%s)", res.NextAction, Blocked, res.Reason)
	}
}

// A BLOCKED task alongside runnable work does not stop the run (the scheduler
// selects runnable work first).
func TestCheckBlockedWithRunnableWorkContinues(t *testing.T) {
	res := check(t, Options{Tasks: []*domain.Task{
		{ID: "T001", Status: domain.BLOCKED, BlockedReason: domain.RETRIES_EXHAUSTED},
		task("T002", domain.PLANNED),
	}}, fixedInspector(planflow.ReconcileResult{Unchanged: []string{"T001", "T002"}}, nil))
	if res.NextAction != ContinueSafe {
		t.Fatalf("next action = %s, want %s (%s)", res.NextAction, ContinueSafe, res.Reason)
	}
}

// Scenario 23: a completed plan does not run again.
func TestCheckPlanCompleteDoesNotRun(t *testing.T) {
	res := check(t, Options{Tasks: []*domain.Task{
		task("T001", domain.LOCAL_DONE),
		task("T002", domain.NOT_REQUIRED),
	}}, fixedInspector(planflow.ReconcileResult{Unchanged: []string{"T001", "T002"}}, nil))
	if res.NextAction != PlanComplete {
		t.Fatalf("next action = %s, want %s (%s)", res.NextAction, PlanComplete, res.Reason)
	}
}

// Scenario 22: the stability check is bounded to a small, fixed number of passes and
// never becomes an unbounded reconciliation loop.
func TestCheckIsBounded(t *testing.T) {
	calls := 0
	counter := func(context.Context, planflow.ReconcileOptions) (planflow.ReconcileResult, error) {
		calls++
		return planflow.ReconcileResult{Unchanged: []string{"T001"}}, nil
	}
	res := check(t, Options{Tasks: []*domain.Task{task("T001", domain.PLANNED)}}, counter)
	if calls != DefaultAttempts {
		t.Fatalf("inspector called %d times, want exactly %d", calls, DefaultAttempts)
	}
	if res.NextAction != ContinueSafe {
		t.Fatalf("next action = %s, want %s", res.NextAction, ContinueSafe)
	}
}
