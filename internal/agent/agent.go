// Package agent defines the boundary between the application and an LLM. The
// interface is intentionally provider-agnostic: callers depend on Agent, never
// on a specific model provider. Implementations must not touch the filesystem
// or workflow state.
package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Capability identifies the kind of engineering work requested from an agent.
// Capabilities are deterministic application values, never free-form model
// output.
type Capability string

const (
	DesignTests     Capability = "DESIGN_TESTS"
	Implement       Capability = "IMPLEMENT"
	DiagnoseFailure Capability = "DIAGNOSE_FAILURE"
	Fix             Capability = "FIX"
	Review          Capability = "REVIEW"
	// Plan is the general planning capability used by the planner (T004).
	Plan Capability = "PLAN"
)

// validCapabilities is the deterministic set of capabilities the transport
// accepts.
var validCapabilities = map[Capability]bool{
	DesignTests:     true,
	Implement:       true,
	DiagnoseFailure: true,
	Fix:             true,
	Review:          true,
	Plan:            true,
}

// Request describes a single agent invocation. It states what capability is
// requested, what task is being performed, the input/context to reason over,
// and the required output shape, without exposing provider-specific concepts.
type Request struct {
	Capability         Capability `json:"capability"`
	Task               string     `json:"task"`
	Input              string     `json:"input"`
	OutputRequirements string     `json:"output_requirements"`
	// Caller-owned acceptance criteria and validation commands. Model output
	// cannot supply or replace this verification contract.
	AcceptanceCriteria []string `json:"acceptance_criteria,omitempty"`
	ValidationCommands []string `json:"validation_commands,omitempty"`
	// Deliverables are the exact repository paths the task requires the agent
	// to create (for example a Markdown report). They are caller-owned task
	// data: the agent must produce them, but they grant no completion,
	// validation, or approval by themselves.
	Deliverables []string `json:"deliverables,omitempty"`
	// RawOutputEvidence reports that the task's contract explicitly requires raw
	// command output in its deliverable. It is derived from the task data (never the
	// artifact), and it tells the agent that a summary or placeholder does not
	// satisfy that requirement.
	RawOutputEvidence bool `json:"raw_output_evidence,omitempty"`
}

// Validate rejects requests the transport must not send to a harness.
func (r Request) Validate() error {
	if !validCapabilities[r.Capability] {
		return fmt.Errorf("invalid agent capability %q", r.Capability)
	}
	if strings.TrimSpace(r.Task) == "" {
		return errors.New("agent request has blank task")
	}
	return nil
}

// OutcomeStatus is the explicit result a command agent reports for a mutating
// capability (IMPLEMENT, FIX).
type OutcomeStatus string

const (
	// OutcomeCompleted: the agent finished; ChangesExpected says whether a
	// repository change was intended.
	OutcomeCompleted OutcomeStatus = "completed"
	// OutcomeNeedsHuman: the agent cannot continue without a human decision.
	OutcomeNeedsHuman OutcomeStatus = "needs_human"
	// OutcomeFailed: the agent could not complete the operation.
	OutcomeFailed OutcomeStatus = "failed"
)

// AlreadySatisfied identifies verified completion without repository mutation.
const AlreadySatisfied = "ALREADY_SATISFIED"

// AcceptanceEvidence maps a caller-owned criterion to inspected repository files.
type AcceptanceEvidence struct {
	Criterion string   `json:"criterion"`
	Paths     []string `json:"paths"`
}

// CompletionEvidence preserves the checks actually executed by the harness.
type CompletionEvidence struct {
	Acceptance         []AcceptanceEvidence `json:"acceptance"`
	ValidationCommands []string             `json:"validation_commands"`
}

// Outcome is the structured execution outcome a command agent returns.
// Completion names a no-mutation conclusion; it carries no authority by itself.
type Outcome struct {
	Status          OutcomeStatus
	Summary         string
	Reason          string
	ChangesExpected bool
	Completion      string
	Evidence        *CompletionEvidence
}

// Response is the raw content returned by an agent, plus, when the command agent
// reported one, a structured execution outcome.
type Response struct {
	Content string
	// VerifiedAlreadySatisfied is set only by a trusted executing harness, never
	// parsed from model or command-provider JSON. The lifecycle still validates.
	VerifiedAlreadySatisfied bool
	// Outcome is set when the command agent returned a structured outcome; it is
	// nil for legacy prose responses and for capabilities that return structured
	// content (plans, reviews) instead.
	Outcome *Outcome
	// ChangedFiles are the repository-relative paths the invocation changed, when
	// the provider can report them. They are authoritative mutation evidence: a
	// provider that reports them lets SOP attribute changes to the task directly
	// instead of inferring them from the working-tree diff, which may carry
	// unrelated pre-existing edits. It is nil when the provider cannot report
	// them, and consumers fall back to the working-tree diff.
	ChangedFiles []string
}

// Agent performs a capability request. Implementations must not mutate the
// filesystem or workflow state.
type Agent interface {
	Generate(ctx context.Context, request Request) (Response, error)
}
