package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/question"
)

// writeRunState materializes a run's durable identity exactly as the executor would,
// creating the run directory first.
func writeRunState(t *testing.T, dir, taskID, createdAt string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, config.DirName, "runs", taskID), 0o755); err != nil {
		t.Fatalf("create run dir: %v", err)
	}
	writeFile(t, dir, filepath.Join(config.DirName, "runs", taskID, "state.json"),
		`{"id":"`+taskID+`","stage":"CREATED","created_at":"`+createdAt+`"}`)
}

// runEpochOf reads a task run's durable epoch (state.json created_at).
func runEpochOf(t *testing.T, dir, taskID string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, config.DirName, "runs", taskID, "state.json"))
	if err != nil {
		t.Fatalf("read run state: %v", err)
	}
	var st struct {
		CreatedAt string `json:"created_at"`
	}
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatalf("parse run state: %v", err)
	}
	return st.CreatedAt
}

// TestPlannerAskBindsFirstRunEpoch proves the ordering fix: a question raised on a task
// that has never run binds the durable run epoch that planner ask materializes, instead of
// an empty one. The previous order read the epoch before the run state existed.
func TestPlannerAskBindsFirstRunEpoch(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "x", Status: domain.PLANNED, MaxAttempts: 3})
	enablePlanner(t, dir, "")

	statePath := filepath.Join(dir, config.DirName, "runs", "T001", "state.json")
	if _, err := os.Stat(statePath); !os.IsNotExist(err) {
		t.Fatalf("a run state already exists before ask (%v); the first-run path is not exercised", err)
	}

	if code, _, errS := runCLI(t, dir, "planner", "ask", "T001", "--question", "proceed?"); code != exitOK {
		t.Fatalf("ask: code=%d stderr=%s", code, errS)
	}

	q := pendingQuestion(t, dir)
	if strings.TrimSpace(q.RunEpoch) == "" {
		t.Fatal("first-run question bound an empty run epoch")
	}
	if got, want := q.RunEpoch, runEpochOf(t, dir, "T001"); got != want {
		t.Fatalf("question epoch = %q, want the durable run epoch %q", got, want)
	}
}

// TestPlannerAskEpochBoundContinuationResumes proves the matching-epoch path end to end: a
// question bound to the live epoch produces a continuation keyed by that epoch, and the
// resume claims and completes it.
func TestPlannerAskEpochBoundContinuationResumes(t *testing.T) {
	dir, st, task := plannerProject(t)
	defer st.Close()
	cfg, err := loadConfigOrDefault(dir)
	if err != nil {
		t.Fatal(err)
	}
	epoch := currentRunEpoch(dir, "T001")
	if epoch == "" {
		t.Fatal("fixture has no run epoch")
	}

	if code, _, errS := runCLI(t, dir, "planner", "ask", "T001", "--question", "proceed?"); code != exitOK {
		t.Fatalf("ask: code=%d stderr=%s", code, errS)
	}
	q := pendingQuestion(t, dir)
	if q.RunEpoch != epoch {
		t.Fatalf("question epoch = %q, want %q", q.RunEpoch, epoch)
	}

	answer(t, dir, q.ID, "go")
	if code, _, errS := runCLI(t, dir, "planner", "continue", "--apply"); code != exitOK {
		t.Fatalf("apply: code=%d stderr=%s", code, errS)
	}

	svc := question.NewService(plannerStore(t, dir), nil)
	cont, err := svc.Continuation(q.ID, epoch)
	if err != nil {
		t.Fatalf("continuation: %v", err)
	}
	if want := question.ContinuationID(q.ID, epoch); cont.ID != want {
		t.Fatalf("continuation id = %q, want the epoch-bound %q", cont.ID, want)
	}
	if cont.State != question.ContinuationPending {
		t.Fatalf("continuation state = %s, want PENDING", cont.State)
	}

	// A resume from IMPLEMENTING reads the parked run's plan.
	writeFile(t, dir, filepath.Join(config.DirName, "runs", "T001", "plan.md"), plannerResumePlanMD)

	a := &scriptedPlannerAgent{}
	var out bytes.Buffer
	code := runScheduledTask(context.Background(), dir, cfg, a, plannerDeps(dir, a, plannerDiff), st, task, newRunSession(), &out, &out)
	if code != exitOK {
		t.Fatalf("resume code=%d out=%s", code, out.String())
	}
	done, _ := st.Get("T001")
	if done.Status != domain.LOCAL_DONE {
		t.Fatalf("task status = %s, want LOCAL_DONE", done.Status)
	}
	finished, err := svc.Continuation(q.ID, epoch)
	if err != nil {
		t.Fatalf("continuation after resume: %v", err)
	}
	if finished.State != question.ContinuationCompleted {
		t.Fatalf("continuation state = %s, want COMPLETED", finished.State)
	}
}

