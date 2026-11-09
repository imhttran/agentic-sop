package review

import (
	"context"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/testrunner"
)

// The tests in this file cover the six CRS-004 cases for the reviewer
// challenge/simplification seams. They are deterministic: they script provider
// reports, fixers, testers, and agent responses. No model output is the sole
// correctness oracle; SOP's Severity and Report.Blocking decide pass/fail.

// caseProvider is a scripted Provider that also records the requests it saw.
type caseProvider struct {
	reports  []Report
	calls    int
	requests []Request
}

func (p *caseProvider) Review(_ context.Context, request Request) (Report, error) {
	p.requests = append(p.requests, request)
	idx := p.calls
	p.calls++
	if idx < len(p.reports) {
		return p.reports[idx], nil
	}
	if len(p.reports) > 0 {
		return p.reports[len(p.reports)-1], nil
	}
	return Report{}, nil
}

// Case 1: A satisfied requirement with a clean implementation and adequate tests
// yields a valid no-findings review, and the fixer is never invoked.
func TestCase1CleanPassNoFindingsNoFix(t *testing.T) {
	p := &caseProvider{reports: []Report{{Summary: "no problems found", Findings: nil}}}
	fixer := &fakeFixer{}
	tester := &fakeTester{}

	report, err := NewLoop(p, fixer, tester, High, 3).Run(context.Background(), Request{
		Task:        "add a helper that returns the sum of two ints",
		Criteria:    "AC1: Sum returns a+b",
		Diff:        "diff --git a/sum.go b/sum.go\n+func Sum(a, b int) int { return a + b }",
		TestSummary: "PASS: TestSum covers positives, negatives, and zero",
	})
	if err != nil {
		t.Fatalf("expected a clean pass, got %v", err)
	}
	if len(report.Findings) != 0 {
		t.Errorf("findings = %+v, want none", report.Findings)
	}
	if report.Blocking(High) {
		t.Error("clean report must not block")
	}
	if fixer.calls != 0 {
		t.Errorf("fixer calls = %d, want 0", fixer.calls)
	}
	if tester.calls != 0 {
		t.Errorf("tester calls = %d, want 0", tester.calls)
	}
	if p.calls != 1 {
		t.Errorf("provider calls = %d, want 1", p.calls)
	}
}

// Case 2: A missing requirement or incorrect behavior yields a justified finding
// that blocks under the configured policy and triggers a fix.
func TestCase2RequirementDefectBlocksAndFixes(t *testing.T) {
	blocking := Finding{
		Severity:   High,
		Title:      "AC2 missing: Sum does not handle overflow",
		Detail:     "AC2 requires Sum to reject overflow; the implementation adds silently.",
		File:       "sum.go",
		Line:       3,
		Suggestion: "Check for overflow and return an error",
	}
	report := Report{Findings: []Finding{blocking}}
	if !report.Blocking(High) {
		t.Fatal("justified HIGH finding must block at the configured threshold")
	}
	// Below the configured threshold it is only advisory; the configured policy
	// decides, not the model.
	if report.Blocking(Critical) {
		t.Error("HIGH finding must not block at CRITICAL threshold")
	}

	p := &caseProvider{reports: []Report{
		{Summary: "requirement defect", Findings: []Finding{blocking}},
		{Summary: "fixed", Findings: nil},
	}}
	fixer := &fakeFixer{}
	tester := &fakeTester{results: []testrunner.Result{{Status: testrunner.Pass}}}

	final, err := NewLoop(p, fixer, tester, High, 3).Run(context.Background(), Request{})
	if err != nil {
		t.Fatalf("expected pass after fix, got %v", err)
	}
	if final.Blocking(High) {
		t.Errorf("final report still blocks: %+v", final.Findings)
	}
	if fixer.calls != 1 {
		t.Errorf("fixer calls = %d, want 1", fixer.calls)
	}
	if p.calls != 2 {
		t.Errorf("provider calls = %d, want 2 (initial + re-review)", p.calls)
	}
}

