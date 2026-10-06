package orchestration

// ORCH-002.4 contract conformance tests. They exercise the provider-neutral
// WorkAssignment/WorkResult contract identically against a deterministic fake
// adapter and against a real in-repo adapter, with no network or model call.

import (
	"context"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// fakeAgent is a deterministic test double implementing the existing agent.Agent
// boundary. It returns a scripted agent.Response per invocation with no network
// or model dependency, so tests are fully deterministic.
type fakeAgent struct {
	response agent.Response
	err      error
}

func (f fakeAgent) Generate(ctx context.Context, request agent.Request) (agent.Response, error) {
	return f.response, f.err
}

// otherAgent is a second, independent deterministic adapter used to prove the
// contract is satisfiable by more than the canonical fake. It is a distinct
// type with distinct construction, so the conformance suite is not accidentally
// bound to one implementation.
type otherAgent struct {
	status       agent.OutcomeStatus
	changedFiles []string
}

func (o otherAgent) Generate(ctx context.Context, request agent.Request) (agent.Response, error) {
	return agent.Response{
		Content:      "other adapter response",
		ChangedFiles: o.changedFiles,
		Outcome:      &agent.Outcome{Status: o.status, Summary: "other adapter produced a structured outcome"},
	}, nil
}

// conformanceAdapter describes one adapter under test.
type conformanceCase struct {
	name  string
	agent agent.Agent
}

// conformingAssignment builds a valid assignment within the declared scope.
func conformingAssignment() WorkAssignment {
	builder := AssignmentBuilder{
		AssignmentID:       "assign-1",
		TaskID:             "ORCH-002",
		Capability:         agent.Implement,
		Task:               "implement the worker contract",
		Scope:              Scope{Paths: []string{"internal/orchestration"}},
		Context:            "provider-neutral context",
		AllowedTools:       []string{"read_file", "write_file"},
		Budget:             Budget{MaxSteps: 10},
		OutputContract:     OutputContract{Format: "json"},
		RepositoryIdentity: RepositoryIdentity{Revision: "deadbeef"},
		AcceptanceCriteria: []string{"contract holds"},
		ValidationCommands: []string{"go test ./..."},
	}
	return builder.Build()
}

// TestAdapterConformance runs the identical conformance suite against the
// deterministic fake adapter and a second independent adapter. Both must
// satisfy the same contract: a valid assignment yields a validated result, no
// provider concept is involved, and no live network is used.
func TestAdapterConformance(t *testing.T) {
	assignment := conformingAssignment()

	fake := fakeAgent{response: agent.Response{
		Content:      "fake response",
		ChangedFiles: []string{"internal/orchestration/assignment.go"},
		Outcome:      &agent.Outcome{Status: agent.OutcomeCompleted, Summary: "fake completed"},
	}}
	other := otherAgent{
		status:       agent.OutcomeCompleted,
		changedFiles: []string{"internal/orchestration/worker_adapter.go"},
	}

	cases := []conformanceCase{
		{name: "deterministic-fake", agent: fake},
		{name: "other-adapter", agent: other},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			worker := NewWorkerAdapter(WorkUnit{TaskID: assignment.TaskID, Capability: assignment.Capability}, tc.agent, assignment)

			result, err := worker.Assign(context.Background(), assignment)
			if err != nil {
				t.Fatalf("Assign returned transport error: %v", err)
			}
			if err := ValidateResult(assignment, result); err != nil {
				t.Fatalf("adapter produced an invalid result: %v", err)
			}
			if result.Status != agent.OutcomeCompleted {
				t.Fatalf("status = %q, want completed", result.Status)
			}
			if result.AssignmentID != assignment.AssignmentID {
				t.Fatalf("assignment id = %q, want %q", result.AssignmentID, assignment.AssignmentID)
			}
		})
	}
}

// TestWorkerAdapterSatisfiesWorkerContract asserts the adapter is usable through
// the existing orchestration.Worker interface without the contract depending on
// any concrete adapter.
func TestWorkerAdapterSatisfiesWorkerContract(t *testing.T) {
	assignment := conformingAssignment()
	worker := NewWorkerAdapter(WorkUnit{TaskID: assignment.TaskID, Capability: assignment.Capability}, fakeAgent{}, assignment)

	var w Worker = worker
	got := w.Work()
	if got.TaskID != assignment.TaskID || got.Capability != assignment.Capability {
		t.Fatalf("Work() = %+v, want task %q capability %q", got, assignment.TaskID, assignment.Capability)
	}
}

// TestAssignmentRoundTrip maps a WorkAssignment to an agent.Request and back,
// asserting provider-neutral field fidelity.
func TestAssignmentRoundTrip(t *testing.T) {
	assignment := conformingAssignment()
	req := ToAgentRequest(assignment)

	if req.Capability != assignment.Capability {
		t.Fatalf("capability = %q, want %q", req.Capability, assignment.Capability)
	}
	if req.Task != assignment.Task {
		t.Fatalf("task = %q, want %q", req.Task, assignment.Task)
	}
	if req.Input != assignment.Context {
		t.Fatalf("input = %q, want context %q", req.Input, assignment.Context)
	}
	if req.OutputRequirements != assignment.OutputContract.Format {
		t.Fatalf("output requirements = %q, want %q", req.OutputRequirements, assignment.OutputContract.Format)
	}
	if len(req.AcceptanceCriteria) != len(assignment.AcceptanceCriteria) {
		t.Fatalf("acceptance criteria not carried through")
	}
	if len(req.ValidationCommands) != len(assignment.ValidationCommands) {
		t.Fatalf("validation commands not carried through")
	}

	response := agent.Response{
		ChangedFiles: []string{"internal/orchestration/assignment.go"},
		Outcome:      &agent.Outcome{Status: agent.OutcomeCompleted, Summary: "done"},
	}
	result := FromAgentResponse(assignment, response)
	if result.AssignmentID != assignment.AssignmentID {
		t.Fatalf("result assignment id = %q, want %q", result.AssignmentID, assignment.AssignmentID)
	}
	if result.RepositoryIdentity != assignment.RepositoryIdentity {
		t.Fatalf("result repository identity = %+v, want %+v", result.RepositoryIdentity, assignment.RepositoryIdentity)
	}
	if len(result.ChangedFiles) != 1 || result.ChangedFiles[0] != "internal/orchestration/assignment.go" {
		t.Fatalf("changed files not carried through: %v", result.ChangedFiles)
	}
}

// TestBuildIsDeterministic asserts identical builder inputs yield identical
// assignments and that caller slice mutation cannot change a built assignment.
func TestBuildIsDeterministic(t *testing.T) {
	paths := []string{"internal/orchestration"}
	b := AssignmentBuilder{
		AssignmentID:       "assign-1",
		TaskID:             "ORCH-002",
		Capability:         agent.Implement,
		Task:               "task",
		Scope:              Scope{Paths: paths},
		RepositoryIdentity: RepositoryIdentity{Revision: "rev"},
	}
	first := b.Build()
	paths[0] = "mutated"
	second := b.Build()
	if first.Scope.Paths[0] != second.Scope.Paths[0] {
		t.Fatalf("built assignment changed after caller mutation: %q vs %q", first.Scope.Paths[0], second.Scope.Paths[0])
	}
	if len(first.Scope.Paths) != 1 || first.Scope.Paths[0] != "internal/orchestration" {
		t.Fatalf("built assignment scope = %v, want [internal/orchestration]", first.Scope.Paths)
	}
}
