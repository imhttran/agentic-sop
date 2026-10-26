package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/question"
)

const extPlannerConfig = "project:\n  name: x\nplanner:\n  enabled: true\nvalidation:\n  build:\n    - \"true\"\n"

// setupExternalContinuation builds a repo + activated plan with the planner enabled, raises
// and answers a planner question for S001, and applies the answer so a durable continuation
// is owed. It binds a real run epoch. It returns the directory, HEAD, and the question.
func setupExternalContinuation(t *testing.T) (string, string, *question.Question) {
	return setupExternalContinuationFixture(t, true)
}

// setupExternalContinuationLegacy raises the same fixture WITHOUT a run epoch, so the
// question and its continuation both carry an empty run_epoch — the `cont:q-1:` shape.
func setupExternalContinuationLegacy(t *testing.T) (string, string, *question.Question) {
	return setupExternalContinuationFixture(t, false)
}

func setupExternalContinuationFixture(t *testing.T, bindEpoch bool) (string, string, *question.Question) {
	t.Helper()
	dir := t.TempDir()
	head := setupExtProject(t, dir, extPlannerConfig, extPlanTwo)

	// Bind a real run epoch before the question is raised, so the continuation carries a
	// non-empty run_epoch and the epoch guard is exercised rather than skipped.
	if bindEpoch {
		seedRunState(t, dir, "S001", "2026-01-01T00:00:00Z")
	}

	if code, _, errS := runCLI(t, dir, "planner", "ask", "S001",
		"--question", "Proceed with the merged implementation?",
		"--option", "yes", "--option", "no"); code != exitOK {
		t.Fatalf("ask: code=%d stderr=%s", code, errS)
	}
	q := pendingQuestion(t, dir)
	if code, _, errS := runCLI(t, dir, "answer", q.ID, "--body", "yes", "--idempotency-key", "k1"); code != exitOK {
		t.Fatalf("answer: code=%d stderr=%s", code, errS)
	}
	if code, out, errS := runCLI(t, dir, "planner", "continue", "--apply"); code != exitOK {
		t.Fatalf("apply: code=%d stdout=%s stderr=%s", code, out, errS)
	}
	return dir, head, q
}

// taskContinuations returns the continuations bound to a task, reloaded from disk.
func taskContinuations(t *testing.T, dir, taskID string) []*question.Continuation {
	t.Helper()
	cs, err := question.NewService(plannerStore(t, dir), nil).Continuations("")
	if err != nil {
		t.Fatalf("list continuations: %v", err)
	}
	var out []*question.Continuation
	for _, c := range cs {
		if c.TaskID == taskID {
			out = append(out, c)
		}
	}
	return out
}

// questionRecordCounts returns the number of preserved answer and outcome records.
func questionRecordCounts(t *testing.T, dir string) (answers, outcomes int) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, config.DirName, questionStoreName))
	if err != nil {
		t.Fatalf("read question store: %v", err)
	}
	var st struct {
		Answers  []json.RawMessage `json:"answers"`
		Outcomes []json.RawMessage `json:"outcomes"`
	}
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatalf("parse question store: %v", err)
	}
	return len(st.Answers), len(st.Outcomes)
}

