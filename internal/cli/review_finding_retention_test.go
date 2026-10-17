package cli

// Regression coverage for FIX-002: a blocking REVIEW finding must never be
// cleared merely because REVIEW did not run. The vulnerable shape is a FIX that
// changes nothing and reports a completed outcome with changes_expected=false and
// no completion token (the legacy no-change shape a command provider may print).
// The lifecycle then skipped REVIEW on the empty working-tree diff, reset the
// report, and evaluated the gate with no unresolved findings, so a HIGH finding
// disappeared and the run PASSED.
//
// Every test here drives the real `run --task` lifecycle with injected deps, a
// scripted working-tree diff sequence, and a fake agent. No network, no timing
// dependency, no live SOP state is touched.

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/github"
)

const (
	retentionImplDiff = "diff --git a/x.go b/x.go\n--- a/x.go\n+++ b/x.go\n@@ -1 +1 @@\n-a\n+b\n"
	retentionFixDiff  = "diff --git a/x.go b/x.go\n--- a/x.go\n+++ b/x.go\n@@ -1 +1 @@\n+b\n+c\n"

	retentionBlockingFinding = `{"summary":"bad","findings":[{"severity":"HIGH","title":"nil deref","file":"x.go","line":3}]}`
	retentionMediumFinding   = `{"summary":"meh","findings":[{"severity":"MEDIUM","title":"style advisory","file":"x.go","line":1}]}`
	retentionCleanReview     = `{"summary":"clean","findings":[]}`

	// The legacy no-change shape: completed, no repository change intended, and no
	// completion token. agent.ParseOutcome maps it to exactly the outcome the
	// lifecycle reads, so it is a supported command-provider response.
	retentionLegacyNoChangeJSON = `{"status":"completed","changes_expected":false,"summary":"no change needed"}`
)

// retentionAgent answers each capability with canned content and a chosen FIX /
// IMPLEMENT structured outcome.
type retentionAgent struct {
	reviews   []string
	reviewIdx int

	implContent string
	implOutcome *agent.Outcome

	fixContent string
	fixOutcome *agent.Outcome
}

func (a *retentionAgent) Generate(_ context.Context, r agent.Request) (agent.Response, error) {
	switch r.Capability {
	case agent.Plan:
		return agent.Response{Content: validPlanJSON}, nil
	case agent.Implement:
		return agent.Response{Content: a.implContent, Outcome: a.implOutcome}, nil
	case agent.Fix:
		return agent.Response{Content: a.fixContent, Outcome: a.fixOutcome}, nil
	case agent.Review:
		i := a.reviewIdx
		if i >= len(a.reviews) {
			i = len(a.reviews) - 1
		}
		a.reviewIdx++
		return agent.Response{Content: a.reviews[i]}, nil
	}
	return agent.Response{Content: "{}"}, nil
}

// retentionRun drives the real lifecycle with a scripted sequence of working-tree
// diffs (the last entry repeats). Snapshotting is not injected, so mutation
// attribution comes from the injected diff.
func retentionRun(t *testing.T, dir string, a agent.Agent, diffs ...string) (int, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	calls := 0
	d := deps{
		getwd: func() (string, error) { return dir, nil },
		newAgent: func(harness, provider, model, class string) (agent.Agent, error) {
			return a, nil
		},
		readDiff: func(context.Context, string) (string, error) {
			i := calls
			if i >= len(diffs) {
				i = len(diffs) - 1
			}
			calls++
			return diffs[i], nil
		},
		commit:    func(context.Context, string, string) error { return nil },
		newGitHub: func(string) github.Client { return &fakeGitHub{} },
	}
	code := run([]string{"run", "--task", "TASK.md"}, &out, &errOut, d)
	return code, out.String()
}

// retentionConfig writes a task and a configuration whose validation passes, so
// the gate verdict depends only on the review findings.
func retentionConfig(t *testing.T, dir string) {
	t.Helper()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
}

// retentionArtifact reads one run artifact for task T001.
func retentionArtifact(t *testing.T, dir, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, stateDirName, "runs", "T001", name))
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(data)
}

