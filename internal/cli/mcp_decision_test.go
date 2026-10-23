package cli

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/question"
)

// callPlannerTool invokes one planner-surface tool through its real handler.
func callPlannerTool(t *testing.T, dir, name, args string) (string, error) {
	t.Helper()
	tool := toolByName(t, mcpPlannerTools(dir, deps{}), name)
	return tool.Handler(context.Background(), json.RawMessage(args))
}

// decisionFixture seeds an initialized project with one task bound to a plan
// revision and one question raised during the task's current attempt, so the
// planner surface has real state to read.
func decisionFixture(t *testing.T, task *domain.Task, planID, sha string) (dir string, q *question.Question) {
	t.Helper()
	dir = t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, task)
	writePlanMeta(t, dir, planID, sha)
	seedRunStage(t, dir, task.ID, "IMPLEMENTING")
	q = createQuestion(t, dir, &question.Question{
		Identity: task.ID, Kind: "blocking", Body: "which input order?", RunID: task.ID,
		PlanRevision: planID + "@" + sha, Revision: 1, Attempt: task.Attempt,
	})
	return dir, q
}

// TestMCPTaskResultTool proves the planner can retrieve a task result: persisted
// status and attempts, the plan revision it is bound to, the latest run stage, and
// the task's active question.
func TestMCPTaskResultTool(t *testing.T) {
	dir, _ := decisionFixture(t, &domain.Task{
		ID: "T001", Title: "Planner contract", Status: domain.PLANNED, Attempt: 1, MaxAttempts: 3,
	}, "plan-x", "abc")

	out, err := callPlannerTool(t, dir, "sop_task_result", `{"task_id":"T001"}`)
	if err != nil {
		t.Fatalf("sop_task_result: %v", err)
	}
	for _, want := range []string{
		"task: T001",
		"title: Planner contract",
		"status: PLANNED",
		"attempt: 1/3",
		"plan_revision: plan-x@abc",
		"run_stage: IMPLEMENTING",
		"question: ",
		"PENDING",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("task result missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "gate: ") {
		t.Errorf("task result reported a gate that was never recorded:\n%s", out)
	}
	if _, err := callPlannerTool(t, dir, "sop_task_result", `{}`); err == nil {
		t.Error("missing task_id must be an error")
	}
}

// TestMCPDecisionListTool proves the planner and the executor can read a task's
// questions, answers, submission outcomes, and continuation state without
// consuming anything.
func TestMCPDecisionListTool(t *testing.T) {
	dir, _ := decisionFixture(t, &domain.Task{
		ID: "T001", Title: "x", Status: domain.PLANNED, Attempt: 1, MaxAttempts: 3,
	}, "plan-x", "abc")

	qs, err := listQuestions(dir, question.StatePending)
	if err != nil || len(qs) != 1 {
		t.Fatalf("pending questions = %d, err = %v; want 1", len(qs), err)
	}
	qid := qs[0].ID

	out, err := callPlannerTool(t, dir, "sop_decision_list", `{"task_id":"T001"}`)
	if err != nil {
		t.Fatalf("sop_decision_list: %v", err)
	}
	for _, want := range []string{"task: T001", "question: " + qid, "state=PENDING"} {
		if !strings.Contains(out, want) {
			t.Errorf("decision list missing %q:\n%s", want, out)
		}
	}

	if _, err := callPlannerTool(t, dir, "sop_decision_submit",
		`{"question_id":"`+qid+`","action":"continue","body":"keep going","answered_by":"planner"}`); err != nil {
		t.Fatalf("submit: %v", err)
	}

	out, err = callPlannerTool(t, dir, "sop_decision_list", `{"task_id":"T001"}`)
	if err != nil {
		t.Fatalf("sop_decision_list after submit: %v", err)
	}
	for _, want := range []string{"state=ANSWERED", "answer: ", "action=continue", "outcome: ACCEPTED"} {
		if !strings.Contains(out, want) {
			t.Errorf("decision list after submit missing %q:\n%s", want, out)
		}
	}
}

// TestMCPDecisionSubmitTool proves a decision is recorded as advice: the question
// advances to ANSWERED with the chosen action and the binding it was made for, and
// no task lifecycle, approval, or execution state changes.
func TestMCPDecisionSubmitTool(t *testing.T) {
	dir, _ := decisionFixture(t, &domain.Task{
		ID: "T001", Title: "x", Status: domain.PLANNED, Attempt: 1, MaxAttempts: 3,
	}, "plan-x", "abc")

	qs, err := listQuestions(dir, question.StatePending)
	if err != nil || len(qs) != 1 {
		t.Fatalf("pending questions = %d, err = %v; want 1", len(qs), err)
	}
	qid := qs[0].ID

	out, err := callPlannerTool(t, dir, "sop_decision_submit",
		`{"question_id":"`+qid+`","task_id":"T001","action":"correct","body":"use the existing record",`+
			`"rationale":"reuse","answered_by":"planner","expected_revision":1,"expected_plan_revision":"plan-x@abc"}`)
	if err != nil {
		t.Fatalf("sop_decision_submit: %v", err)
	}
	for _, want := range []string{"ACCEPTED", "state: ANSWERED", "action: correct", "attempt: 1", "plan_revision: plan-x@abc"} {
		if !strings.Contains(out, want) {
			t.Errorf("submit output missing %q:\n%s", want, out)
		}
	}

	// Advice is not approval and does not execute: the task stays PLANNED and no
	// approval gate exists.
	if _, st, _ := runCLI(t, dir, "status"); !strings.Contains(st, "T001") || !strings.Contains(st, "PLANNED") {
		t.Errorf("task lifecycle changed by a decision: %s", st)
	}
	if _, ap, _ := runCLI(t, dir, "approvals"); !strings.Contains(ap, "no pending approvals") {
		t.Errorf("a decision created an approval gate: %s", ap)
	}

	// An action outside the closed vocabulary is refused, not stored.
	if _, err := callPlannerTool(t, dir, "sop_decision_submit",
		`{"question_id":"`+qid+`","action":"approve","body":"ship it"}`); err == nil {
		t.Error("an unknown decision action must be refused")
	}

	// A decision submitted for the wrong task is stale, not accepted.
	other, err := callPlannerTool(t, dir, "sop_decision_submit",
		`{"question_id":"`+qid+`","task_id":"T999","action":"continue","body":"go"}`)
	if err != nil {
		t.Fatalf("wrong-task submit: %v", err)
	}
	if !strings.Contains(other, "STALE") {
		t.Errorf("wrong-task decision = %q, want STALE", other)
	}
}

// TestMCPDecisionAttemptBindingIsStale proves the attempt binding: a decision made
// for an earlier attempt of the task is refused once the task has moved on, and the
// question's prior record is left intact.
func TestMCPDecisionAttemptBindingIsStale(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "x", Status: domain.PLANNED, Attempt: 2, MaxAttempts: 3})
	writePlanMeta(t, dir, "plan-x", "abc")
	q := createQuestion(t, dir, &question.Question{
		Identity: "T001", Kind: "blocking", Body: "which input order?", RunID: "T001",
		PlanRevision: "plan-x@abc", Revision: 1, Attempt: 1,
	})

	out, err := callPlannerTool(t, dir, "sop_decision_submit",
		`{"question_id":"`+q.ID+`","action":"continue","body":"sort","expected_revision":1}`)
	if err != nil {
		t.Fatalf("sop_decision_submit: %v", err)
	}
	if !strings.Contains(out, "STALE") || !strings.Contains(out, "bound attempt 1 is not current 2") {
		t.Errorf("attempt-stale output = %q, want STALE with the attempt reason", out)
	}

	// The stale submission wrote no answer and did not advance the question.
	got, answers, outcomes, err := readQuestion(dir, q.ID)
	if err != nil {
		t.Fatalf("read question: %v", err)
	}
	if len(answers) != 0 {
		t.Errorf("stale decision recorded %d answer(s), want 0", len(answers))
	}
	if got.State != question.StatePending {
		t.Errorf("question state = %s, want PENDING", got.State)
	}
	if len(outcomes) != 1 || outcomes[0].Kind != question.OutcomeStale {
		t.Errorf("outcomes = %v, want one STALE audit record", outcomes)
	}
}

