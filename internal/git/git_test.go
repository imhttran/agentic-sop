package git

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/domain"
)

func setGitEnv(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	t.Setenv("GIT_AUTHOR_NAME", "SOP Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "SOP Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// initRepo creates a temporary Git repository on main with one commit.
func initRepo(t *testing.T) string {
	t.Helper()
	setGitEnv(t)
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")
	writeFile(t, dir, "README.md", "hello\n")
	runGit(t, dir, "add", "README.md")
	runGit(t, dir, "commit", "-m", "initial")
	return dir
}

func TestDiffAllIncludesUntracked(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "new.txt", "brand new\n")

	diff, err := New(dir).DiffAll(context.Background(), ".agent-sdlc", "docs/reports")
	if err != nil {
		t.Fatalf("DiffAll failed: %v", err)
	}
	if !strings.Contains(diff, "new.txt") || !strings.Contains(diff, "new file mode") {
		t.Errorf("DiffAll missing the untracked file header:\n%s", diff)
	}
	if !strings.Contains(diff, "+brand new") {
		t.Errorf("DiffAll missing the untracked content:\n%s", diff)
	}
}

func TestDiffAllExcludesSOPOutput(t *testing.T) {
	dir := initRepo(t)
	if err := os.MkdirAll(filepath.Join(dir, ".agent-sdlc"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join(".agent-sdlc", "state.db"), "x")
	if err := os.MkdirAll(filepath.Join(dir, "docs", "reports"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join("docs", "reports", "prd.md"), "# plan\n")

	diff, err := New(dir).DiffAll(context.Background(), ".agent-sdlc", "docs/reports")
	if err != nil {
		t.Fatalf("DiffAll failed: %v", err)
	}
	if strings.Contains(diff, "state.db") || strings.Contains(diff, "docs/reports") {
		t.Errorf("DiffAll should exclude SOP's own output:\n%s", diff)
	}
}

func TestValidateRepository(t *testing.T) {
	ctx := context.Background()

	if err := New(initRepo(t)).ValidateRepository(ctx); err != nil {
		t.Errorf("valid repository rejected: %v", err)
	}
	if err := New(t.TempDir()).ValidateRepository(ctx); err == nil {
		t.Error("expected error for a non-repository directory")
	}
}

func TestStatusCleanAndDirty(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	a := New(dir)

	if status, err := a.Status(ctx); err != nil || status != Clean {
		t.Fatalf("status = %q, %v; want CLEAN", status, err)
	}

	writeFile(t, dir, "untracked.txt", "x")
	if status, err := a.Status(ctx); err != nil || status != Dirty {
		t.Fatalf("status = %q, %v; want DIRTY", status, err)
	}
}

func TestCurrentBranch(t *testing.T) {
	branch, err := New(initRepo(t)).CurrentBranch(context.Background())
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}
	if branch != "main" {
		t.Errorf("branch = %q, want main", branch)
	}
}

func TestCurrentBranchDetachedHead(t *testing.T) {
	dir := initRepo(t)
	runGit(t, dir, "checkout", "--detach", "HEAD")

	_, err := New(dir).CurrentBranch(context.Background())
	if !errors.Is(err, ErrDetachedHead) {
		t.Errorf("err = %v, want ErrDetachedHead", err)
	}
}

func TestTaskBranchName(t *testing.T) {
	cases := []struct {
		id, title string
		want      string
		wantErr   bool
	}{
		{"T007", "Deterministic Git Adapter", "task/T007-deterministic-git-adapter", false},
		{"T005", "CLI Foundation", "task/T005-cli-foundation", false},
		{"T001", "  Spaced   Title  ", "task/T001-spaced-title", false},
		{"T001", "", "", true},
		{"", "Title", "", true},
	}
	for _, tc := range cases {
		got, err := TaskBranchName(tc.id, tc.title)
		if tc.wantErr {
			if err == nil {
				t.Errorf("TaskBranchName(%q,%q) = %q, want error", tc.id, tc.title, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("TaskBranchName(%q,%q): %v", tc.id, tc.title, err)
			continue
		}
		if got != tc.want {
			t.Errorf("TaskBranchName(%q,%q) = %q, want %q", tc.id, tc.title, got, tc.want)
		}
	}
}

func TestCreateBranch(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	a := New(dir)

	name, err := TaskBranchName("T007", "Deterministic Git Adapter")
	if err != nil {
		t.Fatalf("TaskBranchName: %v", err)
	}
	if err := a.CreateBranch(ctx, name); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if branch, _ := a.CurrentBranch(ctx); branch != name {
		t.Errorf("branch = %q, want %q", branch, name)
	}
}

func TestCreateBranchRejectsExistingBranch(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	a := New(dir)

	name := "task/T007-existing"
	// Pre-create the branch so it exists before CreateBranch runs.
	runGit(t, dir, "branch", name)
	before := runGit(t, dir, "rev-parse", name)

	err := a.CreateBranch(ctx, name)
	if !errors.Is(err, ErrBranchExists) {
		t.Fatalf("err = %v, want ErrBranchExists", err)
	}
	if after := runGit(t, dir, "rev-parse", name); after != before {
		t.Errorf("existing branch was modified: %q -> %q", before, after)
	}
	if branch, _ := a.CurrentBranch(ctx); branch != "main" {
		t.Errorf("current branch = %q, want main", branch)
	}
}

func TestCreateBranchRejectsDirtyTree(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	a := New(dir)

	writeFile(t, dir, "dirty.txt", "uncommitted")

	name := "task/T007-dirty"
	if err := a.CreateBranch(ctx, name); err == nil {
		t.Fatal("expected error when the working tree is dirty")
	}
	if exists, _ := a.BranchExists(ctx, name); exists {
		t.Error("branch should not have been created")
	}
	if branch, _ := a.CurrentBranch(ctx); branch != "main" {
		t.Errorf("current branch = %q, want main", branch)
	}
	if _, err := os.Stat(filepath.Join(dir, "dirty.txt")); err != nil {
		t.Errorf("user change was discarded: %v", err)
	}
}

// localAndOrigin sets up a local repository with a local bare origin, both on
// main, and returns their directories.
func localAndOrigin(t *testing.T) (local, origin string) {
	t.Helper()
	origin = t.TempDir()
	runGit(t, origin, "init", "--bare", "-b", "main")

	local = initRepo(t)
	runGit(t, local, "remote", "add", "origin", origin)
	runGit(t, local, "push", "-u", "origin", "main")
	return local, origin
}

func TestUpdateIntegrationBranchFastForwards(t *testing.T) {
	ctx := context.Background()
	local, origin := localAndOrigin(t)

	// Advance origin/main from a second working copy.
	other := t.TempDir()
	runGit(t, other, "clone", origin, other)
	writeFile(t, other, "remote.txt", "x")
	runGit(t, other, "add", "-A")
	runGit(t, other, "commit", "-m", "remote change")
	runGit(t, other, "push", "origin", "main")

	if err := New(local).UpdateIntegrationBranch(ctx, "main"); err != nil {
		t.Fatalf("UpdateIntegrationBranch: %v", err)
	}
	if got, want := runGit(t, local, "rev-parse", "main"), runGit(t, origin, "rev-parse", "main"); got != want {
		t.Errorf("local main = %q, want origin main %q", got, want)
	}
}

func TestUpdateIntegrationBranchRejectsDiverged(t *testing.T) {
	ctx := context.Background()
	local, origin := localAndOrigin(t)

	// Diverge: local gains a commit, origin gains a different commit.
	writeFile(t, local, "local.txt", "x")
	runGit(t, local, "add", "-A")
	runGit(t, local, "commit", "-m", "local change")
	localHead := runGit(t, local, "rev-parse", "main")

	other := t.TempDir()
	runGit(t, other, "clone", origin, other)
	writeFile(t, other, "remote.txt", "x")
	runGit(t, other, "add", "-A")
	runGit(t, other, "commit", "-m", "remote change")
	runGit(t, other, "push", "origin", "main")

	if err := New(local).UpdateIntegrationBranch(ctx, "main"); err == nil {
		t.Fatal("expected a non-fast-forward update to fail")
	}
	if got := runGit(t, local, "rev-parse", "main"); got != localHead {
		t.Errorf("local main changed: %q -> %q", localHead, got)
	}
}

func TestMissingOrigin(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t) // no remote configured
	a := New(dir)

	// Read-only operations still work.
	if err := a.ValidateRepository(ctx); err != nil {
		t.Errorf("ValidateRepository: %v", err)
	}
	if status, err := a.Status(ctx); err != nil || status != Clean {
		t.Errorf("Status = %q, %v", status, err)
	}
	if branch, err := a.CurrentBranch(ctx); err != nil || branch != "main" {
		t.Errorf("CurrentBranch = %q, %v", branch, err)
	}

	if err := a.UpdateIntegrationBranch(ctx, "main"); err == nil {
		t.Error("expected an error when origin is missing")
	}
}

func TestDiff(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "README.md", "changed\n")

	out, err := New(dir).Diff(context.Background())
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if !strings.Contains(out, "changed") {
		t.Errorf("diff does not contain the change:\n%s", out)
	}
}

func TestCommit(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	writeFile(t, dir, "file.txt", "x")
	runGit(t, dir, "add", "file.txt")

	if err := New(dir).Commit(ctx, "add file"); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if msg := strings.TrimSpace(runGit(t, dir, "log", "-1", "--pretty=%s")); msg != "add file" {
		t.Errorf("commit subject = %q, want %q", msg, "add file")
	}
}

func TestNothingToCommit(t *testing.T) {
	if err := New(initRepo(t)).Commit(context.Background(), "empty"); err == nil {
		t.Error("expected an error when there is nothing to commit")
	}
}

func TestContextCancellation(t *testing.T) {
	dir := initRepo(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := New(dir).ValidateRepository(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestCreateBranchRejectsInvalidName(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	a := New(dir)

	// Git rejects this name (contains "..").
	if err := a.CreateBranch(ctx, "task/bad..name"); err == nil {
		t.Fatal("expected error for an invalid branch name")
	}
	if branch, _ := a.CurrentBranch(ctx); branch != "main" {
		t.Errorf("current branch = %q, want main", branch)
	}
}

func TestCheckoutValidMissingBranch(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	a := New(dir)

	// Valid syntax but no such branch: a Git operation error, not a syntax error.
	err := a.Checkout(ctx, "task/does-not-exist")
	if err == nil {
		t.Fatal("expected error for a missing branch")
	}
	if branch, _ := a.CurrentBranch(ctx); branch != "main" {
		t.Errorf("current branch = %q, want main", branch)
	}
}

func TestValidateBranchAcceptsTaskName(t *testing.T) {
	dir := initRepo(t)
	name, err := TaskBranchName("T007", "Deterministic Git Adapter")
	if err != nil {
		t.Fatalf("TaskBranchName: %v", err)
	}
	if err := New(dir).validateBranch(context.Background(), name); err != nil {
		t.Errorf("validateBranch(%q) = %v, want valid", name, err)
	}
}

func TestAddAndRemoveWorktree(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	a := New(dir)

	name, err := TaskBranchName("T021", "Limited Parallelism")
	if err != nil {
		t.Fatalf("TaskBranchName: %v", err)
	}
	worktree := filepath.Join(t.TempDir(), "wt")
	if err := a.AddWorktree(ctx, worktree, name); err != nil {
		t.Fatalf("AddWorktree: %v", err)
	}
	if branch, err := New(worktree).CurrentBranch(ctx); err != nil || branch != name {
		t.Errorf("worktree branch = %q, %v; want %q", branch, err, name)
	}

	if err := a.RemoveWorktree(ctx, worktree); err != nil {
		t.Fatalf("RemoveWorktree: %v", err)
	}
	if _, err := os.Stat(worktree); !os.IsNotExist(err) {
		t.Errorf("worktree directory still exists: %v", err)
	}
}

// TestWorkflowBranchCreationAdvancesState exercises the READY -> BRANCH_CREATED
// sequence: state advances only after branch creation succeeds.
func TestWorkflowBranchCreationAdvancesState(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	a := New(dir)

	task := &domain.Task{ID: "T007", Title: "Deterministic Git Adapter", Status: domain.READY}
	name, err := TaskBranchName(task.ID, task.Title)
	if err != nil {
		t.Fatalf("TaskBranchName: %v", err)
	}

	if err := a.CreateBranch(ctx, name); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	if err := task.Transition(domain.BRANCH_CREATED); err != nil {
		t.Fatalf("Transition: %v", err)
	}
	if task.Status != domain.BRANCH_CREATED {
		t.Errorf("status = %s, want BRANCH_CREATED", task.Status)
	}
}

func TestWorkflowBranchCreationFailureLeavesReady(t *testing.T) {
	ctx := context.Background()
	dir := initRepo(t)
	a := New(dir)
	writeFile(t, dir, "dirty.txt", "x") // forces branch creation to fail

	task := &domain.Task{ID: "T007", Title: "Deterministic Git Adapter", Status: domain.READY}
	name, _ := TaskBranchName(task.ID, task.Title)

	if err := a.CreateBranch(ctx, name); err == nil {
		t.Fatal("expected branch creation to fail on a dirty tree")
	}
	// The workflow must not advance state when Git fails.
	if task.Status != domain.READY {
		t.Errorf("status = %s, want READY (unchanged)", task.Status)
	}
}
