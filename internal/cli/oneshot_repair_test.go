package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// oneshotRepairAgent repairs a failing check WITHOUT a repository mutation: its FIX
// flips an external (out-of-repo) check state, so the repository is unchanged. This
// models a failure that is repaired by state outside the task's own diff.
type oneshotRepairAgent struct {
	flip  string
	doFix bool
	fixes int
}

func (a *oneshotRepairAgent) Generate(_ context.Context, r agent.Request) (agent.Response, error) {
	switch r.Capability {
	case agent.Plan:
		return agent.Response{Content: validPlanJSON}, nil
	case agent.Implement:
		return agent.Response{Content: "impl"}, nil
	case agent.Fix:
		a.fixes++
		if a.doFix {
			if err := os.WriteFile(a.flip, []byte("ok"), 0o644); err != nil {
				return agent.Response{}, err
			}
		}
		return agent.Response{Content: "fixed"}, nil
	default:
		return agent.Response{Content: `{"summary":"clean","findings":[]}`}, nil
	}
}

// TestOneShotRepairedFailureRevalidatesAndContinues proves ONESHOT-003: a failure that
// is no longer reproducible (repaired by state outside the task's diff) is
// re-validated within the run and, when it passes, the task completes instead of
// escalating. No production change is required: the existing bounded fix loop already
// re-validates after a fix that makes no repository change.
func TestOneShotRepairedFailureRevalidatesAndContinues(t *testing.T) {
	dir := t.TempDir()
	flip := filepath.Join(t.TempDir(), "flip") // outside the repository
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  test:\n    - \"test -f "+flip+"\"\nquality:\n  max_fix_cycles: 3\n")
	a := &oneshotRepairAgent{flip: flip, doFix: true}

	code, stdout, stderr := runInjectedCLI(t, dir, "diff\n", a, "run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("repaired failure did not continue: code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if a.fixes != 1 {
		t.Errorf("fix invocations = %d, want 1", a.fixes)
	}
}

// TestOneShotUnrepairedFailureEscalatesBounded proves ONESHOT-003 stays fail-closed: a
// failure that is NOT repaired keeps the existing escalation (AUTO_FIX_EXHAUSTED ->
// NEEDS_HUMAN), bounded by the configured fix budget (never an unbounded loop).
func TestOneShotUnrepairedFailureEscalatesBounded(t *testing.T) {
	dir := t.TempDir()
	flip := filepath.Join(t.TempDir(), "flip") // never created -> the check keeps failing
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  test:\n    - \"test -f "+flip+"\"\nquality:\n  max_fix_cycles: 3\n")
	a := &oneshotRepairAgent{flip: flip, doFix: false}

	code, stdout, stderr := runInjectedCLI(t, dir, "diff\n", a, "run", "--task", "TASK.md")
	if code == exitOK {
		t.Fatalf("unrepaired failure must not pass: stdout=%s stderr=%s", stdout, stderr)
	}
	if !strings.Contains(stdout, "NEEDS_HUMAN") {
		t.Errorf("want NEEDS_HUMAN escalation:\n%s", stdout)
	}
	if a.fixes < 1 || a.fixes > 3 {
		t.Errorf("fix invocations = %d, want 1..3 (bounded by the fix budget)", a.fixes)
	}
}
