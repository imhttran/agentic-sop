package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/failure"
	"github.com/imhttran/agentic-sop/internal/store"
)

// errorAgent returns a fixed infrastructure error for every capability except
// PLAN (so plan preparation succeeds and the failure happens at execution).
type errorAgent struct{ err error }

func (a errorAgent) Generate(_ context.Context, r agent.Request) (agent.Response, error) {
	if r.Capability == agent.Plan {
		return agent.Response{Content: validPlanJSON}, nil
	}
	return agent.Response{}, a.err
}

// TestRunGraphBudgetExhaustionContinues proves test 8: an agent that exhausted its
// iterations with unfinished work is CONTINUE, not a hard block — the task is
// requeued so a later run continues, and the classification is recorded.
func TestRunGraphBudgetExhaustionContinues(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join("docs", "PLAN.md"), autoPlanDoc)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")
	a := outcomeAgent{outcome: &agent.Outcome{
		Status: agent.OutcomeFailed,
		Reason: "the Ollama agent IMPLEMENT did not complete after 24 iterations (mutation_observed=true, termination=iteration_limit)",
	}}

	code, stdout, _ := runInjectedCLI(t, dir, "diff\n", a, "run")
	if code != exitError {
		t.Fatalf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stdout, "CONTINUE") {
		t.Errorf("stdout = %q, want the CONTINUE disposition", stdout)
	}

	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	got, err := st.Get("S001")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.Status != domain.PLANNED {
		t.Errorf("status = %s, want PLANNED (CONTINUE requeues rather than blocking)", got.Status)
	}

	// The classification is persisted as a run artifact with the disposition and
	// reason, so the recovery decision is auditable.
	cls := readClassification(t, dir, "S001")
	if cls.Disposition != failure.Continue {
		t.Errorf("recorded disposition = %s, want CONTINUE", cls.Disposition)
	}
	if cls.Kind != failure.IncompleteImplementation {
		t.Errorf("recorded kind = %s, want INCOMPLETE_IMPLEMENTATION", cls.Kind)
	}
	if strings.TrimSpace(cls.Reason) == "" {
		t.Error("recorded classification has no reason")
	}
}

// TestRunGraphTransientProviderRetries proves test 10: a transient provider
// failure is RETRY — the task is requeued, not terminally blocked.
func TestRunGraphTransientProviderRetries(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join("docs", "PLAN.md"), autoPlanDoc)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")
	a := errorAgent{err: fmt.Errorf("request failed: dial tcp 127.0.0.1:11434: connect: connection refused")}

	code, stdout, _ := runInjectedCLI(t, dir, "diff\n", a, "run")
	if code != exitError {
		t.Fatalf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stdout, "RETRY") {
		t.Errorf("stdout = %q, want the RETRY disposition", stdout)
	}

	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	got, err := st.Get("S001")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.Status != domain.PLANNED {
		t.Errorf("status = %s, want PLANNED (RETRY requeues)", got.Status)
	}
	if cls := readClassification(t, dir, "S001"); cls.Disposition != failure.Retry {
		t.Errorf("recorded disposition = %s, want RETRY", cls.Disposition)
	}
}

// TestRunGraphContinueIsBounded proves test 15 at the lifecycle level: a task that
// keeps exhausting its budget is not continued forever — the configured
// continuation budget (max_continuations, default 4) eventually blocks it
// terminally, as a bounded automation failure rather than a human decision.
func TestRunGraphContinueIsBounded(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join("docs", "PLAN.md"), autoPlanDoc)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")

	// Each attempt reports a distinct incomplete reason so it makes "progress";
	// the bound is the continuation budget (default 4), separate from max_attempts.
	for i := 0; i < 4; i++ {
		a := outcomeAgent{outcome: &agent.Outcome{
			Status: agent.OutcomeFailed,
			Reason: fmt.Sprintf("did not complete after 24 iterations (termination=iteration_limit, attempt=%d)", i),
		}}
		if code, _, _ := runInjectedCLI(t, dir, "diff\n", a, "run"); code != exitError {
			t.Fatalf("run %d: code=%d, want %d", i, code, exitError)
		}
	}

	a := outcomeAgent{outcome: &agent.Outcome{
		Status: agent.OutcomeFailed,
		Reason: "did not complete after 24 iterations (termination=iteration_limit, final)",
	}}
	code, stdout, _ := runInjectedCLI(t, dir, "diff\n", a, "run")
	if code != exitError {
		t.Fatalf("final run: code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stdout, "continuation budget exhausted") {
		t.Errorf("stdout = %q, want the bounded continuation budget message", stdout)
	}
	if strings.Contains(stdout, "NEEDS_HUMAN") {
		t.Errorf("stdout = %q, a bounded continuation exhaustion must not be reported as NEEDS_HUMAN", stdout)
	}

	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	got, err := st.Get("S001")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.Status != domain.BLOCKED {
		t.Errorf("status = %s, want BLOCKED after the continuation budget is spent", got.Status)
	}
}

// TestRunGraphHardFailureStillBlocks proves a genuine, non-retryable failure is
// still terminal: the classifier does not weaken the fail-closed default.
func TestRunGraphHardFailureStillBlocks(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join("docs", "PLAN.md"), autoPlanDoc)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")
	a := outcomeAgent{outcome: &agent.Outcome{Status: agent.OutcomeFailed, Reason: "boom"}}

	if code, _, _ := runInjectedCLI(t, dir, "diff\n", a, "run"); code != exitError {
		t.Fatalf("code=%d, want %d", code, exitError)
	}

	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	got, err := st.Get("S001")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if got.Status != domain.BLOCKED {
		t.Errorf("status = %s, want BLOCKED (a hard failure is terminal)", got.Status)
	}
}

// TestRunValidationExhaustionIsClassified proves the AUTO_FIX path is recorded:
// a task whose bounded fix loop cannot resolve a failing check is classified
// AUTO_FIX_EXHAUSTED and reaches NEEDS_HUMAN.
func TestRunValidationExhaustionIsClassified(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"false\"\n  test:\n    - \"true\"\n")
	a := &fakeCapabilityAgent{plan: validPlanJSON, impl: "x", fix: "x", review: `{"summary":"clean","findings":[]}`}

	code, stdout, _ := runInjectedCLI(t, dir, "diff\n", a, "run", "--task", "TASK.md")
	if code != exitError {
		t.Fatalf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stdout, "NEEDS_HUMAN") {
		t.Errorf("stdout = %q, want NEEDS_HUMAN", stdout)
	}
	// The task-file run records the classification from the gate.
	cls := readClassification(t, dir, "T001")
	if cls.Disposition != failure.NeedsHuman || cls.Kind != failure.AutoFixExhausted {
		t.Errorf("classification = %+v, want NEEDS_HUMAN/AUTO_FIX_EXHAUSTED", cls)
	}

	// It is also recorded in the run report, so `sop report` can surface it.
	reportPath := filepath.Join(dir, stateDirName, "runs", "T001", "report.json")
	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read report.json: %v", err)
	}
	if !strings.Contains(string(data), `"classification"`) {
		t.Errorf("report.json does not record the classification:\n%s", data)
	}
}

// readClassification reads the classification.json run artifact for a run id.
func readClassification(t *testing.T, dir, runID string) failure.Classification {
	t.Helper()
	path := filepath.Join(dir, stateDirName, "runs", runID, "classification.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read classification artifact %s: %v", path, err)
	}
	var cls failure.Classification
	if err := json.Unmarshal(data, &cls); err != nil {
		t.Fatalf("parse classification artifact: %v", err)
	}
	return cls
}
