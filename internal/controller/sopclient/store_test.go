package sopclient

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE tasks (
  id TEXT PRIMARY KEY, title TEXT NOT NULL, objective TEXT, acceptance_criteria TEXT,
  status TEXT NOT NULL, blocked_reason TEXT, attempt INTEGER NOT NULL DEFAULT 0,
  max_attempts INTEGER NOT NULL DEFAULT 1, created_at TEXT NOT NULL, updated_at TEXT NOT NULL);
CREATE TABLE task_dependencies (
  task_id TEXT NOT NULL, dependency_task_id TEXT NOT NULL,
  PRIMARY KEY (task_id, dependency_task_id));
CREATE TABLE task_attempts (
  task_id TEXT NOT NULL, number INTEGER NOT NULL, status TEXT NOT NULL, reason TEXT,
  output TEXT, duration INTEGER NOT NULL DEFAULT 0, timestamp TEXT NOT NULL,
  PRIMARY KEY (task_id, number));
CREATE TABLE handoffs (
  task_id TEXT PRIMARY KEY, capsule_json TEXT NOT NULL, status TEXT NOT NULL,
  content TEXT, references_json TEXT, compression_error TEXT, created_at TEXT NOT NULL);
`

// newProject returns a temp project root with an empty SOP state database.
func newProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	initProjectAt(t, root, "demo")
	return root
}

// initProjectAt initializes an SOP project at dir with the given declared name:
// a .agent-sdlc/config.yaml and an empty state database with the SOP schema.
func initProjectAt(t *testing.T, dir, name string) {
	t.Helper()
	sdlc := filepath.Join(dir, ".agent-sdlc")
	if err := os.MkdirAll(filepath.Join(sdlc, "runs", "t2"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "project:\n  name: \"" + name + "\"\n  integration_branch: main\n"
	if err := os.WriteFile(filepath.Join(sdlc, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.Join(sdlc, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
}

func seed(t *testing.T, root string, stmts ...string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+StatePath(root))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("seed %q: %v", s, err)
		}
	}
}

// writeApprovals writes a fixture approval listing that readApprovals reads
// back, standing in for `sop approvals --json` stdout.
func writeApprovals(t *testing.T, root, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, ".agent-sdlc", "approvals.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// readApprovals decodes the fixture written by writeApprovals. No fixture is
// an unreported listing; a malformed one fails the test.
func readApprovals(t *testing.T, st *Store) ApprovalsListing {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(st.root, ".agent-sdlc", "approvals.json"))
	if err != nil {
		return ApprovalsListing{}
	}
	listing, ok := decodeApprovals(raw)
	if !ok {
		t.Fatalf("malformed approvals fixture: %s", raw)
	}
	return listing
}

func TestSummaryTasksAndBlocking(t *testing.T) {
	root := newProject(t)
	now := time.Now().UTC().Format(time.RFC3339)
	seed(t, root,
		`INSERT INTO tasks VALUES ('a','Task A','obj','ac','LOCAL_DONE',NULL,1,3,'`+now+`','`+now+`')`,
		`INSERT INTO tasks VALUES ('b','Task B','obj','ac','PLANNED',NULL,0,3,'`+now+`','`+now+`')`,
		`INSERT INTO tasks VALUES ('c','Task C','obj','ac','BLOCKED','REVIEW_UNRESOLVED',0,3,'`+now+`','`+now+`')`,
		`INSERT INTO task_dependencies VALUES ('b','a')`,
		`INSERT INTO task_dependencies VALUES ('c','b')`,
	)

	st, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	ctx := context.Background()
	// Summary folds in the SOP-reported changed set the caller supplies; with no
	// reported set (zero value) no changed-task count is added.
	sum, err := st.Summary(ctx, ChangedTasks{}, readApprovals(t, st))
	if err != nil {
		t.Fatal(err)
	}
	if sum.Name != "demo" || sum.Branch != "main" {
		t.Fatalf("meta = %q/%q, want demo/main", sum.Name, sum.Branch)
	}
	if sum.Total != 3 || sum.Completed != 1 || sum.Blocked != 1 || sum.Planned != 1 {
		t.Fatalf("summary counts wrong: %+v", sum)
	}
	if sum.PercentComplete() != 33 {
		t.Fatalf("percent = %d, want 33", sum.PercentComplete())
	}

	tasks, err := st.Tasks(ctx, readApprovals(t, st))
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]TaskSummary{}
	for _, task := range tasks {
		byID[task.ID] = task
	}
	if b := byID["b"]; len(b.BlockedBy) != 0 || !b.Eligible() {
		t.Fatalf("b should be eligible (dep a completed): %+v", b)
	}
	if c := byID["c"]; len(c.BlockedBy) != 1 || c.BlockedBy[0] != "b" || c.Eligible() {
		t.Fatalf("c should be blocked by b: %+v", c)
	}
}

// FixCycles is projected onto TaskSummary from the same report.json runMeta
// already reads Stage and Recovery from, and stays zero for a task with no
// run. Retryable/HasRetryableBlocked fold over Status/Attempt/MaxAttempts.
func TestFixCyclesAndRetryable(t *testing.T) {
	root := newProject(t)
	now := time.Now().UTC().Format(time.RFC3339)
	seed(t, root,
		// has a run reporting fix_cycles, BLOCKED with budget left -> retryable
		`INSERT INTO tasks VALUES ('a','A','o','a','BLOCKED',NULL,1,3,'`+now+`','`+now+`')`,
		// BLOCKED but budget spent -> not retryable
		`INSERT INTO tasks VALUES ('b','B','o','a','BLOCKED',NULL,3,3,'`+now+`','`+now+`')`,
		// not BLOCKED -> not retryable, no run -> FixCycles stays 0
		`INSERT INTO tasks VALUES ('c','C','o','a','READY',NULL,0,3,'`+now+`','`+now+`')`,
	)
	writeArtifact(t, root, "a", "report.json", `{"id":"a","stage":"FAILED","decision":"FAIL","fix_cycles":2}`)

	st, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	tasks, err := st.Tasks(context.Background(), readApprovals(t, st))
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]TaskSummary{}
	for _, task := range tasks {
		byID[task.ID] = task
	}

	if a := byID["a"]; a.FixCycles != 2 || !a.Retryable() {
		t.Fatalf("a: FixCycles=%d Retryable=%v, want 2/true: %+v", a.FixCycles, a.Retryable(), a)
	}
	if b := byID["b"]; b.Retryable() {
		t.Fatalf("b: Retryable=true, want false (budget spent): %+v", b)
	}
	if c := byID["c"]; c.FixCycles != 0 || c.Retryable() {
		t.Fatalf("c: FixCycles=%d Retryable=%v, want 0/false (no run, not blocked): %+v", c.FixCycles, c.Retryable(), c)
	}

	detail := ProjectDetail{Tasks: tasks}
	if !detail.HasRetryableBlocked() {
		t.Fatal("HasRetryableBlocked() = false, want true (a is retryable)")
	}
	detail.Tasks = []TaskSummary{byID["b"], byID["c"]}
	if detail.HasRetryableBlocked() {
		t.Fatal("HasRetryableBlocked() = true, want false (no retryable tasks)")
	}
}

// TestTasksProjectHumanDecisionBoundary verifies that the list projection
// (TaskSummary.NeedsHuman/ApprovalKind) and the detail projection
// (TaskDetail.Approval) both come from SOP's authoritative approval listing
// (sop approvals --json), and that no controller-side signal produces a gate:
// a BLOCKED task without a listing entry, a WAITING_FOR_HUMAN run stage, and a
// NEEDS_HUMAN classification are all NOT approvals on their own.
func TestTasksProjectHumanDecisionBoundary(t *testing.T) {
	root := newProject(t)
	now := time.Now().UTC().Format(time.RFC3339)
	seed(t, root,
		// BLOCKED only by an incomplete dependency: not a gate.
		`INSERT INTO tasks VALUES ('dep','Dep','o','a','BLOCKED','waiting on upstream',0,3,'`+now+`','`+now+`')`,
		// BLOCKED task SOP reported an applicable gate for.
		`INSERT INTO tasks VALUES ('blocked-human','BH','o','a','BLOCKED','needs a call',1,3,'`+now+`','`+now+`')`,
		// Run parked at WAITING_FOR_HUMAN, task status not itself BLOCKED: no
		// listing entry, so no gate.
		`INSERT INTO tasks VALUES ('waiting','W','o','a','IMPLEMENTING',NULL,1,3,'`+now+`','`+now+`')`,
		// NEEDS_HUMAN disposition reported independent of status/stage: no
		// listing entry, so no gate.
		`INSERT INTO tasks VALUES ('needs-human','NH','o','a','REVIEW',NULL,1,3,'`+now+`','`+now+`')`,
	)
	writeArtifact(t, root, "waiting", "state.json", `{"id":"waiting","stage":"WAITING_FOR_HUMAN"}`)
	writeArtifact(t, root, "needs-human", "classification.json",
		`{"kind":"TEST_FAILURE","disposition":"NEEDS_HUMAN","confidence":"HIGH","reason":"ask a human"}`)
	// Only blocked-human has an applicable listing entry.
	writeApprovals(t, root, `{"approvals":[
	  {"task_id":"blocked-human","kind":"CONTRACT","target":"blocked-human","reason":"ambiguous contract","evidence":"two contracts remain","stage":"WAITING_FOR_HUMAN","disposition":"NEEDS_HUMAN","status":"PENDING","requested_at":"2026-01-01T00:00:00Z","task_status":"BLOCKED"},
	  {"task_id":"waiting","kind":"CONTRACT","target":"waiting","reason":"resolved","evidence":"","stage":"PASSED","disposition":"RESOLVED","status":"RESOLVED","requested_at":"2026-01-01T00:00:00Z","task_status":"IMPLEMENTING"}
	]}`)

	st, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	tasks, err := st.Tasks(context.Background(), readApprovals(t, st))
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]TaskSummary{}
	for _, task := range tasks {
		byID[task.ID] = task
	}

	// No listing entry: never a gate, regardless of BLOCKED/stage/classification.
	if d := byID["dep"]; d.NeedsHuman {
		t.Fatalf("dep: NeedsHuman = true, want false (dependency-only BLOCKED): %+v", d)
	}
	if n := byID["needs-human"]; n.NeedsHuman {
		t.Fatalf("needs-human: NeedsHuman = true, want false (NEEDS_HUMAN classification alone): %+v", n)
	}
	// A resolved gate entry is never offered as actionable.
	if w := byID["waiting"]; w.NeedsHuman {
		t.Fatalf("waiting: NeedsHuman = true, want false (resolved listing entry): %+v", w)
	}
	// The applicable entry drives the list projection, carrying SOP's kind.
	if b := byID["blocked-human"]; !b.NeedsHuman || b.ApprovalKind != "CONTRACT" {
		t.Fatalf("blocked-human: NeedsHuman=%v ApprovalKind=%q, want true/CONTRACT: %+v", b.NeedsHuman, b.ApprovalKind, b)
	}

	// The detail view must agree with the list over the SAME listing.
	detail, err := st.Task(context.Background(), "blocked-human", readApprovals(t, st))
	if err != nil {
		t.Fatal(err)
	}
	if !detail.Approval.Present || detail.Approval.Kind != "CONTRACT" {
		t.Fatalf("TaskDetail.Approval = %+v, want Present with Kind CONTRACT", detail.Approval)
	}
	if detail.NeedsHuman() != detail.Approval.Present {
		t.Fatalf("TaskDetail.NeedsHuman()=%v disagrees with Approval.Present=%v", detail.NeedsHuman(), detail.Approval.Present)
	}
}

func TestTaskDetailReadsArtifacts(t *testing.T) {
	root := newProject(t)
	now := time.Now().UTC().Format(time.RFC3339)
	seed(t, root,
		`INSERT INTO tasks VALUES ('t2','Task Two','do the thing','works; tested','FIX_REQUIRED','build failed',2,3,'`+now+`','`+now+`')`,
		`INSERT INTO task_attempts VALUES ('t2',1,'FIX_REQUIRED','build failed','boom',1500000000,'`+now+`')`,
		`INSERT INTO handoffs VALUES ('t2','{}','COMPRESSED','carry me','[]',NULL,'`+now+`')`,
	)
	run := filepath.Join(root, ".agent-sdlc", "runs", "t2")
	if err := os.WriteFile(filepath.Join(run, "validation.json"),
		[]byte(`{"Status":"FAIL","Results":[{"Category":"BUILD","Command":"go build ./...","ExitCode":1,"Stderr":"nope","Status":"FAIL"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(run, "review.json"),
		[]byte(`{"Summary":"ok","Findings":[{"Severity":"high","Title":"thing","File":"x.go"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}

	st, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	d, err := st.Task(context.Background(), "t2", readApprovals(t, st))
	if err != nil {
		t.Fatal(err)
	}
	if d.Objective != "do the thing" || d.LatestFailure != "build failed" {
		t.Fatalf("task detail wrong: %+v", d)
	}
	if len(d.Validation) != 1 || d.Validation[0].Category != "BUILD" {
		t.Fatalf("validation not read: %+v", d.Validation)
	}
	if len(d.Review.Findings) != 1 || d.Review.Findings[0].Severity != "high" {
		t.Fatalf("review not read: %+v", d.Review)
	}
	if d.Handoff == nil || d.Handoff.Status != "COMPRESSED" {
		t.Fatalf("handoff not read: %+v", d.Handoff)
	}
}

func TestTaskNotFound(t *testing.T) {
	root := newProject(t)
	st, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := st.Task(context.Background(), "missing", readApprovals(t, st)); err != ErrTaskNotFound {
		t.Fatalf("err = %v, want ErrTaskNotFound", err)
	}
}
