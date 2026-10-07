package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/domain"
)

// readStateBytes reads the raw state database, so a test can prove a read-only
// command did not touch it.
func readStateBytes(t *testing.T, dir string) []byte {
	t.Helper()
	data, err := os.ReadFile(statePath(dir))
	if err != nil {
		t.Fatalf("read state.db: %v", err)
	}
	return data
}

// TestRunContinueRequiresCheck proves `sop continue` has no mutating mode: without
// --check it is a usage error, so an operator cannot mistake it for execution.
func TestRunContinueRequiresCheck(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	code, _, stderr := runCLI(t, dir, "continue")
	if code != exitUsage {
		t.Fatalf("code=%d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr, "--check") {
		t.Errorf("stderr should require --check: %q", stderr)
	}
}

// TestRunContinueUninitialized proves an uninitialized project is reported, not
// silently initialized.
func TestRunContinueUninitialized(t *testing.T) {
	code, _, stderr := runCLI(t, t.TempDir(), "continue", "--check")
	if code != exitError {
		t.Fatalf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "not initialized") {
		t.Errorf("stderr = %q", stderr)
	}
}

// activatePlan installs a plan graph without executing it.
func activatePlan(t *testing.T, dir, rel, doc string) {
	t.Helper()
	writeFile(t, dir, rel, doc)
	if code, _, stderr := runCLIWithAgent(t, dir, &planCountAgent{}, "plan", "activate", rel); code != exitOK {
		t.Fatalf("plan activate: code=%d stderr=%s", code, stderr)
	}
}

// TestRunContinueCleanAuthorizesRun proves a clean active plan is classified
// RECONCILE_CLEAN / CONTINUE_SAFE, exits zero, and does not touch the state
// database.
func TestRunContinueCleanAuthorizesRun(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	activatePlan(t, dir, "PLAN.md", autoPlanDoc)

	before := readStateBytes(t, dir)
	code, stdout, stderr := runCLI(t, dir, "continue", "--check", "--json")
	if code != exitOK {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}

	var doc continueCheckDoc
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("decode json: %v\n%s", err, stdout)
	}
	if doc.Classification != "RECONCILE_CLEAN" {
		t.Errorf("classification = %s, want RECONCILE_CLEAN", doc.Classification)
	}
	if doc.NextAction != "CONTINUE_SAFE" {
		t.Errorf("next_action = %s, want CONTINUE_SAFE", doc.NextAction)
	}
	if !doc.Deterministic {
		t.Error("a clean result must be deterministic")
	}
	if doc.Source != "PLAN.md" {
		t.Errorf("source = %q, want PLAN.md", doc.Source)
	}
	if doc.RunnableTask == "" {
		t.Error("a runnable task should be reported for a clean runnable plan")
	}
	if after := readStateBytes(t, dir); string(after) != string(before) {
		t.Error("sop continue --check mutated the state database")
	}
}

// TestRunContinueSemanticChangeStops proves a change to a task's acceptance
// semantics is a semantic change that exits non-zero and is not continued.
func TestRunContinueSemanticChangeStops(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	activatePlan(t, dir, "PLAN.md", autoPlanDoc)

	// A never-executed task whose acceptance meaning changed.
	changed := strings.Replace(autoPlanDoc, "- Application starts", "- Application starts within 2s", 1)
	writeFile(t, dir, "PLAN.md", changed)

	code, stdout, stderr := runCLI(t, dir, "continue", "--check", "--json")
	if code != exitError {
		t.Fatalf("code=%d, want %d (stdout=%s)", code, exitError, stdout)
	}
	var doc continueCheckDoc
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("decode json: %v\n%s", err, stdout)
	}
	if doc.Classification != "RECONCILE_SEMANTIC_CHANGE" {
		t.Errorf("classification = %s, want RECONCILE_SEMANTIC_CHANGE", doc.Classification)
	}
	if doc.NextAction != "HUMAN_REVIEW_REQUIRED" {
		t.Errorf("next_action = %s, want HUMAN_REVIEW_REQUIRED", doc.NextAction)
	}
	if !strings.Contains(stderr, "not authorized") {
		t.Errorf("stderr should report the stop: %q", stderr)
	}
}

