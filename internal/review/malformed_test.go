package review

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/testrunner"
)

// scriptedAgent returns a different content per Generate call, so retry behavior
// can be exercised deterministically.
type scriptedAgent struct {
	contents []string
	calls    int
}

func (s *scriptedAgent) Generate(_ context.Context, _ agent.Request) (agent.Response, error) {
	content := ""
	if s.calls < len(s.contents) {
		content = s.contents[s.calls]
	} else if len(s.contents) > 0 {
		content = s.contents[len(s.contents)-1]
	}
	s.calls++
	return agent.Response{Content: content}, nil
}

// TestParseReportRetainsValidFindingAlongsideMalformedFinding proves that a
// genuine HIGH finding is retained and still blocks even when another finding in
// the same response is malformed. The malformed finding is rejected explicitly,
// never silently dropped.
func TestParseReportRetainsValidFindingAlongsideMalformedFinding(t *testing.T) {
	content := `{"summary":"s","findings":[
		{"severity":"HIGH","title":"real bug"},
		{"severity":"NOPE","title":"bad"}
	]}`
	report, err := parseReport(content)
	var malformed *ErrMalformedSeverity
	if !errors.As(err, &malformed) {
		t.Fatalf("err = %v, want *ErrMalformedSeverity", err)
	}
	if malformed.Index != 1 || malformed.Value != "NOPE" {
		t.Errorf("malformed = %+v, want index 1 value NOPE", malformed)
	}
	// The valid HIGH finding must be RETAINED and must still block.
	if len(report.Findings) != 1 || report.Findings[0].Severity != High {
		t.Fatalf("valid HIGH finding discarded: findings = %+v", report.Findings)
	}
	if !report.Blocking(High) {
		t.Errorf("retained HIGH finding does not block: %+v", report.Findings)
	}
}

// TestParseReportRetainsCriticalBeforeMalformedFinding proves a CRITICAL finding
// preceding a malformed sibling is retained and blocks at CRITICAL even when the
// malformed finding comes second.
func TestParseReportRetainsCriticalBeforeMalformedFinding(t *testing.T) {
	content := `{"summary":"s","findings":[
		{"severity":"CRITICAL","title":"sec"},
		{"severity":"NOPE","title":"bad"}
	]}`
	report, err := parseReport(content)
	var malformed *ErrMalformedSeverity
	if !errors.As(err, &malformed) {
		t.Fatalf("err = %v, want *ErrMalformedSeverity", err)
	}
	if len(report.Findings) != 1 || report.Findings[0].Severity != Critical {
		t.Fatalf("valid CRITICAL finding discarded: findings = %+v", report.Findings)
	}
	if !report.Blocking(Critical) {
		t.Errorf("retained CRITICAL finding does not block: %+v", report.Findings)
	}
}

// TestParseReportRejectsEmptySeverity proves an empty severity is rejected
// explicitly and never silently dropped.
func TestParseReportRejectsEmptySeverity(t *testing.T) {
	content := `{"summary":"s","findings":[{"severity":"","title":"x"}]}`
	_, err := parseReport(content)
	var malformed *ErrMalformedSeverity
	if !errors.As(err, &malformed) {
		t.Fatalf("err = %v, want *ErrMalformedSeverity", err)
	}
	if malformed.Index != 0 || malformed.Value != "" {
		t.Errorf("malformed = %+v, want index 0 empty value", malformed)
	}
}

// TestParseReportRejectsInvalidSeverity proves an unknown severity is rejected
// deterministically.
func TestParseReportRejectsInvalidSeverity(t *testing.T) {
	content := `{"summary":"s","findings":[{"severity":"SEVERE","title":"x"}]}`
	_, err := parseReport(content)
	var malformed *ErrMalformedSeverity
	if !errors.As(err, &malformed) {
		t.Fatalf("err = %v, want *ErrMalformedSeverity", err)
	}
	if malformed.Value != "SEVERE" {
		t.Errorf("value = %q, want SEVERE", malformed.Value)
	}
}

