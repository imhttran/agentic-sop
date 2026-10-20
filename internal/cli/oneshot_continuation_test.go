package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/store"
)

const oneshotTwoTaskPlan = "# Implementation Plan\n\n## Project\n\nW\n\n## Summary\n\nx\n\n" +
	"## S001 — A\n\n### Dependencies\n\nNone\n\n## S002 — B\n\n### Dependencies\n\nNone\n"

const oneshotDependentPlan = "# Implementation Plan\n\n## Project\n\nW\n\n## Summary\n\nx\n\n" +
	"## S001 — A\n\n### Dependencies\n\nNone\n\n## S002 — B\n\n### Dependencies\n\n- S001\n"

// TestOneShotContinuesIndependentTaskAfterBlock proves ONESHOT-005: when S001
// reaches a task-local terminal block, the independent S002 still executes in the
// same `sop run` invocation, and the run still fails closed.
func TestOneShotContinuesIndependentTaskAfterBlock(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join("docs", "PLAN.md"), oneshotTwoTaskPlan)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")
	seedTask(t, dir, &domain.Task{ID: "S001", Title: "A", Status: domain.PLANNED, Attempt: 1, MaxAttempts: 1})
	seedTask(t, dir, &domain.Task{ID: "S002", Title: "B", Status: domain.PLANNED, MaxAttempts: 3})
	a := outcomeAgent{outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: noProgressReason}}

	code, stdout, _ := runInjectedCLI(t, dir, "diff\n", a, "run")
	if code == exitOK {
		t.Fatalf("want a non-success run (S001 blocked); stdout=\n%s", stdout)
	}
	if !strings.Contains(stdout, "Running: S002") {
		t.Fatalf("independent task S002 did not execute after S001 blocked:\n%s", stdout)
	}
	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if got, _ := st.Get("S001"); got.Status != domain.BLOCKED {
		t.Errorf("S001 status = %s, want BLOCKED", got.Status)
	}
}

// TestOneShotBlockedDependencyDoesNotRun proves ONESHOT-005 fails closed on the
// graph: a task blocked by its dependency never executes.
func TestOneShotBlockedDependencyDoesNotRun(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join("docs", "PLAN.md"), oneshotDependentPlan)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")
	a := outcomeAgent{outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: noProgressReason}}

	code, stdout, _ := runInjectedCLI(t, dir, "diff\n", a, "run")
	if code == exitOK {
		t.Fatalf("want a non-success run (S001 blocked); stdout=\n%s", stdout)
	}
	if strings.Contains(stdout, "Running: S002") {
		t.Fatalf("S002 depends on the blocked S001 and must not execute:\n%s", stdout)
	}
	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if got, err := st.Get("S002"); err == nil && got != nil && got.Status != domain.PLANNED {
		t.Errorf("S002 status = %s, want PLANNED (never executed)", got.Status)
	}
}
