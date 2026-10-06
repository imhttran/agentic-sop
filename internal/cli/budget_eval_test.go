package cli

import (
	"path/filepath"
	"testing"

	"github.com/imhttran/agentic-sop/internal/budget"
)

// clearBudgetEnv forces the canonical budget to its built-in defaults so a budget
// assertion is not perturbed by an ambient operator override.
func clearBudgetEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{budget.EnvImplementIterations, budget.EnvFixIterations, budget.EnvStaleIterations, budget.EnvToolCalls} {
		t.Setenv(k, "")
	}
}

// TestAgenticEvalBudgetLimitsApplied proves the run's trace records the
// deterministic limits that applied.
func TestAgenticEvalBudgetLimitsApplied(t *testing.T) {
	clearBudgetEnv(t)
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")
	a := &fakeCapabilityAgent{plan: validPlanJSON, impl: "changed files", review: `{"summary":"clean","findings":[]}`}
	_, _, _ = runInjectedCLI(t, dir, evalDiff, a, "run", "--task", "TASK.md")
	runEvalFixture(t, filepath.Join("budgets", "limits-applied.expect.json"), loadEvalTrace(t, dir, "T001"))
}

// TestAgenticEvalBudgetConfigured proves an operator override is applied and
// recorded — the budget is configurable, and the trace reflects the override.
func TestAgenticEvalBudgetConfigured(t *testing.T) {
	clearBudgetEnv(t)
	t.Setenv(budget.EnvImplementIterations, "64")
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")
	a := &fakeCapabilityAgent{plan: validPlanJSON, impl: "changed files", review: `{"summary":"clean","findings":[]}`}
	_, _, _ = runInjectedCLI(t, dir, evalDiff, a, "run", "--task", "TASK.md")
	runEvalFixture(t, filepath.Join("budgets", "configured.expect.json"), loadEvalTrace(t, dir, "T001"))
}
