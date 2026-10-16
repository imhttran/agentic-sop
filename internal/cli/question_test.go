package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/question"
)

// seedQuestion writes a durable question bound to taskID and planRev.
func seedQuestion(t *testing.T, dir, taskID, planRev string) *question.Question {
	t.Helper()
	return createQuestion(t, dir, &question.Question{
		Identity: taskID, Kind: "blocking", Body: "preserve input order or sort?", RunID: taskID, PlanRevision: planRev, Revision: 1,
	})
}

// createQuestion writes a durable question through the real service.
func createQuestion(t *testing.T, dir string, q *question.Question) *question.Question {
	t.Helper()
	fs, err := question.OpenFileStore(filepath.Join(dir, config.DirName, questionStoreName))
	if err != nil {
		t.Fatalf("open question store: %v", err)
	}
	got, err := question.NewService(fs, nil).CreateQuestion(q)
	if err != nil {
		t.Fatalf("create question: %v", err)
	}
	return got
}

// writePlanMeta records the plan provenance the binding check compares against.
func writePlanMeta(t *testing.T, dir, planID, sha string) {
	t.Helper()
	writeFile(t, dir, filepath.Join(config.DirName, planMetaName),
		fmt.Sprintf("{\"plan_id\":%q,\"source_sha256\":%q}", planID, sha))
}

func TestCLIQuestionAndAnswerAccepted(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "x", Status: domain.PLANNED, MaxAttempts: 3})
	writePlanMeta(t, dir, "plan-x", "abc")
	q := seedQuestion(t, dir, "T001", "plan-x@abc")

	if code, out, errS := runCLI(t, dir, "question", q.ID); code != exitOK {
		t.Fatalf("question code=%d err=%s", code, errS)
	} else if !strings.Contains(out, q.ID) || !strings.Contains(out, "PENDING") {
		t.Fatalf("question out=%q", out)
	}

	if code, out, errS := runCLI(t, dir, "answer", q.ID, "--body", "sort", "--by", "planner", "--idempotency-key", "k1"); code != exitOK {
		t.Fatalf("answer code=%d err=%s", code, errS)
	} else if !strings.Contains(out, "accepted") {
		t.Fatalf("answer out=%q", out)
	}

	// The question advanced to ANSWERED.
	if _, out, _ := runCLI(t, dir, "question", q.ID); !strings.Contains(out, "ANSWERED") {
		t.Fatalf("question not ANSWERED: %q", out)
	}
	// The task lifecycle is unchanged and no approval gate was created.
	if _, out, _ := runCLI(t, dir, "status"); !strings.Contains(out, "T001") || !strings.Contains(out, "PLANNED") {
		t.Fatalf("task changed: %q", out)
	}
	if _, out, _ := runCLI(t, dir, "approvals"); !strings.Contains(out, "no pending approvals") {
		t.Fatalf("approvals changed: %q", out)
	}
}

func TestCLIAnswerStaleBindingPreservesQuestion(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "x", Status: domain.PLANNED, MaxAttempts: 3})
	writePlanMeta(t, dir, "plan-x", "current")
	q := seedQuestion(t, dir, "T001", "plan-x@old") // bound to a superseded plan revision

	if code, out, _ := runCLI(t, dir, "answer", q.ID, "--body", "sort"); code != exitOK {
		t.Fatalf("code=%d", code)
	} else if !strings.Contains(out, "stale") {
		t.Fatalf("out=%q, want stale", out)
	}
	if _, out, _ := runCLI(t, dir, "question", q.ID); !strings.Contains(out, "PENDING") {
		t.Fatalf("stale answer advanced the question: %q", out)
	}
}

func TestCLIAnswerConflictRejected(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "x", Status: domain.PLANNED, MaxAttempts: 3})
	writePlanMeta(t, dir, "plan-x", "abc")
	q := seedQuestion(t, dir, "T001", "plan-x@abc")

	if _, out, _ := runCLI(t, dir, "answer", q.ID, "--body", "sort", "--idempotency-key", "k1"); !strings.Contains(out, "accepted") {
		t.Fatalf("first answer out=%q", out)
	}
	if _, out, _ := runCLI(t, dir, "answer", q.ID, "--body", "sort", "--idempotency-key", "k1"); !strings.Contains(out, "duplicate") {
		t.Fatalf("duplicate out=%q", out)
	}
	if _, out, _ := runCLI(t, dir, "answer", q.ID, "--body", "different", "--idempotency-key", "k2"); !strings.Contains(out, "rejected") {
		t.Fatalf("conflict out=%q", out)
	}
}

func TestCLIQuestionListDefaultsToPending(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "x", Status: domain.PLANNED, MaxAttempts: 3})
	seedQuestion(t, dir, "T001", "")

	if code, out, errS := runCLI(t, dir, "question", "list"); code != exitOK {
		t.Fatalf("list code=%d err=%s", code, errS)
	} else if !strings.Contains(out, "PENDING") {
		t.Fatalf("list out=%q", out)
	}
}

// TestCLIAnswerWrongProjectStale proves the project identity is validated against the
// live project, not trusted from the caller.
func TestCLIAnswerWrongProjectStale(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "x", Status: domain.PLANNED, MaxAttempts: 3})
	q := createQuestion(t, dir, &question.Question{Identity: "T001", Kind: "k", Body: "q", Project: "some-other-project", Revision: 1})

	if _, out, _ := runCLI(t, dir, "answer", q.ID, "--body", "sort"); !strings.Contains(out, "stale") || !strings.Contains(out, "wrong project") {
		t.Fatalf("out=%q, want a stale wrong-project refusal", out)
	}
	if _, out, _ := runCLI(t, dir, "question", q.ID); !strings.Contains(out, "PENDING") {
		t.Fatalf("stale answer advanced the question: %q", out)
	}
}

// TestCLIAnswerSupersededRunStale proves the bound run's continued eligibility is
// checked: a run epoch that no longer matches the task's run is stale.
func TestCLIAnswerSupersededRunStale(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "x", Status: domain.PLANNED, MaxAttempts: 3})
	// The task's current run, created now.
	runDir := filepath.Join(dir, config.DirName, "runs", "T001")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, filepath.Join(config.DirName, "runs", "T001", "state.json"),
		`{"id":"T001","stage":"WAITING_FOR_HUMAN","created_at":"2026-06-01T00:00:00Z"}`)
	q := createQuestion(t, dir, &question.Question{Identity: "T001", Kind: "k", Body: "q", RunID: "T001", RunEpoch: "2025-01-01T00:00:00Z", Revision: 1})

	if _, out, _ := runCLI(t, dir, "answer", q.ID, "--body", "sort"); !strings.Contains(out, "stale") || !strings.Contains(out, "superseded") {
		t.Fatalf("out=%q, want a stale superseded-run refusal", out)
	}
}

// TestCLIAnswerMissingRunStale proves a question bound to a run that is no longer
// present is stale rather than silently accepted.
func TestCLIAnswerMissingRunStale(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "x", Status: domain.PLANNED, MaxAttempts: 3})
	q := createQuestion(t, dir, &question.Question{Identity: "T001", Kind: "k", Body: "q", RunID: "T001", RunEpoch: "2025-01-01T00:00:00Z", Revision: 1})

	if _, out, _ := runCLI(t, dir, "answer", q.ID, "--body", "sort"); !strings.Contains(out, "stale") {
		t.Fatalf("out=%q, want stale for a missing run", out)
	}
}
