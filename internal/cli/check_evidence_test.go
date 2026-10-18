package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/quality"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/taskfile"
)

// failingCheckRun derives and persists the outcome for one failing `go test` check
// whose test logs body, returning the run, the failed criterion, and the report.
func failingCheckRun(t *testing.T, body string) (*runpkg.Run, map[string]any, string) {
	t.Helper()
	dir := t.TempDir()
	writeFixture(t, dir, "go.mod", "module example.com/evidence\n\ngo 1.22\n")
	writeFixture(t, dir, "fail/fail.go", "package fail\n")
	writeFixture(t, dir, "fail/fail_test.go", "package fail\n\nimport (\n\t\"strings\"\n\t\"testing\"\n)\n\nvar _ = strings.Repeat\n\nfunc TestFail(t *testing.T) { t.Log("+body+"); t.Fail() }\n")

	spec := &taskfile.Spec{ID: "S001", AcceptanceChecks: []string{"go test -count=1 ./fail/"}}
	res := deriveOutcome(spec, dir, quality.Result{Decision: quality.Pass})
	rn, err := runpkg.New(dir, "S001")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeOutcomeArtifact(rn, res); err != nil {
		t.Fatalf("write outcome artifact: %v", err)
	}
	var b strings.Builder
	writeOutcomeReport(&b, res)

	data, err := os.ReadFile(filepath.Join(rn.Dir(), "outcome.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct{ Criteria []map[string]any }
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	for _, cr := range doc.Criteria {
		if cr["ID"] == "check-1" {
			return rn, cr, b.String()
		}
	}
	t.Fatalf("no check-1 criterion in outcome.json: %s", data)
	return nil, nil, ""
}

func readLog(t *testing.T, rn *runpkg.Run, cr map[string]any) string {
	t.Helper()
	path, _ := cr["LogArtifact"].(string)
	if path == "" {
		t.Fatalf("failed check has no LogArtifact: %v", cr)
	}
	data, err := os.ReadFile(filepath.Join(rn.Dir(), filepath.FromSlash(path)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestFailedCheckRecordsFullLogArtifact(t *testing.T) {
	rn, cr, report := failingCheckRun(t, `"HEAD-MARKER" + strings.Repeat("x", 2000) + "TAIL-MARKER"`)

	if argv, _ := cr["Argv"].([]any); len(argv) == 0 || argv[0] != "go" {
		t.Fatalf("Argv = %v, want the executed go argv", cr["Argv"])
	}
	if wd, _ := cr["WorkDir"].(string); wd == "" {
		t.Fatalf("WorkDir missing: %v", cr)
	}
	if code, _ := cr["ExitCode"].(float64); code == 0 {
		t.Fatalf("ExitCode = %v, want non-zero", cr["ExitCode"])
	}
	if out, _ := cr["Output"].(string); len(out) > 400 {
		t.Fatalf("excerpt is %d bytes, want <= 400", len(out))
	}
	if log := readLog(t, rn, cr); len(log) < 2000 || !strings.Contains(log, "HEAD-MARKER") || !strings.Contains(log, "TAIL-MARKER") {
		t.Fatalf("log artifact does not hold the complete output (%d bytes)", len(log))
	}
	for _, want := range []string{"- argv:", "- workdir:", "- exit:", "- log: `checks/check-1.log`"} {
		if !strings.Contains(report, want) {
			t.Fatalf("report missing %q:\n%s", want, report)
		}
	}
}

func TestCheckEvidenceRedactsSecrets(t *testing.T) {
	const secret = "s3cr3t-evidence-value-42"
	t.Setenv("SOP_TEST_API_TOKEN", secret)
	rn, cr, report := failingCheckRun(t, `"token=`+secret+`"`)

	if log := readLog(t, rn, cr); strings.Contains(log, secret) || !strings.Contains(log, "[redacted]") {
		t.Fatalf("log artifact not redacted:\n%s", log)
	}
	if out, _ := cr["Output"].(string); strings.Contains(out, secret) {
		t.Fatalf("excerpt leaks the secret: %s", out)
	}
	if strings.Contains(report, secret) {
		t.Fatalf("report leaks the secret:\n%s", report)
	}
}

// TestFailedCheckLogWriteFailureIsReported proves a failed check's full-log
// artifact is required evidence: if it cannot be persisted, the lifecycle must
// surface the write failure instead of recording a verdict whose artifact is
// missing.
func TestFailedCheckLogWriteFailureIsReported(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "go.mod", "module example.com/evidence\n\ngo 1.22\n")
	writeFixture(t, dir, "fail/fail.go", "package fail\n")
	writeFixture(t, dir, "fail/fail_test.go", "package fail\n\nimport \"testing\"\n\nfunc TestFail(t *testing.T) { t.Fail() }\n")

	spec := &taskfile.Spec{ID: "S001", AcceptanceChecks: []string{"go test -count=1 ./fail/"}}
	res := deriveOutcome(spec, dir, quality.Result{Decision: quality.Pass})
	rn, err := runpkg.New(dir, "S001")
	if err != nil {
		t.Fatal(err)
	}
	// Make the log-artifact directory impossible to create: a regular file occupies
	// its path, so durablefs cannot write checks/check-1.log.
	if err := os.WriteFile(filepath.Join(rn.Dir(), "checks"), []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeOutcomeArtifact(rn, res); err == nil {
		t.Fatal("a failed log-artifact write was swallowed")
	}
	if _, err := os.Stat(filepath.Join(rn.Dir(), "outcome.json")); err == nil {
		t.Fatal("outcome.json was written despite missing check evidence")
	}
}
