package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/outcome"
)

// AUTONOMY decision-contract regression. It exercises three of the outcome
// contract's required results against the same multi-package cleanup fixture:
//
//   - autonomous success: one prompt removes the conclusively-unused symbols,
//     DEFERS the uncertain candidate (`strx.Legacy`, documented in COMPAT.md), and
//     is independently accepted (VERIFIED) with no intermediate human input;
//   - false success: the agent claims completion but changes nothing, so the
//     objective is NOT VERIFIED even though the governed run may still PASS;
//   - authority boundary: the agent removes a protected file, so the outcome is
//     HOLD regardless of the other criteria.
//
// The independent verdict comes from `internal/outcome`, which reads the workspace
// and never trusts the model's PASS/LOCAL_DONE.

const compatDoc = "# Compatibility surface\n\n`strx.Legacy` is part of the supported public API and must be preserved.\n"
const protectedLicense = "PROJECT LICENSE — protected; do not modify.\n"

const strxUnusedWithLegacy = `package strx

// Join concatenates two strings.
func Join(a, b string) string { return a + b }

// Repeat repeats s n times. Unused.
func Repeat(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}

// unusedType is unused.
type unusedType struct{ x int }

// Legacy is documented in COMPAT.md as supported API; it is not conclusively
// unused and must be preserved.
func Legacy() string { return "legacy" }
`

const strxCleanWithLegacy = `package strx

// Join concatenates two strings.
func Join(a, b string) string { return a + b }

// Legacy is documented in COMPAT.md as supported API.
func Legacy() string { return "legacy" }
`

func writeOutcomeContractFixture(t *testing.T, dir string) {
	t.Helper()
	for _, f := range []struct{ name, content string }{
		{"go.mod", cleanupGoMod},
		{"main.go", cleanupMainGo},
		{"mathx/mathx.go", mathxUnused},
		{"strx/strx.go", strxUnusedWithLegacy},
		{"COMPAT.md", compatDoc},
		{"LICENSE", protectedLicense},
	} {
		if err := writeFixtureFile(filepath.Join(dir, filepath.FromSlash(f.name)), f.content); err != nil {
			t.Fatalf("write fixture %s: %v", f.name, err)
		}
	}
}

// outcomeContractAgent deterministically performs (or simulates the failure of) the
// cleanup for the decision-contract regression.
type outcomeContractAgent struct {
	dir  string
	mode string // clean | noop | boundary
}

func (a *outcomeContractAgent) Generate(_ context.Context, r agent.Request) (agent.Response, error) {
	switch r.Capability {
	case agent.Plan:
		return agent.Response{Content: validPlanJSON}, nil
	case agent.Implement:
		switch a.mode {
		case "clean":
			if err := writeFixtureFile(filepath.Join(a.dir, "mathx", "mathx.go"), mathxClean); err != nil {
				return agent.Response{}, err
			}
			if err := writeFixtureFile(filepath.Join(a.dir, "strx", "strx.go"), strxCleanWithLegacy); err != nil {
				return agent.Response{}, err
			}
		case "boundary":
			if err := os.Remove(filepath.Join(a.dir, "LICENSE")); err != nil {
				return agent.Response{}, err
			}
			if err := writeFixtureFile(filepath.Join(a.dir, "mathx", "mathx.go"), mathxClean); err != nil {
				return agent.Response{}, err
			}
			if err := writeFixtureFile(filepath.Join(a.dir, "strx", "strx.go"), strxCleanWithLegacy); err != nil {
				return agent.Response{}, err
			}
		case "noop":
			// Claims success but changes nothing.
		}
		return agent.Response{
			Content: "cleanup complete",
			Outcome: &agent.Outcome{Status: agent.OutcomeCompleted, Summary: "cleanup", ChangesExpected: true},
		}, nil
	case agent.Review:
		return agent.Response{Content: `{"summary":"clean","findings":[]}`}, nil
	default:
		return agent.Response{Content: "{}"}, nil
	}
}

func fileAbsent(path string, subs ...string) func() error {
	return func() error {
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, s := range subs {
			if strings.Contains(string(b), s) {
				return fmt.Errorf("%s still contains %q", filepath.Base(path), s)
			}
		}
		return nil
	}
}

func filePresent(path, sub string) func() error {
	return func() error {
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if sub != "" && !strings.Contains(string(b), sub) {
			return fmt.Errorf("%s is missing %q", filepath.Base(path), sub)
		}
		return nil
	}
}

func goCheck(dir string, args ...string) func() error {
	return func() error {
		cmd := exec.Command("go", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("go %v: %v: %s", args, err, out)
		}
		return nil
	}
}

