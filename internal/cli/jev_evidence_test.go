package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/jev"
	"github.com/imhttran/agentic-sop/internal/review"
)

// Task-evidence assembly tests (JEV review of a resumed/continued task).
//
// JEV reviews the TASK, not merely the latest agent invocation. A task implemented
// across several bounded invocations keeps its accumulated implementation evidence
// in the run directory, so a no-change final invocation still has something for JEV
// to review. These tests drive the lifecycle deterministically with a fake agent
// that reports the paths it changed; they require no live Ollama server.

// evidenceAgent is a deterministic capability-aware agent that reports the
// repository paths each mutating invocation changed, so a test can model task
// change evidence without touching the network.
type evidenceAgent struct {
	implChanged []string
	fixChanged  []string
	implOutcome *agent.Outcome
	fixInputs   []string
}

func (a *evidenceAgent) Generate(_ context.Context, r agent.Request) (agent.Response, error) {
	switch r.Capability {
	case agent.Plan:
		return agent.Response{Content: validPlanJSON}, nil
	case agent.Implement:
		return agent.Response{Content: "impl", Outcome: a.implOutcome, ChangedFiles: a.implChanged}, nil
	case agent.Fix:
		a.fixInputs = append(a.fixInputs, r.Input)
		return agent.Response{Content: "fixed", ChangedFiles: a.fixChanged}, nil
	case agent.Review:
		return agent.Response{Content: `{"summary":"clean","findings":[]}`}, nil
	default:
		return agent.Response{Content: "{}"}, nil
	}
}

// modifiedDiff builds a git-shaped diff that modifies each path, so the lifecycle's
// changed-file extraction has something to find.
func modifiedDiff(paths ...string) string {
	var b strings.Builder
	for _, p := range paths {
		fmt.Fprintf(&b, "diff --git a/%s b/%s\n--- a/%s\n+++ b/%s\n@@ -1 +1 @@\n-old\n+new\n", p, p, p, p)
	}
	return b.String()
}

// sequenceDiffs returns a readDiff that yields the given diffs in order, repeating
// the last one. It models a task whose invocations see different working-tree
// changes (earlier work committed, later no-change).
func sequenceDiffs(diffs ...string) func(context.Context, string) (string, error) {
	i := 0
	return func(context.Context, string) (string, error) {
		d := diffs[i]
		if i < len(diffs)-1 {
			i++
		}
		return d, nil
	}
}

func writeRepoFile(t *testing.T, dir, name, content string) {
	t.Helper()
	full := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func setupJEVTask(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, jevEnabledConfig)
	return dir
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// The change evidence survives a CONTINUE: an invocation that only reports an
// incomplete, budget-exhausted implementation (and so never reaches JEV) still
// records its changes, so the next invocation's JEV reviews the whole task.
func TestJEVEvidenceSurvivesContinue(t *testing.T) {
	dir := setupJEVTask(t)
	analyzer := &sequencedJEV{results: []jev.Result{{Status: jev.StatusPass}}}
	factory := func(config.Config) (jev.Analyzer, error) { return analyzer, nil }

	// Invocation 1: the agent changed internal/a.go but ran out of budget, so the
	// invocation is a resumable CONTINUE and JEV never runs for it.
	a1 := &evidenceAgent{
		implChanged: []string{"internal/a.go"},
		implOutcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: budgetExhaustedReason},
	}
	if code, _, _ := runInjectedCLIWithDiffFunc(t, dir, sequenceDiffs(""), a1, factory, "run", "--task", "TASK.md"); code != exitError {
		t.Fatalf("invocation 1: code=%d, want a non-pass CONTINUE", code)
	}
	if len(analyzer.requests) != 0 {
		t.Fatalf("JEV ran for an incomplete invocation: %d requests", len(analyzer.requests))
	}

	// Invocation 2 (the continuation): the agent changes internal/b.go and the task
	// completes. JEV must see both files.
	a2 := &evidenceAgent{implChanged: []string{"internal/b.go"}}
	if code, _, stderr := runInjectedCLIWithDiffFunc(t, dir, sequenceDiffs(modifiedDiff("internal/b.go")), a2, factory, "run", "--task", "TASK.md"); code != exitOK {
		t.Fatalf("invocation 2: code=%d stderr=%s", code, stderr)
	}
	if len(analyzer.requests) == 0 {
		t.Fatal("JEV was never invoked on the continuation")
	}
	got := analyzer.requests[len(analyzer.requests)-1].ChangedFiles
	want := []string{"internal/a.go", "internal/b.go"}
	if !sameStrings(got, want) {
		t.Errorf("continuation JEV ChangedFiles = %v, want %v", got, want)
	}
}

