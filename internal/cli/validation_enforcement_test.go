package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/store"
)

// TestRunTaskWithoutConfiguredValidationDoesNotPass is the end-to-end guarantee: a
// task that changes the repository with no validation configured must not pass. The
// run stops at a distinct configuration failure, not an implementation failure and
// not a human approval.
func TestRunTaskWithoutConfiguredValidationDoesNotPass(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, "project:\n  name: x\n") // no validation configured
	a := &fakeCapabilityAgent{plan: validPlanJSON, impl: "changed files", review: `{"summary":"clean","findings":[]}`}

	code, stdout, stderr := runInjectedCLI(t, dir, "diff --git a/x b/x\n", a, "run", "--task", "TASK.md")
	if code == exitOK {
		t.Fatalf("a mutating task with no configured validation must not pass; stdout=%s stderr=%s", stdout, stderr)
	}
	if !strings.Contains(stdout, "required validation is not configured") {
		t.Errorf("stdout = %q, want the configuration failure reason", stdout)
	}
	if !strings.Contains(stdout, "VALIDATION_NOT_CONFIGURED") {
		t.Errorf("stdout = %q, want the distinct configuration classification", stdout)
	}
	// A configuration failure is a terminal automation block, not a commit approval.
	if strings.Contains(stdout, "human approval required before commit") {
		t.Errorf("a validation configuration failure must not claim a commit approval; stdout=%s", stdout)
	}
}

// TestRunTaskMaterializesValidationAndPasses proves the configless path is fixed: a
// single --task run without a configuration materializes the documented default
// (which carries the Go validation policy), runs it, and can pass.
func TestRunTaskMaterializesValidationAndPasses(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	// A minimal, self-contained Go module so the materialized default validation
	// (go build/test/vet/gofmt) can actually pass.
	writeFile(t, dir, "go.mod", "module example.com/smoke\n\ngo 1.21\n")
	writeFile(t, dir, "smoke.go", "package smoke\n")
	writeFile(t, dir, "smoke_test.go", "package smoke\n\nimport \"testing\"\n\nfunc TestNothing(t *testing.T) {}\n")
	a := &fakeCapabilityAgent{plan: validPlanJSON, impl: "changed files", review: `{"summary":"clean","findings":[]}`}

	code, stdout, stderr := runInjectedCLI(t, dir, "diff --git a/x b/x\n", a, "run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "PASS") {
		t.Errorf("stdout = %q, want PASS", stdout)
	}
	if _, err := os.Stat(filepath.Join(dir, config.DirName, config.FileName)); err != nil {
		t.Errorf("a configless --task run must materialize the default configuration: %v", err)
	}
}

// TestRunPlanWithoutConfiguredValidationBlocksWithReason is the plan-path
// guarantee (SOP's ordinary `sop run` execution, not only the ad-hoc `--task`
// path): a task that changes the repository with no validation configured is
// blocked, and the persisted block reason names the configuration failure rather
// than an exhausted retry budget.
func TestRunPlanWithoutConfiguredValidationBlocksWithReason(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join("docs", "PLAN.md"), autoPlanDoc)
	writeConfig(t, dir, "project:\n  name: x\n") // no validation configured
	a := &fakeCapabilityAgent{plan: validPlanJSON, impl: "changed files", review: `{"summary":"clean","findings":[]}`}

	code, stdout, stderr := runInjectedCLI(t, dir, "diff --git a/x b/x\n", a, "run")
	if code != exitError {
		t.Fatalf("code=%d, want %d (stdout=%s stderr=%s)", code, exitError, stdout, stderr)
	}
	if !strings.Contains(stdout, "VALIDATION_NOT_CONFIGURED") {
		t.Errorf("stdout = %q, want the configuration classification", stdout)
	}
	if strings.Contains(stdout, "human approval required before commit") {
		t.Errorf("a validation configuration failure must not claim a commit approval; stdout=%s", stdout)
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
		t.Errorf("status = %s, want BLOCKED (terminal)", got.Status)
	}
	if got.BlockedReason != domain.VALIDATION_NOT_CONFIGURED {
		t.Errorf("persisted blocked reason = %q, want %q", got.BlockedReason, domain.VALIDATION_NOT_CONFIGURED)
	}
}

