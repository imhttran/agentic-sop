package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/question"
)

// TestResultPublishDoesNotChangeTaskState proves executor result publication is
// evidence only: the task lifecycle and approval state are untouched.
func TestResultPublishDoesNotChangeTaskState(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "x", Status: domain.PLANNED, Attempt: 1, MaxAttempts: 3})
	writePlanMeta(t, dir, "plan-x", "abc")

	code, out, errS := runCLI(t, dir, "result", "publish", "T001",
		"--kind", "BLOCKER", "--summary", "cannot reach the planner tunnel",
		"--evidence", ".agent-sdlc/runs/T001/log.txt", "--by", "zed",
		"--expected-plan-revision", "plan-x@abc")
	if code != exitOK {
		t.Fatalf("publish code = %d, stderr = %s", code, errS)
	}
	for _, want := range []string{"ACCEPTED", "T001", "BLOCKER", "attempt=1", "publication: "} {
		if !strings.Contains(out, want) {
			t.Errorf("publish output missing %q:\n%s", want, out)
		}
	}

	// Evidence only: lifecycle and approvals are unchanged.
	if _, st, _ := runCLI(t, dir, "status"); !strings.Contains(st, "PLANNED") {
		t.Errorf("task lifecycle changed by a publication: %s", st)
	}
	if _, ap, _ := runCLI(t, dir, "approvals"); !strings.Contains(ap, "no pending approvals") {
		t.Errorf("publication created an approval gate: %s", ap)
	}

	// The publication is readable back with its binding.
	code, out, errS = runCLI(t, dir, "result", "list", "T001")
	if code != exitOK {
		t.Fatalf("list code = %d, stderr = %s", code, errS)
	}
	for _, want := range []string{"BLOCKER", "attempt=1", "by=zed", "cannot reach the planner tunnel"} {
		if !strings.Contains(out, want) {
			t.Errorf("list output missing %q:\n%s", want, out)
		}
	}
}

// TestResultPublishRefusesStaleRevisions proves a publication made against an older
// plan revision or attempt is refused and writes nothing.
func TestResultPublishRefusesStaleRevisions(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "x", Status: domain.PLANNED, Attempt: 2, MaxAttempts: 3})
	writePlanMeta(t, dir, "plan-x", "abc")

	code, out, _ := runCLI(t, dir, "result", "publish", "T001",
		"--kind", "PROGRESS", "--summary", "s", "--expected-plan-revision", "plan-x@old")
	if code != exitError || !strings.Contains(out, "STALE") {
		t.Errorf("stale plan revision = code %d, output %q; want STALE with an error exit", code, out)
	}

	code, out, _ = runCLI(t, dir, "result", "publish", "T001",
		"--kind", "PROGRESS", "--summary", "s", "--expect-attempt", "1")
	if code != exitError || !strings.Contains(out, "STALE") {
		t.Errorf("stale attempt = code %d, output %q; want STALE with an error exit", code, out)
	}

	code, out, _ = runCLI(t, dir, "result", "list", "T001")
	if code != exitOK || !strings.Contains(out, "no published results") {
		t.Errorf("a refused publication was stored: code %d, output %q", code, out)
	}
}

// TestMCPTaskResultSurfacesPublishedResult proves the planner reads an executor's
// published result through the same task-result operation PE-1 added.
func TestMCPTaskResultSurfacesPublishedResult(t *testing.T) {
	dir, q := decisionFixture(t, &domain.Task{
		ID: "T001", Title: "x", Status: domain.PLANNED, Attempt: 1, MaxAttempts: 3,
	}, "plan-x", "abc")

	if code, _, errS := runCLI(t, dir, "result", "publish", "T001",
		"--kind", "TEST", "--summary", "go test ./internal/taskresult passed",
		"--evidence", "internal/taskresult/taskresult_test.go", "--by", "zed"); code != exitOK {
		t.Fatalf("publish code = %d, stderr = %s", code, errS)
	}

	out, err := callPlannerTool(t, dir, "sop_task_result", `{"task_id":"T001"}`)
	if err != nil {
		t.Fatalf("sop_task_result: %v", err)
	}
	for _, want := range []string{
		"result: TEST attempt=1 by=zed: go test ./internal/taskresult passed",
		"result_evidence: internal/taskresult/taskresult_test.go",
		"question: " + q.ID,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("task result missing %q:\n%s", want, out)
		}
	}
	if _, aOut, _ := runCLI(t, dir, "approvals"); !strings.Contains(aOut, "no pending approvals") {
		t.Errorf("publishing created an approval gate: %s", aOut)
	}
}

// TestPlannerConsumeRequiresAnAppliedDecision pins the consumption guard: with no
// applied decision and no owed continuation, the executor path does not execute.
func TestPlannerConsumeRequiresAnAppliedDecision(t *testing.T) {
	dir, _ := decisionFixture(t, &domain.Task{
		ID: "T001", Title: "x", Status: domain.PLANNED, Attempt: 1, MaxAttempts: 3,
	}, "plan-x", "abc")
	// Planner mode is opt-in; this test exercises the enabled path.
	writeFile(t, dir, filepath.Join(".agent-sdlc", "config.yaml"),
		"version: 1\nproject:\n  name: agentic-sop\nplanner:\n  enabled: true\n  mode: interactive\n  round_limit: 3\n")

	// A PENDING question is not consumable: status reports WAITING, and --apply
	// parks the run awaiting the planner without executing anything.
	code, out, errS := runCLI(t, dir, "planner", "continue")
	if code != exitOK {
		t.Fatalf("planner continue code = %d, stderr = %s", code, errS)
	}
	if !strings.Contains(out, "T001") || !strings.Contains(strings.ToUpper(out), "WAIT") {
		t.Errorf("planner status = %q, want the task reported as waiting", out)
	}

	qs, err := listQuestions(dir, question.StatePending)
	if err != nil || len(qs) != 1 {
		t.Fatalf("pending questions = %d, err = %v; want 1", len(qs), err)
	}
	if _, st, _ := runCLI(t, dir, "status"); !strings.Contains(st, "PLANNED") {
		t.Errorf("consuming without an applied decision changed the task: %s", st)
	}
}
