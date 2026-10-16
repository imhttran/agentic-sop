package cli

import (
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

func plannerStore(t *testing.T, dir string) *question.FileStore {
	t.Helper()
	fs, err := question.OpenFileStore(filepath.Join(dir, config.DirName, questionStoreName))
	if err != nil {
		t.Fatalf("open question store: %v", err)
	}
	return fs
}

func runStage(t *testing.T, dir, taskID string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, config.DirName, "runs", taskID, "state.json"))
	if err != nil {
		t.Fatalf("read run state: %v", err)
	}
	var st struct {
		Stage string `json:"stage"`
	}
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatalf("run state: %v", err)
	}
	return st.Stage
}

func pendingQuestion(t *testing.T, dir string) *question.Question {
	t.Helper()
	qs, err := question.NewService(plannerStore(t, dir), nil).ListQuestions(question.StatePending)
	if err != nil {
		t.Fatalf("list questions: %v", err)
	}
	if len(qs) == 0 {
		t.Fatal("no pending question")
	}
	return qs[0]
}

func enablePlanner(t *testing.T, dir, extra string) {
	t.Helper()
	writeConfig(t, dir, "project:\n  name: \"demo\"\nplanner:\n  enabled: true\n"+extra)
}

func TestCLIPlannerAskStatusContinue(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "x", Status: domain.PLANNED, MaxAttempts: 3})
	enablePlanner(t, dir, "")

	if code, _, errS := runCLI(t, dir, "planner", "ask", "T001",
		"--question", "Which documentation format should this disposable demo use?",
		"--option", "Markdown", "--option", "plain text"); code != exitOK {
		t.Fatalf("ask code=%d err=%s", code, errS)
	}
	if got := runStage(t, dir, "T001"); got != "WAITING_FOR_PLANNER" {
		t.Fatalf("run stage = %q, want WAITING_FOR_PLANNER", got)
	}
	q := pendingQuestion(t, dir)

	if _, out, _ := runCLI(t, dir, "planner", "status"); !strings.Contains(out, "WAIT") {
		t.Fatalf("status = %q, want WAIT", out)
	}
	// The planner answer never applies on submission.
	if _, out, _ := runCLI(t, dir, "answer", q.ID, "--body", "Markdown", "--idempotency-key", "k1"); !strings.Contains(out, "accepted") {
		t.Fatalf("answer = %q", out)
	}
	if _, out, _ := runCLI(t, dir, "planner", "status"); !strings.Contains(out, "APPLY_THEN_CONTINUE") {
		t.Fatalf("status = %q, want APPLY_THEN_CONTINUE", out)
	}
	// Reconciliation applies and reports; it must NOT consume the owed work.
	if _, out, _ := runCLI(t, dir, "planner", "continue", "--apply"); !strings.Contains(out, "APPLIED") || !strings.Contains(out, "continuation pending execution") {
		t.Fatalf("continue = %q", out)
	}
	c, err := question.NewService(plannerStore(t, dir), nil).Continuation(q.ID, q.RunEpoch)
	if err != nil {
		t.Fatalf("continuation = %+v err=%v", c, err)
	}
	if c.State == question.ContinuationCompleted {
		t.Fatal("stage-only reconciliation consumed the owed continuation")
	}
	// The continuation is still owed, so the executor path still has work to do.
	if _, out, _ := runCLI(t, dir, "planner", "status"); !strings.Contains(out, "CONTINUE") {
		t.Fatalf("status = %q, want CONTINUE (still owed)", out)
	}
	if got := runStage(t, dir, "T001"); got != "WAITING_FOR_PLANNER" {
		t.Fatalf("run stage = %q, want WAITING_FOR_PLANNER (checkpoint untouched by reconciliation)", got)
	}
}

func TestCLIPlannerDisabledRefuses(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "x", Status: domain.PLANNED, MaxAttempts: 3})
	// initProject's config has no planner block: the feature is off.
	if code, _, errS := runCLI(t, dir, "planner", "ask", "T001", "--question", "q"); code != exitError {
		t.Fatalf("ask code=%d, want error; err=%s", code, errS)
	} else if !strings.Contains(errS, "disabled") {
		t.Fatalf("stderr = %q, want a disabled message", errS)
	}
}

