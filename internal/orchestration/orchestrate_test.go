package orchestration

// ORCH-011 end-to-end orchestration tests. They are deterministic and model-free:
// they drive the pipeline with a scripted fake agent and assert provider-neutral,
// bounded, deterministic, failure-propagating behavior. No provider or network is
// touched.

import (
	"context"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/model"
)

// agentFunc adapts a function to the agent.Agent boundary.
type agentFunc func(context.Context, agent.Request) (agent.Response, error)

func (f agentFunc) Generate(ctx context.Context, r agent.Request) (agent.Response, error) {
	return f(ctx, r)
}

// okAgent always returns a completed claim.
func okAgent() agent.Agent {
	return agentFunc(func(context.Context, agent.Request) (agent.Response, error) {
		return agent.Response{Outcome: &agent.Outcome{Status: agent.OutcomeCompleted, Summary: "ok"}}, nil
	})
}

// orchDecompose builds a decomposition of non-mutating (agent.Plan) analysis units
// with a deny-all scope, so the units are parallel-eligible.
func orchDecompose(tasks ...string) DecomposeRequest {
	cands := make([]DecompositionCandidate, 0, len(tasks))
	for i, task := range tasks {
		cands = append(cands, DecompositionCandidate{
			TaskID:     "T" + strconv.Itoa(i+1),
			Capability: agent.Plan,
			Task:       task,
			Scope:      Scope{},
		})
	}
	return DecomposeRequest{
		Candidates:         cands,
		RepositoryIdentity: RepositoryIdentity{Revision: "rev-1"},
		AcceptanceCriteria: []string{"ac-1"},
		ValidationCommands: []string{"go test ./..."},
	}
}

func orchRequest(tasks ...string) OrchestrateRequest {
	return OrchestrateRequest{
		Decompose: orchDecompose(tasks...),
		Policy:    ExecutionPolicy{Mode: ExecutionParallel, MaxConcurrency: 4},
		Baseline:  NewProgressBaseline(),
		Context:   WorkerContextRequest{TaskID: "T1", Task: "shared task context"},
		Routing:   WorkerRoutingRequest{Baseline: model.ClassMedium, Capability: agent.Plan},
	}
}

// TestOrchestrateRunsBoundedPipelineDeterministically proves the end-to-end path is
// reachable, bounded, non-authoritative at the integration boundary, and
// deterministic across repeated runs.
func TestOrchestrateRunsBoundedPipelineDeterministically(t *testing.T) {
	ev, err := Orchestrate(context.Background(), okAgent(), orchRequest("analyse alpha", "analyse beta"))
	if err != nil {
		t.Fatalf("orchestrate: %v", err)
	}
	if len(ev.Assignments) != 2 {
		t.Errorf("assignments = %d, want 2", len(ev.Assignments))
	}
	if ev.Report.Completed != 2 {
		t.Errorf("completed = %d, want 2", ev.Report.Completed)
	}
	if ev.View.Authoritative {
		t.Error("integrated view must be non-authoritative")
	}
	if ev.View.NextStage != StageVerify {
		t.Errorf("next stage = %q, want %q", ev.View.NextStage, StageVerify)
	}
	if len(ev.View.Claims) != 2 {
		t.Errorf("claims = %d, want 2", len(ev.View.Claims))
	}
	if len(ev.Progress) != len(ev.Report.Outcomes) {
		t.Errorf("progress = %d, want %d", len(ev.Progress), len(ev.Report.Outcomes))
	}

	ev2, err := Orchestrate(context.Background(), okAgent(), orchRequest("analyse alpha", "analyse beta"))
	if err != nil {
		t.Fatalf("orchestrate (repeat): %v", err)
	}
	if !reflect.DeepEqual(ev, ev2) {
		t.Error("repeated identical runs produced different evidence")
	}
}

// TestOrchestrateFailurePropagates proves a child worker failure is represented in
// the report and the integration claims, never silently dropped.
func TestOrchestrateFailurePropagates(t *testing.T) {
	a := agentFunc(func(_ context.Context, r agent.Request) (agent.Response, error) {
		status := agent.OutcomeCompleted
		if strings.Contains(r.Task, "fail") {
			status = agent.OutcomeFailed
		}
		return agent.Response{Outcome: &agent.Outcome{Status: status, Summary: "s"}}, nil
	})
	ev, err := Orchestrate(context.Background(), a, orchRequest("ok one", "fail two"))
	if err != nil {
		t.Fatalf("orchestrate: %v", err)
	}
	if ev.Report.Completed != 1 {
		t.Errorf("completed = %d, want 1", ev.Report.Completed)
	}
	// The failed worker's claim is explicitly rejected (ValidateResult refuses a
	// worker-reported failure), never silently dropped and never reinterpreted as
	// success.
	rejected := false
	for _, o := range ev.Report.Outcomes {
		if o.Failure != FailureNone {
			rejected = true
			if o.Result.Status != agent.OutcomeFailed {
				t.Errorf("rejected outcome result status = %q, want %q", o.Result.Status, agent.OutcomeFailed)
			}
		}
	}
	if !rejected {
		t.Error("a child failure was silently dropped")
	}
	if ev.Report.Succeeded() {
		t.Error("report must not claim success when a child failed")
	}
}

