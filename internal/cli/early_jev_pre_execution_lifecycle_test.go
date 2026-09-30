package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/jev"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/store"
)

// These tests pin the P3-011 integration of the pre-execution gate through the
// GRAPH RUN entry point (driveGraph -> runScheduledTask -> runStages), not only
// single-task mode. They assert that the gate runs after precheck/planning and
// before implementation, applies deterministic dispositions through the EXISTING
// human boundary without introducing new task states, and is a strict no-op when
// disabled.

// earlyPreExecGraphConfig enables ONLY the pre-execution gate, so the triage
// checkpoint stays off and the pre-execution checkpoint is exercised in isolation.
const earlyPreExecGraphConfig = "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\nearly_jev:\n  enabled: true\n  mode: review\n  gates:\n    pre_execution: true\n"

// earlyBothGatesGraphConfig enables BOTH early gates, so a test can assert the
// pre-execution checkpoint runs after the triage checkpoint in the same run.
const earlyBothGatesGraphConfig = "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\nearly_jev:\n  enabled: true\n  mode: review\n  gates:\n    task_triage: true\n    pre_execution: true\n"

// orderedEarlyAnalyzer records the purpose of every analysis request, so a test
// can assert the order in which the early checkpoints ran.
type orderedEarlyAnalyzer struct {
	purposes []jev.Purpose
	result   jev.Result
}

func (a *orderedEarlyAnalyzer) Analyze(_ context.Context, req jev.Request) (jev.Result, error) {
	a.purposes = append(a.purposes, req.Purpose)
	return a.result, nil
}

// TestGraphRunPreExecutionDisabledIsNoOp proves that with the pre-execution gate
// disabled (the default), a normal task runs through the standard local lifecycle
// unchanged: the analyzer is never consulted, no pre-execution checkpoint runs,
// and the task reaches LOCAL_DONE.
func TestGraphRunPreExecutionDisabledIsNoOp(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "Add widget", Objective: "Add the widget.", AcceptanceCriteria: "widget works", Status: domain.PLANNED, MaxAttempts: 3})
	a := &earlyCountingAgent{}
	analyzer := &earlyFakeAnalyzer{result: earlyEvidenceResult(jev.PurposePreExecution, jev.CategorySecurity, jev.SeverityHigh)}

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, "diff --git a/x b/x\n", a,
		func(config.Config) (jev.Analyzer, error) { return analyzer, nil },
		"run")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if analyzer.calls != 0 {
		t.Errorf("disabled pre-execution consulted the analyzer %d times, want 0", analyzer.calls)
	}
	if a.impl != 1 {
		t.Errorf("implementation ran %d times, want 1 (normal task unchanged)", a.impl)
	}
	if !strings.Contains(stdout, "T001 LOCAL_DONE") {
		t.Errorf("normal task did not complete: %q", stdout)
	}
	if _, ok := readEarlyArtifact(t, dir, "T001"); ok {
		t.Errorf("disabled pre-execution must not write an early artifact")
	}
}

// TestGraphRunPreExecutionClearContinues proves that when the gate is enabled and
// the analysis is clear, a normal bounded task still runs to completion, and the
// checkpoint runs after planning and before implementation.
func TestGraphRunPreExecutionClearContinues(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeConfig(t, dir, earlyPreExecGraphConfig)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "Add widget", Objective: "Add the widget.", AcceptanceCriteria: "widget works", Status: domain.PLANNED, MaxAttempts: 3})
	a := &earlyCountingAgent{}
	analyzer := &earlyFakeAnalyzer{result: earlyClearResult(jev.PurposePreExecution)}

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, "diff --git a/x b/x\n", a,
		func(config.Config) (jev.Analyzer, error) { return analyzer, nil },
		"run")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if analyzer.calls != 1 {
		t.Errorf("pre-execution analyzer calls = %d, want 1", analyzer.calls)
	}
	if a.impl != 1 {
		t.Errorf("implementation ran %d times, want 1", a.impl)
	}
	if !strings.Contains(stdout, "T001 LOCAL_DONE") {
		t.Errorf("task did not complete: %q", stdout)
	}

	art, ok := readEarlyArtifact(t, dir, "T001")
	if !ok {
		t.Fatalf("an enabled pre-execution checkpoint must write an early artifact")
	}
	if art.Checkpoint != runpkg.CheckpointPreExecution {
		t.Errorf("checkpoint = %q, want %q", art.Checkpoint, runpkg.CheckpointPreExecution)
	}
}