// TestTaskCompleteExternalDischargesPlannerContinuation proves that an external
// completion, backed by a contained implementation commit, discharges the task's owed
// planner continuation as EXTERNALLY_COMPLETED — never COMPLETED — and preserves the
// question, answer, and outcome history.
func TestTaskCompleteExternalDischargesPlannerContinuation(t *testing.T) {
	dir, head, q := setupExternalContinuation(t)

	conts := taskContinuations(t, dir, "S001")
	if len(conts) != 1 {
		t.Fatalf("continuations for S001 = %d, want 1", len(conts))
	}
	if conts[0].State != question.ContinuationPending {
		t.Fatalf("continuation before = %s, want PENDING", conts[0].State)
	}
	answersBefore, outcomesBefore := questionRecordCounts(t, dir)

	code, out, errS := runCLI(t, dir, "task", "complete", "S001", "--external", "--commit", head)
	if code != exitOK {
		t.Fatalf("complete: code=%d stdout=%s stderr=%s", code, out, errS)
	}

	conts = taskContinuations(t, dir, "S001")
	if len(conts) != 1 {
		t.Fatalf("continuations for S001 after = %d, want 1", len(conts))
	}
	c := conts[0]
	if c.State != question.ContinuationExternallyCompleted {
		t.Errorf("continuation state = %s, want EXTERNALLY_COMPLETED", c.State)
	}
	if c.State == question.ContinuationCompleted {
		t.Error("a discharged continuation must never be recorded COMPLETED: no stage ran")
	}
	if !strings.Contains(c.Reason, head) {
		t.Errorf("reason %q does not record the implementation commit %s", c.Reason, head)
	}
	if !strings.Contains(c.Reason, "not executed") {
		t.Errorf("reason %q does not state that the owed stage was not executed", c.Reason)
	}
	if !strings.Contains(out, "EXTERNALLY_COMPLETED") {
		t.Errorf("stdout does not report the discharge: %s", out)
	}
	if got := taskStatus(t, dir, "S001"); got != domain.LOCAL_DONE {
		t.Errorf("S001 status = %s, want LOCAL_DONE", got)
	}

	// The invariant: a recorded completion leaves nothing actionable behind.
	for _, other := range taskContinuations(t, dir, "S001") {
		if !other.State.Terminal() {
			t.Errorf("continuation %s is still actionable after completion: %s", other.ID, other.State)
		}
	}

	// History is preserved verbatim: no answer or outcome was rewritten or added.
	answersAfter, outcomesAfter := questionRecordCounts(t, dir)
	if answersAfter != answersBefore || outcomesAfter != outcomesBefore {
		t.Errorf("record counts changed: answers %d->%d outcomes %d->%d",
			answersBefore, answersAfter, outcomesBefore, outcomesAfter)
	}
	svc := question.NewService(plannerStore(t, dir), nil)
	got, _, err := svc.ReadQuestion(q.ID)
	if err != nil {
		t.Fatalf("read question: %v", err)
	}
	if got.State != question.StateApplied {
		t.Errorf("question state = %s, want APPLIED (history preserved)", got.State)
	}
}

// TestTaskCompleteExternalLeavesStalePlanBinding proves a continuation whose question is
// bound to a different plan revision is NOT discharged, and is reported as skipped.
func TestTaskCompleteExternalLeavesStalePlanBinding(t *testing.T) {
	dir, head, _ := setupExternalContinuation(t)
	rewritePlanFingerprint(t, dir, "0000000000000000000000000000000000000000000000000000000000000000")

	code, out, errS := runCLI(t, dir, "task", "complete", "S001", "--external", "--commit", head)
	if code != exitOK {
		t.Fatalf("complete: code=%d stdout=%s stderr=%s", code, out, errS)
	}

	conts := taskContinuations(t, dir, "S001")
	if len(conts) != 1 || conts[0].State != question.ContinuationPending {
		t.Fatalf("stale plan binding was discharged: %+v", conts)
	}
	if !strings.Contains(out, "plan revision changed") {
		t.Errorf("stdout does not report the skipped plan binding: %s", out)
	}
	// The completion itself is still proven by repository evidence.
	if got := taskStatus(t, dir, "S001"); got != domain.LOCAL_DONE {
		t.Errorf("S001 status = %s, want LOCAL_DONE", got)
	}
}

