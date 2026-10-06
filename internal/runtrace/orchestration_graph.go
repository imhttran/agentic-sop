package runtrace

// This file implements ORCH-009-C: orchestration graph reconstruction from the
// persisted trace alone.
//
// ReconstructOrchestrationGraph is a read-only constructor: it rebuilds the
// orchestration graph (nodes and edges) purely from a Trace's OrchestrationEvents
// (plus the existing Stages/Verification/Termination references) without
// re-running anything, consulting any provider, or reading any external input. It
// preserves the observation-only boundary of the runtrace package: nothing reads
// its output back to drive a routing, lifecycle, retry, progress, approval, or
// termination decision.
//
// The graph is deterministic: identical trace input yields an identical graph,
// byte for byte. Nodes and edges are emitted in a canonical order (sorted by ID),
// so a round-trip test can compare emitted and reconstructed graphs directly.
//
// REFERENCE-ONLY PAYLOAD DISCIPLINE
//
// The graph carries only opaque identifiers, enumerated labels, and bounded short
// text copied from the events. It never introduces secret material or raw
// prompt/completion content.

import (
	"sort"
	"strconv"
	"strings"
)

// GraphNodeKind is the enumerated kind of one reconstructed orchestration graph
// node.
type GraphNodeKind string

const (
	// NodeTask is the domain task node.
	NodeTask GraphNodeKind = "task"
	// NodeAssignment is an assignment node (ORCH-002 WorkAssignment).
	NodeAssignment GraphNodeKind = "assignment"
	// NodeWorker is a selected-worker node.
	NodeWorker GraphNodeKind = "worker"
	// NodeModelClass is the routing decision (model-class) node.
	NodeModelClass GraphNodeKind = "model_class"
	// NodeProviderModel is the resolved executing target (provider/model) node.
	NodeProviderModel GraphNodeKind = "provider_model"
	// NodeResult is a worker result node (accepted or rejected).
	NodeResult GraphNodeKind = "result"
	// NodeIntegration is an INTEGRATE stage node.
	NodeIntegration GraphNodeKind = "integration"
	// NodeVerification is a verification-result node.
	NodeVerification GraphNodeKind = "verification"
	// NodeTermination is the run end-state node.
	NodeTermination GraphNodeKind = "termination"
)

// GraphNode is one reconstructed orchestration graph node. ID is a stable,
// caller-readable reference; Kind is the enumerated node kind; Label is a bounded,
// enumerated label (never a verdict).
type GraphNode struct {
	ID    string        `json:"id"`
	Kind  GraphNodeKind `json:"kind"`
	Label string        `json:"label,omitempty"`
}

// GraphEdge is one reconstructed orchestration graph edge, from a node to the node
// it depends on. It is derived from the events' explicit parent references, so a
// reader of trace.json alone can rebuild the graph.
type GraphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind,omitempty"`
}

// OrchestrationGraph is the reconstructed orchestration graph. Nodes and Edges are
// emitted in canonical (ID-sorted) order, so reconstruction is deterministic.
type OrchestrationGraph struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

// graphBuilder accumulates nodes and edges by ID, deduplicating.
type graphBuilder struct {
	nodes map[string]GraphNode
	edges map[string]GraphEdge
}

func newGraphBuilder() *graphBuilder {
	return &graphBuilder{nodes: map[string]GraphNode{}, edges: map[string]GraphEdge{}}
}

func (b *graphBuilder) addNode(id string, kind GraphNodeKind, label string) {
	if id == "" {
		return
	}
	if _, ok := b.nodes[id]; ok {
		return
	}
	b.nodes[id] = GraphNode{ID: id, Kind: kind, Label: OneLine(label)}
}

func (b *graphBuilder) hasNode(id string) bool {
	_, ok := b.nodes[id]
	return ok
}

// addEdge records an edge from -> to. It drops the edge when either endpoint is
// empty or when either endpoint is not a known node, so the reconstructor never
// emits a dangling edge: the round-trip invariant is that every edge endpoint is
// a node in the emitted graph.
func (b *graphBuilder) addEdge(from, to, kind string) {
	if from == "" || to == "" {
		return
	}
	if !b.hasNode(from) || !b.hasNode(to) {
		return
	}
	key := from + "\x00" + to + "\x00" + kind
	b.edges[key] = GraphEdge{From: from, To: to, Kind: kind}
}

