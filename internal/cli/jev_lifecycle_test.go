package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/github"
	"github.com/imhttran/agentic-sop/internal/jev"
	"github.com/imhttran/agentic-sop/internal/quality"
	"github.com/imhttran/agentic-sop/internal/review"
)

// jevEnabledConfig enables JEV in review mode with passing validation, so a run
// reaches the quality seam with no validation or review pressure of its own.
const jevEnabledConfig = "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\nquality:\n  jev:\n    enabled: true\n    mode: review\n"

// jevDiff is a realistic working-tree diff with proper git headers, so the
// changed-file extraction JEV receives has something to find.
const jevDiff = "diff --git a/internal/x.go b/internal/x.go\n--- a/internal/x.go\n+++ b/internal/x.go\n@@ -1 +1 @@\n-old\n+new\n"

// jevTestAgent is a deterministic capability-aware agent that records the input
// each FIX received, so a test can assert what reached FIX.
type jevTestAgent struct {
	plan, impl, fix string
	review          string
	fixInputs       []string
}

func (a *jevTestAgent) Generate(_ context.Context, r agent.Request) (agent.Response, error) {
	switch r.Capability {
	case agent.Plan:
		return agent.Response{Content: a.plan}, nil
	case agent.Implement:
		return agent.Response{Content: a.impl}, nil
	case agent.Fix:
		a.fixInputs = append(a.fixInputs, r.Input)
		return agent.Response{Content: a.fix}, nil
	case agent.Review:
		return agent.Response{Content: a.review}, nil
	default:
		return agent.Response{Content: "{}"}, nil
	}
}

// sequencedJEV is a deterministic Analyzer test double that returns results (and
// errors) in order, repeating the last entry, and records the requests it
// received. It touches no network and no repository.
type sequencedJEV struct {
	results  []jev.Result
	errs     []error
	calls    int
	requests []jev.Request
}

func (a *sequencedJEV) Analyze(_ context.Context, req jev.Request) (jev.Result, error) {
	a.requests = append(a.requests, req)
	i := a.calls
	a.calls++
	if len(a.errs) > 0 {
		j := i
		if j >= len(a.errs) {
			j = len(a.errs) - 1
		}
		if a.errs[j] != nil {
			return jev.Result{}, a.errs[j]
		}
	}
	if len(a.results) == 0 {
		return jev.Result{Status: jev.StatusPass}, nil
	}
	if i >= len(a.results) {
		i = len(a.results) - 1
	}
	return a.results[i], nil
}

// runInjectedCLIWithJEV runs a command with an injected working-tree diff, agent,
// and JEV analyzer factory.
func runInjectedCLIWithJEV(t *testing.T, dir, diff string, a agent.Agent, newJEV func(config.Config) (jev.Analyzer, error), args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	d := deps{
		getwd: func() (string, error) { return dir, nil },
		newAgent: func(string, string, string) (agent.Agent, error) {
			if a == nil {
				return nil, errors.New("no agent configured")
			}
			return a, nil
		},
		readDiff:       func(context.Context, string) (string, error) { return diff, nil },
		commit:         func(context.Context, string, string) error { return nil },
		newGitHub:      func(string) github.Client { return &fakeGitHub{} },
		newJEVAnalyzer: newJEV,
	}
	code = run(args, &out, &errOut, d)
	return code, out.String(), errOut.String()
}

func jevFinding(sev review.Severity, path string, line int, msg, evidence string) jev.Finding {
	return jev.Finding{Severity: sev, Category: "quality", Path: path, Line: line, Message: msg, Evidence: evidence}
}

// --- enabled flow -----------------------------------------------------------

// A blocking JEV finding fails the gate, enters the existing fix loop, and (once
// the analyzer stops reporting it) the run completes. The finding and its
// provenance reach FIX verbatim.
func TestJEVBlockingFindingFeedsFixAndResolves(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, jevEnabledConfig)
	a := &jevTestAgent{plan: validPlanJSON, impl: "impl", fix: "fixed", review: `{"summary":"clean","findings":[]}`}
	analyzer := &sequencedJEV{results: []jev.Result{
		{Status: jev.StatusFindings, Findings: []jev.Finding{
			jevFinding(review.High, "internal/x.go", 12, "file handle not closed", "os.Open without defer Close"),
		}},
		{Status: jev.StatusPass},
	}}

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, jevDiff, a,
		func(config.Config) (jev.Analyzer, error) { return analyzer, nil },
		"run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "fix cycles: 1/3") {
		t.Errorf("stdout = %q", stdout)
	}
	if len(a.fixInputs) != 1 {
		t.Fatalf("fix invocations = %d, want 1", len(a.fixInputs))
	}
	fixInput := a.fixInputs[0]
	for _, want := range []string{
		"# JEV Findings (task T001, run T001)",
		"[HIGH] internal/x.go:12",
		"Finding: file handle not closed",
		"Evidence: os.Open without defer Close",
	} {
		if !strings.Contains(fixInput, want) {
			t.Errorf("fix input missing %q:\n%s", want, fixInput)
		}
	}
	if analyzer.calls != 2 {
		t.Errorf("JEV invocations = %d, want 2 (before and after the fix)", analyzer.calls)
	}
}

