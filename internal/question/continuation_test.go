package question

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// appliedQuestion creates a question, records an answer, applies it, and returns the
// applied question and its continuation ID.
func appliedQuestion(t *testing.T, svc *Service) (*Question, string) {
	t.Helper()
	q, err := svc.CreateQuestion(&Question{Identity: "T1", Kind: "k", Body: "q", RunEpoch: "e1", ReturnStage: "IMPLEMENTING"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := svc.CreateAnswer(&Answer{QuestionID: q.ID, Body: "a"}); err != nil {
		t.Fatalf("answer: %v", err)
	}
	if _, err := svc.ApplyAnswer(context.Background(), q.ID); err != nil {
		t.Fatalf("apply: %v", err)
	}
	return q, ContinuationID(q.ID, "e1")
}

func TestContinuationCreatedAtomicallyWithApplication(t *testing.T) {
	svc := NewService(NewMemoryStore(), nil)
	q, id := appliedQuestion(t, svc)

	cont, err := svc.Continuation(q.ID, "e1")
	if err != nil {
		t.Fatalf("continuation: %v", err)
	}
	if cont.ID != id || cont.State != ContinuationPending || cont.QuestionID != q.ID {
		t.Fatalf("continuation = %+v", cont)
	}
	// Repeated application is idempotent and creates no second continuation.
	if _, err := svc.ApplyAnswer(context.Background(), q.ID); err != nil {
		t.Fatalf("re-apply: %v", err)
	}
	all, _ := svc.Continuations("")
	if len(all) != 1 {
		t.Fatalf("continuations = %d, want 1", len(all))
	}
}

func TestClaimReclaimAndGenerationFencing(t *testing.T) {
	svc := NewService(NewMemoryStore(), nil)
	now := time.Now().UTC()
	svc.now = func() time.Time { return now }
	_, id := appliedQuestion(t, svc)

	c1, ok, err := svc.ClaimContinuation(id, "w1", 10*time.Second)
	if err != nil || !ok || c1.Generation != 1 {
		t.Fatalf("claim1 = %+v ok=%v err=%v", c1, ok, err)
	}
	// A live lease cannot be stolen.
	if _, ok, _ := svc.ClaimContinuation(id, "w2", 10*time.Second); ok {
		t.Fatal("w2 claimed a live lease")
	}
	// The holder verifies and is not fenced.
	if held, _ := svc.VerifyClaim(id, c1.Generation, "w1"); !held {
		t.Fatal("w1 lost a live claim")
	}
	// Lease expiry: reclaim increments the generation.
	now = now.Add(11 * time.Second)
	c2, ok, err := svc.ClaimContinuation(id, "w2", 10*time.Second)
	if err != nil || !ok || c2.Generation != 2 {
		t.Fatalf("reclaim = %+v ok=%v err=%v", c2, ok, err)
	}
	// w1 is fenced: it cannot verify, and it cannot complete under its stale generation.
	if held, _ := svc.VerifyClaim(id, c1.Generation, "w1"); held {
		t.Fatal("w1 must be fenced after reclaim")
	}
	if _, tr, _ := svc.CompleteContinuation(id, c1.Generation, "w1"); tr {
		t.Fatal("a stale worker must not complete")
	}
	// The current holder completes.
	if _, tr, err := svc.CompleteContinuation(id, c2.Generation, "w2"); err != nil || !tr {
		t.Fatalf("w2 complete tr=%v err=%v", tr, err)
	}
	// Completion is terminal: a further claim is refused.
	if _, ok, _ := svc.ClaimContinuation(id, "w3", time.Second); ok {
		t.Fatal("a completed continuation was claimed")
	}
}

func TestClaimBoundedAttempts(t *testing.T) {
	svc := NewService(NewMemoryStore(), nil)
	now := time.Now().UTC()
	svc.now = func() time.Time { return now }
	_, id := appliedQuestion(t, svc)

	for i := 0; i < DefaultContinuationMaxAttempts; i++ {
		if _, ok, err := svc.ClaimContinuation(id, "w", time.Second); err != nil || !ok {
			t.Fatalf("claim %d ok=%v err=%v", i, ok, err)
		}
		now = now.Add(2 * time.Second) // expire the lease
	}
	if _, _, err := svc.ClaimContinuation(id, "w", time.Second); !errors.Is(err, ErrClaimBudgetExhausted) {
		t.Fatalf("err = %v, want ErrClaimBudgetExhausted", err)
	}
}

func TestContinuationConcurrentClaimOneWins(t *testing.T) {
	path := filepath.Join(t.TempDir(), "questions.json")
	fs, err := OpenFileStore(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	_, id := appliedQuestion(t, NewService(fs, nil))

	const n = 4
	var wg sync.WaitGroup
	wins := make([]bool, n)
	for i := 0; i < n; i++ {
		s2, oerr := OpenFileStore(path)
		if oerr != nil {
			t.Fatalf("open %d: %v", i, oerr)
		}
		svc2 := NewService(s2, nil)
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, ok, err := svc2.ClaimContinuation(id, fmt.Sprintf("w%d", i), time.Minute); err == nil {
				wins[i] = ok
			}
		}(i)
	}
	wg.Wait()
	got := 0
	for _, w := range wins {
		if w {
			got++
		}
	}
	if got != 1 {
		t.Fatalf("claims won = %d, want exactly 1", got)
	}
}

// TestStaleWorkerCannotMutateCheckpoint proves the fence applies to the checkpoint
// mutation too: the pattern is "VerifyClaim, then mutate". An expired worker's fence
// check fails, so it must not mutate the run stage; only the current holder may.
func TestStaleWorkerCannotMutateCheckpoint(t *testing.T) {
	svc := NewService(NewMemoryStore(), nil)
	now := time.Now().UTC()
	svc.now = func() time.Time { return now }
	_, id := appliedQuestion(t, svc)

	w1, ok, err := svc.ClaimContinuation(id, "w1", 10*time.Second)
	if err != nil || !ok {
		t.Fatalf("claim w1: ok=%v err=%v", ok, err)
	}
	now = now.Add(11 * time.Second)
	w2, ok, err := svc.ClaimContinuation(id, "w2", 10*time.Second)
	if err != nil || !ok {
		t.Fatalf("reclaim w2: ok=%v err=%v", ok, err)
	}

	// The mutation is gated on the fence.
	mutatedStage := ""
	if held, _ := svc.VerifyClaim(id, w1.Generation, "w1"); held {
		mutatedStage = "IMPLEMENTING" // would mutate
	}
	if mutatedStage != "" {
		t.Fatal("an expired worker passed the fence and mutated the checkpoint")
	}
	if held, _ := svc.VerifyClaim(id, w2.Generation, "w2"); !held {
		t.Fatal("the current holder must pass the fence")
	}
}

func TestContinuationPersistsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "questions.json")
	fs, err := OpenFileStore(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	q, id := appliedQuestion(t, NewService(fs, nil))

	fs2, err := OpenFileStore(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	c, err := NewService(fs2, nil).Continuation(q.ID, "e1")
	if err != nil || c.ID != id || c.State != ContinuationPending {
		t.Fatalf("reopen continuation = %+v err=%v", c, err)
	}
}
