package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/store"
)

// The CLI historicalization tests drive the dispatcher end to end. They prove the
// command is model-free, fails closed on ineligible plans, reports readiness
// without mutation, and is idempotent.

// activatePlanAndComplete marks a task satisfied with verification evidence, so
// the active plan is eligible for a normal historicalization.
func activatePlanAndComplete(t *testing.T, dir, id string) {
	t.Helper()
	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	tasks, err := st.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	found := false
	for _, task := range tasks {
		if task.ID == id {
			task.Status = domain.LOCAL_DONE
			if err := st.Save(task); err != nil {
				t.Fatalf("save task: %v", err)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("task %s not found", id)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".agent-sdlc", "runs", id), 0o755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}
	writeFile(t, dir, filepath.Join(".agent-sdlc", "runs", id, "state.json"), `{"id":"`+id+`","stage":"PASSED"}`+"\n")
}

// TestRunPlanHistoricalizeArchivesCompletedPlan proves a satisfied, verified plan
// is historicalized as COMPLETE and removed from ACTIVE selection.
func TestRunPlanHistoricalizeArchivesCompletedPlan(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeFile(t, dir, "PLAN.md", autoPlanDoc)
	if code, _, errOut := runCLIWithAgent(t, dir, &planCountAgent{}, "plan", "activate", "PLAN.md"); code != exitOK {
		t.Fatalf("activate: code=%d stderr=%s", code, errOut)
	}
	activatePlanAndComplete(t, dir, "S001")

	a := &planCountAgent{}
	code, out, errOut := runCLIWithAgent(t, dir, a, "plan", "historicalize")
	if code != exitOK {
		t.Fatalf("historicalize: code=%d stdout=%s stderr=%s", code, out, errOut)
	}
	if a.calls != 0 {
		t.Errorf("historicalize invoked the model %d time(s); it must not", a.calls)
	}
	if !strings.Contains(out, "HISTORICALIZED") || !strings.Contains(out, "COMPLETE") {
		t.Errorf("stdout = %q", out)
	}
	code, out2, _ := runCLI(t, dir, "status")
	if code != exitOK || strings.Contains(out2, "State: ACTIVE") {
		t.Errorf("the historicalized plan must not be ACTIVE: %s", out2)
	}
	if !stateExists(filepath.Join(dir, ".agent-sdlc", "archive", "plan", "lifecycle.json")) {
		t.Error("the plan was not archived")
	}
}

// TestRunPlanHistoricalizeCheckIsReadOnly proves --check reports readiness and
// mutates nothing when the plan is ineligible.
func TestRunPlanHistoricalizeCheckIsReadOnly(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeFile(t, dir, "PLAN.md", autoPlanDoc)
	if code, _, errOut := runCLIWithAgent(t, dir, &planCountAgent{}, "plan", "activate", "PLAN.md"); code != exitOK {
		t.Fatalf("activate: code=%d stderr=%s", code, errOut)
	}

	code, out, _ := runCLI(t, dir, "plan", "historicalize", "--check")
	if code == exitOK {
		t.Fatalf("--check must fail closed on an ineligible plan: %s", out)
	}
	if !strings.Contains(out, "Eligible: no") || !strings.Contains(out, "PLANNED") {
		t.Errorf("stdout = %q", out)
	}
	code, out2, _ := runCLI(t, dir, "status")
	if code != exitOK || !strings.Contains(out2, "State: ACTIVE") {
		t.Errorf("--check must not mutate: %s", out2)
	}
	if stateExists(filepath.Join(dir, ".agent-sdlc", "archive", "plan", "lifecycle.json")) {
		t.Error("--check must not archive")
	}
}

// TestRunPlanHistoricalizeCheckEligibleSucceeds proves --check exits cleanly for
// an eligible plan without mutating it.
func TestRunPlanHistoricalizeCheckEligibleSucceeds(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeFile(t, dir, "PLAN.md", autoPlanDoc)
	if code, _, errOut := runCLIWithAgent(t, dir, &planCountAgent{}, "plan", "activate", "PLAN.md"); code != exitOK {
		t.Fatalf("activate: code=%d stderr=%s", code, errOut)
	}
	activatePlanAndComplete(t, dir, "S001")

	code, out, errOut := runCLI(t, dir, "plan", "historicalize", "--check")
	if code != exitOK {
		t.Fatalf("--check: code=%d stdout=%s stderr=%s", code, out, errOut)
	}
	if !strings.Contains(out, "Eligible: yes") {
		t.Errorf("stdout = %q", out)
	}
	if stateExists(filepath.Join(dir, ".agent-sdlc", "archive", "plan", "lifecycle.json")) {
		t.Error("--check must not archive")
	}
}

// TestRunPlanHistoricalizeRefusesIneligible proves the mutation refuses an
// unverified plan and leaves it ACTIVE.
func TestRunPlanHistoricalizeRefusesIneligible(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeFile(t, dir, "PLAN.md", autoPlanDoc)
	if code, _, errOut := runCLIWithAgent(t, dir, &planCountAgent{}, "plan", "activate", "PLAN.md"); code != exitOK {
		t.Fatalf("activate: code=%d stderr=%s", code, errOut)
	}

	code, out, errOut := runCLI(t, dir, "plan", "historicalize")
	if code == exitOK {
		t.Fatalf("historicalize must refuse unresolved work: %s", out)
	}
	if !strings.Contains(errOut, "not eligible") {
		t.Errorf("stderr = %q", errOut)
	}
	if !strings.Contains(out, "No mutation performed") {
		t.Errorf("stdout = %q", out)
	}
	code, out2, _ := runCLI(t, dir, "status")
	if code != exitOK || !strings.Contains(out2, "State: ACTIVE") {
		t.Errorf("a refused historicalization must leave the plan ACTIVE: %s", out2)
	}
}

// TestRunPlanHistoricalizeIdempotent proves a repeated invocation is a no-op.
func TestRunPlanHistoricalizeIdempotent(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeFile(t, dir, "PLAN.md", autoPlanDoc)
	if code, _, errOut := runCLIWithAgent(t, dir, &planCountAgent{}, "plan", "activate", "PLAN.md"); code != exitOK {
		t.Fatalf("activate: code=%d stderr=%s", code, errOut)
	}
	activatePlanAndComplete(t, dir, "S001")

	if code, _, errOut := runCLI(t, dir, "plan", "historicalize", "PLAN.md"); code != exitOK {
		t.Fatalf("first historicalize: code=%d stderr=%s", code, errOut)
	}
	code, out, errOut := runCLI(t, dir, "plan", "historicalize", "PLAN.md")
	if code != exitOK {
		t.Fatalf("second historicalize: code=%d stdout=%s stderr=%s", code, out, errOut)
	}
	if !strings.Contains(out, "Already historicalized") {
		t.Errorf("stdout = %q, want an already-historicalized no-op", out)
	}
}

// TestRunPlanHistoricalizeJSON proves the machine-readable output reports the
// readiness and the outcome.
func TestRunPlanHistoricalizeJSON(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeFile(t, dir, "PLAN.md", autoPlanDoc)
	if code, _, errOut := runCLIWithAgent(t, dir, &planCountAgent{}, "plan", "activate", "PLAN.md"); code != exitOK {
		t.Fatalf("activate: code=%d stderr=%s", code, errOut)
	}
	activatePlanAndComplete(t, dir, "S001")

	code, out, errOut := runCLI(t, dir, "plan", "historicalize", "--json")
	if code != exitOK {
		t.Fatalf("json: code=%d stdout=%s stderr=%s", code, out, errOut)
	}
	for _, want := range []string{`"outcome": "HISTORICALIZED"`, `"eligible": true`, `"state": "HISTORICALIZED"`} {
		if !strings.Contains(out, want) {
			t.Errorf("json output missing %q: %s", want, out)
		}
	}
}

// TestRunPlanHistoricalizeCheckJSONReportsIneligible proves --check --json reports
// an ineligible plan with its blockers as data.
func TestRunPlanHistoricalizeCheckJSONReportsIneligible(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeFile(t, dir, "PLAN.md", autoPlanDoc)
	if code, _, errOut := runCLIWithAgent(t, dir, &planCountAgent{}, "plan", "activate", "PLAN.md"); code != exitOK {
		t.Fatalf("activate: code=%d stderr=%s", code, errOut)
	}

	code, out, _ := runCLI(t, dir, "plan", "historicalize", "--check", "--json")
	if code == exitOK {
		t.Fatalf("--check must fail closed: %s", out)
	}
	for _, want := range []string{`"eligible": false`, `"state": "ACTIVE"`, `"unresolved_tasks": [`, `"S001 (PLANNED)"`} {
		if !strings.Contains(out, want) {
			t.Errorf("json output missing %q: %s", want, out)
		}
	}
}

// TestRunPlanHistoricalizeUsageErrors proves flags and dispositions are validated.
func TestRunPlanHistoricalizeUsageErrors(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)

	if code, _, _ := runCLI(t, dir, "plan", "historicalize", "--bogus"); code != exitUsage {
		t.Errorf("unknown flag: code=%d, want %d", code, exitUsage)
	}
	if code, _, _ := runCLI(t, dir, "plan", "historicalize", "--disposition"); code != exitUsage {
		t.Errorf("missing disposition: code=%d, want %d", code, exitUsage)
	}
	if code, _, _ := runCLI(t, dir, "plan", "historicalize", "--disposition", "DEFERRED"); code == exitOK {
		t.Errorf("unknown disposition must fail")
	}
}

