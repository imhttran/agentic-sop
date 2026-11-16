package ollamaagent

import (
	"os"
	"testing"

	"github.com/imhttran/agentic-sop/internal/budget"
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
	// Isolation is mandatory (M1 + G3 + G4) and has no override, so an ambient
	// value cannot change it. Tests that execute commands or inspect the working
	// tree gate on toolharness.SandboxRunnable and skip when this host cannot start
	// the platform sandbox (CI has no bwrap).
	os.Exit(m.Run())
}
