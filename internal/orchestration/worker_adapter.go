package orchestration

// This file implements ORCH-002.2: deterministic assignment construction and
// adapter invocation through the existing agent.Agent boundary.
//
// A WorkerAdapter satisfies both the existing Worker contract and a richer
// Assign method. It builds an agent.Request from a WorkAssignment, invokes the
// agent through the existing boundary, and maps the agent.Response back into a
// WorkResult. No provider, model, or transport concept appears here: the only
// execution entry point is agent.Agent.Generate.

import (
	"context"
	"fmt"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// AssignmentBuilder deterministically constructs a WorkAssignment from task
// data, declared scope, context, allowed tools, budget, output contract, and
// repository identity. It performs no I/O: every value is caller-supplied.
type AssignmentBuilder struct {
	AssignmentID       string
	TaskID             string
	Capability         agent.Capability
	Task               string
	Scope              Scope
	Context            string
	AllowedTools       []string
	Budget             Budget
	OutputContract     OutputContract
	RepositoryIdentity RepositoryIdentity
	AcceptanceCriteria []string
	ValidationCommands []string
}

// Build returns the WorkAssignment described by the builder. Identical builder
// inputs always produce an identical assignment. On first use the builder takes
// ownership of its slice inputs by copying them into itself, so a later
// mutation of the caller's original slices cannot change an assignment built
// from this builder, and repeated Build calls are stable.
func (b *AssignmentBuilder) Build() WorkAssignment {
	// Detach this builder from caller-owned slices. The copies become the
	// builder's own backing arrays, so subsequent mutation of the caller's
	// original variables is invisible to this builder.
	b.Scope = copyScope(b.Scope)
	b.AllowedTools = copyStrings(b.AllowedTools)
	b.OutputContract = copyOutputContract(b.OutputContract)
	b.AcceptanceCriteria = copyStrings(b.AcceptanceCriteria)
	b.ValidationCommands = copyStrings(b.ValidationCommands)

	return WorkAssignment{
		AssignmentID:       b.AssignmentID,
		TaskID:             b.TaskID,
		Capability:         b.Capability,
		Task:               b.Task,
		Scope:              copyScope(b.Scope),
		Context:            b.Context,
		AllowedTools:       copyStrings(b.AllowedTools),
		Budget:             b.Budget,
		OutputContract:     copyOutputContract(b.OutputContract),
		RepositoryIdentity: b.RepositoryIdentity,
		AcceptanceCriteria: copyStrings(b.AcceptanceCriteria),
		ValidationCommands: copyStrings(b.ValidationCommands),
	}
}

// ToAgentRequest maps a WorkAssignment onto the provider-neutral agent.Request
// boundary. Only domain/agent-neutral values cross: capability, task, a context
// rendering, the output format, the acceptance criteria and validation commands,
// and the declared deliverables (scope paths). No provider concept is leaked.
func ToAgentRequest(a WorkAssignment) agent.Request {
	return agent.Request{
		Capability:         a.Capability,
		Task:               a.Task,
		Input:              renderAssignmentInput(a),
		OutputRequirements: a.OutputContract.Format,
		AcceptanceCriteria: copyStrings(a.AcceptanceCriteria),
		ValidationCommands: copyStrings(a.ValidationCommands),
		Deliverables:       copyStrings(a.Scope.Paths),
	}
}

// renderAssignmentInput renders the assignment context into the agent request
// input. It is deterministic and includes only provider-neutral data.
func renderAssignmentInput(a WorkAssignment) string {
	if a.Context == "" {
		return a.Task
	}
	return a.Context
}

// WorkerAdapter is the canonical adapter-backed worker: it satisfies the
// existing Worker contract (Work) and adds Assign, building the agent request
// and mapping the response back into a validated WorkResult. It depends only on
// the provider-neutral agent.Agent boundary and holds no provider handle of its
// own.
type WorkerAdapter struct {
	unit  WorkUnit
	agent agent.Agent
	// Assignment supplies the boundary values (scope, repository identity, budget,
	// output contract) the agent.Request cannot carry. When zero, Assign uses the
	// unit's task ID and capability with an empty scope.
	Assignment WorkAssignment
}

// NewWorkerAdapter builds an adapter-backed worker for a single unit using the
// supplied agent. The agent is referenced only through the agent.Agent
// interface.
func NewWorkerAdapter(unit WorkUnit, a agent.Agent, assignment WorkAssignment) *WorkerAdapter {
	return &WorkerAdapter{unit: unit, agent: a, Assignment: assignment}
}

// Work satisfies the existing Worker contract, returning the unit this worker
// is responsible for. It grants no lifecycle authority.
func (w *WorkerAdapter) Work() WorkUnit { return w.unit }

// Assign invokes the agent boundary with the assignment and maps the response
// back into a WorkResult. It refuses to run without a valid assignment or a
// configured agent, returning an explicit failure rather than a success.
func (w *WorkerAdapter) Assign(ctx context.Context, a WorkAssignment) (WorkResult, error) {
	if w == nil || w.agent == nil {
		return WorkResult{}, fmt.Errorf("worker adapter has no agent configured")
	}
	if err := ValidateAssignment(a); err != nil {
		return WorkResult{}, err
	}

	response, err := w.agent.Generate(ctx, ToAgentRequest(a))
	if err != nil {
		// The transport failed: this is an explicit failure result, never success.
		return WorkResult{
			AssignmentID:       a.AssignmentID,
			Status:             agent.OutcomeFailed,
			RepositoryIdentity: a.RepositoryIdentity,
			Diagnostics: []Diagnostic{{
				Code:    "agent_error",
				Message: err.Error(),
			}},
		}, err
	}

	return FromAgentResponse(a, response), nil
}

// FromAgentResponse maps a provider-neutral agent.Response back into a
// WorkResult. When the response carries a structured agent.Outcome its status
// is adopted; otherwise the response content is treated as an unparsable
// payload and the result status is explicitly failed, never completed. Changed
// files and repository identity are carried through so the contract boundary
// can reject out-of-scope or stale results.
func FromAgentResponse(a WorkAssignment, r agent.Response) WorkResult {
	result := WorkResult{
		AssignmentID:       a.AssignmentID,
		ChangedFiles:       copyStrings(r.ChangedFiles),
		RepositoryIdentity: a.RepositoryIdentity,
		Progress:           Progress{Complete: false},
	}

	if r.Outcome != nil {
		result.Status = r.Outcome.Status
		result.Findings = oneLine(r.Outcome.Summary)
		if r.Outcome.Reason != "" {
			result.Diagnostics = append(result.Diagnostics, Diagnostic{Code: "agent_reason", Message: r.Outcome.Reason})
		}
		result.Progress.Complete = r.Outcome.Status == agent.OutcomeCompleted
		return result
	}

	// No structured outcome: the payload is unparsable model output. This is an
	// explicit failure, never reinterpreted as success.
	result.Status = agent.OutcomeFailed
	result.Diagnostics = append(result.Diagnostics, Diagnostic{
		Code:    "unparsable_payload",
		Message: "agent response carried no structured outcome",
	})
	return result
}

// oneLine returns the summary as a single-element findings slice, or nil when
// the summary is empty.
func oneLine(summary string) []string {
	if summary == "" {
		return nil
	}
	return []string{summary}
}

// copyStrings returns a copy of in, or nil when in is empty, so callers cannot
// mutate a built assignment or result through the returned slice.
func copyStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

// copyScope copies a Scope value and its slices.
func copyScope(s Scope) Scope {
	return Scope{Paths: copyStrings(s.Paths), Globs: copyStrings(s.Globs)}
}

// copyOutputContract copies an OutputContract value and its slice.
func copyOutputContract(o OutputContract) OutputContract {
	return OutputContract{Format: o.Format, RequiredFields: copyStrings(o.RequiredFields)}
}
