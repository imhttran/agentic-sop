package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/question"
)

// These tests exercise the PRODUCTION question-creation paths (the executor raise and the
// CLI `planner ask`). They never seed Question.Deadline directly: every deadline asserted
// here is derived by the store from planner.timeout_seconds and the record's creation time.
//
// Expiry is evaluated only when SOP reads the task's planner state (command re-entry: the
// planner gate on `sop run`, or `sop continue`). There is no background timer.

func plannerConfigWithTimeout(timeout string) string {
	block := "planner:\n  enabled: true\n"
	if timeout != "" {
		block += "  timeout_seconds: " + timeout + "\n"
	}
	return "project:\n  name: \"demo\"\n" + block + "validation:\n  build: [\"true\"]\n  test: [\"true\"]\n"
}

// TestPlannerDeadlineDerivedOnExecutorRaise proves the executor-created question receives a
// deadline derived from planner.timeout_seconds, sharing one creation timestamp.
func TestPlannerDeadlineDerivedOnExecutorRaise(t *testing.T) {
	dir, st, task := plannerProject(t)
	defer st.Close()
	writeConfig(t, dir, plannerConfigWithTimeout("600"))
	cfg, _ := loadConfigOrDefault(dir)
	ask := &agent.PlannerRequest{Question: "which format?", Options: []string{"Markdown", "plain text"}}
	a := &scriptedPlannerAgent{asks: []*agent.PlannerRequest{ask}}

	var out bytes.Buffer
	if code := runScheduledTask(context.Background(), dir, cfg, a, plannerDeps(dir, a, plannerDiff), st, task, newRunSession(), &out, &out); code == exitOK {
		t.Fatalf("the run should pause: %s", out.String())
	}
	q := pendingQuestion(t, dir)
	if q.Deadline.IsZero() {
		t.Fatal("executor-raised question has no deadline")
	}
	if got := q.Deadline.Sub(q.CreatedAt); got != 600*time.Second {
		t.Fatalf("deadline - created_at = %v, want 600s (one creation timestamp)", got)
	}
}

// TestPlannerDeadlineDerivedOnCLIAsk proves the CLI-created question receives the same
// derived deadline.
func TestPlannerDeadlineDerivedOnCLIAsk(t *testing.T) {
	dir, st, _ := plannerProject(t)
	defer st.Close()
	writeConfig(t, dir, plannerConfigWithTimeout("120"))
	if code, _, errS := runCLI(t, dir, "planner", "ask", "T001", "--question", "which format?"); code != exitOK {
		t.Fatalf("ask code=%d err=%s", code, errS)
	}
	q := pendingQuestion(t, dir)
	if q.Deadline.IsZero() {
		t.Fatal("CLI-created question has no deadline")
	}
	if got := q.Deadline.Sub(q.CreatedAt); got != 120*time.Second {
		t.Fatalf("deadline - created_at = %v, want 120s", got)
	}
}

// TestPlannerZeroTimeoutMeansNoDeadline pins the zero-timeout policy: no deadline, so the
// question waits indefinitely (still parked, never auto-retried).
func TestPlannerZeroTimeoutMeansNoDeadline(t *testing.T) {
	dir, st, task := plannerProject(t)
	defer st.Close()
	writeConfig(t, dir, plannerConfigWithTimeout(""))
	cfg, _ := loadConfigOrDefault(dir)
	ask := &agent.PlannerRequest{Question: "which format?", Options: []string{"a", "b"}}
	a := &scriptedPlannerAgent{asks: []*agent.PlannerRequest{ask}}

	var out bytes.Buffer
	if code := runScheduledTask(context.Background(), dir, cfg, a, plannerDeps(dir, a, plannerDiff), st, task, newRunSession(), &out, &out); code == exitOK {
		t.Fatalf("the run should pause: %s", out.String())
	}
	if q := pendingQuestion(t, dir); !q.Deadline.IsZero() {
		t.Fatalf("zero timeout produced a deadline: %v", q.Deadline)
	}
}

