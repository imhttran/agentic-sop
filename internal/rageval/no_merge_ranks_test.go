package rageval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// assertNoMergeRanks fails if any non-test source file in internal/rageval
// declares a mergeRanks function. Arm B must merge through sopctx.Build, never
// by hand.
func assertNoMergeRanks(t *testing.T) {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if strings.Contains(string(raw), "func mergeRanks(") {
			t.Fatalf("internal/rageval/%s declares mergeRanks; arm B must merge through sopctx.Build", name)
		}
	}
}

// TestNoMergeRanks locks the property that internal/rageval holds no
// hand-written rank merge. Arm B merges only through sopctx.Build.
func TestNoMergeRanks(t *testing.T) {
	assertNoMergeRanks(t)
}
