//go:build acceptance

// Operator-authored acceptance check for ACC-008: acceptance-command hardening (a
// rejected command is a HOLD boundary and is never executed), per-check outcome
// evidence (Argv, ExitCode, Output), and no VERIFIED from deliverables alone.

package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/imhttran/agentic-sop/internal/outcome"
	"github.com/imhttran/agentic-sop/internal/taskfile"
)

func accOp008Module(t *testing.T) string {
	t.Helper()
	return accOpModule(t, map[string]string{
		"pass/pass.go":      "package pass\n\n// V is a value.\nconst V = 1\n",
		"pass/pass_test.go": "package pass\n\nimport \"testing\"\n\nfunc TestPass(t *testing.T) {}\n",
		"noisy/noisy.go":    "package noisy\n\n// V is a value.\nconst V = 1\n",
		"noisy/noisy_test.go": "package noisy\n\nimport (\n\t\"strings\"\n\t\"testing\"\n)\n\n" +
			"func TestNoisy(t *testing.T) { t.Fatal(strings.Repeat(\"x\", 2048)) }\n",
		"cmd/tool/main.go": "package main\n\nfunc main() {}\n",
	})
}

// accOp008Forms renders a flag/value pair in the -f v, -f=v, --f v and --f=v forms.
func accOp008Forms(flag, value string) []string {
	return []string{
		"-" + flag + " " + value,
		"-" + flag + "=" + value,
		"--" + flag + " " + value,
		"--" + flag + "=" + value,
	}
}

func accOp008AssertRejected(t *testing.T, dir, check string) {
	t.Helper()
	res := accOpOutcome(t, dir, check)
	if res.Status != outcome.Hold {
		t.Fatalf("status = %s for %q, want HOLD; criteria=%+v", res.Status, check, res.Criteria)
	}
	c := accOpCheck(t, res, check)
	if !c.Boundary || c.Met {
		t.Fatalf("check %q: Boundary=%v Met=%v, want an unmet boundary", check, c.Boundary, c.Met)
	}
}

func TestOperatorAcceptanceACC008RejectedCommandsHoldAndNeverRun(t *testing.T) {
	// Each flag is tried in all four forms. A script flag's value is an executable
	// that touches the marker when run; -o's value is the marker path itself.
	cases := []struct {
		flag, prefix, suffix string
		script               bool
	}{
		{"exec", "go test -count=1 ", " ./pass/", true},
		{"toolexec", "go test -count=1 ", " ./pass/", true},
		{"vettool", "go vet ", " ./pass/", true},
		{"o", "go build ", " ./cmd/tool/", false},
	}
	for _, tc := range cases {
		for i := 0; i < 4; i++ {
			tc, i := tc, i
			t.Run(tc.flag+"/form"+string(rune('0'+i)), func(t *testing.T) {
				dir := accOp008Module(t)
				tmp := t.TempDir()
				marker := filepath.Join(tmp, "marker")
				value := marker
				if tc.script {
					value = accOpScript(t, tmp, tc.flag+".sh", marker)
				}
				check := tc.prefix + accOp008Forms(tc.flag, value)[i] + tc.suffix
				accOp008AssertRejected(t, dir, check)
				if accOpExists(marker) {
					t.Fatalf("rejected command %q was executed (marker exists)", check)
				}
			})
		}
	}

	// A relative output path is rejected too, so rejecting absolute paths is not enough.
	t.Run("o/relative", func(t *testing.T) {
		dir := accOp008Module(t)
		check := "go build -o tool.bin ./cmd/tool/"
		accOp008AssertRejected(t, dir, check)
		if accOpExists(filepath.Join(dir, "tool.bin")) {
			t.Fatalf("rejected command %q wrote tool.bin", check)
		}
	})

	// -overlay and -modfile in every form.
	for _, flag := range []string{"overlay", "modfile"} {
		for i, form := range accOp008Forms(flag, map[string]string{"overlay": "x.json", "modfile": "alt.mod"}[flag]) {
			flag, form := flag, form
			t.Run(flag+"/form"+string(rune('0'+i)), func(t *testing.T) {
				accOp008AssertRejected(t, accOp008Module(t), "go build "+form+" ./...")
			})
		}
	}
}

