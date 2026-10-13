package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// oneshotSatPlan has two independent tasks, each declaring the same criterion the
// injected agent's ALREADY_SATISFIED evidence names.
const oneshotSatPlan = "# Implementation Plan\n\n## Project\n\nW\n\n## Summary\n\nx\n\n" +
	"## S001 — A\n\nDo A.\n\n### Dependencies\n\nNone\n\n### Acceptance Criteria\n\n- existing implementation\n\n" +
	"## S002 — B\n\nDo B.\n\n### Dependencies\n\nNone\n\n### Acceptance Criteria\n\n- existing implementation\n"

// TestOneShotAlreadySatisfiedAdvancesGraph proves ONESHOT-002 in a multi-task graph:
// an independently verified ALREADY_SATISFIED task advances the run to the NEXT
// runnable task instead of stalling.
func TestOneShotAlreadySatisfiedAdvancesGraph(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join("docs", "PLAN.md"), oneshotSatPlan)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  test:\n    - \"true\"\nquality:\n  max_fix_cycles: 1\n")
	a := &satisfiedAgent{verified: true, outcomeAgent: outcomeAgent{outcome: &agent.Outcome{
		Status: agent.OutcomeCompleted, Completion: agent.AlreadySatisfied, ChangesExpected: false,
		Evidence: &agent.CompletionEvidence{
			Acceptance:         []agent.AcceptanceEvidence{{Criterion: "existing implementation", Paths: []string{"existing.go"}}},
			ValidationCommands: []string{"true"},
		},
	}}}

	code, stdout, stderr := runInjectedCLI(t, dir, "", a, "run")
	if code != exitOK {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Running: S002") {
		t.Fatalf("S002 did not run after S001 was independently verified already-satisfied:\n%s", stdout)
	}
}