// TestMCPDecisionDuplicateIsIdempotent proves a re-delivered decision is a
// duplicate: the same idempotency key and payload create no second answer, and the
// first recorded decision is returned.
func TestMCPDecisionDuplicateIsIdempotent(t *testing.T) {
	dir, _ := decisionFixture(t, &domain.Task{
		ID: "T001", Title: "x", Status: domain.PLANNED, Attempt: 1, MaxAttempts: 3,
	}, "plan-x", "abc")

	qs, err := listQuestions(dir, question.StatePending)
	if err != nil || len(qs) != 1 {
		t.Fatalf("pending questions = %d, err = %v; want 1", len(qs), err)
	}
	qid := qs[0].ID
	args := `{"question_id":"` + qid + `","action":"revise","body":"use the durable record",` +
		`"answered_by":"planner","idempotency_key":"decision-1","expected_revision":1}`

	first, err := callPlannerTool(t, dir, "sop_decision_submit", args)
	if err != nil {
		t.Fatalf("first submit: %v", err)
	}
	if !strings.Contains(first, "ACCEPTED") {
		t.Fatalf("first submit = %q, want ACCEPTED", first)
	}
	second, err := callPlannerTool(t, dir, "sop_decision_submit", args)
	if err != nil {
		t.Fatalf("second submit: %v", err)
	}
	if !strings.Contains(second, "DUPLICATE") {
		t.Errorf("second submit = %q, want DUPLICATE", second)
	}

	_, answers, _, err := readQuestion(dir, qid)
	if err != nil {
		t.Fatalf("read question: %v", err)
	}
	if len(answers) != 1 {
		t.Errorf("answers = %d, want exactly 1 after a duplicate delivery", len(answers))
	}
}
