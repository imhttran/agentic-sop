package eval

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/imhttran/agentic-sop/internal/failure"
	"github.com/imhttran/agentic-sop/internal/runtrace"
)

// Diagnostic is one exact expectation mismatch.
type Diagnostic struct {
	Field    string `json:"field"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
	Message  string `json:"message"`
}

// Result is the deterministic outcome of evaluating one fixture against one trace.
type Result struct {
	Name        string       `json:"name"`
	Passed      bool         `json:"passed"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}

// Evaluate compares a completed run's trace against a fixture. It is pure and
// deterministic and never calls a model: a PASS or FAIL is decided from the
// recorded evidence alone. It reads no execution state and is never read by the
// lifecycle.
func Evaluate(t runtrace.Trace, f Fixture) Result {
	var diags []Diagnostic
	add := func(field, expected, actual string) {
		diags = append(diags, Diagnostic{
			Field:    field,
			Expected: expected,
			Actual:   actual,
			Message:  fmt.Sprintf("%s: expected %s, got %s", field, expected, actual),
		})
	}

	if e := f.Expected.Execution; e != nil {
		if e.Capability != nil && t.Execution.Capability != *e.Capability {
			add("execution.capability", strconv.Quote(*e.Capability), strconv.Quote(t.Execution.Capability))
		}
		if e.ModelClass != nil && t.Execution.ModelClass != *e.ModelClass {
			add("execution.model_class", strconv.Quote(*e.ModelClass), strconv.Quote(t.Execution.ModelClass))
		}
		if e.Provider != nil && t.Execution.Provider != *e.Provider {
			add("execution.provider", strconv.Quote(*e.Provider), strconv.Quote(t.Execution.Provider))
		}
	}

	if p := f.Expected.Progress; p != nil {
		checkCount("progress.discovery", p.Discovery, t.ProgressSummary.Discovery, add)
		checkCount("progress.repository_mutations", p.RepositoryMutations, t.ProgressSummary.RepositoryMutations, add)
		checkCount("progress.verification", p.Verification, t.ProgressSummary.Verification, add)
		checkCount("progress.state_transitions", p.StateTransitions, t.ProgressSummary.StateTransitions, add)
	}

	if b := f.Expected.Budgets; b != nil {
		checkCount("budgets.implement_iterations", b.ImplementIterations, t.Budgets.ImplementIterations, add)
		checkCount("budgets.fix_iterations", b.FixIterations, t.Budgets.FixIterations, add)
		checkCount("budgets.stale_iterations", b.StaleIterations, t.Budgets.StaleIterations, add)
		checkCount("budgets.tool_calls", b.ToolCalls, t.Budgets.ToolCalls, add)
	}

	if te := f.Expected.Termination; te != nil {
		if te.Stage != nil && string(t.Termination.Stage) != *te.Stage {
			add("termination.stage", strconv.Quote(*te.Stage), strconv.Quote(string(t.Termination.Stage)))
		}
		if te.Kind != nil && string(t.Termination.Kind) != *te.Kind {
			add("termination.kind", strconv.Quote(*te.Kind), strconv.Quote(string(t.Termination.Kind)))
		}
		if te.Disposition != nil && string(t.Termination.Disposition) != *te.Disposition {
			add("termination.disposition", strconv.Quote(*te.Disposition), strconv.Quote(string(t.Termination.Disposition)))
		}
		if te.Retryable != nil {
			// Retryability is derived from the recorded disposition by the
			// authoritative failure rule, so the evaluation tracks the lifecycle.
			actual := failure.Classification{Disposition: failure.Disposition(t.Termination.Disposition)}.Retryable()
			if actual != *te.Retryable {
				add("termination.retryable", strconv.FormatBool(*te.Retryable), strconv.FormatBool(actual))
			}
		}
		if te.HumanRequired != nil && t.Termination.HumanRequired != *te.HumanRequired {
			add("termination.human_required", strconv.FormatBool(*te.HumanRequired), strconv.FormatBool(t.Termination.HumanRequired))
		}
		if te.DiagnosticContains != nil && !strings.Contains(t.Termination.Diagnostic, *te.DiagnosticContains) {
			add("termination.diagnostic_contains", strconv.Quote(*te.DiagnosticContains), "not present")
		}
	}

	return Result{Name: f.Name, Passed: len(diags) == 0, Diagnostics: diags}
}

// checkCount asserts one count bound.
func checkCount(field string, a *CountAssertion, actual int, add func(field, expected, actual string)) {
	if a == nil {
		return
	}
	if a.Equal != nil && actual != *a.Equal {
		add(field, strconv.Itoa(*a.Equal), strconv.Itoa(actual))
	}
	if a.AtLeast != nil && actual < *a.AtLeast {
		add(field, "at least "+strconv.Itoa(*a.AtLeast), strconv.Itoa(actual))
	}
	if a.AtMost != nil && actual > *a.AtMost {
		add(field, "at most "+strconv.Itoa(*a.AtMost), strconv.Itoa(actual))
	}
}
