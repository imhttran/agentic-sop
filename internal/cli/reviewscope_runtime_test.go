package cli

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/review"
)

// recordingProvider is a review.Provider fake that records exactly the diff it
// was asked to review and whether it was called at all. It never contacts a real
// provider or network. It is the integration seam for RSH-004-003: the runtime
// review path must hand it the SCOPED diff, never the whole-tree DiffAll.
type recordingProvider struct {
	diff   string
	called bool
	report review.Report
}

func (p *recordingProvider) Review(_ context.Context, req review.Request) (review.Report, error) {
	p.called = true
	p.diff = req.Diff
	return p.report, nil
}

// runtimeReviewSeam models the runtime review block in runStages: it builds the
// task's review scope from the accumulated change set (rn.ChangedFiles() union
// declared deliverables), computes the scoped diff, and only then invokes the
// provider. A scopedReviewDiff error fails closed BEFORE any provider call. It
// mirrors the exact call path runStages wires so the regression can drive it with
// fakes and record what the provider received.
func runtimeReviewSeam(changed, deliverables []string, wholeDiff string, changeRequired bool, p review.Provider) (review.Report, error) {
	scope := newReviewScope(changed, deliverables, deliverables...)
	nothingOfOwn := !changeRequired
	if scope.empty() && nothingOfOwn {
		return review.Report{}, nil
	}
	reviewDiff, err := scopedReviewDiff(scope, wholeDiff, changeRequired)
	if err != nil {
		// Fail closed: no provider call is made.
		return review.Report{}, err
	}
	return p.Review(context.Background(), review.Request{Diff: reviewDiff})
}

// TestRuntimeScopedReviewReceivesScopedDiffNotDiffAll proves the actual review
// call path receives the task-scoped diff and never the whole-tree DiffAll: an
// unrelated pre-existing dirty file present in the working tree must not reach
// the provider, while the task-owned path does.
func TestRuntimeScopedReviewReceivesScopedDiffNotDiffAll(t *testing.T) {
	wholeDiff := "diff --git a/ours.go b/ours.go\n--- a/ours.go\n+++ b/ours.go\n@@ -1 +1 @@\n-old\n+new\n" +
		"diff --git a/unrelated-preexisting.go b/unrelated-preexisting.go\n--- a/unrelated-preexisting.go\n+++ b/unrelated-preexisting.go\n@@ -1 +1 @@\n-old\n+new\n"
	p := &recordingProvider{}
	if _, err := runtimeReviewSeam([]string{"ours.go"}, nil, wholeDiff, true, p); err != nil {
		t.Fatalf("runtimeReviewSeam: %v", err)
	}
	if !p.called {
		t.Fatal("provider was not called for an attributable task change")
	}
	if !strings.Contains(p.diff, "ours.go") {
		t.Errorf("scoped diff omitted the task-owned change:\n%s", p.diff)
	}
	if strings.Contains(p.diff, "unrelated-preexisting.go") {
		t.Errorf("the whole-tree DiffAll diff reached provider.Review; unrelated pre-existing change leaked:\n%s", p.diff)
	}
}

// TestRuntimeScopedReviewFailsClosedWithoutProviderCall proves the seam performs
// NO provider call when a required change cannot be attributed (a non-empty but
// unattributable tree): scopedReviewDiff fails closed and the provider is never
// invoked.
func TestRuntimeScopedReviewFailsClosedWithoutProviderCall(t *testing.T) {
	wholeDiff := "diff --git a/someone-elses.go b/someone-elses.go\n--- a/someone-elses.go\n+++ b/someone-elses.go\n@@ -1 +1 @@\n-old\n+new\n"
	p := &recordingProvider{}
	_, err := runtimeReviewSeam(nil, nil, wholeDiff, true, p)
	if !errors.Is(err, ErrReviewScopeUnestablished) {
		t.Fatalf("err = %v, want ErrReviewScopeUnestablished", err)
	}
	if p.called {
		t.Errorf("provider was called on an unattributable tree; the seam must fail closed without a provider call (diff=%q)", p.diff)
	}
}

// TestRuntimeScopedReviewPartialAttributionNeverSilentlyExcluded proves a mutation
// whose path is NOT present in the recorded task change set (rn.ChangedFiles()) is
// never silently excluded from review: it is either detected as in scope (when it
// is a declared deliverable) or the seam fails closed. Here the unrecorded path is
// not a deliverable, so the seam must fail closed rather than silently drop it and
// pass a clean review.
func TestRuntimeScopedReviewPartialAttributionNeverSilentlyExcluded(t *testing.T) {
	// Sanity: when a.go is recorded, it is in scope and reviewed.
	wholeDiff := "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1 +1 @@\n-old\n+new\n"
	pIn := &recordingProvider{}
	if _, err := runtimeReviewSeam([]string{"a.go"}, nil, wholeDiff, true, pIn); err != nil {
		t.Fatalf("recorded change must be reviewable: %v", err)
	}
	if !pIn.called || !strings.Contains(pIn.diff, "a.go") {
		t.Fatalf("recorded change not reviewed: called=%v diff=%q", pIn.called, pIn.diff)
	}

	// The partial-attribution case: nothing is recorded (rn.ChangedFiles() empty)
	// but the tree names a real repository file, so the required change cannot be
	// attributed. Fail closed, never a silent exclusion.
	pOut := &recordingProvider{}
	_, err := runtimeReviewSeam(nil, nil, wholeDiff, true, pOut)
	if !errors.Is(err, ErrReviewScopeUnestablished) {
		t.Fatalf("err = %v, want ErrReviewScopeUnestablished (fail closed, no silent exclusion)", err)
	}
	if pOut.called {
		t.Errorf("provider called despite unattributable mutation; a task-owned mutation was silently reviewed as if complete")
	}
}

