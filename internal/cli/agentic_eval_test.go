package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/eval"
	"github.com/imhttran/agentic-sop/internal/runtrace"
)

// evalsRoot holds the deterministic evaluation fixtures, one per scenario.
const evalsRoot = "../../evals"

// evalDiff is a realistic unified diff so the run attributes a changed file.
const evalDiff = "diff --git a/pkg/x.go b/pkg/x.go\n--- a/pkg/x.go\n+++ b/pkg/x.go\n@@ -0,0 +1 @@\n+hello\n"

// loadEvalTrace reads the canonical trace a run wrote.
func loadEvalTrace(t *testing.T, dir, id string) runtrace.Trace {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, stateDirName, "runs", id, runtrace.FileName))
	if err != nil {
		t.Fatalf("read trace: %v", err)
	}
	var tr runtrace.Trace
	if err := json.Unmarshal(data, &tr); err != nil {
		t.Fatalf("parse trace: %v", err)
	}
	return tr
}

// runEvalFixture evaluates a captured trace against a fixture and fails with the
// deterministic diagnostics on any mismatch.
func runEvalFixture(t *testing.T, rel string, tr runtrace.Trace) {
	t.Helper()
	f, err := eval.LoadFixture(filepath.Join(evalsRoot, rel))
	if err != nil {
		t.Fatalf("load fixture %s: %v", rel, err)
	}
	res := eval.Evaluate(tr, f)
	if !res.Passed {
		for _, d := range res.Diagnostics {
			t.Errorf("%s", d.Message)
		}
		t.Fatalf("evaluation %q FAILED", f.Name)
	}
}

// TestAgenticEvalImplementationSuccess runs a bounded implementation and
// evaluates the real trace: a mutation, passing verification, a lifecycle
// advance, and a successful terminal, with no human decision.
func TestAgenticEvalImplementationSuccess(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
	a := &fakeCapabilityAgent{plan: validPlanJSON, impl: "changed files", review: `{"summary":"clean","findings":[]}`}

	if code, _, stderr := runInjectedCLI(t, dir, evalDiff, a, "run", "--task", "TASK.md"); code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	runEvalFixture(t, "implementation/success.expect.json", loadEvalTrace(t, dir, "T001"))
}

// TestAgenticEvalImplementationNoProgress is the regression evaluation for the
// failure pattern that motivated the reliability work: repeated activity with no
// repository mutation blocks for operator intervention, is not retryable, needs
// no human, and preserves the diagnostic.
func TestAgenticEvalImplementationNoProgress(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join("docs", "PLAN.md"), autoPlanDoc)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")
	a := outcomeAgent{outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: noProgressReason}}

	if code, _, _ := runInjectedCLI(t, dir, "diff\n", a, "run"); code != exitError {
		t.Fatalf("code=%d, want %d", code, exitError)
	}
	runEvalFixture(t, "implementation/no-progress.expect.json", loadEvalTrace(t, dir, "S001"))
}

// TestAgenticEvalHumanBoundary proves a destructive, authorization-required
// action is a genuine human boundary on the real trace.
func TestAgenticEvalHumanBoundary(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")
	a := outcomeAgent{outcome: &agent.Outcome{
		Status: agent.OutcomeNeedsHuman,
		Reason: "the requested change is destructive and irreversible, so it requires authorization",
	}}

	_, _, _ = runInjectedCLI(t, dir, evalDiff, a, "run", "--task", "TASK.md")
	runEvalFixture(t, "human-boundary/destructive.expect.json", loadEvalTrace(t, dir, "T001"))
}

// TestAgenticEvalVerificationFailure proves a failing deterministic check is not
// counted as verification progress, and the lifecycle outcome follows the current
// semantics.
func TestAgenticEvalVerificationFailure(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"false\"\n")
	a := &fakeCapabilityAgent{plan: validPlanJSON, impl: "changed files", fix: "changed files", review: `{"summary":"clean","findings":[]}`}

	_, _, _ = runInjectedCLI(t, dir, evalDiff, a, "run", "--task", "TASK.md")
	tr := loadEvalTrace(t, dir, "T001")
	t.Logf("verification-failure termination=%+v progress=%+v", tr.Termination, tr.ProgressSummary)
	runEvalFixture(t, "verification/failure.expect.json", tr)
}
