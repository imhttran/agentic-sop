package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// This is the deterministic regression for the AUTONOMY-001 scenario: a single
// `sop prompt --capability implement` (one prompt, autonomous) removes unused
// code across a multi-package Go module, the deterministic validation passes,
// and the commit boundary holds. It uses no model, no network, and no clock, so
// it is reproducible under the race detector. The real-model run of the same
// scenario is retained separately as supporting evidence under
// docs/reports/autonomy/autonomy-001-scenario/.

const cleanupGoMod = "module example.com/multicleanup\n\ngo 1.27\n"

const cleanupMainGo = `package main

import (
	"fmt"

	"example.com/multicleanup/mathx"
	"example.com/multicleanup/strx"
)

func main() {
	fmt.Println(mathx.Add(2, 3), strx.Join("a", "b"))
}
`

// mathxUnused retains one used and two unused symbols (an exported function and
// an unexported constant).
const mathxUnused = `package mathx

// Add returns a+b.
func Add(a, b int) int { return a + b }

// Sub subtracts b from a. Unused.
func Sub(a, b int) int { return a - b }

// unusedConst is unused.
const unusedConst = 99
`

const mathxClean = `package mathx

// Add returns a+b.
func Add(a, b int) int { return a + b }
`

// strxUnused retains one used and two unused symbols (an exported function and
// an unexported type).
const strxUnused = `package strx

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
`

const strxClean = `package strx

// Join concatenates two strings.
func Join(a, b string) string { return a + b }
`

// cleanupScenarioAgent deterministically performs the multi-package unused-code
// cleanup: on IMPLEMENT it rewrites the two non-main package files to drop the
// unused symbols and reports a completed, mutating outcome. PLAN and REVIEW are
// canned. It is the deterministic stand-in for the real-model agent.
type cleanupScenarioAgent struct{ dir string }

func (a *cleanupScenarioAgent) Generate(_ context.Context, r agent.Request) (agent.Response, error) {
	switch r.Capability {
	case agent.Plan:
		return agent.Response{Content: validPlanJSON}, nil
	case agent.Implement:
		if err := writeFixtureFile(filepath.Join(a.dir, "mathx", "mathx.go"), mathxClean); err != nil {
			return agent.Response{}, err
		}
		if err := writeFixtureFile(filepath.Join(a.dir, "strx", "strx.go"), strxClean); err != nil {
			return agent.Response{}, err
		}
		return agent.Response{
			Content: "removed unused symbols from mathx and strx",
			Outcome: &agent.Outcome{Status: agent.OutcomeCompleted, Summary: "removed unused code", ChangesExpected: true},
		}, nil
	case agent.Review:
		return agent.Response{Content: `{"summary":"clean","findings":[]}`}, nil
	default:
		return agent.Response{Content: "{}"}, nil
	}
}

func writeFixtureFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func writeMultiPackageFixture(t *testing.T, dir string) {
	t.Helper()
	for _, f := range []struct{ name, content string }{
		{"go.mod", cleanupGoMod},
		{"main.go", cleanupMainGo},
		{"mathx/mathx.go", mathxUnused},
		{"strx/strx.go", strxUnused},
	} {
		if err := writeFixtureFile(filepath.Join(dir, filepath.FromSlash(f.name)), f.content); err != nil {
			t.Fatalf("write fixture %s: %v", f.name, err)
		}
	}
}

// TestAutonomousMultiPackageUnusedCodeCleanup proves the one-prompt autonomous
// cleanup end to end: the governed lifecycle reaches PASS using only existing
// components, unused symbols are removed from BOTH non-main packages while the
// used code survives (the module still builds and vets), and the high-risk
// commit action stays behind the human approval boundary.
func TestAutonomousMultiPackageUnusedCodeCleanup(t *testing.T) {
	clearProviderEnv(t)
	dir := t.TempDir()
	writeMultiPackageFixture(t, dir)
	// Real deterministic validation: the module is actually built and vetted, so
	// a wrong cleanup would fail the gate instead of passing on a stub.
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - go build ./...\n  lint:\n    - go vet ./...\nautonomy:\n  level: high\n")

	diff := "diff --git a/mathx/mathx.go b/mathx/mathx.go\n" +
		"diff --git a/strx/strx.go b/strx/strx.go\n"
	a := &cleanupScenarioAgent{dir: dir}

	code, stdout, stderr := runInjectedCLI(t, dir, diff, a,
		"prompt", "--capability", "implement",
		"Remove all unused code from this multi-package Go module")
	if code != exitOK {
		t.Fatalf("code=%d, want exitOK; stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "PASS") {
		t.Errorf("governed cleanup should pass the gate: %q", stdout)
	}
	// The high-risk action (commit) must remain behind the human boundary.
	if !strings.Contains(stdout, "human approval required before commit") {
		t.Errorf("commit boundary not reported: %q", stdout)
	}

	// Independent re-verification: the unused symbols are gone from both non-main
	// packages, and the used code remains intact.
	mathx, err := os.ReadFile(filepath.Join(dir, "mathx", "mathx.go"))
	if err != nil {
		t.Fatalf("read mathx: %v", err)
	}
	strx, err := os.ReadFile(filepath.Join(dir, "strx", "strx.go"))
	if err != nil {
		t.Fatalf("read strx: %v", err)
	}
	for _, sym := range []string{"func Sub", "unusedConst"} {
		if strings.Contains(string(mathx), sym) {
			t.Errorf("mathx still contains unused %q:\n%s", sym, mathx)
		}
	}
	for _, sym := range []string{"func Repeat", "unusedType"} {
		if strings.Contains(string(strx), sym) {
			t.Errorf("strx still contains unused %q:\n%s", sym, strx)
		}
	}
	if !strings.Contains(string(mathx), "func Add") {
		t.Errorf("mathx lost the used symbol Add:\n%s", mathx)
	}
	if !strings.Contains(string(strx), "func Join") {
		t.Errorf("strx lost the used symbol Join:\n%s", strx)
	}
	if !strings.Contains(cleanupMainGo, "mathx.Add") || !strings.Contains(cleanupMainGo, "strx.Join") {
		t.Fatalf("fixture main.go does not exercise both packages")
	}
}
