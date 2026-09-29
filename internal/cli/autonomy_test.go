package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/autonomy"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/jev"
	"github.com/imhttran/agentic-sop/internal/store"
)

// Risk-based autonomy integration tests (dogfood scenarios).
//
// They prove the lifecycle consults the autonomy policy: a bounded automation
// exhaustion is terminal under the high level rather than a human boundary, a
// conservative level withholds automatic fixes, and the decision is recorded and
// rendered so it is obvious why SOP stopped.

const (
	// failingBuildBalanced is the default (no autonomy block) with a failing build.
	failingBuildBalanced = "project:\n  name: x\nvalidation:\n  build:\n    - \"false\"\n  test:\n    - \"true\"\n"
	failingBuildHigh     = "project:\n  name: x\nvalidation:\n  build:\n    - \"false\"\n  test:\n    - \"true\"\nautonomy:\n  level: high\n"
	failingBuildLow      = "project:\n  name: x\nvalidation:\n  build:\n    - \"false\"\n  test:\n    - \"true\"\nautonomy:\n  level: low\n"
)

// neverFixingAgent implements but never repairs, so the bounded fix loop exhausts.
func neverFixingAgent() agent.Agent {
	return &fakeCapabilityAgent{plan: validPlanJSON, impl: "x", fix: "x", review: `{"summary":"clean","findings":[]}`}
}

func readClassificationArtifact(t *testing.T, dir, id string) classificationArtifact {
	t.Helper()
	path := filepath.Join(dir, stateDirName, "runs", id, "classification.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read classification artifact: %v", err)
	}
	var artifact classificationArtifact
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatalf("decode classification artifact: %v", err)
	}
	return artifact
}

// TestHighAutonomyExhaustionIsTerminalNotHuman is dogfood scenario 4 + the
// exhaustion requirement: under the high level a bounded fix exhaustion is a
// terminal automation state, not a human decision.
func TestHighAutonomyExhaustionIsTerminalNotHuman(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, failingBuildHigh)

	code, stdout, _ := runInjectedCLI(t, dir, "diff\n", neverFixingAgent(), "run", "--task", "TASK.md")
	if code != exitError {
		t.Fatalf("code=%d, want %d", code, exitError)
	}
	if strings.Contains(stdout, "NEEDS_HUMAN") {
		t.Errorf("HIGH exhaustion must not require a human:\n%s", stdout)
	}
	if !strings.Contains(stdout, "decision=TERMINAL") {
		t.Errorf("stdout missing the terminal autonomy decision:\n%s", stdout)
	}

	artifact := readClassificationArtifact(t, dir, "T001")
	if artifact.Autonomy == nil || artifact.Autonomy.Action != autonomy.ActionTerminal {
		t.Fatalf("classification artifact autonomy = %+v, want a TERMINAL action", artifact.Autonomy)
	}
	if artifact.Autonomy.Level != autonomy.High {
		t.Errorf("autonomy level = %s, want HIGH", artifact.Autonomy.Level)
	}
	if artifact.Kind != "AUTO_FIX_EXHAUSTED" {
		t.Errorf("classification kind = %s, want AUTO_FIX_EXHAUSTED", artifact.Kind)
	}
}

// TestBalancedAutonomyExhaustionStaysHuman proves the conservative default keeps
// the previous behavior: an exhausted fix loop remains a human boundary.
func TestBalancedAutonomyExhaustionStaysHuman(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, failingBuildBalanced)

	code, stdout, _ := runInjectedCLI(t, dir, "diff\n", neverFixingAgent(), "run", "--task", "TASK.md")
	if code != exitError {
		t.Fatalf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stdout, "NEEDS_HUMAN") {
		t.Errorf("balanced exhaustion must keep the human boundary:\n%s", stdout)
	}
}

// TestLowAutonomyWithholdsFixLoop proves the conservative level hands a
// deterministic failure to the policy instead of mutating: the fix loop never
// runs, and the decision requires approval.
func TestLowAutonomyWithholdsFixLoop(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, failingBuildLow)

	code, stdout, _ := runInjectedCLI(t, dir, "diff\n", neverFixingAgent(), "run", "--task", "TASK.md")
	if code != exitError {
		t.Fatalf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stdout, "fix cycles: 0/3") {
		t.Errorf("low autonomy must not run the fix loop:\n%s", stdout)
	}
	if !strings.Contains(stdout, "decision=HUMAN_APPROVAL_REQUIRED") {
		t.Errorf("stdout missing the human-approval decision:\n%s", stdout)
	}
}

// TestAutonomyDecisionIsRenderedInReport proves the decision is observable in the
// run report, so a reader sees the risk, the level, and the action.
func TestAutonomyDecisionIsRenderedInReport(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, failingBuildHigh)

	if code, _, _ := runInjectedCLI(t, dir, "diff\n", neverFixingAgent(), "run", "--task", "TASK.md"); code != exitError {
		t.Fatalf("code=%d, want %d", code, exitError)
	}
	report, err := os.ReadFile(filepath.Join(dir, stateDirName, "runs", "T001", "report.md"))
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	for _, want := range []string{"## Autonomy", "- Autonomy: `HIGH`", "- Decision: `TERMINAL`"} {
		if !strings.Contains(string(report), want) {
			t.Errorf("report missing %q:\n%s", want, report)
		}
	}
}

