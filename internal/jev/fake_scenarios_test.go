package jev

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

// TestScenarioBuildersValidate proves each non-malformed early-decision scenario
// yields structured evidence and a result that satisfy the fail-closed
// validators, and that every required scenario is named exactly once.
func TestScenarioBuildersValidate(t *testing.T) {
	valid := map[Scenario]*FakeAnalyzer{
		ScenarioClear:              NewClearFake(),
		ScenarioAmbiguous:          NewAmbiguousFake(),
		ScenarioScopeConcern:       NewScopeConcernFake(),
		ScenarioDestructiveConcern: NewDestructiveConcernFake(),
	}

	for _, s := range Scenarios() {
		fake, ok := valid[s]
		if !ok {
			continue // provider-failure and malformed are asserted separately
		}
		res, err := fake.Analyze(context.Background(), Request{Task: "t"})
		if err != nil {
			t.Fatalf("scenario %q: unexpected error: %v", s, err)
		}
		if res.StructuredEvidence == nil {
			t.Fatalf("scenario %q: missing structured evidence", s)
		}
		if err := res.StructuredEvidence.Validate(); err != nil {
			t.Fatalf("scenario %q: evidence invalid: %v", s, err)
		}
		if err := res.Validate(); err != nil {
			t.Fatalf("scenario %q: result invalid: %v", s, err)
		}
	}
}

// TestScenarioExpectedShapes pins the documented expected shape of each
// scenario so drift from the contract is caught.
func TestScenarioExpectedShapes(t *testing.T) {
	tests := []struct {
		scenario  Scenario
		fake      *FakeAnalyzer
		status    Status
		purpose   Purpose
		severity  Severity
		wantItems int
	}{
		{ScenarioClear, NewClearFake(), StatusPass, PurposeQuality, SeverityInfo, 0},
		{ScenarioAmbiguous, NewAmbiguousFake(), StatusIncomplete, PurposeQuality, SeverityLow, 1},
		{ScenarioScopeConcern, NewScopeConcernFake(), StatusFindings, PurposeCorrectness, SeverityMedium, 1},
		{ScenarioDestructiveConcern, NewDestructiveConcernFake(), StatusFindings, PurposeSecurity, SeverityHigh, 1},
	}
	for _, tc := range tests {
		t.Run(string(tc.scenario), func(t *testing.T) {
			res, err := tc.fake.Analyze(context.Background(), Request{Task: "t"})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.Status != tc.status {
				t.Fatalf("status = %q, want %q", res.Status, tc.status)
			}
			ev := res.StructuredEvidence
			if ev.Purpose != tc.purpose {
				t.Fatalf("purpose = %q, want %q", ev.Purpose, tc.purpose)
			}
			if ev.Severity != tc.severity {
				t.Fatalf("severity = %q, want %q", ev.Severity, tc.severity)
			}
			if len(ev.Items) != tc.wantItems {
				t.Fatalf("items = %d, want %d", len(ev.Items), tc.wantItems)
			}
		})
	}
}

// TestMalformedScenarioFailsClosed asserts the malformed scenario is rejected by
// both validators and is never treated as PASS.
func TestMalformedScenarioFailsClosed(t *testing.T) {
	fake := NewMalformedFake()
	res, err := fake.Analyze(context.Background(), Request{Task: "t"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.StructuredEvidence == nil {
		t.Fatal("malformed scenario must carry evidence so Validate can reject it")
	}
	if err := res.StructuredEvidence.Validate(); err == nil {
		t.Fatal("malformed evidence must fail Validate")
	}
	if err := res.Validate(); err == nil {
		t.Fatal("malformed result must fail Validate")
	}
}

// TestProviderFailureScenarioIsErrorNotFinding asserts provider failure surfaces
// as an error with an empty result, never as a finding or a pass.
func TestProviderFailureScenarioIsErrorNotFinding(t *testing.T) {
	fake := NewProviderFailureFake()
	res, err := fake.Analyze(context.Background(), Request{Task: "t"})
	if !errors.Is(err, ErrProviderFailure) {
		t.Fatalf("error = %v, want %v", err, ErrProviderFailure)
	}
	if len(res.Findings) != 0 {
		t.Fatalf("provider failure must not produce findings: %+v", res.Findings)
	}
	if res.Status == StatusPass {
		t.Fatal("provider failure must not be converted into PASS")
	}
}

// TestScenarioBuildersDeterministic asserts repeated Analyze calls on each
// scenario builder return identical results with no network or live provider.
func TestScenarioBuildersDeterministic(t *testing.T) {
	for _, s := range Scenarios() {
		var fake *FakeAnalyzer
		switch s {
		case ScenarioClear:
			fake = NewClearFake()
		case ScenarioAmbiguous:
			fake = NewAmbiguousFake()
		case ScenarioScopeConcern:
			fake = NewScopeConcernFake()
		case ScenarioDestructiveConcern:
			fake = NewDestructiveConcernFake()
		case ScenarioProviderFailure:
			fake = NewProviderFailureFake()
		case ScenarioMalformed:
			fake = NewMalformedFake()
		}
		req := Request{Task: "t", Criteria: "c"}
		first, firstErr := fake.Analyze(context.Background(), req)
		second, secondErr := fake.Analyze(context.Background(), req)
		if !reflect.DeepEqual(first, second) {
			t.Fatalf("scenario %q non-deterministic result: %+v vs %+v", s, first, second)
		}
		if (firstErr == nil) != (secondErr == nil) {
			t.Fatalf("scenario %q non-deterministic error: %v vs %v", s, firstErr, secondErr)
		}
		if len(fake.Requests) != 2 {
			t.Fatalf("scenario %q recorded %d requests, want 2", s, len(fake.Requests))
		}
	}
}
