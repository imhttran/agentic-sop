package cli

import (
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/jev"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/store"
)

// These tests exercise the early JEV task-triage checkpoint through the GRAPH
// RUN entry point (driveGraph -> runScheduledTask), not only single-task mode.
// They assert the integration contract from PRD-Phase-3: a policy escalation
// stops at the existing human boundary before implementation, and the gate is a
// strict no-op for normal tasks when disabled.

// earlyTriageGraphConfig enables the early JEV decision layer with ONLY the task
// triage gate on, so the triage checkpoint runs after task selection and before
// implementation in the graph-run path.
const earlyTriageGraphConfig = "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\nearly_jev:\n  enabled: true\n  mode: review\n  gates:\n    task_triage: true\n"

// TestGraphRunTriageDisabledIsNoOp proves that with the triage gate disabled
// (the default), a normal task runs through the standard local lifecycle
// unchanged: the analyzer is never consulted, no triage boundary is emitted, and
// the task reaches LOCAL_DONE.
func TestGraphRunTriageDisabledIsNoOp(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "Add widget", Objective: "Add the widget.", AcceptanceCriteria: "widget works", Status: domain.PLANNED, MaxAttempts: 3})
	a := &earlyCountingAgent{}
	analyzer := &earlyFakeAnalyzer{result: earlyEvidenceResult(jev.PurposeTaskTriage, jev.CategoryAmbiguity, jev.SeverityHigh)}

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, "diff --git a/x b/x\n", a,
		func(config.Config) (jev.Analyzer, error) { return analyzer, nil },
		"run")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if analyzer.calls != 0 {
		t.Errorf("disabled triage consulted the analyzer %d times, want 0", analyzer.calls)
	}
	if a.impl != 1 {
		t.Errorf("implementation ran %d times, want 1 (normal task unchanged)", a.impl)
	}
	if strings.Contains(stdout, "early JEV triage") {
		t.Errorf("disabled triage emitted a boundary: %q", stdout)
	}
	if !strings.Contains(stdout, "T001 LOCAL_DONE") {
		t.Errorf("normal task did not complete: %q", stdout)
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
	if got.Status != domain.LOCAL_DONE {
		t.Errorf("status = %s, want LOCAL_DONE", got.Status)
	}
}

// TestGraphRunTriageClearContinues proves that when the gate is enabled and the
// analysis is clear, a normal task still runs to completion unchanged.
func TestGraphRunTriageClearContinues(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeConfig(t, dir, earlyTriageGraphConfig)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "Add widget", Objective: "Add the widget.", AcceptanceCriteria: "widget works", Status: domain.PLANNED, MaxAttempts: 3})
	a := &earlyCountingAgent{}
	analyzer := &earlyFakeAnalyzer{result: earlyClearResult(jev.PurposeTaskTriage)}

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, "diff --git a/x b/x\n", a,
		func(config.Config) (jev.Analyzer, error) { return analyzer, nil },
		"run")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if analyzer.calls != 1 {
		t.Errorf("triage analyzer calls = %d, want 1", analyzer.calls)
	}
	if a.impl != 1 {
		t.Errorf("implementation ran %d times, want 1", a.impl)
	}
	if !strings.Contains(stdout, "T001 LOCAL_DONE") {
		t.Errorf("task did not complete: %q", stdout)
	}
}

// TestGraphRunTriageEscalationStopsAtHumanBoundary proves that a blocking triage
// disposition in the graph-run path stops the task before implementation via the
// EXISTING human boundary (WAITING_FOR_HUMAN + NEEDS_HUMAN approval request and
// the existing requeue path), and introduces no new task state.
func TestGraphRunTriageEscalationStopsAtHumanBoundary(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeConfig(t, dir, earlyTriageGraphConfig)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "Add widget", Objective: "Add the widget.", AcceptanceCriteria: "widget works", Status: domain.PLANNED, MaxAttempts: 3})
	a := &earlyCountingAgent{}
	analyzer := &earlyFakeAnalyzer{result: earlyEvidenceResult(jev.PurposeTaskTriage, jev.CategoryAmbiguity, jev.SeverityHigh)}

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, "diff --git a/x b/x\n", a,
		func(config.Config) (jev.Analyzer, error) { return analyzer, nil },
		"run")
	if code != exitError {
		t.Fatalf("code=%d, want %d (stderr=%s stdout=%s)", code, exitError, stderr, stdout)
	}
	if analyzer.calls != 1 {
		t.Errorf("triage analyzer calls = %d, want 1", analyzer.calls)
	}
	if a.impl != 0 {
		t.Errorf("implementation ran %d times, want 0 (the task must not start)", a.impl)
	}
	if !strings.Contains(stdout, "T001 NEEDS_HUMAN (early JEV triage)") {
		t.Errorf("stdout missing the triage human boundary: %q", stdout)
	}

	// The escalation reuses the existing WAITING_FOR_HUMAN run stage and the
	// existing requeue path: the task status is drawn only from the pre-existing
	// set (PLANNED after requeue). No new task state is introduced.
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

	// The run stage is the existing WAITING_FOR_HUMAN, not a new state.
	if stage, ok := runpkg.Load(dir, "T001"); !ok || stage != runpkg.WaitingForHuman {
		t.Errorf("run stage = %q (loaded=%v), want WAITING_FOR_HUMAN", stage, ok)
	}
}

// TestGraphRunTriageProviderFailureContinues proves that a provider failure at
// triage is recorded, never treated as a finding, and does not block a normal
// task in the graph-run path.
func TestGraphRunTriageProviderFailureContinues(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeConfig(t, dir, earlyTriageGraphConfig)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "Add widget", Objective: "Add the widget.", AcceptanceCriteria: "widget works", Status: domain.PLANNED, MaxAttempts: 3})
	a := &earlyCountingAgent{}
	analyzer := &earlyFakeAnalyzer{err: jev.ErrProviderFailure}

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, "diff --git a/x b/x\n", a,
		func(config.Config) (jev.Analyzer, error) { return analyzer, nil },
		"run")
	if code != exitOK {
		t.Fatalf("an advisory provider failure must not block: code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if strings.Contains(stdout, "early JEV triage") {
		t.Errorf("a provider failure must not escalate: %q", stdout)
	}
	if a.impl != 1 {
		t.Errorf("implementation ran %d times, want 1", a.impl)
	}
}
