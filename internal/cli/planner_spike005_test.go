package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/question"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/store"
)

// This file holds the SPIKE-005 (restart, timeout, stale-state, duplicate, authority)
// tests. It adds only the cases the plan's acceptance criteria do not already cover.

// TestPlannerTimeoutParksWithoutRetry proves an expired deadline (with pause_on_timeout)
// preserves an explicit paused condition: the run parks at WAITING_FOR_PLANNER, the
// question stays PENDING (no auto-retry), and no execution is dispatched.
func TestPlannerTimeoutParksWithoutRetry(t *testing.T) {
	dir, st, task := plannerProject(t)
	defer st.Close()
	writeConfig(t, dir, "project:\n  name: \"demo\"\nplanner:\n  enabled: true\n  pause_on_timeout: true\nvalidation:\n  build: [\"true\"]\n  test: [\"true\"]\n")
	cfg, _ := loadConfigOrDefault(dir)
	epoch := currentRunEpoch(dir, "T001")
	svc := question.NewService(plannerStore(t, dir), nil)
	if _, err := svc.CreateQuestion(&question.Question{
		Identity: "T001", Kind: "k", Body: "q?", Project: projectIdentity(dir), RunID: "T001",
		RunEpoch: epoch, PlanRevision: currentPlanRevision(dir), ReturnStage: "IMPLEMENTING", Revision: 1,
		Deadline: time.Now().Add(-time.Minute),
	}); err != nil {
		t.Fatal(err)
	}

	a := &scriptedPlannerAgent{}
	var out bytes.Buffer
	handled, code := plannerLifecycleGate(context.Background(), dir, cfg, a, plannerDeps(dir, a, plannerDiff), st, task, newRunSession(), &out, &out)
	if !handled || code != exitError {
		t.Fatalf("timeout: handled=%v code=%d out=%s", handled, code, out.String())
	}
	if _, impl := a.counts(); impl != 0 {
		t.Fatal("a timed-out question dispatched execution")
	}
	if got := runStage(t, dir, "T001"); got != "WAITING_FOR_PLANNER" {
		t.Fatalf("stage = %q, want WAITING_FOR_PLANNER (paused)", got)
	}
	if qs, _ := svc.ListQuestions(question.StatePending); len(qs) != 1 {
		t.Fatalf("pending questions = %d, want 1 (paused, not retried)", len(qs))
	}
}

// TestPlannerRestartWhileWaitingReparks proves a restart while waiting retains the
// question and re-parks: two re-entries neither dispatch the pending question nor change
// the run epoch.
func TestPlannerRestartWhileWaitingReparks(t *testing.T) {
	dir, st, task := plannerProject(t)
	defer st.Close()
	cfg, _ := loadConfigOrDefault(dir)
	epoch := currentRunEpoch(dir, "T001")
	createQuestion(t, dir, &question.Question{
		Identity: "T001", Kind: "k", Body: "q?", Project: projectIdentity(dir), RunID: "T001",
		RunEpoch: epoch, PlanRevision: currentPlanRevision(dir), ReturnStage: "IMPLEMENTING", Revision: 1,
	})

	a := &scriptedPlannerAgent{}
	for i := 0; i < 2; i++ {
		var out bytes.Buffer
		handled, code := plannerLifecycleGate(context.Background(), dir, cfg, a, plannerDeps(dir, a, plannerDiff), st, task, newRunSession(), &out, &out)
		if !handled || code != exitError {
			t.Fatalf("restart %d: handled=%v code=%d out=%s", i, handled, code, out.String())
		}
		if got := runStage(t, dir, "T001"); got != "WAITING_FOR_PLANNER" {
			t.Fatalf("restart %d: stage = %q, want WAITING_FOR_PLANNER", i, got)
		}
		if got := currentRunEpoch(dir, "T001"); got != epoch {
			t.Fatalf("restart %d: epoch changed %q -> %q", i, epoch, got)
		}
	}
	if _, impl := a.counts(); impl != 0 {
		t.Fatal("a restart dispatched execution for a still-pending question")
	}
}

