package orchestration

// This file implements ORCH-010: Context & Routing Integration.
//
// It connects two already-existing, provider-neutral Phase 8 facilities into the
// Phase 9 assignment path without introducing a second subsystem:
//
//   - the CTX-001 Context Engine (internal/context) builds the bounded,
//     worker-scoped context, so a worker receives the smallest sufficient evidence
//     (task, plan, acceptance criteria, declared scope, already-identified
//     retrieval evidence, execution, recovery, and memory) rather than the whole
//     repository; and
//   - the CTX-011 Adaptive Routing policy (internal/adaptiveroute) selects a
//     canonical, capability/evidence-driven model class for the assignment.
//
// INTEGRATION FLOW
//
//	WorkAssignment
//	  -> BuildWorkerContext   (CTX-001 Context Engine)
//	  -> RouteWorker          (CTX-011 Adaptive Routing)
//	  -> WorkAssignment (bounded Context) + WorkerRoute (canonical class)
//	  -> WorkerAdapter.Assign (existing provider-neutral agent boundary)
//
// AUTHORITY INVARIANT
//
// Routing and context selection are harness-owned and grant a worker NO additional
// authority. PrepareWorker updates ONLY WorkAssignment.Context (the bounded worker
// context); the canonical model class is returned as routing data in WorkerRoute, so
// scope, allowed tools, budget, capability, repository
// identity, output contract, and the verification contract carry through
// unchanged. Neither function selects a provider or a model: provider/model
// resolution stays behind the existing model/provider boundary, and this file
// names no provider, model, or transport.

import (
	"sort"
	"strings"

	"github.com/imhttran/agentic-sop/internal/adaptiveroute"
	"github.com/imhttran/agentic-sop/internal/agent"
	sopctx "github.com/imhttran/agentic-sop/internal/context"
	"github.com/imhttran/agentic-sop/internal/model"
)

// WorkerContextRequest is the already-known evidence for one worker's context.
// Every field is harness evidence: construction performs no retrieval, no I/O, and
// consults no provider. The structural index and BM25 retrieval (or any other
// deterministic selection) supply already-identified evidence through Retrieved.
type WorkerContextRequest struct {
	// AssignmentID, TaskID, and Task identify the assignment and its statement.
	AssignmentID string
	TaskID       string
	Task         string
	// Plan is the plan the assignment executes, when known.
	Plan string
	// AcceptanceCriteria are the assignment's caller-owned criteria.
	AcceptanceCriteria []string
	// ScopePaths are the assignment's declared repository-relative paths. They are
	// the worker's file evidence: the context includes the declared scope, never the
	// whole repository.
	ScopePaths []string
	// Retrieved is already-identified repository evidence from the structural index
	// or BM25 retrieval. It is repository evidence and outranks decision memory.
	Retrieved []sopctx.Item
	// Execution and Recovery are caller-built harness evidence (execution identity,
	// failure/continuation/escalation/replan evidence).
	Execution []sopctx.Item
	Recovery  []sopctx.Item
	// Memory is durable decision memory. It is guidance, not authority: this
	// function forces it to the lowest priority, so stale memory can never outrank
	// current task, repository, execution, or recovery evidence.
	Memory []sopctx.Item
	// Limits bound the context. A zero value resolves to the Context Engine defaults.
	Limits sopctx.Limits
}

// WorkerContext is the bounded, provider-neutral context supplied to one worker,
// with explicit provenance (the per-source contribution).
type WorkerContext struct {
	// AssignmentID is the assignment the context was built for.
	AssignmentID string
	// Rendered is the deterministic text handed to the worker.
	Rendered string
	// Items, Files, Bytes, and Lines are the retained context sizes.
	Items int
	Files int
	Bytes int
	Lines int
	// Truncated reports whether a limit dropped or shortened any item.
	Truncated bool
	// Sources is the per-source contribution, in the canonical source order. It is
	// the context's provenance.
	Sources []sopctx.SourceCount
}

