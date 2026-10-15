package question

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// TestFenceAndMutateRejectsReclaimedWorker is the interleaving case: A verifies (as it
// would before writing), B reclaims, then A attempts the protected checkpoint write.
// The write must be rejected and the mutation must not run, because verification and
// mutation are coordinated under the store lock rather than being separate steps.
func TestFenceAndMutateRejectsReclaimedWorker(t *testing.T) {
	svc := NewService(NewMemoryStore(), nil)
	now := time.Now().UTC()
	svc.now = func() time.Time { return now }
	_, id := appliedQuestion(t, svc)

	a, ok, err := svc.ClaimContinuation(id, "A", 10*time.Second)
	if err != nil || !ok {
		t.Fatalf("claim A: ok=%v err=%v", ok, err)
	}
	if held, _ := svc.VerifyClaim(id, a.Generation, "A"); !held {
		t.Fatal("A should hold its claim at this moment")
	}

	now = now.Add(11 * time.Second)
	b, ok, err := svc.ClaimContinuation(id, "B", 10*time.Second)
	if err != nil || !ok {
		t.Fatalf("reclaim B: ok=%v err=%v", ok, err)
	}

	written := false
	if err := svc.FenceAndMutate(id, a.Generation, "A", func() error { written = true; return nil }); !errors.Is(err, ErrClaimFenced) {
		t.Fatalf("A's fenced write err = %v, want ErrClaimFenced", err)
	}
	if written {
		t.Fatal("a fenced write executed the protected mutation")
	}
	written = false
	if err := svc.FenceAndMutate(id, b.Generation, "B", func() error { written = true; return nil }); err != nil {
		t.Fatalf("B's write err = %v", err)
	}
	if !written {
		t.Fatal("the current holder's write did not run")
	}
}

// TestContinuationRecoveryCrashCases models the crash points: after application
// (PENDING, unclaimed) and after work before completion (CLAIMED). Both are
// recoverable, and a completed continuation is never re-run.
func TestContinuationRecoveryCrashCases(t *testing.T) {
	path := filepath.Join(t.TempDir(), "questions.json")
	fs, err := OpenFileStore(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	svc := NewService(fs, nil)
	now := time.Now().UTC()
	svc.now = func() time.Time { return now }
	_, id := appliedQuestion(t, svc)

	// Crash after application, before dispatch: the continuation is PENDING and
	// unclaimed; a later worker claims it (no lost work).
	c, ok, err := svc.ClaimContinuation(id, "w", time.Second)
	if err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}

	// Crash during or after the work, before completion: the record is CLAIMED with a
	// lapsed lease. A second worker reclaims (generation advances) and completes.
	now = now.Add(2 * time.Second)
	c2, ok, err := svc.ClaimContinuation(id, "w2", time.Second)
	if err != nil || !ok {
		t.Fatalf("reclaim: ok=%v err=%v", ok, err)
	}
	if c2.Generation != c.Generation+1 {
		t.Fatalf("generation = %d, want %d", c2.Generation, c.Generation+1)
	}
	if _, tr, err := svc.CompleteContinuation(id, c2.Generation, "w2"); err != nil || !tr {
		t.Fatalf("complete: tr=%v err=%v", tr, err)
	}
	// Recovery converges: a completed continuation cannot be re-claimed, so an
	// already-applied (local, idempotent) effect is not repeated.
	if _, ok, _ := svc.ClaimContinuation(id, "w3", time.Second); ok {
		t.Fatal("a completed continuation was re-claimed")
	}
}