// TestHighAutonomyProductiveNoChangeContinues is dogfood scenario A: productive
// discovery that changed nothing is CONTINUE, never NEEDS_HUMAN, under HIGH.
func TestHighAutonomyProductiveNoChangeContinues(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\nautonomy:\n  level: high\n")
	a := outcomeAgent{outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: budgetExhaustedReason}}

	code, stdout, _ := runInjectedCLI(t, dir, "diff\n", a, "run", "--task", "TASK.md")
	if code != exitError {
		t.Fatalf("code=%d, want a non-pass continuation", code)
	}
	if strings.Contains(stdout, "NEEDS_HUMAN") {
		t.Errorf("productive no-change work must not be human:\n%s", stdout)
	}
	if !strings.Contains(stdout, "decision=AUTO_CONTINUE") {
		t.Errorf("stdout missing AUTO_CONTINUE:\n%s", stdout)
	}
}

// TestJEVFailureIsAutoRetryNotHuman is dogfood scenario D: a fail-closed JEV is a
// bounded analysis failure SOP retries, not a human decision.
func TestJEVFailureIsAutoRetryNotHuman(t *testing.T) {
	dir := setupJEVTask(t)
	analyzer := &sequencedJEV{errs: []error{errors.New("provider down")}}
	factory := func(config.Config) (jev.Analyzer, error) { return analyzer, nil }

	a := &evidenceAgent{implChanged: []string{"internal/a.go"}}
	code, stdout, _ := runInjectedCLIWithDiffFunc(t, dir, sequenceDiffs(modifiedDiff("internal/a.go")), a, factory, "run", "--task", "TASK.md")
	if code != exitError {
		t.Fatalf("code=%d, want a non-pass", code)
	}
	if strings.Contains(stdout, "NEEDS_HUMAN") {
		t.Errorf("a fail-closed JEV must not require a human:\n%s", stdout)
	}
	if !strings.Contains(stdout, "decision=AUTO_RETRY") {
		t.Errorf("stdout missing AUTO_RETRY:\n%s", stdout)
	}
}

// TestHighAutonomyAutoReconcilesEquivalentExecutedChange is dogfood scenario E: a
// descriptive-only change to an executed task is reconciled automatically under
// HIGH, with provenance and the history preserved.
func TestHighAutonomyAutoReconcilesEquivalentExecutedChange(t *testing.T) {
	dir := seedRAGProject(t)
	writeConfig(t, dir, "project:\n  name: Book RAG\nautonomy:\n  level: high\n")
	markTaskExecuted(t, dir, "S001")
	writeFile(t, dir, filepath.Join("docs", "PLAN.md"),
		strings.ReplaceAll(planRAGDoc, "Application skeleton", "Renamed skeleton"))

	code, stdout, stderr := runCLI(t, dir, "reconcile", filepath.Join("docs", "PLAN.md"))
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "auto-reconciled") || !strings.Contains(stdout, "S001") {
		t.Errorf("stdout missing the auto-reconciliation:\n%s", stdout)
	}

	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	task, err := st.Get("S001")
	if err != nil {
		t.Fatalf("get S001: %v", err)
	}
	if task.Title != "Renamed skeleton" {
		t.Errorf("title = %q, want the reconciled definition", task.Title)
	}
	if task.Status != domain.LOCAL_DONE || task.Attempt != 1 {
		t.Errorf("history must be preserved: %+v", task)
	}

	// Provenance is durable: plan.meta.json records the two definition hashes.
	meta, err := os.ReadFile(filepath.Join(dir, stateDirName, "plan.meta.json"))
	if err != nil {
		t.Fatalf("read plan.meta.json: %v", err)
	}
	if !strings.Contains(string(meta), "auto_reconciled") || !strings.Contains(string(meta), `"task_id": "S001"`) {
		t.Errorf("metadata missing the auto-reconciliation provenance:\n%s", meta)
	}
}

// TestHighAutonomyStillRequiresApprovalForMaterialExecutedChange proves a material
// change to executed semantics remains a human decision even under HIGH.
func TestHighAutonomyStillRequiresApprovalForMaterialExecutedChange(t *testing.T) {
	dir := seedRAGProject(t)
	writeConfig(t, dir, "project:\n  name: Book RAG\nautonomy:\n  level: high\n")
	markTaskExecuted(t, dir, "S001")
	// An acceptance criterion changes: the executable semantics materially changed.
	writeFile(t, dir, filepath.Join("docs", "PLAN.md"),
		strings.ReplaceAll(planRAGDoc, "Application starts successfully", "Application starts reliably"))

	code, _, stderr := runCLI(t, dir, "reconcile", filepath.Join("docs", "PLAN.md"))
	if code != exitError {
		t.Fatalf("code=%d, want %d (a material executed change needs approval)", code, exitError)
	}
	if !strings.Contains(stderr, "execution history") {
		t.Errorf("stderr = %q", stderr)
	}
}

// TestReconcileKeepsUnrelatedDirtyFile proves requirement 23: automatic
// reconciliation never touches an unrelated working-tree change.
func TestReconcileKeepsUnrelatedDirtyFile(t *testing.T) {
	dir := seedRAGProject(t)
	writeConfig(t, dir, "project:\n  name: Book RAG\nautonomy:\n  level: high\n")
	markTaskExecuted(t, dir, "S001")
	writeFile(t, dir, "keep.txt", "user work\n")
	writeFile(t, dir, filepath.Join("docs", "PLAN.md"),
		strings.ReplaceAll(planRAGDoc, "Application skeleton", "Renamed skeleton"))

	if code, _, stderr := runCLI(t, dir, "reconcile", filepath.Join("docs", "PLAN.md")); code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if data, err := os.ReadFile(filepath.Join(dir, "keep.txt")); err != nil || string(data) != "user work\n" {
		t.Errorf("unrelated working-tree file changed: %q err=%v", data, err)
	}
}