// TestParseReportRejectsNearMissSeverity proves near-miss severities an LLM
// frequently emits (lowercase and leading/trailing whitespace) are rejected
// deterministically and never silently accepted.
func TestParseReportRejectsNearMissSeverity(t *testing.T) {
	for _, sev := range []string{"high", " high", "high ", "High", "critical", " "} {
		content := fmt.Sprintf(`{"summary":"s","findings":[{"severity":%q,"title":"x"}]}`, sev)
		report, err := parseReport(content)
		var malformed *ErrMalformedSeverity
		if !errors.As(err, &malformed) {
			t.Fatalf("severity %q accepted, want rejection: err = %v", sev, err)
		}
		if malformed.Value != sev {
			t.Errorf("severity %q: recorded value %q, want %q", sev, malformed.Value, sev)
		}
		if len(report.Findings) != 0 {
			t.Errorf("severity %q: findings = %+v, want none retained", sev, report.Findings)
		}
	}
}

// TestParseReportAllowsEverySeverity proves the allowed severity set is unchanged.
func TestParseReportAllowsEverySeverity(t *testing.T) {
	for _, sev := range []string{"INFO", "LOW", "MEDIUM", "HIGH", "CRITICAL"} {
		content := fmt.Sprintf(`{"summary":"s","findings":[{"severity":%q,"title":"x"}]}`, sev)
		report, err := parseReport(content)
		if err != nil {
			t.Fatalf("severity %s rejected: %v", sev, err)
		}
		if len(report.Findings) != 1 || string(report.Findings[0].Severity) != sev {
			t.Errorf("severity %s not parsed: %+v", sev, report.Findings)
		}
	}
}

// TestAgentProviderRetriesThenSucceeds proves a malformed response is retried
// within the bound and a later valid response is accepted.
func TestAgentProviderRetriesThenSucceeds(t *testing.T) {
	a := &scriptedAgent{contents: []string{
		"not json",
		`{"summary":"ok","findings":[{"severity":"HIGH","title":"bug"}]}`,
	}}
	report, err := NewAgentProvider(a).Review(context.Background(), Request{})
	if err != nil {
		t.Fatalf("Review failed: %v", err)
	}
	if a.calls != 2 {
		t.Errorf("calls = %d, want 2", a.calls)
	}
	if len(report.Findings) != 1 || report.Findings[0].Severity != High {
		t.Errorf("findings = %+v", report.Findings)
	}
}

// TestAgentProviderMalformedOutputBounded proves repeated malformed responses
// terminate within the bound and preserve the raw response, never yielding a pass.
func TestAgentProviderMalformedOutputBounded(t *testing.T) {
	a := &scriptedAgent{contents: []string{"nope one", "nope two"}}
	_, err := NewAgentProvider(a).Review(context.Background(), Request{})
	if err == nil {
		t.Fatal("expected a bounded malformed-output error")
	}
	if !IsMalformedOutput(err) {
		t.Fatalf("err = %v, want a malformed-output outcome", err)
	}
	if a.calls != reviewRetryBound {
		t.Errorf("calls = %d, want the fixed bound %d", a.calls, reviewRetryBound)
	}
	var malformed *ErrMalformedOutput
	if !errors.As(err, &malformed) {
		t.Fatalf("err = %v, want *ErrMalformedOutput", err)
	}
	if !strings.Contains(malformed.Raw, "nope two") {
		t.Errorf("raw response not preserved: %q", malformed.Raw)
	}
	if malformed.Attempts != reviewRetryBound {
		t.Errorf("attempts = %d, want %d", malformed.Attempts, reviewRetryBound)
	}
	// The raw response must be surfaced in the error text so an operator sees it.
	if !strings.Contains(err.Error(), "nope two") {
		t.Errorf("raw response not surfaced in error text: %q", err.Error())
	}
}

