package eval

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// This file defines the deterministic evaluation contract: a fixture is an
// expectation about a completed run, evaluated against the run's canonical
// structured trace (AGENT-001/AGENT-002). Evaluation is strictly downstream of
// execution — it observes completed evidence and never feeds a routing,
// lifecycle, retry, budget, human-boundary, or termination decision.

// Fixture is one deterministic evaluation: a scenario, described by the
// observable properties its trace must satisfy.
type Fixture struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Expected    Expectation `json:"expected"`
}

// Expectation is the observable contract. Every field is optional: a fixture
// asserts only what it names. An unknown field is rejected at parse time, so an
// unsupported expectation is an explicit error rather than a silently ignored one.
type Expectation struct {
	Execution   *ExecutionExpectation   `json:"execution,omitempty"`
	Progress    *ProgressExpectation    `json:"progress,omitempty"`
	Budgets     *BudgetExpectation      `json:"budgets,omitempty"`
	Replans     *ReplansExpectation     `json:"replans,omitempty"`
	Context     *ContextExpectation     `json:"context,omitempty"`
	Termination *TerminationExpectation `json:"termination,omitempty"`
	// Orchestration asserts the ORCH-009 orchestration graph described by the
	// composed orchestration events (see OrchestrationExpectation).
	Orchestration *OrchestrationExpectation `json:"orchestration,omitempty"`
}

// ContextExpectation asserts the Context Engine's supplied-context summary. Every
// field is optional. Sources lists source names that must be present; extra sources
// are permitted, so a fixture asserts what it requires without over-constraining the
// harness's selection.
type ContextExpectation struct {
	Items     *CountAssertion `json:"items,omitempty"`
	Files     *CountAssertion `json:"files,omitempty"`
	Bytes     *CountAssertion `json:"bytes,omitempty"`
	Truncated *bool           `json:"truncated,omitempty"`
	Sources   []string        `json:"sources,omitempty"`
}

// ReplansExpectation asserts the AGENT-005 bounded strategy changes the run
// recorded. It is observation-only: it reads the trace, never an execution state,
// and is never read by the lifecycle.
type ReplansExpectation struct {
	// Count asserts how many bounded replans the run performed.
	Count *CountAssertion `json:"count,omitempty"`
	// Reason asserts a substring of the first replan's recorded reason.
	Reason *string `json:"reason,omitempty"`
}

// BudgetExpectation asserts the deterministic execution limits that applied to the
// run.
type BudgetExpectation struct {
	ImplementIterations *CountAssertion `json:"implement_iterations,omitempty"`
	FixIterations       *CountAssertion `json:"fix_iterations,omitempty"`
	StaleIterations     *CountAssertion `json:"stale_iterations,omitempty"`
	ToolCalls           *CountAssertion `json:"tool_calls,omitempty"`
}

// ExecutionExpectation asserts what executed. Only the identity that is stable
// across environments is expected; a model name is intentionally not required.
type ExecutionExpectation struct {
	Capability *string `json:"capability,omitempty"`
	ModelClass *string `json:"model_class,omitempty"`
	Provider   *string `json:"provider,omitempty"`
}

// ProgressExpectation asserts the AGENT-002 progress-signal counts.
type ProgressExpectation struct {
	Discovery           *CountAssertion `json:"discovery,omitempty"`
	RepositoryMutations *CountAssertion `json:"repository_mutations,omitempty"`
	Verification        *CountAssertion `json:"verification,omitempty"`
	StateTransitions    *CountAssertion `json:"state_transitions,omitempty"`
}

// CountAssertion is an exact, minimum, or maximum count. At least one bound is
// required.
type CountAssertion struct {
	Equal   *int `json:"equal,omitempty"`
	AtLeast *int `json:"at_least,omitempty"`
	AtMost  *int `json:"at_most,omitempty"`
}

// TerminationExpectation asserts why the run stopped, using the existing
// taxonomy (stage, failure kind, disposition, autonomy action, human-required).
// Retryable is derived from the recorded disposition by the authoritative
// failure rule, not stored separately.
type TerminationExpectation struct {
	Stage              *string `json:"stage,omitempty"`
	Kind               *string `json:"kind,omitempty"`
	Disposition        *string `json:"disposition,omitempty"`
	Retryable          *bool   `json:"retryable,omitempty"`
	HumanRequired      *bool   `json:"human_required,omitempty"`
	DiagnosticContains *string `json:"diagnostic_contains,omitempty"`
}

