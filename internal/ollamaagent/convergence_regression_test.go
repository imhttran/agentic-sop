package ollamaagent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// CONV-002 — Deterministic IMPLEMENT convergence regression.
//
// This test reproduces the current IMPLEMENT condition end-to-end with the
// scripted fake Ollama server only: a run that has sufficient context and a
// concrete target, but performs repeated *non-mutating* activity until the bound
// is reached. Unlike the existing guard tests:
//
//   - the reads here are novel, successful, and non-mutating (they hit real,
//     seeded repository files), so discovery IS credited during the discovery
//     window; and
//   - the run never mutates and never returns a terminal outcome.
//
// The credited-discovery path is what carries the stop out to
// implementNowAfter + staleIterations: while the discovery window is open each
// first-seen successful inspection resets the stale streak, and only afterwards
// do the stale turns accumulate to the bound. That bound (17 with the default
// budget) is strictly below the IMPLEMENT iteration ceiling (32), so this pins
// "stops at the no-progress bound, not at the iteration ceiling".
//
// No real model, provider, network, or Clef dependency is involved: the model is
// an in-process httptest server (newFakeOllama) and every fixture lives in a
// per-test t.TempDir(), so repeated runs are deterministic.

// TestConvergenceImplementCreditedDiscoveryStopsAtNoProgressBound drives
// IMPLEMENT with novel, successful, non-mutating reads and asserts it terminates
// with *noProgressError at the credited-discovery bound.
func TestConvergenceImplementCreditedDiscoveryStopsAtNoProgressBound(t *testing.T) {
	dir := t.TempDir()
	// Real repository files so every read succeeds (an uncredited, missing-file
	// read is what the existing guard tests already cover). More files than the
	// bound guarantees the script never runs out of novel, first-seen paths.
	seedToolFiles(t, dir, "pkg", implementNowAfter+maxNoProgressIterations+4)

	// A script of distinct, successful, non-mutating reads: enough to fill the
	// discovery window (turn <= implementNowAfter, each earning credit) and then
	// the stale allowance. No mutation and no terminal outcome is ever scripted.
	responses := make([]string, 0, implementNowAfter+maxNoProgressIterations+2)
	for i := 0; i < implementNowAfter+maxNoProgressIterations+2; i++ {
		responses = append(responses, readToolCall(i))
	}
	fake, srv := newFakeOllama(t, responses...)

	cfg := testConfig(srv.URL)
	// Raise the tool-call limit well above the script so the tool-call limit is
	// never the thing that stops the run.
	cfg.MaxToolCalls = 200

	h := New(cfg, dir)
	ev := &mutationEvidence{}
	_, err := h.ExecuteWithEvidence(context.Background(), implementRequest(), ev)

	// The run terminates with the capability no-progress error.
	var stalled *noProgressError
	if !errors.As(err, &stalled) {
		t.Fatalf("err = %v, want *noProgressError", err)
	}
	msg := err.Error()
	for _, want := range []string{
		"IMPLEMENT_NO_PROGRESS",
		"no repository progress",
		"repository_mutations=0",
		"changed_files=0",
		"termination=no_progress",
		"a retry may succeed",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("err = %q, want it to contain %q", msg, want)
		}
	}

	// It stops at the credited-discovery bound (implementNowAfter +
	// staleIterations), not at the iteration ceiling. The credited-discovery path
	// is exactly what places the stop here: the discovery-window resets buy the
	// extra turns before the stale streak accumulates to the bound.
	if got, want := fake.count(), implementNowAfter+maxNoProgressIterations; got != want {
		t.Errorf("model turns = %d, want %d (implementNowAfter + staleIterations)", got, want)
	}
	if got := fake.count(); got >= maxIterationsImplement {
		t.Errorf("model turns = %d, want strictly below the %d-iteration ceiling", got, maxIterationsImplement)
	}

	// Credited discovery really was exercised: the run inspected every seeded
	// path it could reach inside the window, so the credited path (not the
	// uncredited missing-file path) is what governed this stop.
	if got := len(h.TraceRecords()); got == 0 {
		t.Error("no trace records; the run did not exercise the discovery path")
	}
	if !strings.Contains(msg, "discovery_inspections=") {
		t.Errorf("err = %q, want a discovery_inspections count", msg)
	}

	// No mutation evidence was produced: no CHANGE transition, zero observed
	// mutation paths, and the evidence sink was never triggered.
	if hasEvent(h.TraceRecords(), implementChangeEvent) {
		t.Errorf("unexpected CHANGE transition in a non-mutating run: %+v", h.TraceRecords())
	}
	if ev.observed || len(ev.mutationPaths()) != 0 {
		t.Errorf("non-mutating run supplied mutation evidence: %+v", ev)
	}
}

// TestConvergenceImplementIsDeterministic repeats the run and asserts the exact
// same stop point and diagnostic every time, so the regression is not flaky.
func TestConvergenceImplementIsDeterministic(t *testing.T) {
	for run := 0; run < 3; run++ {
		t.Run(fmt.Sprintf("run=%d", run), func(t *testing.T) {
			dir := t.TempDir()
			seedToolFiles(t, dir, "pkg", implementNowAfter+maxNoProgressIterations+4)
			responses := make([]string, 0, implementNowAfter+maxNoProgressIterations+2)
			for i := 0; i < implementNowAfter+maxNoProgressIterations+2; i++ {
				responses = append(responses, readToolCall(i))
			}
			fake, srv := newFakeOllama(t, responses...)
			cfg := testConfig(srv.URL)
			cfg.MaxToolCalls = 200

			_, err := New(cfg, dir).Execute(context.Background(), implementRequest())
			var stalled *noProgressError
			if !errors.As(err, &stalled) {
				t.Fatalf("run %d: err = %v, want *noProgressError", run, err)
			}
			if got, want := fake.count(), implementNowAfter+maxNoProgressIterations; got != want {
				t.Errorf("run %d: model turns = %d, want %d", run, got, want)
			}
			if !strings.Contains(err.Error(), "termination=no_progress") {
				t.Errorf("run %d: err = %q, want termination=no_progress", run, err)
			}
		})
	}
}
