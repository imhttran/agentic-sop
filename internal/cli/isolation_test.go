package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRequirePlanIsolation(t *testing.T) {
	repo := t.TempDir()
	git := func(dir string, args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git(repo, "init", "-q", "-b", "main")
	git(repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "init")

	// New plan on main in the primary checkout: refused with the worktree command.
	err := requirePlanIsolation(repo, "docs/plans/PLAN-Cache.md")
	if err == nil || !strings.Contains(err.Error(), "git worktree add -b sop/plan-cache ") {
		t.Fatalf("main primary checkout: got %v, want refusal naming the worktree command", err)
	}

	// Existing SOP state means a resume: allowed in place.
	if err := os.MkdirAll(filepath.Dir(statePath(repo)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath(repo), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := requirePlanIsolation(repo, ""); err != nil {
		t.Fatalf("resume on main: got %v, want nil", err)
	}
	if err := os.Remove(statePath(repo)); err != nil {
		t.Fatal(err)
	}

	// Another branch in the primary checkout, and main in a linked worktree: allowed.
	git(repo, "switch", "-q", "-c", "feature")
	if err := requirePlanIsolation(repo, ""); err != nil {
		t.Fatalf("feature branch: got %v, want nil", err)
	}
	wt := filepath.Join(t.TempDir(), "wt")
	git(repo, "worktree", "add", "-q", wt, "main")
	if err := requirePlanIsolation(wt, ""); err != nil {
		t.Fatalf("linked worktree on main: got %v, want nil", err)
	}
}
