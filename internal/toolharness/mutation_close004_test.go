package toolharness

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestObservedCreateInNewDirectoryIsCredited reproduces the CLOSE-004 shape: a
// declared deliverable under a directory that does not exist yet. Path resolution
// must accept the new path (via its deepest existing ancestor) and the completed
// create must be observed as a verified change, so a successful write contributes
// progress instead of a false NO_PROGRESS.
func TestObservedCreateInNewDirectoryIsCredited(t *testing.T) {
	root := t.TempDir()
	h := newTestHarness(t, root, nil)
	rel := "docs/reports/pre-performance-closure/CLOSE-004-controller-deterministic-baseline.md"
	_, observation, err := h.RunObservedMutation(context.Background(), ToolCreateFile, map[string]any{
		"path":    rel,
		"content": "# baseline\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !observation.Succeeded || !observation.Verified || !observation.Changed {
		t.Fatalf("observation=%+v, want a credited create in a new directory", observation)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
		t.Fatalf("deliverable not written: %v", err)
	}
}

// TestObservedMutationMissingPathIsNotCredited pins the conservative side: a file
// mutation whose arguments do not reach the tool never counts and is never
// reported as a verified change.
func TestObservedMutationMissingPathIsNotCredited(t *testing.T) {
	h := newTestHarness(t, t.TempDir(), nil)
	_, observation, err := h.RunObservedMutation(context.Background(), ToolCreateFile, map[string]any{"content": "c"})
	if err == nil {
		t.Fatal("a create with no path must fail")
	}
	if observation.Succeeded || observation.Verified || observation.Changed {
		t.Fatalf("observation=%+v, want no credit for a refused operation", observation)
	}
}
