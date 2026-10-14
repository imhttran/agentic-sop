package fallback

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/decision"
)

// stub is a decision provider whose result and error are scripted, so the failover
// semantics can be exercised without a backend.
type stub struct {
	name      string
	available bool
	err       error
	result    decision.DecisionResult
	calls     int
}

func (s *stub) Name() string                   { return s.name }
func (s *stub) Available(context.Context) bool { return s.available }
func (s *stub) Decide(context.Context, decision.DecisionRequest) (decision.DecisionResult, error) {
	s.calls++
	if s.err != nil {
		return decision.DecisionResult{}, s.err
	}
	r := s.result
	if r.Provider == "" {
		r.Provider = s.name
	}
	return r, nil
}

// TestMalformedPrimaryFallsBack proves a provider failure that reports a malformed
// payload (the provider classified it) yields the backup's answer, with the primary
// name preserved and an advisory fallback diagnostic.
func TestMalformedPrimaryFallsBack(t *testing.T) {
	primary := &stub{name: "clef", err: decision.ErrMalformedResponse}
	backup := &stub{name: "nimble", available: true, result: decision.DecisionResult{Model: "nimble"}}
	p := New(primary, backup)

	res, err := p.Decide(context.Background(), decision.DecisionRequest{})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if backup.calls != 1 {
		t.Fatalf("backup calls = %d, want 1", backup.calls)
	}
	if res.Provider != "clef" {
		t.Errorf("provider = %q, want the primary name clef", res.Provider)
	}
	if !p.ServedBackup() {
		t.Error("ServedBackup = false, want true")
	}
	if res.Diagnostics["served_provider"] != "nimble" || res.Diagnostics["fallback"] == "" {
		t.Errorf("fallback diagnostics = %+v, want the serving backup and a reason", res.Diagnostics)
	}
}

// TestInvalidConfidenceFallbackIsAdvisoryOnly proves a primary that fails an
// invalid-confidence classification falls back, and that the composite reports the
// primary name so advisory evidence cannot rename the configured profile.
func TestInvalidConfidenceFallbackIsAdvisoryOnly(t *testing.T) {
	primary := &stub{name: "clef", err: decision.ErrProviderFailure}
	backup := &stub{name: "nimble", available: true}
	p := New(primary, backup)

	res, err := p.Decide(context.Background(), decision.DecisionRequest{})
	if err != nil {
		t.Fatalf("Decide: %v", err)
	}
	if res.Provider != primary.Name() {
		t.Errorf("provider = %q, want %q (the configured profile must not change)", res.Provider, primary.Name())
	}
}

// TestCancelledPrimaryDoesNotAttemptBackup proves a primary that fails because the
// context is done does not silently spend the backup: the primary error is returned.
func TestCancelledPrimaryDoesNotAttemptBackup(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	primary := &stub{name: "clef", err: context.Canceled}
	backup := &stub{name: "nimble", available: true}
	p := New(primary, backup)

	if _, err := p.Decide(ctx, decision.DecisionRequest{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if backup.calls != 0 {
		t.Errorf("backup calls = %d, want 0 after a cancelled primary", backup.calls)
	}
}

// TestDeadlinePrimaryDoesNotAttemptBackup proves a primary that fails on a deadline
// does not attempt the backup (the bounded consultation budget is spent).
func TestDeadlinePrimaryDoesNotAttemptBackup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	time.Sleep(time.Millisecond)
	primary := &stub{name: "clef", err: context.DeadlineExceeded}
	backup := &stub{name: "nimble", available: true}
	p := New(primary, backup)

	if _, err := p.Decide(ctx, decision.DecisionRequest{}); err == nil {
		t.Fatal("Decide after a deadline: expected an error")
	}
	if backup.calls != 0 {
		t.Errorf("backup calls = %d, want 0 after a deadline", backup.calls)
	}
}

// TestBothUnavailableIsAnError proves the composite reports an error rather than a
// zero-value result when neither provider can answer.
func TestBothUnavailableIsAnError(t *testing.T) {
	primary := &stub{name: "clef", err: decision.ErrUnavailable}
	backup := &stub{name: "nimble", err: decision.ErrUnavailable}
	p := New(primary, backup)

	if _, err := p.Decide(context.Background(), decision.DecisionRequest{}); err == nil {
		t.Fatal("both unavailable: expected an error, not a zero-value result")
	}
}
