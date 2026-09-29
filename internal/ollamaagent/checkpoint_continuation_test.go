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

// TestUnmutatedRunCarriesContinuationCheckpoint proves the no-change diagnostic is
// a compact, bounded continuation checkpoint: the phase the run stopped in and the
// repository paths it already inspected, so the next bounded invocation resumes
// instead of repeating discovery.
func TestUnmutatedRunCarriesContinuationCheckpoint(t *testing.T) {
	dir := t.TempDir()

	// Only inspects; never mutates.
	_, srv := newFakeOllama(t, distinctToolCalls(maxIterationsImplement)...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 200

	_, err := New(cfg, dir).Execute(context.Background(), implementRequest())
	if err == nil {
		t.Fatal("expected a bounded no-change termination")
	}
	var incomplete *changeIncompleteError
	if !errors.As(err, &incomplete) {
		t.Fatalf("err = %v, want a resumable *changeIncompleteError", err)
	}
	msg := err.Error()
	for _, want := range []string{
		"made no repository change", // the deterministic continuation marker
		"termination=no_change",
		"continuation checkpoint",
		"phase=FINALIZE",
		"inspected=pkg/f0.go",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("err = %q, want %q", msg, want)
		}
	}
	// The checkpoint is bounded: it renders a few paths and a count of the rest.
	if !strings.Contains(msg, "(+") {
		t.Errorf("err = %q, want the checkpoint to summarize the remaining paths", msg)
	}
}

// TestMutationOpportunityAfterImplementNow covers requirement 5: once an unmutated
// run has been told (CHANGE_CONTINUE) to stop exploring and implement, it keeps its
// tools, so a mutation issued on the very interaction that would otherwise trigger
// finalization is executed rather than refused by a premature finalization.
func TestMutationOpportunityAfterImplementNow(t *testing.T) {
	dir := t.TempDir()

	// Discovery up to one interaction short of the unmutated late-stage threshold,
	// then a mutation landing exactly on it.
	responses := distinctToolCalls(implementLateStageAfter - 1)
	responses = append(responses,
		`{"tool":"write_file","args":{"path":"out.txt","content":"x"}}`,
		`{"status":"completed","summary":"did it","changes_expected":true}`,
	)

	fake, srv := newFakeOllama(t, responses...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 200

	h := New(cfg, dir)
	content, err := h.Execute(context.Background(), implementRequest())
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !strings.Contains(content, `"status":"completed"`) {
		t.Errorf("content = %q, want the completed outcome", content)
	}

	records := h.TraceRecords()
	if !hasEvent(records, implementContinueEvent) {
		t.Errorf("the run was never told to implement (no %s): %+v", implementContinueEvent, records)
	}
	if hasEvent(records, implementFinalizeEvent) {
		t.Errorf("finalization must not begin before the mutation opportunity: %+v", records)
	}

	// The write after the implement-now instruction executed: the mutation
	// opportunity is real, not merely nominal.
	allowed, denied := writeFileAudit(h.AuditRecords())
	if allowed != 1 || denied != 0 {
		t.Errorf("write_file allowed=%d denied=%d, want 1/0 (tools stay available after %s)", allowed, denied, implementContinueEvent)
	}
	if fake.count() == 0 {
		t.Error("the model was never called")
	}
}

// TestUnmutatedFixIsNoChangeNotFinalizationLimit proves an unmutated FIX stays
// bounded and reports a no-change (continuation) termination, never the
// finalization-limit or iteration-limit diagnostics reserved for a run that
// mutated.
func TestUnmutatedFixIsNoChangeNotFinalizationLimit(t *testing.T) {
	dir := t.TempDir()

	_, srv := newFakeOllama(t, distinctToolCalls(maxIterationsFix+4)...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 200

	_, err := New(cfg, dir).Execute(context.Background(), fixRequest())
	if err == nil {
		t.Fatal("expected a bounded no-change termination")
	}
	msg := err.Error()
	for _, notWant := range []string{"finalization_limit", "iteration_limit"} {
		if strings.Contains(msg, notWant) {
			t.Errorf("err = %q, must not report %s for an unmutated run", msg, notWant)
		}
	}
	for _, want := range []string{"made no repository change", "termination=no_change"} {
		if !strings.Contains(msg, want) {
			t.Errorf("err = %q, want %q", msg, want)
		}
	}
	var incomplete *changeIncompleteError
	if !errors.As(err, &incomplete) {
		t.Errorf("err = %v, want a resumable *changeIncompleteError", err)
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