// TestBlockingReviewFindingCannotBeClearedBySkippedReview is the FIX-002
// regression. A HIGH finding is raised, then a FIX changes nothing and returns the
// legacy no-change outcome, leaving the working tree empty so REVIEW does not run.
// The finding must remain unresolved and the run must NOT pass.
func TestBlockingReviewFindingCannotBeClearedBySkippedReview(t *testing.T) {
	dir := t.TempDir()
	retentionConfig(t, dir)
	a := &retentionAgent{
		reviews:     []string{retentionBlockingFinding},
		implContent: "changed files",
		fixContent:  retentionLegacyNoChangeJSON,
		fixOutcome:  agent.ParseOutcome(retentionLegacyNoChangeJSON),
	}

	code, stdout := retentionRun(t, dir, a, retentionImplDiff, "")
	if code == exitOK {
		t.Fatalf("the run PASSED with a blocking finding unresolved; stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, "NEEDS_HUMAN") && !strings.Contains(stdout, "FAIL") {
		t.Errorf("stdout does not report a failing or human disposition:\n%s", stdout)
	}
	// The finding must survive in the decision artifacts, not only in the console.
	if report := retentionArtifact(t, dir, "report.json"); !strings.Contains(report, "nil deref") {
		t.Errorf("the blocking finding is absent from report.json:\n%s", report)
	}
	if gate := retentionArtifact(t, dir, "gate.json"); !strings.Contains(gate, "FAIL") && !strings.Contains(gate, "NEEDS_HUMAN") {
		t.Errorf("gate.json is not failing or human:\n%s", gate)
	}
	cls := retentionArtifact(t, dir, "classification.json")
	if !strings.Contains(cls, "AUTO_FIX_EXHAUSTED") && !strings.Contains(cls, "NO_CHANGES_PRODUCED") {
		t.Errorf("classification is neither AUTO_FIX_EXHAUSTED nor NO_CHANGES_PRODUCED:\n%s", cls)
	}
	t.Logf("retained disposition:\n%s", strings.TrimSpace(cls))
	t.Logf("gate:\n%s", strings.TrimSpace(retentionArtifact(t, dir, "gate.json")))
}

// TestBlockingFindingRetainedWhenDiffRemainsNonEmpty is the causality control:
// with the same blocking finding and the same no-change FIX, but a non-empty diff
// so REVIEW runs again, the finding stays blocking. Existing behavior is
// preserved (the bounded fix budget is exhausted).
func TestBlockingFindingRetainedWhenDiffRemainsNonEmpty(t *testing.T) {
	dir := t.TempDir()
	retentionConfig(t, dir)
	a := &retentionAgent{
		reviews:     []string{retentionBlockingFinding},
		implContent: "changed files",
		fixContent:  retentionLegacyNoChangeJSON,
		fixOutcome:  agent.ParseOutcome(retentionLegacyNoChangeJSON),
	}

	code, stdout := retentionRun(t, dir, a, retentionImplDiff, retentionFixDiff)
	if code == exitOK {
		t.Fatalf("the run PASSED while REVIEW kept reporting a blocking finding; stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, "fix cycles: 3/3") {
		t.Errorf("the bounded fix budget was not exhausted:\n%s", stdout)
	}
	if report := retentionArtifact(t, dir, "report.json"); !strings.Contains(report, "nil deref") {
		t.Errorf("the blocking finding is absent from report.json:\n%s", report)
	}
}

// TestNoChangeFixWithCompletionTokenStillFailsClosed is the shape control: a
// no-change FIX that carries a completion token still trips the existing
// NO_CHANGES_PRODUCED guard.
func TestNoChangeFixWithCompletionTokenStillFailsClosed(t *testing.T) {
	dir := t.TempDir()
	retentionConfig(t, dir)
	claim := `{"status":"completed","changes_expected":false,"completion":"ALREADY_SATISFIED"}`
	a := &retentionAgent{
		reviews:     []string{retentionBlockingFinding},
		implContent: "changed files",
		fixContent:  claim,
		fixOutcome:  agent.ParseOutcome(claim),
	}

	code, stdout := retentionRun(t, dir, a, retentionImplDiff, "")
	if code == exitOK {
		t.Fatalf("the run PASSED after an unverified no-change FIX claim; stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, "NO_CHANGES_PRODUCED") {
		t.Errorf("the NO_CHANGES_PRODUCED guard did not fire:\n%s", stdout)
	}
}

// TestCleanReviewWithNoChangeCompletionStillPasses proves a legitimate no-change
// completion is unaffected: a clean review with no repository change still passes.
func TestCleanReviewWithNoChangeCompletionStillPasses(t *testing.T) {
	dir := t.TempDir()
	retentionConfig(t, dir)
	a := &retentionAgent{
		reviews:     []string{retentionCleanReview},
		implContent: "changed files",
		fixContent:  retentionLegacyNoChangeJSON,
		fixOutcome:  agent.ParseOutcome(retentionLegacyNoChangeJSON),
	}

	code, stdout := retentionRun(t, dir, a, retentionImplDiff, "")
	if code != exitOK {
		t.Fatalf("a legitimate no-change completion did not pass; stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, "PASS") {
		t.Errorf("stdout does not report PASS:\n%s", stdout)
	}
}

// TestNonBlockingFindingWithNoChangeFixStillPasses proves a sub-threshold finding
// neither blocks nor is silently dropped: the run passes and the advisory is still
// reported.
func TestNonBlockingFindingWithNoChangeFixStillPasses(t *testing.T) {
	dir := t.TempDir()
	retentionConfig(t, dir)
	a := &retentionAgent{
		reviews:     []string{retentionMediumFinding},
		implContent: "changed files",
		fixContent:  retentionLegacyNoChangeJSON,
		fixOutcome:  agent.ParseOutcome(retentionLegacyNoChangeJSON),
	}

	code, stdout := retentionRun(t, dir, a, retentionImplDiff, "")
	if code != exitOK {
		t.Fatalf("a non-blocking finding blocked the run; stdout:\n%s", stdout)
	}
	if report := retentionArtifact(t, dir, "report.json"); !strings.Contains(report, "style advisory") {
		t.Errorf("the non-blocking finding is absent from report.json:\n%s", report)
	}
}

// TestGenuineFixWithCleanReReviewStillPasses proves the resolution path is intact:
// a FIX that really changes the repository, followed by a clean re-review, passes.
func TestGenuineFixWithCleanReReviewStillPasses(t *testing.T) {
	dir := t.TempDir()
	retentionConfig(t, dir)
	a := &retentionAgent{
		reviews:     []string{retentionBlockingFinding, retentionCleanReview},
		implContent: "changed files",
		fixContent:  "fixed it",
		fixOutcome:  agent.ParseOutcome(`{"status":"completed","changes_expected":true}`),
	}

	code, stdout := retentionRun(t, dir, a, retentionImplDiff, retentionFixDiff)
	if code != exitOK {
		t.Fatalf("a genuine fix with a clean re-review did not pass; stdout:\n%s", stdout)
	}
	if !strings.Contains(stdout, "fix cycles: 1/3") {
		t.Errorf("the fix was not resolved on the first cycle:\n%s", stdout)
	}
}

// TestParseOutcomeAcceptsLegacyNoChangeShape pins the input shape the regression
// relies on: the command provider's parser accepts a completed outcome with
// changes_expected=false and no completion token, yielding exactly the outcome
// the lifecycle treats as a legacy no-change completion.
func TestParseOutcomeAcceptsLegacyNoChangeShape(t *testing.T) {
	for _, content := range []string{
		retentionLegacyNoChangeJSON,
		`{"status":"completed","changes_expected":false}`,
	} {
		o := agent.ParseOutcome(content)
		if o == nil {
			t.Fatalf("ParseOutcome(%s) = nil, want a completed outcome", content)
		}
		if o.Status != agent.OutcomeCompleted || o.ChangesExpected || o.Completion != "" {
			t.Fatalf("ParseOutcome(%s) = %+v; want completed, changes_expected=false, completion=\"\"", content, o)
		}
	}
}
