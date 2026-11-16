package ollamaagent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestHarnessPassesCargoAgentToolPermission proves the native tool harness wires
// the project's cargo.agent_tool permission into the shared toolharness. The
// toolharness unit test covers the policy itself; this test covers the production
// plumbing that previously dropped the flag.
func TestHarnessPassesCargoAgentToolPermission(t *testing.T) {
	needSandboxAvailable(t)
	root := t.TempDir()
	// The script lives under the harness root, not the host temp dir: bubblewrap
	// mounts a private /tmp, so a program there is invisible inside the sandbox.
	dir := filepath.Join(root, ".testbin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(dir, "cargo")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho fake-cargo-output\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	on := New(Config{
		CommandTimeout:        30 * time.Second,
		MaxOutputBytes:        1 << 16,
		CargoAgentToolEnabled: true,
	}, root)
	out, err := on.tools.Run(context.Background(), "run_command", map[string]any{"command": "cargo test --quiet"})
	if err != nil {
		t.Fatalf("enabled cargo should execute: %v", err)
	}
	if !strings.Contains(out, "fake-cargo-output") {
		t.Fatalf("expected fake cargo output, got %q", out)
	}

	off := New(Config{CommandTimeout: 30 * time.Second, MaxOutputBytes: 1 << 16}, root)
	if _, err := off.tools.Run(context.Background(), "run_command", map[string]any{"command": "cargo test --quiet"}); err == nil {
		t.Fatal("disabled cargo should be refused")
	}
}
