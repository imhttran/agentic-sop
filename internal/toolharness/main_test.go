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
// override to assert the default (TestRunCommandIsolationIsOnByDefault and
// TestRunCommandIsolationDefaultsToFailClosed).
func TestMain(m *testing.M) {
	_ = os.Setenv(IsolationEnv, "off")
	os.Exit(m.Run())
}
