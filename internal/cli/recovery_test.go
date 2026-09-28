package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/resume"
)

// newDirtyRepo initializes a Git repository with one commit and then leaves a
// pre-existing user change in the working tree: a modified tracked file and an
// untracked file. A recovery path that ran `git reset --hard` would revert the
// tracked edit, and one that ran `git clean` would delete the untracked file, so
// both are canaries for destructive recovery.
func newDirtyRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	env := append(os.Environ(),
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_CONFIG_SYSTEM="+os.DevNull,
		"GIT_AUTHOR_NAME=SOP Test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=SOP Test", "GIT_COMMITTER_EMAIL=test@example.com",
	)
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}

	git("init", "-q", "-b", "main")
	writeFile(t, dir, "tracked.txt", "committed\n")
	git("add", "tracked.txt")
	git("commit", "-q", "-m", "initial")

	// Pre-existing user work that recovery must not touch.
	writeFile(t, dir, "tracked.txt", "user edit\n")
	writeFile(t, dir, "untracked.txt", "scratch\n")
	return dir
}

// assertRecoveryPreserved checks that recovery left the pre-existing working-tree
// change, the state database, and the run history intact.
func assertRecoveryPreserved(t *testing.T, dir, id string) {
	t.Helper()
	if got, err := os.ReadFile(filepath.Join(dir, "tracked.txt")); err != nil || string(got) != "user edit\n" {
		t.Errorf("tracked.txt = %q, %v; recovery reverted the user's edit", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "untracked.txt")); err != nil || string(got) != "scratch\n" {
		t.Errorf("untracked.txt = %q, %v; recovery deleted the user's untracked file", got, err)
	}
	if !stateExists(statePath(dir)) {
		t.Error("recovery deleted the state database")
	}
	if !stateExists(filepath.Join(dir, stateDirName, "runs", id, "state.json")) {
		t.Error("recovery deleted the run history")
	}
}

// TestRetryPreservesWorkingTreeAndRunHistory proves retry is non-destructive
// recovery: it requeues the task while preserving a pre-existing user change, the
// state database, and prior run history.
func TestRetryPreservesWorkingTreeAndRunHistory(t *testing.T) {
	dir := newDirtyRepo(t)
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "x", Status: domain.BLOCKED, BlockedReason: domain.REVIEW_UNRESOLVED, MaxAttempts: 3})
	seedRunStage(t, dir, "T001", "REVIEWING")

	code, stdout, stderr := runCLI(t, dir, "retry", "T001")
	if code != exitOK {
		t.Fatalf("retry: code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "requeued T001") {
		t.Errorf("stdout = %q, want the task requeued", stdout)
	}
	assertRecoveryPreserved(t, dir, "T001")
}

// TestResumePreservesWorkingTreeAndRunHistory proves resume is non-destructive
// recovery: it persists the recovered status while preserving a pre-existing user
// change, the state database, and prior run history.
func TestResumePreservesWorkingTreeAndRunHistory(t *testing.T) {
	dir := newDirtyRepo(t)
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "x", Status: domain.READY, MaxAttempts: 1})
	seedRunStage(t, dir, "T001", "IMPLEMENTING")

	res := &fakeResources{obs: resume.Observation{BranchExists: true}}
	code, stdout, stderr := runCLIWithResources(t, dir, res, "resume", "T001")
	if code != exitOK {
		t.Fatalf("resume: code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "recovered=BRANCH_CREATED") {
		t.Errorf("stdout = %q, want the recovery persisted", stdout)
	}
	assertRecoveryPreserved(t, dir, "T001")
}

// TestActiveTaskRecoveryPreservesWorkingTreeAndRunHistory proves the interrupted
// active-task recovery in `sop run` preserves a pre-existing user change, the
// state database, and prior run history while resuming the interrupted task.
func TestActiveTaskRecoveryPreservesWorkingTreeAndRunHistory(t *testing.T) {
	dir := newDirtyRepo(t)
	initProject(t, dir)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")
	seedTask(t, dir, &domain.Task{ID: "AHV2002", Title: "Introduce Harness Abstraction", Status: domain.READY, MaxAttempts: 3})
	seedRunStage(t, dir, "AHV2002", "PLANNING")
	a := &fakeCapabilityAgent{plan: validPlanJSON, impl: "x", review: `{"summary":"clean","findings":[]}`}

	code, stdout, stderr := runInjectedCLI(t, dir, "diff\n", a, "run")
	if code != exitOK {
		t.Fatalf("run: code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "Running: AHV2002") {
		t.Errorf("stdout = %q, want the interrupted task resumed", stdout)
	}
	assertRecoveryPreserved(t, dir, "AHV2002")
}