// TestOrchestrateDeterministicAcrossCandidateOrder proves the emitted evidence does
// not depend on the caller's candidate order.
func TestOrchestrateDeterministicAcrossCandidateOrder(t *testing.T) {
	mk := func(pairs [][2]string) DecomposeRequest {
		cands := make([]DecompositionCandidate, 0, len(pairs))
		for _, p := range pairs {
			cands = append(cands, DecompositionCandidate{TaskID: p[0], Capability: agent.Plan, Task: p[1], Scope: Scope{}})
		}
		return DecomposeRequest{
			Candidates:         cands,
			RepositoryIdentity: RepositoryIdentity{Revision: "rev-1"},
			AcceptanceCriteria: []string{"ac-1"},
			ValidationCommands: []string{"go test ./..."},
		}
	}
	base := OrchestrateRequest{
		Policy:   ExecutionPolicy{Mode: ExecutionParallel, MaxConcurrency: 4},
		Baseline: NewProgressBaseline(),
		Context:  WorkerContextRequest{TaskID: "T1", Task: "ctx"},
		Routing:  WorkerRoutingRequest{Baseline: model.ClassMedium, Capability: agent.Plan},
	}
	r1 := base
	r1.Decompose = mk([][2]string{{"T1", "a"}, {"T2", "b"}, {"T3", "c"}})
	r2 := base
	r2.Decompose = mk([][2]string{{"T3", "c"}, {"T1", "a"}, {"T2", "b"}})

	ev1, err := Orchestrate(context.Background(), okAgent(), r1)
	if err != nil {
		t.Fatal(err)
	}
	ev2, err := Orchestrate(context.Background(), okAgent(), r2)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ev1, ev2) {
		t.Error("candidate order leaked into the evidence")
	}
}

// TestOrchestrateBudgetBoundRejectsDispatch proves the pipeline is bounded: when the
// envelope admits fewer assignments than requested, NO worker runs (no partial
// dispatch).
func TestOrchestrateBudgetBoundRejectsDispatch(t *testing.T) {
	calls := 0
	a := agentFunc(func(context.Context, agent.Request) (agent.Response, error) {
		calls++
		return agent.Response{Outcome: &agent.Outcome{Status: agent.OutcomeCompleted}}, nil
	})
	req := orchRequest("a", "b")
	req.Policy.Budget = OrchestrationBudget{MaxAssignments: 1}
	ev, err := Orchestrate(context.Background(), a, req)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Errorf("worker invoked %d time(s); none may run when the envelope is exceeded", calls)
	}
	for _, o := range ev.Report.Outcomes {
		if o.Failure == FailureNone {
			t.Errorf("assignment %s was not rejected under an exceeded envelope", o.AssignmentID)
		}
	}
}

// TestOrchestrateConflictStopsDispatch proves a detected conflict prevents ALL
// workers from executing (no partial dispatch).
func TestOrchestrateConflictStopsDispatch(t *testing.T) {
	dec, err := DecomposeWork(orchDecompose("a", "b"))
	if err != nil {
		t.Fatal(err)
	}
	id0, id1 := dec.Assignments[0].AssignmentID, dec.Assignments[1].AssignmentID

	calls := 0
	a := agentFunc(func(context.Context, agent.Request) (agent.Response, error) {
		calls++
		return agent.Response{Outcome: &agent.Outcome{Status: agent.OutcomeCompleted}}, nil
	})
	req := orchRequest("a", "b")
	req.Conflict = ConflictInput{
		CurrentIdentity: RepositoryIdentity{Revision: "rev-1"},
		Units: []ConflictUnit{
			{AssignmentID: id0, Files: []string{"internal/shared/x.go"}, RepositoryIdentity: RepositoryIdentity{Revision: "rev-1"}, ResultIdentity: RepositoryIdentity{Revision: "rev-1"}},
			{AssignmentID: id1, Files: []string{"internal/shared/x.go"}, RepositoryIdentity: RepositoryIdentity{Revision: "rev-1"}, ResultIdentity: RepositoryIdentity{Revision: "rev-1"}},
		},
	}
	ev, err := Orchestrate(context.Background(), a, req)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 0 {
		t.Errorf("worker invoked %d time(s); a conflict must prevent all dispatch", calls)
	}
	for _, o := range ev.Report.Outcomes {
		if o.Failure != FailureConflict {
			t.Errorf("outcome for %s = %q, want %q", o.AssignmentID, o.Failure, FailureConflict)
		}
	}
}
