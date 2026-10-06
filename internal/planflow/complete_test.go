package planflow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
)

// TestCompleteArchivesFullySatisfiedPlan proves a completed plan is archived as
// COMPLETE, its task records are preserved, and it is no longer ACTIVE.
func TestCompleteArchivesFullySatisfiedPlan(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", reconcileDocTwo)
	for _, task := range st.tasks {
		task.Status = domain.LOCAL_DONE
	}
	n := len(st.tasks)

	res, err := Complete(Options{Dir: dir, Store: st})
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if res.Source != "PLAN.md" || res.Tasks != n || res.Archived == "" || res.PlanID == "" {
		t.Fatalf("res = %+v", res)
	}
	if len(st.tasks) != 0 {
		t.Errorf("tasks were not cleared: %d remain", len(st.tasks))
	}
	if _, _, ok := ActivePlanID(dir); ok {
		t.Error("the completed plan is still ACTIVE")
	}
	if fileExists(filepath.Join(dir, config.DirName, planFileName)) {
		t.Error("plan.json was not removed")
	}
	life := readLifecycle(t, filepath.Join(dir, config.DirName, archiveDirName, res.PlanID, lifecycleFile))
	if life.Disposition != DispositionComplete {
		t.Errorf("disposition = %q, want COMPLETE", life.Disposition)
	}
}

// TestCompleteRefusesUnresolvedWork proves completion fails closed when any task is
// unresolved, leaving the plan active and its records intact.
func TestCompleteRefusesUnresolvedWork(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", reconcileDocTwo) // tasks are PLANNED

	if _, err := Complete(Options{Dir: dir, Store: st}); err == nil {
		t.Fatal("must refuse to complete a plan with unresolved work")
	}
	if len(st.tasks) == 0 {
		t.Error("tasks were cleared despite the refusal")
	}
	if _, _, ok := ActivePlanID(dir); !ok {
		t.Error("the plan lost its active association on a refused completion")
	}
}

// TestCompleteRefusesWhenNoPlanActive proves completion fails closed with no active plan.
func TestCompleteRefusesWhenNoPlanActive(t *testing.T) {
	dir := t.TempDir()
	if _, err := Complete(Options{Dir: dir, Store: &fakeStore{}}); err == nil {
		t.Fatal("must refuse when no plan is active")
	}
}

func readLifecycle(t *testing.T, path string) Lifecycle {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read lifecycle: %v", err)
	}
	var life Lifecycle
	if err := json.Unmarshal(data, &life); err != nil {
		t.Fatalf("parse lifecycle: %v", err)
	}
	return life
}
