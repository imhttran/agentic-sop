package cli

import (
	"errors"
	"strings"
	"testing"
)

// A tracked change made during the task is in scope: its path is in the task's
// change set and its diff chunk survives the filter.
func TestReviewScopeIncludesTrackedChange(t *testing.T) {
	diff := "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1 +1 @@\n-old\n+new\n"
	scope := newReviewScope([]string{"a.go"}, nil)
	got, err := scopedReviewDiff(scope, diff, true)
	if err != nil {
		t.Fatalf("scopedReviewDiff: %v", err)
	}
	if !strings.Contains(got, "+++ b/a.go") {
		t.Errorf("tracked change omitted from scope:\n%s", got)
	}
}

// An unrelated pre-existing working-tree modification is excluded: the diff
// names it, but it is not in the task's change set.
func TestReviewScopeExcludesUnrelatedChange(t *testing.T) {
	diff := "diff --git a/ours.go b/ours.go\n--- a/ours.go\n+++ b/ours.go\n@@ -1 +1 @@\n-old\n+new\n" +
		"diff --git a/unrelated.go b/unrelated.go\n--- a/unrelated.go\n+++ b/unrelated.go\n@@ -1 +1 @@\n-old\n+new\n"
	scope := newReviewScope([]string{"ours.go"}, nil)
	got, err := scopedReviewDiff(scope, diff, true)
	if err != nil {
		t.Fatalf("scopedReviewDiff: %v", err)
	}
	if !strings.Contains(got, "+++ b/ours.go") {
		t.Errorf("task-owned change omitted:\n%s", got)
	}
	if strings.Contains(got, "unrelated.go") {
		t.Errorf("unrelated pre-existing modification leaked into review:\n%s", got)
	}
}

// A task-owned untracked deliverable (a synthetic new-file chunk in the diff)
// is included when it is in the scope.
func TestReviewScopeIncludesTaskOwnedUntracked(t *testing.T) {
	diff := "diff --git a/new.md b/new.md\nnew file mode 100644\n--- /dev/null\n+++ b/new.md\n@@ -0,0 +1,1 @@\n+content\n"
	scope := newReviewScope([]string{"new.md"}, nil)
	got, err := scopedReviewDiff(scope, diff, true)
	if err != nil {
		t.Fatalf("scopedReviewDiff: %v", err)
	}
	if !strings.Contains(got, "+++ b/new.md") {
		t.Errorf("task-owned untracked deliverable omitted:\n%s", got)
	}
}

// A declared deliverable is included even when it is not otherwise present in
// the raw change listing.
func TestReviewScopeIncludesDeclaredDeliverable(t *testing.T) {
	diff := "diff --git a/report.md b/report.md\nnew file mode 100644\n--- /dev/null\n+++ b/report.md\n@@ -0,0 +1,1 @@\n+r\n"
	scope := newReviewScope(nil, []string{"report.md"})
	got, err := scopedReviewDiff(scope, diff, true)
	if err != nil {
		t.Fatalf("scopedReviewDiff: %v", err)
	}
	if !strings.Contains(got, "+++ b/report.md") {
		t.Errorf("declared deliverable omitted:\n%s", got)
	}
}

// A deleted file is in scope: the diff names its real (pre-deletion) side and a
// /dev/null side, and the chunk is retained.
func TestReviewScopeIncludesDeletedFile(t *testing.T) {
	diff := "diff --git a/gone.go b/gone.go\ndeleted file mode 100644\n--- a/gone.go\n+++ /dev/null\n@@ -1 +0,0 @@\n-gone\n"
	scope := newReviewScope([]string{"gone.go"}, nil)
	got, err := scopedReviewDiff(scope, diff, true)
	if err != nil {
		t.Fatalf("scopedReviewDiff: %v", err)
	}
	if !strings.Contains(got, "--- a/gone.go") {
		t.Errorf("deleted file omitted:\n%s", got)
	}
}

// A renamed file is in scope: both its sides are recognized and retained when
// recorded in the task's change set.
func TestReviewScopeIncludesRenamedFile(t *testing.T) {
	diff := "diff --git a/old.go b/new.go\nsimilarity index 100%\nrename from old.go\nrename to new.go\n"
	scope := newReviewScope([]string{"old.go", "new.go"}, nil)
	got, err := scopedReviewDiff(scope, diff, true)
	if err != nil {
		t.Fatalf("scopedReviewDiff: %v", err)
	}
	if !strings.Contains(got, "rename to new.go") {
		t.Errorf("renamed file omitted:\n%s", got)
	}
}

// A dirty tree that names a real repository file, none of which is attributable
// to a task that required a change, fails closed rather than reviewing the whole
// tree.
func TestReviewScopeFailsClosedWhenUnattributable(t *testing.T) {
	diff := "diff --git a/someone-elses.go b/someone-elses.go\n--- a/someone-elses.go\n+++ b/someone-elses.go\n@@ -1 +1 @@\n-old\n+new\n"
	scope := newReviewScope(nil, nil)
	_, err := scopedReviewDiff(scope, diff, true)
	if !errors.Is(err, ErrReviewScopeUnestablished) {
		t.Fatalf("err = %v, want ErrReviewScopeUnestablished", err)
	}
}

// A legitimate no-change completion (the task did not require a change and
// produced no attributable change) yields an empty review input rather than
// failing closed or reviewing the whole tree.
func TestReviewScopeNoChangeCompletionIsEmpty(t *testing.T) {
	diff := "diff --git a/someone-elses.go b/someone-elses.go\n--- a/someone-elses.go\n+++ b/someone-elses.go\n@@ -1 +1 @@\n-old\n+new\n"
	scope := newReviewScope(nil, nil)
	got, err := scopedReviewDiff(scope, diff, false)
	if err != nil {
		t.Fatalf("scopedReviewDiff: %v", err)
	}
	if got != "" {
		t.Errorf("no-change completion = %q, want empty review input", got)
	}
}

// An opaque diff that names no repository file cannot leak an unrelated
// modification and passes through unchanged.
func TestReviewScopePassesThroughOpaqueDiff(t *testing.T) {
	scope := newReviewScope(nil, nil)
	got, err := scopedReviewDiff(scope, "diff --git a/x b/x\n", true)
	if err != nil {
		t.Fatalf("scopedReviewDiff: %v", err)
	}
	if got != "diff --git a/x b/x\n" {
		t.Errorf("opaque diff = %q, want it unchanged", got)
	}
}

// SOP-owned state and generated output are never part of the review scope.
func TestReviewScopeDropsSOPPaths(t *testing.T) {
	scope := newReviewScope([]string{stateDirName + "/config.yaml", "ours.go"}, nil)
	if scope.contains(stateDirName + "/config.yaml") {
		t.Errorf("SOP-owned path must not be in scope")
	}
	if !scope.contains("ours.go") {
		t.Errorf("task-owned path must be in scope")
	}
}
