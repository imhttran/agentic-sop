package eval

import (
	"fmt"
	"sort"
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

	if rp := f.Expected.Replans; rp != nil {
		checkCount("replans.count", rp.Count, len(t.Replans), add)
		if rp.Reason != nil {
			actual := ""
			if len(t.Replans) > 0 {
				actual = t.Replans[0].Reason
			}
			if !strings.Contains(actual, *rp.Reason) {
				add("replans.reason", strconv.Quote(*rp.Reason), strconv.Quote(actual))
			}
		}
	}

	if cx := f.Expected.Context; cx != nil {
		checkCount("context.items", cx.Items, t.Context.Items, add)
		checkCount("context.files", cx.Files, t.Context.Files, add)
		checkCount("context.bytes", cx.Bytes, t.Context.Bytes, add)
		if cx.Truncated != nil && t.Context.Truncated != *cx.Truncated {
			add("context.truncated", strconv.FormatBool(*cx.Truncated), strconv.FormatBool(t.Context.Truncated))
		}
		if len(cx.Sources) > 0 {
			actual := make(map[string]bool, len(t.Context.Sources))
			for _, s := range t.Context.Sources {
				actual[s.Source] = true
			}
			var missing []string
			for _, want := range cx.Sources {
				if !actual[want] {
					missing = append(missing, want)
				}
			}
			if len(missing) > 0 {
				add("context.sources", "includes "+strings.Join(cx.Sources, ","), "missing "+strings.Join(missing, ","))
			}
		}
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

	if o := f.Expected.Orchestration; o != nil {
		checkOrchestration(o, t.OrchestrationEvents, add)
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

// checkOrchestration asserts the composed orchestration graph described by the
// events. It counts kinds, statuses, and distinct assignments, checks required
// reason substrings, and verifies the sequence invariant. It is pure and
// deterministic: the same events and expectation always yield the same result.
func checkOrchestration(e *OrchestrationExpectation, events []runtrace.OrchestrationEvent, add func(field, expected, actual string)) {
	kinds := map[string]int{}
	statuses := map[string]int{}
	assignments := map[string]struct{}{}
	var reasons []string
	integrationInputs := 0
	for _, ev := range events {
		kinds[string(ev.Kind)]++
		if ev.Status != "" {
			statuses[ev.Status]++
		}
		if ev.AssignmentID != "" {
			assignments[ev.AssignmentID] = struct{}{}
		}
		if ev.Reason != "" {
			reasons = append(reasons, ev.Reason)
		}
		if ev.Kind == runtrace.EventIntegrationStarted && len(ev.Parents) > integrationInputs {
			integrationInputs = len(ev.Parents)
		}
	}
	for _, kind := range sortedCountKeys(e.Kinds) {
		checkCount("orchestration.kinds."+kind, e.Kinds[kind], kinds[kind], add)
	}
	for _, status := range sortedCountKeys(e.Statuses) {
		checkCount("orchestration.statuses."+status, e.Statuses[status], statuses[status], add)
	}
	checkCount("orchestration.assignments", e.Assignments, len(assignments), add)
	checkCount("orchestration.integration_inputs", e.IntegrationInputs, integrationInputs, add)
	for _, want := range e.ReasonContains {
		found := false
		for _, r := range reasons {
			if strings.Contains(r, want) {
				found = true
				break
			}
		}
		if !found {
			add("orchestration.reason_contains", strconv.Quote(want), "not present")
		}
	}
	if s := e.Sequence; s != nil {
		total, unique := sequenceProperties(events)
		if s.Total != nil && total != *s.Total {
			add("orchestration.sequence.total", strconv.FormatBool(*s.Total), strconv.FormatBool(total))
		}
		if s.Unique != nil && unique != *s.Unique {
			add("orchestration.sequence.unique", strconv.FormatBool(*s.Unique), strconv.FormatBool(unique))
		}
	}
}

// sequenceProperties reports whether the events' sequence numbers are exactly
// 1..N (total) and free of duplicates (unique). Both make the graph
// reconstructable from the trace alone.
func sequenceProperties(events []runtrace.OrchestrationEvent) (total, unique bool) {
	unique = true
	seen := make(map[int]bool, len(events))
	for _, ev := range events {
		if seen[ev.Sequence] {
			unique = false
		}
		seen[ev.Sequence] = true
	}
	total = unique
	if total {
		for i := 1; i <= len(events); i++ {
			if !seen[i] {
				total = false
				break
			}
		}
	}
	return total, unique
}

// sortedCountKeys returns the map keys in deterministic order, so diagnostics are
// stable across runs.
func sortedCountKeys(m map[string]*CountAssertion) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
