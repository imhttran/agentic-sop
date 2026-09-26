package ciremediation

import (
	"context"
	"errors"
	"testing"

	"github.com/imhttran/agentic-sop/internal/github"
)

func checks(state github.CheckState) []github.Check {
	return []github.Check{{Name: "ci", State: state}}
}

type fakeCI struct {
	sequences [][]github.Check
	idx       int
	logs      string
	checkErr  error
	logsErr   error
}

func (f *fakeCI) Checks(context.Context) ([]github.Check, error) {
	if f.checkErr != nil {
		return nil, f.checkErr
	}
	if f.idx < len(f.sequences) {
		result := f.sequences[f.idx]
		f.idx++
		return result, nil
	}
	if len(f.sequences) > 0 {
		return f.sequences[len(f.sequences)-1], nil
	}
	return nil, nil
}

func (f *fakeCI) FailureLogs(context.Context) (string, error) {
	if f.logsErr != nil {
		return "", f.logsErr
	}
	return f.logs, nil
}

type fakeRemediator struct {
	err   error
	calls int
}

func (f *fakeRemediator) Attempt(context.Context, Failure) error {
	f.calls++
	return f.err
}

func TestPassWhenNoFailingChecks(t *testing.T) {
	r := &fakeRemediator{}
	result, err := NewLoop(&fakeCI{sequences: [][]github.Check{checks(github.CheckPass)}}, r, 3, nil).Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result.Outcome != Pass || result.Attempts != 0 {
		t.Errorf("result = %+v, want PASS/0", result)
	}
	if r.calls != 0 {
		t.Errorf("remediator calls = %d, want 0", r.calls)
	}
}

func TestBlockedWhenNotActionable(t *testing.T) {
	r := &fakeRemediator{}
	ci := &fakeCI{sequences: [][]github.Check{checks(github.CheckFail)}, logs: ""}
	result, err := NewLoop(ci, r, 3, nil).Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result.Outcome != Blocked {
		t.Errorf("outcome = %s, want BLOCKED", result.Outcome)
	}
	if r.calls != 0 {
		t.Errorf("remediator must not run for a non-actionable failure (calls=%d)", r.calls)
	}
}

func TestRemediateThenPass(t *testing.T) {
	ci := &fakeCI{
		sequences: [][]github.Check{checks(github.CheckFail), checks(github.CheckFail), checks(github.CheckPass)},
		logs:      "compile error",
	}
	r := &fakeRemediator{}
	result, err := NewLoop(ci, r, 3, nil).Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result.Outcome != Pass || result.Attempts != 2 {
		t.Errorf("result = %+v, want PASS/2", result)
	}
	if r.calls != 2 {
		t.Errorf("remediator calls = %d, want 2", r.calls)
	}
}

func TestExhaustion(t *testing.T) {
	ci := &fakeCI{sequences: [][]github.Check{checks(github.CheckFail)}, logs: "boom"}
	r := &fakeRemediator{}
	result, err := NewLoop(ci, r, 3, nil).Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result.Outcome != Blocked || result.Attempts != 3 {
		t.Errorf("result = %+v, want BLOCKED/3", result)
	}
	if r.calls != 3 {
		t.Errorf("remediator calls = %d, want 3", r.calls)
	}
}

func TestCustomClassifierCanBlock(t *testing.T) {
	ci := &fakeCI{sequences: [][]github.Check{checks(github.CheckFail)}, logs: "boom"}
	r := &fakeRemediator{}
	result, err := NewLoop(ci, r, 3, func(Failure) bool { return false }).Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if result.Outcome != Blocked || r.calls != 0 {
		t.Errorf("result = %+v calls = %d, want BLOCKED/0", result, r.calls)
	}
}

func TestErrorsPropagate(t *testing.T) {
	if _, err := NewLoop(&fakeCI{checkErr: errors.New("gh down")}, &fakeRemediator{}, 3, nil).Run(context.Background()); err == nil {
		t.Error("expected CI error to propagate")
	}
	ci := &fakeCI{sequences: [][]github.Check{checks(github.CheckFail)}, logs: "boom"}
	if _, err := NewLoop(ci, &fakeRemediator{err: errors.New("fix failed")}, 3, nil).Run(context.Background()); err == nil {
		t.Error("expected remediator error to propagate")
	}
}

func TestCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewLoop(&fakeCI{}, &fakeRemediator{}, 3, nil).Run(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}