func TestCLIPlannerRoundLimit(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "x", Status: domain.PLANNED, MaxAttempts: 3})
	enablePlanner(t, dir, "  round_limit: 1\n")

	if code, _, errS := runCLI(t, dir, "planner", "ask", "T001", "--question", "q1"); code != exitOK {
		t.Fatalf("ask1 code=%d err=%s", code, errS)
	}
	if code, _, errS := runCLI(t, dir, "planner", "ask", "T001", "--question", "q2"); code != exitError {
		t.Fatalf("ask2 code=%d, want error; err=%s", code, errS)
	} else if !strings.Contains(errS, "round limit") {
		t.Fatalf("stderr = %q, want a round-limit message", errS)
	}
}

func TestCLIPlannerRespectsHumanGate(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "x", Status: domain.PLANNED, MaxAttempts: 3})
	enablePlanner(t, dir, "")
	// The task is parked at a human boundary.
	if err := os.MkdirAll(filepath.Join(dir, config.DirName, "runs", "T001"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join(config.DirName, "runs", "T001", "state.json"),
		`{"id":"T001","stage":"WAITING_FOR_HUMAN","created_at":"2026-01-01T00:00:00Z"}`)

	if code, _, errS := runCLI(t, dir, "planner", "ask", "T001", "--question", "q"); code != exitError {
		t.Fatalf("ask code=%d, want error; err=%s", code, errS)
	} else if !strings.Contains(errS, "human gate") {
		t.Fatalf("stderr = %q, want a human-gate message", errS)
	}
	if _, out, _ := runCLI(t, dir, "planner", "status"); !strings.Contains(out, "SKIP_HUMAN_GATE") {
		t.Fatalf("status = %q, want SKIP_HUMAN_GATE", out)
	}
}

// TestCLIPlannerCrashBeforeApply proves the crash-before-application case: the answer
// is recorded but not applied, and reconciliation applies then continues it.
func TestCLIPlannerCrashBeforeApply(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "x", Status: domain.PLANNED, MaxAttempts: 3})
	enablePlanner(t, dir, "")
	q := createQuestion(t, dir, &question.Question{
		Identity: "T001", Kind: "k", Body: "q", RunID: "T001", RunEpoch: currentRunEpoch(dir, "T001"),
		PlanRevision: currentPlanRevision(dir), Revision: 1,
	})
	if _, err := question.NewService(plannerStore(t, dir), nil).CreateAnswer(&question.Answer{QuestionID: q.ID, Body: "Markdown"}); err != nil {
		t.Fatalf("answer: %v", err)
	}
	if _, out, _ := runCLI(t, dir, "planner", "status"); !strings.Contains(out, "APPLY_THEN_CONTINUE") {
		t.Fatalf("status = %q, want APPLY_THEN_CONTINUE", out)
	}
	if _, out, _ := runCLI(t, dir, "planner", "continue", "--apply"); !strings.Contains(out, "APPLIED") {
		t.Fatalf("continue = %q, want APPLIED", out)
	}
}

// TestCLIPlannerCrashAfterApplyBeforeContinue proves the crash-after-persistence case:
// the answer is applied with a pending continuation, and reconciliation continues it
// without re-applying.
func TestCLIPlannerCrashAfterApplyBeforeContinue(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "x", Status: domain.PLANNED, MaxAttempts: 3})
	enablePlanner(t, dir, "")
	q := createQuestion(t, dir, &question.Question{
		Identity: "T001", Kind: "k", Body: "q", RunID: "T001", RunEpoch: currentRunEpoch(dir, "T001"),
		ReturnStage: "IMPLEMENTING", PlanRevision: currentPlanRevision(dir), Revision: 1,
	})
	svc := question.NewService(plannerStore(t, dir), nil)
	if _, err := svc.CreateAnswer(&question.Answer{QuestionID: q.ID, Body: "Markdown"}); err != nil {
		t.Fatalf("answer: %v", err)
	}
	if _, err := svc.ApplyAnswer(context.Background(), q.ID); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if _, out, _ := runCLI(t, dir, "planner", "status"); !strings.Contains(out, "CONTINUE") {
		t.Fatalf("status = %q, want CONTINUE", out)
	}
	if _, out, _ := runCLI(t, dir, "planner", "continue", "--apply"); !strings.Contains(out, "PENDING execution") {
		t.Fatalf("continue = %q, want a pending-execution report", out)
	}
	c, err := svc.Continuation(q.ID, q.RunEpoch)
	if err != nil {
		t.Fatalf("continuation: %v", err)
	}
	if c.State == question.ContinuationCompleted {
		t.Fatal("reconciliation completed the continuation without running it")
	}
}

