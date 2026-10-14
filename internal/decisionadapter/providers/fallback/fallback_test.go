package fallback

import (
	"context"
	"errors"
	"testing"

	"github.com/imhttran/agentic-sop/decision"
)

type stubProvider struct {
	name  string
	avail bool
	res   decision.DecisionResult
	err   error
	calls int
}

func (s *stubProvider) Name() string                   { return s.name }
func (s *stubProvider) Available(context.Context) bool { return s.avail }
func (s *stubProvider) Decide(context.Context, decision.DecisionRequest) (decision.DecisionResult, error) {
	s.calls++
	return s.res, s.err
}

func req() decision.DecisionRequest {
	return decision.DecisionRequest{State: "x", Questions: []decision.Question{{ID: "q", Type: decision.QuestionChoice, Criteria: "c", Choices: []string{"LOW", "HIGH"}}}}
}

func okResult(name, choice string) decision.DecisionResult {
	c := 0.9
	return decision.DecisionResult{Provider: name, Model: "m", Answers: map[string]decision.Answer{
		"q": {Type: decision.AnswerChoice, Choice: choice, Confidence: &c},
	}}
}

// TestPrimaryServes proves the backup is not consulted when the primary succeeds.
func TestPrimaryServes(t *testing.T) {
	primary := &stubProvider{name: "clef", res: okResult("clef", "HIGH")}
	backup := &stubProvider{name: "nimble", res: okResult("nimble", "LOW")}
	p := New(primary, backup)
	res, err := p.Decide(context.Background(), req())
	if err != nil || res.Answers["q"].Choice != "HIGH" {
		t.Fatalf("res=%+v err=%v, want primary HIGH", res, err)
	}
	if backup.calls != 0 || p.ServedBackup() {
		t.Errorf("backup consulted on primary success: calls=%d servedBackup=%v", backup.calls, p.ServedBackup())
	}
	if p.Name() != "clef" {
		t.Errorf("Name = %q, want clef", p.Name())
	}
}

// TestBackupServesOnPrimaryFailure proves the backup serves when the primary
// errors, and the result reports the primary (profile) name.
func TestBackupServesOnPrimaryFailure(t *testing.T) {
	primary := &stubProvider{name: "clef", err: errors.New("clef down")}
	backup := &stubProvider{name: "nimble", res: okResult("nimble", "LOW")}
	p := New(primary, backup)
	res, err := p.Decide(context.Background(), req())
	if err != nil {
		t.Fatalf("err = %v, want the backup to serve", err)
	}
	if res.Answers["q"].Choice != "LOW" || res.Provider != "clef" {
		t.Errorf("res = %+v, want LOW with provider clef", res)
	}
	if !p.ServedBackup() {
		t.Error("ServedBackup = false, want true")
	}
}

// TestBothFail proves the composite fails when both providers fail.
func TestBothFail(t *testing.T) {
	primary := &stubProvider{name: "clef", err: errors.New("clef down")}
	backup := &stubProvider{name: "nimble", err: errors.New("nimble down")}
	if _, err := New(primary, backup).Decide(context.Background(), req()); err == nil {
		t.Fatal("both failing must return an error")
	}
}

// TestAvailableEither proves availability is true when either provider is up.
func TestAvailableEither(t *testing.T) {
	p := New(&stubProvider{name: "clef", avail: false}, &stubProvider{name: "nimble", avail: true})
	if !p.Available(context.Background()) {
		t.Error("Available = false, want true (backup up)")
	}
}

// TestBackupServesRecordsDiagnostics proves the serving backend is recorded as
// advisory diagnostics only when the backup serves, and never on the primary.
func TestBackupServesRecordsDiagnostics(t *testing.T) {
	primary := &stubProvider{name: "clef", res: okResult("clef", "HIGH")}
	backup := &stubProvider{name: "nimble", res: okResult("nimble", "LOW")}
	res, err := New(primary, backup).Decide(context.Background(), req())
	if err != nil {
		t.Fatal(err)
	}
	if res.Diagnostics["served_provider"] != "" {
		t.Errorf("primary-served must not set diagnostics: %v", res.Diagnostics)
	}

	primary2 := &stubProvider{name: "clef", err: errors.New("down")}
	res2, err := New(primary2, backup).Decide(context.Background(), req())
	if err != nil {
		t.Fatal(err)
	}
	if res2.Diagnostics["served_provider"] != "nimble" || res2.Diagnostics["fallback"] == "" {
		t.Errorf("fallback diagnostics = %v, want served_provider=nimble + fallback", res2.Diagnostics)
	}
}
