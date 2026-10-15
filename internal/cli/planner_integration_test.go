package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/github"
	"github.com/imhttran/agentic-sop/internal/question"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/store"
)

// scriptedPlannerAgent is a deterministic fake executor. On the i-th IMPLEMENT it
// either raises a structured planner request (asks[i] != nil) or completes with
// impls[i]. It records per-capability call counts and every request input.
type scriptedPlannerAgent struct {
	mu                   sync.Mutex
	planCalls, implCalls int
	inputs               []string
	asks                 []*agent.PlannerRequest
	impls                []string
}

func (a *scriptedPlannerAgent) Generate(_ context.Context, req agent.Request) (agent.Response, error) {
	a.mu.Lock()
	a.inputs = append(a.inputs, req.Input)
	a.mu.Unlock()
	switch req.Capability {
	case agent.Plan:
		a.mu.Lock()
		a.planCalls++
		a.mu.Unlock()
		return agent.Response{Content: validPlanJSON}, nil
	case agent.Implement:
		a.mu.Lock()
		i := a.implCalls
		a.implCalls++
		a.mu.Unlock()
		if i < len(a.asks) && a.asks[i] != nil {
			return agent.Response{Content: "asking planner", Outcome: &agent.Outcome{Status: agent.OutcomeNeedsPlanner, Planner: a.asks[i]}}, nil
		}
		content := "created x"
		if i < len(a.impls) && a.impls[i] != "" {
			content = a.impls[i]
		}
		return agent.Response{Content: content}, nil
	case agent.Review:
		return agent.Response{Content: `{"summary":"clean","findings":[]}`}, nil
	default:
		return agent.Response{Content: "ok"}, nil
	}
}

func (a *scriptedPlannerAgent) counts() (plan, impl int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.planCalls, a.implCalls
}

func (a *scriptedPlannerAgent) saw(sub string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return strings.Contains(strings.Join(a.inputs, "\n"), sub)
}

func plannerDeps(dir string, a agent.Agent, diff string) deps {
	return deps{
		getwd:     func() (string, error) { return dir, nil },
		newAgent:  func(string, string, string) (agent.Agent, error) { return a, nil },
		readDiff:  func(context.Context, string) (string, error) { return diff, nil },
		commit:    func(context.Context, string, string) error { return nil },
		newGitHub: func(string) github.Client { return &fakeGitHub{} },
	}
}

const plannerDiff = "diff --git a/x b/x\n--- a/x\n+++ b/x\n@@ -0,0 +1 @@\n+x\n"

// plannerResumePlanMD is a valid plan document for a parked run's plan.md, so a
// resume from IMPLEMENTING can load the prior plan and skip the PLAN stage.
const plannerResumePlanMD = `# Implementation Plan

## Project

demo

## Summary

Disposable planner resume demo.

## S001 — demo

Demonstrate the resumed planner continuation.

### Dependencies

None

### Deliverables

- demo

### Acceptance Criteria

- The task completes after the planner answer
`

func plannerProject(t *testing.T) (string, *store.Store, *domain.Task) {
	t.Helper()
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "demo", Status: domain.PLANNED, MaxAttempts: 3})
	writeConfig(t, dir, "project:\n  name: \"demo\"\nplanner:\n  enabled: true\nvalidation:\n  build: [\"true\"]\n  test: [\"true\"]\n")
	writePlanMeta(t, dir, "p", "h")
	if err := os.MkdirAll(filepath.Join(dir, config.DirName, "runs", "T001"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join(config.DirName, "runs", "T001", "state.json"),
		`{"id":"T001","stage":"CREATED","created_at":"2026-01-02T00:00:00Z"}`)
	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	task, err := st.Get("T001")
	if err != nil {
		t.Fatal(err)
	}
	return dir, st, task
}

func answer(t *testing.T, dir, questionID, body string) {
	t.Helper()
	if _, _, err := question.NewService(plannerStore(t, dir), nil).ReadQuestion(questionID); err != nil {
		t.Fatalf("read question: %v", err)
	}
	if _, err := question.NewService(plannerStore(t, dir), nil).CreateAnswer(&question.Answer{QuestionID: questionID, Body: body}); err != nil {
		t.Fatalf("answer: %v", err)
	}
}

