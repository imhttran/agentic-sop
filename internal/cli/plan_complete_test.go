package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/domain"
)

// seedApprovalPending writes a PENDING approval request for a task under
// .agent-sdlc/runs/<task-id>/approval.json, the artifact historicalize and the
// tightened complete both read.
func seedApprovalPending(t *testing.T, dir, taskID string) {
	t.Helper()
	runDir := filepath.Join(dir, stateDirName, "runs", taskID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `{
  "id": "APR-1",
  "task_id": "` + taskID + `",
  "status": "PENDING",
  "requested_at": "2026-10-08T00:00:00Z"
}
`
	if err := os.WriteFile(filepath.Join(runDir, "approval.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// seedRunPassed writes a PASSED run state for a task, the verification evidence a
// satisfied task needs to be closed.
func seedRunPassed(t *testing.T, dir, taskID string) {
	t.Helper()
	runDir := filepath.Join(dir, stateDirName, "runs", taskID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `{
  "id": "` + taskID + `",
  "stage": "PASSED",
  "created_at": "2026-10-08T00:00:00Z",
  "updated_at": "2026-10-08T00:00:00Z"
}
`
	if err := os.WriteFile(filepath.Join(runDir, "state.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestRunPlanCompleteRefusesUnresolvedWork proves sop plan complete fails closed when
// a task is unresolved, leaving the plan active and its tasks intact.
func TestRunPlanCompleteRefusesUnresolvedWork(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "S001", Title: "t", Status: domain.PLANNED, MaxAttempts: 3})

	if code, _, _ := runCLI(t, dir, "plan", "complete"); code == exitOK {
		t.Fatal("completion must refuse unresolved work")
	}
	if code, out, _ := runCLI(t, dir, "status"); code != exitOK || !strings.Contains(out, "S001") {
		t.Errorf("a refused completion must leave the plan's tasks intact: %s", out)
	}
}

// TestRunPlanCompleteRefusesUnresolvedApproval proves that a satisfied task with a
// PENDING approval gate blocks completion, matching historicalize readiness. The
// approval is a plan-level blocker regardless of task satisfaction.
func TestRunPlanCompleteRefusesUnresolvedApproval(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "S001", Title: "t", Status: domain.LOCAL_DONE, MaxAttempts: 3})
	seedRunPassed(t, dir, "S001")
	seedApprovalPending(t, dir, "S001")

	code, _, errOut := runCLI(t, dir, "plan", "complete")
	if code == exitOK {
		t.Fatal("completion must refuse an unresolved approval")
	}
	if !strings.Contains(errOut, "approval") {
		t.Errorf("refusal reason should name the approval blocker: %s", errOut)
	}
	if code, out, _ := runCLI(t, dir, "status"); code != exitOK || !strings.Contains(out, "S001") {
		t.Errorf("a refused completion must leave the plan's tasks intact: %s", out)
	}
}

// TestRunPlanCompleteRefusesMissingVerification proves that a satisfied task with no
// recorded verification evidence blocks completion, matching historicalize readiness.
func TestRunPlanCompleteRefusesMissingVerification(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "S001", Title: "t", Status: domain.LOCAL_DONE, MaxAttempts: 3})

	code, _, errOut := runCLI(t, dir, "plan", "complete")
	if code == exitOK {
		t.Fatal("completion must refuse missing verification")
	}
	if !strings.Contains(errOut, "verification") {
		t.Errorf("refusal reason should name the verification blocker: %s", errOut)
	}
	if code, out, _ := runCLI(t, dir, "status"); code != exitOK || !strings.Contains(out, "S001") {
		t.Errorf("a refused completion must leave the plan's tasks intact: %s", out)
	}
}

// TestRunPlanCompleteArchivesSatisfiedPlan proves sop plan complete archives a fully
// satisfied, verified plan as COMPLETE and releases its active association.
func TestRunPlanCompleteArchivesSatisfiedPlan(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "S001", Title: "t", Status: domain.LOCAL_DONE, MaxAttempts: 3})
	seedRunPassed(t, dir, "S001")

	code, out, errOut := runCLI(t, dir, "plan", "complete")
	if code != exitOK {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, out, errOut)
	}
	if !strings.Contains(out, "COMPLETE") {
		t.Errorf("stdout = %q", out)
	}
	code, out2, _ := runCLI(t, dir, "status")
	if code != exitOK || strings.Contains(out2, "State: ACTIVE") {
		t.Errorf("the completed plan must not be ACTIVE: %s", out2)
	}
}

// TestPlanCompleteAndHistoricalizeAgreeOnIneligibility proves the two closure paths
// agree on the same fixtures: an unresolved-approval plan and an
// unresolved-verification plan are refused by both, so complete and historicalize
// cannot disagree about what "ready to close" means (I3/I4/I5).
func TestPlanCompleteAndHistoricalizeAgreeOnIneligibility(t *testing.T) {
	cases := []struct {
		name  string
		seed  func(t *testing.T, dir string)
		match string
	}{
		{
			name: "unresolved approval",
			seed: func(t *testing.T, dir string) {
				seedTask(t, dir, &domain.Task{ID: "S001", Title: "t", Status: domain.LOCAL_DONE, MaxAttempts: 3})
				seedRunPassed(t, dir, "S001")
				seedApprovalPending(t, dir, "S001")
			},
			match: "approval",
		},
		{
			name: "unresolved verification",
			seed: func(t *testing.T, dir string) {
				seedTask(t, dir, &domain.Task{ID: "S001", Title: "t", Status: domain.LOCAL_DONE, MaxAttempts: 3})
			},
			match: "verification",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// complete path
			dirA := t.TempDir()
			initProject(t, dirA)
			tc.seed(t, dirA)
			completeCode, _, completeErr := runCLI(t, dirA, "plan", "complete")
			if completeCode == exitOK {
				t.Fatalf("complete must refuse %s", tc.name)
			}
			if !strings.Contains(completeErr, tc.match) {
				t.Errorf("complete refusal reason %q should mention %q", completeErr, tc.match)
			}

			// historicalize path
			dirB := t.TempDir()
			initProject(t, dirB)
			tc.seed(t, dirB)
			histCode, _, histErr := runCLI(t, dirB, "plan", "historicalize")
			if histCode == exitOK {
				t.Fatalf("historicalize must refuse %s", tc.name)
			}
			if !strings.Contains(histErr, tc.match) {
				t.Errorf("historicalize refusal reason %q should mention %q", histErr, tc.match)
			}
		})
	}
}
