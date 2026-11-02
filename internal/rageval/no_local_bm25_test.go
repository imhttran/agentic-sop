package rageval

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoLocalBM25 proves the package holds no BM25 implementation of its own.
// Ranking goes through internal/retrieval, so a BM25 scorer here would be a
// second, diverging implementation.
func TestNoLocalBM25(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if name == "bm25.go" {
			t.Fatalf("internal/rageval must not hold a BM25 implementation: found %s", name)
		}
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		src := string(raw)
		for _, banned := range []string{"NewBM25", "BM25 struct", "func tokenize(", "k1, b float64"} {
			if strings.Contains(src, banned) {
				t.Fatalf("internal/rageval/%s contains a local BM25 implementation marker %q", name, banned)
			}
		}
	}
}
