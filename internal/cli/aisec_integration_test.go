package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// TestAISEC002FalseVerificationClaimIsRefused submits an ALREADY_SATISFIED claim
// with no trusted execution evidence through the production lifecycle. SOP must
// refuse a passed verdict and must not persist a PASS gate.
func TestAISEC002FalseVerificationClaimIsRefused(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile+"\n## Acceptance Criteria\n\n- verified behavior\n")
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  test:\n    - \"true\"\nquality:\n  max_fix_cycles: 1\n")
	a := &satisfiedAgent{verified: false, outcomeAgent: outcomeAgent{outcome: &agent.Outcome{
		Status:          agent.OutcomeCompleted,
		Completion:      agent.AlreadySatisfied,
		ChangesExpected: false,
		Evidence: &agent.CompletionEvidence{
			Acceptance:         []agent.AcceptanceEvidence{{Criterion: "verified behavior", Paths: []string{"existing.go"}}},
			ValidationCommands: []string{"true"},
		},
	}}}
	code, stdout, stderr := runInjectedCLI(t, dir, "", a, "run", "--task", "TASK.md")
	if code == exitOK {
		t.Fatalf("false verification claim passed: stdout=%s stderr=%s", stdout, stderr)
	}
	if strings.Contains(stdout, "run T001: PASS") {
		t.Fatalf("false verification claim reported PASS: %s", stdout)
	}

	gates, err := filepath.Glob(filepath.Join(dir, stateDirName, "runs", "*", "gate.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range gates {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var gate struct {
			Decision string `json:"decision"`
		}
		if err := json.Unmarshal(data, &gate); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		if strings.EqualFold(gate.Decision, "PASS") {
			t.Fatalf("a successful gate was persisted at %s: %s", path, data)
		}
	}

	reports, err := filepath.Glob(filepath.Join(dir, stateDirName, "runs", "*", "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range reports {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var report struct {
			Decision string `json:"decision"`
		}
		if err := json.Unmarshal(data, &report); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		if strings.EqualFold(report.Decision, "PASS") {
			t.Fatalf("a passed report was persisted at %s: %s", path, data)
		}
	}
}