// BuildWorkerContext assembles the smallest sufficient worker context from
// already-known evidence, using the CTX-001 Context Engine for ordering, bounding,
// provenance, and truncation. It performs no retrieval and reads no repository: it
// organizes evidence the caller already holds.
func BuildWorkerContext(req WorkerContextRequest) WorkerContext {
	lim := req.Limits
	if lim == (sopctx.Limits{}) {
		lim = sopctx.DefaultLimits()
	}
	c := sopctx.Build(workerContextItems(req), lim)
	return WorkerContext{
		AssignmentID: req.AssignmentID,
		Rendered:     c.Render(),
		Items:        len(c.Items()),
		Files:        c.Files(),
		Bytes:        c.Bytes(),
		Lines:        c.Lines(),
		Truncated:    c.Truncated(),
		Sources:      c.Sources(),
	}
}

// workerContextItems builds the canonical context items for a worker, using the
// Context Engine's sources and priorities. Task, plan, and acceptance criteria are
// lifecycle evidence; declared scope and retrieved evidence are repository
// evidence; execution and recovery are harness evidence; memory is forced to the
// lowest priority so it can never outrank current evidence.
func workerContextItems(req WorkerContextRequest) []sopctx.Item {
	var items []sopctx.Item

	if text := strings.TrimSpace(req.Task); text != "" {
		items = append(items, sopctx.Item{
			Source:   sopctx.SourceTask,
			Identity: req.TaskID,
			Reason:   "the assignment under execution",
			Priority: sopctx.PriorityLifecycle,
			Text:     text,
		})
	}
	if text := strings.TrimSpace(req.Plan); text != "" {
		items = append(items, sopctx.Item{
			Source:   sopctx.SourceTask,
			Identity: req.TaskID,
			Reason:   "the plan the assignment executes",
			Priority: sopctx.PriorityLifecycle,
			Text:     text,
		})
	}
	if len(req.AcceptanceCriteria) > 0 {
		items = append(items, sopctx.Item{
			Source:   sopctx.SourceTask,
			Identity: req.TaskID,
			Reason:   "the assignment's acceptance criteria",
			Priority: sopctx.PriorityLifecycle,
			Text:     strings.Join(req.AcceptanceCriteria, "\n"),
		})
	}

	// Declared scope paths: the worker's bounded file evidence, sorted so the
	// representation never depends on caller order.
	paths := append([]string(nil), req.ScopePaths...)
	sort.Strings(paths)
	for _, p := range paths {
		if strings.TrimSpace(p) == "" {
			continue
		}
		items = append(items, sopctx.Item{
			Source:   sopctx.SourceRepository,
			Identity: p,
			Reason:   "declared assignment scope",
			Priority: sopctx.PriorityRepository,
			Text:     p,
		})
	}

	// Already-identified retrieval/index evidence: repository evidence that
	// outranks memory. A caller-supplied priority is preserved unless it would let
	// retrieved evidence outrank explicit or lifecycle evidence.
	for _, it := range req.Retrieved {
		if it.Source == "" {
			it.Source = sopctx.SourceRepository
		}
		if it.Priority < sopctx.PriorityRepository {
			it.Priority = sopctx.PriorityRepository
		}
		items = append(items, it)
	}

	items = append(items, req.Execution...)
	items = append(items, req.Recovery...)

	// Memory is guidance, not authority: force the lowest priority so stale memory
	// can never outrank current task, repository, execution, or recovery evidence.
	for i := range req.Memory {
		m := req.Memory[i]
		m.Source = sopctx.SourceMemory
		m.Priority = sopctx.PriorityGeneric
		items = append(items, m)
	}

	return items
}