// OrchestrationExpectation asserts the ORCH-009 orchestration graph described by
// the composed orchestration events. Every field is optional: a fixture asserts
// only the properties it names. It lets a deterministic, model-free fixture prove
// the orchestration trace producer works, without naming a live provider or model.
type OrchestrationExpectation struct {
	// Kinds asserts, per orchestration event kind (assignment_created,
	// worker_selected, model_class_selected, provider_resolved, worker_started,
	// worker_completed, result_accepted, result_rejected, integration_started,
	// integration_completed, verification_result, termination), how many events
	// occurred. A kind named with a zero bound asserts its absence.
	Kinds map[string]*CountAssertion `json:"kinds,omitempty"`
	// Statuses asserts, per status label (completed, failed, accepted, rejected,
	// PASS, FAIL, and the termination disposition), how many events carry it.
	Statuses map[string]*CountAssertion `json:"statuses,omitempty"`
	// Assignments asserts the number of distinct assignment nodes.
	Assignments *CountAssertion `json:"assignments,omitempty"`
	// IntegrationInputs asserts the number of worker-result references the largest
	// integration pass consumed (the integration_started event's inputs). It proves
	// a multi-worker integration consumed more than one worker result.
	IntegrationInputs *CountAssertion `json:"integration_inputs,omitempty"`
	// ReasonContains asserts substrings that must appear in the reason of some
	// event (for example a rejection or integration outcome label).
	ReasonContains []string `json:"reason_contains,omitempty"`
	// Sequence asserts the ordering invariant that makes the graph reconstructable:
	// the events form a total, unique ordering.
	Sequence *SequenceExpectation `json:"sequence,omitempty"`
}

// SequenceExpectation asserts the orchestration event ordering invariant.
type SequenceExpectation struct {
	// Total asserts the sequence numbers are exactly 1..N.
	Total *bool `json:"total,omitempty"`
	// Unique asserts no two events share a sequence number.
	Unique *bool `json:"unique,omitempty"`
}

// ParseFixture parses a fixture from JSON. Unknown fields are rejected, so a
// fixture that names an unsupported expectation fails loudly.
func ParseFixture(data []byte) (Fixture, error) {
	var f Fixture
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return Fixture{}, fmt.Errorf("eval: parse fixture: %w", err)
	}
	if strings.TrimSpace(f.Name) == "" {
		return Fixture{}, fmt.Errorf("eval: fixture name is required")
	}
	if err := f.Expected.validate(); err != nil {
		return Fixture{}, err
	}
	return f, nil
}

// LoadFixture reads and parses a fixture file.
func LoadFixture(path string) (Fixture, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Fixture{}, fmt.Errorf("eval: read fixture: %w", err)
	}
	return ParseFixture(data)
}

// validate rejects an assertion that names no bound, so a typo cannot silently
// assert nothing.
func (e Expectation) validate() error {
	counts := map[string]*CountAssertion{}
	if p := e.Progress; p != nil {
		counts["progress.discovery"] = p.Discovery
		counts["progress.repository_mutations"] = p.RepositoryMutations
		counts["progress.verification"] = p.Verification
		counts["progress.state_transitions"] = p.StateTransitions
	}
	if b := e.Budgets; b != nil {
		counts["budgets.implement_iterations"] = b.ImplementIterations
		counts["budgets.fix_iterations"] = b.FixIterations
		counts["budgets.stale_iterations"] = b.StaleIterations
		counts["budgets.tool_calls"] = b.ToolCalls
	}
	if r := e.Replans; r != nil {
		counts["replans.count"] = r.Count
	}
	if c := e.Context; c != nil {
		counts["context.items"] = c.Items
		counts["context.files"] = c.Files
		counts["context.bytes"] = c.Bytes
	}
	for field, c := range counts {
		if err := checkCountBounds(field, c); err != nil {
			return err
		}
	}
	if o := e.Orchestration; o != nil {
		for kind, c := range o.Kinds {
			if err := checkCountBounds("orchestration.kinds."+kind, c); err != nil {
				return err
			}
		}
		for status, c := range o.Statuses {
			if err := checkCountBounds("orchestration.statuses."+status, c); err != nil {
				return err
			}
		}
		if err := checkCountBounds("orchestration.assignments", o.Assignments); err != nil {
			return err
		}
		if err := checkCountBounds("orchestration.integration_inputs", o.IntegrationInputs); err != nil {
			return err
		}
	}
	return nil
}

// checkCountBounds rejects a named count assertion that names no bound.
func checkCountBounds(field string, c *CountAssertion) error {
	if c == nil {
		return nil
	}
	if c.Equal == nil && c.AtLeast == nil && c.AtMost == nil {
		return fmt.Errorf("eval: %s names no bound (equal, at_least, or at_most)", field)
	}
	return nil
}
