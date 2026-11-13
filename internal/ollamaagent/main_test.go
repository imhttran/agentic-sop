package ollamaagent

import (
	"os"
	"testing"

	"github.com/imhttran/agentic-sop/internal/budget"
	"github.com/imhttran/agentic-sop/internal/toolharness"
)

// TestMain makes the package's tests hermetic with respect to the optional
// IMPLEMENT/FIX iteration-ceiling environment overrides.
//
// The environment is process-global: the canonical budget reads the SOP_OLLAMA_*
// overrides, so a developer's shell (or a harness that exports them) could
// otherwise raise a ceiling and turn the policy/lifecycle assertions into failures
// that have nothing to do with the code under test. The
// tests that DELIBERATELY exercise the overrides set the variables with t.Setenv,
// which both applies and restores them, so clearing the namespace here only removes
// ambient leakage; it does not weaken any test.
func TestMain(m *testing.M) {
	for _, k := range []string{budget.EnvImplementIterations, budget.EnvFixIterations, budget.EnvStaleIterations, budget.EnvToolCalls} {
		_ = os.Unsetenv(k)
	}
	// run_command isolation is on by default in production. These tests exercise
	// the agent loop, and the tool harness tests own the isolation contract, so the
	// suite is pinned to policy-only here: that keeps it meaningful on hosts without
	// a sandbox backend (CI has no bwrap). A test that needs the sandbox sets
	// toolharness.Config.IsolateCommands, which wins over this override.
	_ = os.Setenv(toolharness.IsolationEnv, "off")
	_ = os.Setenv(toolharness.UnsandboxedAckEnv, "1")
	os.Exit(m.Run())
}