// TestPlannerExhaustedRoundsParkAtHumanBoundary proves an exhausted planner round limit
// preserves an explicit paused/human boundary with evidence instead of falling through to
// the normal lifecycle (which would retry the task) or looping.
func TestPlannerExhaustedRoundsParkAtHumanBoundary(t *testing.T) {
	dir, st, task := plannerProject(t)
	defer st.Close()
	writeConfig(t, dir, "project:\n  name: \"demo\"\nplanner:\n  enabled: true\n  round_limit: 1\nvalidation:\n  build: [\"true\"]\n  test: [\"true\"]\n")
	cfg, _ := loadConfigOrDefault(dir)
	epoch := currentRunEpoch(dir, "T001")
	svc := question.NewService(plannerStore(t, dir), nil)
	// Two questions for the task exceed round_limit: 1.
	for i := 1; i <= 2; i++ {
		if _, err := svc.CreateQuestion(&question.Question{
			Identity: "T001", Kind: fmt.Sprintf("k%d", i), Body: "q?", Project: projectIdentity(dir), RunID: "T001",
			RunEpoch: epoch, PlanRevision: currentPlanRevision(dir), ReturnStage: "IMPLEMENTING", Revision: 1,
		}); err != nil {
			t.Fatal(err)
		}
	}

	a := &scriptedPlannerAgent{}
	var out bytes.Buffer
	handled, code := plannerLifecycleGate(context.Background(), dir, cfg, a, plannerDeps(dir, a, plannerDiff), st, task, newRunSession(), &out, &out)
	if !handled || code != exitError {
		t.Fatalf("exhausted: handled=%v code=%d out=%s", handled, code, out.String())
	}
	if _, impl := a.counts(); impl != 0 {
		t.Fatal("an exhausted planner round dispatched execution instead of pausing")
	}
	if got := runStage(t, dir, "T001"); got != "WAITING_FOR_HUMAN" {
		t.Fatalf("stage = %q, want WAITING_FOR_HUMAN (explicit paused boundary)", got)
	}
	if req, ok := runpkg.At(runpkg.Dir(dir, "T001")).Approval(); !ok || req.Status != domain.ApprovalPending {
		t.Fatalf("no human boundary recorded for exhausted rounds: %+v ok=%v", req, ok)
	}
}

// TestPlannerAnswerCannotSatisfyHumanGate proves a planner answer is never an approval,
// regardless of its text: on a human-gated task an approval-shaped answer is recorded as
// ANSWERED only, the approval request stays PENDING, and the planner does not act.
func TestPlannerAnswerCannotSatisfyHumanGate(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "x", Status: domain.PLANNED, MaxAttempts: 3})
	writeConfig(t, dir, "project:\n  name: \"demo\"\nplanner:\n  enabled: true\nvalidation:\n  build: [\"true\"]\n  test: [\"true\"]\n")
	writePlanMeta(t, dir, "p", "h")
	seedGate(t, dir, "T001") // parks the task with a pending human approval request

	q := createQuestion(t, dir, &question.Question{
		Identity: "T001", Kind: "k", Body: "q?", Project: projectIdentity(dir), RunID: "T001",
		RunEpoch: currentRunEpoch(dir, "T001"), PlanRevision: currentPlanRevision(dir), ReturnStage: "IMPLEMENTING", Revision: 1,
	})

	// An approval-shaped planner answer is recorded (never applied as an approval).
	if code, out, errS := runCLI(t, dir, "answer", q.ID, "--body", "APPROVED - proceed with the change", "--idempotency-key", "k1"); code != exitOK {
		t.Fatalf("answer code=%d err=%s out=%s", code, errS, out)
	}
	if _, out, _ := runCLI(t, dir, "question", q.ID); !strings.Contains(out, "ANSWERED") {
		t.Fatalf("question not ANSWERED: %q", out)
	}
	// The human gate still owns the task: the approval stays pending and listed.
	if req, ok := runpkg.At(runpkg.Dir(dir, "T001")).Approval(); !ok || req.Status != domain.ApprovalPending {
		t.Fatalf("a planner answer changed the approval: %+v ok=%v", req, ok)
	}
	if _, out, _ := runCLI(t, dir, "approvals"); !strings.Contains(out, "T001") {
		t.Fatalf("the human gate is no longer listed: %q", out)
	}
	// The planner does not act across the human gate.
	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	task, err := st.Get("T001")
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := loadConfigOrDefault(dir)
	a := &scriptedPlannerAgent{}
	var out bytes.Buffer
	if handled, _ := plannerLifecycleGate(context.Background(), dir, cfg, a, plannerDeps(dir, a, plannerDiff), st, task, newRunSession(), &out, &out); handled {
		t.Fatal("the planner acted on a human-gated task")
	}
}

