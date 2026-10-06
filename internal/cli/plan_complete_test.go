package cli

import (
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/domain"
)

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

// TestRunPlanCompleteArchivesSatisfiedPlan proves sop plan complete archives a fully
// satisfied plan as COMPLETE and releases its active association.
func TestRunPlanCompleteArchivesSatisfiedPlan(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "S001", Title: "t", Status: domain.LOCAL_DONE, MaxAttempts: 3})

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
