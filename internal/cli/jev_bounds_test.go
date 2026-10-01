package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestJEVContextBoundsFromEnv proves the task-file context bounds can be raised or
// lowered by environment and that an unset, blank, non-numeric, or non-positive value
// keeps the built-in default, so behavior is unchanged when they are not set.
func TestJEVContextBoundsFromEnv(t *testing.T) {
	filesCases := []struct {
		name string
		val  string
		want int
	}{
		{"unset", "", maxJEVContextFiles},
		{"blank", "   ", maxJEVContextFiles},
		{"valid", "40", 40},
		{"non-numeric", "many", maxJEVContextFiles},
		{"zero", "0", maxJEVContextFiles},
		{"negative", "-1", maxJEVContextFiles},
	}
	for _, tc := range filesCases {
		t.Run("files/"+tc.name, func(t *testing.T) {
			t.Setenv(envJEVContextFiles, tc.val)
			if got := jevContextFiles(); got != tc.want {
				t.Errorf("jevContextFiles() = %d, want %d", got, tc.want)
			}
		})
	}

	t.Run("file bytes", func(t *testing.T) {
		t.Setenv(envJEVContextFileBytes, "65536")
		if got := jevContextFileBytes(); got != 65536 {
			t.Errorf("jevContextFileBytes() = %d, want 65536", got)
		}
		t.Setenv(envJEVContextFileBytes, "nope")
		if got := jevContextFileBytes(); got != maxJEVContextFileBytes {
			t.Errorf("invalid file bytes = %d, want the default %d", got, maxJEVContextFileBytes)
		}
	})

	t.Run("total bytes", func(t *testing.T) {
		t.Setenv(envJEVContextBytes, "262144")
		if got := jevContextBytes(); got != 262144 {
			t.Errorf("jevContextBytes() = %d, want 262144", got)
		}
		t.Setenv(envJEVContextBytes, "0")
		if got := jevContextBytes(); got != maxJEVContextBytes {
			t.Errorf("zero total = %d, want the default %d", got, maxJEVContextBytes)
		}
	})
}

// TestJEVContextBoundActuallyLimitsExcerpts proves the bound is used, not merely
// computed: lowering SOP_JEV_CONTEXT_FILES to 1 excerpts only the first file.
func TestJEVContextBoundActuallyLimitsExcerpts(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.md"), []byte("alpha content"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.md"), []byte("bravo content"), 0o644); err != nil {
		t.Fatal(err)
	}
	files := []string{"a.md", "b.md"}

	// The default bound excerpts both files.
	if got := taskFileContext(dir, files); !strings.Contains(got, "### a.md") || !strings.Contains(got, "### b.md") {
		t.Fatalf("default context = %q, want both files", got)
	}

	t.Setenv(envJEVContextFiles, "1")
	got := taskFileContext(dir, files)
	if !strings.Contains(got, "### a.md") {
		t.Errorf("lowered context = %q, want the first file", got)
	}
	if strings.Contains(got, "### b.md") {
		t.Errorf("lowered context = %q: the second file is beyond the files bound", got)
	}
}