// TestPlannerInvalidTimeoutRejected proves a negative or overflowing timeout is rejected
// when a question is created, and no question is recorded.
func TestPlannerInvalidTimeoutRejected(t *testing.T) {
	cases := []struct{ name, secs, want string }{
		{"negative", "-5", "must not be negative"},
		{"overflow", "9223372036854775807", "overflows a duration"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			initProject(t, dir)
			seedTask(t, dir, &domain.Task{ID: "T001", Title: "x", Status: domain.PLANNED, MaxAttempts: 3})
			writeConfig(t, dir, plannerConfigWithTimeout(tc.secs))
			if code, _, errS := runCLI(t, dir, "planner", "ask", "T001", "--question", "q"); code == exitOK {
				t.Fatalf("%s: expected rejection", tc.name)
			} else if !strings.Contains(errS, tc.want) {
				t.Fatalf("%s: stderr = %q, want %q", tc.name, errS, tc.want)
			}
			qs, err := question.NewService(plannerStore(t, dir), nil).ListQuestions(question.StatePending)
			if err != nil {
				t.Fatal(err)
			}
			if len(qs) != 0 {
				t.Fatalf("%s: created a question despite an invalid timeout", tc.name)
			}
		})
	}
}

// TestPlannerDeadlineNotExtendedOnReentry proves re-entry preserves the deadline (and its
// creation timestamp): reconciliation never extends a waiting question's deadline.
func TestPlannerDeadlineNotExtendedOnReentry(t *testing.T) {
	dir, st, task := plannerProject(t)
	defer st.Close()
	writeConfig(t, dir, plannerConfigWithTimeout("600"))
	cfg, _ := loadConfigOrDefault(dir)
	ask := &agent.PlannerRequest{Question: "which format?", Options: []string{"a", "b"}}
	a := &scriptedPlannerAgent{asks: []*agent.PlannerRequest{ask}}

	var out bytes.Buffer
	runScheduledTask(context.Background(), dir, cfg, a, plannerDeps(dir, a, plannerDiff), st, task, newRunSession(), &out, &out)
	first := pendingQuestion(t, dir)

	for i := 0; i < 2; i++ {
		out.Reset()
		plannerLifecycleGate(context.Background(), dir, cfg, a, plannerDeps(dir, a, plannerDiff), st, task, newRunSession(), &out, &out)
	}
	got := pendingQuestion(t, dir)
	if !got.CreatedAt.Equal(first.CreatedAt) || !got.Deadline.Equal(first.Deadline) {
		t.Fatalf("re-entry changed the record: created %v->%v deadline %v->%v",
			first.CreatedAt, got.CreatedAt, first.Deadline, got.Deadline)
	}
}

// TestPlannerTimeoutExpiryPausesWithEvidence proves that once the derived deadline passes,
// re-entry produces the configured pause outcome with evidence, and that clarification
// consumes no failure retry. Expiry is evaluated on this re-entry only (no timer).
func TestPlannerTimeoutExpiryPausesWithEvidence(t *testing.T) {
	dir, st, task := plannerProject(t)
	defer st.Close()
	writeConfig(t, dir, "project:\n  name: \"demo\"\nplanner:\n  enabled: true\n  timeout_seconds: 1\n  pause_on_timeout: true\nvalidation:\n  build: [\"true\"]\n  test: [\"true\"]\n")
	cfg, _ := loadConfigOrDefault(dir)
	ask := &agent.PlannerRequest{Question: "which format?", Options: []string{"a", "b"}}
	a := &scriptedPlannerAgent{asks: []*agent.PlannerRequest{ask}}

	var out bytes.Buffer
	runScheduledTask(context.Background(), dir, cfg, a, plannerDeps(dir, a, plannerDiff), st, task, newRunSession(), &out, &out)
	q := pendingQuestion(t, dir)
	if q.Deadline.IsZero() {
		t.Fatal("no deadline to expire")
	}

	// Wait past the derived deadline, then re-enter (the only place expiry is evaluated).
	time.Sleep(time.Until(q.Deadline) + 300*time.Millisecond)

	before, err := st.Get("T001")
	if err != nil {
		t.Fatal(err)
	}
	out.Reset()
	handled, code := plannerLifecycleGate(context.Background(), dir, cfg, a, plannerDeps(dir, a, plannerDiff), st, task, newRunSession(), &out, &out)
	if !handled || code != exitError {
		t.Fatalf("expiry: handled=%v code=%d out=%s", handled, code, out.String())
	}
	if !strings.Contains(out.String(), "deadline passed") {
		t.Fatalf("expiry evidence missing from %q", out.String())
	}
	if got := runStage(t, dir, "T001"); got != "WAITING_FOR_PLANNER" {
		t.Fatalf("stage = %q, want WAITING_FOR_PLANNER (paused)", got)
	}
	after, err := st.Get("T001")
	if err != nil {
		t.Fatal(err)
	}
	if after.Attempt != before.Attempt || after.MaxAttempts != before.MaxAttempts {
		t.Fatalf("clarification consumed a failure retry: attempt %d->%d max %d->%d",
			before.Attempt, after.Attempt, before.MaxAttempts, after.MaxAttempts)
	}
}