// TestPlannerClarificationPreservesFailureBudget proves a clarification consumes a planner
// round, not a failure retry: a park leaves the task's attempt counters and attempt
// evidence unchanged and consumes no continuation budget.
func TestPlannerClarificationPreservesFailureBudget(t *testing.T) {
	dir, st, task := plannerProject(t)
	defer st.Close()
	attemptsDir := filepath.Join(dir, config.DirName, "runs", "T001", "attempts")
	if err := os.MkdirAll(attemptsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(attemptsDir, "sentinel")
	if err := os.WriteFile(sentinel, []byte("kept"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, _ := loadConfigOrDefault(dir)
	epoch := currentRunEpoch(dir, "T001")
	createQuestion(t, dir, &question.Question{
		Identity: "T001", Kind: "k", Body: "q?", Project: projectIdentity(dir), RunID: "T001",
		RunEpoch: epoch, PlanRevision: currentPlanRevision(dir), ReturnStage: "IMPLEMENTING", Revision: 1,
	})

	a := &scriptedPlannerAgent{}
	var out bytes.Buffer
	if handled, _ := plannerLifecycleGate(context.Background(), dir, cfg, a, plannerDeps(dir, a, plannerDiff), st, task, newRunSession(), &out, &out); !handled {
		t.Fatalf("expected a planner park: %s", out.String())
	}
	got, err := st.Get("T001")
	if err != nil {
		t.Fatal(err)
	}
	if got.Attempt != task.Attempt || got.MaxAttempts != task.MaxAttempts {
		t.Fatalf("failure budget changed by clarification: attempt %d->%d max %d->%d", task.Attempt, got.Attempt, task.MaxAttempts, got.MaxAttempts)
	}
	if _, err := os.Stat(sentinel); err != nil {
		t.Fatalf("attempt evidence was reset by a planner park: %v", err)
	}
	if n := runpkg.At(runpkg.Dir(dir, "T001")).ReadContinuations(); n != 0 {
		t.Fatalf("clarification consumed %d continuation(s), want 0", n)
	}
}

// TestPlannerDisabledIgnoresExistingQuestion proves disabled mode preserves prior execution
// behavior: with the planner off, a leftover question neither parks nor alters the run, and
// the task executes normally.
func TestPlannerDisabledIgnoresExistingQuestion(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "x", Status: domain.PLANNED, MaxAttempts: 3})
	// No planner block: the feature is disabled.
	writeConfig(t, dir, "project:\n  name: \"demo\"\nvalidation:\n  build: [\"true\"]\n  test: [\"true\"]\n")
	writePlanMeta(t, dir, "p", "h")
	q := createQuestion(t, dir, &question.Question{
		Identity: "T001", Kind: "k", Body: "leftover?", Project: projectIdentity(dir), RunID: "T001",
		RunEpoch: currentRunEpoch(dir, "T001"), PlanRevision: currentPlanRevision(dir), ReturnStage: "IMPLEMENTING", Revision: 1,
	})

	a := &scriptedPlannerAgent{impls: []string{"created x"}}
	code, stdout, stderr := runInjectedCLI(t, dir, plannerDiff, a, "run")
	if code != exitOK {
		t.Fatalf("run code=%d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	if strings.Contains(stdout, "WAITING_FOR_PLANNER") {
		t.Fatalf("disabled planner parked the task:\n%s", stdout)
	}
	if _, out, _ := runCLI(t, dir, "question", q.ID); !strings.Contains(out, "PENDING") {
		t.Fatalf("disabled planner changed the question: %q", out)
	}
	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	task, err := st.Get("T001")
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != domain.LOCAL_DONE {
		t.Fatalf("task status = %s, want LOCAL_DONE (normal execution)", task.Status)
	}
}