// TestTaskCompleteExternalLeavesStaleRunEpoch proves a continuation bound to a superseded
// run epoch is NOT discharged.
func TestTaskCompleteExternalLeavesStaleRunEpoch(t *testing.T) {
	dir, head, _ := setupExternalContinuation(t)

	conts := taskContinuations(t, dir, "S001")
	if len(conts) != 1 {
		t.Fatalf("continuations = %d, want 1", len(conts))
	}
	if conts[0].RunEpoch == "" {
		t.Fatal("fixture did not bind a run epoch; the epoch guard would not be exercised")
	}
	rewriteRunEpoch(t, dir, "S001", "2000-01-01T00:00:00.000000Z")

	code, out, errS := runCLI(t, dir, "task", "complete", "S001", "--external", "--commit", head)
	if code != exitOK {
		t.Fatalf("complete: code=%d stdout=%s stderr=%s", code, out, errS)
	}
	conts = taskContinuations(t, dir, "S001")
	if len(conts) != 1 || conts[0].State != question.ContinuationPending {
		t.Fatalf("stale run epoch was discharged: %+v", conts)
	}
	if !strings.Contains(out, "run epoch changed") {
		t.Errorf("stdout does not report the skipped run epoch: %s", out)
	}
}

// TestTaskCompleteExternalLeavesOtherTaskContinuation proves reconciliation is scoped to
// the completed task and never touches another task's obligation.
func TestTaskCompleteExternalLeavesOtherTaskContinuation(t *testing.T) {
	dir, head, _ := setupExternalContinuation(t)

	if code, _, errS := runCLI(t, dir, "planner", "ask", "S002",
		"--question", "Proceed with the second stage?",
		"--option", "yes", "--option", "no"); code != exitOK {
		t.Fatalf("ask S002: code=%d stderr=%s", code, errS)
	}
	q2 := pendingQuestion(t, dir)
	if code, _, errS := runCLI(t, dir, "answer", q2.ID, "--body", "yes", "--idempotency-key", "k2"); code != exitOK {
		t.Fatalf("answer S002: code=%d stderr=%s", code, errS)
	}
	if code, out, errS := runCLI(t, dir, "planner", "continue", "--apply"); code != exitOK {
		t.Fatalf("apply S002: code=%d stdout=%s stderr=%s", code, out, errS)
	}

	code, out, errS := runCLI(t, dir, "task", "complete", "S001", "--external", "--commit", head)
	if code != exitOK {
		t.Fatalf("complete S001: code=%d stdout=%s stderr=%s", code, out, errS)
	}

	if got := taskContinuations(t, dir, "S001"); len(got) != 1 || got[0].State != question.ContinuationExternallyCompleted {
		t.Errorf("S001 continuation = %+v, want EXTERNALLY_COMPLETED", got)
	}
	if got := taskContinuations(t, dir, "S002"); len(got) != 1 || got[0].State != question.ContinuationPending {
		t.Errorf("S002 continuation = %+v, want untouched PENDING", got)
	}
}

// TestTaskCompleteExternalRefusesUncontainedCommit keeps the pre-existing commit binding
// check honest: a commit that HEAD does not contain cannot complete the task, so it also
// cannot discharge a continuation.
func TestTaskCompleteExternalRefusesUncontainedCommit(t *testing.T) {
	dir, _, _ := setupExternalContinuation(t)

	code, _, errS := runCLI(t, dir, "task", "complete", "S001", "--external", "--commit", "0123456789abcdef0123456789abcdef01234567")
	if code != exitError {
		t.Fatalf("complete with uncontained commit: code=%d, want error; stderr=%s", code, errS)
	}
	if got := taskContinuations(t, dir, "S001"); len(got) != 1 || got[0].State != question.ContinuationPending {
		t.Errorf("continuation must stay PENDING when the commit is not contained: %+v", got)
	}
	if got := taskStatus(t, dir, "S001"); got == domain.LOCAL_DONE {
		t.Errorf("S001 must not be completed from an uncontained commit")
	}
}

