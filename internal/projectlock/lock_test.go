package projectlock

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestAcquireExcludesSecondAcquirer proves mutual exclusion: while one lock is
// held, a second acquisition does not succeed until the first is released.
func TestAcquireExcludesSecondAcquirer(t *testing.T) {
	dir := t.TempDir()
	first, err := Acquire(context.Background(), dir)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}

	acquired := make(chan struct{})
	var once sync.Once
	var secondErr error
	go func() {
		second, err := Acquire(context.Background(), dir)
		secondErr = err
		once.Do(func() { close(acquired) })
		if err == nil {
			second.Release()
		}
	}()

	select {
	case <-acquired:
		t.Fatal("second acquirer succeeded while the first lock was held")
	case <-time.After(150 * time.Millisecond):
	}

	first.Release()
	select {
	case <-acquired:
		if secondErr != nil {
			t.Fatalf("second acquire after release: %v", secondErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("second acquirer did not proceed after the lock was released")
	}
}

// TestReleaseIsIdempotent proves a deferred release after an explicit one is safe
// and does not close the descriptor twice in a way that breaks a later acquirer.
func TestReleaseIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	l, err := Acquire(context.Background(), dir)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	l.Release()
	l.Release()

	again, err := Acquire(context.Background(), dir)
	if err != nil {
		t.Fatalf("acquire after double release: %v", err)
	}
	again.Release()
}

// TestAcquireHonorsCancellation proves a contender whose context expires stops
// waiting with a context error instead of blocking indefinitely.
func TestAcquireHonorsCancellation(t *testing.T) {
	dir := t.TempDir()
	held, err := Acquire(context.Background(), dir)
	if err != nil {
		t.Fatalf("hold: %v", err)
	}
	defer held.Release()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := Acquire(ctx, dir); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("acquire err = %v, want context.DeadlineExceeded", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("cancellation took %v, want it bounded", elapsed)
	}
}

// TestAcquireWaitBounds proves the explicit wait bound applies when the context
// carries no deadline of its own.
func TestAcquireWaitBounds(t *testing.T) {
	dir := t.TempDir()
	held, err := Acquire(context.Background(), dir)
	if err != nil {
		t.Fatalf("hold: %v", err)
	}
	defer held.Release()

	if _, err := AcquireWait(context.Background(), dir, 100*time.Millisecond); err == nil {
		t.Fatal("AcquireWait returned a lock while another holder was active")
	}
}

// TestAcquireCreatesStateDir proves acquisition creates the state directory rather
// than failing on a project whose state directory does not exist yet.
func TestAcquireCreatesStateDir(t *testing.T) {
	dir := t.TempDir()
	l, err := Acquire(context.Background(), dir)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer l.Release()
	if _, err := os.Stat(filepath.Join(dir, ".agent-sdlc", FileName)); err != nil {
		t.Fatalf("lock file not created: %v", err)
	}
}

// TestLockReleasedOnProcessExit proves the kernel drops the lock when the holding
// process exits without releasing it, so a crash cannot wedge the project.
func TestLockReleasedOnProcessExit(t *testing.T) {
	if os.Getenv("PROJECTLOCK_HELPER_DIR") != "" {
		l, err := Acquire(context.Background(), os.Getenv("PROJECTLOCK_HELPER_DIR"))
		if err != nil {
			os.Exit(3)
		}
		_ = l // deliberately never released
		os.Exit(0)
	}

	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=TestLockReleasedOnProcessExit")
	cmd.Env = append(os.Environ(), "PROJECTLOCK_HELPER_DIR="+dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("helper process: %v\n%s", err, out)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	l, err := Acquire(ctx, dir)
	if err != nil {
		t.Fatalf("acquire after the holder exited: %v", err)
	}
	l.Release()
}

// TestReleaseWithoutHoldIsSafe proves a nil receiver and a zero-value lock do not
// panic, so callers can defer Release unconditionally.
func TestReleaseWithoutHoldIsSafe(t *testing.T) {
	var l *Lock
	l.Release()
	var zero Lock
	zero.Release()
}

// TestAcquireAppliesDefaultTimeoutWithoutDeadline proves a context with no deadline
// does not wait forever on a held lock.
func TestAcquireAppliesDefaultTimeoutWithoutDeadline(t *testing.T) {
	old := defaultWait
	defaultWait = 100 * time.Millisecond
	defer func() { defaultWait = old }()

	dir := t.TempDir()
	held, err := Acquire(context.Background(), dir)
	if err != nil {
		t.Fatalf("hold: %v", err)
	}
	defer held.Release()

	start := time.Now()
	if _, err := Acquire(context.Background(), dir); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("acquire err = %v, want context.DeadlineExceeded", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("default timeout took %v, want it bounded", elapsed)
	}
}
