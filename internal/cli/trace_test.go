package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/model"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/runtrace"
)

// readRunTrace loads .agent-sdlc/runs/<id>/trace.json.
func readRunTrace(t *testing.T, dir, id string) runtrace.Trace {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, stateDirName, "runs", id, runtrace.FileName))
	if err != nil {
		t.Fatalf("read trace.json: %v", err)
	}
	var tr runtrace.Trace
	if err := json.Unmarshal(data, &tr); err != nil {
		t.Fatalf("parse trace.json: %v", err)
	}
	return tr
}

// TestRunTraceSuccessRecordsIdentityTrajectoryAndVerification proves a successful
// run persists a versioned trace with the run identity, the execution identity,
// an observed trajectory, the verification evidence, and a successful terminal.
func TestRunTraceSuccessRecordsIdentityTrajectoryAndVerification(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
	a := &fakeCapabilityAgent{plan: validPlanJSON, impl: "changed files", review: `{"summary":"clean","findings":[]}`}

	code, _, stderr := runInjectedCLI(t, dir, "diff --git a/pkg/x.go b/pkg/x.go\n--- a/pkg/x.go\n+++ b/pkg/x.go\n@@ -0,0 +1 @@\n+hello\n", a, "run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	tr := readRunTrace(t, dir, "T001")
	if tr.SchemaVersion != runtrace.SchemaVersion {
		t.Errorf("schema = %d, want %d", tr.SchemaVersion, runtrace.SchemaVersion)
	}
	if tr.RunID != "T001" || tr.TaskID != "T001" {
		t.Errorf("identity = %q/%q, want T001/T001", tr.RunID, tr.TaskID)
	}
	if tr.Execution.Capability != "implement" {
		t.Errorf("capability = %q, want implement", tr.Execution.Capability)
	}
	if len(tr.Verification) == 0 {
		t.Errorf("verification evidence not captured")
	}
	if tr.Termination.Stage != "PASSED" {
		t.Errorf("termination stage = %q, want PASSED", tr.Termination.Stage)
	}
	if len(tr.Iterations) == 0 {
		t.Errorf("agent trajectory not captured")
	}
	// AGENT-002: the successful run carries progress signals (a mutation, the
	// passing verifications, and the lifecycle transitions).
	if tr.SchemaVersion != runtrace.SchemaVersion {
		t.Errorf("schema = %d, want %d", tr.SchemaVersion, runtrace.SchemaVersion)
	}
	if tr.ProgressSummary.RepositoryMutations == 0 {
		t.Errorf("no repository-mutation progress signal: %+v", tr.ProgressSummary)
	}
	if tr.ProgressSummary.Verification == 0 {
		t.Errorf("no verification progress signal: %+v", tr.ProgressSummary)
	}
	if tr.ProgressSummary.StateTransitions == 0 {
		t.Errorf("no state-transition progress signal: %+v", tr.ProgressSummary)
	}
}

// TestRunTraceNoProgressPreservesSemantics proves a no-progress execution yields
// trace evidence that preserves the existing semantics: zero verified mutations,
// zero invocation-attributed changed files, the NO_PROGRESS kind and diagnostic,
// and no human requirement. The trace must not turn NO_PROGRESS into a retry,
// escalation, or approval.
func TestRunTraceNoProgressPreservesSemantics(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join("docs", "PLAN.md"), autoPlanDoc)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")
	a := outcomeAgent{outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: noProgressReason}}

	code, _, _ := runInjectedCLI(t, dir, "diff\n", a, "run")
	if code != exitError {
		t.Fatalf("code=%d, want %d", code, exitError)
	}
	tr := readRunTrace(t, dir, "S001")
	if tr.Termination.Kind != "NO_PROGRESS" {
		t.Errorf("termination kind = %q, want NO_PROGRESS", tr.Termination.Kind)
	}
	if tr.Termination.Disposition != "BLOCK" {
		t.Errorf("termination disposition = %q, want BLOCK", tr.Termination.Disposition)
	}
	if tr.Termination.HumanRequired {
		t.Errorf("no-progress must not require a human")
	}
	if tr.RepositoryMutations != 0 {
		t.Errorf("repository mutations = %d, want 0", tr.RepositoryMutations)
	}
	if len(tr.ChangedFiles) != 0 {
		t.Errorf("changed files = %v, want none", tr.ChangedFiles)
	}
	if !strings.Contains(tr.Termination.Diagnostic, "IMPLEMENT_NO_PROGRESS") {
		t.Errorf("diagnostic = %q, want it to carry IMPLEMENT_NO_PROGRESS", tr.Termination.Diagnostic)
	}
	// AGENT-002: the richer trace explains what happened without changing why SOP
	// stopped — zero verified mutations and zero verification signals remain.
	if tr.ProgressSummary.RepositoryMutations != 0 {
		t.Errorf("no-progress run recorded mutation progress: %+v", tr.ProgressSummary)
	}
	if tr.ProgressSummary.Verification != 0 {
		t.Errorf("no-progress run recorded verification progress: %+v", tr.ProgressSummary)
	}
}