// WorkerRoutingRequest is the capability/evidence-driven routing request for one
// assignment. It names no provider or model: only a canonical baseline class, the
// deterministic capability, an optional operator override, and observed evidence.
type WorkerRoutingRequest struct {
	// Baseline is the deterministic router's class for the task. It bounds the
	// selection from below.
	Baseline model.Class
	// Capability scopes the evidence to comparable work.
	Capability agent.Capability
	// Override is an explicit operator model-class override; it wins when valid.
	Override model.Class
	// Evidence is the accumulated, provider-neutral evaluation evidence.
	Evidence []adaptiveroute.Outcome
	// MinSample and Threshold tune the evidence gate; zero values select the CTX-011
	// defaults.
	MinSample int
	Threshold float64
}

// WorkerRoute is the explainable, capability/evidence-driven class decision for one
// assignment. It carries a canonical model class only, never a provider or model.
type WorkerRoute struct {
	// Class is the selected canonical model class.
	Class model.Class
	// Baseline is the deterministic router's class, before adaptation.
	Baseline model.Class
	// Reason is the fixed phrase explaining the selection.
	Reason string
	// Evidence is the deterministic summary of the evidence used.
	Evidence string
}

// RouteWorker selects the canonical model class for one assignment by reusing the
// CTX-011 Adaptive Routing policy. It is pure and deterministic, and it never
// selects a provider or a model: provider/model resolution remains behind the
// existing model/provider boundary.
func RouteWorker(req WorkerRoutingRequest) WorkerRoute {
	d := adaptiveroute.Route(adaptiveroute.Input{
		Baseline:   req.Baseline,
		Capability: req.Capability,
		Override:   req.Override,
		Evidence:   req.Evidence,
		MinSample:  req.MinSample,
		Threshold:  req.Threshold,
	})
	return WorkerRoute{Class: d.Class, Baseline: d.Baseline, Reason: d.Reason, Evidence: d.Evidence}
}

// WorkerPreparationRequest is the deterministic input to PrepareWorker.
type WorkerPreparationRequest struct {
	// Assignment is the harness-built assignment to prepare.
	Assignment WorkAssignment
	// Context is the already-known evidence for the worker's bounded context.
	Context WorkerContextRequest
	// Routing is the capability/evidence-driven routing request.
	Routing WorkerRoutingRequest
}

// WorkerPreparation is the prepared assignment plus the context and routing
// provenance used to produce it.
type WorkerPreparation struct {
	// Assignment is the input assignment with ONLY its Context (the bounded worker
	// context) updated; the model class is carried separately in Route.
	Assignment WorkAssignment
	// Context is the bounded, worker-scoped context (with provenance).
	Context WorkerContext
	// Route is the canonical model-class decision (with its reason and evidence).
	Route WorkerRoute
}

// PrepareWorker connects the existing abstractions for one assignment: it builds
// the bounded worker context with the Context Engine, selects the canonical model
// class with adaptive routing, and returns the assignment with its bounded Context
// updated. The canonical model class is returned separately as routing data, so
// the provider-neutral assignment contract (ORCH-002) gains no model/provider
// field.
//
// AUTHORITY INVARIANT: routing and context selection cannot expand a worker's
// authority. Scope, allowed tools, budget, capability, repository identity, output
// contract, and the verification contract carry through unchanged, and the function
// selects no provider or model.
func PrepareWorker(req WorkerPreparationRequest) WorkerPreparation {
	a := req.Assignment

	ctxReq := req.Context
	ctxReq.AssignmentID = a.AssignmentID
	if ctxReq.TaskID == "" {
		ctxReq.TaskID = a.TaskID
	}
	if ctxReq.Task == "" {
		ctxReq.Task = a.Task
	}
	if len(ctxReq.ScopePaths) == 0 {
		ctxReq.ScopePaths = a.Scope.Paths
	}
	wc := BuildWorkerContext(ctxReq)

	route := RouteWorker(req.Routing)

	a.Context = wc.Rendered
	return WorkerPreparation{Assignment: a, Context: wc, Route: route}
}
