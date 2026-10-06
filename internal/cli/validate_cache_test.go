package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunValidateCacheHitsOnUnchangedTree proves sop validate --cache records a
// verification and reuses it for an identical identity.
func TestRunValidateCacheHitsOnUnchangedTree(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")

	code, out1, err1 := runCLI(t, dir, "validate", "--cache")
	if code != exitOK {
		t.Fatalf("first validate: code=%d stdout=%s stderr=%s", code, out1, err1)
	}
	if strings.Contains(out1, "cached") {
		t.Errorf("the first run must not be a cache hit: %s", out1)
	}
	if _, err := os.Stat(filepath.Join(dir, stateDirName, "context", "verify-cache.json")); err != nil {
		t.Errorf("verification cache artifact missing: %v", err)
	}

	code, out2, err2 := runCLI(t, dir, "validate", "--cache")
	if code != exitOK {
		t.Fatalf("second validate: code=%d stdout=%s stderr=%s", code, out2, err2)
	}
	if !strings.Contains(out2, "(cached)") {
		t.Errorf("the second run must be a cache hit: %s", out2)
	}
}

// TestRunValidateWithoutCacheNeverCaches proves the cache is opt-in.
func TestRunValidateWithoutCacheNeverCaches(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")

	if _, out, _ := runCLI(t, dir, "validate"); strings.Contains(out, "cached") {
		t.Errorf("validation must not cache without --cache: %s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, stateDirName, "context", "verify-cache.json")); err == nil {
		t.Errorf("no verification cache artifact may be written without --cache")
	}
}