// A blocking JEV finding with no other failing evidence still consumes the
// existing fix budget: the loop reruns validation, review, and JEV each cycle and
// escalates to a human when the budget is spent.
func TestJEVBlockingFindingExhaustsFixBudget(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, jevEnabledConfig)
	a := &jevTestAgent{plan: validPlanJSON, impl: "impl", fix: "still broken", review: `{"summary":"clean","findings":[]}`}
	analyzer := &sequencedJEV{results: []jev.Result{
		{Status: jev.StatusFindings, Findings: []jev.Finding{
			jevFinding(review.Critical, "a.go", 1, "always", "proof"),
		}},
	}}

	code, stdout, _ := runInjectedCLIWithJEV(t, dir, jevDiff, a,
		func(config.Config) (jev.Analyzer, error) { return analyzer, nil },
		"run", "--task", "TASK.md")
	if code != exitError {
		t.Errorf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stdout, "NEEDS_HUMAN") {
		t.Errorf("stdout = %q", stdout)
	}
	if !strings.Contains(stdout, "fix cycles: 3/3") {
		t.Errorf("stdout = %q", stdout)
	}
	if analyzer.calls != 4 {
		t.Errorf("JEV invocations = %d, want 4 (initial + one per fix cycle)", analyzer.calls)
	}
}

// The JEV section carries only findings that are blocking under the configured
// policy; a non-blocking finding neither blocks nor reaches FIX.
func TestJEVNonBlockingFindingDoesNotBlockOrReachFix(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, jevEnabledConfig)
	a := &jevTestAgent{plan: validPlanJSON, impl: "impl", review: `{"summary":"clean","findings":[]}`}
	analyzer := &sequencedJEV{results: []jev.Result{
		{Status: jev.StatusFindings, Findings: []jev.Finding{
			jevFinding(review.Medium, "a.go", 3, "style nit", "evidence"),
		}},
	}}

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, jevDiff, a,
		func(config.Config) (jev.Analyzer, error) { return analyzer, nil },
		"run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if len(a.fixInputs) != 0 {
		t.Errorf("a MEDIUM JEV finding must not trigger a fix; got %d", len(a.fixInputs))
	}
	if !strings.Contains(stdout, "PASS") {
		t.Errorf("stdout = %q", stdout)
	}
}

// When JEV reports both a blocking and a non-blocking finding, only the blocking
// one reaches FIX. The blocking finding drives the fix loop (there is no
// validation or review pressure in this fixture).
func TestJEVActionableOnlyFindingsReachFix(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, jevEnabledConfig)
	a := &jevTestAgent{plan: validPlanJSON, impl: "impl", fix: "fixed", review: `{"summary":"clean","findings":[]}`}
	analyzer := &sequencedJEV{results: []jev.Result{
		{Status: jev.StatusFindings, Findings: []jev.Finding{
			jevFinding(review.Low, "low.go", 1, "low thing", "e-low"),
			jevFinding(review.High, "high.go", 2, "high thing", "e-high"),
		}},
		{Status: jev.StatusPass},
	}}

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, jevDiff, a,
		func(config.Config) (jev.Analyzer, error) { return analyzer, nil },
		"run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if len(a.fixInputs) != 1 {
		t.Fatalf("fix invocations = %d, want 1", len(a.fixInputs))
	}
	fixInput := a.fixInputs[0]
	if !strings.Contains(fixInput, "[HIGH] high.go:2") || !strings.Contains(fixInput, "Finding: high thing") {
		t.Errorf("fix input missing the blocking finding:\n%s", fixInput)
	}
	if strings.Contains(fixInput, "low thing") || strings.Contains(fixInput, "[LOW]") {
		t.Errorf("fix input must not carry the non-blocking finding:\n%s", fixInput)
	}
}

