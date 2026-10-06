package orchestration

// This file implements ORCH-011: the opt-in, bounded end-to-end multi-agent
// execution path.
//
// Orchestrate composes the already-implemented Phase 9 components into one
// deterministic pipeline — deterministic decomposition (ORCH-003), per-assignment
// context and routing (ORCH-010), bounded worker execution with write-ownership and
// budget admission plus conflict admission (ORCH-004/005/006/008), and controlled
// integration (ORCH-007) — and returns the evidence it produced.
//
// It is provider/model-neutral: its only execution dependency is the existing
// agent.Agent interface, and it names no provider, model, or transport.
//
// AUTHORITY INVARIANT
//
// Orchestrate performs no lifecycle transition, approval, commit, retry, replan, or
// termination decision. A worker result is a claim, never success; the integrated
// view is explicitly non-authoritative and hands off to centralized verification.
// Enabling orchestration changes no SOP lifecycle, approval, retry, replan, budget,
// or provider/model default.

import (
	"context"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// OrchestrateRequest is the deterministic input to one bounded end-to-end
// orchestration run. Every field is caller-supplied evidence: the run performs no
// I/O and consults no provider.
type OrchestrateRequest struct {
	// Decompose describes the already-known work units and the uniform assignment
	// contract (scope, tools, budget, repository identity, verification contract).
	Decompose DecomposeRequest
	// Policy bounds execution (mode, concurrency, ownership) and carries the
	// ORCH-008 envelope.
	Policy ExecutionPolicy
	// Conflict is the ORCH-006 conflict input. An empty input detects no conflict and
	// the run behaves exactly as a clean run.
	Conflict ConflictInput
	// Baseline is the progress baseline integration compares its consolidated
	// evidence against (ORCH-008). The zero value compares against nothing.
	Baseline ProgressBaseline
	// Context is the worker-scoped context evidence (ORCH-010) applied to every
	// assignment.
	Context WorkerContextRequest
	// Routing is the capability/evidence-driven routing request (ORCH-010) applied to
	// every assignment.
	Routing WorkerRoutingRequest
}

// OrchestrateEvidence is the deterministic evidence one orchestration run produced.
// None of it carries lifecycle authority.
type OrchestrateEvidence struct {
	// Assignments are the bounded, scoped assignments actually dispatched, in
	// deterministic order.
	Assignments []WorkAssignment
	// Routes are the canonical model-class decisions for those assignments (routing
	// data only; no provider or model).
	Routes []WorkerRoute
	// Report is the aggregated worker-execution report (claim-level outcomes).
	Report ExecutionReport
	// View is the controlled integration of the worker claims. It is explicitly
	// non-authoritative and hands off to verification.
	View IntegratedView
	// Progress are the deterministic, non-authoritative progress records derived from
	// the report.
	Progress []ProgressRecord
}

// Orchestrate runs the bounded end-to-end multi-agent pipeline for one task and
// returns its evidence. It is pure with respect to lifecycle state: it mutates
// nothing, transitions nothing, and grants no authority. Identical inputs yield
// identical evidence.
func Orchestrate(ctx context.Context, a agent.Agent, req OrchestrateRequest) (OrchestrateEvidence, error) {
	decomposed, err := DecomposeWork(req.Decompose)
	if err != nil {
		return OrchestrateEvidence{}, err
	}

	assignments := make([]WorkAssignment, 0, len(decomposed.Assignments))
	routes := make([]WorkerRoute, 0, len(decomposed.Assignments))
	for _, asn := range decomposed.Assignments {
		prepared := PrepareWorker(WorkerPreparationRequest{
			Assignment: asn,
			Context:    req.Context,
			Routing:    req.Routing,
		})
		assignments = append(assignments, prepared.Assignment)
		routes = append(routes, prepared.Route)
	}

	report := NewCoordinator(a).RunWithConflicts(ctx, req.Policy, req.Conflict, assignments)
	return OrchestrateEvidence{
		Assignments: assignments,
		Routes:      routes,
		Report:      report,
		View:        FromReport(report),
		Progress:    ProgressFromReport(report, req.Baseline),
	}, nil
}
