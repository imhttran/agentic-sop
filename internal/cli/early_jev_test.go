package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/jev"
)

// earlyJEVTriageConfig enables the early JEV decision layer with ONLY the task
// triage gate on, so the gate runs before implementation.
const earlyJEVTriageConfig = "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\nearly_jev:\n  enabled: true\n  mode: review\n  gates:\n    task_triage: true\n"

// earlyJEVPreExecConfig enables only the pre-execution gate.
const earlyJEVPreExecConfig = "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\nearly_jev:\n  enabled: true\n  mode: review\n  gates:\n    pre_execution: true\n"

// earlyNoGateConfig enables the early layer but no gate, so nothing runs.
const earlyNoGateConfig = "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\nearly_jev:\n  enabled: true\n  mode: review\n"

// earlyFakeAnalyzer is a deterministic Analyzer test double for the early
// checkpoints. It performs no network or repository access.
type earlyFakeAnalyzer struct {
	result jev.Result
	err    error
	calls  int
	last   jev.Request
}

func (a *earlyFakeAnalyzer) Analyze(_ context.Context, req jev.Request) (jev.Result, error) {
	a.calls++
	a.last = req
	return a.result, a.err
}

// earlyEvidenceResult builds a consistent early analysis result carrying one
// typed structured-evidence item.
func earlyEvidenceResult(purpose jev.Purpose, category jev.Category, sev jev.Severity) jev.Result {
	ev := &jev.Evidence{
		Version:    jev.EvidenceVersion,
		Purpose:    purpose,
		Severity:   sev,
		Status:     jev.EvidenceFindings,
		Confidence: 0.8,
		Items: []jev.EvidenceItem{{
			Purpose:  purpose,
			Severity: sev,
			Category: category,
			Detail:   "typed early finding",
		}},
	}
	return jev.Result{Status: jev.StatusFindings, StructuredEvidence: ev}
}

// earlyClearResult is a clean early analysis: PASS with no items.
func earlyClearResult(purpose jev.Purpose) jev.Result {
	return jev.Result{
		Status: jev.StatusPass,
		StructuredEvidence: &jev.Evidence{
			Version:  jev.EvidenceVersion,
			Purpose:  purpose,
			Severity: jev.SeverityInfo,
			Status:   jev.EvidencePass,
		},
	}
}

// earlyCountingAgent records whether implementation ran.
type earlyCountingAgent struct {
	impl int
}

func (a *earlyCountingAgent) Generate(_ context.Context, r agent.Request) (agent.Response, error) {
	switch r.Capability {
	case agent.Plan:
		return agent.Response{Content: validPlanJSON}, nil
	case agent.Implement:
		a.impl++
		return agent.Response{Content: "changed files", Outcome: &agent.Outcome{Status: agent.OutcomeCompleted, ChangesExpected: true}}, nil
	case agent.Review:
		return agent.Response{Content: `{"summary":"clean","findings":[]}`}, nil
	default:
		return agent.Response{Content: "{}"}, nil
	}
}

// With the triage gate disabled (the default), the analyzer is never consulted
// and the task runs exactly as before.
func TestEarlyTriageDisabledIsNoOp(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
	a := &earlyCountingAgent{}
	analyzer := &earlyFakeAnalyzer{result: earlyEvidenceResult(jev.PurposeTaskTriage, jev.CategoryAmbiguity, jev.SeverityHigh)}

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, "diff --git a/x b/x\n", a,
		func(config.Config) (jev.Analyzer, error) { return analyzer, nil },
		"run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if analyzer.calls != 0 {
		t.Errorf("disabled triage consulted the analyzer %d times, want 0", analyzer.calls)
	}
	if a.impl != 1 {
		t.Errorf("implementation ran %d times, want 1", a.impl)
	}
	if strings.Contains(stdout, "early JEV triage") {
		t.Errorf("disabled triage emitted a boundary: %q", stdout)
	}
}

// A blocking triage finding escalates: the task stops at a human boundary and is
// never implemented.
func TestEarlyTriageBlockingFindingEscalates(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, earlyJEVTriageConfig)
	a := &earlyCountingAgent{}
	analyzer := &earlyFakeAnalyzer{result: earlyEvidenceResult(jev.PurposeTaskTriage, jev.CategoryAmbiguity, jev.SeverityHigh)}

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, "diff --git a/x b/x\n", a,
		func(config.Config) (jev.Analyzer, error) { return analyzer, nil },
		"run", "--task", "TASK.md")
	if code == exitOK {
		t.Fatalf("escalated triage must not report success: stdout=%s stderr=%s", stdout, stderr)
	}
	if analyzer.calls != 1 {
		t.Errorf("triage analyzer calls = %d, want 1", analyzer.calls)
	}
	if a.impl != 0 {
		t.Errorf("implementation ran %d times, want 0 (the task must not start)", a.impl)
	}
	if !strings.Contains(stdout, "NEEDS_HUMAN (early JEV triage)") {
		t.Errorf("stdout missing the human boundary: %q", stdout)
	}
}

