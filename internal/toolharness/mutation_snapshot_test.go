package toolharness

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// snapshotRepo initialises a Git repository containing one committed source
// file, so the snapshot has a real baseline to compare against.
func snapshotRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitInitRepo(t, dir)
	writeRepoFile(t, dir, "src/main.go", "package main\n")
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "initial")
	return dir
}

// TestSnapshotExcludesIgnoredBuildOutput proves ignored generated output is not
// selected (the failure mode that made the IMPLEMENT baseline hash gigabytes of
// Cargo artifacts), while tracked source is still present.
func TestSnapshotExcludesIgnoredBuildOutput(t *testing.T) {
	dir := snapshotRepo(t)
	writeRepoFile(t, dir, ".gitignore", "target/\n")
	runGit(t, dir, "add", ".gitignore")
	runGit(t, dir, "commit", "-q", "-m", "ignore target")

	writeRepoFile(t, dir, "target/debug/heavy.bin", strings.Repeat("x", 1<<20))

	paths, err := New(dir, DefaultConfig(), nil).RepositoryPathFingerprints(context.Background())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if _, ok := paths["target/debug/heavy.bin"]; ok {
		t.Fatal("ignored build output was included in the snapshot")
	}
	for _, want := range []string{"src/main.go", ".gitignore"} {
		if _, ok := paths[want]; !ok {
			t.Errorf("snapshot missing %s", want)
		}
	}
}

// TestSnapshotIncludesTrackedButIgnoredFiles proves a file Git tracks remains
// part of the snapshot even after an ignore rule starts matching it.
func TestSnapshotIncludesTrackedButIgnoredFiles(t *testing.T) {
	dir := t.TempDir()
	gitInitRepo(t, dir)
	writeRepoFile(t, dir, "generated/keep.txt", "tracked content\n")
	runGit(t, dir, "add", "generated/keep.txt")
	runGit(t, dir, "commit", "-q", "-m", "track generated file")
	writeRepoFile(t, dir, ".gitignore", "generated/\n")
	runGit(t, dir, "add", ".gitignore")
	runGit(t, dir, "commit", "-q", "-m", "ignore generated")

	paths, err := New(dir, DefaultConfig(), nil).RepositoryPathFingerprints(context.Background())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if _, ok := paths["generated/keep.txt"]; !ok {
		t.Fatal("tracked-but-ignored file was dropped from the snapshot")
	}
}

// TestSnapshotIncludesUntrackedSource proves non-ignored untracked source (for
// example a file the agent just created) is observed.
func TestSnapshotIncludesUntrackedSource(t *testing.T) {
	dir := snapshotRepo(t)
	writeRepoFile(t, dir, "src/feature.go", "package main\n\nfunc Feature() {}\n")

	paths, err := New(dir, DefaultConfig(), nil).RepositoryPathFingerprints(context.Background())
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if _, ok := paths["src/feature.go"]; !ok {
		t.Fatal("untracked source file was not included in the snapshot")
	}
}

// TestSnapshotDetectsSourceModification proves before/after fingerprints still
// identify a mutated tracked file while leaving untouched files stable.
func TestSnapshotDetectsSourceModification(t *testing.T) {
	dir := snapshotRepo(t)
	writeRepoFile(t, dir, "src/other.go", "package main\n")
	h := New(dir, DefaultConfig(), nil)
	ctx := context.Background()

	before, err := h.RepositoryPathFingerprints(ctx)
	if err != nil {
		t.Fatalf("baseline snapshot: %v", err)
	}
	writeRepoFile(t, dir, "src/main.go", "package main\n\nfunc main() {}\n")
	after, err := h.RepositoryPathFingerprints(ctx)
	if err != nil {
		t.Fatalf("after snapshot: %v", err)
	}

	if before["src/main.go"] == after["src/main.go"] {
		t.Fatal("source modification was not detected")
	}
	if before["src/other.go"] != after["src/other.go"] {
		t.Fatal("unmodified source changed its fingerprint")
	}
}

