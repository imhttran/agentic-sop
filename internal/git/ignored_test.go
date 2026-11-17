package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// writeRepoFile creates a repository-relative file, including parent directories.
func writeRepoFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, name, content)
}

// TestIgnoredPathsIsDisjointFromSnapshotFiles is the contract the security
// adapters rely on: an ignored path is never in-scope content, and a tracked path
// is never ignored, even when an ignore rule matches it.
func TestIgnoredPathsIsDisjointFromSnapshotFiles(t *testing.T) {
	dir := initRepo(t)
	writeRepoFile(t, dir, ".gitignore", "ignored/\n*.gen.go\n")
	// Tracked despite both ignore rules.
	writeRepoFile(t, dir, "tracked.gen.go", "package main\n")
	writeRepoFile(t, dir, filepath.Join("ignored", "tracked.go"), "package main\n")
	// Ignored and untracked.
	writeRepoFile(t, dir, "untracked.gen.go", "package main\n")
	writeRepoFile(t, dir, filepath.Join("ignored", "other.go"), "package main\n")
	runGit(t, dir, "add", ".gitignore")
	runGit(t, dir, "add", "-f", "tracked.gen.go", filepath.Join("ignored", "tracked.go"))
	runGit(t, dir, "commit", "-m", "ignore rules")

	ignored, err := New(dir).IgnoredPaths(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := New(dir).SnapshotFiles(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	inScope := make(map[string]bool, len(snapshot))
	for _, path := range snapshot {
		inScope[path] = true
	}
	for _, path := range ignored {
		if inScope[path] {
			t.Errorf("ignored path %q is also in the snapshot", path)
		}
	}
	for _, path := range []string{"tracked.gen.go", filepath.Join("ignored", "tracked.go")} {
		if !inScope[path] {
			t.Errorf("tracked path %q is not in the snapshot (ignored=%v)", path, ignored)
		}
		if containsString(ignored, path) {
			t.Errorf("tracked path %q was reported as ignored", path)
		}
	}
	for _, path := range []string{"untracked.gen.go", filepath.Join("ignored", "other.go")} {
		if !containsString(ignored, path) {
			t.Errorf("ignored path %q was not reported (ignored=%v)", path, ignored)
		}
	}
}

// TestIgnoredPathsCollapsesIgnoredDirectories proves a fully ignored directory is
// one entry, not one entry per file, so the result stays bounded.
func TestIgnoredPathsCollapsesIgnoredDirectories(t *testing.T) {
	dir := initRepo(t)
	writeRepoFile(t, dir, ".gitignore", "build/\n*.log\n")
	for _, name := range []string{"a.go", "b.go", "nested/c.go"} {
		writeRepoFile(t, dir, filepath.Join("build", filepath.FromSlash(name)), "package main\n")
	}
	writeRepoFile(t, dir, "root.log", "log\n")

	ignored, err := New(dir).IgnoredPaths(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !containsString(ignored, "build/") {
		t.Errorf("ignored directory was not collapsed to one entry: %v", ignored)
	}
	if containsString(ignored, "build/a.go") || containsString(ignored, "build/nested/c.go") {
		t.Errorf("files of a collapsed ignored directory were listed individually: %v", ignored)
	}
	if !containsString(ignored, "root.log") {
		t.Errorf("ignored root file was not reported: %v", ignored)
	}
}

// TestIgnoredPathsPreservesSpaces proves NUL-separated output keeps a path that
// contains a space intact instead of splitting it.
func TestIgnoredPathsPreservesSpaces(t *testing.T) {
	dir := initRepo(t)
	writeRepoFile(t, dir, ".gitignore", "with space/\n")
	writeRepoFile(t, dir, filepath.Join("with space", "a.go"), "package main\n")

	ignored, err := New(dir).IgnoredPaths(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !containsString(ignored, "with space/") {
		t.Errorf("path with a space was not preserved: %v", ignored)
	}
}

// TestIgnoredPathsFailsOutsideRepository proves an unavailable ignore database is
// an error, never an empty set.
func TestIgnoredPathsFailsOutsideRepository(t *testing.T) {
	if _, err := New(t.TempDir()).IgnoredPaths(context.Background()); err == nil {
		t.Fatal("IgnoredPaths succeeded outside a repository, want an error")
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