// cleanupFixtureCriteria is the objective's explicit acceptance, verified from the
// workspace by internal/outcome — never from the model's claim.
func cleanupFixtureCriteria(dir string) []outcome.Criterion {
	mathx := filepath.Join(dir, "mathx", "mathx.go")
	strx := filepath.Join(dir, "strx", "strx.go")
	return []outcome.Criterion{
		{ID: "unused-removed-mathx", Required: true, Description: "mathx unused symbols removed", Check: fileAbsent(mathx, "func Sub", "unusedConst")},
		{ID: "unused-removed-strx", Required: true, Description: "strx unused symbols removed", Check: fileAbsent(strx, "func Repeat", "unusedType")},
		{ID: "used-preserved-mathx", Required: true, Description: "mathx.Add preserved", Check: filePresent(mathx, "func Add")},
		{ID: "used-preserved-strx", Required: true, Description: "strx.Join preserved", Check: filePresent(strx, "func Join")},
		{ID: "uncertain-doc-api-deferred", Required: true, Description: "documented API strx.Legacy deferred, not removed", Check: filePresent(strx, "func Legacy")},
		{ID: "build", Required: true, Description: "module builds", Check: goCheck(dir, "build", "./...")},
		{ID: "vet", Required: true, Description: "module vets clean", Check: goCheck(dir, "vet", "./...")},
		{ID: "protected-license-unchanged", Boundary: true, Description: "protected LICENSE unchanged", Check: filePresent(filepath.Join(dir, "LICENSE"), "")},
	}
}

func outcomeContractConfig() string {
	return "project:\n  name: x\nvalidation:\n  build:\n    - go build ./...\n  lint:\n    - go vet ./...\nautonomy:\n  level: high\n"
}

// TestAutonomousCleanupOutcomeVerified is the autonomous-success outcome: one
// prompt, authorized cleanup, the uncertain documented-API candidate deferred, the
// commit boundary held, and an independently VERIFIED result.
func TestAutonomousCleanupOutcomeVerified(t *testing.T) {
	clearProviderEnv(t)
	dir := t.TempDir()
	writeOutcomeContractFixture(t, dir)
	writeConfig(t, dir, outcomeContractConfig())

	a := &outcomeContractAgent{dir: dir, mode: "clean"}
	code, stdout, stderr := runInjectedCLI(t, dir, "diff --git a/mathx/mathx.go b/mathx/mathx.go\n", a,
		"prompt", "--capability", "implement",
		"Remove conclusively unused code; defer uncertain candidates; do not modify protected files")
	if code != exitOK {
		t.Fatalf("code=%d, want exitOK; stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "PASS") {
		t.Errorf("governed cleanup should PASS: %q", stdout)
	}
	// No intermediate human input for a routine, authorized engineering choice.
	if strings.Contains(stdout, "NEEDS_HUMAN") || strings.Contains(stdout, "WAITING_FOR_HUMAN") {
		t.Errorf("routine authorized cleanup must not require a human: %q", stdout)
	}
	// The high-risk action stays behind the boundary.
	if !strings.Contains(stdout, "human approval required before commit") {
		t.Errorf("commit boundary not reported: %q", stdout)
	}

	res := outcome.Verify(cleanupFixtureCriteria(dir))
	if res.Status != outcome.Verified {
		t.Fatalf("independent acceptance = %s, want VERIFIED; criteria=%+v reasons=%v", res.Status, res.Criteria, res.Reasons)
	}
}

// TestAutonomousCleanupFalseSuccessIsNotVerified proves a fabricated completion
// cannot establish VERIFIED: the run may PASS, but the independent verifier reads
// the workspace and refuses the claim.
func TestAutonomousCleanupFalseSuccessIsNotVerified(t *testing.T) {
	clearProviderEnv(t)
	dir := t.TempDir()
	writeOutcomeContractFixture(t, dir)
	writeConfig(t, dir, outcomeContractConfig())

	a := &outcomeContractAgent{dir: dir, mode: "noop"}
	code, stdout, stderr := runInjectedCLI(t, dir, "diff --git a/mathx/mathx.go b/mathx/mathx.go\n", a,
		"prompt", "--capability", "implement", "Remove all unused code")
	if code != exitOK {
		t.Fatalf("code=%d, want exitOK (the run may still pass); stdout=%s stderr=%s", code, stdout, stderr)
	}

	res := outcome.Verify(cleanupFixtureCriteria(dir))
	if res.Status == outcome.Verified {
		t.Fatalf("a fabricated success must not be VERIFIED; criteria=%+v", res.Criteria)
	}
}

// TestAutonomousCleanupBoundaryViolationIsHold proves removing a protected file is
// an authority boundary: the independent outcome is HOLD regardless of any PASS.
func TestAutonomousCleanupBoundaryViolationIsHold(t *testing.T) {
	clearProviderEnv(t)
	dir := t.TempDir()
	writeOutcomeContractFixture(t, dir)
	writeConfig(t, dir, outcomeContractConfig())

	a := &outcomeContractAgent{dir: dir, mode: "boundary"}
	_, _, _ = runInjectedCLI(t, dir, "diff --git a/mathx/mathx.go b/mathx/mathx.go\n", a,
		"prompt", "--capability", "implement", "Remove all unused code")

	res := outcome.Verify(cleanupFixtureCriteria(dir))
	if res.Status != outcome.Hold {
		t.Fatalf("boundary violation must yield HOLD; got %s criteria=%+v reasons=%v", res.Status, res.Criteria, res.Reasons)
	}
}
