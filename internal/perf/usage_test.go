package perf

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestUsageSumsCallsByModel pins that calls under one context add up per model,
// concurrently, that a context without a Usage is a no-op, and that the run
// aggregate and the report carry the totals.
func TestUsageSumsCallsByModel(t *testing.T) {
	AddUsage(context.Background(), "m", 1, 1) // no Usage: must not panic

	ctx := WithUsage(context.Background(), &Usage{})
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); AddUsage(ctx, "big", 100, 10) }()
	}
	wg.Wait()
	AddUsage(ctx, "small", 5, 1)

	got := UsageFrom(ctx)
	if got["big"] != (Tokens{Calls: 10, Input: 1000, Output: 100}) || got["small"] != (Tokens{Calls: 1, Input: 5, Output: 1}) {
		t.Fatalf("usage = %+v", got)
	}

	run := NewRun(time.Time{})
	run.Add(Task{ID: "a", Tokens: got})
	run.Add(Task{ID: "b", Tokens: map[string]Tokens{"small": {Calls: 2, Input: 7, Output: 3}}})
	if sum := TotalTokens(run.Tokens); sum != (Tokens{Calls: 13, Input: 1012, Output: 104}) {
		t.Errorf("run total = %+v", sum)
	}
	var b bytes.Buffer
	WriteRun(&b, *run)
	if !strings.Contains(b.String(), "big") || !strings.Contains(b.String(), "1012") {
		t.Errorf("report missing token rows:\n%s", b.String())
	}
	if line := (Task{Tokens: got}).Line(); !strings.Contains(line, "tokens 1005 in, 101 out") {
		t.Errorf("task line = %q", line)
	}
}