// A task implemented across two invocations keeps both files as task evidence: the
// final invocation's JEV request carries the union, not only the latest change.
func TestJEVAggregatesTaskChangeEvidenceAcrossInvocations(t *testing.T) {
	dir := setupJEVTask(t)
	analyzer := &sequencedJEV{results: []jev.Result{{Status: jev.StatusPass}}}
	factory := func(config.Config) (jev.Analyzer, error) { return analyzer, nil }

	a1 := &evidenceAgent{implChanged: []string{"internal/a.go"}}
	if code, _, stderr := runInjectedCLIWithDiffFunc(t, dir, sequenceDiffs(modifiedDiff("internal/a.go")), a1, factory, "run", "--task", "TASK.md"); code != exitOK {
		t.Fatalf("invocation 1: code=%d stderr=%s", code, stderr)
	}
	a2 := &evidenceAgent{implChanged: []string{"internal/b.go"}}
	if code, _, stderr := runInjectedCLIWithDiffFunc(t, dir, sequenceDiffs(modifiedDiff("internal/b.go")), a2, factory, "run", "--task", "TASK.md"); code != exitOK {
		t.Fatalf("invocation 2: code=%d stderr=%s", code, stderr)
	}

	if len(analyzer.requests) < 2 {
		t.Fatalf("JEV requests = %d, want one per invocation", len(analyzer.requests))
	}
	got := analyzer.requests[len(analyzer.requests)-1].ChangedFiles
	want := []string{"internal/a.go", "internal/b.go"}
	if !sameStrings(got, want) {
		t.Errorf("final JEV ChangedFiles = %v, want %v (accumulated across invocations)", got, want)
	}
}

// A final invocation that makes no repository change still hands JEV the task's
// accumulated implementation: the changed files and bounded file context.
func TestJEVNoChangeFinalInvocationReviewsAccumulatedEvidence(t *testing.T) {
	dir := setupJEVTask(t)
	writeRepoFile(t, dir, "internal/a.go", "package internal\n\nconst Marker = 1\n")
	analyzer := &sequencedJEV{results: []jev.Result{{Status: jev.StatusPass}}}
	factory := func(config.Config) (jev.Analyzer, error) { return analyzer, nil }

	a1 := &evidenceAgent{implChanged: []string{"internal/a.go"}}
	if code, _, stderr := runInjectedCLIWithDiffFunc(t, dir, sequenceDiffs(modifiedDiff("internal/a.go")), a1, factory, "run", "--task", "TASK.md"); code != exitOK {
		t.Fatalf("invocation 1: code=%d stderr=%s", code, stderr)
	}

	// The final invocation completes normally without touching the repository.
	a2 := &evidenceAgent{implOutcome: &agent.Outcome{Status: agent.OutcomeCompleted, ChangesExpected: false}}
	code, stdout, stderr := runInjectedCLIWithDiffFunc(t, dir, sequenceDiffs(""), a2, factory, "run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("final invocation: code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}

	req := analyzer.requests[len(analyzer.requests)-1]
	if !sameStrings(req.ChangedFiles, []string{"internal/a.go"}) {
		t.Errorf("final JEV ChangedFiles = %v, want [internal/a.go]", req.ChangedFiles)
	}
	if !strings.Contains(req.RepositoryContext, "const Marker = 1") {
		t.Errorf("final JEV repository context lacks the accumulated implementation:\n%s", req.RepositoryContext)
	}
}