// TestTaskCompleteExternalDischargesLegacyEmptyEpoch covers the legacy binding rule: when
// neither the continuation nor its question recorded a run epoch, the plan binding is the
// remaining proof and the record is discharged. This is the `cont:q-1:` shape.
func TestTaskCompleteExternalDischargesLegacyEmptyEpoch(t *testing.T) {
	dir, head, _ := setupExternalContinuationLegacy(t)

	conts := taskContinuations(t, dir, "S001")
	if len(conts) != 1 {
		t.Fatalf("continuations = %d, want 1", len(conts))
	}
	if conts[0].RunEpoch != "" {
		t.Fatalf("fixture bound an epoch (%q); the legacy rule would not be exercised", conts[0].RunEpoch)
	}

	code, out, errS := runCLI(t, dir, "task", "complete", "S001", "--external", "--commit", head)
	if code != exitOK {
		t.Fatalf("complete: code=%d stdout=%s stderr=%s", code, out, errS)
	}
	conts = taskContinuations(t, dir, "S001")
	if len(conts) != 1 || conts[0].State != question.ContinuationExternallyCompleted {
		t.Errorf("legacy continuation = %+v, want EXTERNALLY_COMPLETED", conts)
	}
	if got := taskStatus(t, dir, "S001"); got != domain.LOCAL_DONE {
		t.Errorf("S001 status = %s, want LOCAL_DONE", got)
	}
}

// TestTaskCompleteExternalFailsWhenContinuationHeld proves a live worker blocks external
// completion: the completion is not recorded and the continuation stays owed. Releasing the
// lock lets a retry succeed, so the refusal is recoverable rather than terminal.
func TestTaskCompleteExternalFailsWhenContinuationHeld(t *testing.T) {
	dir, head, _ := setupExternalContinuation(t)
	conts := taskContinuations(t, dir, "S001")
	if len(conts) != 1 {
		t.Fatalf("continuations = %d, want 1", len(conts))
	}
	c := conts[0]

	release, ok, err := question.TryAcquireLock(plannerEffectPath(dir, "S001", c.ID) + ".lock")
	if err != nil || !ok {
		t.Fatalf("acquire continuation lock: ok=%v err=%v", ok, err)
	}

	code, _, errS := runCLI(t, dir, "task", "complete", "S001", "--external", "--commit", head)
	if code != exitError {
		t.Fatalf("complete with a held continuation: code=%d, want error; stderr=%s", code, errS)
	}
	if !strings.Contains(errS, "held by another worker") {
		t.Errorf("stderr = %q, want a held-worker refusal", errS)
	}
	if got := taskStatus(t, dir, "S001"); got == domain.LOCAL_DONE {
		t.Error("completion must not be recorded while a continuation is still actionable")
	}
	if got := taskContinuations(t, dir, "S001"); len(got) != 1 || got[0].State != question.ContinuationPending {
		t.Errorf("continuation = %+v, want still PENDING", got)
	}

	release()
	code, out, errS := runCLI(t, dir, "task", "complete", "S001", "--external", "--commit", head)
	if code != exitOK {
		t.Fatalf("retry after release: code=%d stdout=%s stderr=%s", code, out, errS)
	}
	if got := taskContinuations(t, dir, "S001"); len(got) != 1 || got[0].State != question.ContinuationExternallyCompleted {
		t.Errorf("after retry = %+v, want EXTERNALLY_COMPLETED", got)
	}
	if got := taskStatus(t, dir, "S001"); got != domain.LOCAL_DONE {
		t.Errorf("after retry status = %s, want LOCAL_DONE", got)
	}
}

