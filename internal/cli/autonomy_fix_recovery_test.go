package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// requiredTestRecoveryAgent leaves a required check red on the first IMPLEMENT
// and repairs it during the FIX cycle, so the deterministic FIX lifecycle is
// exercised: failing required gate -> FIX -> passing required gate.
type requiredTestRecoveryAgent struct {
	dir      string
	fixCalls int
}

func (a *requiredTestRecoveryAgent) Generate(_ context.Context, r agent.Request) (agent.Response, error) {
	switch r.Capability {
	case agent.Plan:
		return agent.Response{Content: validPlanJSON}, nil
	case agent.Implement:
		// First attempt reports completion but does not satisfy the required check.
		return agent.Response{Content: "implemented", Outcome: &agent.Outcome{Status: agent.OutcomeCompleted, Summary: "implemented", ChangesExpected: true}}, nil
	case agent.Fix:
		a.fixCalls++
		if err := os.WriteFile(filepath.Join(a.dir, "fixed.txt"), []byte("ok\n"), 0o644); err != nil {
			return agent.Response{}, err
		}
		return agent.Response{Content: "fixed", Outcome: &agent.Outcome{Status: agent.OutcomeCompleted, Summary: "repaired", ChangesExpected: true}}, nil
	case agent.Review:
		return agent.Response{Content: `{"summary":"clean","findings":[]}`}, nil
	default:
		return agent.Response{Content: "{}"}, nil
	}
}

// TestAutonomousRecoveryRepairsFailingRequiredCheck proves the existing bounded
// FIX lifecycle recovers a deterministic first-attempt failure: the required gate
// fails, a FIX cycle repairs it, the required gate then passes, and the repair is
// independently verified on the workspace.
func TestAutonomousRecoveryRepairsFailingRequiredCheck(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - test -f fixed.txt\nquality:\n  require_tests: true\n  max_fix_cycles: 2\n")
	a := &requiredTestRecoveryAgent{dir: dir}

	code, stdout, stderr := runInjectedCLI(t, dir, "diff --git a/x b/x\n", a, "run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "PASS") {
		t.Errorf("run should pass after the fix: %q", stdout)
	}
	if strings.Contains(stdout, "NEEDS_HUMAN") {
		t.Errorf("a recoverable failure must not require a human: %q", stdout)
	}
	if !strings.Contains(stdout, "fix cycles: 1/2") {
		t.Errorf("expected exactly one fix cycle: %q", stdout)
	}
	if a.fixCalls != 1 {
		t.Errorf("FIX invocations = %d, want 1", a.fixCalls)
	}

	// Independent behavioral verification: the required check now passes because the
	// FIX actually created the artifact it depends on.
	if _, err := os.Stat(filepath.Join(dir, "fixed.txt")); err != nil {
		t.Fatalf("fixed.txt not created by the fix: %v", err)
	}
	gate, err := os.ReadFile(filepath.Join(dir, stateDirName, "runs", "T001", "gate.json"))
	if err != nil {
		t.Fatalf("read gate.json: %v", err)
	}
	if !strings.Contains(string(gate), `"PASS"`) {
		t.Errorf("gate.json = %s, want PASS", gate)
	}
}
