package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/git"
	"github.com/imhttran/agentic-sop/internal/review"
)

// reviewGitRun runs one Git command in dir and fails the test on error.
func reviewGitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v (%s)", args, err, out)
	}
}

// reviewRepo creates an isolated temporary Git repository for the review-context
// tests. It never touches the module's own repository or any SOP state.
func reviewRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	reviewGitRun(t, dir, "init", "-q", "-b", "main")
	reviewGitRun(t, dir, "config", "user.email", "test@example.com")
	reviewGitRun(t, dir, "config", "user.name", "test")
	return dir
}

// reviewCommit writes path with content and commits it.
func reviewCommit(t *testing.T, dir, path, content, message string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	reviewGitRun(t, dir, "add", "--", path)
	reviewGitRun(t, dir, "commit", "-q", "-m", message)
}

// reviewWorkingTreeDiff returns the same working-tree diff runStages reads, so a
// test exercises the real "absent from this invocation's diff" condition.
func reviewWorkingTreeDiff(t *testing.T, dir string) string {
	t.Helper()
	diff, err := git.New(dir).DiffAll(context.Background(), ".agent-sdlc", "docs/reports")
	if err != nil {
		t.Fatalf("DiffAll: %v", err)
	}
	return diff
}

const reviewBarDiff = "diff --git a/internal/bar.go b/internal/bar.go\n" +
	"--- /dev/null\n+++ b/internal/bar.go\n@@ -0,0 +1 @@\n+package internal\n"

// TestReviewInputAddsCommittedContextForScopePathsAbsentFromDiff covers the mixed
// case: the invocation changed one scoped path, while another scoped path is
// already committed. The reviewer must receive both, with the committed content
// clearly labelled as not-this-invocation.
func TestReviewInputAddsCommittedContextForScopePathsAbsentFromDiff(t *testing.T) {
	dir := reviewRepo(t)
	reviewCommit(t, dir, "internal/foo.go", "package internal\n\nfunc Foo() {}\n", "add foo")

	scope := newReviewScope([]string{"internal/bar.go", "internal/foo.go"}, nil)
	input, err := reviewInput(context.Background(), dir, scope, reviewBarDiff, true, true)
	if err != nil {
		t.Fatalf("reviewInput: %v", err)
	}
	if !strings.Contains(input, "b/internal/bar.go") {
		t.Errorf("the invocation diff was dropped:\n%s", input)
	}
	if !strings.Contains(input, "EXISTING COMMITTED CONTENT") {
		t.Fatalf("the committed section was not added:\n%s", input)
	}
	if !strings.Contains(input, "--- path: internal/foo.go") || !strings.Contains(input, "func Foo() {}") {
		t.Errorf("the committed path or its content is missing:\n%s", input)
	}
	// The committed path must not be presented as an invocation change.
	if strings.Contains(input, "diff --git a/internal/foo.go") {
		t.Errorf("a committed path was presented as an invocation diff:\n%s", input)
	}
	// The invocation diff must precede the committed section.
	if strings.Index(input, "b/internal/bar.go") > strings.Index(input, "EXISTING COMMITTED CONTENT") {
		t.Errorf("committed context was placed before the invocation diff:\n%s", input)
	}
}

// TestReviewProviderReceivesCommittedContext proves the composed input is exactly
// what the review provider is handed.
func TestReviewProviderReceivesCommittedContext(t *testing.T) {
	dir := reviewRepo(t)
	reviewCommit(t, dir, "internal/foo.go", "package internal\n\nfunc Foo() {}\n", "add foo")

	scope := newReviewScope([]string{"internal/bar.go", "internal/foo.go"}, nil)
	input, err := reviewInput(context.Background(), dir, scope, reviewBarDiff, true, true)
	if err != nil {
		t.Fatalf("reviewInput: %v", err)
	}
	p := &recordingProvider{}
	if _, err := p.Review(context.Background(), review.Request{Diff: input}); err != nil {
		t.Fatalf("provider.Review: %v", err)
	}
	if !p.called || !strings.Contains(p.diff, "EXISTING COMMITTED CONTENT") {
		t.Fatalf("provider did not receive the committed context: %+v", p.diff)
	}
}

// TestReviewExistingContextIgnoresOutOfScopeCommittedPaths proves scope isolation:
// a committed path that is not in the task's scope never reaches review.
func TestReviewExistingContextIgnoresOutOfScopeCommittedPaths(t *testing.T) {
	dir := reviewRepo(t)
	reviewCommit(t, dir, "internal/unrelated.go", "package internal\n\nconst Unrelated = 1\n", "add unrelated")

	scope := newReviewScope([]string{"internal/bar.go"}, nil)
	section := reviewExistingContext(context.Background(), dir, scope, reviewBarDiff)
	if section != "" {
		t.Errorf("an out-of-scope committed path produced context:\n%s", section)
	}
}

