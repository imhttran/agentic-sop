package config

import (
	"os"
	"strings"
	"testing"
)

// TestMain keeps the operator's real controller configuration out of these
// tests. The package reads its configuration from the process environment, so an
// ambient SOP_CONTROLLER_ALLOW_NETWORK would otherwise pair with any value a test
// does not set, and Load() would abort on the missing-token guard. That happens
// in practice when a test run is sandboxed and the sandbox scrubs the
// credential-named SOP_CONTROLLER_TOKEN while leaving its companions set.
// Every test that needs a value sets it itself with t.Setenv.
func TestMain(m *testing.M) {
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(name, "SOP_CONTROLLER_") {
			_ = os.Unsetenv(name)
		}
	}
	os.Exit(m.Run())
}
