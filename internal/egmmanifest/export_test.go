package egmmanifest_test

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/imhttran/agentic-sop/internal/egmmanifest"
)

func TestReviewerExportExcludesPrivate(t *testing.T) {
	m, _ := newFixture(t)
	b, err := egmmanifest.ReviewerExport(m)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{syntheticAnswer, "missing-failure-mode", "synthetic distractor", "independent check recorded", "authored_by", "sealed_sha256"} {
		if bytes.Contains(b, []byte(secret)) {
			t.Fatalf("reviewer export contains private value %q", secret)
		}
	}
}

func TestReviewerExportDeterministic(t *testing.T) {
	m, _ := newFixture(t)
	a, _ := egmmanifest.ReviewerExport(m)
	b, _ := egmmanifest.ReviewerExport(m)
	if !bytes.Equal(a, b) {
		t.Fatal("repeated exports differ")
	}
}

func TestReviewerExportDigestIgnoresPrivateChanges(t *testing.T) {
	m, _ := newFixture(t)
	before, err := egmmanifest.ReviewerExportDigest(m)
	if err != nil {
		t.Fatal(err)
	}
	m.Cases[0].DeterminabilityRationale = "a different private rationale"
	m.Cases[0].ExpectedFinding.Location = "a different private location"
	m.Cases[0].Distractors = []string{"a different distractor"}
	after, err := egmmanifest.ReviewerExportDigest(m)
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Fatal("a private-field change altered the reviewer export")
	}
}

func TestReviewerExportDigestPublicChanges(t *testing.T) {
	m, _ := newFixture(t)
	before, _ := egmmanifest.ReviewerExportDigest(m)
	m.Cases[0].RevisionPin = "a-different-pin"
	after, _ := egmmanifest.ReviewerExportDigest(m)
	if before == after {
		t.Fatal("a public-field change did not alter the export digest")
	}
}

func TestTreeHashDeterministic(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.md"), "a")
	writeFile(t, filepath.Join(dir, "sub", "b.md"), "b")
	h1, err := egmmanifest.TreeHash(dir)
	if err != nil {
		t.Fatal(err)
	}
	h2, _ := egmmanifest.TreeHash(dir)
	if h1 != h2 {
		t.Fatal("repeated tree hashes differ")
	}
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	h3, _ := egmmanifest.TreeHash(dir)
	if h1 == h3 {
		t.Fatal("a changed file did not change the tree hash")
	}
}

func TestTreeHashSkips(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.md"), "a")
	writeFile(t, filepath.Join(dir, ".git", "HEAD"), "one")
	h1, err := egmmanifest.TreeHash(dir, ".git")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("two"), 0o600); err != nil {
		t.Fatal(err)
	}
	h2, _ := egmmanifest.TreeHash(dir, ".git")
	if h1 != h2 {
		t.Fatal("a skipped file changed the tree hash")
	}
}

func TestTreeHashRejectsSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation needs privileges on Windows")
	}
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.md"), "a")
	if err := os.Symlink(filepath.Join(dir, "a.md"), filepath.Join(dir, "link")); err != nil {
		t.Skip("symlink unsupported:", err)
	}
	if _, err := egmmanifest.TreeHash(dir); err == nil {
		t.Fatal("symlink accepted in an exported tree")
	}
}
