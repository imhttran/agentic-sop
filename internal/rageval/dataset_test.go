package rageval

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixtureCases are the small self-contained datasets under testdata/.
// Each case names the fixture directory, the error class Load must return,
// and substrings the error message must contain.
var fixtureCases = []struct {
	name      string
	wantClass error
	wantSubs  []string
}{
	{
		name:      "tampered-hash",
		wantClass: ErrHashMismatch,
		wantSubs:  []string{"doc1.md"},
	},
	{
		name:      "missing-field",
		wantClass: ErrMissingField,
		wantSubs:  []string{"question"},
	},
	{
		name:      "unknown-category",
		wantClass: ErrUnknownCategory,
		wantSubs:  []string{"bogus", "Q-BAD"},
	},
	{
		name:      "under-minimum",
		wantClass: ErrUnderMinimum,
		wantSubs:  []string{"no_answer", "have 0"},
	},
}

func TestLoadRejectsInvalidFixtures(t *testing.T) {
	for _, tc := range fixtureCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join("testdata", tc.name)
			_, err := Load(dir)
			if err == nil {
				t.Fatalf("Load(%s) = nil error, want %v", dir, tc.wantClass)
			}
			if !errors.Is(err, tc.wantClass) {
				t.Fatalf("Load(%s) error = %v, want class %v", dir, err, tc.wantClass)
			}
			for _, sub := range tc.wantSubs {
				if !strings.Contains(err.Error(), sub) {
					t.Errorf("Load(%s) error %q does not contain %q", dir, err, sub)
				}
			}
		})
	}
}

func TestLoadValidFixture(t *testing.T) {
	ds, err := Load(filepath.Join("testdata", "valid"))
	if err != nil {
		t.Fatalf("Load(valid) error = %v, want nil", err)
	}
	if len(ds.Questions) == 0 {
		t.Fatalf("Load(valid) returned no questions")
	}
	for _, c := range Categories {
		min := H1Minimums[c]
		if got := ds.Counts[c]; got < min {
			t.Errorf("category %q count = %d, want >= %d", c, got, min)
		}
	}
}

// TestCommittedDatasetValid loads the committed evals/rag dataset from the
// repository and asserts the manifest hashes match and every category meets
// its H-1 minimum. It fails clearly if the committed directory is absent.
func TestCommittedDatasetValid(t *testing.T) {
	root := repoRoot(t)
	dir := filepath.Join(root, "evals", "rag")
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("committed dataset directory %s is missing or not a directory: %v", dir, err)
	}
	ds, err := Load(dir)
	if err != nil {
		t.Fatalf("Load(%s) failed: %v", dir, err)
	}
	if len(ds.Manifest) == 0 {
		t.Fatalf("committed manifest under %s has no entries", dir)
	}
	for _, c := range Categories {
		min := H1Minimums[c]
		if got := ds.Counts[c]; got < min {
			t.Errorf("committed category %q count = %d, below H-1 minimum %d", c, got, min)
		}
	}
}

// repoRoot walks up from the package directory until it finds go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not locate go.mod above %s", dir)
		}
		dir = parent
	}
}