// JEV receives the same task-specific context the gate reasons about.
func TestJEVReceivesTaskValidationAndReviewContext(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, jevEnabledConfig)
	a := &jevTestAgent{plan: validPlanJSON, impl: "impl", review: `{"summary":"clean","findings":[]}`}
	analyzer := &sequencedJEV{results: []jev.Result{{Status: jev.StatusPass}}}

	code, _, stderr := runInjectedCLIWithJEV(t, dir, jevDiff, a,
		func(config.Config) (jev.Analyzer, error) { return analyzer, nil },
		"run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if len(analyzer.requests) == 0 {
		t.Fatal("JEV was never invoked")
	}
	req := analyzer.requests[0]
	if !strings.Contains(req.Task, "Add widget") {
		t.Errorf("JEV request missing the task: %q", req.Task)
	}
	if !strings.Contains(req.ValidationResult, "true") {
		t.Errorf("JEV request missing the validation outcome: %q", req.ValidationResult)
	}
	if !strings.Contains(req.ReviewResult, "clean") {
		t.Errorf("JEV request missing the review outcome: %q", req.ReviewResult)
	}
	if len(req.ChangedFiles) != 1 || req.ChangedFiles[0] != "internal/x.go" {
		t.Errorf("JEV request changed files = %v, want [internal/x.go]", req.ChangedFiles)
	}
}

// A JEV analysis that fails to produce a usable result is fail-closed: the run
// must never pass.
func TestJEVAnalyzerErrorFailsClosed(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, jevEnabledConfig)
	a := &jevTestAgent{plan: validPlanJSON, impl: "impl", review: `{"summary":"clean","findings":[]}`}
	analyzer := &sequencedJEV{errs: []error{errors.New("provider down")}}

	code, stdout, _ := runInjectedCLIWithJEV(t, dir, jevDiff, a,
		func(config.Config) (jev.Analyzer, error) { return analyzer, nil },
		"run", "--task", "TASK.md")
	if code == exitOK {
		t.Fatalf("a fail-closed JEV must not pass; stdout=%s", stdout)
	}
	if strings.Contains(stdout, ": PASS") {
		t.Errorf("stdout claimed PASS on a fail-closed JEV: %q", stdout)
	}
}

// A JEV analysis that exhausts its bounded corrective retries (malformed
// structured output) is fail-closed but not a human decision: the gate fails with
// the JEV reason and the run is classified as a bounded JEV analysis failure
// (RETRY), never NEEDS_HUMAN.
func TestJEVMalformedOutputIsBoundedNotHuman(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, jevEnabledConfig)
	a := &jevTestAgent{plan: validPlanJSON, impl: "impl", review: `{"summary":"clean","findings":[]}`}
	analyzer := &sequencedJEV{errs: []error{&jev.ProviderError{
		Kind:     jev.ErrMalformedOutput,
		Err:      errors.New("model output is not valid JSON: invalid character '#' looking for beginning of value"),
		Attempts: 3,
	}}}

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, jevDiff, a,
		func(config.Config) (jev.Analyzer, error) { return analyzer, nil },
		"run", "--task", "TASK.md")
	if code != exitError {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if strings.Contains(stdout, "NEEDS_HUMAN") {
		t.Errorf("a fail-closed JEV must not be reported as NEEDS_HUMAN: %q", stdout)
	}
	if !strings.Contains(stdout, "classification: RETRY (JEV_ANALYSIS_FAILURE") {
		t.Errorf("stdout missing the bounded JEV classification: %q", stdout)
	}
	if !strings.Contains(stdout, "MALFORMED_OUTPUT") {
		t.Errorf("stdout must preserve the JEV failure reason: %q", stdout)
	}
}

// --- disabled / absent flow -------------------------------------------------

// With JEV disabled the analyzer factory is never consulted and the run behaves
// exactly as it did before JEV existed.
func TestJEVDisabledNeverInvokesAnalyzer(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
	a := &jevTestAgent{plan: validPlanJSON, impl: "impl", review: `{"summary":"clean","findings":[]}`}
	called := 0

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, jevDiff, a,
		func(config.Config) (jev.Analyzer, error) {
			called++
			return &sequencedJEV{results: []jev.Result{{Status: jev.StatusFindings, Findings: []jev.Finding{
				jevFinding(review.Critical, "a.go", 1, "boom", "x"),
			}}}}, nil
		},
		"run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if called != 0 {
		t.Errorf("analyzer factory called %d times with JEV disabled; want 0", called)
	}
	if !strings.Contains(stdout, "PASS") {
		t.Errorf("stdout = %q", stdout)
	}
}

// An absent analyzer (nil, no error) is not fatal: JEV stays optional.
func TestJEVAbsentAnalyzerIsNonFatal(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, jevEnabledConfig)
	a := &jevTestAgent{plan: validPlanJSON, impl: "impl", review: `{"summary":"clean","findings":[]}`}

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, jevDiff, a,
		func(config.Config) (jev.Analyzer, error) { return nil, nil },
		"run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "PASS") {
		t.Errorf("stdout = %q", stdout)
	}
}