// A clear triage result continues: the task is implemented normally.
func TestEarlyTriageClearContinues(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, earlyJEVTriageConfig)
	a := &earlyCountingAgent{}
	analyzer := &earlyFakeAnalyzer{result: earlyClearResult(jev.PurposeTaskTriage)}

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, "diff --git a/x b/x\n", a,
		func(config.Config) (jev.Analyzer, error) { return analyzer, nil },
		"run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if analyzer.calls != 1 {
		t.Errorf("triage analyzer calls = %d, want 1", analyzer.calls)
	}
	if a.impl != 1 {
		t.Errorf("implementation ran %d times, want 1", a.impl)
	}
}

// A provider failure is recorded, never treated as a finding, and does not block
// normal work (the advisory fallback).
func TestEarlyTriageProviderFailureContinues(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, earlyJEVTriageConfig)
	a := &earlyCountingAgent{}
	analyzer := &earlyFakeAnalyzer{err: jev.ErrProviderFailure}

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, "diff --git a/x b/x\n", a,
		func(config.Config) (jev.Analyzer, error) { return analyzer, nil },
		"run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("an advisory provider failure must not block: code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if strings.Contains(stdout, "NEEDS_HUMAN (early JEV triage)") {
		t.Errorf("a provider failure must not escalate: %q", stdout)
	}
	if a.impl != 1 {
		t.Errorf("implementation ran %d times, want 1", a.impl)
	}
}

// A pre-execution finding only escalates when its gate is enabled; enabling only
// pre_execution leaves triage off.
func TestEarlyPreExecutionGateEscalates(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, earlyJEVPreExecConfig)
	a := &earlyCountingAgent{}
	analyzer := &earlyFakeAnalyzer{result: earlyEvidenceResult(jev.PurposePreExecution, jev.CategorySecurity, jev.SeverityHigh)}

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, "diff --git a/x b/x\n", a,
		func(config.Config) (jev.Analyzer, error) { return analyzer, nil },
		"run", "--task", "TASK.md")
	if code == exitOK {
		t.Fatalf("pre-execution escalation must not report success: stdout=%s stderr=%s", stdout, stderr)
	}
	if analyzer.calls != 1 {
		t.Errorf("pre-execution analyzer calls = %d, want 1", analyzer.calls)
	}
}

// Enabling the early layer without any gate runs neither checkpoint.
func TestEarlyLayerWithoutGatesIsNoOp(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, earlyNoGateConfig)
	a := &earlyCountingAgent{}
	analyzer := &earlyFakeAnalyzer{result: earlyEvidenceResult(jev.PurposeTaskTriage, jev.CategoryAmbiguity, jev.SeverityHigh)}

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, "diff --git a/x b/x\n", a,
		func(config.Config) (jev.Analyzer, error) { return analyzer, nil },
		"run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if analyzer.calls != 0 {
		t.Errorf("no gate is enabled, but the analyzer was consulted %d times", analyzer.calls)
	}
	if a.impl != 1 {
		t.Errorf("implementation ran %d times, want 1", a.impl)
	}
}

// A malformed early result fails closed: it is never treated as a pass or a
// finding, so the advisory path continues without a fabricated escalation.
func TestEarlyTriageMalformedResultContinuesFailClosed(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, earlyJEVTriageConfig)
	a := &earlyCountingAgent{}
	// An unknown purpose makes the structured evidence invalid.
	bad := earlyEvidenceResult(jev.Purpose("MYSTERY"), jev.CategoryAmbiguity, jev.SeverityHigh)
	analyzer := &earlyFakeAnalyzer{result: bad}

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, "diff --git a/x b/x\n", a,
		func(config.Config) (jev.Analyzer, error) { return analyzer, nil },
		"run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("a malformed advisory result must not block: code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if strings.Contains(stdout, "NEEDS_HUMAN (early JEV triage)") {
		t.Errorf("malformed evidence must not fabricate an escalation: %q", stdout)
	}
	if a.impl != 1 {
		t.Errorf("implementation ran %d times, want 1", a.impl)
	}
}
