package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The execution lease is off unless a project asks for it, and only the three
// documented modes are accepted.
func TestExecutionLeaseConfig(t *testing.T) {
	write := func(t *testing.T, body string) *Config {
		t.Helper()
		dir := t.TempDir()
		if err := os.MkdirAll(filepath.Join(dir, DirName), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(Path(dir), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		c, err := LoadDir(dir)
		if err != nil {
			t.Fatalf("LoadDir: %v", err)
		}
		return c
	}

	// Omitted block: the zero value, which internal/executionlease reads as off.
	omitted := write(t, "project:\n  name: x\n")
	if omitted.Execution.Lease != "" {
		t.Errorf("Execution.Lease = %q, want empty (off) when the block is omitted", omitted.Execution.Lease)
	}

	required := write(t, "project:\n  name: x\nexecution:\n  lease: required\n")
	if required.Execution.Lease != "required" {
		t.Errorf("Execution.Lease = %q, want required", required.Execution.Lease)
	}

	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, DirName), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(Path(dir), []byte("project:\n  name: x\nexecution:\n  lease: sometimes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadDir(dir)
	if err == nil || !strings.Contains(err.Error(), "execution.lease") {
		t.Fatalf("LoadDir with an unknown mode = %v, want a validation error naming execution.lease", err)
	}
}
