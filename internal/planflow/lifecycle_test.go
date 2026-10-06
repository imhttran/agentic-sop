package planflow

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
)

// TestReconcileRemovesNeverExecutedReadyTask proves a scheduled task that has never
// run (READY, no attempts) is an ordinary unexecuted removal, not a removed-executed
// task that would need an approval. This is the CTX-001 false-positive case.
func TestReconcileRemovesNeverExecutedReadyTask(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", reconcileDocTwo)
	// S002 is READY (dependencies met by S001) but has never executed.
	taskByID(t, st.tasks, "S002").Status = domain.READY

	write(t, dir, "PLAN.md", planDoc) // the requested plan drops S002
	res, err := reconcile(t, dir, "PLAN.md", st)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(res.Removed) != 1 || res.Removed[0] != "S002" {
		t.Errorf("removed = %v, want [S002]", res.Removed)
	}
	if len(res.RemovedExecuted) != 0 {
		t.Errorf("removed_executed = %v, want none (S002 never executed)", res.RemovedExecuted)
	}
}

// TestReconcileStillProtectsExecutedRemoval proves a genuinely executed task that is
// removed still stops with a human boundary: the fix narrows classification to
// never-executed tasks only.
func TestReconcileStillProtectsExecutedRemoval(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", reconcileDocTwo)
	markExecuted(taskByID(t, st.tasks, "S002"))
	write(t, dir, "PLAN.md", planDoc)
	if _, err := reconcile(t, dir, "PLAN.md", st); err == nil {
		t.Fatal("removing an executed task must still stop with NEEDS_HUMAN")
	}
}

// TestSupersedeReplacesActivePlanPreservingEvidence proves an explicit supersession
// replaces an unfinished active plan: the old plan is archived as SUPERSEDED with
// its task states preserved verbatim, the new plan is installed with its first task
// never-executed, and nothing is fabricated.
func TestSupersedeReplacesActivePlanPreservingEvidence(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", reconcileDocTwo)
	markExecuted(taskByID(t, st.tasks, "S001")) // S001 done; S002 still PLANNED/unfinished
	write(t, dir, "PLAN2.md", planDocB)

	res, err := Supersede(context.Background(), Options{Dir: dir, PlanSource: filepath.Join(dir, "PLAN2.md"), Store: st})
	if err != nil {
		t.Fatalf("supersede: %v", err)
	}
	if res.SupersededPlanID == "" || res.Source != "PLAN2.md" {
		t.Fatalf("res = %+v", res)
	}
	if len(st.tasks) == 0 {
		t.Fatal("no tasks installed for the new plan")
	}
	for _, task := range st.tasks {
		if task.ID != "B001" {
			t.Errorf("unexpected task %s after supersede", task.ID)
		}
		if task.Executed() {
			t.Errorf("supersede must not execute %s", task.ID)
		}
	}

	base := filepath.Join(dir, config.DirName, "archive", res.SupersededPlanID)
	life := read(t, filepath.Join(base, "lifecycle.json"))
	if !strings.Contains(life, DispositionSuperseded) {
		t.Errorf("lifecycle = %s, want %s", life, DispositionSuperseded)
	}
	archived := read(t, filepath.Join(base, "tasks.json"))
	if !strings.Contains(archived, "\"S001\"") || !strings.Contains(archived, "LOCAL_DONE") {
		t.Errorf("archived tasks lost evidence: %s", archived)
	}
	if !strings.Contains(archived, "\"S002\"") {
		t.Errorf("archived tasks lost the unfinished S002: %s", archived)
	}
}

// TestSupersedeDoesNotArchiveOrExecuteWhenInvalid proves a plan that cannot be
// validated leaves the active plan and its evidence untouched.
func TestSupersedeDoesNotArchiveOrExecuteWhenInvalid(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", reconcileDocTwo)
	markExecuted(taskByID(t, st.tasks, "S001"))
	before := len(st.tasks)

	write(t, dir, "BAD.md", "# not a plan\n\nno stages here\n")
	if _, err := Supersede(context.Background(), Options{Dir: dir, PlanSource: filepath.Join(dir, "BAD.md"), Store: st}); err == nil {
		t.Fatal("an invalid plan must not supersede the active plan")
	}
	if len(st.tasks) != before {
		t.Errorf("active graph changed on a failed supersede: %d -> %d", before, len(st.tasks))
	}
}
