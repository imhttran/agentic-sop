package jev

import (
	"context"
	"testing"

	"github.com/imhttran/agentic-sop/internal/review"
)

// TestAnalyzerInterfaceConformance confirms the deterministic fake satisfies the
// Analyzer interface and returns a structured result with no model or network.
func TestAnalyzerInterfaceConformance(t *testing.T) {
	var a Analyzer = &FakeAnalyzer{Result: Result{Status: StatusPass, Summary: "ok"}}

	got, err := a.Analyze(context.Background(), Request{Task: "t", Criteria: "c"})
	if err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}
	if got.Status != StatusPass {
		t.Fatalf("status = %q, want %q", got.Status, StatusPass)
	}
}

// TestFakeRecordsRequest confirms the fake records the bounded context JEV
// received, and that the request carries no runtime or persistence ownership.
func TestFakeRecordsRequest(t *testing.T) {
	fake := &FakeAnalyzer{Result: Result{Status: StatusFindings, Findings: []Finding{{Severity: review.High}}}}

	req := Request{
		Task:              "task",
		Criteria:          "criteria",
		ChangedFiles:      []string{"internal/x/x.go"},
		RepositoryContext: "diff",
		ValidationResult:  "PASS",
		ReviewResult:      "PASS",
	}
	if _, err := fake.Analyze(context.Background(), req); err != nil {
		t.Fatalf("Analyze returned error: %v", err)
	}
	if len(fake.Requests) != 1 {
		t.Fatalf("recorded %d requests, want 1", len(fake.Requests))
	}
	if fake.Requests[0].Task != "task" {
		t.Fatalf("recorded task = %q, want %q", fake.Requests[0].Task, "task")
	}
}

// TestFakeReturnsError confirms a provider error propagates rather than becoming
// a pass.
func TestFakeReturnsError(t *testing.T) {
	fake := &FakeAnalyzer{Err: errInvalidStatus("BOGUS")}
	if _, err := fake.Analyze(context.Background(), Request{}); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestResultValidate(t *testing.T) {
	tests := []struct {
		name    string
		result  Result
		wantErr bool
	}{
		{name: "pass", result: Result{Status: StatusPass}},
		{name: "findings", result: Result{Status: StatusFindings, Findings: []Finding{{Severity: review.Info}}}},
		{name: "incomplete", result: Result{Status: StatusIncomplete}},
		{name: "error", result: Result{Status: StatusError}},
		{name: "empty status fails closed", result: Result{}, wantErr: true},
		{name: "unknown status fails closed", result: Result{Status: Status("WHATEVER")}, wantErr: true},
		{name: "bad severity fails closed", result: Result{Status: StatusFindings, Findings: []Finding{{Severity: review.Severity("NOPE")}}}, wantErr: true},
		{name: "pass with findings fails", result: Result{Status: StatusPass, Findings: []Finding{{Severity: review.Info}}}, wantErr: true},
		{name: "findings without findings fails", result: Result{Status: StatusFindings}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.result.Validate()
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
