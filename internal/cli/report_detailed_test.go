package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/quality"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
)

// TestNextActionDerivesFromOutcome proves the concise default states a single next
// action derived from the gate verdict, without changing task state.
func TestNextActionDerivesFromOutcome(t *testing.T) {
	cases := []struct {
		name string
		doc  runReportDoc
		want string
	}{
		{"pass", runReportDoc{ID: "T1", Decision: quality.Pass, Stage: runpkg.Passed}, "sop commit"},
		{"fail", runReportDoc{ID: "T1", Decision: quality.Fail}, "sop retry T1"},
		{"needs human", runReportDoc{ID: "T1", Decision: quality.NeedsHuman}, "resolve the human gate"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := nextAction(tc.doc); !strings.Contains(got, tc.want) {
				t.Errorf("nextAction = %q, want it to contain %q", got, tc.want)
			}
		})
	}
}

// TestReportDetailedAddsFindings proves the output-efficiency contract: the default
// report stays concise (severity counts + a next action) and `--detailed` adds the
// per-finding detail, which is already present in the run artifacts.
func TestReportDetailedAddsFindings(t *testing.T) {
	dir := t.TempDir()
	runDir := filepath.Join(dir, stateDirName, "runs", "T001")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	report := `{"id":"T001","stage":"PASSED","provider":"command","review_engine":"self","decision":"PASS","fix_cycles":0,` +
		`"validation":[{"Category":"BUILD","Command":"go build ./...","Status":"PASS"}],` +
		`"findings":[{"Severity":"MEDIUM","Title":"consider x","File":"a.go","Line":7}]}`
	if err := os.WriteFile(filepath.Join(runDir, "report.json"), []byte(report), 0o644); err != nil {
		t.Fatal(err)
	}

	// Concise default: a next action, but no per-finding detail.
	if code, out, errOut := runCLI(t, dir, "report"); code != exitOK {
		t.Fatalf("report code=%d stderr=%s", code, errOut)
	} else {
		if !strings.Contains(out, "Next action:") {
			t.Errorf("default report missing the next action:\n%s", out)
		}
		if strings.Contains(out, "Findings (detail):") {
			t.Errorf("default report must stay concise:\n%s", out)
		}
	}

	// Detailed: the per-finding lines are added.
	if code, out, errOut := runCLI(t, dir, "report", "--detailed"); code != exitOK {
		t.Fatalf("report --detailed code=%d stderr=%s", code, errOut)
	} else {
		if !strings.Contains(out, "Findings (detail):") || !strings.Contains(out, "consider x") {
			t.Errorf("detailed report missing per-finding detail:\n%s", out)
		}
	}
}
