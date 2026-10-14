package cli

import (
	"os"
	"testing"
)

// TestMain neutralizes the operator's project .env adapter-runtime configuration for
// this package's tests. The .env supplies live configuration (CLEF_*, NIMBLE_*,
// OLLAMA_BASE_URL, the decision fallback); these tests assert the CLI's own defaults
// and provider-selection semantics, so the operator's live values must not decide the
// outcome. Tests that need a value set it with t.Setenv.
func TestMain(m *testing.M) {
	for _, k := range []string{
		"CLEF_ENABLED", "CLEF_MODEL", "CLEF_TRANSPORT", "CLEF_TIMEOUT",
		"NIMBLE_MODEL", "NIMBLE_TRANSPORT", "NIMBLE_SCORE_URL", "NIMBLE_TIMEOUT",
		"OLLAMA_BASE_URL", "SOP_DECISION_FALLBACK",
	} {
		_ = os.Unsetenv(k)
	}
	os.Exit(m.Run())
}