// TestReviewExistingContextExcludesStagedAndUnstagedContent proves a staged-only
// modification is never presented as committed content, and an unstaged
// modification is treated as this invocation's own change instead.
func TestReviewExistingContextExcludesStagedAndUnstagedContent(t *testing.T) {
	dir := reviewRepo(t)
	reviewCommit(t, dir, "internal/foo.go", "package internal\n\nconst Committed = 1\n", "add foo")
	// A second committed scoped path keeps the committed section non-empty, so the
	// negative assertions below are meaningful rather than trivially true.
	reviewCommit(t, dir, "internal/keep.go", "package internal\n\nconst Keep = 9\n", "add keep")
	// An unrelated working-tree change makes the review branch run at all.
	if err := os.WriteFile(filepath.Join(dir, "internal", "bar.go"), []byte("package internal\n\nconst Bar = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	diff := reviewWorkingTreeDiff(t, dir)

	// Stage-only edit of the committed path: `git diff` does not name it, so it is a
	// candidate for committed context, and HEAD content must be shown, not the edit.
	if err := os.WriteFile(filepath.Join(dir, "internal", "foo.go"), []byte("package internal\n\nconst Staged = 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reviewGitRun(t, dir, "add", "--", "internal/foo.go")

	scope := newReviewScope([]string{"internal/bar.go", "internal/foo.go", "internal/keep.go"}, nil)
	section := reviewExistingContext(context.Background(), dir, scope, diff)
	if !strings.Contains(section, "Committed = 1") || !strings.Contains(section, "Keep = 9") {
		t.Fatalf("committed content missing:\n%s", section)
	}
	if strings.Contains(section, "Staged = 2") {
		t.Errorf("staged content was presented as committed content:\n%s", section)
	}

	// Unstaged edit: the working-tree diff names the path, so it is this
	// invocation's own change and must not be repeated as committed content.
	reviewGitRun(t, dir, "reset", "-q")
	if err := os.WriteFile(filepath.Join(dir, "internal", "foo.go"), []byte("package internal\n\nconst Unstaged = 3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	diff = reviewWorkingTreeDiff(t, dir)
	section = reviewExistingContext(context.Background(), dir, scope, diff)
	if !strings.Contains(section, "Keep = 9") {
		t.Fatalf("the committed section was lost for the unchanged path:\n%s", section)
	}
	if strings.Contains(section, "internal/foo.go") || strings.Contains(section, "Unstaged = 3") {
		t.Errorf("an unstaged change was presented as committed content:\n%s", section)
	}
}

// TestCommittedReviewPathExclusions is the exclusion table: sensitive, unsafe,
// SOP-owned, generated-report, and Git-metadata paths are all refused, while a
// normal source path and an exactly declared report deliverable are included.
func TestCommittedReviewPathExclusions(t *testing.T) {
	cases := []struct {
		path    string
		reports []string
		want    bool
	}{
		{"internal/a.go", nil, true},
		{"internal/deep/a.go", nil, true},
		{"docs/reports/TASK.md", []string{"docs/reports/TASK.md"}, true},
		{"docs/reports/GENERATED.md", nil, false},
		{".env", nil, false},
		{".env.local", nil, false},
		{"config/.env", nil, false},
		{".aws/credentials", nil, false},
		{".ssh/id_rsa", nil, false},
		{".kube/config", nil, false},
		{"deploy/service.key", nil, false},
		{"certs/ca.pem", nil, false},
		{"internal/secrets/manager.go", nil, false},
		{"secrets.yaml", nil, false},
		{"internal/credentials_helper.go", nil, false},
		{".git/config", nil, false},
		{".agent-sdlc/state.db", nil, false},
		{"../escape.go", nil, false},
		{"/etc/passwd", nil, false},
		{"", nil, false},
	}
	for _, tc := range cases {
		if got := includeCommittedReviewPath(tc.path, tc.reports...); got != tc.want {
			t.Errorf("includeCommittedReviewPath(%q, %v) = %v, want %v", tc.path, tc.reports, got, tc.want)
		}
	}
}

// TestReviewExistingContextBudgets proves the file, per-file, and total byte
// budgets are enforced deterministically and that every cutoff is stated
// explicitly without including the omitted content.
func TestReviewExistingContextBudgets(t *testing.T) {
	dir := reviewRepo(t)
	var paths []string
	for i := 0; i < maxCommittedReviewFiles+1; i++ {
		path := fmt.Sprintf("internal/f%02d.go", i)
		reviewCommit(t, dir, path, "package internal\n", "add "+path)
		paths = append(paths, path)
	}
	scope := newReviewScope(paths, nil)
	section := reviewExistingContext(context.Background(), dir, scope, reviewBarDiff)
	if got := strings.Count(section, "--- path: "); got != maxCommittedReviewFiles {
		t.Errorf("included %d path(s), want %d", got, maxCommittedReviewFiles)
	}
	if !strings.Contains(section, "Omitted for budget (1 path(s)") {
		t.Errorf("the file-budget cutoff was not stated:\n%s", section)
	}
	if again := reviewExistingContext(context.Background(), dir, scope, reviewBarDiff); again != section {
		t.Error("the committed section is not deterministic")
	}

	// Per-file budget: an oversized committed file is omitted whole, never
	// partially included.
	bigDir := reviewRepo(t)
	big := strings.Repeat("a", maxCommittedReviewFileBytes+1024)
	reviewCommit(t, bigDir, "internal/big.go", big, "big")
	bigScope := newReviewScope([]string{"internal/big.go"}, nil)
	if section := reviewExistingContext(context.Background(), bigDir, bigScope, reviewBarDiff); strings.Contains(section, "aaaa") {
		t.Errorf("an oversized file was partially included:\n%s", section)
	}

	// Total budget: fifteen-kilobyte files fill the total before the fifth.
	totalDir := reviewRepo(t)
	var totalPaths []string
	chunk := strings.Repeat("b", 15<<10)
	for i := 0; i < 5; i++ {
		path := fmt.Sprintf("internal/t%02d.txt", i+1)
		reviewCommit(t, totalDir, path, chunk, "add "+path)
		totalPaths = append(totalPaths, path)
	}
	totalScope := newReviewScope(totalPaths, nil)
	totalSection := reviewExistingContext(context.Background(), totalDir, totalScope, reviewBarDiff)
	if got := strings.Count(totalSection, "--- path: "); got != 4 {
		t.Errorf("included %d path(s), want 4 before the total budget closes", got)
	}
	if !strings.Contains(totalSection, "Omitted for budget (1 path(s)") {
		t.Errorf("the total-budget cutoff was not stated:\n%s", totalSection)
	}
}

// TestReviewIdentityChangesWithCommittedContext proves appended committed content
// changes the review cache identity, so a cached report can never be reused for a
// different review input.
func TestReviewIdentityChangesWithCommittedContext(t *testing.T) {
	section := "EXISTING COMMITTED CONTENT"
	if reviewIdentity("self", "task", "diff") == reviewIdentity("self", "task", "diff+"+section) {
		t.Fatal("committed content must change the review identity")
	}
}

// TestReviewInputDoesNotAddCommittedContextForNoChangeCompletion proves a
// completion the invocation does not own keeps the existing review input: a
// verified no-change completion must not suddenly be reviewed against committed
// content.
func TestReviewInputDoesNotAddCommittedContextForNoChangeCompletion(t *testing.T) {
	dir := reviewRepo(t)
	reviewCommit(t, dir, "internal/foo.go", "package internal\n\nfunc Foo() {}\n", "add foo")

	scope := newReviewScope([]string{"internal/foo.go"}, nil)
	got, err := reviewInput(context.Background(), dir, scope, "", false, false)
	if err != nil {
		t.Fatalf("reviewInput: %v", err)
	}
	if got != "" {
		t.Errorf("input = %q, want the unchanged empty review input", got)
	}
}

// TestReviewInputUnchangedWithoutCommittedContext proves there is no behaviour
// change when the section adds nothing: an empty scope performs no Git access at
// all (the directory is not a repository), and a fully-scoped diff is returned
// byte-identical.
func TestReviewInputUnchangedWithoutCommittedContext(t *testing.T) {
	input, err := reviewInput(context.Background(), t.TempDir(), reviewScope{}, "", false, false)
	if err != nil {
		t.Fatalf("reviewInput with an empty scope: %v", err)
	}
	if input != "" {
		t.Errorf("input = %q, want an empty review input", input)
	}

	dir := reviewRepo(t)
	diff := "diff --git a/internal/a.go b/internal/a.go\n" +
		"--- a/internal/a.go\n+++ b/internal/a.go\n@@ -1 +1 @@\n-a\n+b\n"
	scope := newReviewScope([]string{"internal/a.go"}, nil)
	got, err := reviewInput(context.Background(), dir, scope, diff, true, true)
	if err != nil {
		t.Fatalf("reviewInput: %v", err)
	}
	if want := strings.TrimRight(diff, "\n"); got != want {
		t.Errorf("review input changed without committed context:\n got: %q\nwant: %q", got, want)
	}
}

// TestReviewInputPreservesScopeFailClosed proves the committed-content addition
// cannot mask the existing fail-closed seam: an unattributable dirty tree with a
// required change still returns ErrReviewScopeUnestablished and no review input.
func TestReviewInputPreservesScopeFailClosed(t *testing.T) {
	dir := reviewRepo(t)
	dirty := "diff --git a/x.go b/x.go\n--- /dev/null\n+++ b/x.go\n@@ -0,0 +1 @@\n+x\n"
	input, err := reviewInput(context.Background(), dir, reviewScope{}, dirty, true, true)
	if !errors.Is(err, ErrReviewScopeUnestablished) {
		t.Fatalf("err = %v, want ErrReviewScopeUnestablished", err)
	}
	if input != "" {
		t.Errorf("input = %q, want no review input on a fail-closed scope", input)
	}
}