// TestPlannerResumeSavesHandoffCapsule proves a planner completion persists the RBV-011
// handoff capsule under context.handoff on BOTH success paths — normal success and
// success-marker recovery — and never when the flag is off.
func TestPlannerResumeSavesHandoffCapsule(t *testing.T) {
	cases := []struct {
		name     string
		recovery bool
		handoff  bool
	}{
		{"normal success, handoff on", false, true},
		{"normal success, handoff off", false, false},
		{"marker recovery, handoff on", true, true},
		{"marker recovery, handoff off", true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir, st, task := plannerProject(t)
			defer st.Close()
			handoffBlock := ""
			if tc.handoff {
				handoffBlock = "context:\n  handoff: true\n"
			}
			writeConfig(t, dir, "project:\n  name: \"demo\"\nplanner:\n  enabled: true\n"+handoffBlock+"validation:\n  build: [\"true\"]\n  test: [\"true\"]\n")
			cfg, _ := loadConfigOrDefault(dir)
			epoch := currentRunEpoch(dir, "T001")
			svc := question.NewService(plannerStore(t, dir), nil)
			q := createQuestion(t, dir, &question.Question{
				Identity: "T001", Kind: "k", Body: "q?", Project: projectIdentity(dir), RunID: "T001",
				RunEpoch: epoch, PlanRevision: currentPlanRevision(dir), ReturnStage: "IMPLEMENTING", Revision: 1,
			})
			if _, err := svc.CreateAnswer(&question.Answer{QuestionID: q.ID, Body: "Markdown"}); err != nil {
				t.Fatal(err)
			}
			if _, err := svc.ApplyAnswer(context.Background(), q.ID); err != nil {
				t.Fatal(err)
			}

			a := &scriptedPlannerAgent{}
			var out bytes.Buffer
			if tc.recovery {
				// A prior attempt's effect succeeded but its completion was not persisted: the
				// success-marker path completes without running the stage.
				if err := writePlannerEffect(dir, "T001", question.ContinuationID(q.ID, epoch), 1); err != nil {
					t.Fatal(err)
				}
				if handled, code := plannerLifecycleGate(context.Background(), dir, cfg, a, plannerDeps(dir, a, plannerDiff), st, task, newRunSession(), &out, &out); !handled || code != exitOK {
					t.Fatalf("marker recovery handled=%v code=%d out=%s", handled, code, out.String())
				}
			} else {
				// A resume from IMPLEMENTING loads the prior plan, so the parked run needs one.
				if err := os.MkdirAll(filepath.Join(dir, config.DirName, "runs", "T001"), 0o755); err != nil {
					t.Fatal(err)
				}
				writeFile(t, dir, filepath.Join(config.DirName, "runs", "T001", "plan.md"), plannerResumePlanMD)
				if code := runScheduledTask(context.Background(), dir, cfg, a, plannerDeps(dir, a, plannerDiff), st, task, newRunSession(), &out, &out); code != exitOK {
					t.Fatalf("resume code=%d out=%s", code, out.String())
				}
			}

			recs, err := st.Handoffs()
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, r := range recs {
				if r.TaskID == "T001" {
					found = true
				}
			}
			if found != tc.handoff {
				t.Fatalf("handoff capsule present=%v, want %v (records=%+v)", found, tc.handoff, recs)
			}
		})
	}
}

