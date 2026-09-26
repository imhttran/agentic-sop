package review

import (
	"context"
	"errors"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/testrunner"
)

// --- fakes ---

type fakeAgent struct {
	content  string
	err      error
	requests []agent.Request
}

func (f *fakeAgent) Generate(_ context.Context, req agent.Request) (agent.Response, error) {
	f.requests = append(f.requests, req)
	if f.err != nil {
		return agent.Response{}, f.err
	}
	return agent.Response{Content: f.content}, nil
}

type fakeProvider struct {
	reports []Report
	err     error
	calls   int
}

func (f *fakeProvider) Review(context.Context, Request) (Report, error) {
	if f.err != nil {
		return Report{}, f.err
	}
	var report Report
	if f.calls < len(f.reports) {
		report = f.reports[f.calls]
	} else if len(f.reports) > 0 {
		report = f.reports[len(f.reports)-1]
	}
	f.calls++
	return report, nil
}

type fakeFixer struct {
	err   error
	calls int
}

func (f *fakeFixer) Fix(context.Context, Report) error {
	f.calls++
	return f.err
}

type fakeTester struct {
	results []testrunner.Result
	calls   int
}

func (f *fakeTester) Run(context.Context) testrunner.Result {
	result := testrunner.Result{Status: testrunner.Pass}
	if f.calls < len(f.results) {
		result = f.results[f.calls]
	}
	f.calls++
	return result
}

func finding(severity Severity) Finding { return Finding{Severity: severity, Title: "issue"} }

// --- tests ---

func TestReportBlocking(t *testing.T) {
	cases := []struct {
		name      string
		findings  []Finding
		threshold Severity
		want      bool
	}{
		{"empty", nil, Medium, false},
		{"below threshold", []Finding{finding(Low)}, Medium, false},
		{"at threshold", []Finding{finding(Medium)}, Medium, true},
		{"above threshold", []Finding{finding(Critical)}, High, true},
		{"mixed", []Finding{finding(Info), finding(High)}, High, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report := Report{Findings: tc.findings}
			if got := report.Blocking(tc.threshold); got != tc.want {
				t.Errorf("Blocking(%s) = %v, want %v", tc.threshold, got, tc.want)
			}
		})
	}
}

func TestAgentProviderParsesFindings(t *testing.T) {
	a := &fakeAgent{content: `{"summary":"ok","findings":[{"severity":"HIGH","title":"bug","detail":"d","file":"x.go","line":3,"suggestion":"fix"}]}`}
	report, err := NewAgentProvider(a).Review(context.Background(), Request{Task: "T1"})
	if err != nil {
		t.Fatalf("Review failed: %v", err)
	}
	if len(report.Findings) != 1 || report.Findings[0].Severity != High || report.Findings[0].Line != 3 {
		t.Errorf("findings = %+v", report.Findings)
	}
	if len(a.requests) != 1 || a.requests[0].Capability != agent.Review {
		t.Errorf("agent request = %+v, want REVIEW", a.requests)
	}
}

func TestAgentProviderRejectsMalformed(t *testing.T) {
	if _, err := NewAgentProvider(&fakeAgent{content: "not json"}).Review(context.Background(), Request{}); err == nil {
		t.Fatal("expected error for malformed review output")
	}
}

func TestAgentProviderRejectsUnknownSeverity(t *testing.T) {
	a := &fakeAgent{content: `{"summary":"","findings":[{"severity":"NOPE","title":"x"}]}`}
	if _, err := NewAgentProvider(a).Review(context.Background(), Request{}); err == nil {
		t.Fatal("expected error for unknown severity")
	}
}

func TestAgentProviderPropagatesAgentError(t *testing.T) {
	a := &fakeAgent{err: errors.New("agent down")}
	if _, err := NewAgentProvider(a).Review(context.Background(), Request{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestLoopPassesWithNoBlockingFindings(t *testing.T) {
	p := &fakeProvider{reports: []Report{{Findings: []Finding{finding(Low)}}}}
	fixer := &fakeFixer{}
	tester := &fakeTester{}

	if _, err := NewLoop(p, fixer, tester, High, 3).Run(context.Background(), Request{}); err != nil {
		t.Fatalf("expected pass, got %v", err)
	}
	if fixer.calls != 0 {
		t.Errorf("fixer calls = %d, want 0", fixer.calls)
	}
}

func TestLoopFixesThenPasses(t *testing.T) {
	p := &fakeProvider{reports: []Report{
		{Findings: []Finding{finding(High)}},
		{Findings: nil},
	}}
	fixer := &fakeFixer{}
	tester := &fakeTester{results: []testrunner.Result{{Status: testrunner.Pass}}}

	report, err := NewLoop(p, fixer, tester, High, 3).Run(context.Background(), Request{})
	if err != nil {
		t.Fatalf("expected pass, got %v", err)
	}
	if report.Blocking(High) {
		t.Error("final report still blocking")
	}
	if fixer.calls != 1 || tester.calls != 1 {
		t.Errorf("fixer=%d tester=%d, want 1/1", fixer.calls, tester.calls)
	}
}

func TestLoopExhaustion(t *testing.T) {
	p := &fakeProvider{reports: []Report{{Findings: []Finding{finding(Critical)}}}}
	fixer := &fakeFixer{}
	tester := &fakeTester{}

	_, err := NewLoop(p, fixer, tester, High, 3).Run(context.Background(), Request{})
	if err == nil {
		t.Fatal("expected exhaustion error")
	}
	if fixer.calls != 2 {
		t.Errorf("fixer calls = %d, want 2 (rounds-1)", fixer.calls)
	}
}

func TestLoopFixBreaksTests(t *testing.T) {
	p := &fakeProvider{reports: []Report{{Findings: []Finding{finding(High)}}}}
	tester := &fakeTester{results: []testrunner.Result{{Status: testrunner.Fail}}}

	if _, err := NewLoop(p, &fakeFixer{}, tester, High, 3).Run(context.Background(), Request{}); err == nil {
		t.Fatal("expected error when a fix breaks verification")
	}
}

func TestLoopCancellation(t *testing.T) {
	p := &fakeProvider{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewLoop(p, &fakeFixer{}, &fakeTester{}, High, 3).Run(ctx, Request{})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}
