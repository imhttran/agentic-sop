package orchestration

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// recordingAgent is a deterministic, provider-free agent.Agent used by the
// coordinator tests. It records invocations and returns a canned response or a
// canned error. It is distinct from the contract_test fakeAgent so the two test
// files can coexist in the same package.
type recordingAgent struct {
	mu        sync.Mutex
	calls     []agent.Request
	respond   func(req agent.Request) (agent.Response, error)
	generated int32
}

func (f *recordingAgent) Generate(_ context.Context, req agent.Request) (agent.Response, error) {
	atomic.AddInt32(&f.generated, 1)
	f.mu.Lock()
	f.calls = append(f.calls, req)
	f.mu.Unlock()
	return f.respond(req)
}

func (f *recordingAgent) callCount() int { return int(atomic.LoadInt32(&f.generated)) }

// completedAgent returns a fake agent that always reports a completed outcome
// with no changed files, which ValidateResult accepts for an empty scope.
func completedAgent() *recordingAgent {
	return &recordingAgent{
		respond: func(agent.Request) (agent.Response, error) {
			return agent.Response{Outcome: &agent.Outcome{Status: agent.OutcomeCompleted, Summary: "ok"}}, nil
		},
	}
}

// transportErrorAgent returns a fake agent that always fails at transport.
func transportErrorAgent() *recordingAgent {
	return &recordingAgent{
		respond: func(agent.Request) (agent.Response, error) {
			return agent.Response{}, errors.New("boom")
		},
	}
}

// validAssignment builds a minimal valid assignment. Capability defaults to
// REVIEW (non-mutating).
func validAssignment(id string, cap agent.Capability) WorkAssignment {
	return WorkAssignment{
		AssignmentID:       id,
		TaskID:             "task-" + id,
		Capability:         cap,
		Task:               "do " + id,
		RepositoryIdentity: RepositoryIdentity{Revision: "rev1"},
	}
}

// TestCoordinatorSequentialValidResult covers the valid completed path.
func TestCoordinatorSequentialValidResult(t *testing.T) {
	c := NewCoordinator(completedAgent())
	r := c.Run(context.Background(), ExecutionPolicy{}, []WorkAssignment{validAssignment("a1", agent.Review)})
	if r.Completed != 1 || r.Succeeded() != true {
		t.Fatalf("expected one completed outcome, got %+v", r)
	}
	if r.Outcomes[0].Failure != FailureNone {
		t.Fatalf("expected no failure, got %q", r.Outcomes[0].Failure)
	}
}

// TestCoordinatorInvalidAssignmentIsNotSent covers the assignment-validation
// failure path: an invalid assignment must never reach the agent.
func TestCoordinatorInvalidAssignmentIsNotSent(t *testing.T) {
	fa := completedAgent()
	c := NewCoordinator(fa)
	invalid := WorkAssignment{AssignmentID: "", TaskID: "", Capability: agent.Review}
	r := c.Run(context.Background(), ExecutionPolicy{}, []WorkAssignment{invalid})
	if r.Outcomes[0].Failure != FailureInvalidAssignment {
		t.Fatalf("expected invalid-assignment failure, got %q", r.Outcomes[0].Failure)
	}
	if fa.callCount() != 0 {
		t.Fatalf("invalid assignment reached the agent: %d calls", fa.callCount())
	}
}

// TestCoordinatorNilAgentFailsExplicitly covers the nil-agent path.
func TestCoordinatorNilAgentFailsExplicitly(t *testing.T) {
	c := NewCoordinator(nil)
	r := c.Run(context.Background(), ExecutionPolicy{}, []WorkAssignment{validAssignment("a1", agent.Review)})
	if r.Outcomes[0].Failure != FailureNoAgent {
		t.Fatalf("expected no-agent failure, got %q", r.Outcomes[0].Failure)
	}
	if r.Completed != 0 {
		t.Fatalf("nil agent must never yield a completed outcome")
	}
}

// TestCoordinatorTransportError covers the agent transport failure path.
func TestCoordinatorTransportError(t *testing.T) {
	c := NewCoordinator(transportErrorAgent())
	r := c.Run(context.Background(), ExecutionPolicy{}, []WorkAssignment{validAssignment("a1", agent.Review)})
	if r.Outcomes[0].Failure != FailureTransport {
		t.Fatalf("expected transport failure, got %q", r.Outcomes[0].Failure)
	}
	if r.Completed != 0 {
		t.Fatalf("transport error must never yield a completed outcome")
	}
}

// outOfScopeAgent claims a changed file outside any declared scope.
func outOfScopeAgent() *recordingAgent {
	return &recordingAgent{
		respond: func(agent.Request) (agent.Response, error) {
			return agent.Response{
				Outcome:      &agent.Outcome{Status: agent.OutcomeCompleted, Summary: "changed"},
				ChangedFiles: []string{"internal/secret/file.go"},
			}, nil
		},
	}
}

// TestCoordinatorOutOfScopeResultRejected covers the invalid-result path.
func TestCoordinatorOutOfScopeResultRejected(t *testing.T) {
	c := NewCoordinator(outOfScopeAgent())
	a := validAssignment("a1", agent.Implement)
	a.Scope = Scope{Paths: []string{"internal/agent"}}
	r := c.Run(context.Background(), ExecutionPolicy{}, []WorkAssignment{a})
	if r.Outcomes[0].Failure != FailureInvalidResult {
		t.Fatalf("expected invalid-result failure, got %q", r.Outcomes[0].Failure)
	}
	if r.Completed != 0 {
		t.Fatalf("out-of-scope result must never yield a completed outcome")
	}
}

// TestCoordinatorResultOrderingMatchesCallerOrder asserts ordering is stable
// regardless of execution timing.
func TestCoordinatorResultOrderingMatchesCallerOrder(t *testing.T) {
	assignments := []WorkAssignment{
		validAssignment("a1", agent.Review),
		validAssignment("a2", agent.Plan),
		validAssignment("a3", agent.DesignTests),
	}
	c := NewCoordinator(completedAgent())
	r := c.Run(context.Background(), ExecutionPolicy{}, assignments)
	for i, out := range r.Outcomes {
		if out.AssignmentID != assignments[i].AssignmentID {
			t.Fatalf("outcome %d = %q, want %q", i, out.AssignmentID, assignments[i].AssignmentID)
		}
	}
}

// TestCoordinatorDeterminismRepeatedRuns asserts identical reports across runs.
func TestCoordinatorDeterminismRepeatedRuns(t *testing.T) {
	assignments := []WorkAssignment{
		validAssignment("a1", agent.Review),
		validAssignment("a2", agent.Plan),
	}
	c := NewCoordinator(completedAgent())
	first := c.Run(context.Background(), ExecutionPolicy{}, assignments).Canonicalize()
	second := c.Run(context.Background(), ExecutionPolicy{}, assignments).Canonicalize()
	if first != second {
		t.Fatalf("reports differ across runs:\n%s\n%s", first, second)
	}
}