// TestRunContinueMaterialExecutedChangeStops proves that changing an executed
// task's acceptance criteria stops the continuation as a semantic change.
func TestRunContinueMaterialExecutedChangeStops(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "docs/PLAN.md", autoPlanDoc)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
	a := &fakeCapabilityAgent{plan: validPlanJSON, impl: "x", review: `{"summary":"clean","findings":[]}`}

	// Complete the plan so the task is executed (LOCAL_DONE).
	if code, stdout, stderr := runInjectedCLI(t, dir, "diff\n", a, "run"); code != exitOK {
		t.Fatalf("run: code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}

	changed := strings.Replace(autoPlanDoc, "- Application starts", "- Application starts and exits 0", 1)
	writeFile(t, dir, "docs/PLAN.md", changed)

	code, stdout, _ := runCLI(t, dir, "continue", "--check", "--json")
	if code != exitError {
		t.Fatalf("code=%d, want %d (stdout=%s)", code, exitError, stdout)
	}
	var doc continueCheckDoc
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("decode json: %v\n%s", err, stdout)
	}
	if doc.Classification != "RECONCILE_SEMANTIC_CHANGE" {
		t.Errorf("classification = %s, want RECONCILE_SEMANTIC_CHANGE (%s)", doc.Classification, doc.Reason)
	}
	if len(doc.ChangedMaterial) == 0 {
		t.Errorf("changed_executed_material = %v, want at least one task", doc.ChangedMaterial)
	}
}

// TestRunContinueNoActivePlanStops proves a project with state but no recorded
// active plan reports a stop rather than inventing a graph.
func TestRunContinueNoActivePlanStops(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	// Seed a task but no plan provenance.
	seedTask(t, dir, &domain.Task{ID: "T001", Status: domain.PLANNED, Title: "t"})

	code, stdout, _ := runCLI(t, dir, "continue", "--check", "--json")
	if code != exitError {
		t.Fatalf("code=%d, want %d", code, exitError)
	}
	var doc continueCheckDoc
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("decode json: %v\n%s", err, stdout)
	}
	if doc.Classification != "RECONCILE_FAILED" {
		t.Errorf("classification = %s, want RECONCILE_FAILED (%s)", doc.Classification, doc.Reason)
	}
	if !strings.Contains(doc.Reason, "no active plan") {
		t.Errorf("reason %q should name the missing active plan", doc.Reason)
	}
}

// TestRunContinueBadArgs proves an unknown flag is a usage error.
func TestRunContinueBadArgs(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	code, _, stderr := runCLI(t, dir, "continue", "--check", "--nope")
	if code != exitUsage {
		t.Fatalf("code=%d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr, "unknown flag") {
		t.Errorf("stderr = %q", stderr)
	}
}

// TestRunContinueChangedThenReconcileThenClean proves the documented continuation
// path for a semantics-preserving change: the check classifies RECONCILE_CHANGED,
// the deterministic reconciliation applies it, and the plan checks clean.
func TestRunContinueChangedThenReconcileThenClean(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	activatePlan(t, dir, "PLAN.md", autoPlanDoc)

	// A plan-level metadata change that alters no task's definition.
	writeFile(t, dir, "PLAN.md", strings.Replace(autoPlanDoc, "Add a widget.", "Add a small widget.", 1))

	code, stdout, _ := runCLI(t, dir, "continue", "--check", "--json")
	if code != exitOK {
		t.Fatalf("check after change: code=%d stdout=%s", code, stdout)
	}
	var changed continueCheckDoc
	if err := json.Unmarshal([]byte(stdout), &changed); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if changed.Classification != "RECONCILE_CHANGED" || !changed.PlanChanged {
		t.Fatalf("classification=%s plan_changed=%v, want RECONCILE_CHANGED/true", changed.Classification, changed.PlanChanged)
	}

	// Apply the deterministic, semantics-preserving reconciliation.
	if code, out, errOut := runCLI(t, dir, "reconcile", "PLAN.md"); code != exitOK {
		t.Fatalf("reconcile: code=%d stdout=%s stderr=%s", code, out, errOut)
	}

	// The plan now checks clean, so `sop run` would proceed.
	code, stdout, _ = runCLI(t, dir, "continue", "--check", "--json")
	if code != exitOK {
		t.Fatalf("check after reconcile: code=%d stdout=%s", code, stdout)
	}
	var clean continueCheckDoc
	if err := json.Unmarshal([]byte(stdout), &clean); err != nil {
		t.Fatalf("decode json: %v", err)
	}
	if clean.Classification != "RECONCILE_CLEAN" {
		t.Errorf("classification = %s, want RECONCILE_CLEAN", clean.Classification)
	}
}

// TestRunContinueHumanOutput proves the human-readable report names the
// classification and the next action.
func TestRunContinueHumanOutput(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	activatePlan(t, dir, "PLAN.md", autoPlanDoc)

	code, stdout, stderr := runCLI(t, dir, "continue", "--check")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	for _, want := range []string{"RECONCILE_CLEAN", "CONTINUE_SAFE", "read-only", "PLAN.md"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout)
		}
	}
}

// TestRunContinueExplicitPlan proves an explicit plan argument is resolved.
func TestRunContinueExplicitPlan(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	activatePlan(t, dir, "PLAN.md", autoPlanDoc)

	code, stdout, stderr := runCLI(t, dir, "continue", "--check", "PLAN.md", "--json")
	if code != exitOK {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, filepath.Join("PLAN.md")) {
		t.Errorf("stdout should name the resolved source:\n%s", stdout)
	}
}
