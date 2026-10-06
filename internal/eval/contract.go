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
	Termination *TerminationExpectation `json:"termination,omitempty"`
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
	for field, c := range counts {
		if c == nil {
			continue
		}
		if c.Equal == nil && c.AtLeast == nil && c.AtMost == nil {
			return fmt.Errorf("eval: %s names no bound (equal, at_least, or at_most)", field)
		}
	}
	return nil
}