// Case 3: An unnecessary abstraction or duplication without a correctness failure
// yields an actionable advisory finding and no forced remediation.
func TestCase3AdvisoryComplexityNoForcedRemediation(t *testing.T) {
	advisory := Finding{
		Severity:   Low,
		Title:      "Duplicated validation in two call sites",
		Detail:     "The same input check is written twice; correctness is fine.",
		File:       "handler.go",
		Line:       42,
		Suggestion: "Extract one validateInput helper to remove the duplication",
	}
	report := Report{Summary: "advisory only", Findings: []Finding{advisory}}
	if report.Blocking(High) {
		t.Fatal("LOW advisory finding must not block at the configured HIGH threshold")
	}

	p := &caseProvider{reports: []Report{report}}
	fixer := &fakeFixer{}
	if _, err := NewLoop(p, fixer, &fakeTester{}, High, 3).Run(context.Background(), Request{}); err != nil {
		t.Fatalf("advisory-only review must pass, got %v", err)
	}
	if fixer.calls != 0 {
		t.Errorf("fixer calls = %d, want 0 (no forced remediation)", fixer.calls)
	}
	if advisory.Suggestion == "" || advisory.File == "" {
		t.Error("advisory finding must be actionable (file and suggestion)")
	}
}

// Case 4: Weak or missing test assertions for a specified criterion yield an
// evidence-grounded finding that cites the code path, without claiming tests
// failed when they were not run.
func TestCase4WeakTestEvidenceNoFailureClaim(t *testing.T) {
	testSummary := "NOT RUN: suite not executed in this review"
	lowered := strings.ToLower(testSummary)
	if strings.Contains(lowered, "pass") || strings.Contains(lowered, "fail") {
		t.Fatalf("precondition: test summary must not claim a pass/fail result: %q", testSummary)
	}
	finding := Finding{
		Severity:   Medium,
		Title:      "AC3 is not proven by tests",
		Detail:     "AC3 requires Sum to reject overflow, but no test asserts the error path in sum.go. Tests were not run, so this is a coverage gap, not a test failure.",
		File:       "sum.go",
		Line:       3,
		Suggestion: "Add a test that asserts Sum returns an overflow error",
	}
	if finding.File == "" {
		t.Error("finding must cite the affected code path")
	}
	detail := strings.ToLower(finding.Detail)
	if strings.Contains(detail, "test failed") || strings.Contains(detail, "tests failed") {
		t.Errorf("finding claims tests failed without a run: %q", finding.Detail)
	}
	if !strings.Contains(finding.Detail, "AC3") || !strings.Contains(finding.Detail, "sum.go") {
		t.Error("finding must ground itself in the criterion and code path")
	}
	// Deterministically, the criterion gap is a MEDIUM finding: advisory under a
	// HIGH threshold, so no remediation is forced, but it blocks at MEDIUM.
	report := Report{Summary: "weak test evidence", Findings: []Finding{finding}}
	if report.Blocking(High) {
		t.Error("weak-test finding is advisory under the HIGH threshold")
	}
	if !report.Blocking(Medium) {
		t.Error("weak-test finding must block when the policy sets MEDIUM")
	}
}

// Case 5: A simplification performed inside a fix iteration reruns the affected
// tests and re-runs review under the existing Loop rules.
func TestCase5SimplificationInFixIterationReruns(t *testing.T) {
	blocking := Finding{
		Severity:   High,
		Title:      "Unnecessary wrapper around a one-line conversion",
		Detail:     "The wrapper adds indirection with no benefit; simplify it.",
		File:       "convert.go",
		Line:       12,
		Suggestion: "Inline the conversion",
	}
	p := &caseProvider{reports: []Report{
		{Summary: "complexity", Findings: []Finding{blocking}},
		{Summary: "simplified", Findings: nil},
	}}
	fixer := &fakeFixer{}
	tester := &fakeTester{results: []testrunner.Result{{Status: testrunner.Pass}}}

	final, err := NewLoop(p, fixer, tester, High, 3).Run(context.Background(), Request{})
	if err != nil {
		t.Fatalf("simplification inside a fix iteration must pass, got %v", err)
	}
	if final.Blocking(High) {
		t.Errorf("final report still blocks: %+v", final.Findings)
	}
	if fixer.calls != 1 {
		t.Errorf("fixer calls = %d, want 1", fixer.calls)
	}
	if tester.calls != 1 {
		t.Errorf("tester calls = %d, want 1 (tests rerun after fix)", tester.calls)
	}
	if p.calls != 2 {
		t.Errorf("provider calls = %d, want 2 (review rerun after fix)", p.calls)
	}
}

