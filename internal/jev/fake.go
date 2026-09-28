package jev

import "context"

// FakeAnalyzer is a deterministic Analyzer test double. It performs no model or
// network access and no repository mutation: it returns a canned Result (or
// error), which is exactly the shape the real boundary allows.
//
// It exists so lifecycle tests can exercise SOP policy against JEV results
// without a live model, and can also be used to confirm interface conformance.
type FakeAnalyzer struct {
	// Result is returned by Analyze.
	Result Result
	// Err, when set, is returned instead of Result.
	Err error
	// Requests records each request, so tests can assert on the bounded context
	// JEV received.
	Requests []Request
}

var _ Analyzer = (*FakeAnalyzer)(nil)

// Analyze returns the configured result or error. It never mutates state.
func (f *FakeAnalyzer) Analyze(_ context.Context, req Request) (Result, error) {
	f.Requests = append(f.Requests, req)
	if f.Err != nil {
		return Result{}, f.Err
	}
	return f.Result, nil
}
