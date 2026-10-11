package planflow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/run"
)

// TestCompleteArchivesFullySatisfiedPlan proves a completed plan is archived as
// COMPLETE, its task records are preserved, and it is no longer ACTIVE. Every
// satisfied task carries recorded verification evidence (a PASSED run), so the
// clean plan completes exactly as before the closure tightening.
func TestCompleteArchivesFullySatisfiedPlan(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", reconcileDocTwo)
	for _, task := range st.tasks {
		task.Status = domain.LOCAL_DONE
		recordPassedRun(t, dir, task.ID)
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

// TestCompleteRefusesUnresolvedApproval proves completion fails closed when a
// satisfied, verified task still carries a PENDING approval gate bound to the
// active plan, so `plan complete` matches historicalize readiness.
func TestCompleteRefusesUnresolvedApproval(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", reconcileDocTwo)
	for _, task := range st.tasks {
		task.Status = domain.LOCAL_DONE
		recordPassedRun(t, dir, task.ID)
	}
	recordPendingApproval(t, dir, "S001")

	_, err := Complete(Options{Dir: dir, Store: st})
	if err == nil {
		t.Fatal("must refuse to complete a plan with an unresolved approval")
	}
	if !strings.Contains(err.Error(), "approval") || !strings.Contains(err.Error(), "S001") {
		t.Errorf("error %q must name the unresolved approval", err)
	}
	if len(st.tasks) == 0 {
		t.Error("tasks were cleared despite the refusal")
	}
	if _, _, ok := ActivePlanID(dir); !ok {
		t.Error("the plan lost its active association on a refused completion")
	}
}

// TestCompleteRefusesMissingVerification proves completion fails closed when a
// satisfied task has no recorded verification evidence, so `plan complete`
// matches historicalize readiness.
func TestCompleteRefusesMissingVerification(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", reconcileDocTwo)
	for _, task := range st.tasks {
		task.Status = domain.LOCAL_DONE
	}
	// No run evidence is seeded for S002, so completion must refuse.
	recordPassedRun(t, dir, "S001")

	_, err := Complete(Options{Dir: dir, Store: st})
	if err == nil {
		t.Fatal("must refuse to complete a plan with missing verification")
	}
	if !strings.Contains(err.Error(), "verification") || !strings.Contains(err.Error(), "S002") {
		t.Errorf("error %q must name the missing verification", err)
	}
	if len(st.tasks) == 0 {
		t.Error("tasks were cleared despite the refusal")
	}
	if _, _, ok := ActivePlanID(dir); !ok {
		t.Error("the plan lost its active association on a refused completion")
	}
}

// TestCompleteIgnoresUnrelatedPlanApproval proves a PENDING approval bound to a task
// outside the active plan does not block completion: scoping, not staleness, is the
// blocker (I2). Only a gate on an active-plan task is a plan-level blocker.
func TestCompleteIgnoresUnrelatedPlanApproval(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", reconcileDocTwo)
	for _, task := range st.tasks {
		task.Status = domain.LOCAL_DONE
		recordPassedRun(t, dir, task.ID)
	}
	// A PENDING gate on a task that is NOT part of the active plan.
	recordPendingApproval(t, dir, "OTHER-1")

	if _, err := Complete(Options{Dir: dir, Store: st}); err != nil {
		t.Fatalf("an unrelated-plan approval must not block completion: %v", err)
	}
}

// TestCompleteIgnoresSupersededApproval proves a SUPERSEDED approval is resolved and
// does not block completion, so an operator can clear a stale gate and still close
// the plan (LC-007 + LC-004 together).
func TestCompleteIgnoresSupersededApproval(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", reconcileDocTwo)
	for _, task := range st.tasks {
		task.Status = domain.LOCAL_DONE
		recordPassedRun(t, dir, task.ID)
	}
	recordSupersededApproval(t, dir, "S001")

	if _, err := Complete(Options{Dir: dir, Store: st}); err != nil {
		t.Fatalf("a SUPERSEDED approval must not block completion: %v", err)
	}
}

// TestCompleteExecutionDoneNeedsNoVerification proves the ExecutionDone exemption is
// preserved: a satisfied task declared ExecutionDone needs no run evidence to close.
func TestCompleteExecutionDoneNeedsNoVerification(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", reconcileDocTwo)
	for _, task := range st.tasks {
		task.Status = domain.LOCAL_DONE
		task.ExecutionMode = domain.ExecutionDone
	}

	if _, err := Complete(Options{Dir: dir, Store: st}); err != nil {
		t.Fatalf("an ExecutionDone plan must complete without run evidence: %v", err)
	}
}

// TestCompleteRefusesWhenNoPlanActive proves completion fails closed with no active plan.
func TestCompleteRefusesWhenNoPlanActive(t *testing.T) {
	dir := t.TempDir()
	if _, err := Complete(Options{Dir: dir, Store: &fakeStore{}}); err == nil {
		t.Fatal("must refuse when no plan is active")
	}
}

// recordPassedRun writes a PASSED run state for the task, the verification
// evidence a satisfied task needs before the plan can close.
func recordPassedRun(t *testing.T, dir, id string) {
	t.Helper()
	r, err := run.Open(dir, id)
	if err != nil {
		t.Fatalf("open run %s: %v", id, err)
	}
	if err := r.SetStage(run.Passed); err != nil {
		t.Fatalf("record PASSED for %s: %v", id, err)
	}
}

// recordPendingApproval writes a PENDING approval request bound to the task, the
// plan-level gate that blocks closure.
func recordPendingApproval(t *testing.T, dir, id string) {
	t.Helper()
	r, err := run.Open(dir, id)
	if err != nil {
		t.Fatalf("open run %s: %v", id, err)
	}
	if err := r.SaveApproval(domain.ApprovalRequest{ID: "APR-1", TaskID: id, Status: domain.ApprovalPending}); err != nil {
		t.Fatalf("record approval for %s: %v", id, err)
	}
}

// recordSupersededApproval writes a SUPERSEDED approval request bound to the task,
// the resolved state an operator records for a stale gate (LC-007). It is resolved,
// so it must not block closure.
func recordSupersededApproval(t *testing.T, dir, id string) {
	t.Helper()
	r, err := run.Open(dir, id)
	if err != nil {
		t.Fatalf("open run %s: %v", id, err)
	}
	if err := r.SaveApproval(domain.ApprovalRequest{ID: "APR-1", TaskID: id, Status: domain.ApprovalSuperseded}); err != nil {
		t.Fatalf("record supersession for %s: %v", id, err)
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