// A factory error (misconfigured/absent provider) leaves JEV absent rather than
// failing the lifecycle.
func TestJEVAnalyzerFactoryErrorIsNonFatal(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, jevEnabledConfig)
	a := &jevTestAgent{plan: validPlanJSON, impl: "impl", review: `{"summary":"clean","findings":[]}`}

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, jevDiff, a,
		func(config.Config) (jev.Analyzer, error) { return nil, errors.New("no provider") },
		"run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
}

// --- section rendering (unit) -----------------------------------------------

// jevRunEvidenceFromResult builds the lifecycle's evidence the same way
// runOptionalJEV does: the gate-shaped evidence is derived from the raw result,
// so the payload projection reads the same source the lifecycle does.
func jevRunEvidenceFromResult(taskID, runID string, res jev.Result) *jevRunEvidence {
	ev := quality.NewJEVEvidence(res)
	return &jevRunEvidence{TaskID: taskID, RunID: runID, Evidence: &ev, Result: res}
}

// The FIX-context section is deterministic: blocking findings only, in the
// payload contract's stable order, with provenance in the heading.
func TestJEVFindingsSectionOrderFilteringAndProvenance(t *testing.T) {
	ev := jevRunEvidenceFromResult("T9", "R9", jev.Result{
		Status: jev.StatusFindings,
		Findings: []jev.Finding{
			jevFinding(review.Medium, "m.go", 1, "medium thing", ""),
			jevFinding(review.Critical, "c.go", 2, "crit thing", "proof-c"),
			jevFinding(review.High, "h.go", 3, "high thing", "proof-h"),
		},
	})

	got := ev.findingsSection([]string{"high", "critical"})
	if !strings.HasPrefix(got, "# JEV Findings (task T9, run R9)") {
		t.Errorf("section heading = %q", got)
	}
	ci, hi := strings.Index(got, "[CRITICAL] c.go:2"), strings.Index(got, "[HIGH] h.go:3")
	if ci < 0 || hi < 0 {
		t.Fatalf("missing blocking findings:\n%s", got)
	}
	if ci > hi {
		t.Errorf("findings must keep analyzer order (CRITICAL before HIGH):\n%s", got)
	}
	if strings.Contains(got, "medium thing") || strings.Contains(got, "[MEDIUM]") {
		t.Errorf("non-blocking finding leaked into the section:\n%s", got)
	}
	if !strings.Contains(got, "Evidence: proof-c") {
		t.Errorf("evidence missing:\n%s", got)
	}
	if !strings.Contains(got, "Category: quality") {
		t.Errorf("category metadata missing:\n%s", got)
	}
	if !strings.Contains(got, "Finding: crit thing") {
		t.Errorf("actionable message missing:\n%s", got)
	}
}

func TestJEVFindingsSectionEmptyCases(t *testing.T) {
	if got := (*jevRunEvidence)(nil).findingsSection(nil); got != "" {
		t.Errorf("nil evidence section = %q, want empty", got)
	}
	if got := (&jevRunEvidence{}).findingsSection(nil); got != "" {
		t.Errorf("nil inner evidence section = %q, want empty", got)
	}
	failClosed := &jevRunEvidence{Evidence: &quality.JEVEvidence{FailClosed: true, Reason: "x"}}
	if got := failClosed.findingsSection(nil); got != "" {
		t.Errorf("fail-closed section = %q, want empty", got)
	}
	pass := &jevRunEvidence{Evidence: &quality.JEVEvidence{Status: jev.StatusPass}}
	if got := pass.findingsSection(nil); got != "" {
		t.Errorf("pass section = %q, want empty", got)
	}
	// Unknown provenance degrades to a bare heading rather than an empty pair.
	only := jevRunEvidenceFromResult("", "", jev.Result{
		Status:   jev.StatusFindings,
		Findings: []jev.Finding{jevFinding(review.High, "a.go", 1, "m", "")},
	})
	if got := only.findingsSection([]string{"high"}); !strings.HasPrefix(got, "# JEV Findings\n") {
		t.Errorf("bare heading = %q", got)
	}
}

func TestJEVRunEvidenceGateEvidenceIsNilSafe(t *testing.T) {
	if got := (*jevRunEvidence)(nil).gateEvidence(); got != nil {
		t.Errorf("nil receiver gate evidence = %v, want nil", got)
	}
	ev := &quality.JEVEvidence{Status: jev.StatusPass}
	if got := (&jevRunEvidence{Evidence: ev}).gateEvidence(); got != ev {
		t.Errorf("gate evidence = %v, want the inner evidence", got)
	}
}
