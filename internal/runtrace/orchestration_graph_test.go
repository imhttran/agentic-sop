package runtrace

// ORCH-009 graph-reconstruction tests. They are deterministic and model-free:
// they reconstruct the orchestration graph from a trace produced by the
// deterministic producer only, with no provider, model, network, or filesystem.

import (
	"reflect"
	"testing"
	"time"
)

// graphBase is a fixed timestamp so composed traces are identical across runs.
var graphBase = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

// graphRun is a two-worker success with integration, verification, and termination.
func graphRun() OrchestrationRun {
	b := graphBase
	return OrchestrationRun{
		RunID:  "R1",
		TaskID: "T1",
		Workers: []WorkerRun{
			{AssignmentID: "A1", TaskID: "T1", WorkerID: "W1", ModelClass: "MEDIUM", RoutingSource: "policy", Provider: "fake-a", Model: "fake-model-a", Started: b, Completed: b.Add(time.Minute), Accepted: true, AcceptedReason: "ok"},
			{AssignmentID: "A2", TaskID: "T1", WorkerID: "W2", ModelClass: "MEDIUM", RoutingSource: "policy", Provider: "fake-b", Model: "fake-model-b", Started: b, Completed: b.Add(time.Minute), Accepted: true, AcceptedReason: "ok"},
		},
		Integration:  []IntegrationRun{{TaskID: "T1", Inputs: []string{"A1", "A2"}, Output: "IR1", Started: b.Add(time.Minute), Completed: b.Add(90 * time.Second), Reason: "integrated"}},
		Verification: []VerificationRun{{TaskID: "T1", Command: "go test ./...", Status: "PASS", Timestamp: b.Add(2 * time.Minute)}},
		Termination:  TerminationRun{TaskID: "T1", Disposition: "PASSED", Reason: "done", Timestamp: b.Add(3 * time.Minute)},
	}
}

// assertNoDanglingEdges fails if any edge references a node that does not exist,
// so the graph is internally consistent from the trace alone.
func assertNoDanglingEdges(t *testing.T, g OrchestrationGraph) {
	t.Helper()
	nodes := map[string]bool{}
	for _, n := range g.Nodes {
		nodes[n.ID] = true
	}
	for _, e := range g.Edges {
		if !nodes[e.From] {
			t.Errorf("edge from unknown node %q: %+v", e.From, e)
		}
		if !nodes[e.To] {
			t.Errorf("edge to unknown node %q: %+v", e.To, e)
		}
	}
}

// TestReconstructOrchestrationGraphFromTraceAlone proves a recorded run reconstructs
// the orchestration graph from trace alone: every orchestration node kind is present,
// every edge resolves to an existing node, and reconstruction is deterministic.
func TestReconstructOrchestrationGraphFromTraceAlone(t *testing.T) {
	tr := Build(Inputs{RunID: "R1", OrchestrationEvents: ComposeOrchestrationEvents(graphRun())})
	g := ReconstructOrchestrationGraph(tr)

	kinds := map[GraphNodeKind]int{}
	for _, n := range g.Nodes {
		kinds[n.Kind]++
	}
	for _, want := range []GraphNodeKind{
		NodeTask, NodeAssignment, NodeWorker, NodeModelClass, NodeProviderModel,
		NodeResult, NodeIntegration, NodeVerification, NodeTermination,
	} {
		if kinds[want] == 0 {
			t.Errorf("missing node kind %q; nodes = %+v", want, g.Nodes)
		}
	}
	if kinds[NodeAssignment] != 2 {
		t.Errorf("assignment nodes = %d, want 2", kinds[NodeAssignment])
	}
	if kinds[NodeResult] != 2 {
		t.Errorf("result nodes = %d, want 2", kinds[NodeResult])
	}
	assertNoDanglingEdges(t, g)

	if g2 := ReconstructOrchestrationGraph(tr); !reflect.DeepEqual(g, g2) {
		t.Errorf("reconstruction is not deterministic")
	}
}

// TestReconstructOrchestrationGraphEmptyTrace proves an empty trace reconstructs an
// empty graph rather than inventing nodes.
func TestReconstructOrchestrationGraphEmptyTrace(t *testing.T) {
	g := ReconstructOrchestrationGraph(Build(Inputs{RunID: "R"}))
	if len(g.Nodes) != 0 || len(g.Edges) != 0 {
		t.Errorf("empty trace should reconstruct an empty graph: %+v", g)
	}
}

// TestReconstructOrchestrationGraphStagesFallback proves a trace produced before
// orchestration events were emitted still reconstructs a graph from the existing
// Stages/Verification/Termination references, with no dangling edges.
func TestReconstructOrchestrationGraphStagesFallback(t *testing.T) {
	tr := Build(Inputs{
		RunID:        "R",
		Stages:       []StageRecord{{Sequence: 1, Stage: "INTEGRATE", TaskID: "T1"}},
		Verification: []Verification{{Command: "go test ./...", Status: "PASS"}},
		Termination:  Termination{Stage: "PASSED", Disposition: "CONTINUE"},
	})
	g := ReconstructOrchestrationGraph(tr)
	if len(g.Nodes) == 0 {
		t.Errorf("fallback reconstruction produced no nodes")
	}
	assertNoDanglingEdges(t, g)
}
