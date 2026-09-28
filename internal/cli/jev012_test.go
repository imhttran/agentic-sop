package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/jev"
	"github.com/imhttran/agentic-sop/internal/review"
)

// JEV012 deterministic integration coverage: PASS completion and the human gate,
// malformed/fail-closed results at the lifecycle seam, diagnostic persistence,
// and compatibility with automatic blocked recovery and completed-plan handoff.
// Every test is offline: a sequenced fake analyzer and a fake agent stand in for
// the model.

// TestJEVPASSAllowsCompletion is JEV012 case 3 (a JEV PASS allows completion) and
// case 13 (the human approval gate is untouched by JEV).
func TestJEVPASSAllowsCompletion(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, jevEnabledConfig)
	a := &jevTestAgent{plan: validPlanJSON, impl: "impl", review: `{"summary":"clean","findings":[]}`}
	analyzer := &sequencedJEV{results: []jev.Result{{Status: jev.StatusPass}}}

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, jevDiff, a,
		func(config.Config) (jev.Analyzer, error) { return analyzer, nil },
		"run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if analyzer.calls != 1 {
		t.Errorf("JEV invocations = %d, want 1", analyzer.calls)
	}
	if !strings.Contains(stdout, "run T001: PASS") {
		t.Errorf("stdout = %q, want a PASS run", stdout)
	}
	// The human gate still fires: JEV never authorizes a commit.
	if !strings.Contains(stdout, "human approval required before commit") {
		t.Errorf("stdout = %q, want the human approval gate", stdout)
	}
}

// TestJEVMalformedOrIncompleteResultFailsClosed is JEV012 case 6 (malformed output
// fails closed) and case 7 (an unusable result never becomes PASS), exercised
// through the lifecycle seam.
func TestJEVMalformedOrIncompleteResultFailsClosed(t *testing.T) {
	cases := []struct {
		name string
		res  jev.Result
	}{
		{"unknown status is malformed", jev.Result{Status: jev.Status("WHATEVER")}},
		{"pass with findings is malformed", jev.Result{Status: jev.StatusPass, Findings: []jev.Finding{
			{Severity: review.High, Message: "contradiction"},
		}}},
		{"incomplete is not a pass", jev.Result{Status: jev.StatusIncomplete}},
		{"error is not a pass", jev.Result{Status: jev.StatusError}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "TASK.md", runTaskFile)
			writeConfig(t, dir, jevEnabledConfig)
			a := &jevTestAgent{plan: validPlanJSON, impl: "impl", review: `{"summary":"clean","findings":[]}`}
			analyzer := &sequencedJEV{results: []jev.Result{tc.res}}

			code, stdout, stderr := runInjectedCLIWithJEV(t, dir, jevDiff, a,
				func(config.Config) (jev.Analyzer, error) { return analyzer, nil },
				"run", "--task", "TASK.md")
			if code == exitOK {
				t.Fatalf("a fail-closed JEV must not pass; stdout=%s stderr=%s", stdout, stderr)
			}
			if strings.Contains(stdout, ": PASS") {
				t.Errorf("stdout claimed PASS on a fail-closed JEV:\n%s", stdout)
			}
		})
	}
}

// TestJEVPersistsDiagnosticArtifact is JEV012 case 12: the result is persisted as a
// run artifact. A blocking finding forces a fix, so JEV runs twice and the
// append-only history preserves both results while jev.json holds the latest.
func TestJEVPersistsDiagnosticArtifact(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, jevEnabledConfig)
	a := &jevTestAgent{plan: validPlanJSON, impl: "impl", fix: "fixed", review: `{"summary":"clean","findings":[]}`}
	analyzer := &sequencedJEV{results: []jev.Result{
		{Status: jev.StatusFindings, Findings: []jev.Finding{
			jevFinding(review.High, "internal/x.go", 12, "file handle not closed", "proof"),
		}},
		{Status: jev.StatusPass},
	}}

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, jevDiff, a,
		func(config.Config) (jev.Analyzer, error) { return analyzer, nil },
		"run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}

	runDir := filepath.Join(dir, stateDirName, "runs", "T001")
	latest, err := readFileString(filepath.Join(runDir, "jev.json"))
	if err != nil {
		t.Fatalf("read jev.json: %v", err)
	}
	for _, want := range []string{`"task_id": "T001"`, `"status": "PASS"`, `"fail_closed": false`} {
		if !strings.Contains(latest, want) {
			t.Errorf("jev.json missing %q:\n%s", want, latest)
		}
	}

	history, err := readFileString(filepath.Join(runDir, "jev-history.jsonl"))
	if err != nil {
		t.Fatalf("read jev-history.jsonl: %v", err)
	}
	lines := strings.Split(strings.TrimRight(history, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("history lines = %d, want 2 (one per JEV invocation):\n%s", len(lines), history)
	}
	if !strings.Contains(lines[0], `"status":"FINDINGS"`) {
		t.Errorf("first history entry = %q, want the initial FINDINGS result", lines[0])
	}
	if !strings.Contains(lines[1], `"status":"PASS"`) {
		t.Errorf("second history entry = %q, want the post-fix PASS result", lines[1])
	}
}

