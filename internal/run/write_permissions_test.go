package run

import (
	"os"
	"path/filepath"
	"testing"
)

// TestWriteArtifactIsOwnerOnly proves artifacts are 0600 even when a file of the same
// name already exists with a wider mode. Model output and diffs land here.
func TestWriteArtifactIsOwnerOnly(t *testing.T) {
	r := &Run{dir: t.TempDir()}
	path := filepath.Join(r.dir, "implementation.md")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := r.Write("implementation.md", "reply"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("artifact mode = %o, want 0600", mode)
	}
}
