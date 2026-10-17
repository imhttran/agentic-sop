package git

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFileDeep writes a file at a slash-separated path, creating parent
// directories. The package helper writeFile writes a flat name only.
func writeFileDeep(t *testing.T, dir, name, content string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", name, err)
	}
	writeFile(t, dir, name, content)
}

// TestHeadFileReadsCommittedContentNotWorkingTree proves the read-only committed
// boundary reads HEAD, never the working tree, so a staged or unstaged
// modification of the same path can never be presented as committed content.
func TestHeadFileReadsCommittedContentNotWorkingTree(t *testing.T) {
	dir := initRepo(t)
	committed := "package a\n\nconst V = 1\n"
	writeFileDeep(t, dir, "src/a.go", committed)
	runGit(t, dir, "add", "src/a.go")
	runGit(t, dir, "commit", "-m", "add a")

	entry, data, err := New(dir).HeadFile(context.Background(), "src/a.go", 1<<20)
	if err != nil {
		t.Fatalf("HeadFile: %v", err)
	}
	if entry.Mode != "100644" {
		t.Errorf("mode = %q, want 100644", entry.Mode)
	}
	if entry.Size != int64(len(committed)) {
		t.Errorf("size = %d, want %d", entry.Size, len(committed))
	}
	if !isHexObjectID(entry.SHA) {
		t.Errorf("SHA = %q, want a well-formed object id", entry.SHA)
	}
	if string(data) != committed {
		t.Fatalf("content = %q, want the committed content", data)
	}

	// An unstaged modification must not appear.
	writeFileDeep(t, dir, "src/a.go", "package a\n\nconst V = 2\n")
	if _, data, err = New(dir).HeadFile(context.Background(), "src/a.go", 1<<20); err != nil {
		t.Fatalf("HeadFile after unstaged edit: %v", err)
	}
	if string(data) != committed {
		t.Errorf("unstaged modification leaked into committed content: %q", data)
	}

	// A staged modification must not appear either.
	writeFileDeep(t, dir, "src/a.go", "package a\n\nconst V = 3\n")
	runGit(t, dir, "add", "src/a.go")
	if _, data, err = New(dir).HeadFile(context.Background(), "src/a.go", 1<<20); err != nil {
		t.Fatalf("HeadFile after staged edit: %v", err)
	}
	if string(data) != committed {
		t.Errorf("staged modification leaked into committed content: %q", data)
	}
}

// TestHeadFileRejectsNonRegularAndUnsafePaths proves only a committed regular
// file is ever read: an absent or untracked path, a directory, a symbolic link, a
// path that escapes the repository, and an empty path are all refused.
func TestHeadFileRejectsNonRegularAndUnsafePaths(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "untracked.go", "package x\n")
	writeFileDeep(t, dir, "pkg/b.go", "package pkg\n")
	if err := os.Symlink("README.md", filepath.Join(dir, "link.md")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	runGit(t, dir, "add", "pkg/b.go", "link.md")
	runGit(t, dir, "commit", "-m", "add directory file and symlink")

	for _, path := range []string{"untracked.go", "pkg", "link.md", "nope.go", "../escape.go", "/etc/passwd", ""} {
		if _, data, err := New(dir).HeadFile(context.Background(), path, 1<<20); err == nil || data != nil {
			t.Errorf("path %q was accepted as committed content (data=%q, err=%v)", path, data, err)
		}
		if _, err := New(dir).HeadEntry(context.Background(), path); !errors.Is(err, ErrHeadPathAbsent) && path != "../escape.go" && path != "/etc/passwd" && path != "" {
			t.Errorf("HeadEntry(%q) err = %v, want ErrHeadPathAbsent", path, err)
		}
	}
}

// TestHeadFileRefusesOversizedBlob proves the size limit is enforced before the
// blob is read, so a caller can bound its own context without allocating it.
func TestHeadFileRefusesOversizedBlob(t *testing.T) {
	dir := initRepo(t)
	big := strings.Repeat("x", 4096)
	writeFile(t, dir, "big.txt", big)
	runGit(t, dir, "add", "big.txt")
	runGit(t, dir, "commit", "-m", "big")

	entry, data, err := New(dir).HeadFile(context.Background(), "big.txt", 1024)
	if !errors.Is(err, ErrHeadFileTooLarge) {
		t.Fatalf("err = %v, want ErrHeadFileTooLarge", err)
	}
	if data != nil {
		t.Errorf("content was read despite the limit: %q", data)
	}
	if entry.Size != int64(len(big)) {
		t.Errorf("size = %d, want %d", entry.Size, len(big))
	}
}

// TestLastCommitProvenanceIsDescriptive proves the provenance helper returns a
// full commit id and subject for a committed path and an empty, non-error result
// for an uncommitted one.
func TestLastCommitProvenanceIsDescriptive(t *testing.T) {
	dir := initRepo(t)
	writeFile(t, dir, "a.go", "package a\n")
	runGit(t, dir, "add", "a.go")
	runGit(t, dir, "commit", "-m", "add a.go")

	got, err := New(dir).LastCommit(context.Background(), "a.go")
	if err != nil {
		t.Fatalf("LastCommit: %v", err)
	}
	fields := strings.SplitN(got, " ", 2)
	if len(fields) != 2 || len(fields[0]) != 40 {
		t.Fatalf("provenance = %q, want a full commit id and a subject", got)
	}
	if fields[1] != "add a.go" {
		t.Errorf("subject = %q, want %q", fields[1], "add a.go")
	}

	got, err = New(dir).LastCommit(context.Background(), "nope.go")
	if err != nil || got != "" {
		t.Errorf("unknown path: got %q, err %v; want an empty, non-error result", got, err)
	}
}

// TestIsHexObjectID proves only a well-formed lowercase hexadecimal object id is
// accepted, so a malformed revision can never be handed to cat-file.
func TestIsHexObjectID(t *testing.T) {
	valid := []string{
		strings.Repeat("a", 40),                    // SHA-1
		strings.Repeat("0", 40),                    // SHA-1, all digits
		strings.Repeat("f", 40),                    // SHA-1, all letters
		"0123456789abcdef0123456789abcdef01234567", // SHA-1, mixed
		strings.Repeat("a", 64),                    // SHA-256
		"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", // SHA-256, mixed
	}
	for _, s := range valid {
		if !isHexObjectID(s) {
			t.Errorf("isHexObjectID(%q) = false, want true", s)
		}
	}

	invalid := []string{
		"",                                       // empty
		"abc",                                    // far too short
		strings.Repeat("a", 39),                  // one short of SHA-1
		strings.Repeat("a", 41),                  // one over SHA-1
		strings.Repeat("a", 63),                  // one short of SHA-256
		strings.Repeat("a", 65),                  // one over SHA-256
		strings.ToUpper(strings.Repeat("a", 40)), // uppercase
		strings.Repeat("g", 40),                  // non-hexadecimal letter
		strings.Repeat("z", 64),                  // non-hexadecimal letter, SHA-256 length
		strings.Repeat("a", 39) + " ",            // trailing space
		strings.Repeat("a", 39) + "-",            // trailing punctuation
		"-" + strings.Repeat("a", 39),            // leading option-like character
		"--batch",                                // a flag-shaped value
		"HEAD",                                   // a revision, not an object id
		strings.Repeat("1", 40) + "\n",           // embedded newline
	}
	for _, s := range invalid {
		if isHexObjectID(s) {
			t.Errorf("isHexObjectID(%q) = true, want false", s)
		}
	}
}
