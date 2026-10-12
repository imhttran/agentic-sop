//go:build acceptance

// Operator-authored acceptance check for ACC-011: the repository gofmt guard must
// actually detect an unformatted Go file (non-vacuous) and accept formatted ones.
// It exercises the guard's helper, unformattedGoFiles(root string) ([]string, error),
// which ACC-011 defines alongside TestRepositoryIsGofmtFormatted.

package archtest

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func accOp011Write(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestOperatorAcceptanceACC011GuardDetectsUnformattedFiles(t *testing.T) {
	root := t.TempDir()
	accOp011Write(t, root, "ok/ok.go", "package ok\n\n// V is a value.\nconst V = 1\n")
	accOp011Write(t, root, "bad/bad.go", "package bad\n\nconst   W   =   2\n")
	accOp011Write(t, root, "bad/bad_test.go", "package bad\n\nimport \"testing\"\n\nfunc TestX(t *testing.T) {   }\n")
	accOp011Write(t, root, "testdata/ignored.go", "package   ignored\n")
	accOp011Write(t, root, ".agent-sdlc/ignored.go", "package   ignored\n")

	got, err := unformattedGoFiles(root)
	if err != nil {
		t.Fatalf("unformattedGoFiles: %v", err)
	}
	// The helper may report absolute or root-relative paths, in any order.
	var rel []string
	for _, p := range got {
		if filepath.IsAbs(p) {
			if r, err := filepath.Rel(root, p); err == nil {
				p = r
			}
		}
		rel = append(rel, filepath.ToSlash(p))
	}
	sort.Strings(rel)
	want := []string{"bad/bad.go", "bad/bad_test.go"}
	if !reflect.DeepEqual(rel, want) {
		t.Fatalf("unformatted files = %q, want %q (testdata and .agent-sdlc skipped)", rel, want)
	}
}

func TestOperatorAcceptanceACC011GuardAcceptsFormattedTree(t *testing.T) {
	root := t.TempDir()
	accOp011Write(t, root, "ok/ok.go", "package ok\n\n// V is a value.\nconst V = 1\n")
	got, err := unformattedGoFiles(root)
	if err != nil || len(got) != 0 {
		t.Fatalf("formatted tree: unformatted=%q err=%v, want none", got, err)
	}
}
