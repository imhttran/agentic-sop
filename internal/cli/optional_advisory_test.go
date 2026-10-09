package cli

import (
	"strings"
	"testing"
)

// TestOptionalCheckFailureIsAdvisoryInRun proves a failing check that policy does
// not require keeps a passing run but is reported explicitly in the summary, so a
// PASS never silently hides a red optional check.
func TestOptionalCheckFailureIsAdvisoryInRun(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"false\"\nquality:\n  require_tests: false\n  max_fix_cycles: 2\n")
	a := &fakeCapabilityAgent{plan: validPlanJSON, impl: "changed files", review: `{"summary":"clean","findings":[]}`}

	code, stdout, stderr := runInjectedCLI(t, dir, "diff --git a/x b/x\n", a, "run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "advisory: unit tests failed") {
		t.Errorf("a failing optional check must be reported as an advisory: %q", stdout)
	}
}