// TestAgentProviderMalformedOutputRetainsBlockingFinding proves that when the
// only responses carry both a genuine HIGH finding and a malformed severity, the
// provider surfaces a bounded malformed-output outcome AND still returns the
// retained blocking finding, so a genuine HIGH is never lost across retries.
func TestAgentProviderMalformedOutputRetainsBlockingFinding(t *testing.T) {
	malformed := `{"summary":"s","findings":[
		{"severity":"HIGH","title":"real bug"},
		{"severity":"","title":"bad"}
	]}`
	a := &scriptedAgent{contents: []string{malformed}}
	report, err := NewAgentProvider(a).Review(context.Background(), Request{})
	if err == nil {
		t.Fatal("expected a bounded malformed-output error")
	}
	if !IsMalformedOutput(err) {
		t.Fatalf("err = %v, want a malformed-output outcome", err)
	}
	if len(report.Findings) != 1 || report.Findings[0].Severity != High {
		t.Fatalf("retained HIGH finding lost: findings = %+v", report.Findings)
	}
	if !report.Blocking(High) {
		t.Errorf("retained HIGH finding does not block: %+v", report.Findings)
	}
	var malformedErr *ErrMalformedOutput
	if !errors.As(err, &malformedErr) {
		t.Fatalf("err = %v, want *ErrMalformedOutput", err)
	}
	if len(malformedErr.Report.Findings) != 1 || malformedErr.Report.Findings[0].Severity != High {
		t.Errorf("malformed output did not preserve the retained finding: %+v", malformedErr.Report.Findings)
	}
}

// TestMalformedOutputNeverPasses proves a malformed review response cannot be
// interpreted as a passing report: it is an error, and the zero Report carries no
// findings a gate could treat as a clean pass.
func TestMalformedOutputNeverPasses(t *testing.T) {
	a := &scriptedAgent{contents: []string{"garbage"}}
	report, err := NewAgentProvider(a).Review(context.Background(), Request{})
	if err == nil {
		t.Fatal("malformed output must not yield a nil error (a pass)")
	}
	if len(report.Findings) != 0 || report.Summary != "" {
		t.Errorf("malformed output must not yield findings or summary: %+v", report)
	}
}

// noopFixer is a Fixer that never changes anything.
type noopFixer struct{}

func (noopFixer) Fix(context.Context, Report) error { return nil }

// passTester always reports a passing verification.
type passTester struct{}

func (passTester) Run(context.Context) testrunner.Result {
	return testrunner.Result{Status: testrunner.Pass}
}

// malformedProvider always fails Review with a bounded malformed-output error
// carrying a retained blocking finding and the raw response.
type malformedProvider struct{}

func (malformedProvider) Review(context.Context, Request) (Report, error) {
	retained := Report{Summary: "s", Findings: []Finding{{Severity: High, Title: "real bug"}}}
	return retained, &ErrMalformedOutput{
		Raw:      `{"findings":[{"severity":"HIGH"},{"severity":""}]}`,
		Attempts: reviewRetryBound,
		Last:     &ErrMalformedSeverity{Index: 1, Value: ""},
		Report:   retained,
	}
}

// TestLoopMalformedOutputIsNeedsHuman proves the fix loop never treats malformed
// review output as a pass or a silent exit: it terminates with a classified
// malformed-output error, preserves the raw response, and still surfaces the
// retained blocking finding.
func TestLoopMalformedOutputIsNeedsHuman(t *testing.T) {
	loop := NewLoop(malformedProvider{}, noopFixer{}, passTester{}, High, 3)
	report, err := loop.Run(context.Background(), Request{})
	if err == nil {
		t.Fatal("malformed review output must not produce a pass")
	}
	if !IsMalformedOutput(err) {
		t.Fatalf("err = %v, want a malformed-output (NEEDS_HUMAN) outcome", err)
	}
	if len(report.Findings) != 1 || report.Findings[0].Severity != High {
		t.Fatalf("loop discarded the retained HIGH finding: findings = %+v", report.Findings)
	}
	if !report.Blocking(High) {
		t.Errorf("retained HIGH finding does not block: %+v", report.Findings)
	}
	var malformed *ErrMalformedOutput
	if !errors.As(err, &malformed) {
		t.Fatalf("err = %v, want *ErrMalformedOutput", err)
	}
	if !strings.Contains(malformed.Raw, "severity") {
		t.Errorf("raw response not preserved: %q", malformed.Raw)
	}
}
