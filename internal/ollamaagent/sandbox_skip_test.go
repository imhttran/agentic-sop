package ollamaagent

import (
	"os"
	"testing"

	"github.com/imhttran/agentic-sop/internal/toolharness"
)

// needSandboxAvailable skips a test that needs real model-reachable execution.
//
// Isolation is mandatory (M1 + G3 + G4): run_command, git_status, git_diff, and
// the per-turn git fingerprints all require a sandbox that can actually start. A
// host that cannot start one refuses every such call, so the agent loop cannot be
// exercised there. These tests skip rather than assert a behaviour the host
// cannot produce.
//
// SOP_SANDBOX_REQUIRED=1 turns the skip into a failure. The sandbox-enabled Linux
// integration job sets it, so a runner that silently lost its backend fails the
// job instead of reporting a green suite of skips.
func needSandboxAvailable(t *testing.T) {
	t.Helper()
	if err := toolharness.SandboxRunnable(); err != nil {
		if os.Getenv("SOP_SANDBOX_REQUIRED") == "1" {
			t.Fatalf("SOP_SANDBOX_REQUIRED=1 but no runnable sandbox backend: %v", err)
		}
		t.Skipf("no runnable sandbox backend: %v", err)
	}
}