func TestOperatorAcceptanceACC008GoFmtHoldsAndDoesNotRewrite(t *testing.T) {
	unformatted := "package pass\n\nconst   W   =   2\n"
	dir := accOp008Module(t)
	accOpWrite(t, dir, "pass/ugly.go", unformatted)
	accOp008AssertRejected(t, dir, "go fmt ./...")
	data, err := os.ReadFile(filepath.Join(dir, "pass/ugly.go"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != unformatted {
		t.Fatalf("go fmt acceptance check rewrote a file:\n%s", data)
	}
}

func TestOperatorAcceptanceACC008GitConfigInjectionHolds(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := accOp008Module(t)
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	for _, check := range []string{
		"git -c core.pager=cat status",
		"git -ccore.pager=cat status",
		"git --config-env=core.pager=PAGER status",
		"git --config-env core.pager=PAGER status",
	} {
		accOp008AssertRejected(t, dir, check)
	}
	// Control: a plain read-only git check still runs and verifies.
	if res := accOpOutcome(t, dir, "git status"); res.Status != outcome.Verified {
		t.Fatalf("plain `git status` = %s, want VERIFIED; criteria=%+v", res.Status, res.Criteria)
	}
}

func TestOperatorAcceptanceACC008AllowedChecksStillRun(t *testing.T) {
	// Control: hardening must not reject ordinary SAFE checks.
	dir := accOp008Module(t)
	for _, check := range []string{"go vet ./pass/", "go build ./...", "go test -count=1 ./pass/"} {
		res := accOpOutcome(t, dir, check)
		if res.Status != outcome.Verified {
			t.Fatalf("status = %s for allowed %q, want VERIFIED; criteria=%+v", res.Status, check, res.Criteria)
		}
	}
}

// accOp008Field reads a named field of a criterion result; it compiles before the
// field exists and fails the test when it is absent.
func accOp008Field(t *testing.T, c outcome.CriterionResult, name string) reflect.Value {
	t.Helper()
	v := reflect.ValueOf(c).FieldByName(name)
	if !v.IsValid() {
		t.Fatalf("outcome.CriterionResult has no %s evidence field", name)
	}
	return v
}

func TestOperatorAcceptanceACC008EvidenceRecorded(t *testing.T) {
	dir := accOp008Module(t)
	check := "go vet ./doesnotexist/"
	c := accOpCheck(t, accOpOutcome(t, dir, check), check)
	if c.Met {
		t.Fatalf("failing check reported met")
	}
	argv := accOp008Field(t, c, "Argv")
	if argv.Kind() != reflect.Slice || argv.Len() < 3 ||
		argv.Index(0).String() != "go" || argv.Index(1).String() != "vet" ||
		argv.Index(argv.Len()-1).String() != "./doesnotexist/" {
		t.Fatalf("Argv evidence = %v, want the executed argv [go vet ./doesnotexist/]", argv)
	}
	if code := accOp008Field(t, c, "ExitCode"); code.Int() == 0 {
		t.Fatalf("ExitCode evidence = %d, want non-zero", code.Int())
	}
	if out := accOp008Field(t, c, "Output").String(); out == "" || len(out) > 400 {
		t.Fatalf("Output evidence len %d, want a non-empty excerpt of at most 400 bytes", len(out))
	}

	// A failing check with ~2 KB of output is truncated to at most 400 bytes.
	noisy := "go test -count=1 ./noisy/"
	nc := accOpCheck(t, accOpOutcome(t, dir, noisy), noisy)
	if nc.Met {
		t.Fatalf("noisy failing check reported met")
	}
	if out := accOp008Field(t, nc, "Output").String(); out == "" || len(out) > 400 {
		t.Fatalf("noisy Output evidence len %d, want 1..400 (truncated)", len(out))
	}
}

func TestOperatorAcceptanceACC008DeliverablesAloneNeverVerified(t *testing.T) {
	dir := t.TempDir()
	accOpWrite(t, dir, "out.md", "report\n")
	only := &taskfile.Spec{ID: "D", Title: "deliverable only", Deliverables: []string{"out.md"}}
	if res := deriveOutcome(only, dir, accOpPassGate()); res == nil || res.Status == outcome.Verified {
		t.Fatalf("deliverable-only spec with a passing gate = %+v, want not VERIFIED", res)
	}
	withCriteria := &taskfile.Spec{ID: "D", Title: "deliverable + criteria", Deliverables: []string{"out.md"}, AcceptanceCriteria: []string{"it is written"}}
	if res := deriveOutcome(withCriteria, dir, accOpPassGate()); res == nil || res.Status == outcome.Verified {
		t.Fatalf("deliverable + free-text spec with a passing gate = %+v, want not VERIFIED", res)
	}
	// Control: an executed passing acceptance check alongside the deliverable verifies.
	mod := accOp008Module(t)
	accOpWrite(t, mod, "out.md", "report\n")
	withCheck := &taskfile.Spec{ID: "D", Title: "deliverable + check", Deliverables: []string{"out.md"}, AcceptanceChecks: []string{"go vet ./pass/"}}
	if res := deriveOutcome(withCheck, mod, accOpPassGate()); res == nil || res.Status != outcome.Verified {
		t.Fatalf("deliverable + passing check = %+v, want VERIFIED", res)
	}
}
