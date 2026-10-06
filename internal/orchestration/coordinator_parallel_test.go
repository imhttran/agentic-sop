package orchestration

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// concurrencyTrackingAgent records the maximum number of concurrent Generate
// calls observed, so tests can prove the worker-pool bound and the mutating
// exclusion.
type concurrencyTrackingAgent struct {
	current int32
	max     int32
}

func (a *concurrencyTrackingAgent) Generate(_ context.Context, req agent.Request) (agent.Response, error) {
	cur := atomic.AddInt32(&a.current, 1)
	defer atomic.AddInt32(&a.current, -1)
	for {
		observed := atomic.LoadInt32(&a.max)
		if cur <= observed || atomic.CompareAndSwapInt32(&a.max, observed, cur) {
			break
		}
	}
	return agent.Response{Outcome: &agent.Outcome{Status: agent.OutcomeCompleted, Summary: "ok"}}, nil
}

func (a *concurrencyTrackingAgent) maxObserved() int { return int(atomic.LoadInt32(&a.max)) }

// TestParallelRespectsConcurrencyLimit proves no more than N non-mutating
// assignments run at once with a concurrency limit of N. A tiny bounded spin
// creates observable overlap.
func TestParallelRespectsConcurrencyLimit(t *testing.T) {
	const limit = 2
	ta := &concurrencyTrackingAgent{}
	c := NewCoordinator(ta)
	assignments := []WorkAssignment{
		validAssignment("a1", agent.Review),
		validAssignment("a2", agent.Review),
		validAssignment("a3", agent.Review),
		validAssignment("a4", agent.Review),
	}
	r := c.Run(context.Background(), ExecutionPolicy{Mode: ExecutionParallel, MaxConcurrency: limit}, assignments)
	if r.Completed != len(assignments) {
		t.Fatalf("expected all completed, got %+v", r)
	}
	if got := ta.maxObserved(); got > limit {
		t.Fatalf("observed %d concurrent calls, limit is %d", got, limit)
	}
}

// TestMutatingAssignmentsNeverRunConcurrently proves IMPLEMENT and FIX
// assignments execute one at a time even in parallel mode.
func TestMutatingAssignmentsNeverRunConcurrently(t *testing.T) {
	var overlap int32
	var active int32
	mutating := &mutatingOverlapAgent{active: &active, overlap: &overlap}
	c := NewCoordinator(mutating)
	assignments := []WorkAssignment{
		validAssignment("m1", agent.Implement),
		validAssignment("m2", agent.Fix),
		validAssignment("m3", agent.Implement),
	}
	c.Run(context.Background(), ExecutionPolicy{Mode: ExecutionParallel, MaxConcurrency: 4}, assignments)
	if atomic.LoadInt32(&overlap) != 0 {
		t.Fatalf("mutating assignments overlapped %d times", overlap)
	}
}

// mutatingOverlapAgent sets overlap to 1 if two Generate calls are ever active
// at once.
type mutatingOverlapAgent struct {
	active  *int32
	overlap *int32
}

func (a *mutatingOverlapAgent) Generate(_ context.Context, _ agent.Request) (agent.Response, error) {
	if atomic.AddInt32(a.active, 1) > 1 {
		atomic.StoreInt32(a.overlap, 1)
	}
	atomic.AddInt32(a.active, -1)
	return agent.Response{Outcome: &agent.Outcome{Status: agent.OutcomeCompleted, Summary: "ok"}}, nil
}

// TestParallelAndSequentialReportsMatch asserts the report ordering is
// identical between modes for the same assignment set.
func TestParallelAndSequentialReportsMatch(t *testing.T) {
	assignments := []WorkAssignment{
		validAssignment("a1", agent.Review),
		validAssignment("a2", agent.Implement),
		validAssignment("a3", agent.Plan),
	}
	c := NewCoordinator(completedAgent())
	seq := c.Run(context.Background(), ExecutionPolicy{Mode: ExecutionSequential}, assignments)
	par := c.Run(context.Background(), ExecutionPolicy{Mode: ExecutionParallel, MaxConcurrency: 3}, assignments)
	for i := range assignments {
		if seq.Outcomes[i].AssignmentID != par.Outcomes[i].AssignmentID {
			t.Fatalf("mode ordering mismatch at %d: %q vs %q", i, seq.Outcomes[i].AssignmentID, par.Outcomes[i].AssignmentID)
		}
	}
	if seq.Canonicalize() != par.Canonicalize() {
		t.Fatalf("canonical reports differ between modes")
	}
}

// TestCancelledContextYieldsTypedOutcomes proves a cancelled context yields
// typed cancelled outcomes, not silent success.
func TestCancelledContextYieldsTypedOutcomes(t *testing.T) {
	assignments := []WorkAssignment{
		validAssignment("a1", agent.Review),
		validAssignment("a2", agent.Plan),
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := NewCoordinator(completedAgent())
	r := c.Run(ctx, ExecutionPolicy{Mode: ExecutionParallel, MaxConcurrency: 2}, assignments)
	for _, out := range r.Outcomes {
		if out.Failure != FailureCancelled {
			t.Fatalf("expected cancelled outcome, got %q", out.Failure)
		}
	}
	if r.Completed != 0 || r.Cancelled != len(assignments) {
		t.Fatalf("expected all cancelled, got %+v", r)
	}
}
