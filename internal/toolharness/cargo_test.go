package toolharness

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// installFakeCargo puts a fake `cargo` executable first on PATH so a Cargo run is
// observable without a real Rust toolchain. It returns nothing; the caller's
// environment is restored by t.Setenv.
func installFakeCargo(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, "cargo")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho fake-cargo-output\n"), 0o755); err != nil {
		t.Fatalf("write fake cargo: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// TestAgentToolCargoExecutesWhenEnabled proves an enabled agent-tool permission
// lets a supported Cargo command execute, and that with the permission off the
// harness refuses the same command (SOP-001).
func TestAgentToolCargoExecutesWhenEnabled(t *testing.T) {
	installFakeCargo(t)
	root := t.TempDir()

	on := New(root, Config{CommandTimeout: 30 * time.Second, MaxOutputBytes: 1 << 16, CargoAgentToolEnabled: true}, nil)
	out, err := on.Run(context.Background(), ToolRunCommand, map[string]any{"command": "cargo build"})
	if err != nil {
		t.Fatalf("enabled cargo build should execute: %v", err)
	}
	if !strings.Contains(out, "fake-cargo-output") {
		t.Fatalf("expected fake cargo output, got %q", out)
	}

	// An unsupported form is refused even when the permission is on.
	if _, err := on.Run(context.Background(), ToolRunCommand, map[string]any{"command": "cargo publish"}); !errors.Is(err, errCommandNotAllowed) {
		t.Fatalf("cargo publish should be refused, got %v", err)
	}

	off := New(root, Config{CommandTimeout: 30 * time.Second, MaxOutputBytes: 1 << 16}, nil)
	if _, err := off.Run(context.Background(), ToolRunCommand, map[string]any{"command": "cargo build"}); !errors.Is(err, errCommandNotAllowed) {
		t.Fatalf("disabled cargo build should be refused with errCommandNotAllowed, got %v", err)
	}
}
