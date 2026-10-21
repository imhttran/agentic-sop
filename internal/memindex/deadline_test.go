package memindex

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestMemoryCanceledOperations(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := OpenContext(ctx, filepath.Join(t.TempDir(), "index.db")); !errors.Is(err, context.Canceled) {
		t.Fatalf("open: %v", err)
	}
	if _, err := SnapshotContext(ctx, t.TempDir()); !errors.Is(err, context.Canceled) {
		t.Fatalf("snapshot: %v", err)
	}
	ix := openTestIndex(t)
	if _, err := ix.RebuildFromDir(ctx, t.TempDir()); !errors.Is(err, context.Canceled) {
		t.Fatalf("rebuild: %v", err)
	}
}
