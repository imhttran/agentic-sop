package ollamaagent

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// Unmutated-run no-progress tests.
//
// The P3-006 dogfood failure was an IMPLEMENT that spent its whole budget on
// read-only discovery and wrote nothing. The repository no-progress guard now stops
// such a run early, so the older implement-now/closing steering is no longer
// reached by a run that never changes the repository. These tests pin that: an
// unmutated run stops with a no-progress result and never receives the steering,
// while a run that changed the repository is never told to implement.

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

// TestUnmutatedRunStopsBeforeSteering proves the unmutated-run implement-now and
// closing steering is no longer reached: the repository no-progress guard stops a
// run that never changes the repository before those thresholds, with a distinct
// no-progress result rather than the CHANGE_CONTINUE steering.
func TestUnmutatedRunStopsBeforeSteering(t *testing.T) {
	dir := t.TempDir()

	responses := distinctToolCalls(implementLateStageAfter)
	responses = append(responses, `{"status":"needs_human","reason":"not yet"}`)
	fake, srv := newFakeOllama(t, responses...)
	cfg := testConfig(srv.URL)
	cfg.MaxToolCalls = 200

	h := New(cfg, dir)
	_, err := h.Execute(context.Background(), implementRequest())
	var stalled *noProgressError
	if !errors.As(err, &stalled) {
		t.Fatalf("err = %v, want *noProgressError", err)
	}

	if now := countAdvice(fake, "but you have not yet made the"); now != 0 {
		t.Errorf("implement-now steering sent %d times to a run stopped for no progress, want 0", now)
	}
	if closing := countAdvice(fake, "This invocation is about to end"); closing != 0 {
		t.Errorf("closing steering sent %d times, want 0", closing)
	}
	if got := countEvent(h.TraceRecords(), implementContinueEvent); got != 0 {
		t.Errorf("%s recorded %d times, want 0 (the steering thresholds are never reached)", implementContinueEvent, got)
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
