package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/store"
)

// These tests are the focused automatic-recovery coverage TASK007 requires: they
// drive the new scheduler path (`sop run` automatically requeuing and
// re-evaluating a BLOCKED task) and assert that it does not weaken the existing
// PREJEV016 recovery guarantees. The plain recovery paths are covered by
// recovery_test.go; these tests exercise the automatic scheduler path instead of
// the explicit `sop retry` / `sop resume` entry points.

// gitStatusOutput returns `git status --porcelain` for dir, so a test can prove
// exactly which working-tree files a recovery touched.
func gitStatusOutput(t *testing.T, dir string) string {
	t.Helper()
	env := append(os.Environ(),
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_SYSTEM="+os.DevNull,
	)
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git status: %v\n%s", err, out)
	}
	return string(out)
}

// TestAutomaticRecoveryPreservesWorkingTreeAndRunHistory proves the automatic
// blocked-task recovery path is as non-destructive as the explicit retry and
// resume paths (the G1 guarantee): recovering a BLOCKED task keeps a pre-existing
// tracked edit and untracked file, does not delete the state database or prior
// run history, and never runs `git reset --hard` or `git clean` (which the canary
// files would reveal). It runs the scheduler path, not `sop retry`.
func TestAutomaticRecoveryPreservesWorkingTreeAndRunHistory(t *testing.T) {
	dir := newDirtyRepo(t)
	initProject(t, dir)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "T001", Status: domain.BLOCKED, BlockedReason: domain.REVIEW_UNRESOLVED, Attempt: 1, MaxAttempts: 3})
	seedRunStage(t, dir, "T001", "REVIEWING")
	a := &recoveryAgent{}

	code, stdout, stderr := runInjectedCLI(t, dir, "diff\n", a, "run")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	// The task really was recovered through the automatic scheduler path.
	if !strings.Contains(stdout, "Recovering: T001") {
		t.Errorf("stdout missing automatic recovery of T001:\n%s", stdout)
	}

	// The pre-existing user work survives: no `git reset --hard`, no `git clean`.
	assertRecoveryPreserved(t, dir, "T001")

	// The working tree still carries exactly the user's two changes, untouched.
	status := gitStatusOutput(t, dir)
	for _, want := range []string{" M tracked.txt", "?? untracked.txt"} {
		if !strings.Contains(status, want) {
			t.Errorf("git status missing %q after automatic recovery:\n%s", want, status)
		}
	}
}

// TestAutomaticRecoveryDoesNotBypassNeedsHuman proves the new scheduler path
// never recovers a NEEDS_HUMAN boundary as ordinary blocked work: a task parked at
// the human gate is not selected, not requeued, and not reported as recovered.
func TestAutomaticRecoveryDoesNotBypassNeedsHuman(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "T001", Status: domain.PLANNED, Attempt: 2, MaxAttempts: 3})
	seedRunStage(t, dir, "T001", "WAITING_FOR_HUMAN")
	a := &recoveryAgent{}

	code, stdout, stderr := runInjectedCLI(t, dir, "diff\n", a, "run")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}

	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	got, err := st.Get("T001")
	if err != nil {
		t.Fatalf("get T001: %v", err)
	}
	if got.Status == domain.BLOCKED {
		t.Errorf("T001 status = BLOCKED; automatic recovery must not manufacture a blocker")
	}
	if strings.Contains(stdout, "Recovering: T001") {
		t.Errorf("a human boundary must not be reported as automatic recovery:\n%s", stdout)
	}
}

