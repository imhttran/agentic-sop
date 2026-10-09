package cli

import "testing"

// goModuleFixture makes a minimal buildable Go module so a `go ...` acceptance
// check can run deterministically in the test workspace.
func goModuleFixture(t *testing.T, dir string) {
	t.Helper()
	writeFile(t, dir, "go.mod", "module example.com/t\n\ngo 1.27\n")
	writeFile(t, dir, "t.go", "package t\n\nfunc T() {}\n")
}

func acceptFixtureAgent() *fakeCapabilityAgent {
	return &fakeCapabilityAgent{plan: validPlanJSON, impl: "changed", review: `{"summary":"clean","findings":[]}`}
}

// TestPromptAcceptanceCheckVerified proves an authorized check that passes yields
// an evidence-backed VERIFIED outcome — independent of the model's claim.
func TestPromptAcceptanceCheckVerified(t *testing.T) {
	dir := t.TempDir()
	goModuleFixture(t, dir)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
	code, _, stderr := runInjectedCLI(t, dir, "diff --git a/t.go b/t.go\n", acceptFixtureAgent(),
		"prompt", "--capability", "implement", "--accept", "go build ./...", "implement the change")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if got := promptOutcomeStatus(t, dir); got != "VERIFIED" {
		t.Errorf("outcome = %q, want VERIFIED", got)
	}
}

// TestPromptAcceptanceCheckFailureIsPartial proves a failing acceptance check holds
// the verdict at PARTIAL, never VERIFIED.
func TestPromptAcceptanceCheckFailureIsPartial(t *testing.T) {
	dir := t.TempDir()
	goModuleFixture(t, dir)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
	code, _, stderr := runInjectedCLI(t, dir, "diff --git a/t.go b/t.go\n", acceptFixtureAgent(),
		"prompt", "--capability", "implement", "--accept", "go build ./nonexistent", "implement the change")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if got := promptOutcomeStatus(t, dir); got != "PARTIAL" {
		t.Errorf("outcome = %q, want PARTIAL", got)
	}
}

// TestPromptAcceptanceCheckBoundaryIsHold proves an unauthorized acceptance check
// (a safety/authorization boundary) yields HOLD.
func TestPromptAcceptanceCheckBoundaryIsHold(t *testing.T) {
	dir := t.TempDir()
	goModuleFixture(t, dir)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
	code, _, stderr := runInjectedCLI(t, dir, "diff --git a/t.go b/t.go\n", acceptFixtureAgent(),
		"prompt", "--capability", "implement", "--accept", "git push", "implement the change")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if got := promptOutcomeStatus(t, dir); got != "HOLD" {
		t.Errorf("outcome = %q, want HOLD", got)
	}
}