// TestPlannerIntegrationStructuredAskParksAndResumes proves the executor raises a
// question DURING implementation (no pre-seeded question, no manual ask), the same run
// parks, and the applied answer resumes the SAME epoch, skipping the earlier PLAN stage.
func TestPlannerIntegrationStructuredAskParksAndResumes(t *testing.T) {
	dir, st, task := plannerProject(t)
	defer st.Close()
	cfg, _ := loadConfigOrDefault(dir)
	ask1 := &agent.PlannerRequest{Question: "which format?", Options: []string{"Markdown", "plain text"}}
	a := &scriptedPlannerAgent{asks: []*agent.PlannerRequest{ask1}}

	var out bytes.Buffer
	code := runScheduledTask(context.Background(), dir, cfg, a, plannerDeps(dir, a, plannerDiff), st, task, newRunSession(), &out, &out)
	if code == exitOK {
		t.Fatalf("the run should pause at WAITING_FOR_PLANNER: %s", out.String())
	}
	if got := runStage(t, dir, "T001"); got != "WAITING_FOR_PLANNER" {
		t.Fatalf("stage = %q, want WAITING_FOR_PLANNER", got)
	}
	if plan, _ := a.counts(); plan != 1 {
		t.Fatalf("PLAN calls = %d, want 1", plan)
	}
	q1 := pendingQuestion(t, dir)
	if q1.Body != "which format?" || len(q1.Options) != 2 {
		t.Fatalf("question = %+v", q1)
	}
	// The parked run's epoch is the one the question was bound to. The first,
	// non-resumed invocation went through runpkg.New, which rewrote the seeded
	// state.json with a fresh created_at, so the epoch must be read here.
	epoch := q1.RunEpoch

	// Seed attempts evidence AFTER the parking invocation. New clears attempts/ while
	// Resume preserves it, so its survival across the resume proves the resume path did
	// not restart the run with New (attempt accounting is preserved).
	attemptsDir := filepath.Join(dir, config.DirName, "runs", "T001", "attempts")
	if err := os.MkdirAll(attemptsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(attemptsDir, "sentinel.txt")
	if err := os.WriteFile(sentinel, []byte("kept"), 0o644); err != nil {
		t.Fatal(err)
	}

	answer(t, dir, q1.ID, "Markdown")
	out.Reset()
	code = runScheduledTask(context.Background(), dir, cfg, a, plannerDeps(dir, a, plannerDiff), st, task, newRunSession(), &out, &out)
	if code != exitOK {
		t.Fatalf("resume code=%d out=%s", code, out.String())
	}
	if _, err := os.Stat(sentinel); err != nil {
		t.Fatalf("the resume reset the attempts evidence (New was used, not Resume): %v", err)
	}
	if plan, impl := a.counts(); plan != 1 || impl != 2 {
		t.Fatalf("counts plan=%d impl=%d, want plan=1 (not repeated) impl=2", plan, impl)
	}
	if !a.saw("Planner decision: Markdown") {
		t.Fatalf("the answer did not reach the resumed stage: %v", a.inputs)
	}
	if currentRunEpoch(dir, "T001") != epoch {
		t.Fatal("the run epoch changed on resume")
	}
	cont, err := question.NewService(plannerStore(t, dir), nil).Continuation(q1.ID, epoch)
	if err != nil || cont.State != question.ContinuationCompleted {
		t.Fatalf("continuation = %+v err=%v, want COMPLETED", cont, err)
	}
}

// TestPlannerIntegrationTwoRoundsOneRun proves two planner rounds within ONE run: the
// executor asks, answers resume the SAME epoch, the executor asks again, and the task
// finally passes. PLAN is not repeated.
func TestPlannerIntegrationTwoRoundsOneRun(t *testing.T) {
	dir, st, task := plannerProject(t)
	defer st.Close()
	cfg, _ := loadConfigOrDefault(dir)
	ask1 := &agent.PlannerRequest{Question: "which format?", Options: []string{"Markdown", "plain text"}}
	ask2 := &agent.PlannerRequest{Question: "uppercase too?", Options: []string{"yes", "no"}}
	a := &scriptedPlannerAgent{asks: []*agent.PlannerRequest{ask1, ask2, nil}}

	run := func() (int, string) {
		var out bytes.Buffer
		code := runScheduledTask(context.Background(), dir, cfg, a, plannerDeps(dir, a, plannerDiff), st, task, newRunSession(), &out, &out)
		return code, out.String()
	}

	// Round 1: ask during implementation.
	if code, o := run(); code == exitOK {
		t.Fatalf("round 1 should pause: %s", o)
	}
	q1 := pendingQuestion(t, dir)
	// The parked run's epoch is the one the question was bound to (New rewrote the
	// seeded state on the first, non-resumed invocation).
	epoch := q1.RunEpoch
	answer(t, dir, q1.ID, "Markdown")
	// Round 1 resume: the resumed stage asks AGAIN.
	if code, o := run(); code == exitOK {
		t.Fatalf("round 1 resume should pause again: %s", o)
	}
	if currentRunEpoch(dir, "T001") != epoch {
		t.Fatal("the run epoch changed between rounds")
	}
	answer(t, dir, pendingQuestion(t, dir).ID, "yes")
	// Round 2 resume: the task finally passes.
	if code, o := run(); code != exitOK {
		t.Fatalf("round 2 resume code=%d out=%s", code, o)
	}
	if plan, impl := a.counts(); plan != 1 || impl != 3 {
		t.Fatalf("counts plan=%d impl=%d, want plan=1 (never repeated) impl=3", plan, impl)
	}
	if !a.saw("Planner decision: Markdown") || !a.saw("Planner decision: yes") {
		t.Fatalf("both answers must reach execution: %v", a.inputs)
	}
	if currentRunEpoch(dir, "T001") != epoch {
		t.Fatal("the run epoch changed across the two rounds")
	}
}

// TestPlannerIntegrationStageLabelAloneCannotPass proves the completion boundary: with
// no successful stage work, the continuation is not completed.
func TestPlannerIntegrationStageLabelAloneCannotPass(t *testing.T) {
	dir, st, task := plannerProject(t)
	defer st.Close()
	cfg, _ := loadConfigOrDefault(dir)
	ask1 := &agent.PlannerRequest{Question: "q?", Options: []string{"a", "b"}}
	a := &scriptedPlannerAgent{asks: []*agent.PlannerRequest{ask1}, impls: []string{"", "no changes"}}

	var out bytes.Buffer
	runScheduledTask(context.Background(), dir, cfg, a, plannerDeps(dir, a, ""), st, task, newRunSession(), &out, &out)
	q1 := pendingQuestion(t, dir)
	answer(t, dir, q1.ID, "a")
	out.Reset()
	runScheduledTask(context.Background(), dir, cfg, a, plannerDeps(dir, a, ""), st, task, newRunSession(), &out, &out)
	cont, err := question.NewService(plannerStore(t, dir), nil).Continuation(q1.ID, q1.RunEpoch)
	if err != nil {
		t.Fatal(err)
	}
	if cont.State == question.ContinuationCompleted {
		t.Fatal("the continuation completed without successful stage work")
	}
}

// TestPlannerRecoveryGapEffectMarker proves the crash gap is recovered without
// replaying the effect: with the effect marker present and completion not persisted, a
// resume records completion without executing the executor again.
func TestPlannerRecoveryGapEffectMarker(t *testing.T) {
	dir, st, task := plannerProject(t)
	defer st.Close()
	epoch := currentRunEpoch(dir, "T001")
	svc := question.NewService(plannerStore(t, dir), nil)
	q, err := svc.CreateQuestion(&question.Question{
		Identity: "T001", Kind: "k", Body: "q?", Options: []string{"a", "b"},
		Project: projectIdentity(dir), RunID: "T001", RunEpoch: epoch,
		PlanRevision: currentPlanRevision(dir), ReturnStage: "IMPLEMENTING", Revision: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateAnswer(&question.Answer{QuestionID: q.ID, Body: "a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ApplyAnswer(context.Background(), q.ID); err != nil {
		t.Fatal(err)
	}
	// Simulate: the effect succeeded in a prior attempt, but completion was not persisted.
	if err := writePlannerEffect(dir, "T001", question.ContinuationID(q.ID, epoch), 1); err != nil {
		t.Fatal(err)
	}

	a := &scriptedPlannerAgent{}
	cfg, _ := loadConfigOrDefault(dir)
	var out bytes.Buffer
	handled, code := plannerLifecycleGate(context.Background(), dir, cfg, a, plannerDeps(dir, a, plannerDiff), st, task, newRunSession(), &out, &out)
	if !handled || code != exitOK {
		t.Fatalf("effect-marker recovery: handled=%v code=%d out=%s", handled, code, out.String())
	}
	if _, impl := a.counts(); impl != 0 {
		t.Fatalf("the executor re-ran (impl calls=%d) despite the effect marker", impl)
	}
	cont, err := svc.Continuation(q.ID, epoch)
	if err != nil || cont.State != question.ContinuationCompleted {
		t.Fatalf("continuation = %+v err=%v, want COMPLETED without replay", cont, err)
	}
}

// TestPlannerIntegrationHumanGateBlocks proves a human gate is not handled as a planner
// continuation and the executor does not run across it.
func TestPlannerIntegrationHumanGateBlocks(t *testing.T) {
	dir, st, task := plannerProject(t)
	defer st.Close()
	epoch := currentRunEpoch(dir, "T001")
	svc := question.NewService(plannerStore(t, dir), nil)
	if _, err := svc.CreateQuestion(&question.Question{
		Identity: "T001", Kind: "k", Body: "q?", Project: projectIdentity(dir), RunID: "T001",
		RunEpoch: epoch, PlanRevision: currentPlanRevision(dir), ReturnStage: "IMPLEMENTING", Revision: 1,
	}); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join(config.DirName, "runs", "T001", "state.json"),
		`{"id":"T001","stage":"WAITING_FOR_HUMAN","created_at":"2026-01-02T00:00:00Z"}`)
	cfg, _ := loadConfigOrDefault(dir)
	a := &scriptedPlannerAgent{}
	var out bytes.Buffer
	handled, _ := plannerLifecycleGate(context.Background(), dir, cfg, a, plannerDeps(dir, a, plannerDiff), st, task, newRunSession(), &out, &out)
	if handled {
		t.Fatal("a human gate must not be handled as a planner continuation")
	}
	if _, impl := a.counts(); impl != 0 {
		t.Fatal("the executor ran across a human gate")
	}
}

// TestPlannerRecoveryUncertainExecutionStopsForReview proves the uncertain-execution gap
// is handled explicitly: a prior resumed attempt BEGAN (its durable start marker exists)
// and may have completed its effect, but the SUCCESS marker was never persisted. Safe
// replay cannot be established, so recovery must neither replay the effect nor mark
// completion — it stops for human review.
func TestPlannerRecoveryUncertainExecutionStopsForReview(t *testing.T) {
	dir, st, task := plannerProject(t)
	defer st.Close()
	cfg, _ := loadConfigOrDefault(dir)
	epoch := currentRunEpoch(dir, "T001")
	svc := question.NewService(plannerStore(t, dir), nil)
	q := createQuestion(t, dir, &question.Question{
		Identity: "T001", Kind: "k", Body: "q?", Project: projectIdentity(dir), RunID: "T001",
		RunEpoch: epoch, PlanRevision: currentPlanRevision(dir), ReturnStage: "IMPLEMENTING", Revision: 1,
	})
	if _, err := svc.CreateAnswer(&question.Answer{QuestionID: q.ID, Body: "a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ApplyAnswer(context.Background(), q.ID); err != nil {
		t.Fatal(err)
	}
	contID := question.ContinuationID(q.ID, epoch)

	// A prior attempt began execution (its start marker is durable) and the effect occurred
	// (an observable repository edit), but the success marker was never persisted.
	if err := writePlannerEffectStarted(dir, "T001", contID); err != nil {
		t.Fatal(err)
	}
	effect := filepath.Join(dir, "effect.txt")
	if err := os.WriteFile(effect, []byte("done"), 0o644); err != nil {
		t.Fatal(err)
	}

	a := &scriptedPlannerAgent{}
	var out bytes.Buffer
	handled, code := plannerLifecycleGate(context.Background(), dir, cfg, a, plannerDeps(dir, a, plannerDiff), st, task, newRunSession(), &out, &out)
	if !handled || code != exitError {
		t.Fatalf("uncertain recovery handled=%v code=%d out=%s", handled, code, out.String())
	}
	if _, impl := a.counts(); impl != 0 {
		t.Fatal("the executor replayed an effect of uncertain completion")
	}
	c, err := svc.Continuation(q.ID, epoch)
	if err != nil {
		t.Fatal(err)
	}
	if c.State == question.ContinuationCompleted {
		t.Fatal("completion was recorded for uncertain execution")
	}
	if got := runStage(t, dir, "T001"); got != "WAITING_FOR_HUMAN" {
		t.Fatalf("stage = %q, want WAITING_FOR_HUMAN (stopped for review)", got)
	}
	req, ok := runpkg.At(runpkg.Dir(dir, "T001")).Approval()
	if !ok || req.Status != domain.ApprovalPending {
		t.Fatalf("no human review boundary recorded: %+v ok=%v", req, ok)
	}
	if b, err := os.ReadFile(effect); err != nil || string(b) != "done" {
		t.Fatalf("the prior effect was disturbed: %q err=%v", b, err)
	}
}

// TestPlannerResumeStandsDownWithoutOwnership proves the ownership ordering: execution
// ownership is acquired BEFORE the checkpoint is modified, so a worker that cannot obtain
// it leaves the run checkpoint unchanged and does not claim, execute, or complete.
func TestPlannerResumeStandsDownWithoutOwnership(t *testing.T) {
	dir, st, task := plannerProject(t)
	defer st.Close()
	cfg, _ := loadConfigOrDefault(dir)
	epoch := currentRunEpoch(dir, "T001")
	svc := question.NewService(plannerStore(t, dir), nil)
	q := createQuestion(t, dir, &question.Question{
		Identity: "T001", Kind: "k", Body: "q?", Project: projectIdentity(dir), RunID: "T001",
		RunEpoch: epoch, PlanRevision: currentPlanRevision(dir), ReturnStage: "IMPLEMENTING", Revision: 1,
	})
	if _, err := svc.CreateAnswer(&question.Answer{QuestionID: q.ID, Body: "a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ApplyAnswer(context.Background(), q.ID); err != nil {
		t.Fatal(err)
	}
	contID := question.ContinuationID(q.ID, epoch)

	// Another worker already holds execution ownership for this continuation.
	release, held, err := question.TryAcquireLock(plannerEffectPath(dir, "T001", contID) + ".lock")
	if err != nil || !held {
		t.Fatalf("hold execution lock: held=%v err=%v", held, err)
	}
	defer release()

	before := runStage(t, dir, "T001")
	a := &scriptedPlannerAgent{}
	var out bytes.Buffer
	handled, code := plannerLifecycleGate(context.Background(), dir, cfg, a, plannerDeps(dir, a, plannerDiff), st, task, newRunSession(), &out, &out)
	if !handled || code != exitError {
		t.Fatalf("stand-down handled=%v code=%d out=%s", handled, code, out.String())
	}
	if _, impl := a.counts(); impl != 0 {
		t.Fatal("the executor ran without execution ownership")
	}
	if after := runStage(t, dir, "T001"); after != before {
		t.Fatalf("checkpoint changed without execution ownership: %q -> %q", before, after)
	}
	c, err := svc.Continuation(q.ID, epoch)
	if err != nil {
		t.Fatal(err)
	}
	if c.State == question.ContinuationCompleted {
		t.Fatal("a continuation completed without execution ownership")
	}
}

// appliedPlannerContinuation creates an APPLIED question and returns it with its
// continuation ID, bound to the project's current run epoch and plan revision.
func appliedPlannerContinuation(t *testing.T, dir string) (*question.Question, string) {
	t.Helper()
	epoch := currentRunEpoch(dir, "T001")
	svc := question.NewService(plannerStore(t, dir), nil)
	q := createQuestion(t, dir, &question.Question{
		Identity: "T001", Kind: "k", Body: "q?", Project: projectIdentity(dir), RunID: "T001",
		RunEpoch: epoch, PlanRevision: currentPlanRevision(dir), ReturnStage: "IMPLEMENTING", Revision: 1,
	})
	if _, err := svc.CreateAnswer(&question.Answer{QuestionID: q.ID, Body: "a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ApplyAnswer(context.Background(), q.ID); err != nil {
		t.Fatal(err)
	}
	return q, question.ContinuationID(q.ID, epoch)
}

// TestPlannerResumeStartMarkerWriteFailureDoesNotExecute proves that stage work does NOT
// begin when the durable start marker cannot be persisted: the failure is surfaced, the
// executor never runs, and no completion is recorded.
func TestPlannerResumeStartMarkerWriteFailureDoesNotExecute(t *testing.T) {
	dir, st, task := plannerProject(t)
	defer st.Close()
	cfg, _ := loadConfigOrDefault(dir)
	q, contID := appliedPlannerContinuation(t, dir)

	// Force the start-marker write to fail: the marker path exists as a directory, so the
	// atomic rename over it fails.
	if err := os.MkdirAll(plannerEffectStartedPath(dir, "T001", contID), 0o755); err != nil {
		t.Fatal(err)
	}

	a := &scriptedPlannerAgent{}
	var out bytes.Buffer
	handled, code := plannerLifecycleGate(context.Background(), dir, cfg, a, plannerDeps(dir, a, plannerDiff), st, task, newRunSession(), &out, &out)
	if !handled || code != exitError {
		t.Fatalf("marker-write failure handled=%v code=%d out=%s", handled, code, out.String())
	}
	if _, impl := a.counts(); impl != 0 {
		t.Fatal("stage work began despite a failed start-marker write")
	}
	c, err := question.NewService(plannerStore(t, dir), nil).Continuation(q.ID, q.RunEpoch)
	if err != nil {
		t.Fatal(err)
	}
	if c.State == question.ContinuationCompleted {
		t.Fatal("completion recorded despite a failed start-marker write")
	}
}

// TestPlannerRecoveryMalformedMarkersStopForReview proves a marker that is present but
// unreadable or malformed is treated as uncertain execution — never as "absent": recovery
// neither replays the effect nor records completion, and stops for human review.
func TestPlannerRecoveryMalformedMarkersStopForReview(t *testing.T) {
	cases := []struct {
		name  string
		write func(dir, contID string) error
	}{
		{"malformed success marker", func(dir, contID string) error {
			return os.WriteFile(plannerEffectPath(dir, "T001", contID), []byte("{not json"), 0o644)
		}},
		{"truncated success marker", func(dir, contID string) error {
			return os.WriteFile(plannerEffectPath(dir, "T001", contID), nil, 0o644)
		}},
		{"malformed start marker", func(dir, contID string) error {
			return os.WriteFile(plannerEffectStartedPath(dir, "T001", contID), []byte(`{"continuation_id"`), 0o644)
		}},
		{"truncated start marker", func(dir, contID string) error {
			return os.WriteFile(plannerEffectStartedPath(dir, "T001", contID), nil, 0o644)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir, st, task := plannerProject(t)
			defer st.Close()
			cfg, _ := loadConfigOrDefault(dir)
			q, contID := appliedPlannerContinuation(t, dir)
			if err := tc.write(dir, contID); err != nil {
				t.Fatal(err)
			}

			a := &scriptedPlannerAgent{}
			var out bytes.Buffer
			handled, code := plannerLifecycleGate(context.Background(), dir, cfg, a, plannerDeps(dir, a, plannerDiff), st, task, newRunSession(), &out, &out)
			if !handled || code != exitError {
				t.Fatalf("malformed marker handled=%v code=%d out=%s", handled, code, out.String())
			}
			if _, impl := a.counts(); impl != 0 {
				t.Fatal("the executor ran on a malformed marker (unsafe replay)")
			}
			c, err := question.NewService(plannerStore(t, dir), nil).Continuation(q.ID, q.RunEpoch)
			if err != nil {
				t.Fatal(err)
			}
			if c.State == question.ContinuationCompleted {
				t.Fatal("completion recorded on a malformed marker")
			}
			if got := runStage(t, dir, "T001"); got != "WAITING_FOR_HUMAN" {
				t.Fatalf("stage = %q, want WAITING_FOR_HUMAN (stopped for review)", got)
			}
		})
	}
}

// TestPlannerResumeCountsOneCompletion reproduces the live demo: a task parked at
// WAITING_FOR_PLANNER with an ANSWERED question must be finalized in the SAME scheduler
// pass and reported as a single completion. Before the fix the planner resume left the
// task runnable, so a second scheduler pass finished it and the run reported
// "2 completed this run" for one task.
func TestPlannerResumeCountsOneCompletion(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "demo", Status: domain.PLANNED, MaxAttempts: 3})
	writeConfig(t, dir, "project:\n  name: \"demo\"\nplanner:\n  enabled: true\nvalidation:\n  build: [\"true\"]\n  test: [\"true\"]\n")
	writePlanMeta(t, dir, "p", "h")

	// A run parked at WAITING_FOR_PLANNER, with the prior plan.md a resume needs.
	epoch := "2026-01-02T00:00:00Z"
	runDir := filepath.Join(config.DirName, "runs", "T001")
	if err := os.MkdirAll(filepath.Join(dir, runDir), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join(runDir, "state.json"),
		`{"id":"T001","stage":"WAITING_FOR_PLANNER","created_at":"`+epoch+`"}`)
	writeFile(t, dir, filepath.Join(runDir, "plan.md"), plannerResumePlanMD)

	// The executor's question, already answered, bound to this run's epoch.
	svc := question.NewService(plannerStore(t, dir), nil)
	q := createQuestion(t, dir, &question.Question{
		Identity: "T001", Kind: "k", Body: "q?", Project: projectIdentity(dir), RunID: "T001",
		RunEpoch: epoch, PlanRevision: currentPlanRevision(dir), ReturnStage: "IMPLEMENTING", Revision: 1,
	})
	if _, err := svc.CreateAnswer(&question.Answer{QuestionID: q.ID, Body: "Markdown"}); err != nil {
		t.Fatal(err)
	}

	// The resumed IMPLEMENT reports success; the injected diff supplies the observed
	// mutation, so the resumed run reaches PASSED and the task is finalized.
	a := &scriptedPlannerAgent{impls: []string{"created x"}}
	code, stdout, stderr := runInjectedCLI(t, dir, plannerDiff, a, "run")
	if code != exitOK {
		t.Fatalf("run code=%d\nstdout=%s\nstderr=%s", code, stdout, stderr)
	}
	if strings.Contains(stdout, "2 completed this run") {
		t.Fatalf("double counting one task:\n%s", stdout)
	}
	if !strings.Contains(stdout, "1 completed this run") {
		t.Fatalf("stdout = %q, want a single completion", stdout)
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
		t.Fatalf("task status = %s, want LOCAL_DONE", task.Status)
	}
	if c, err := svc.Continuation(q.ID, epoch); err != nil || c.State != question.ContinuationCompleted {
		t.Fatalf("continuation = %+v err=%v, want COMPLETED", c, err)
	}
	if plan, impl := a.counts(); plan != 0 || impl != 1 {
		t.Fatalf("counts plan=%d impl=%d, want plan=0 (PLAN not repeated) impl=1", plan, impl)
	}
}