// TestAutomaticRecoveryDoesNotBypassDependencies proves dependency ordering is
// preserved on the new path: a blocked task whose dependency is unsatisfied is
// not automatically recovered, and the scheduler stops rather than reaching later
// work. T001 is refused mid remote lifecycle (a local run opens no PR and runs no
// CI), so it never reaches a satisfied state and T002's dependency stays
// unsatisfied for the whole run.
func TestAutomaticRecoveryDoesNotBypassDependencies(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")
	// T002 is blocked and depends on T001, whose work is unfinished: it is parked
	// in the remote lifecycle, which a local run cannot complete.
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "T001", Status: domain.PR_OPEN, MaxAttempts: 3})
	seedTask(t, dir, &domain.Task{ID: "T002", Title: "T002", Status: domain.BLOCKED, BlockedReason: domain.REVIEW_UNRESOLVED, Attempt: 1, MaxAttempts: 3, DependencyIDs: []string{"T001"}})
	a := &recoveryAgent{}

	// The in-flight dependency occupies the execution slot, so no recovery is
	// attempted; the run stops without recovering T002 or starting later work.
	code, stdout, _ := runInjectedCLI(t, dir, "diff\n", a, "run")
	if code == exitOK {
		t.Fatalf("expected a non-zero exit; stdout=%s", stdout)
	}
	if strings.Contains(stdout, "Recovering: T002") {
		t.Errorf("a dependency-blocked task must not be auto-recovered:\n%s", stdout)
	}

	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	got, err := st.Get("T002")
	if err != nil {
		t.Fatalf("get T002: %v", err)
	}
	if got.Status != domain.BLOCKED {
		t.Errorf("T002 status = %s, want untouched BLOCKED", got.Status)
	}
}

// TestAutomaticRecoveryDoesNotFabricateSuccess proves a recovered task must still
// earn its completion: a task whose implementation fails is not advanced to a
// satisfied state, and the run stops at that task without executing later work.
func TestAutomaticRecoveryDoesNotFabricateSuccess(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "T001", Status: domain.BLOCKED, BlockedReason: domain.REVIEW_UNRESOLVED, Attempt: 1, MaxAttempts: 3})
	seedTask(t, dir, &domain.Task{ID: "T002", Title: "T002", Status: domain.BLOCKED, BlockedReason: domain.REVIEW_UNRESOLVED, Attempt: 1, MaxAttempts: 3})
	a := &recoveryAgent{failOn: "T001"}

	code, stdout, _ := runInjectedCLI(t, dir, "diff\n", a, "run")
	if code == exitOK {
		t.Fatalf("a failed recovery must not pass; stdout=%s", stdout)
	}

	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	got, err := st.Get("T001")
	if err != nil {
		t.Fatalf("get T001: %v", err)
	}
	if got.IsSatisfied() {
		t.Errorf("T001 = %s; recovery must not fabricate a satisfied state", got.Status)
	}
	// T002 must not be reached after T001's recovery failed.
	if strings.Contains(stdout, "Recovering: T002") || strings.Contains(stdout, "Running: T002") {
		t.Errorf("T002 must not be executed after T001's recovery failed:\n%s", stdout)
	}
	other, err := st.Get("T002")
	if err != nil {
		t.Fatalf("get T002: %v", err)
	}
	if other.Status != domain.BLOCKED {
		t.Errorf("T002 status = %s, want untouched BLOCKED", other.Status)
	}
}

// TestAutomaticRecoveryStopsAtFailedRecovery proves the failed-recovery stop
// reports the task and that later tasks were not executed, and that it performs
// no destructive cleanup of the state database or run history.
func TestAutomaticRecoveryStopsAtFailedRecovery(t *testing.T) {
	dir := newDirtyRepo(t)
	initProject(t, dir)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "T001", Status: domain.BLOCKED, BlockedReason: domain.REVIEW_UNRESOLVED, Attempt: 1, MaxAttempts: 3})
	seedRunStage(t, dir, "T001", "REVIEWING")
	a := &recoveryAgent{failOn: "T001"}

	code, stdout, stderr := runInjectedCLI(t, dir, "diff\n", a, "run")
	if code == exitOK {
		t.Fatalf("a failed recovery must stop the run; stdout=%s", stdout)
	}
	combined := stdout + stderr
	for _, want := range []string{"T001", "recovery", "No later task"} {
		if !strings.Contains(combined, want) {
			t.Errorf("failure diagnostics missing %q:\n%s", want, combined)
		}
	}

	// The stop performed no destructive cleanup: the state database and the prior
	// run history survive, and the pre-existing working-tree change is intact.
	assertRecoveryPreserved(t, dir, "T001")
	if got, err := os.ReadFile(filepath.Join(dir, "tracked.txt")); err != nil || string(got) != "user edit\n" {
		t.Errorf("tracked.txt = %q, %v; failed recovery reverted the user's edit", got, err)
	}
}
