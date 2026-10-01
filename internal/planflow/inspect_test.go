package planflow

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
)

// inspect is the read-only listing a client uses to see what a reconciliation
// would change before anything is applied.
func inspect(t *testing.T, dir, name string, st *fakeStore) (ReconcileResult, error) {
	t.Helper()
	return Inspect(context.Background(), ReconcileOptions{
		Dir:        dir,
		PlanSource: filepath.Join(dir, name),
		Store:      st,
	})
}

// TestInspectReportsChangedExecutedWithoutMutating proves the listing names the
// changed executed task a reconciliation would stop on, and applies nothing: the
// graph, the machine plan, and the provenance are all left exactly as they were.
func TestInspectReportsChangedExecutedWithoutMutating(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", planDoc)
	markExecuted(taskByID(t, st.tasks, "S001"))
	write(t, dir, "PLAN.md", strings.ReplaceAll(planDoc, "Application skeleton", "Renamed"))

	planPath := filepath.Join(dir, config.DirName, planFileName)
	metaPath := filepath.Join(dir, config.DirName, metaFileName)
	beforePlan := read(t, planPath)
	beforeMeta := read(t, metaPath)
	beforeTasks := len(st.tasks)

	res, err := inspect(t, dir, "PLAN.md", st)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if !res.PlanChanged {
		t.Error("PlanChanged = false, want true (the requested plan differs)")
	}
	if len(res.ChangedExecuted) != 1 || res.ChangedExecuted[0] != "S001" {
		t.Errorf("ChangedExecuted = %v, want [S001]", res.ChangedExecuted)
	}
	if len(res.RemovedExecuted) != 0 {
		t.Errorf("RemovedExecuted = %v, want none", res.RemovedExecuted)
	}
	if len(res.Updated) != 0 {
		t.Errorf("Updated = %v, want none (an executed change is not silently updated)", res.Updated)
	}

	// Nothing was applied.
	if got := taskByID(t, st.tasks, "S001"); got.Title != "Application skeleton" || got.Status != domain.LOCAL_DONE {
		t.Errorf("S001 = %+v, want its definition and history untouched", got)
	}
	if len(st.tasks) != beforeTasks {
		t.Errorf("task count = %d, want %d", len(st.tasks), beforeTasks)
	}
	if got := read(t, planPath); got != beforePlan {
		t.Error("Inspect rewrote the machine plan")
	}
	if got := read(t, metaPath); got != beforeMeta {
		t.Error("Inspect rewrote the provenance")
	}
}

// TestInspectReportsRemovedExecuted proves a task the requested plan drops is
// reported rather than approved: a removed executed task cannot be reconciled
// with --accept-changed, so the listing must never offer it as one.
func TestInspectReportsRemovedExecuted(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", reconcileDocTwo)
	markExecuted(taskByID(t, st.tasks, "S002"))
	// The requested plan drops S002 entirely.
	write(t, dir, "PLAN.md", "# Implementation Plan\n\n## Project\n\nWidget\n\n## Summary\n\nAdd a widget.\n\n## S001 — Application skeleton\n\nCreate it.\n\n### Acceptance Criteria\n\n- starts\n")

	res, err := inspect(t, dir, "PLAN.md", st)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if len(res.RemovedExecuted) != 1 || res.RemovedExecuted[0] != "S002" {
		t.Errorf("RemovedExecuted = %v, want [S002]", res.RemovedExecuted)
	}
	if len(res.ChangedExecuted) != 0 {
		t.Errorf("ChangedExecuted = %v, want none", res.ChangedExecuted)
	}
	// The removed executed task is still in the graph the listing validated, and
	// was not removed from the store.
	if got := taskByID(t, st.tasks, "S002"); got.ID != "S002" {
		t.Errorf("S002 = %+v, want it kept", got)
	}
}

// TestInspectUnchangedPlanReportsNoHumanDecision proves a plan that already matches
// needs no decision, and the listing says so.
func TestInspectUnchangedPlanReportsNoHumanDecision(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", planDoc)
	markExecuted(taskByID(t, st.tasks, "S001"))

	res, err := inspect(t, dir, "PLAN.md", st)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if res.PlanChanged {
		t.Error("PlanChanged = true, want false for an unchanged plan")
	}
	if len(res.ChangedExecuted) != 0 || len(res.RemovedExecuted) != 0 {
		t.Errorf("changed=%v removed=%v, want none", res.ChangedExecuted, res.RemovedExecuted)
	}
	if len(res.Unchanged) != 1 {
		t.Errorf("Unchanged = %v, want the executed task listed as unchanged", res.Unchanged)
	}
}

// TestInspectClassificationMatchesReconcile proves the listing and the applied
// reconciliation are one diff: a plan a reconciliation can apply reports exactly
// the categories the reconciliation then applies.
func TestInspectClassificationMatchesReconcile(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", planDoc)
	// Rename S001 before it has executed: the reconciliation may update it silently.
	write(t, dir, "PLAN.md", strings.ReplaceAll(planDoc, "Application skeleton", "Renamed"))

	listed, err := inspect(t, dir, "PLAN.md", st)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if len(listed.Updated) != 1 || listed.Updated[0] != "S001" {
		t.Fatalf("Inspect Updated = %v, want [S001]", listed.Updated)
	}

	applied, err := reconcile(t, dir, "PLAN.md", st)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if strings.Join(applied.Updated, ",") != strings.Join(listed.Updated, ",") {
		t.Errorf("applied Updated = %v, listed Updated = %v; the listing and the reconciliation must agree", applied.Updated, listed.Updated)
	}
	if len(applied.ChangedExecuted) != 0 || len(listed.ChangedExecuted) != 0 {
		t.Errorf("changed executed: applied=%v listed=%v, want none", applied.ChangedExecuted, listed.ChangedExecuted)
	}
}