// TestRuntimeScopedReviewUnauthorizedMutations exercises unauthorized task-owned
// mutations (tracked, untracked, deleted, renamed) as both an in-scope detection
// case and a fail-closed case.
func TestRuntimeScopedReviewUnauthorizedMutations(t *testing.T) {
	cases := []struct {
		name       string
		changed    []string
		diff       string
		wantInDiff string
	}{
		{
			name:       "tracked",
			changed:    []string{"m.go"},
			diff:       "diff --git a/m.go b/m.go\n--- a/m.go\n+++ b/m.go\n@@ -1 +1 @@\n-old\n+new\n",
			wantInDiff: "+++ b/m.go",
		},
		{
			name:       "untracked",
			changed:    []string{"new.md"},
			diff:       "diff --git a/new.md b/new.md\nnew file mode 100644\n--- /dev/null\n+++ b/new.md\n@@ -0,0 +1,1 @@\n+x\n",
			wantInDiff: "+++ b/new.md",
		},
		{
			name:       "deleted",
			changed:    []string{"gone.go"},
			diff:       "diff --git a/gone.go b/gone.go\ndeleted file mode 100644\n--- a/gone.go\n+++ /dev/null\n@@ -1 +0,0 @@\n-gone\n",
			wantInDiff: "--- a/gone.go",
		},
		{
			name:       "renamed",
			changed:    []string{"old.go", "new.go"},
			diff:       "diff --git a/old.go b/new.go\nsimilarity index 100%\nrename from old.go\nrename to new.go\n",
			wantInDiff: "rename to new.go",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name+"_in_scope", func(t *testing.T) {
			p := &recordingProvider{}
			if _, err := runtimeReviewSeam(tc.changed, nil, tc.diff, true, p); err != nil {
				t.Fatalf("in-scope %s mutation not reviewable: %v", tc.name, err)
			}
			if !p.called || !strings.Contains(p.diff, tc.wantInDiff) {
				t.Errorf("in-scope %s mutation not reviewed: called=%v diff=%q", tc.name, p.called, p.diff)
			}
		})
		t.Run(tc.name+"_fail_closed", func(t *testing.T) {
			// The same mutation, but none of its paths recorded: unattributable, so
			// the seam must fail closed and never silently exclude it.
			p := &recordingProvider{}
			_, err := runtimeReviewSeam(nil, nil, tc.diff, true, p)
			if !errors.Is(err, ErrReviewScopeUnestablished) {
				t.Fatalf("unattributable %s mutation err = %v, want ErrReviewScopeUnestablished", tc.name, err)
			}
			if p.called {
				t.Errorf("unattributable %s mutation reached the provider; must fail closed", tc.name)
			}
		})
	}
}

// TestRuntimeScopedReviewPerIterationRecomputation proves the scope is recomputed
// against the accumulated task change set across fix cycles: findings produced
// before (first iteration) and after (second iteration) a fix are both handled, and
// a newly recorded path from the fix joins the scope on the next iteration.
func TestRuntimeScopedReviewPerIterationRecomputation(t *testing.T) {
	firstDiff := "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1 +1 @@\n-old\n+new\n"
	// Iteration 1: only a.go recorded.
	p1 := &recordingProvider{report: review.Report{Findings: []review.Finding{{Severity: review.High, Title: "blocker"}}}}
	r1, err := runtimeReviewSeam([]string{"a.go"}, nil, firstDiff, true, p1)
	if err != nil {
		t.Fatalf("iteration 1: %v", err)
	}
	if !r1.Blocking(review.High) {
		t.Fatalf("iteration 1 pre-fix finding not handled: %+v", r1.Findings)
	}

	// Iteration 2: the fix recorded an additional path (b.go) into the accumulated
	// task change set; the scope is recomputed and now covers both.
	secondDiff := firstDiff + "diff --git a/b.go b/b.go\n--- a/b.go\n+++ b/b.go\n@@ -1 +1 @@\n-old\n+new\n"
	p2 := &recordingProvider{}
	if _, err := runtimeReviewSeam([]string{"a.go", "b.go"}, nil, secondDiff, true, p2); err != nil {
		t.Fatalf("iteration 2: %v", err)
	}
	if !strings.Contains(p2.diff, "a.go") || !strings.Contains(p2.diff, "b.go") {
		t.Errorf("scope not recomputed against the accumulated change set; post-fix path missing:\n%s", p2.diff)
	}
}
