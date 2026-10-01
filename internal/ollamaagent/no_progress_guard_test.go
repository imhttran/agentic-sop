package ollamaagent

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// Repository no-progress guard tests.
//
// The observed failure (prompt-20261001-191507, P3-006): an IMPLEMENT run
// alternated narration ("planning") with distinct read-only tool calls and never
// mutated, consuming the whole 32-iteration budget. The pre-existing guard only
// caught *consecutive identical* turns, and an alternating shape never repeats a
// fingerprint consecutively.
//
// The authoritative progress signal is a successful repository mutation and nothing
// else. Every non-mutating turn increments the consecutive counter — a read, a
// search, an inspection, narration, a denied tool, a repeat, and a NOVEL read alike;
// only a successful mutation resets it. Once the counter reaches
// maxNoProgressIterations the run stops with a distinct IMPLEMENT_NO_PROGRESS /
// FIX_NO_PROGRESS diagnostic, well before the iteration ceiling. Repeated behavior
// is detected independently by turnProgress.

// stalledNarration is a plain-prose turn: no tool call and no final object, so the
// loop treats it as narration (planning/reasoning).
const stalledNarration = "I am still considering how to implement this change."

// TestImplementDistinctReadsAccumulateNoProgress is the core correction: distinct
// reads are activity, not progress. A run that reads a new file every turn and never
// mutates must still stop near the no-progress bound.
func TestImplementDistinctReadsAccumulateNoProgress(t *testing.T) {
	dir := t.TempDir()
	// More than the bound, every read a different file.
	responses := distinctToolCalls(maxNoProgressIterations + 3)
	fake, srv := newFakeOllama(t, responses...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 200

	_, err := New(cfg, dir).Execute(context.Background(), implementRequest())
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
	if got := fake.count(); got > maxNoProgressIterations {
		t.Errorf("chat calls = %d, want <= %d (stop near the bound, never at the ceiling)", got, maxNoProgressIterations)
	}
}

// TestImplementNoProgressStopsStalledRun is the regression for the observed shape:
// narration alternating with distinct read-only calls, zero mutations, for a full
// 32-iteration budget. The run must stop near the no-progress bound instead.
func TestImplementNoProgressStopsStalledRun(t *testing.T) {
	dir := t.TempDir()
	responses := make([]string, 0, maxIterationsImplement*2)
	for i := 0; i < maxIterationsImplement; i++ {
		responses = append(responses, stalledNarration, readToolCall(i))
	}
	fake, srv := newFakeOllama(t, responses...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 200

	_, err := New(cfg, dir).Execute(context.Background(), implementRequest())
	var stalled *noProgressError
	if !errors.As(err, &stalled) {
		t.Fatalf("err = %v, want *noProgressError", err)
	}
	if got := fake.count(); got >= maxIterationsImplement {
		t.Errorf("chat calls = %d, want well under the %d-iteration ceiling", got, maxIterationsImplement)
	}
}

// TestImplementMutationResetsNoProgress proves a successful mutation resets the
// counter: reads either side of a write never accumulate to the bound.
func TestImplementMutationResetsNoProgress(t *testing.T) {
	dir := t.TempDir()
	_, srv := newFakeOllama(t,
		readToolCall(0),
		readToolCall(1),
		`{"tool":"write_file","args":{"path":"out.txt","content":"x"}}`, // reset
		readToolCall(2),
		readToolCall(3),
		`{"status":"completed","summary":"done","changes_expected":true}`,
	)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 200

	content, err := New(cfg, dir).Execute(context.Background(), implementRequest())
	if err != nil {
		t.Fatalf("a run that mutated must not be stopped for no progress: %v", err)
	}
	if !strings.Contains(content, `"status":"completed"`) {
		t.Errorf("content = %q, want the completed outcome", content)
	}
}

// TestImplementRepeatedReadsAreStopped proves repeated reads remain protected: a
// model that reads the same file over and over is stopped by the repetition guard
// (or the no-progress guard), well before the ceiling.
func TestImplementRepeatedReadsAreStopped(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "notes.txt", "stable")
	readSame := `{"tool":"read_file","args":{"path":"notes.txt"}}`
	fake, srv := newFakeOllama(t, repeat(readSame, maxIterationsImplement)...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 200

	_, err := New(cfg, dir).Execute(context.Background(), implementRequest())
	if err == nil || !strings.Contains(err.Error(), "termination=no_progress") {
		t.Fatalf("err = %v, want a no-progress termination", err)
	}
	if got := fake.count(); got >= maxIterationsImplement {
		t.Errorf("chat calls = %d, want well under the ceiling", got)
	}
}

// TestImplementNarrationIsNotProgress proves a model's narrative is never evidence
// of progress, even when it claims it implemented the change: narration increments
// the counter like any other non-mutating turn.
func TestImplementNarrationIsNotProgress(t *testing.T) {
	dir := t.TempDir()
	claim := "I have implemented the change and updated all the required files."
	_, srv := newFakeOllama(t,
		claim, readToolCall(0),
		claim, readToolCall(1),
		claim,
	)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 200

	_, err := New(cfg, dir).Execute(context.Background(), implementRequest())
	var stalled *noProgressError
	if !errors.As(err, &stalled) {
		t.Fatalf("err = %v, want *noProgressError", err)
	}
	if !strings.Contains(err.Error(), "repository_mutations=0") {
		t.Errorf("err = %q, want it to record zero repository mutations", err)
	}
}

// TestImplementDeniedMutationIsNotProgress proves a denied/failed mutation attempt
// does not count as progress: it increments the counter like any other non-mutating
// turn, so the run still stops for no progress.
func TestImplementDeniedMutationIsNotProgress(t *testing.T) {
	dir := t.TempDir()
	_, srv := newFakeOllama(t,
		`{"tool":"write_file","args":{"path":"../escape.txt","content":"x"}}`, // refused: escapes the repo
		readToolCall(0),
		readToolCall(1),
		readToolCall(2),
		readToolCall(3),
	)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 200

	_, err := New(cfg, dir).Execute(context.Background(), implementRequest())
	var stalled *noProgressError
	if !errors.As(err, &stalled) {
		t.Fatalf("err = %v, want *noProgressError (a denied write is not progress)", err)
	}
	if !strings.Contains(err.Error(), "repository_mutations=0") {
		t.Errorf("err = %q, want zero repository mutations", err)
	}
}

// TestImplementNoProgressGuardIsSeparateFromCeiling proves the guard is a distinct,
// smaller bound: the iteration ceilings are unchanged (final safety ceilings), and
// the no-progress bound is well below them.
func TestImplementNoProgressGuardIsSeparateFromCeiling(t *testing.T) {
	if got := PolicyFor(agent.Implement).MaxIterations; got != maxIterationsImplement {
		t.Fatalf("IMPLEMENT MaxIterations = %d, want %d", got, maxIterationsImplement)
	}
	if got := PolicyFor(agent.Fix).MaxIterations; got != maxIterationsFix {
		t.Fatalf("FIX MaxIterations = %d, want %d", got, maxIterationsFix)
	}
	if maxIterationsImplement != 32 || maxIterationsFix != 24 {
		t.Fatalf("the iteration ceilings changed: implement=%d fix=%d, want 32/24", maxIterationsImplement, maxIterationsFix)
	}
	if maxNoProgressIterations <= 0 || maxNoProgressIterations >= maxIterationsFix {
		t.Fatalf("maxNoProgressIterations = %d, want a small positive bound below the ceilings", maxNoProgressIterations)
	}
}
