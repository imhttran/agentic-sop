package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Regression tests for the acceptance-command authorization/execution contract:
// authorization validates the exact argument vector that executes, shell syntax is
// rejected before execution, and a rejected command is a HOLD boundary.

func securityAcceptDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	goModuleFixture(t, dir)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
	return dir
}

func promptOutcomeRaw(t *testing.T, dir string) string {
	t.Helper()
	base := filepath.Join(dir, stateDirName, "runs", "prompts")
	entries, err := os.ReadDir(base)
	if err != nil {
		t.Fatalf("read prompts dir: %v", err)
	}
	found := ""
	for _, e := range entries {
		if e.IsDir() {
			found = e.Name()
		}
	}
	if found == "" {
		t.Fatal("no prompt run dir")
	}
	b, err := os.ReadFile(filepath.Join(base, found, "outcome.json"))
	if err != nil {
		t.Fatalf("read outcome.json: %v", err)
	}
	return string(b)
}

// TestAcceptanceApprovedGoTestExecutes: an approved `go test` command runs (without
// a shell) and yields VERIFIED.
func TestAcceptanceApprovedGoTestExecutes(t *testing.T) {
	dir := securityAcceptDir(t)
	code, _, stderr := runInjectedCLI(t, dir, "diff --git a/t.go b/t.go\n", acceptFixtureAgent(),
		"prompt", "--capability", "implement", "--accept", "go test ./...", "implement the change")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if got := promptOutcomeStatus(t, dir); got != "VERIFIED" {
		t.Errorf("outcome = %q, want VERIFIED", got)
	}
}

// TestAcceptanceUnauthorizedExecutableRejected: a program the policy does not
// classify SAFE is a HOLD boundary with evidence.
func TestAcceptanceUnauthorizedExecutableRejected(t *testing.T) {
	dir := securityAcceptDir(t)
	_, _, _ = runInjectedCLI(t, dir, "diff --git a/t.go b/t.go\n", acceptFixtureAgent(),
		"prompt", "--capability", "implement", "--accept", "curl http://example.com", "x")
	if got := promptOutcomeStatus(t, dir); got != "HOLD" {
		t.Errorf("outcome = %q, want HOLD (unauthorized executable)", got)
	}
	raw := promptOutcomeRaw(t, dir)
	if !strings.Contains(raw, "not authorized") || !strings.Contains(raw, `"Boundary": true`) {
		t.Errorf("outcome evidence missing the boundary rationale:\n%s", raw)
	}
}

// TestAcceptanceShellChainingRejectedNoSideEffect: command chaining is rejected
// before execution, so the chained command never runs.
func TestAcceptanceShellChainingRejectedNoSideEffect(t *testing.T) {
	dir := securityAcceptDir(t)
	_, _, _ = runInjectedCLI(t, dir, "diff --git a/t.go b/t.go\n", acceptFixtureAgent(),
		"prompt", "--capability", "implement", "--accept", "go build ./... ; touch sideeffect.txt", "x")
	if got := promptOutcomeStatus(t, dir); got != "HOLD" {
		t.Errorf("outcome = %q, want HOLD (shell chaining)", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "sideeffect.txt")); !os.IsNotExist(err) {
		t.Errorf("command chaining produced a side effect: %v", err)
	}
}

// TestAcceptanceSubstitutionAndRedirectionRejected: command substitution and
// redirection are rejected before execution.
func TestAcceptanceSubstitutionAndRedirectionRejected(t *testing.T) {
	for _, accept := range []string{
		"go test $(pwd)",
		"go test `pwd`",
		"go build ./... > out.txt",
		"go build ./... < in.txt",
	} {
		dir := securityAcceptDir(t)
		_, _, _ = runInjectedCLI(t, dir, "diff --git a/t.go b/t.go\n", acceptFixtureAgent(),
			"prompt", "--capability", "implement", "--accept", accept, "x")
		if got := promptOutcomeStatus(t, dir); got != "HOLD" {
			t.Errorf("%q: outcome = %q, want HOLD", accept, got)
		}
		if _, err := os.Stat(filepath.Join(dir, "out.txt")); !os.IsNotExist(err) {
			t.Errorf("%q: redirection produced a side effect", accept)
		}
	}
}

// TestAcceptanceMalformedCommandFailsClosed: a malformed command and an empty
// command both fail closed.
func TestAcceptanceMalformedCommandFailsClosed(t *testing.T) {
	dir := securityAcceptDir(t)
	_, _, _ = runInjectedCLI(t, dir, "diff --git a/t.go b/t.go\n", acceptFixtureAgent(),
		"prompt", "--capability", "implement", "--accept", "go build 'unterminated", "x")
	if got := promptOutcomeStatus(t, dir); got != "HOLD" {
		t.Errorf("outcome = %q, want HOLD (malformed command)", got)
	}

	dir2 := securityAcceptDir(t)
	if code, _, _ := runInjectedCLI(t, dir2, "diff --git a/t.go b/t.go\n", acceptFixtureAgent(),
		"prompt", "--capability", "implement", "--accept", "", "x"); code == exitOK {
		t.Error("empty --accept must fail closed")
	}
}
