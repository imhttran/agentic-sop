package run

import (
	"sync"
	"testing"
)

// TestConsumeContinuationDoesNotOverspendConcurrently proves the read, check, and
// write are one locked critical section: concurrent consumers cannot both spend
// the same continuation, and the persisted count never exceeds the maximum.
func TestConsumeContinuationDoesNotOverspendConcurrently(t *testing.T) {
	dir := t.TempDir()
	const (
		max     = 5
		workers = 24
	)

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		allowed int
	)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rn, err := Open(dir, "T1")
			if err != nil {
				t.Errorf("open: %v", err)
				return
			}
			_, ok, err := rn.ConsumeContinuation(max)
			if err != nil {
				t.Errorf("consume: %v", err)
				return
			}
			if ok {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if allowed != max {
		t.Errorf("allowed = %d, want exactly %d", allowed, max)
	}
	rn, err := Open(dir, "T1")
	if err != nil {
		t.Fatal(err)
	}
	if got := rn.ReadContinuations(); got != max {
		t.Errorf("persisted continuations = %d, want %d", got, max)
	}
	if n, ok, err := rn.ConsumeContinuation(max); err != nil || ok || n != max {
		t.Errorf("over-budget consume = (%d, %v, %v), want (%d, false, nil)", n, ok, err, max)
	}
}