// TestRunHistoricalizesOnCompletionWhenEnabled proves the opt-in final lifecycle
// stage archives a fully satisfied plan after a successful `sop run`.
func TestRunHistoricalizesOnCompletionWhenEnabled(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join("docs", "PLAN-Hardening.md"), autoPlanDoc)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
	a := &fakeCapabilityAgent{plan: validPlanJSON, impl: "x", review: `{"summary":"clean","findings":[]}`}
	t.Setenv("SOP_HISTORICALIZE_ON_COMPLETION", "true")

	code, stdout, stderr := runInjectedCLI(t, dir, "diff\n", a, "run", filepath.Join("docs", "PLAN-Hardening.md"))
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "Historicalized (COMPLETE)") {
		t.Fatalf("stdout = %s", stdout)
	}
	if !stateExists(filepath.Join(dir, ".agent-sdlc", "archive", "plan-hardening", "lifecycle.json")) {
		t.Error("the completed plan was not archived")
	}
	code, out2, _ := runCLI(t, dir, "status")
	if code != exitOK || strings.Contains(out2, "State: ACTIVE") {
		t.Errorf("the plan must not be ACTIVE after on-completion historicalization: %s", out2)
	}
}

// TestRunDoesNotHistoricalizeByDefault proves the final stage is OFF unless the
// operator opts in, so existing execution behavior is unchanged.
func TestRunDoesNotHistoricalizeByDefault(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join("docs", "PLAN-Hardening.md"), autoPlanDoc)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
	a := &fakeCapabilityAgent{plan: validPlanJSON, impl: "x", review: `{"summary":"clean","findings":[]}`}
	t.Setenv("SOP_HISTORICALIZE_ON_COMPLETION", "")

	code, stdout, stderr := runInjectedCLI(t, dir, "diff\n", a, "run", filepath.Join("docs", "PLAN-Hardening.md"))
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if strings.Contains(stdout, "Historicalized") {
		t.Errorf("historicalization must be opt-in: %s", stdout)
	}
	if stateExists(filepath.Join(dir, ".agent-sdlc", "archive", "plan-hardening", "lifecycle.json")) {
		t.Error("the plan must not be archived by default")
	}
}