// TestRunTraceMutationAttribution proves the trace records the invocation
// attributed changed files (never the whole dirty tree) and counts mutating
// iterations only.
func TestRunTraceMutationAttribution(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")
	a := &fakeCapabilityAgent{plan: validPlanJSON, impl: "changed files", review: `{"summary":"clean","findings":[]}`}

	if code, _, stderr := runInjectedCLI(t, dir, "diff --git a/pkg/x.go b/pkg/x.go\n--- a/pkg/x.go\n+++ b/pkg/x.go\n@@ -0,0 +1 @@\n+hello\n", a, "run", "--task", "TASK.md"); code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	// A no-op invocation (empty diff) must not attribute any change.
	dir2 := t.TempDir()
	writeFile(t, dir2, "TASK.md", runTaskFile)
	writeConfig(t, dir2, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")
	a2 := &fakeCapabilityAgent{plan: validPlanJSON, impl: "nothing"}
	_, _, _ = runInjectedCLI(t, dir2, "   \n", a2, "run", "--task", "TASK.md")
	tr2 := readRunTrace(t, dir2, "T001")
	if len(tr2.ChangedFiles) != 0 {
		t.Errorf("a no-change invocation attributed changed files: %v", tr2.ChangedFiles)
	}
	if tr2.RepositoryMutations != 0 {
		t.Errorf("a no-change invocation counted %d mutations, want 0", tr2.RepositoryMutations)
	}
}

// TestTraceExecutionDistinguishesDecisionFromExecutionTarget proves a SMALL
// decision that ran its cloud availability fallback stays distinguishable from a
// direct cloud selection: the class is the DECISION, the provider/model/locality
// and execution_source are the actual EXECUTION target.
func TestTraceExecutionDistinguishesDecisionFromExecutionTarget(t *testing.T) {
	res := lifeResult{routing: &taskRouting{
		Class:  model.ClassSmall,
		Source: runpkg.RoutingSourcePolicy,
		Selection: model.Selection{
			Class: model.ClassSmall, Provider: "ollama", Model: "qwen3:4b",
			Locality: model.LocalityLocal, Fallback: true,
		},
		ExecutionTarget: model.ExecutionTarget{
			Class: model.ClassSmall, Provider: "ollama", Model: "nemotron-3-nano:30b-cloud",
			Locality: model.LocalityCloud, Source: model.ExecutionSourceAvailabilityFallback,
		},
	}}
	ex := traceExecution(res, config.Config{})
	if ex.ModelClass != "small" {
		t.Errorf("model_class = %q, want small (the routing decision)", ex.ModelClass)
	}
	if ex.ExecutionSource != string(model.ExecutionSourceAvailabilityFallback) {
		t.Errorf("execution_source = %q, want availability-fallback", ex.ExecutionSource)
	}
	if ex.Model != "nemotron-3-nano:30b-cloud" || ex.Locality != "cloud" {
		t.Errorf("executed target = %q/%q, want the fallback model/cloud", ex.Model, ex.Locality)
	}
	if !ex.Fallback {
		t.Errorf("fallback flag not set for an availability fallback")
	}
	if ex.RoutingSource != string(runpkg.RoutingSourcePolicy) {
		t.Errorf("routing_source = %q, want policy", ex.RoutingSource)
	}
}

// TestRunTracePersistsNoSecrets proves a credential in the environment is not
// persisted into the trace.
func TestRunTracePersistsNoSecrets(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")
	t.Setenv("SOP_OLLAMA_API_KEY", "super-secret-token-xyz")
	a := &fakeCapabilityAgent{plan: validPlanJSON, impl: "changed files", review: `{"summary":"clean","findings":[]}`}

	if code, _, stderr := runInjectedCLI(t, dir, "diff --git a/pkg/x.go b/pkg/x.go\n--- a/pkg/x.go\n+++ b/pkg/x.go\n@@ -0,0 +1 @@\n+hello\n", a, "run", "--task", "TASK.md"); code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	data, err := os.ReadFile(filepath.Join(dir, stateDirName, "runs", "T001", runtrace.FileName))
	if err != nil {
		t.Fatalf("read trace.json: %v", err)
	}
	if strings.Contains(string(data), "super-secret-token-xyz") {
		t.Errorf("trace.json persisted a credential")
	}
}
