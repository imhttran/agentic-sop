package toolharness

import (
	"os"
	"testing"
)

// TestMain disables run_command isolation for the package's command-policy tests
// unless a test opts back in. Isolation is on by default in production, so this
// keeps the policy tests meaningful on hosts without a sandbox backend (CI has no
// bwrap) instead of turning every command into a denial. Tests that exercise the
// sandbox set Config.IsolateCommands, which wins over the override, or clear the
// override to assert the default. The acknowledgement goes with the override:
// since D4-5 the off value alone no longer disables the sandbox (TestRunCommandIsolationIsOnByDefault and
// TestRunCommandIsolationDefaultsToFailClosed).
func TestMain(m *testing.M) {
	_ = os.Setenv(IsolationEnv, "off")
	_ = os.Setenv(UnsandboxedAckEnv, "1")
	os.Exit(m.Run())
}