// TestHistoricalizeOnCompletionPreservesApprovalBoundary proves a pending human
// gate is never crossed by the automatic final stage: the plan stays ACTIVE.
func TestHistoricalizeOnCompletionPreservesApprovalBoundary(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "S001", Title: "t", Status: domain.LOCAL_DONE, MaxAttempts: 3})
	if err := os.MkdirAll(filepath.Join(dir, ".agent-sdlc", "runs", "S001"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join(".agent-sdlc", "runs", "S001", "state.json"), `{"id":"S001","stage":"PASSED"}`+"\n")
	writeFile(t, dir, filepath.Join(".agent-sdlc", "runs", "S001", "approval.json"),
		`{"id":"req-1","task_id":"S001","kind":"NEEDS_HUMAN","target":"S001","status":"PENDING"}`+"\n")
	t.Setenv("SOP_HISTORICALIZE_ON_COMPLETION", "true")

	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	var out, errOut bytes.Buffer
	historicalizeOnCompletion(dir, st, &out, &errOut)
	if stateExists(filepath.Join(dir, ".agent-sdlc", "archive", "plan", "lifecycle.json")) {
		t.Error("must not historicalize across a pending approval gate")
	}
	if !strings.Contains(out.String(), "skipped") {
		t.Errorf("stdout = %q, want a skip", out.String())
	}
	tasks, err := st.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("the plan's task must remain active when the final stage is skipped: %d", len(tasks))
	}
}
