package ollamaagent

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// Unmutated-run continuation lifecycle tests.
//
// The CTRL006 dogfood failure was a FIX that spent its whole bounded budget on
// productive repository discovery, reached FINALIZE with no mutation, and was
// then reported to the operator as a human boundary. An unmutated run is not a
// human decision: it is a resumable continuation. These tests pin the
// deterministic half of that contract for both phased capabilities: the run stays
// bounded, it reports a no-change (continuation) termination rather than a
// finalization/iteration limit, it hands the next invocation a compact
// continuation checkpoint, and it keeps a real mutation opportunity before
// finalization withdraws its tools.

// TestUnmutatedRunCarriesContinuationCheckpoint proves the no-progress diagnostic
// is also a compact continuation checkpoint: the phase the run stopped in and the
// repository paths it already inspected, so the next bounded invocation resumes
// instead of repeating discovery.
func TestUnmutatedRunCarriesContinuationCheckpoint(t *testing.T) {
	dir := t.TempDir()

	// Only inspects; never mutates. The run must stop at the no-progress bound, not
	// consume the whole iteration budget.
	_, srv := newFakeOllama(t, distinctToolCalls(maxIterationsImplement)...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 200

	_, err := New(cfg, dir).Execute(context.Background(), implementRequest())
	if err == nil {
		t.Fatal("expected a bounded no-progress termination")
	}
	var stalled *noProgressError
	if !errors.As(err, &stalled) {
		t.Fatalf("err = %v, want a resumable *noProgressError", err)
	}
	msg := err.Error()
	for _, want := range []string{
		"IMPLEMENT_NO_PROGRESS",
		"no repository progress",
		"termination=no_progress",
		"continuation checkpoint",
		"inspected=pkg/f0.go",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("err = %q, want %q", msg, want)
		}
	}
}

// TestUnmutatedRunStopsBeforeLateMutation replaces the former mutation-opportunity
// test. With the repository no-progress guard an unmutated run is stopped after
// maxNoProgressIterations consecutive non-mutating turns, so a write that would
// only have arrived in the closing turns is never reached.
func TestUnmutatedRunStopsBeforeLateMutation(t *testing.T) {
	dir := t.TempDir()

	responses := distinctToolCalls(implementLateStageAfter - 1)
	responses = append(responses,
		`{"tool":"write_file","args":{"path":"out.txt","content":"x"}}`,
		`{"status":"completed","summary":"did it","changes_expected":true}`,
	)

	_, srv := newFakeOllama(t, responses...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 200

	_, err := New(cfg, dir).Execute(context.Background(), implementRequest())
	var stalled *noProgressError
	if !errors.As(err, &stalled) {
		t.Fatalf("err = %v, want *noProgressError", err)
	}
	// The late write was never reached: no repository mutation was observed.
	if !strings.Contains(err.Error(), "repository_mutations=0") {
		t.Errorf("err = %q, want zero mutations (the late write must not be reached)", err)
	}
}

// TestUnmutatedFixIsNoProgressNotFinalizationLimit proves an unmutated FIX stops at
// the repository no-progress bound with a no-progress diagnostic, never the
// finalization-limit or iteration-limit diagnostics reserved for a run that
// mutated.
func TestUnmutatedFixIsNoProgressNotFinalizationLimit(t *testing.T) {
	dir := t.TempDir()

	_, srv := newFakeOllama(t, distinctToolCalls(maxIterationsFix+4)...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 200

	_, err := New(cfg, dir).Execute(context.Background(), fixRequest())
	if err == nil {
		t.Fatal("expected a bounded no-progress termination")
	}
	msg := err.Error()
	for _, notWant := range []string{"finalization_limit", "iteration_limit"} {
		if strings.Contains(msg, notWant) {
			t.Errorf("err = %q, must not report %s for an unmutated run", msg, notWant)
		}
	}
	for _, want := range []string{"FIX_NO_PROGRESS", "no repository progress", "termination=no_progress"} {
		if !strings.Contains(msg, want) {
			t.Errorf("err = %q, want %q", msg, want)
		}
	}
	var stalled *noProgressError
	if !errors.As(err, &stalled) {
		t.Errorf("err = %v, want a resumable *noProgressError", err)
	}
}

// TestUnmutatedRunStaysBounded proves an unmutated run remains bounded whatever
// shape its turns take: it never runs past its iteration ceiling and always ends
// with a termination diagnostic.
func TestUnmutatedRunStaysBounded(t *testing.T) {
	dir := t.TempDir()

	// A model that only narrates: no tool call, no final object, forever.
	fake, srv := newFakeOllama(t, repeat("I am still considering the repository.", maxIterationsImplement+5)...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 200

	_, err := New(cfg, dir).Execute(context.Background(), implementRequest())
	if err == nil {
		t.Fatal("expected a bounded termination")
	}
	if fake.count() > maxIterationsImplement {
		t.Errorf("model calls = %d, want <= %d (the hard ceiling)", fake.count(), maxIterationsImplement)
	}
	if !strings.Contains(err.Error(), "termination=") {
		t.Errorf("err = %q, want a termination diagnostic", err)
	}
}

// TestCheckpointIsBoundedAndDeduplicated pins the checkpoint accumulator: it keeps
// the first-seen order, deduplicates, ignores blanks, and never grows past its
// bound, so a long discovery run cannot turn the checkpoint into a transcript.
func TestCheckpointIsBoundedAndDeduplicated(t *testing.T) {
	st := newExecutionState()
	st.recordInspected("a.go")
	st.recordInspected("a.go") // duplicate
	st.recordInspected("  ")   // blank
	st.recordInspected("b.go")
	if got := st.inspectedSummary(); got != "a.go, b.go" {
		t.Errorf("summary = %q, want %q", got, "a.go, b.go")
	}

	for i := 0; i < maxCheckpointFiles*2; i++ {
		st.recordInspected(strings.Repeat("x", i+1) + ".go")
	}
	if len(st.inspected) != maxCheckpointFiles {
		t.Errorf("inspected = %d entries, want the bound %d", len(st.inspected), maxCheckpointFiles)
	}
	var empty executionState
	if got := empty.inspectedSummary(); got != "" {
		t.Errorf("empty summary = %q, want %q", got, "")
	}
}

// repeat returns s repeated n times, so a scripted model can respond the same way
// on every turn without an unbounded slice.
func repeat(s string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = s
	}
	return out
}
