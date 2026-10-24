package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/question"
)

// gateFixture seeds an initialized project with one task, a planner-enabled
// configuration, and a run parked at WAITING_FOR_HUMAN.
func gateFixture(t *testing.T, status domain.TaskStatus) string {
	t.Helper()
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "x", Status: status, Attempt: 1, MaxAttempts: 3})
	writeFile(t, dir, filepath.Join(".agent-sdlc", "config.yaml"),
		"version: 1\nproject:\n  name: gate-fixture\nplanner:\n  enabled: true\n  mode: interactive\n  round_limit: 3\n")
	seedRunStage(t, dir, "T001", "WAITING_FOR_HUMAN")
	return dir
}

// gateTask loads one persisted task for the gate assertions.
func gateTask(t *testing.T, dir, id string) *domain.Task {
	t.Helper()
	task, _ := loadTask(t, dir, id)
	if task == nil {
		t.Fatalf("task %s not found", id)
	}
	return task
}

// approvalHistoryPath is the append-only decision ledger for a task.
func approvalHistoryPath(dir string) string {
	return filepath.Join(dir, stateDirName, "runs", "T001", "approval-history.json")
}

// TestPlannerHumanGateDeclinedRequeueAllowsAsk proves the reported bug is fixed: a
// DECLINED approval on a task the run already requeued to PLANNED no longer blocks
// `sop planner ask`, and the decision ledger is left byte-identical.
func TestPlannerHumanGateDeclinedRequeueAllowsAsk(t *testing.T) {
	dir := gateFixture(t, domain.PLANNED)
	seedApprovalStatus(t, dir, "T001", domain.ApprovalDeclined)
	history := `[{"ID":"APR-1","TaskID":"T001","Status":"DECLINED"}]`
	writeFile(t, dir, filepath.Join(stateDirName, "runs", "T001", "approval-history.json"), history)
	before, err := os.ReadFile(approvalHistoryPath(dir))
	if err != nil {
		t.Fatalf("read history: %v", err)
	}

	task := gateTask(t, dir, "T001")
	if plannerHumanGate(dir, task) {
		t.Fatal("a declined approval on a requeued task is still reported as a human gate")
	}

	code, out, errS := runCLI(t, dir, "planner", "ask", "T001", "--question", "proceed?", "--option", "yes")
	if code != exitOK {
		t.Fatalf("planner ask code = %d, stderr = %s", code, errS)
	}
	if !strings.Contains(out, "PENDING") || !strings.Contains(out, "T001") {
		t.Errorf("planner ask output = %q, want a PENDING question for T001", out)
	}
	qs, err := listQuestions(dir, question.StatePending)
	if err != nil || len(qs) != 1 {
		t.Fatalf("pending questions = %d, err = %v; want 1", len(qs), err)
	}

	after, err := os.ReadFile(approvalHistoryPath(dir))
	if err != nil {
		t.Fatalf("read history after ask: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("approval history changed by planner ask:\nbefore=%s\nafter=%s", before, after)
	}
}

// TestPlannerHumanGatePendingApprovalBlocksAsk proves an unresolved request still
// owns the task, even though the task itself is runnable.
func TestPlannerHumanGatePendingApprovalBlocksAsk(t *testing.T) {
	dir := gateFixture(t, domain.PLANNED)
	seedApprovalStatus(t, dir, "T001", domain.ApprovalPending)

	task := gateTask(t, dir, "T001")
	if !plannerHumanGate(dir, task) {
		t.Fatal("a PENDING approval is not reported as a human gate")
	}
	code, _, errS := runCLI(t, dir, "planner", "ask", "T001", "--question", "proceed?")
	if code != exitError || !strings.Contains(errS, "human gate") {
		t.Errorf("planner ask = code %d, stderr %q; want a refused human-gate error", code, errS)
	}
	if qs, _ := listQuestions(dir, question.StatePending); len(qs) != 0 {
		t.Errorf("a gated task accepted %d question(s), want 0", len(qs))
	}
}

// TestPlannerHumanGateBlockedTaskRemainsGated proves the parked stage still owns a
// task that never left the boundary: requeueing is what releases it, and a BLOCKED
// task is not requeued.
func TestPlannerHumanGateBlockedTaskRemainsGated(t *testing.T) {
	dir := gateFixture(t, domain.BLOCKED)
	seedApprovalStatus(t, dir, "T001", domain.ApprovalDeclined)

	task := gateTask(t, dir, "T001")
	if !plannerHumanGate(dir, task) {
		t.Fatal("a BLOCKED task parked at WAITING_FOR_HUMAN is not reported as a human gate")
	}
	code, _, errS := runCLI(t, dir, "planner", "ask", "T001", "--question", "proceed?")
	if code != exitError || !strings.Contains(errS, "human gate") {
		t.Errorf("planner ask = code %d, stderr %q; want a refused human-gate error", code, errS)
	}
}

// TestPlannerHumanGateIgnoresUnparkedStage proves the stage check is scoped to the
// parked boundary: a runnable task whose stage is anything else is not gated by it.
func TestPlannerHumanGateIgnoresUnparkedStage(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "x", Status: domain.PLANNED, Attempt: 1, MaxAttempts: 3})
	seedRunStage(t, dir, "T001", "IMPLEMENTING")

	task := gateTask(t, dir, "T001")
	if plannerHumanGate(dir, task) {
		t.Fatal("a runnable task in a non-parked stage is reported as a human gate")
	}
}
