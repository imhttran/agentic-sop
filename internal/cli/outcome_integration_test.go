package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These regressions prove the production outcome integration: the lifecycle derives
// an objective-level verdict from the AUTHORIZED TASK SPECIFICATION, records it, and
// never equates PASS/LOCAL_DONE with verified objective completion.

func outcomeStatus(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, stateDirName, "runs", "T001", "outcome.json"))
	if err != nil {
		t.Fatalf("read outcome.json: %v", err)
	}
	var doc struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("unmarshal outcome.json: %v", err)
	}
	return doc.Status
}

func outcomeFixtureAgent() *fakeCapabilityAgent {
	return &fakeCapabilityAgent{plan: validPlanJSON, impl: "changed files", review: `{"summary":"clean","findings":[]}`}
}

// TestOutcomeIntegrationDeliverableAloneIsPartial proves a passing gate plus an
// existing declared deliverable is NOT VERIFIED: VERIFIED requires at least one
// executed, passing acceptance check (ACC-008).
func TestOutcomeIntegrationDeliverableAloneIsPartial(t *testing.T) {
	dir := t.TempDir()
	// The declared deliverable exists and the deterministic checks pass, but no
	// acceptance check was declared/executed => PARTIAL, never VERIFIED.
	writeFile(t, dir, "out.md", "report\n")
	writeFile(t, dir, "TASK.md", "# T001 --- Deliver a report\n\n## Objective\n\nWrite the report.\n\n## Deliverables\n\n- `out.md` — the report.\n")
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")

	code, stdout, stderr := runInjectedCLI(t, dir, "diff --git a/out.md b/out.md\n", outcomeFixtureAgent(), "run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "outcome: PARTIAL") {
		t.Errorf("stdout missing partial outcome: %q", stdout)
	}
	if got := outcomeStatus(t, dir); got != "PARTIAL" {
		t.Errorf("outcome.json status = %q, want PARTIAL", got)
	}
}

// TestOutcomeIntegrationVerifiedWithExecutedCheck proves a deliverable that exists
// alongside an executed, passing acceptance check is VERIFIED.
func TestOutcomeIntegrationVerifiedWithExecutedCheck(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "out.md", "report\n")
	// The acceptance check `go vet ./...` must run against a real module and pass.
	goModuleFixture(t, dir)
	writeFile(t, dir, "TASK.md", "# T001 --- Deliver a report\n\n## Objective\n\nWrite the report.\n\n## Deliverables\n\n- `out.md` — the report.\n\n## Acceptance checks\n\n- go vet ./...\n")
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")

	code, stdout, stderr := runInjectedCLI(t, dir, "diff --git a/out.md b/out.md\n", outcomeFixtureAgent(), "run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "outcome: VERIFIED") {
		t.Errorf("stdout missing verified outcome: %q", stdout)
	}
	if got := outcomeStatus(t, dir); got != "VERIFIED" {
		t.Errorf("outcome.json status = %q, want VERIFIED", got)
	}
}

// TestOutcomeIntegrationMissingDeliverableIsPartial proves a passing run whose
// declared deliverable is absent is not VERIFIED: PASS is not objective completion.
func TestOutcomeIntegrationMissingDeliverableIsPartial(t *testing.T) {
	dir := t.TempDir()
	// Deliverable is declared but NOT created; validation passes, so the gate PASSes.
	writeFile(t, dir, "TASK.md", "# T001 --- Deliver a report\n\n## Objective\n\nWrite the report.\n\n## Deliverables\n\n- `out.md` — the report.\n")
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")

	code, stdout, stderr := runInjectedCLI(t, dir, "diff --git a/out.md b/out.md\n", outcomeFixtureAgent(), "run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "outcome: PARTIAL") {
		t.Errorf("stdout missing partial outcome: %q", stdout)
	}
	if got := outcomeStatus(t, dir); got != "PARTIAL" {
		t.Errorf("outcome.json status = %q, want PARTIAL", got)
	}
}

// TestOutcomeIntegrationFreeTextCriteriaIsPartial proves a free-text acceptance
// criterion (no deterministic determinant on this branch) holds the verdict at
// PARTIAL rather than letting a model PASS imply verified completion.
func TestOutcomeIntegrationFreeTextCriteriaIsPartial(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", "# T001 --- Report\n\n## Objective\n\nWrite it.\n\n## Acceptance Criteria\n\n- the report is accurate\n")
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")

	code, stdout, stderr := runInjectedCLI(t, dir, "diff --git a/x b/x\n", outcomeFixtureAgent(), "run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if got := outcomeStatus(t, dir); got != "PARTIAL" {
		t.Errorf("outcome.json status = %q, want PARTIAL", got)
	}
}
