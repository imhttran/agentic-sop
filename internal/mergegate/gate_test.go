package mergegate

import (
	"context"
	"errors"
	"testing"

	"github.com/imhttran/agentic-sop/internal/github"
)

// readyInput is an Input where every required condition holds.
func readyInput() Input {
	return Input{
		PR:           github.PullRequest{Number: 7, Mergeable: "MERGEABLE"},
		Checks:       []github.Check{{Name: "ci", State: github.CheckPass}},
		TestsPassed:  true,
		ReviewPassed: true,
	}
}

type fakeMerger struct {
	err    error
	calls  int
	pr     int
	method string
}

func (f *fakeMerger) Merge(_ context.Context, prNumber int, method string) error {
	f.calls++
	f.pr = prNumber
	f.method = method
	return f.err
}

func TestMergeWhenAllConditionsHold(t *testing.T) {
	m := &fakeMerger{}
	result, err := New(m, "squash").Merge(context.Background(), readyInput())
	if err != nil {
		t.Fatalf("Merge failed: %v", err)
	}
	if result.Outcome != Merged {
		t.Errorf("outcome = %s, want MERGED", result.Outcome)
	}
	if m.calls != 1 || m.pr != 7 || m.method != "squash" {
		t.Errorf("merger calls=%d pr=%d method=%s, want 1/7/squash", m.calls, m.pr, m.method)
	}
}

func TestDefaultMethodIsSquash(t *testing.T) {
	m := &fakeMerger{}
	if _, err := New(m, "").Merge(context.Background(), readyInput()); err != nil {
		t.Fatalf("Merge failed: %v", err)
	}
	if m.method != "squash" {
		t.Errorf("method = %q, want squash", m.method)
	}
}

func TestSkippingChecksAreReady(t *testing.T) {
	in := readyInput()
	in.Checks = []github.Check{
		{Name: "ci", State: github.CheckPass},
		{Name: "docs", State: github.CheckSkipping},
	}
	m := &fakeMerger{}
	result, err := New(m, "squash").Merge(context.Background(), in)
	if err != nil {
		t.Fatalf("Merge failed: %v", err)
	}
	if result.Outcome != Merged || m.calls != 1 {
		t.Errorf("result=%+v calls=%d, want MERGED/1", result, m.calls)
	}
}

func TestBlockedConditionsNeverMerge(t *testing.T) {
	notMergeable := readyInput()
	notMergeable.PR.Mergeable = "CONFLICTING"

	tests := []struct {
		name string
		in   Input
		want Reason
	}{
		{"tests fail", func() Input { i := readyInput(); i.TestsPassed = false; return i }(), ReasonTests},
		{"review fails", func() Input { i := readyInput(); i.ReviewPassed = false; return i }(), ReasonReview},
		{"check fails", func() Input { i := readyInput(); i.Checks = []github.Check{{State: github.CheckFail}}; return i }(), ReasonChecks},
		{"check pending", func() Input { i := readyInput(); i.Checks = []github.Check{{State: github.CheckPending}}; return i }(), ReasonChecks},
		{"check unknown", func() Input { i := readyInput(); i.Checks = []github.Check{{State: github.CheckUnknown}}; return i }(), ReasonChecks},
		{"check cancelled", func() Input { i := readyInput(); i.Checks = []github.Check{{State: github.CheckCancel}}; return i }(), ReasonChecks},
		{"not mergeable", notMergeable, ReasonMergeable},
		{"mergeable unknown", func() Input { i := readyInput(); i.PR.Mergeable = "UNKNOWN"; return i }(), ReasonMergeable},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &fakeMerger{}
			result, err := New(m, "squash").Merge(context.Background(), tt.in)
			if err != nil {
				t.Fatalf("Merge failed: %v", err)
			}
			if result.Outcome != Blocked || result.Reason != tt.want {
				t.Errorf("result = %+v, want BLOCKED/%s", result, tt.want)
			}
			if m.calls != 0 {
				t.Errorf("merger must not run when blocked (calls=%d)", m.calls)
			}
		})
	}
}

func TestMergerErrorPropagates(t *testing.T) {
	m := &fakeMerger{err: errors.New("gh down")}
	if _, err := New(m, "squash").Merge(context.Background(), readyInput()); err == nil {
		t.Error("expected merger error to propagate")
	}
}

func TestCancellationPropagates(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	m := &fakeMerger{}
	_, err := New(m, "squash").Merge(ctx, readyInput())
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if m.calls != 0 {
		t.Errorf("merger must not run when canceled (calls=%d)", m.calls)
	}
}