// TestJEVEnabledRecoveryRemainsCompatible is JEV012 case 14: automatic blocked-task
// recovery still works with JEV enabled, and JEV runs on the recovered task.
func TestJEVEnabledRecoveryRemainsCompatible(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeConfig(t, dir, jevEnabledConfig)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "T001", Status: domain.BLOCKED, BlockedReason: domain.REVIEW_UNRESOLVED, Attempt: 1, MaxAttempts: 3})
	a := &recoveryAgent{}
	analyzer := &sequencedJEV{results: []jev.Result{{Status: jev.StatusPass}}}

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, "diff\n", a,
		func(config.Config) (jev.Analyzer, error) { return analyzer, nil },
		"run")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "Recovering: T001") {
		t.Errorf("stdout = %q, want the blocked task recovered", stdout)
	}
	if !strings.Contains(stdout, "T001 LOCAL_DONE") {
		t.Errorf("stdout = %q, want T001 completed", stdout)
	}
	if analyzer.calls == 0 {
		t.Error("JEV did not run on the recovered task")
	}
}

// TestJEVEnabledCompletedPlanHandoffRemainsCompatible is JEV012 case 15 (the
// completed-plan handoff half): a JEV-enabled run completes a plan, and a request
// for a different plan still hands off in the same invocation with state preserved.
func TestJEVEnabledCompletedPlanHandoffRemainsCompatible(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join("docs", "PLAN-Hardening.md"), autoPlanDoc)
	writeFile(t, dir, filepath.Join("docs", "PLAN.md"), autoPlanDoc2)
	writeConfig(t, dir, jevEnabledConfig)
	a := &fakeCapabilityAgent{plan: validPlanJSON, impl: "x", review: `{"summary":"clean","findings":[]}`}
	analyzer := &sequencedJEV{results: []jev.Result{{Status: jev.StatusPass}}}
	factory := func(config.Config) (jev.Analyzer, error) { return analyzer, nil }

	if code, _, errs := runInjectedCLIWithJEV(t, dir, "diff\n", a, factory, "run", filepath.Join("docs", "PLAN-Hardening.md")); code != exitOK {
		t.Fatalf("first plan: code=%d stderr=%s", code, errs)
	}

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, "diff\n", a, factory, "run", filepath.Join("docs", "PLAN.md"))
	if code != exitOK {
		t.Fatalf("handoff: code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	for _, want := range []string{"Source: " + filepath.Join("docs", "PLAN.md"), "all tasks done"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout)
		}
	}
	if !stateExists(statePath(dir)) {
		t.Error("state.db must be preserved across handoff")
	}
}

// TestJEVEnabledReconcileRemainsCompatible is JEV012 case 15 (the reconciliation
// half): with JEV configured, reconciling an unchanged plan is still a safe no-op.
func TestJEVEnabledReconcileRemainsCompatible(t *testing.T) {
	dir := seedRAGProject(t)
	writeConfig(t, dir, jevEnabledConfig)

	code, stdout, stderr := runCLI(t, dir, "reconcile", filepath.Join("docs", "PLAN.md"))
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "matches the recorded plan") {
		t.Errorf("stdout = %q, want the unchanged-plan reconciliation", stdout)
	}
	if !stateExists(statePath(dir)) {
		t.Error("state.db must be preserved across reconciliation")
	}
}

// readFileString reads a file into a string.
func readFileString(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
