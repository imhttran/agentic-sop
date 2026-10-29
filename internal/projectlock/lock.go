// Package projectlock serializes the writers of a project's durable state.
//
// SOP's durable state lives in two stores that cannot share a transaction: the
// JSON artifacts under .agent-sdlc (plan.json, plan.meta.json, approval.json and
// its history, lifecycle records) and the SQLite task graph in state.db. Several
// operations read a value, decide from it, and then write (a decision reads the
// approval head; invalidation reads the head and the task definition;
// reconciliation reads the active graph). Without mutual exclusion a competing
// writer can change the input between the read and the write.
//
// This package provides one project-scoped advisory lock that every participating
// writer takes before it reads those inputs and releases only after it has
// written. It provides MUTUAL EXCLUSION ONLY. It does not make the JSON and
// SQLite writes atomic with each other: a process that dies between them still
// leaves the two stores disagreeing, and no caller may claim otherwise.
//
// Participation is by convention, not enforcement: a writer that does not take
// the lock is not excluded, and nothing here can detect one.
package projectlock

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/imhttran/agentic-sop/internal/config"
)

// FileName is the project-scoped lock file, relative to the state directory. It is
// separate from every state artifact so locking never rewrites state.
const FileName = "state.lock"

// DefaultTimeout bounds an acquisition when the caller's context has no deadline
// of its own, so a contended lock fails with a clear error instead of hanging.
const DefaultTimeout = 30 * time.Second

// defaultWait is DefaultTimeout as applied by Acquire; tests shorten it.
var defaultWait = DefaultTimeout

// ErrUnsupported reports a platform that cannot provide the advisory lock. SOP
// never silently degrades to a no-op: a caller that requires the lock must fail
// rather than proceed unprotected.
var ErrUnsupported = errors.New("projectlock: advisory locking is not available on this platform")

// Lock is a held project lock. Release is idempotent, so a deferred release after
// an explicit one is safe.
type Lock struct {
	f    *os.File
	once sync.Once
}

// Acquire takes the exclusive project lock for dir, waiting until it is available,
// the context is done, or DefaultTimeout elapses when the context has no deadline.
//
// Cancellation is bounded and leak-free: acquisition happens in the background, so
// a canceled caller returns immediately, but the file descriptor is closed only
// after the background acquisition finishes. That ordering is what prevents both an
// orphaned descriptor and a lock released without ever having been held.
func Acquire(ctx context.Context, dir string) (*Lock, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultWait)
		defer cancel()
	}
	path := filepath.Join(dir, config.DirName, FileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}

	// The platform lock call blocks, so it runs in the background and the caller
	// selects against its context. A canceled caller returns at once; the descriptor
	// is closed only after the background acquisition resolves, which is what keeps
	// a cancellation from leaking a descriptor or releasing a lock never held.
	done := make(chan error, 1)
	go func() { done <- lockFile(f) }()

	select {
	case err := <-done:
		if err != nil {
			_ = f.Close()
			return nil, err
		}
		return &Lock{f: f}, nil
	case <-ctx.Done():
		go func() {
			if err := <-done; err == nil {
				_ = unlockFile(f)
			}
			_ = f.Close()
		}()
		return nil, ctx.Err()
	}
}

// AcquireWait is Acquire with an explicit wait bound. A non-positive timeout means
// DefaultTimeout.
func AcquireWait(ctx context.Context, dir string, timeout time.Duration) (*Lock, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	wait := timeout
	if wait <= 0 {
		wait = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, wait)
	defer cancel()
	return Acquire(ctx, dir)
}

// Release drops the lock. It is idempotent and safe on a nil receiver, so callers
// can defer it unconditionally.
func (l *Lock) Release() {
	if l == nil {
		return
	}
	l.once.Do(func() {
		if l.f == nil {
			return
		}
		_ = unlockFile(l.f)
		_ = l.f.Close()
	})
}