// TestSnapshotDoesNotFollowSymlinksOutsideRepository proves a tracked symlink is
// observed as a link (its target string), not dereferenced, so content outside
// the repository never enters the snapshot.
func TestSnapshotDoesNotFollowSymlinksOutsideRepository(t *testing.T) {
	dir := t.TempDir()
	gitInitRepo(t, dir)
	outside := t.TempDir()
	writeRepoFile(t, outside, "secret.txt", "outside secret\n")
	if err := os.Symlink(outside, filepath.Join(dir, "escape")); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-q", "-m", "tracked symlink")

	h := New(dir, DefaultConfig(), nil)
	ctx := context.Background()
	before, err := h.RepositoryPathFingerprints(ctx)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if _, ok := before["escape"]; !ok {
		t.Fatal("tracked symlink was not observed")
	}
	if _, ok := before["escape/secret.txt"]; ok {
		t.Fatal("snapshot followed a symlink outside the repository")
	}

	writeRepoFile(t, outside, "secret.txt", "changed outside secret\n")
	after, err := h.RepositoryPathFingerprints(ctx)
	if err != nil {
		t.Fatalf("second snapshot: %v", err)
	}
	if before["escape"] != after["escape"] {
		t.Fatal("snapshot fingerprint changed when only the outside target changed")
	}
}

// TestSnapshotPathRejectsEscapes proves a hostile or corrupt Git listing cannot
// widen the observation boundary, including through a symlinked parent.
func TestSnapshotPathRejectsEscapes(t *testing.T) {
	root := t.TempDir()
	h := New(root, DefaultConfig(), nil)
	bad := []string{"", "../outside.txt", "a/../../outside.txt"}
	if abs := filepath.Join(string(filepath.Separator), "abs", "outside.txt"); filepath.IsAbs(abs) {
		bad = append(bad, abs)
	}
	for _, listed := range bad {
		if _, _, err := h.snapshotPath(listed); err == nil {
			t.Errorf("snapshotPath(%q) accepted an escaping path", listed)
		}
	}

	outside := t.TempDir()
	writeRepoFile(t, outside, "secret.txt", "outside secret\n")
	if err := os.Symlink(outside, filepath.Join(root, "leak")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := h.snapshotPath("leak/secret.txt"); err == nil {
		t.Fatal("snapshotPath followed a symlinked parent outside the repository")
	}
}

// TestSnapshotFailsClosedWithoutGitRepository proves a directory that is not a
// Git working tree produces an unavailable observation, never an empty
// successful snapshot that would prove nothing changed.
func TestSnapshotFailsClosedWithoutGitRepository(t *testing.T) {
	root := t.TempDir()
	writeRepoFile(t, root, "a.go", "package a\n")

	paths, err := New(root, DefaultConfig(), nil).RepositoryPathFingerprints(context.Background())
	if err == nil {
		t.Fatal("non-Git directory produced a successful snapshot")
	}
	if !errors.Is(err, ErrSnapshotUnavailable) {
		t.Fatalf("error = %v, want ErrSnapshotUnavailable", err)
	}
	if paths != nil {
		t.Fatalf("failed snapshot returned paths: %v", paths)
	}
}

// TestSnapshotFailsClosedOnGitDiscoveryFailure proves corrupt Git metadata is
// reported as an unavailable snapshot rather than silently yielding an empty one.
func TestSnapshotFailsClosedOnGitDiscoveryFailure(t *testing.T) {
	root := t.TempDir()
	writeRepoFile(t, root, "a.go", "package a\n")
	writeRepoFile(t, root, ".git", "this is not a valid gitfile\n")

	paths, err := New(root, DefaultConfig(), nil).RepositoryPathFingerprints(context.Background())
	if err == nil {
		t.Fatal("corrupt Git metadata produced a successful snapshot")
	}
	if !errors.Is(err, ErrSnapshotUnavailable) {
		t.Fatalf("error = %v, want ErrSnapshotUnavailable", err)
	}
	if paths != nil {
		t.Fatalf("failed snapshot returned paths: %v", paths)
	}
}