// A FIX that changes another task-owned file contributes that file to the task's
// evidence, alongside the implementation's own change.
func TestJEVIncludesFixChangesInTaskEvidence(t *testing.T) {
	dir := setupJEVTask(t)
	analyzer := &sequencedJEV{results: []jev.Result{
		{Status: jev.StatusFindings, Findings: []jev.Finding{
			jevFinding(review.High, "internal/a.go", 1, "problem", "evidence"),
		}},
		{Status: jev.StatusPass},
	}}
	factory := func(config.Config) (jev.Analyzer, error) { return analyzer, nil }

	a := &evidenceAgent{
		implChanged: []string{"internal/a.go"},
		fixChanged:  []string{"internal/b.go"},
	}
	readDiff := sequenceDiffs(modifiedDiff("internal/a.go"), modifiedDiff("internal/a.go", "internal/b.go"))
	code, stdout, stderr := runInjectedCLIWithDiffFunc(t, dir, readDiff, a, factory, "run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}

	got := analyzer.requests[len(analyzer.requests)-1].ChangedFiles
	want := []string{"internal/a.go", "internal/b.go"}
	if !sameStrings(got, want) {
		t.Errorf("post-fix JEV ChangedFiles = %v, want %v", got, want)
	}
}

// A pre-existing dirty file the invocation never changed is not attributed to the
// task: the agent's own mutation evidence wins over the working-tree diff.
func TestJEVDoesNotAttributeUnrelatedDirtyFile(t *testing.T) {
	dir := setupJEVTask(t)
	analyzer := &sequencedJEV{results: []jev.Result{{Status: jev.StatusPass}}}
	factory := func(config.Config) (jev.Analyzer, error) { return analyzer, nil }

	a := &evidenceAgent{implChanged: []string{"internal/a.go"}}
	diff := modifiedDiff("internal/a.go", "notes.txt")
	if code, _, stderr := runInjectedCLIWithDiffFunc(t, dir, sequenceDiffs(diff), a, factory, "run", "--task", "TASK.md"); code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}

	if got := analyzer.requests[0].ChangedFiles; !sameStrings(got, []string{"internal/a.go"}) {
		t.Errorf("JEV ChangedFiles = %v, want only the task file [internal/a.go]", got)
	}
}

// When the provider cannot report changed files, SOP falls back to the diff, but
// SOP's own state and output are never attributed to the task — so an intentional
// user-owned .agent-sdlc/config.yaml edit is not task evidence.
func TestJEVIgnoresSOPOwnedPathsInDiffFallback(t *testing.T) {
	dir := setupJEVTask(t)
	analyzer := &sequencedJEV{results: []jev.Result{{Status: jev.StatusPass}}}
	factory := func(config.Config) (jev.Analyzer, error) { return analyzer, nil }

	a := &evidenceAgent{} // no reported files: the diff-derived set is used
	diff := modifiedDiff("internal/a.go", ".agent-sdlc/config.yaml", "docs/reports/r.md")
	if code, _, stderr := runInjectedCLIWithDiffFunc(t, dir, sequenceDiffs(diff), a, factory, "run", "--task", "TASK.md"); code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}

	if got := analyzer.requests[0].ChangedFiles; !sameStrings(got, []string{"internal/a.go"}) {
		t.Errorf("JEV ChangedFiles = %v, want only [internal/a.go]", got)
	}
}

// Even when a provider reports a SOP-owned path, it is filtered: the invariant does
// not depend on the provider behaving.
func TestJEVFiltersSOPOwnedReportedPaths(t *testing.T) {
	dir := setupJEVTask(t)
	analyzer := &sequencedJEV{results: []jev.Result{{Status: jev.StatusPass}}}
	factory := func(config.Config) (jev.Analyzer, error) { return analyzer, nil }

	a := &evidenceAgent{implChanged: []string{"internal/a.go", ".agent-sdlc/config.yaml"}}
	if code, _, stderr := runInjectedCLIWithDiffFunc(t, dir, sequenceDiffs(modifiedDiff("internal/a.go")), a, factory, "run", "--task", "TASK.md"); code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}

	if got := analyzer.requests[0].ChangedFiles; !sameStrings(got, []string{"internal/a.go"}) {
		t.Errorf("JEV ChangedFiles = %v, want only [internal/a.go]", got)
	}
}