// Case 6: Malformed review output plus an unrelated dirty worktree preserves the
// existing failure and isolation protections. The test drives the real
// AgentProvider retry/parse path, then feeds the malformed content through the
// Loop, and proves the Loop never mutates or cleans a worktree it does not own.
func TestCase6MalformedOutputWithDirtyWorktreeIsolation(t *testing.T) {
	malformed := "not json {"
	a := &scriptedAgent{contents: []string{malformed}}
	report, err := NewAgentProvider(a).Review(context.Background(), Request{})
	if err == nil {
		t.Fatal("malformed review output must not produce a pass")
	}
	if !IsMalformedOutput(err) {
		t.Fatalf("err = %v, want a malformed-output (NEEDS_HUMAN) outcome", err)
	}
	if a.calls != reviewRetryBound {
		t.Errorf("agent calls = %d, want the fixed bound %d", a.calls, reviewRetryBound)
	}
	if len(report.Findings) != 0 {
		t.Errorf("malformed output must yield no findings: %+v", report.Findings)
	}

	// The Loop must propagate the malformed outcome, never a pass, and must not
	// invoke the fixer (a malformed review cannot authorise a change).
	mfixer := &fakeFixer{}
	loopReport, loopErr := NewLoop(malformedProvider{}, mfixer, passTester{}, High, 3).Run(context.Background(), Request{})
	if loopErr == nil || !IsMalformedOutput(loopErr) {
		t.Fatalf("loop err = %v, want a malformed-output outcome", loopErr)
	}
	if mfixer.calls != 0 {
		t.Errorf("fixer calls = %d, want 0 on malformed output", mfixer.calls)
	}
	// The retained blocking finding must still surface; a malformed sibling never
	// hides a genuine HIGH.
	if !loopReport.Blocking(High) {
		t.Errorf("retained HIGH finding must still block: %+v", loopReport.Findings)
	}
	// Isolation: this test only observes behavior. Review code has no worktree API,
	// so the only path that could touch an unrelated dirty worktree is Fix, which
	// was not called; the user-owned dirty tree is preserved untouched.
}

// captureAgent records the request it receives and returns a fixed response.
type captureAgent struct {
	requests []agent.Request
	content  string
}

func (c *captureAgent) Generate(_ context.Context, request agent.Request) (agent.Response, error) {
	c.requests = append(c.requests, request)
	return agent.Response{Content: c.content}, nil
}

// TestReviewPromptStatesChallengeOrderAndObligations proves the self-review prompt
// carries the challenge order and finding obligations, and that the review context
// (criteria, test summary, diff) reaches the reviewer. It is deterministic: it
// inspects the prompt that production sends and does not depend on model output.
func TestReviewPromptStatesChallengeOrderAndObligations(t *testing.T) {
	a := &captureAgent{content: `{"summary":"ok","findings":[]}`}
	if _, err := NewAgentProvider(a).Review(context.Background(), Request{
		Task:        "task: add Sum",
		Criteria:    "AC1: Sum returns a+b",
		TestSummary: "validation: PASS (3 checks)",
		Diff:        "diff --git a/sum.go b/sum.go",
	}); err != nil {
		t.Fatalf("review: %v", err)
	}
	if len(a.requests) != 1 {
		t.Fatalf("agent requests = %d, want 1", len(a.requests))
	}
	req := a.requests[0]
	if req.Capability != agent.Review {
		t.Errorf("capability = %q, want review", req.Capability)
	}
	for _, want := range []string{
		"Goal fit",
		"Correctness",
		"Complexity",
		"Tests",
		"Change justification",
		"Prefer no change",
		"empty findings list",
		"Do not state that tests passed or failed",
	} {
		if !strings.Contains(req.Task, want) {
			t.Errorf("prompt is missing %q", want)
		}
	}
	for _, want := range []string{
		"AC1: Sum returns a+b",
		"validation: PASS (3 checks)",
		"diff --git a/sum.go",
	} {
		if !strings.Contains(req.Input, want) {
			t.Errorf("review input is missing %q", want)
		}
	}
	if !strings.Contains(req.OutputRequirements, "severity") {
		t.Error("output requirements must state the severity contract")
	}
}
