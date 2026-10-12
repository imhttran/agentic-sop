// Unit tests for structured `go test -json` acceptance-check evaluation. These carry
// no build tag, so ordinary validation (`go test ./...`) exercises them. They cover
// the pure parser/decision seam (goTestJSONMet / ensureGoTestJSON), plus the
// runAcceptanceCheck wiring against a real fixture module.

package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestGoTestJSONMetDecision(t *testing.T) {
	passEvent := `{"Time":"2020-01-01T00:00:00Z","Action":"run","Package":"p","Test":"TestX"}
{"Time":"2020-01-01T00:00:00Z","Action":"pass","Package":"p","Test":"TestX","Elapsed":0.01}`
	tmainOnly := `{"Time":"2020-01-01T00:00:00Z","Action":"pass","Package":"p","Elapsed":0.00}`
	skipOnly := `{"Time":"2020-01-01T00:00:00Z","Action":"run","Package":"p","Test":"TestSkip"}
{"Time":"2020-01-01T00:00:00Z","Action":"skip","Package":"p","Test":"TestSkip"}`
	runNoPass := `{"Time":"2020-01-01T00:00:00Z","Action":"run","Package":"p","Test":"TestNomatch"}
{"Time":"2020-01-01T00:00:00Z","Action":"output","Package":"p","Test":"TestNomatch","Output":"=== RUN\n"}`
	zeroTest := `{"Time":"2020-01-01T00:00:00Z","Action":"start","Package":"p"}
{"Time":"2020-01-01T00:00:00Z","Action":"pass","Package":"p","Elapsed":0.00}`
	mixedPassFail := passEvent + "\n" + `{"Time":"2020-01-01T00:00:00Z","Action":"fail","Package":"q","Test":"TestY"}`
	withTrailingText := passEvent + "\nok  \texample.com/p\t0.01s"

	cases := []struct {
		name     string
		exitCode int
		output   string
		wantMet  bool
	}{
		{"passing test event with exit 0", 0, passEvent, true},
		{"pass event plus trailing non-JSON text", 0, withTrailingText, true},
		{"non-zero exit with a pass event", 1, mixedPassFail, false},
		{"all skipped", 0, skipOnly, false},
		{"run events with no pass", 0, runNoPass, false},
		{"zero test-level events", 0, zeroTest, false},
		{"TestMain pass with no test-level pass", 0, tmainOnly, false},
		{"non-JSON text only", 0, "ok\texample.com/p\t0.01s", false},
		{"empty output", 0, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			met, _ := goTestJSONMet(tc.exitCode, tc.output)
			if met != tc.wantMet {
				t.Fatalf("goTestJSONMet(%d, %q) = %v, want %v", tc.exitCode, tc.output, met, tc.wantMet)
			}
		})
	}
}

func TestEnsureGoTestJSON(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{
			"adds -json when absent",
			[]string{"go", "test", "-count=1", "./pass/"},
			[]string{"go", "test", "-json", "-count=1", "./pass/"},
		},
		{
			"does not duplicate operator -json",
			[]string{"go", "test", "-json", "-count=1", "./pass/"},
			[]string{"go", "test", "-json", "-count=1", "./pass/"},
		},
		{
			"does not duplicate --json",
			[]string{"go", "test", "--json", "./pass/"},
			[]string{"go", "test", "--json", "./pass/"},
		},
		{
			"non-go-test unchanged",
			[]string{"go", "vet", "./pass/"},
			[]string{"go", "vet", "./pass/"},
		},
		{
			"preserves -count",
			[]string{"go", "test", "-count=1", "./..."},
			[]string{"go", "test", "-json", "-count=1", "./..."},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ensureGoTestJSON(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("ensureGoTestJSON(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// TestRunAcceptanceCheckGoTestJSON exercises the wiring against a real fixture
// module: a `go test` check is executed once with -json added, and judged from its
// events (a passing test is met; an all-skipped package is not).
func TestRunAcceptanceCheckGoTestJSON(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "go.mod", "module example.com/acc009\n\ngo 1.22\n")
	writeFixture(t, dir, "pass/pass.go", "package pass\n\n// V is a value.\nconst V = 1\n")
	writeFixture(t, dir, "pass/pass_test.go", "package pass\n\nimport \"testing\"\n\nfunc TestPass(t *testing.T) {}\n")
	writeFixture(t, dir, "skip/skip.go", "package skip\n\n// V is a value.\nconst V = 1\n")
	writeFixture(t, dir, "skip/skip_test.go", "package skip\n\nimport \"testing\"\n\nfunc TestSkip(t *testing.T) { t.Skip(\"skip\") }\n")

	// A passing check is met, and -json was added to the executed argv exactly once.
	err, run := runAcceptanceCheck(dir, []string{"go", "test", "-count=1", "./pass/"})
	if err != nil {
		t.Fatalf("passing go test check = %v, want nil", err)
	}
	if n := countJSONFlag(run.argv); n != 1 {
		t.Fatalf("executed argv %v contains %d -json flags, want exactly 1", run.argv, n)
	}

	// Operator-supplied -json is not duplicated.
	_, run2 := runAcceptanceCheck(dir, []string{"go", "test", "-json", "-count=1", "./pass/"})
	if n := countJSONFlag(run2.argv); n != 1 {
		t.Fatalf("executed argv %v contains %d -json flags, want exactly 1", run2.argv, n)
	}

	// An all-skipped package exits 0 but is not met.
	if err, _ := runAcceptanceCheck(dir, []string{"go", "test", "-count=1", "./skip/"}); err == nil {
		t.Fatalf("all-skipped go test check = nil, want a no test executed error")
	}
	// A -run filter matching nothing is not met.
	if err, _ := runAcceptanceCheck(dir, []string{"go", "test", "-count=1", "-run", "NoSuchTest", "./pass/"}); err == nil {
		t.Fatalf("unmatched -run go test check = nil, want a no test executed error")
	}
}

func countJSONFlag(argv []string) int {
	n := 0
	for _, a := range argv {
		if a == "-json" {
			n++
		}
	}
	return n
}

// writeFixture writes content to dir/rel, creating parent directories.
func writeFixture(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