// With genuinely no implementation evidence, JEV still runs and its INCOMPLETE
// result is preserved: it is never converted into a PASS.
func TestJEVPreservesIncompleteWhenThereIsNoEvidence(t *testing.T) {
	dir := setupJEVTask(t)
	analyzer := &sequencedJEV{results: []jev.Result{{Status: jev.StatusIncomplete, Summary: "no evidence to review"}}}
	factory := func(config.Config) (jev.Analyzer, error) { return analyzer, nil }

	a := &evidenceAgent{implOutcome: &agent.Outcome{Status: agent.OutcomeCompleted, ChangesExpected: false}}
	code, stdout, stderr := runInjectedCLIWithDiffFunc(t, dir, sequenceDiffs(""), a, factory, "run", "--task", "TASK.md")
	if code == exitOK {
		t.Fatalf("an INCOMPLETE JEV must not pass; stdout=%s stderr=%s", stdout, stderr)
	}
	if strings.Contains(stdout, ": PASS") {
		t.Errorf("stdout claimed PASS on an INCOMPLETE JEV: %q", stdout)
	}
	if len(analyzer.requests) == 0 {
		t.Fatal("JEV was never invoked")
	}
	if got := analyzer.requests[0].ChangedFiles; got != nil {
		t.Errorf("JEV ChangedFiles = %v, want none when there is no evidence", got)
	}
}

// Task evidence stays bounded: an unbounded accumulated set is capped, and the
// repository context stays a bounded snapshot rather than a repository dump.
func TestJEVTaskEvidenceIsBounded(t *testing.T) {
	dir := setupJEVTask(t)
	var paths []string
	for i := 0; i < 80; i++ {
		p := fmt.Sprintf("internal/f%02d.go", i)
		writeRepoFile(t, dir, p, strings.Repeat("x", 3000))
		paths = append(paths, p)
	}
	analyzer := &sequencedJEV{results: []jev.Result{{Status: jev.StatusPass}}}
	factory := func(config.Config) (jev.Analyzer, error) { return analyzer, nil }

	a1 := &evidenceAgent{implChanged: paths}
	if code, _, stderr := runInjectedCLIWithDiffFunc(t, dir, sequenceDiffs(modifiedDiff(paths...)), a1, factory, "run", "--task", "TASK.md"); code != exitOK {
		t.Fatalf("invocation 1: code=%d stderr=%s", code, stderr)
	}
	a2 := &evidenceAgent{implOutcome: &agent.Outcome{Status: agent.OutcomeCompleted, ChangesExpected: false}}
	if code, _, stderr := runInjectedCLIWithDiffFunc(t, dir, sequenceDiffs(""), a2, factory, "run", "--task", "TASK.md"); code != exitOK {
		t.Fatalf("final invocation: code=%d stderr=%s", code, stderr)
	}

	req := analyzer.requests[len(analyzer.requests)-1]
	if len(req.ChangedFiles) == 0 || len(req.ChangedFiles) > 64 {
		t.Errorf("ChangedFiles = %d entries, want a non-empty set bounded at 64", len(req.ChangedFiles))
	}
	if len(req.RepositoryContext) > maxJEVContextBytes+1024 {
		t.Errorf("RepositoryContext = %d bytes, want a bounded snapshot (<= %d)", len(req.RepositoryContext), maxJEVContextBytes+1024)
	}
}

// A single-invocation task reports the same changed files it always did, so the
// task-evidence assembly does not change the ordinary case.
func TestJEVSingleInvocationChangeEvidenceUnchanged(t *testing.T) {
	dir := setupJEVTask(t)
	analyzer := &sequencedJEV{results: []jev.Result{{Status: jev.StatusPass}}}
	factory := func(config.Config) (jev.Analyzer, error) { return analyzer, nil }

	a := &evidenceAgent{implChanged: []string{"internal/a.go"}}
	if code, _, stderr := runInjectedCLIWithDiffFunc(t, dir, sequenceDiffs(modifiedDiff("internal/a.go")), a, factory, "run", "--task", "TASK.md"); code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if got := analyzer.requests[0].ChangedFiles; !sameStrings(got, []string{"internal/a.go"}) {
		t.Errorf("JEV ChangedFiles = %v, want [internal/a.go]", got)
	}
	if got := analyzer.requests[0].RepositoryContext; !strings.Contains(got, "diff --git a/internal/a.go") {
		t.Errorf("JEV repository context should still carry the diff:\n%s", got)
	}
}
