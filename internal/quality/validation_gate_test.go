package quality

import (
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
)

// defaultPolicy exercises the real built-in policy (tests required, fix budget 3,
// fail_on critical/high), so these tests pin the shipped defaults.
func defaultPolicy() config.Quality { return config.Default().Quality }

// TestEvaluateFailsWhenRequiredValidationIsNotConfigured is the core guarantee: a
// task that requires validation cannot pass on an empty suite. A vacuously
// "passing" suite (no results) must not satisfy a required validation set.
func TestEvaluateFailsWhenRequiredValidationIsNotConfigured(t *testing.T) {
	res := Evaluate(defaultPolicy(), Input{
		ValidationRequired:   true,
		ValidationConfigured: false,
		BuildPassed:          true,
		TestPassed:           true,
	})
	if res.Decision != Fail {
		t.Fatalf("decision = %s, want FAIL when required validation is not configured", res.Decision)
	}
	if !strings.Contains(strings.Join(res.Reasons, " "), "required validation is not configured") {
		t.Errorf("reasons = %v, want the configuration reason", res.Reasons)
	}
}

// TestEvaluatePassesWhenRequiredValidationIsConfigured proves the enforcement does
// not block a properly configured run.
func TestEvaluatePassesWhenRequiredValidationIsConfigured(t *testing.T) {
	res := Evaluate(defaultPolicy(), Input{
		ValidationRequired:   true,
		ValidationConfigured: true,
		BuildPassed:          true,
		TestPassed:           true,
	})
	if res.Decision != Pass {
		t.Fatalf("decision = %s, want PASS", res.Decision)
	}
}

// TestEvaluateValidationNotRequiredIsUnchanged proves an omitted requirement (a
// verify-first, no-change, or already-satisfied task) keeps the previous behavior.
func TestEvaluateValidationNotRequiredIsUnchanged(t *testing.T) {
	res := Evaluate(defaultPolicy(), Input{BuildPassed: true, TestPassed: true})
	if res.Decision != Pass {
		t.Fatalf("decision = %s, want PASS", res.Decision)
	}
}

// TestEvaluateHumanRequiredOutranksMissingValidation preserves the human boundary:
// a human-required run is NEEDS_HUMAN even when validation is also missing.
func TestEvaluateHumanRequiredOutranksMissingValidation(t *testing.T) {
	res := Evaluate(defaultPolicy(), Input{
		HumanRequired:        true,
		ValidationRequired:   true,
		ValidationConfigured: false,
	})
	if res.Decision != NeedsHuman {
		t.Fatalf("decision = %s, want NEEDS_HUMAN (the human boundary is preserved)", res.Decision)
	}
}

// TestEvaluateFailsOnFailedValidation: a configured check that fails is not a pass.
func TestEvaluateFailsOnFailedValidation(t *testing.T) {
	res := Evaluate(defaultPolicy(), Input{
		ValidationRequired:   true,
		ValidationConfigured: true,
		BuildPassed:          false,
		TestPassed:           true,
	})
	if res.Decision != Fail {
		t.Fatalf("decision = %s, want FAIL", res.Decision)
	}
}

// TestEvaluateFailsWhenLaterCheckSkippedOrTimedOut: a configured but un-run
// (fail-fast) or timed-out check reports the category as not-passed, so the gate
// fails rather than treating the absent result as a pass.
func TestEvaluateFailsWhenLaterCheckSkippedOrTimedOut(t *testing.T) {
	res := Evaluate(defaultPolicy(), Input{
		ValidationRequired:   true,
		ValidationConfigured: true,
		BuildPassed:          true,
		TestPassed:           true,
		LintRequired:         true,
		LintPassed:           false,
	})
	if res.Decision != Fail {
		t.Fatalf("decision = %s, want FAIL when a required check did not pass", res.Decision)
	}
}