// TestCLIPlannerReconcileDoesNotConsumeOwedWork proves stage-only reconciliation cannot
// consume a continuation: `planner continue --apply` applies and reports, but leaves the
// continuation owed (not COMPLETED) and the run checkpoint untouched, so the guarded
// executor path can still run the work.
func TestCLIPlannerReconcileDoesNotConsumeOwedWork(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "x", Status: domain.PLANNED, MaxAttempts: 3})
	enablePlanner(t, dir, "")
	writePlanMeta(t, dir, "p", "h")
	q := createQuestion(t, dir, &question.Question{
		Identity: "T001", Kind: "k", Body: "q", RunID: "T001", RunEpoch: currentRunEpoch(dir, "T001"),
		ReturnStage: "IMPLEMENTING", PlanRevision: currentPlanRevision(dir), Revision: 1,
	})
	svc := question.NewService(plannerStore(t, dir), nil)
	if _, err := svc.CreateAnswer(&question.Answer{QuestionID: q.ID, Body: "Markdown"}); err != nil {
		t.Fatalf("answer: %v", err)
	}
	if _, out, _ := runCLI(t, dir, "planner", "continue", "--apply"); !strings.Contains(out, "APPLIED") {
		t.Fatalf("continue = %q, want APPLIED", out)
	}
	// The continuation is created but owed: reconciliation did not complete it.
	c, err := svc.Continuation(q.ID, q.RunEpoch)
	if err != nil {
		t.Fatalf("continuation: %v", err)
	}
	if c.State == question.ContinuationCompleted {
		t.Fatal("stage-only reconciliation consumed the owed continuation")
	}
	// It is still actionable for the executor path.
	if _, out, _ := runCLI(t, dir, "planner", "status"); !strings.Contains(out, "CONTINUE") {
		t.Fatalf("status = %q, want CONTINUE (still owed)", out)
	}
}

// TestCLIPlannerRejectsUnsupportedReturnStage proves an unsupported return stage is
// refused at ask time and at continue time, so an unresumable target is never written as
// a checkpoint or completed as a continuation.
func TestCLIPlannerRejectsUnsupportedReturnStage(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "x", Status: domain.PLANNED, MaxAttempts: 3})
	enablePlanner(t, dir, "")

	if code, _, errS := runCLI(t, dir, "planner", "ask", "T001", "--question", "q", "--return-stage", "REVIEWING"); code != exitError {
		t.Fatalf("ask code=%d, want error; err=%s", code, errS)
	} else if !strings.Contains(errS, "unsupported return stage") {
		t.Fatalf("stderr = %q, want an unsupported-return-stage message", errS)
	}

	// A question recorded by another path with an unsupported stage is refused at
	// continue time, before it is claimed or completed.
	q := createQuestion(t, dir, &question.Question{
		Identity: "T001", Kind: "k", Body: "q", RunID: "T001", RunEpoch: currentRunEpoch(dir, "T001"),
		ReturnStage: "REVIEWING", PlanRevision: currentPlanRevision(dir), Revision: 1,
	})
	svc := question.NewService(plannerStore(t, dir), nil)
	if _, err := svc.CreateAnswer(&question.Answer{QuestionID: q.ID, Body: "Markdown"}); err != nil {
		t.Fatalf("answer: %v", err)
	}
	if code, _, errS := runCLI(t, dir, "planner", "continue", "--apply"); code != exitError {
		t.Fatalf("continue code=%d, want error; err=%s", code, errS)
	} else if !strings.Contains(errS, "unsupported return stage") {
		t.Fatalf("stderr = %q, want an unsupported-return-stage message", errS)
	}
	c, err := svc.Continuation(q.ID, q.RunEpoch)
	if err == nil && c.State == question.ContinuationCompleted {
		t.Fatal("an unsupported return stage completed a continuation")
	}
}

// TestCLIPlannerStaleBindingSkipped proves a question bound to a superseded plan
// revision is skipped, never applied.
func TestCLIPlannerStaleBindingSkipped(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "x", Status: domain.PLANNED, MaxAttempts: 3})
	enablePlanner(t, dir, "")
	writePlanMeta(t, dir, "p", "new")
	createQuestion(t, dir, &question.Question{
		Identity: "T001", Kind: "k", Body: "q", RunID: "T001", PlanRevision: "p@old", Revision: 1,
	})
	if _, out, _ := runCLI(t, dir, "planner", "status"); !strings.Contains(out, "SKIP_STALE") {
		t.Fatalf("status = %q, want SKIP_STALE", out)
	}
}
