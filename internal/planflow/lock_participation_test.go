package planflow

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/projectlock"
)

// TestPrepareWaitsForProjectLock proves initial plan creation participates in the
// project lock: while another writer holds it, Prepare cannot reach its writes.
func TestPrepareWaitsForProjectLock(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "PLAN.md", planDoc)

	held, err := projectlock.Acquire(context.Background(), dir)
	if err != nil {
		t.Fatalf("hold lock: %v", err)
	}
	defer held.Release()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err = Prepare(ctx, Options{Dir: dir, PlanSource: filepath.Join(dir, "PLAN.md"), Store: &fakeStore{}})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Prepare err = %v, want context.DeadlineExceeded while the lock was held", err)
	}
}

// TestReconcileWaitsForProjectLock proves reconciliation participates.
func TestReconcileWaitsForProjectLock(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", planDoc)
	write(t, dir, "PLAN.md", planDoc)

	held, err := projectlock.Acquire(context.Background(), dir)
	if err != nil {
		t.Fatalf("hold lock: %v", err)
	}
	defer held.Release()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err = Reconcile(ctx, ReconcileOptions{Dir: dir, PlanSource: filepath.Join(dir, "PLAN.md"), Store: st})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Reconcile err = %v, want context.DeadlineExceeded while the lock was held", err)
	}
}

// TestPrepareReleasesProjectLock proves a completed operation leaves no lock held.
func TestPrepareReleasesProjectLock(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", planDoc)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	l, err := projectlock.Acquire(ctx, dir)
	if err != nil {
		t.Fatalf("acquire after Prepare: %v", err)
	}
	l.Release()
	_ = st
}

// TestConcurrentPrepareSerializes proves two concurrent creators serialize through
// the lock and leave one coherent graph rather than racing writes.
func TestConcurrentPrepareSerializes(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "PLAN.md", planDoc)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = Prepare(context.Background(), Options{
				Dir:        dir,
				PlanSource: filepath.Join(dir, "PLAN.md"),
				Store:      &fakeStore{},
			})
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent Prepare %d: %v", i, err)
		}
	}

	// The lock is free afterwards and the plan files are coherent.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	l, err := projectlock.Acquire(ctx, dir)
	if err != nil {
		t.Fatalf("lock not released: %v", err)
	}
	l.Release()
	if _, ok := loadPlan(filepath.Join(dir, ".agent-sdlc", "plan.json")); !ok {
		t.Fatal("plan.json missing after concurrent creation")
	}
}
