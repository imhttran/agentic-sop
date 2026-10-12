package archtest

import (
	"bytes"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// This file is the frozen gofmt formatting guard for the module.
//
// The invariant it enforces: every Go file in the module is gofmt-formatted, so an
// ordinary `go test ./...` fails, naming the offending files, when one is not. It is
// stdlib-only (go/format), offline, and CI-safe: it needs no external gofmt binary, no
// network, and no credentials.
//
// The walk skips directory trees that are not module source: `testdata` (Go's
// conventional fixture directory, which may deliberately contain malformed or
// unformatted files) and `.agent-sdlc` (tool-owned workflow state).

// skippedDirs are directory names the gofmt walk does not descend into.
var skippedDirs = map[string]bool{
	"testdata":    true,
	".agent-sdlc": true,
}

// unformattedGoFiles walks root for `.go` files (including `_test.go` files), skipping
// directories named in skippedDirs, and returns the paths of files whose content differs
// from canonical gofmt output. Returned paths may be absolute or root-relative; callers
// that compare against a fixed set should normalize them first.
func unformattedGoFiles(root string) ([]string, error) {
	var unformatted []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skippedDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		formatted, err := format.Source(src)
		if err != nil {
			// A file that cannot be parsed or formatted is reported as unformatted so
			// the guard never silently accepts a malformed Go file.
			unformatted = append(unformatted, path)
			return nil
		}
		if !bytes.Equal(src, formatted) {
			unformatted = append(unformatted, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(unformatted)
	return unformatted, nil
}

// TestRepositoryIsGofmtFormatted fails when any module .go file is not gofmt-formatted,
// naming the offending files. It derives the repository root via moduleRoot, so it
// evaluates the real module tree under ordinary `go test ./...`.
func TestRepositoryIsGofmtFormatted(t *testing.T) {
	root := moduleRoot(t)
	unformatted, err := unformattedGoFiles(root)
	if err != nil {
		t.Fatalf("scanning %s for gofmt differences: %v", root, err)
	}
	for _, path := range unformatted {
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		t.Errorf("%s is not gofmt-formatted; run `gofmt -w %s`", filepath.ToSlash(rel), filepath.ToSlash(rel))
	}
}
