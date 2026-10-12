//go:build acceptance

// Operator-authored acceptance check for ACC-009: a `go test` acceptance check is
// evaluated from structured `go test -json` events, executes exactly once, requires
// at least one passing test, rejects all-skipped and zero-test results, and ignores
// unrelated packages that have no test files.

package cli

import (
	"path/filepath"
	"testing"

	"github.com/imhttran/agentic-sop/internal/outcome"
)

const accOp009Pass = `package pass

import "testing"

func TestPass(t *testing.T) {}
`

func accOp009Module(t *testing.T, counter string) string {
	t.Helper()
	return accOpModule(t, map[string]string{
		"pass/pass.go":          "package pass\n\n// V is a value.\nconst V = 1\n",
		"pass/pass_test.go":     accOp009Pass,
		"skip/skip.go":          "package skip\n\n// V is a value.\nconst V = 1\n",
		"skip/skip_test.go":     "package skip\n\nimport \"testing\"\n\nfunc TestSkipA(t *testing.T) { t.Skip(\"a\") }\n\nfunc TestSkipB(t *testing.T) { t.Skip(\"b\") }\n",
		"notests/notests.go":    "package notests\n\n// V is a value.\nconst V = 1\n",
		"tmain/tmain.go":        "package tmain\n\n// V is a value.\nconst V = 1\n",
		"tmain/tmain_test.go":   "package tmain\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\nfunc TestMain(m *testing.M) { os.Exit(0) }\n\nfunc TestNeverRuns(t *testing.T) { t.Fatal(\"must not run\") }\n",
		"broken/broken.go":      "package broken\n\nfunc Broken() int { return \"not an int\" }\n",
		"broken/broken_test.go": "package broken\n\nimport \"testing\"\n\nfunc TestBroken(t *testing.T) {}\n",
		"fail/fail.go":          "package fail\n\n// V is a value.\nconst V = 1\n",
		"fail/fail_test.go":     "package fail\n\nimport \"testing\"\n\nfunc TestFail(t *testing.T) { t.Fatal(\"fails\") }\n",
		"counter/counter.go":    "package counter\n\n// V is a value.\nconst V = 1\n",
		"counter/counter_test.go": "package counter\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\n" +
			"func TestCount(t *testing.T) {\n\tf, err := os.OpenFile(" + "`" + counter + "`" +
			", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)\n\tif err != nil {\n\t\tt.Fatal(err)\n\t}\n" +
			"\tdefer f.Close()\n\tif _, err := f.WriteString(\"run\\n\"); err != nil {\n\t\tt.Fatal(err)\n\t}\n}\n",
	})
}

func TestOperatorAcceptanceACC009NotVerifiedWithoutAPassingTest(t *testing.T) {
	dir := accOp009Module(t, filepath.Join(t.TempDir(), "count.txt"))
	for _, check := range []string{
		"go test -count=1 ./skip/",                 // every test skipped
		"go test -count=1 -run NoSuchTest ./pass/", // -run matches nothing
		"go test -count=1 ./notests/",              // no test files at all
		"go test -count=1 ./tmain/",                // TestMain exits without running tests
		"go test -count=1 ./fail/",                 // a failing test
		"go test -count=1 ./skip/ ./notests/",
		"go test -count=1 ./pass/ ./fail/",   // a pass event but a non-zero exit
		"go test -count=1 ./pass/ ./broken/", // a pass event but a package fails to build      // only skips and no-test packages
	} {
		t.Run(check, func(t *testing.T) {
			res := accOpOutcome(t, dir, check)
			if res.Status == outcome.Verified {
				t.Fatalf("status = VERIFIED for %q, want not VERIFIED; criteria=%+v", check, res.Criteria)
			}
			if c := accOpCheck(t, res, check); c.Met {
				t.Fatalf("check %q reported met; evidence=%q", check, c.Evidence)
			}
		})
	}
}

func TestOperatorAcceptanceACC009VerifiedWhenRealTestsPass(t *testing.T) {
	dir := accOp009Module(t, filepath.Join(t.TempDir(), "count.txt"))
	for _, check := range []string{
		"go test -count=1 ./pass/",
		"go test -count=1 ./pass/ ./notests/",    // unrelated no-test package is ignored
		"go test -json -count=1 ./pass/",         // operator already asked for -json
		"go test -count=1 -run TestPass ./pass/", // -run selecting a real test
	} {
		t.Run(check, func(t *testing.T) {
			res := accOpOutcome(t, dir, check)
			if res.Status != outcome.Verified {
				t.Fatalf("status = %s for %q, want VERIFIED; criteria=%+v", res.Status, check, res.Criteria)
			}
		})
	}
}

func TestOperatorAcceptanceACC009MixedModuleWithNoTestPackageVerified(t *testing.T) {
	// A whole-module check where some packages have no test files and the rest pass.
	dir := accOpModule(t, map[string]string{
		"pass/pass.go":       "package pass\n\n// V is a value.\nconst V = 1\n",
		"pass/pass_test.go":  accOp009Pass,
		"notests/notests.go": "package notests\n\n// V is a value.\nconst V = 1\n",
		"cmd/tool/main.go":   "package main\n\nfunc main() {}\n",
	})
	res := accOpOutcome(t, dir, "go test -count=1 ./...")
	if res.Status != outcome.Verified {
		t.Fatalf("status = %s for go test ./... over passing + no-test packages, want VERIFIED; criteria=%+v", res.Status, res.Criteria)
	}
}

func TestOperatorAcceptanceACC009ExecutesEachCommandOnce(t *testing.T) {
	counter := filepath.Join(t.TempDir(), "count.txt")
	dir := accOp009Module(t, counter)
	check := "go test -count=1 -run TestCount ./counter/"
	res := accOpOutcome(t, dir, check)
	if res.Status != outcome.Verified {
		t.Fatalf("status = %s, want VERIFIED; criteria=%+v", res.Status, res.Criteria)
	}
	if n := accOpCount(counter); n != 1 {
		t.Fatalf("acceptance command executed %d times, want exactly 1", n)
	}
}