// TestTaskCompleteExternalRetryAfterPartialDischargeIsSafe simulates an earlier attempt that
// discharged the obligation but stopped before recording the completion. The retry must
// complete the task and must not rewrite the already-terminal record.
func TestTaskCompleteExternalRetryAfterPartialDischargeIsSafe(t *testing.T) {
	dir, head, _ := setupExternalContinuation(t)
	conts := taskContinuations(t, dir, "S001")
	if len(conts) != 1 {
		t.Fatalf("continuations = %d, want 1", len(conts))
	}
	c := conts[0]

	svc := question.NewService(plannerStore(t, dir), nil)
	if _, ok, err := svc.DischargeExternalContinuation(c.ID, "discharged by an earlier attempt"); err != nil || !ok {
		t.Fatalf("pre-discharge: ok=%v err=%v", ok, err)
	}
	if got := taskStatus(t, dir, "S001"); got == domain.LOCAL_DONE {
		t.Fatal("the task was completed before the retry")
	}

	code, out, errS := runCLI(t, dir, "task", "complete", "S001", "--external", "--commit", head)
	if code != exitOK {
		t.Fatalf("retry: code=%d stdout=%s stderr=%s", code, out, errS)
	}
	if got := taskStatus(t, dir, "S001"); got != domain.LOCAL_DONE {
		t.Errorf("status = %s, want LOCAL_DONE", got)
	}
	got := taskContinuations(t, dir, "S001")
	if len(got) != 1 || got[0].State != question.ContinuationExternallyCompleted {
		t.Fatalf("after retry = %+v, want EXTERNALLY_COMPLETED", got)
	}
	if got[0].Reason != "discharged by an earlier attempt" {
		t.Errorf("retry rewrote the terminal record: reason = %q", got[0].Reason)
	}
}

// TestExternalContinuationVerdict pins the binding gate: only a fully matching record is
// dischargeable, a provable mismatch is stale, and an unprovable one is ambiguous.
func TestExternalContinuationVerdict(t *testing.T) {
	const (
		taskID    = "S1"
		livePlan  = "plan@abc"
		liveEpoch = "2026-01-01T00:00:00Z"
	)
	cases := []struct {
		name            string
		qIdentity       string
		qPlan           string
		qEpoch          string
		cEpoch          string
		livePlan        string
		liveEpoch       string
		missingQuestion bool
		want            continuationVerdict
	}{
		{name: "fully matching", qIdentity: taskID, qPlan: livePlan, qEpoch: liveEpoch, cEpoch: liveEpoch, livePlan: livePlan, liveEpoch: liveEpoch, want: continuationDischargeable},
		{name: "legacy both epochs empty", qIdentity: taskID, qPlan: livePlan, qEpoch: "", cEpoch: "", livePlan: livePlan, liveEpoch: liveEpoch, want: continuationDischargeable},
		{name: "epoch bound only on the question", qIdentity: taskID, qPlan: livePlan, qEpoch: liveEpoch, cEpoch: "", livePlan: livePlan, liveEpoch: liveEpoch, want: continuationDischargeable},
		{name: "question bound to another task", qIdentity: "OTHER", qPlan: livePlan, qEpoch: liveEpoch, cEpoch: liveEpoch, livePlan: livePlan, liveEpoch: liveEpoch, want: continuationStale},
		{name: "plan revision mismatch", qIdentity: taskID, qPlan: "plan@zzz", qEpoch: liveEpoch, cEpoch: liveEpoch, livePlan: livePlan, liveEpoch: liveEpoch, want: continuationStale},
		{name: "epoch disagrees with question", qIdentity: taskID, qPlan: livePlan, qEpoch: liveEpoch, cEpoch: "2025-01-01T00:00:00Z", livePlan: livePlan, liveEpoch: liveEpoch, want: continuationStale},
		{name: "epoch mismatch with live run", qIdentity: taskID, qPlan: livePlan, qEpoch: liveEpoch, cEpoch: liveEpoch, livePlan: livePlan, liveEpoch: "2027-01-01T00:00:00Z", want: continuationStale},
		{name: "question has no plan binding", qIdentity: taskID, qPlan: "", qEpoch: liveEpoch, cEpoch: liveEpoch, livePlan: livePlan, liveEpoch: liveEpoch, want: continuationAmbiguous},
		{name: "live plan binding missing", qIdentity: taskID, qPlan: livePlan, qEpoch: liveEpoch, cEpoch: liveEpoch, livePlan: "", liveEpoch: liveEpoch, want: continuationAmbiguous},
		{name: "live epoch binding missing", qIdentity: taskID, qPlan: livePlan, qEpoch: liveEpoch, cEpoch: liveEpoch, livePlan: livePlan, liveEpoch: "", want: continuationAmbiguous},
		// A question with an empty identity is rejected by CreateQuestion, so the matching
		// guard in externalContinuationVerdict is defensive only and is not reachable here.
		{name: "bound question is unreadable", qIdentity: taskID, qPlan: livePlan, qEpoch: liveEpoch, cEpoch: liveEpoch, livePlan: livePlan, liveEpoch: liveEpoch, missingQuestion: true, want: continuationAmbiguous},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := question.NewService(question.NewMemoryStore(), nil)
			q, err := svc.CreateQuestion(&question.Question{
				Identity: tc.qIdentity, Kind: "k", Body: "b",
				PlanRevision: tc.qPlan, RunEpoch: tc.qEpoch, ReturnStage: "IMPLEMENTING",
			})
			if err != nil {
				t.Fatalf("create question: %v", err)
			}
			c := &question.Continuation{
				ID: "cont:test", QuestionID: q.ID, TaskID: taskID,
				RunEpoch: tc.cEpoch, ReturnStage: "IMPLEMENTING", State: question.ContinuationPending,
			}
			if tc.missingQuestion {
				c.QuestionID = "q-absent"
			}
			got, reason := externalContinuationVerdict(svc, &domain.Task{ID: taskID}, c, tc.livePlan, tc.liveEpoch)
			if got != tc.want {
				t.Fatalf("verdict = %v (%s), want %v", got, reason, tc.want)
			}
			if got != continuationDischargeable && strings.TrimSpace(reason) == "" {
				t.Error("a rejected verdict must explain why")
			}
		})
	}
}