// TestPlannerRecoveryDefersToHeldExecutionOwnership proves recovery cannot supersede a
// predecessor while its worker still holds execution ownership: `sop planner ask` can
// persist a successor during a resumed stage, so ownership — not the successor check alone —
// decides. Once ownership is released, recovery proceeds.
func TestPlannerRecoveryDefersToHeldExecutionOwnership(t *testing.T) {
	dir, _, task := plannerProject(t)
	cfg, _ := loadConfigOrDefault(dir)
	epoch := currentRunEpoch(dir, "T001")
	svc := question.NewService(plannerStore(t, dir), nil)
	q1 := createQuestion(t, dir, &question.Question{
		Identity: "T001", Kind: "order", Body: "q1?", Project: projectIdentity(dir), RunID: "T001",
		RunEpoch: epoch, PlanRevision: currentPlanRevision(dir), ReturnStage: "IMPLEMENTING", Revision: 1,
	})
	if _, err := svc.CreateAnswer(&question.Answer{QuestionID: q1.ID, Body: "sorted"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ApplyAnswer(context.Background(), q1.ID); err != nil {
		t.Fatal(err)
	}
	contID := question.ContinuationID(q1.ID, epoch)
	if _, ok, err := svc.ClaimContinuation(contID, plannerWorker, plannerLease); err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}
	// The successor is persisted (for example `sop planner ask` during the resumed stage).
	createQuestion(t, dir, &question.Question{
		Identity: "T001", Kind: "duplicates", Body: "q2?", Project: projectIdentity(dir), RunID: "T001",
		RunEpoch: epoch, PlanRevision: currentPlanRevision(dir), ReturnStage: "IMPLEMENTING", Revision: 1,
	})

	// The predecessor's worker still holds execution ownership.
	release, ok, err := question.TryAcquireLock(plannerEffectPath(dir, "T001", contID) + ".lock")
	if err != nil || !ok {
		t.Fatalf("hold ownership: ok=%v err=%v", ok, err)
	}
	qs, err := questionsForTask(svc, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	plannerResolvePredecessors(svc, dir, cfg, task, qs)
	if c, err := svc.Continuation(q1.ID, epoch); err != nil || c.State != question.ContinuationClaimed {
		t.Fatalf("recovery superseded in-flight work: %+v err=%v, want CLAIMED", c, err)
	}

	// Once the owner releases, recovery resolves it.
	release()
	if qs, err = questionsForTask(svc, task.ID); err != nil {
		t.Fatal(err)
	}
	plannerResolvePredecessors(svc, dir, cfg, task, qs)
	if c, err := svc.Continuation(q1.ID, epoch); err != nil || c.State != question.ContinuationSuperseded {
		t.Fatalf("recovery did not resolve after release: %+v err=%v, want SUPERSEDED", c, err)
	}
}

// TestPlannerRecoveryLeavesActivePredecessorWhenNoSuccessor proves recovery cannot supersede
// a predecessor whose work may still be running: with no successor question, the CLAIMED
// continuation is left untouched (the durable-successor invariant).
func TestPlannerRecoveryLeavesActivePredecessorWhenNoSuccessor(t *testing.T) {
	dir, _, task := plannerProject(t)
	cfg, _ := loadConfigOrDefault(dir)
	epoch := currentRunEpoch(dir, "T001")
	svc := question.NewService(plannerStore(t, dir), nil)
	q1 := createQuestion(t, dir, &question.Question{
		Identity: "T001", Kind: "order", Body: "q1?", Project: projectIdentity(dir), RunID: "T001",
		RunEpoch: epoch, PlanRevision: currentPlanRevision(dir), ReturnStage: "IMPLEMENTING", Revision: 1,
	})
	if _, err := svc.CreateAnswer(&question.Answer{QuestionID: q1.ID, Body: "sorted"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ApplyAnswer(context.Background(), q1.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := svc.ClaimContinuation(question.ContinuationID(q1.ID, epoch), plannerWorker, plannerLease); err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}

	qs, err := questionsForTask(svc, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	plannerResolvePredecessors(svc, dir, cfg, task, qs)

	c1, err := svc.Continuation(q1.ID, epoch)
	if err != nil {
		t.Fatal(err)
	}
	if c1.State != question.ContinuationClaimed {
		t.Fatalf("recovery superseded an active predecessor with no successor: %s", c1.State)
	}
}
