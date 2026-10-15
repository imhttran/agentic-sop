package durablefs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileRoundTripCreatesDirsAndOverwrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "file.json")
	if err := WriteFile(path, []byte("one")); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "one" {
		t.Fatalf("read = %q err=%v, want %q", got, err, "one")
	}
	// A second write replaces the content atomically.
	if err := WriteFile(path, []byte("two")); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "two" {
		t.Fatalf("read = %q err=%v, want %q", got, err, "two")
	}
	// No temp files are left behind.
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "file.json" {
		t.Fatalf("leftover files in %s: %v", filepath.Dir(path), entries)
	}
}

func TestWriteFileFailsOnDirectoryTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "as-dir")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WriteFile(target, []byte("x")); err == nil {
		t.Fatal("expected an error writing durably over a directory")
	}
}