// seedRunState writes a run state with a fixed created_at so a planner question raised
// afterwards binds that run epoch.
func seedRunState(t *testing.T, dir, taskID, createdAt string) {
	t.Helper()
	path := filepath.Join(dir, config.DirName, "runs", taskID)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("create run dir: %v", err)
	}
	st := map[string]any{
		"id":         taskID,
		"stage":      "PLANNED",
		"created_at": createdAt,
		"updated_at": createdAt,
	}
	data, err := json.Marshal(st)
	if err != nil {
		t.Fatalf("encode run state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(path, "state.json"), data, 0o644); err != nil {
		t.Fatalf("write run state: %v", err)
	}
}

// rewritePlanFingerprint changes the live plan fingerprint so a question's bound plan
// revision no longer matches it.
func rewritePlanFingerprint(t *testing.T, dir, sha string) {
	t.Helper()
	path := filepath.Join(dir, config.DirName, planMetaName)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read plan meta: %v", err)
	}
	var meta map[string]any
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatalf("parse plan meta: %v", err)
	}
	meta["source_sha256"] = sha
	out, err := json.Marshal(meta)
	if err != nil {
		t.Fatalf("encode plan meta: %v", err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatalf("write plan meta: %v", err)
	}
}

// rewriteRunEpoch changes a task's live run epoch so a bound continuation reads as stale.
func rewriteRunEpoch(t *testing.T, dir, taskID, createdAt string) {
	t.Helper()
	path := filepath.Join(dir, config.DirName, "runs", taskID, "state.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read run state: %v", err)
	}
	var st map[string]any
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatalf("parse run state: %v", err)
	}
	st["created_at"] = createdAt
	out, err := json.Marshal(st)
	if err != nil {
		t.Fatalf("encode run state: %v", err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatalf("write run state: %v", err)
	}
}