// TestRunPlanHumanBoundaryOutranksMissingValidation proves the human boundary wins
// over the missing-validation enforcement end to end: a needs-human execution is
// reported as a human decision (and requeued for a later run), never reclassified as
// a validation-configuration failure. It is the lifecycle-level counterpart of the
// quality-gate unit test.
func TestRunPlanHumanBoundaryOutranksMissingValidation(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join("docs", "PLAN.md"), autoPlanDoc)
	writeConfig(t, dir, "project:\n  name: x\n") // no validation configured either
	a := outcomeAgent{outcome: &agent.Outcome{
		Status: agent.OutcomeNeedsHuman,
		Reason: "this would expose local command execution to unauthenticated remote clients",
	}}

	code, stdout, _ := runInjectedCLI(t, dir, "diff\n", a, "run")
	if code != exitError {
		t.Fatalf("code=%d, want %d (stdout=%s)", code, exitError, stdout)
	}
	if !strings.Contains(stdout, "NEEDS_HUMAN") {
		t.Errorf("stdout = %q, want the human boundary", stdout)
	}
	if strings.Contains(stdout, "VALIDATION_NOT_CONFIGURED") {
		t.Errorf("a human boundary must outrank the missing-validation enforcement; stdout=%s", stdout)
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
	if got.Status == domain.BLOCKED && got.BlockedReason == domain.VALIDATION_NOT_CONFIGURED {
		t.Errorf("human boundary reclassified as a validation-config failure: %+v", got)
	}
}

// TestRunNoChangeClaimWithoutValidationIsNotAValidationConfigFailure proves the
// enforcement does not over-trigger: a task whose invocation produced no repository
// change is classified by the existing bounded no-change path, not as a
// missing-validation configuration failure.
func TestRunNoChangeClaimWithoutValidationIsNotAValidationConfigFailure(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, "project:\n  name: x\n") // no validation configured
	a := outcomeAgent{outcome: &agent.Outcome{Status: agent.OutcomeCompleted, ChangesExpected: true}}

	code, stdout, _ := runInjectedCLI(t, dir, "   \n", a, "run", "--task", "TASK.md")
	if code == exitOK {
		t.Fatalf("a claimed change with none present must not pass; stdout=%s", stdout)
	}
	if !strings.Contains(stdout, "no repository changes") {
		t.Errorf("stdout = %q, want the bounded no-change diagnostic", stdout)
	}
	if strings.Contains(stdout, "VALIDATION_NOT_CONFIGURED") {
		t.Errorf("a no-change failure must not be reported as a validation-configuration failure; stdout=%s", stdout)
	}
}

// TestRunChangesExpectedFalseWithoutValidationIsNotAValidationConfigFailure pins the
// changes_expected:false path: a legacy no-change completion with no configured
// validation uses the existing bounded no-change classification, never the
// missing-validation configuration failure (which is reserved for a task that
// actually changed the repository).
func TestRunChangesExpectedFalseWithoutValidationIsNotAValidationConfigFailure(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, "project:\n  name: x\n") // no validation configured
	a := outcomeAgent{outcome: &agent.Outcome{Status: agent.OutcomeCompleted, ChangesExpected: false}}

	code, stdout, _ := runInjectedCLI(t, dir, "   \n", a, "run", "--task", "TASK.md")
	if code == exitOK {
		t.Fatalf("an unproven no-change completion must not pass; stdout=%s", stdout)
	}
	if !strings.Contains(stdout, "IMPLEMENT_NO_CHANGES") {
		t.Errorf("stdout = %q, want the bounded no-change classification", stdout)
	}
	if strings.Contains(stdout, "VALIDATION_NOT_CONFIGURED") {
		t.Errorf("a changes_expected:false completion must not be reported as a validation-configuration failure; stdout=%s", stdout)
	}
}