// TestGraphRunPreExecutionEscalationStopsAtHumanBoundary proves that a blocking
// pre-execution disposition in the graph-run path stops the task before
// implementation via the EXISTING human boundary (WAITING_FOR_HUMAN run stage +
// the existing requeue path), and introduces no new task state.
func TestGraphRunPreExecutionEscalationStopsAtHumanBoundary(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeConfig(t, dir, earlyPreExecGraphConfig)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "Add widget", Objective: "Add the widget.", AcceptanceCriteria: "widget works", Status: domain.PLANNED, MaxAttempts: 3})
	a := &earlyCountingAgent{}
	analyzer := &earlyFakeAnalyzer{result: earlyEvidenceResult(jev.PurposePreExecution, jev.CategorySecurity, jev.SeverityHigh)}

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, "diff --git a/x b/x\n", a,
		func(config.Config) (jev.Analyzer, error) { return analyzer, nil },
		"run")
	if code != exitError {
		t.Fatalf("code=%d, want %d (stderr=%s stdout=%s)", code, exitError, stderr, stdout)
	}
	if analyzer.calls != 1 {
		t.Errorf("pre-execution analyzer calls = %d, want 1", analyzer.calls)
	}
	if a.impl != 0 {
		t.Errorf("implementation ran %d times, want 0 (the task must not start)", a.impl)
	}

	// The escalation reuses the EXISTING WAITING_FOR_HUMAN run stage and the
	// existing requeue path (PLANNED). No new task state is introduced.
	if stage, ok := runpkg.Load(dir, "T001"); !ok || stage != runpkg.WaitingForHuman {
		t.Errorf("run stage = %q (loaded=%v), want WAITING_FOR_HUMAN", stage, ok)
	}

	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	got, err := st.Get("T001")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.PLANNED {
		t.Errorf("status = %s, want PLANNED (requeued through the existing path)", got.Status)
	}
}

// TestGraphRunPreExecutionInvalidEvidenceContinues proves that malformed/invalid
// evidence is fail-closed (recorded as a provider failure, never a finding) and
// does NOT escalate: the advisory path continues under deterministic policy.
func TestGraphRunPreExecutionInvalidEvidenceContinues(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeConfig(t, dir, earlyPreExecGraphConfig)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "Add widget", Objective: "Add the widget.", AcceptanceCriteria: "widget works", Status: domain.PLANNED, MaxAttempts: 3})
	a := &earlyCountingAgent{}
	// An unknown purpose makes the structured evidence invalid.
	bad := earlyEvidenceResult(jev.Purpose("MYSTERY"), jev.CategorySecurity, jev.SeverityHigh)
	analyzer := &earlyFakeAnalyzer{result: bad}

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, "diff --git a/x b/x\n", a,
		func(config.Config) (jev.Analyzer, error) { return analyzer, nil },
		"run")
	if code != exitOK {
		t.Fatalf("invalid advisory evidence must not block: code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if a.impl != 1 {
		t.Errorf("implementation ran %d times, want 1 (invalid evidence continues)", a.impl)
	}

	art, ok := readEarlyArtifact(t, dir, "T001")
	if !ok {
		t.Fatalf("a provider failure must be recorded as an early artifact")
	}
	if !art.ProviderFailed {
		t.Errorf("provider_failed = false, want true (invalid evidence fails closed)")
	}
	if len(art.Evidence.Items) != 0 {
		t.Errorf("invalid evidence carried %d evidence items, want 0", len(art.Evidence.Items))
	}
}

// TestGraphRunPreExecutionRunsAfterTriageBeforeImplementation proves the
// pre-execution checkpoint runs after the triage checkpoint and before
// implementation when both gates are enabled.
func TestGraphRunPreExecutionRunsAfterTriageBeforeImplementation(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeConfig(t, dir, earlyBothGatesGraphConfig)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "Add widget", Objective: "Add the widget.", AcceptanceCriteria: "widget works", Status: domain.PLANNED, MaxAttempts: 3})
	a := &earlyCountingAgent{}
	analyzer := &orderedEarlyAnalyzer{result: earlyClearResult(jev.PurposePreExecution)}

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, "diff --git a/x b/x\n", a,
		func(config.Config) (jev.Analyzer, error) { return analyzer, nil },
		"run")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if len(analyzer.purposes) != 2 {
		t.Fatalf("analysis invocations = %d, want 2 (triage then pre-execution): %v", len(analyzer.purposes), analyzer.purposes)
	}
	if analyzer.purposes[0] != jev.PurposeTaskTriage {
		t.Errorf("first analysis purpose = %q, want %q (triage before pre-execution)", analyzer.purposes[0], jev.PurposeTaskTriage)
	}
	if analyzer.purposes[1] != jev.PurposePreExecution {
		t.Errorf("second analysis purpose = %q, want %q", analyzer.purposes[1], jev.PurposePreExecution)
	}
	if a.impl != 1 {
		t.Errorf("implementation ran %d times, want 1", a.impl)
	}
}
