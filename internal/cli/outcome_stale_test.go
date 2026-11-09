package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/outcome"
)

// TestFailedRunDoesNotExposePreviousVerifiedOutcome pins GROUND-003: the recorded
// verdict artifact describes the LATEST attempt. A run that stops before deriving
// one (here, a completion claim with no repository change) must not leave an
// earlier run's VERIFIED visible to `sop verify`.
func TestFailedRunDoesNotExposePreviousVerifiedOutcome(t *testing.T) {
	dir := t.TempDir()
	// A previous run verified this task. Its artifact must not survive a later
	// attempt that derives no verdict.
	writeOutcomeArtifactFor(t, dir, "T001", &outcomeArtifact{
		Status:   outcome.Verified,
		Criteria: []outcome.CriterionResult{{ID: "criterion-1", Required: true, Met: true, Evidence: "met"}},
		Reasons:  []string{"every required criterion was independently met"},
	})

	writeFile(t, dir, "TASK.md", "# T001 --- Add widget\n\n## Objective\n\nAdd the widget.\n")
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")

	// The agent claims completion but the tree is unchanged, so the lifecycle stops
	// before deriving an objective-level verdict.
	a := &fakeCapabilityAgent{plan: validPlanJSON, impl: "changed files", review: `{"summary":"clean","findings":[]}`}
	_, stdout, stderr := runInjectedCLI(t, dir, "", a, "run", "--task", "TASK.md")
	if strings.Contains(stdout, "outcome: VERIFIED") {
		t.Fatalf("stale VERIFIED was reported for a run that derived no outcome: stdout=%q stderr=%q", stdout, stderr)
	}
	if _, err := os.Stat(outcomeArtifactPath(dir, "T001")); !os.IsNotExist(err) {
		t.Fatalf("stale outcome.json survived the attempt: %v", err)
	}

	// The read-only verify surface agrees with the persisted state: no current
	// verdict exists, so it must not reprint the previous run's VERIFIED.
	code, verifyOut, verifyErr := runCLI(t, dir, "verify", "T001")
	if code != exitOK {
		t.Fatalf("verify code=%d stderr=%s", code, verifyErr)
	}
	if strings.Contains(verifyOut, "VERIFIED") {
		t.Fatalf("verify reprinted a stale VERIFIED: %s", verifyOut)
	}
	if !strings.Contains(verifyOut, "no recorded outcome") {
		t.Fatalf("verify output = %q, want an explicit no-verdict result", verifyOut)
	}
}
