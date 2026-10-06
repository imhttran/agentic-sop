package eval

import (
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/runtrace"
)

func strptr(s string) *string { return &s }
func intptr(i int) *int       { return &i }
func boolptr(b bool) *bool    { return &b }

// passingTrace is a minimal successful-run trace.
func passingTrace() runtrace.Trace {
	return runtrace.Build(runtrace.Inputs{
		RunID:        "T001",
		Execution:    runtrace.Execution{Capability: "implement", Provider: "command"},
		Iterations:   []runtrace.Iteration{{Sequence: 1, Phase: "PLAN"}},
		Verification: []runtrace.Verification{{Command: "true", Status: "PASS"}},
		Termination:  runtrace.Termination{Stage: "PASSED"},
	})
}

func TestEvaluateAllMatchPasses(t *testing.T) {
	f := Fixture{Name: "ok", Expected: Expectation{
		Execution:   &ExecutionExpectation{Capability: strptr("implement"), Provider: strptr("command")},
		Progress:    &ProgressExpectation{StateTransitions: &CountAssertion{AtLeast: intptr(1)}, Verification: &CountAssertion{AtLeast: intptr(1)}},
		Termination: &TerminationExpectation{Stage: strptr("PASSED"), HumanRequired: boolptr(false)},
	}}
	got := Evaluate(passingTrace(), f)
	if !got.Passed {
		t.Fatalf("expected PASS, got diagnostics %+v", got.Diagnostics)
	}
	if got.Name != "ok" {
		t.Errorf("name = %q", got.Name)
	}
}

func TestEvaluateTerminationMismatchFails(t *testing.T) {
	f := Fixture{Name: "no-progress", Expected: Expectation{
		Termination: &TerminationExpectation{Kind: strptr("NO_PROGRESS"), Disposition: strptr("BLOCK")},
	}}
	got := Evaluate(passingTrace(), f)
	if got.Passed {
		t.Fatal("expected FAIL for a mismatched termination")
	}
	fields := diagFields(got)
	if !contains(fields, "termination.kind") || !contains(fields, "termination.disposition") {
		t.Errorf("diagnostic fields = %v", fields)
	}
}

func TestEvaluateProgressMismatchFails(t *testing.T) {
	f := Fixture{Name: "p", Expected: Expectation{
		Progress: &ProgressExpectation{RepositoryMutations: &CountAssertion{Equal: intptr(3)}},
	}}
	got := Evaluate(passingTrace(), f)
	if got.Passed {
		t.Fatal("expected FAIL for a mismatched progress count")
	}
	if d := got.Diagnostics[0]; d.Field != "progress.repository_mutations" || d.Expected != "3" || d.Actual != "0" {
		t.Errorf("diagnostic = %+v", d)
	}
}

func TestEvaluateHumanRequiredMismatchFails(t *testing.T) {
	f := Fixture{Name: "h", Expected: Expectation{
		Termination: &TerminationExpectation{HumanRequired: boolptr(true)},
	}}
	got := Evaluate(passingTrace(), f)
	if got.Passed {
		t.Fatal("expected FAIL for a human-required mismatch")
	}
	if got.Diagnostics[0].Field != "termination.human_required" {
		t.Errorf("diagnostic = %+v", got.Diagnostics[0])
	}
}

func TestEvaluateMultipleMismatchesAreDeterministic(t *testing.T) {
	f := Fixture{Name: "m", Expected: Expectation{
		Execution:   &ExecutionExpectation{Capability: strptr("review")},
		Progress:    &ProgressExpectation{RepositoryMutations: &CountAssertion{AtLeast: intptr(2)}},
		Termination: &TerminationExpectation{Stage: strptr("FAILED"), HumanRequired: boolptr(true)},
	}}
	got := Evaluate(passingTrace(), f)
	if got.Passed {
		t.Fatal("expected FAIL")
	}
	want := []string{"execution.capability", "progress.repository_mutations", "termination.stage", "termination.human_required"}
	if strings.Join(diagFields(got), ",") != strings.Join(want, ",") {
		t.Errorf("fields = %v, want %v (deterministic order)", diagFields(got), want)
	}
	for _, d := range got.Diagnostics {
		if d.Message == "" {
			t.Errorf("diagnostic %+v has no message", d)
		}
	}
}

func TestEvaluateMissingExpectationIsIgnored(t *testing.T) {
	got := Evaluate(passingTrace(), Fixture{Name: "empty"})
	if !got.Passed {
		t.Fatalf("an empty expectation must pass, got %+v", got.Diagnostics)
	}
}

// TestEvaluateRetryableDerivedFromDisposition proves retryability is read from
// the authoritative failure rule, not stored separately.
func TestEvaluateRetryableDerivedFromDisposition(t *testing.T) {
	blocked := runtrace.Build(runtrace.Inputs{Termination: runtrace.Termination{Disposition: "BLOCK"}})
	if got := Evaluate(blocked, Fixture{Name: "b", Expected: Expectation{Termination: &TerminationExpectation{Retryable: boolptr(false)}}}); !got.Passed {
		t.Errorf("BLOCK must be non-retryable: %+v", got.Diagnostics)
	}
	cont := runtrace.Build(runtrace.Inputs{Termination: runtrace.Termination{Disposition: "CONTINUE"}})
	if got := Evaluate(cont, Fixture{Name: "c", Expected: Expectation{Termination: &TerminationExpectation{Retryable: boolptr(true)}}}); !got.Passed {
		t.Errorf("CONTINUE must be retryable: %+v", got.Diagnostics)
	}
}

func TestEvaluateDiagnosticContains(t *testing.T) {
	tr := runtrace.Build(runtrace.Inputs{Termination: runtrace.Termination{Diagnostic: "IMPLEMENT_NO_PROGRESS: made no progress"}})
	ok := Fixture{Name: "ok", Expected: Expectation{Termination: &TerminationExpectation{DiagnosticContains: strptr("IMPLEMENT_NO_PROGRESS")}}}
	if got := Evaluate(tr, ok); !got.Passed {
		t.Errorf("diagnostic should match: %+v", got.Diagnostics)
	}
	miss := Fixture{Name: "miss", Expected: Expectation{Termination: &TerminationExpectation{DiagnosticContains: strptr("FIX_NO_PROGRESS")}}}
	if got := Evaluate(tr, miss); got.Passed {
		t.Error("diagnostic should not match")
	}
}

func TestParseFixtureRejectsUnknownField(t *testing.T) {
	_, err := ParseFixture([]byte(`{"name":"x","expected":{"termination":{"banana":true}}}`))
	if err == nil {
		t.Fatal("an unknown expectation must be an explicit error")
	}
}

func TestParseFixtureRejectsUnboundedCount(t *testing.T) {
	_, err := ParseFixture([]byte(`{"name":"x","expected":{"progress":{"verification":{}}}}`))
	if err == nil {
		t.Fatal("a count assertion with no bound must be an error")
	}
}

func TestParseFixtureRequiresName(t *testing.T) {
	if _, err := ParseFixture([]byte(`{"expected":{}}`)); err == nil {
		t.Fatal("a fixture without a name must be an error")
	}
}

func diagFields(r Result) []string {
	out := make([]string, 0, len(r.Diagnostics))
	for _, d := range r.Diagnostics {
		out = append(out, d.Field)
	}
	return out
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
