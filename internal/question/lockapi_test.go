package question

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// TestExecutionLockBlocksReclaimedWorkerWhileRunning proves the coherent ownership
// protocol: the claim lease bounds WHEN a worker may claim, but the execution lock bounds
// WHO may run the resumed work. If worker A's lease lapses while A is still executing, B
// may reclaim the claim (and A is fenced from writing the checkpoint or recording
// completion), yet B must NOT execute the same continuation concurrently, because A still
// holds the execution lock. Only after A releases may a worker take execution ownership.
func TestExecutionLockBlocksReclaimedWorkerWhileRunning(t *testing.T) {
	svc := NewService(NewMemoryStore(), nil)
	now := time.Now().UTC()
	svc.now = func() time.Time { return now }
	_, id := appliedQuestion(t, svc)

	lockPath := filepath.Join(t.TempDir(), "exec.lock")

	a, ok, err := svc.ClaimContinuation(id, "A", 10*time.Second)
	if err != nil || !ok {
		t.Fatalf("claim A: ok=%v err=%v", ok, err)
	}
	releaseA, gotA, err := TryAcquireLock(lockPath)
	if err != nil || !gotA {
		t.Fatalf("A execution lock: got=%v err=%v", gotA, err)
	}

	// The lease lapses while A is still running.
	now = now.Add(11 * time.Second)
	b, ok, err := svc.ClaimContinuation(id, "B", 10*time.Second)
	if err != nil || !ok || b.Generation != a.Generation+1 {
		t.Fatalf("reclaim B: c=%+v ok=%v err=%v", b, ok, err)
	}
	// A is now fenced for the protected writes: no checkpoint mutation, no completion.
	if err := svc.FenceAndMutate(id, a.Generation, "A", func() error { return nil }); !errors.Is(err, ErrClaimFenced) {
		t.Fatalf("A checkpoint write err = %v, want ErrClaimFenced", err)
	}
	if _, tr, _ := svc.CompleteContinuation(id, a.Generation, "A"); tr {
		t.Fatal("A recorded completion after being reclaimed")
	}

	// B holds the claim, but it must stand down: A still owns the execution.
	if _, gotB, err := TryAcquireLock(lockPath); err != nil {
		t.Fatalf("B execution lock err = %v", err)
	} else if gotB {
		t.Fatal("B executed the same continuation while A was still running")
	}

	// Only after A finishes and releases may a worker take execution ownership.
	releaseA()
	releaseC, gotC, err := TryAcquireLock(lockPath)
	if err != nil || !gotC {
		t.Fatalf("after release: got=%v err=%v", gotC, err)
	}
	releaseC()
}

// TestExecutionLockDoesNotBlockQuestionStore proves the execution lock is a SEPARATE file
// from the question store's lock: holding execution ownership for the whole resumed work
// never blocks question creation or answer submission (the operator/planner keeps working
// while the executor holds its stage-ownership lock).
func TestExecutionLockDoesNotBlockQuestionStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "questions.json")
	fs, err := OpenFileStore(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	svc := NewService(fs, nil)

	release, ok, err := TryAcquireLock(filepath.Join(t.TempDir(), "exec.lock"))
	if err != nil || !ok {
		t.Fatalf("execution lock: ok=%v err=%v", ok, err)
	}
	defer release()

	q, err := svc.CreateQuestion(&Question{Identity: "T1", Kind: "k", Body: "q", RunEpoch: "e1", ReturnStage: "IMPLEMENTING"})
	if err != nil {
		t.Fatalf("create question while executing: %v", err)
	}
	if _, err := svc.CreateAnswer(&Answer{QuestionID: q.ID, Body: "a"}); err != nil {
		t.Fatalf("submit answer while executing: %v", err)
	}
}