// TestMCPDecisionStaleRunEpochIsRefused proves a decision bound to a superseded run epoch is
// refused rather than applied: no answer is written, the question stays PENDING, and the
// refusal is recorded as a STALE audit outcome.
func TestMCPDecisionStaleRunEpochIsRefused(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "x", Status: domain.PLANNED, Attempt: 1, MaxAttempts: 3})
	writePlanMeta(t, dir, "plan-x", "abc")
	// The live run is a LATER epoch than the one the question was bound to.
	writeRunState(t, dir, "T001", "2026-02-02T00:00:00Z")
	q := createQuestion(t, dir, &question.Question{
		Identity: "T001", Kind: "blocking", Body: "proceed?", RunID: "T001",
		RunEpoch: "2026-01-01T00:00:00Z", PlanRevision: "plan-x@abc", Revision: 1, Attempt: 1,
	})

	out, err := callPlannerTool(t, dir, "sop_decision_submit",
		`{"question_id":"`+q.ID+`","action":"continue","body":"go"}`)
	if err != nil {
		t.Fatalf("sop_decision_submit: %v", err)
	}
	if !strings.Contains(out, "STALE") || !strings.Contains(out, "run epoch changed") {
		t.Errorf("stale-epoch decision = %q, want STALE with the run-epoch reason", out)
	}

	got, answers, outcomes, err := readQuestion(dir, q.ID)
	if err != nil {
		t.Fatalf("read question: %v", err)
	}
	if len(answers) != 0 {
		t.Errorf("a refused decision recorded %d answer(s), want 0", len(answers))
	}
	if got.State != question.StatePending {
		t.Errorf("question state = %s, want PENDING", got.State)
	}
	if len(outcomes) != 1 || outcomes[0].Kind != question.OutcomeStale {
		t.Errorf("outcomes = %v, want one STALE audit record", outcomes)
	}
}

// TestMCPDecisionListReportsEpochBinding proves the read surface states the run-epoch binding
// explicitly, including its absence for a legacy record, and that a legacy unbound question is
// still decidable (it is unbound, not stale).
func TestMCPDecisionListReportsEpochBinding(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "x", Status: domain.PLANNED, Attempt: 1, MaxAttempts: 3})
	writePlanMeta(t, dir, "plan-x", "abc")
	writeRunState(t, dir, "T001", "2026-01-01T00:00:00Z")

	bound := createQuestion(t, dir, &question.Question{
		Identity: "T001", Kind: "bound", Body: "bound question", RunID: "T001",
		RunEpoch: "2026-01-01T00:00:00Z", PlanRevision: "plan-x@abc", Revision: 1, Attempt: 1,
	})
	createQuestion(t, dir, &question.Question{
		Identity: "T001", Kind: "legacy", Body: "legacy question", RunID: "T001",
		PlanRevision: "plan-x@abc", Revision: 1, Attempt: 1,
	})

	out, err := callPlannerTool(t, dir, "sop_decision_list", `{"task_id":"T001"}`)
	if err != nil {
		t.Fatalf("sop_decision_list: %v", err)
	}
	if !strings.Contains(out, "question_run_epoch: 2026-01-01T00:00:00Z") {
		t.Errorf("decision list does not report the bound epoch:\n%s", out)
	}
	if !strings.Contains(out, "question_run_epoch: unbound (legacy)") {
		t.Errorf("decision list does not report the legacy unbound epoch:\n%s", out)
	}
	if strings.Contains(out, "question_stale:") {
		t.Errorf("neither question is stale; the list must not claim staleness:\n%s", out)
	}

	// A legacy unbound question is decidable: the epoch is absent, not mismatched.
	legacyOut, err := callPlannerTool(t, dir, "sop_decision_submit",
		`{"question_id":"`+legacyQuestionID(t, dir)+`","action":"continue","body":"go"}`)
	if err != nil {
		t.Fatalf("legacy submit: %v", err)
	}
	if !strings.Contains(legacyOut, "ACCEPTED") {
		t.Errorf("legacy unbound decision = %q, want ACCEPTED", legacyOut)
	}
	if bound.ID == "" {
		t.Fatal("bound question was not created")
	}
}

// legacyQuestionID returns the id of the question created with the legacy Kind.
func legacyQuestionID(t *testing.T, dir string) string {
	t.Helper()
	qs, err := listQuestions(dir, question.StatePending)
	if err != nil {
		t.Fatalf("list questions: %v", err)
	}
	for _, q := range qs {
		if q.Kind == "legacy" {
			return q.ID
		}
	}
	t.Fatal("legacy question not found")
	return ""
}
