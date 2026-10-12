package sopclient

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"sop-controller/internal/config"
)

// listingDocJSON builds a SOP `--list-changed --json` document carrying the given
// changed_executed ids; other keys are present but empty.
func listingDocJSON(ids ...string) string {
	quoted := make([]string, len(ids))
	for i, id := range ids {
		quoted[i] = `"` + id + `"`
	}
	return `{"version":1,"source":"docs/PLAN.md","plan_id":"p","plan_changed":true,` +
		`"unchanged":[],"updated":[],"added":[],"removed":[],` +
		`"changed_executed":[` + strings.Join(quoted, ",") + `],` +
		`"removed_executed":[],"auto_reconciled":[]}`
}

// Every action delegates to SOP for a known project and reports the
// not-found sentinel for an unknown one.
func TestActionsRejectUnknownProject(t *testing.T) {
	root := newProject(t)
	// An active plan + reported changed set, so AcceptChangedTask reaches SOP
	// rather than stopping at a precondition sentinel.
	if err := os.WriteFile(filepath.Join(root, ".agent-sdlc", "plan.meta.json"),
		[]byte(`{"source":"docs/PLAN.md"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	bin, _ := fakeListingSop(t, listingDocJSON("t2"))
	c, err := New([]string{root}, bin, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	pid := config.ProjectID(root)
	ctx := context.Background()

	if err := c.AcceptChangedTask(ctx, pid, "t2"); err != nil {
		t.Errorf("AcceptChangedTask err = %v, want nil (delegated to SOP)", err)
	}
	if err := c.ApproveTask(ctx, "nope", "t2", DecisionOptions{}); !errors.Is(err, ErrProjectNotFound) {
		t.Errorf("ApproveTask(unknown project) err = %v, want ErrProjectNotFound", err)
	}
	if err := c.DeclineTask(ctx, "nope", "t2", DecisionOptions{}); !errors.Is(err, ErrProjectNotFound) {
		t.Errorf("DeclineTask(unknown project) err = %v, want ErrProjectNotFound", err)
	}
	if err := c.AcceptChangedTask(ctx, "nope", "t2"); !errors.Is(err, ErrProjectNotFound) {
		t.Errorf("AcceptChangedTask(unknown project) err = %v, want ErrProjectNotFound", err)
	}
}

// AcceptChangedTask rejects an unknown/unrelated task, an unreported changed
// set, and a missing active plan, each with its own distinct sentinel, before
// ever reaching the action itself. These checks run in a fixed order (plan, then
// reported-set, then membership) so the first violated precondition is always
// what the caller sees.
func TestAcceptChangedTaskRejectsInvalidApprovals(t *testing.T) {
	ctx := context.Background()

	t.Run("no active plan", func(t *testing.T) {
		root := newProject(t)
		bin, _ := fakeListingSop(t, listingDocJSON("t2"))
		c, err := New([]string{root}, bin, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		if err := c.AcceptChangedTask(ctx, config.ProjectID(root), "t2"); !errors.Is(err, ErrNoActivePlan) {
			t.Errorf("AcceptChangedTask err = %v, want ErrNoActivePlan", err)
		}
	})

	t.Run("changed set not reported", func(t *testing.T) {
		root := newProject(t)
		if err := os.WriteFile(filepath.Join(root, ".agent-sdlc", "plan.meta.json"),
			[]byte(`{"source":"docs/PLAN.md"}`), 0o644); err != nil {
			t.Fatal(err)
		}
		// A failed listing is surfaced as an error, never an empty set.
		bin := fakeFailingSop(t)
		c, err := New([]string{root}, bin, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		if err := c.AcceptChangedTask(ctx, config.ProjectID(root), "t2"); err == nil {
			t.Errorf("AcceptChangedTask err = nil, want a surfaced listing failure")
		}
	})

	t.Run("task not in reported set", func(t *testing.T) {
		root := newProject(t)
		if err := os.WriteFile(filepath.Join(root, ".agent-sdlc", "plan.meta.json"),
			[]byte(`{"source":"docs/PLAN.md"}`), 0o644); err != nil {
			t.Fatal(err)
		}
		bin, _ := fakeListingSop(t, listingDocJSON("t2"))
		c, err := New([]string{root}, bin, time.Minute)
		if err != nil {
			t.Fatal(err)
		}
		defer c.Close()
		if err := c.AcceptChangedTask(ctx, config.ProjectID(root), "unrelated-task"); !errors.Is(err, ErrTaskNotInChangedSet) {
			t.Errorf("AcceptChangedTask err = %v, want ErrTaskNotInChangedSet", err)
		}
	})
}

func TestReadOperationsReportSOPValues(t *testing.T) {
	root := newProject(t)
	now := time.Now().UTC().Format(time.RFC3339)
	seed(t, root,
		`INSERT INTO tasks VALUES ('t1','Task One','obj','ac','LOCAL_DONE',NULL,1,3,'`+now+`','`+now+`')`,
		`INSERT INTO tasks VALUES ('t2','Task Two','obj','ac','BLOCKED','REVIEW_UNRESOLVED',0,3,'`+now+`','`+now+`')`,
		`INSERT INTO tasks VALUES ('t3','Task Three','obj','ac','PLANNED',NULL,0,3,'`+now+`','`+now+`')`,
		`INSERT INTO task_dependencies VALUES ('t3','t2')`,
	)
	writeArtifact(t, root, "t2", "activity.jsonl",
		`{"stage":"IMPLEMENT","action":"implementing","timestamp":"2026-01-01T00:00:01Z"}`+"\n")
	if err := os.WriteFile(filepath.Join(root, ".agent-sdlc", "plan.meta.json"),
		[]byte(`{"source":"docs/PLAN.md","plan_id":"p"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	c, err := New([]string{root}, "sop", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	pid := config.ProjectID(root)
	ctx := context.Background()

	// GetPlan
	if src, ok := c.PlanSource(pid); !ok || src != "docs/PLAN.md" {
		t.Errorf("GetPlan -> PlanSource = %q,%v, want docs/PLAN.md,true", src, ok)
	}

	// GetTasks + GetTaskProgress
	proj, err := c.Project(ctx, pid)
	if err != nil {
		t.Fatal(err)
	}
	if len(proj.Tasks) != 3 {
		t.Fatalf("GetTasks -> %d tasks, want 3", len(proj.Tasks))
	}
	if proj.Summary.Total != 3 || proj.Summary.Completed != 1 || proj.Summary.PercentComplete() != 33 {
		t.Errorf("GetTaskProgress summary wrong: %+v", proj.Summary)
	}

	// GetTask
	task, err := c.Task(ctx, pid, "t2")
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != StatusBlocked || task.BlockedReason != "REVIEW_UNRESOLVED" {
		t.Errorf("GetTask status = %q/%q, want BLOCKED/REVIEW_UNRESOLVED", task.Status, task.BlockedReason)
	}
	// t3 depends on the non-terminal t2, so t3 is blocked by it.
	blocked, err := c.Task(ctx, pid, "t3")
	if err != nil {
		t.Fatal(err)
	}
	if len(blocked.BlockedBy) != 1 || blocked.BlockedBy[0] != "t2" {
		t.Errorf("GetTask(t3) blockedBy = %v, want [t2]", blocked.BlockedBy)
	}

	// GetTaskActivity
	if ev := task.Activity; len(ev) != 1 || ev[0].Stage != "IMPLEMENT" {
		t.Errorf("GetTaskActivity = %+v, want one IMPLEMENT event", ev)
	}

	// Not-found cases must use the boundary sentinels, not a controller guess.
	if _, err := c.Project(ctx, "nope"); !errors.Is(err, ErrProjectNotFound) {
		t.Errorf("Project(unknown) err = %v, want ErrProjectNotFound", err)
	}
	if _, err := c.Task(ctx, pid, "missing"); !errors.Is(err, ErrTaskNotFound) {
		t.Errorf("Task(unknown) err = %v, want ErrTaskNotFound", err)
	}
}

// The command operations delegate to SOP's own CLI verbs; the controller runs no
// command logic itself.
func TestCommandOperationsDelegateToSOP(t *testing.T) {
	root := newProject(t)
	bin, args := fakeSop(t)
	c, err := New([]string{root}, bin, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	pid := config.ProjectID(root)
	ctx := context.Background()

	cases := []struct {
		name string
		verb string
		run  func() error
	}{
		{"start-or-continue-run (run)", "run", func() error { return c.Run(ctx, pid) }},
		{"start-or-continue-run (resume)", "resume", func() error { return c.Resume(ctx, pid) }},
		{"retry-task", "retry", func() error { return c.Retry(ctx, pid, "t2") }},
		{"get-task-report", "report", func() error { _, err := c.ReportTask(ctx, pid, "t2"); return err }},
		{"reconcile-plan", "reconcile", func() error { _, err := c.Reconcile(ctx, pid, "docs/PLAN.md"); return err }},
		{"approve-task", "approve", func() error { return c.ApproveTask(ctx, pid, "t2", DecisionOptions{}) }},
		{"decline-task", "decline", func() error { return c.DeclineTask(ctx, pid, "t2", DecisionOptions{}) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); err != nil {
				t.Fatalf("run: %v", err)
			}
			got := args()
			if len(got) == 0 || got[0] != tc.verb {
				t.Fatalf("sop argv = %v, want verb %q", got, tc.verb)
			}
		})
	}
}

// The authoritative approval listing read delegates to `sop approvals --json`
// through the CLI boundary; the controller parses no `sop approval` human text.
func TestApprovalsDelegatesToSOP(t *testing.T) {
	root := newProject(t)
	bin, args := fakeSop(t)
	c, err := New([]string{root}, bin, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	pid := config.ProjectID(root)
	ctx := context.Background()

	// fakeSop exits 0 with empty output: a well-formed empty listing, not an error.
	listing, err := c.Approvals(ctx, pid)
	if err != nil {
		t.Fatalf("Approvals: %v", err)
	}
	if len(listing.Entries) != 0 {
		t.Fatalf("listing = %+v, want no entries from empty fake output", listing)
	}
	if got := args(); !equal(got, []string{"approvals", "--json"}) {
		t.Fatalf("sop argv = %v, want [approvals --json]", got)
	}

	if _, err := c.Approvals(ctx, "nope"); !errors.Is(err, ErrProjectNotFound) {
		t.Errorf("Approvals(unknown project) err = %v, want ErrProjectNotFound", err)
	}
}

// No boundary operation writes SOP persistence: reads report SOP state and
// commands delegate to the sop CLI, so the controller-side call itself leaves
// state.db and the run artifacts byte-for-byte unchanged.
func TestBoundaryDoesNotMutateSOPPersistence(t *testing.T) {
	root := newProject(t)
	now := time.Now().UTC().Format(time.RFC3339)
	seed(t, root, `INSERT INTO tasks VALUES ('t2','Task Two','obj','ac','BLOCKED','REVIEW_UNRESOLVED',1,3,'`+now+`','`+now+`')`)
	writeArtifact(t, root, "t2", "state.json", `{"id":"t2","stage":"WAITING_FOR_HUMAN"}`)
	bin, _ := fakeListingSop(t, listingDocJSON("t2"))
	c, err := New([]string{root}, bin, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	pid := config.ProjectID(root)
	ctx := context.Background()

	dir := filepath.Join(root, ".agent-sdlc")
	before := snapshotTree(t, dir)

	_, _ = c.Projects(ctx)
	_, _ = c.Project(ctx, pid)
	_, _ = c.Task(ctx, pid, "t2")
	_, _ = c.PlanSource(pid)
	_, _ = c.Approvals(ctx, pid)
	_ = c.PendingApprovals(ctx)
	_ = c.Run(ctx, pid)
	_ = c.Resume(ctx, pid)
	_ = c.Retry(ctx, pid, "t2")
	_, _ = c.ReportTask(ctx, pid, "t2")
	_, _ = c.Reconcile(ctx, pid, "docs/PLAN.md")
	_ = c.ApproveTask(ctx, pid, "t2", DecisionOptions{})
	_ = c.DeclineTask(ctx, pid, "t2", DecisionOptions{})
	_, _ = c.ChangedTasks(ctx, pid)
	_ = c.AcceptChangedTask(ctx, pid, "t2")

	after := snapshotTree(t, dir)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("boundary mutated SOP persistence:\nbefore=%v\nafter =%v", before, after)
	}
}

// snapshotTree maps every file under dir to the hex digest of its contents, so a
// mutation (edit, add, or delete) shows as a diff.
func snapshotTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		out[rel] = fmt.Sprintf("%x", sha256.Sum256(b))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// recordingSop is a sop binary that appends the verb of every invocation (no
// truncation, unlike fakeSop), so a test can assert the complete set of verbs a
// boundary operation drove across several projects.
func recordingSop(t *testing.T) (bin string, verbs func() []string) {
	t.Helper()
	dir := t.TempDir()
	record := filepath.Join(dir, "verbs")
	bin = filepath.Join(dir, "sop")
	script := "#!/bin/sh\nprintf '%s\\n' \"$1\" >> " + record + "\nexit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, func() []string {
		raw, err := os.ReadFile(record)
		if err != nil {
			return nil
		}
		trimmed := strings.TrimSpace(string(raw))
		if trimmed == "" {
			return nil
		}
		return strings.Split(trimmed, "\n")
	}
}

// The cross-project inbox read delegates only to `sop approvals --json`, once per
// project, and starts no other SOP operation: it adds no orchestration or
// decision logic and no mutation path (docs/architecture/SOP-BOUNDARY.md).
func TestPendingApprovalsDelegatesOnlyToSOPApprovals(t *testing.T) {
	rootA := t.TempDir()
	initProjectAt(t, rootA, "alpha")
	rootB := t.TempDir()
	initProjectAt(t, rootB, "beta")

	bin, verbs := recordingSop(t)
	c, err := New([]string{rootA, rootB}, bin, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	_ = c.PendingApprovals(context.Background())

	got := verbs()
	if len(got) != 2 {
		t.Fatalf("invocations = %v, want one per project", got)
	}
	for _, v := range got {
		if v != "approvals" {
			t.Fatalf("inbox drove %q, want only approvals", v)
		}
	}
}
