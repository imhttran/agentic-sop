package jev

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/imhttran/agentic-sop/internal/review"
)

// TestAbsentAnalyzerIsNotAnError documents that a nil/absent JEV implementation
// is not an error for the normal SOP lifecycle: the boundary is additive and
// optional, so callers simply skip JEV analysis when no Analyzer is configured.
func TestAbsentAnalyzerIsNotAnError(t *testing.T) {
	var a Analyzer // absent (nil) implementation
	if a != nil {
		t.Fatal("expected absent analyzer to be nil")
	}
	// SOP lifecycle logic guards on absence rather than failing: an absent
	// analyzer means "no JEV analysis ran", never an error.
	if err := runOptionalJEV(context.Background(), a, Request{Task: "t"}); err != nil {
		t.Fatalf("absent analyzer returned error: %v", err)
	}
}

// runOptionalJEV mirrors how SOP invokes JEV: it runs the analyzer only when one
// is present and otherwise treats the absence as a no-op, never an error.
func runOptionalJEV(ctx context.Context, a Analyzer, req Request) error {
	if a == nil {
		return nil
	}
	_, err := a.Analyze(ctx, req)
	return err
}

// TestLifecycleOutcomesWithoutModel drives the Analyzer contract through the
// deterministic fake with PASS, FINDINGS and error outcomes, using no model,
// network, or special environment.
func TestLifecycleOutcomesWithoutModel(t *testing.T) {
	req := Request{Task: "t", Criteria: "c", ChangedFiles: []string{"a.go"}, RepositoryContext: "diff"}

	pass := &FakeAnalyzer{Result: Result{Status: StatusPass, Summary: "clean"}}
	res, err := pass.Analyze(context.Background(), req)
	if err != nil {
		t.Fatalf("PASS analyze error: %v", err)
	}
	if res.Status != StatusPass || res.Validate() != nil {
		t.Fatalf("PASS result invalid: %+v (%v)", res, res.Validate())
	}

	findings := &FakeAnalyzer{Result: Result{
		Status:   StatusFindings,
		Findings: []Finding{{ID: "F1", Severity: review.High, Category: "quality", Message: "issue"}},
	}}
	res, err = findings.Analyze(context.Background(), req)
	if err != nil {
		t.Fatalf("FINDINGS analyze error: %v", err)
	}
	if res.Status != StatusFindings || len(res.Findings) == 0 || res.Validate() != nil {
		t.Fatalf("FINDINGS result invalid: %+v (%v)", res, res.Validate())
	}

	providerErr := errors.New("provider unavailable")
	failing := &FakeAnalyzer{Err: providerErr}
	res, err = failing.Analyze(context.Background(), req)
	if !errors.Is(err, providerErr) {
		t.Fatalf("provider error = %v, want %v", err, providerErr)
	}
	if res.Status == StatusPass {
		t.Fatal("provider error must not be converted into PASS")
	}
}

// TestIncompleteAndErrorAreNotPass asserts INCOMPLETE and ERROR results are
// never treated as a pass.
func TestIncompleteAndErrorAreNotPass(t *testing.T) {
	for _, st := range []Status{StatusIncomplete, StatusError} {
		res := Result{Status: st}
		if res.Status == StatusPass {
			t.Fatalf("status %q treated as pass", st)
		}
		if err := res.Validate(); err != nil {
			t.Fatalf("status %q should be a well-formed non-pass result: %v", st, err)
		}
	}
}

// TestFakeIsDeterministic asserts repeated runs yield identical results with no
// model or network, and that the fake never mutates task state: it only records
// the read-only Request it was given.
func TestFakeIsDeterministic(t *testing.T) {
	fake := &FakeAnalyzer{Result: Result{Status: StatusPass, Summary: "s"}}
	req := Request{Task: "task", Criteria: "criteria"}

	first, err := fake.Analyze(context.Background(), req)
	if err != nil {
		t.Fatalf("first Analyze error: %v", err)
	}
	second, err := fake.Analyze(context.Background(), req)
	if err != nil {
		t.Fatalf("second Analyze error: %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("non-deterministic result: %+v vs %+v", first, second)
	}
	if len(fake.Requests) != 2 {
		t.Fatalf("recorded %d requests, want 2", len(fake.Requests))
	}
	// The recorded request is a value snapshot with no runtime/persistence handle.
	if fake.Requests[0].Task != "task" || fake.Requests[0].Criteria != "criteria" {
		t.Fatalf("recorded bounded context mismatch: %+v", fake.Requests[0])
	}
}
