package ollamaagent

import (
	"context"
	"strings"
	"testing"
)

// Unmutated-run steering lifecycle tests.
//
// The P3-006 dogfood failure was an IMPLEMENT that spent its whole budget on
// read-only discovery and wrote nothing: the harness nudged it to implement, the
// model ignored the one-shot nudge, and the run was finalized having changed
// nothing. These tests pin the deterministic half of the fix: the unmutated run's
// steering RECURS past the implement-now threshold instead of firing once, and
// becomes CLOSING just before the late-stage cutoff so the model is told the
// consequence of not writing while it can still write. Nothing about the bounds,
// phases, tool availability, or finalization changes.

// countAdvice counts the recorded model requests whose transcript carries each
// unmutated-run steering instruction. Advice is appended to a tool result, so it
// reaches the model on the NEXT turn's request.
func countAdvice(fake *fakeOllama, want string) int {
	n := 0
	for i := 0; i < fake.count(); i++ {
		if strings.Contains(messageText(fake.request(i)), want) {
			n++
		}
	}
	return n
}

// TestUnmutatedRunIsSteeredEveryTurn proves the unmutated-run steering recurs: past
// implementNowAfter the model is told to implement on EVERY interaction, and the
// interactions just before the late-stage cutoff become closing. A one-shot
// instruction can be outrun; a re-stated one cannot be stale for more than a turn.
func TestUnmutatedRunIsSteeredEveryTurn(t *testing.T) {
	dir := t.TempDir()

	// Read on every interaction up to the late-stage cutoff, then report truthfully.
	responses := distinctToolCalls(implementLateStageAfter)
	responses = append(responses, `{"status":"needs_human","reason":"not yet"}`)
	fake, srv := newFakeOllama(t, responses...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 200

	h := New(cfg, dir)
	if _, err := h.Execute(context.Background(), implementRequest()); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	now := countAdvice(fake, "but you have not yet made the")
	closing := countAdvice(fake, "This invocation is about to end")

	// Every interaction from implementNowAfter up to (but not including)
	// implementClosingAfter restates the standard instruction.
	if min := implementClosingAfter - implementNowAfter; now < min {
		t.Errorf("implement-now instruction seen on %d requests, want at least %d (it must recur, not fire once)", now, min)
	}
	// The closing steering lands in the interactions before the late-stage cutoff,
	// so it must have reached the model at least once.
	if min := implementLateStageAfter - implementClosingAfter; closing < min {
		t.Errorf("closing instruction seen on %d requests, want at least %d", closing, min)
	}

	// The phase transition is still recorded exactly once, so the trace is not
	// flooded by the recurring advice.
	if got := countEvent(h.TraceRecords(), implementContinueEvent); got != 1 {
		t.Errorf("%s recorded %d times, want exactly 1", implementContinueEvent, got)
	}
}

// TestMutatedRunIsNotSteeredToImplement proves the recurring steering is scoped to
// an unmutated run: a run that changed the repository is never told to implement,
// and never receives the closing instruction.
func TestMutatedRunIsNotSteeredToImplement(t *testing.T) {
	dir := t.TempDir()

	responses := []string{
		`{"tool":"write_file","args":{"path":"a.txt","content":"x"}}`,
		`{"status":"completed","summary":"done","changes_expected":true}`,
	}
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
	if now := countAdvice(fake, "but you have not yet made the"); now != 0 {
		t.Errorf("a mutated run was told to implement %d times, want 0", now)
	}
	if closing := countAdvice(fake, "This invocation is about to end"); closing != 0 {
		t.Errorf("a mutated run received the closing instruction %d times, want 0", closing)
	}
	if hasEvent(h.TraceRecords(), implementContinueEvent) {
		t.Error("a mutated run must not record the CHANGE_CONTINUE transition")
	}
}

// countEvent counts the trace records carrying an event marker.
func countEvent(records []TraceRecord, event string) int {
	n := 0
	for _, r := range records {
		if r.Event == event {
			n++
		}
	}
	return n
}