func (b *graphBuilder) nodesSorted() []GraphNode {
	out := make([]GraphNode, 0, len(b.nodes))
	for _, n := range b.nodes {
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (b *graphBuilder) edgesSorted() []GraphEdge {
	out := make([]GraphEdge, 0, len(b.edges))
	for _, e := range b.edges {
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].From != out[j].From {
			return out[i].From < out[j].From
		}
		if out[i].To != out[j].To {
			return out[i].To < out[j].To
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

// ReconstructOrchestrationGraph rebuilds the orchestration graph from a trace's
// OrchestrationEvents alone. It is a pure, deterministic, read-only function: it
// mutates nothing, re-emits no events, consults no provider, and performs no I/O.
//
// Stages, Verification, and Termination are consumed only as references when the
// corresponding orchestration events are absent, so a trace produced before the
// orchestration events were emitted still reconstructs a graph. The reconstruction
// never reads its own output back to drive a decision.
func ReconstructOrchestrationGraph(t Trace) OrchestrationGraph {
	b := newGraphBuilder()

	// integrationSeq tracks, per task, the sequence of the most recent
	// integration_started event, so an integration_completed event attaches to the
	// exact integration node its start created rather than to a sequence-1 guess.
	integrationSeq := map[string]int{}
	for _, ev := range t.OrchestrationEvents {
		switch ev.Kind {
		case EventAssignmentCreated:
			assignID := assignmentNodeID(ev.AssignmentID)
			b.addNode(assignID, NodeAssignment, ev.TaskID)
			if ev.TaskID != "" {
				taskID := "task:" + ev.TaskID
				b.addNode(taskID, NodeTask, "")
				b.addEdge(taskID, assignID, "created")
			}
		case EventWorkerSelected:
			wID := workerNodeID(ev.WorkerID, ev.AssignmentID)
			b.addNode(wID, NodeWorker, ev.WorkerID)
			if ev.AssignmentID != "" {
				b.addNode(assignmentNodeID(ev.AssignmentID), NodeAssignment, ev.TaskID)
			}
			b.addEdgesToParents(wID, ev.Parents, "selected")
		case EventModelClassSelected:
			mID := modelClassNodeID(ev.AssignmentID, ev.ModelClass)
			b.addNode(mID, NodeModelClass, ev.ModelClass)
			b.addEdgesToParents(mID, ev.Parents, "decided")
		case EventProviderResolved:
			pID := providerModelNodeID(ev.AssignmentID, ev.Provider, ev.Model, ev.Locality)
			b.addNode(pID, NodeProviderModel, providerModelLabel(ev))
			b.addEdgesToParents(pID, ev.Parents, "resolved")
		case EventWorkerStarted:
			wID := workerNodeID(ev.WorkerID, ev.AssignmentID)
			b.addNode(wID, NodeWorker, ev.WorkerID)
			b.addEdgesToParents(wID, ev.Parents, "started")
		case EventWorkerCompleted:
			wID := workerNodeID(ev.WorkerID, ev.AssignmentID)
			b.addNode(wID, NodeWorker, ev.WorkerID)
		case EventResultAccepted, EventResultRejected:
			rID := resultNodeID(ev.AssignmentID)
			b.addNode(rID, NodeResult, ev.Status)
			// The result depends on the worker that produced it. Only emit the edge
			// when that worker node actually exists, so a result with no preceding
			// worker event (or an empty WorkerID) does not create a phantom edge.
			wID := workerNodeID(ev.WorkerID, ev.AssignmentID)
			if b.hasNode(wID) {
				b.addEdge(rID, wID, "produced_by")
			} else if ev.AssignmentID != "" {
				assignID := assignmentNodeID(ev.AssignmentID)
				b.addNode(assignID, NodeAssignment, ev.TaskID)
				b.addEdge(rID, assignID, "produced_by")
			}
		case EventIntegrationStarted:
			iID := integrationNodeID(ev.TaskID, ev.Sequence)
			b.addNode(iID, NodeIntegration, ev.TaskID)
			for _, in := range ev.Parents {
				rID := resultNodeID(in)
				if b.hasNode(rID) {
					b.addEdge(iID, rID, "consumes")
				}
			}
			integrationSeq[ev.TaskID] = ev.Sequence
		case EventIntegrationCompleted:
			// Attach the completion to the integration node its start created, when
			// one was seen for this task; otherwise fall back to the completion's own
			// sequence. No sequence-1 heuristic is used, so retries, interleaved, or
			// out-of-order events cannot mis-associate the completion.
			iID := ""
			if seq, ok := integrationSeq[ev.TaskID]; ok {
				iID = integrationNodeID(ev.TaskID, seq)
			}
			if iID == "" || !b.hasNode(iID) {
				iID = integrationNodeID(ev.TaskID, ev.Sequence)
			}
			b.addNode(iID, NodeIntegration, ev.TaskID)
		case EventVerificationResult:
			vID := verificationNodeID(ev.TaskID, ev.Sequence)
			b.addNode(vID, NodeVerification, ev.Status)
			if seq, ok := integrationSeq[ev.TaskID]; ok {
				iID := integrationNodeID(ev.TaskID, seq)
				if b.hasNode(iID) {
					b.addEdge(vID, iID, "verifies")
				}
			}
		case EventTermination:
			tID := "termination"
			b.addNode(tID, NodeTermination, ev.Status)
			if ev.TaskID != "" {
				taskID := "task:" + ev.TaskID
				if b.hasNode(taskID) {
					b.addEdge(tID, taskID, "terminates")
				}
			}
		}
	}

	// Reference-only fallbacks: when orchestration events are absent, consume the
	// existing Stages/Verification/Termination records as references so a trace
	// produced before orchestration events were emitted still reconstructs a graph.
	// These add no verdict.
	if len(t.OrchestrationEvents) == 0 {
		for _, s := range t.Stages {
			b.addNode("stage:"+s.Stage, NodeIntegration, s.Stage)
		}
		for i, v := range t.Verification {
			id := "verification:" + v.Command + ":" + strconv.Itoa(i)
			b.addNode(id, NodeVerification, v.Status)
		}
		if t.Termination.Disposition != "" || t.Termination.Stage != "" {
			b.addNode("termination", NodeTermination, string(t.Termination.Disposition))
		}
	}

	return OrchestrationGraph{Nodes: b.nodesSorted(), Edges: b.edgesSorted()}
}

// addEdgesToParents adds edges from node id to each of its parent node IDs,
// resolving the parent reference to its canonical node ID. An edge is only added
// when the resolved parent (and, implicitly, the child) is a known node, so the
// emitted graph never contains a dangling edge.
func (b *graphBuilder) addEdgesToParents(id string, parents []string, kind string) {
	for _, p := range parents {
		target, ok := b.canonicalParentID(p)
		if !ok {
			continue
		}
		b.addEdge(id, target, kind)
	}
}

// canonicalParentID resolves a producer-emitted parent reference to the canonical
// node ID used by the reconstructor, reporting whether the reference names a node
// kind it can resolve.
//
// The producer (ComposeOrchestrationEvents) emits parent references as either
// "assignment:<id>" (already a canonical assignment node ID) or "worker:<workerID>"
// / "worker:<assignmentID>" (the parent-side reference for a worker node). Worker
// node IDs are built by workerNodeID(workerID, assignmentID), which is
// "worker:<workerID>" (or "worker:<assignmentID>" when the worker ID is empty), so
// the worker reference is passed through unchanged and now matches the node the
// reconstruction added. A reference that carries no recognizable prefix is
// rejected rather than passed through as a dangling target.
func (b *graphBuilder) canonicalParentID(ref string) (string, bool) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", false
	}
	switch {
	case strings.HasPrefix(ref, "assignment:"):
		id := assignmentNodeID(strings.TrimPrefix(ref, "assignment:"))
		return id, true
	case strings.HasPrefix(ref, "worker:"):
		return ref, true
	case strings.HasPrefix(ref, "task:"):
		return ref, true
	case strings.HasPrefix(ref, "model_class:"):
		return ref, true
	case strings.HasPrefix(ref, "provider_model:"):
		return ref, true
	case strings.HasPrefix(ref, "result:"):
		return ref, true
	case strings.HasPrefix(ref, "integration:"):
		return ref, true
	case strings.HasPrefix(ref, "verification:"):
		return ref, true
	}
	// A bare reference is treated as an assignment ID, the most common
	// parent-reference shape emitted by the producer.
	return assignmentNodeID(ref), true
}

func resultNodeID(assignmentID string) string { return "result:" + assignmentID }

func modelClassNodeID(assignmentID, class string) string {
	return "model_class:" + assignmentID + ":" + class
}

func providerModelNodeID(assignmentID, provider, model, locality string) string {
	return "provider_model:" + assignmentID + ":" + provider + ":" + model + ":" + locality
}

func providerModelLabel(ev OrchestrationEvent) string {
	parts := ev.Provider
	if ev.Model != "" {
		if parts != "" {
			parts += "/"
		}
		parts += ev.Model
	}
	if ev.Locality != "" {
		if parts != "" {
			parts += "@"
		}
		parts += ev.Locality
	}
	return parts
}

func integrationNodeID(taskID string, seq int) string {
	return "integration:" + taskID + ":" + strconv.Itoa(seq)
}

func verificationNodeID(taskID string, seq int) string {
	return "verification:" + taskID + ":" + strconv.Itoa(seq)
}
