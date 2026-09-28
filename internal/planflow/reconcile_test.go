package planflow

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
)

// reconcileDocTwo is a two-stage plan used to exercise additions and removals.
const reconcileDocTwo = "# Implementation Plan\n\n## Project\n\nWidget\n\n## Summary\n\nAdd a widget.\n\n## S001 — Application skeleton\n\nCreate it.\n\n### Acceptance Criteria\n\n- starts\n\n## S002 — Ingestion\n\nIngest it.\n\n### Dependencies\n\n- S001\n\n### Acceptance Criteria\n\n- ingests\n"

// prepareForReconcile writes a plan document, runs the normal prepare path (so
// .agent-sdlc/plan.json and the task graph exist), and returns the store.
func prepareForReconcile(t *testing.T, dir, name, doc string) *fakeStore {
	t.Helper()
	write(t, dir, name, doc)
	st := &fakeStore{}
	if _, err := Prepare(context.Background(), Options{Dir: dir, PlanSource: filepath.Join(dir, name), Store: st}); err != nil {
		t.Fatalf("prepare %s: %v", name, err)
	}
	return st
}

func reconcile(t *testing.T, dir, name string, st *fakeStore) (ReconcileResult, error) {
	t.Helper()
	return Reconcile(context.Background(), ReconcileOptions{
		Dir:        dir,
		PlanSource: filepath.Join(dir, name),
		Store:      st,
	})
}

// reconcileAccept runs a reconciliation that approves the named executed tasks
// for explicit definition replacement.
func reconcileAccept(t *testing.T, dir, name string, st *fakeStore, ids ...string) (ReconcileResult, error) {
	t.Helper()
	return Reconcile(context.Background(), ReconcileOptions{
		Dir:           dir,
		PlanSource:    filepath.Join(dir, name),
		Store:         st,
		AcceptChanged: ids,
	})
}

func taskByID(t *testing.T, tasks []*domain.Task, id string) *domain.Task {
	t.Helper()
	for _, task := range tasks {
		if task.ID == id {
			return task
		}
	}
	t.Fatalf("task %s not found in %+v", id, tasks)
	return nil
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// markExecuted gives a task execution history, so reconciliation must treat it as
// off-limits for silent redefinition or removal.
func markExecuted(task *domain.Task) {
	task.Status = domain.LOCAL_DONE
	task.Attempt = 1
	task.Attempts = []domain.Attempt{{Number: 1, Status: domain.LOCAL_DONE, Reason: "done"}}
}

func TestReconcileUnchangedPlanIsNoOp(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", planDoc)

	res, err := reconcile(t, dir, "PLAN.md", st)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if res.PlanChanged {
		t.Error("an unchanged plan must not report a change")
	}
	if len(res.Unchanged) != 1 || res.Unchanged[0] != "S001" {
		t.Errorf("unchanged = %v, want [S001]", res.Unchanged)
	}
	if len(res.Updated)+len(res.Added)+len(res.Removed) != 0 {
		t.Errorf("res = %+v, want no graph movement", res)
	}
	if len(st.tasks) != 1 {
		t.Errorf("tasks = %+v", st.tasks)
	}
}

func TestReconcileUpdatesUnexecutedTask(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", planDoc)
	write(t, dir, "PLAN.md", strings.ReplaceAll(planDoc, "Application skeleton", "Renamed"))

	res, err := reconcile(t, dir, "PLAN.md", st)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if !res.PlanChanged {
		t.Error("a changed plan must report a change")
	}
	if len(res.Updated) != 1 || res.Updated[0] != "S001" {
		t.Errorf("updated = %v, want [S001]", res.Updated)
	}
	task := taskByID(t, st.tasks, "S001")
	if task.Title != "Renamed" {
		t.Errorf("title = %q, want Renamed", task.Title)
	}
	if task.Status != domain.PLANNED {
		t.Errorf("status = %s, want PLANNED", task.Status)
	}
}

func TestReconcileChangedExecutedTaskStops(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", planDoc)
	markExecuted(taskByID(t, st.tasks, "S001"))
	write(t, dir, "PLAN.md", strings.ReplaceAll(planDoc, "Application skeleton", "Renamed"))

	_, err := reconcile(t, dir, "PLAN.md", st)
	if err == nil {
		t.Fatal("expected NEEDS_HUMAN for a changed executed task")
	}
	for _, want := range []string{"S001", "execution history", "title"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q missing %q", err, want)
		}
	}
	cur := taskByID(t, st.tasks, "S001")
	if cur.Title != "Application skeleton" || cur.Status != domain.LOCAL_DONE {
		t.Errorf("executed task must be untouched: %+v", cur)
	}
}

func TestReconcileTreatsProgressedTaskAsExecuted(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", planDoc)
	// A task that has progressed beyond PLANNED has left the pre-execution state,
	// even if it carries no attempt rows.
	taskByID(t, st.tasks, "S001").Status = domain.IMPLEMENTING
	write(t, dir, "PLAN.md", strings.ReplaceAll(planDoc, "Application skeleton", "Renamed"))

	if _, err := reconcile(t, dir, "PLAN.md", st); err == nil {
		t.Fatal("expected NEEDS_HUMAN for a progressed task")
	}
	if taskByID(t, st.tasks, "S001").Title != "Application skeleton" {
		t.Error("progressed task must be untouched")
	}
}

func TestReconcileAddsNewTask(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", planDoc)
	write(t, dir, "PLAN.md", reconcileDocTwo)

	res, err := reconcile(t, dir, "PLAN.md", st)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(res.Added) != 1 || res.Added[0] != "S002" {
		t.Errorf("added = %v, want [S002]", res.Added)
	}
	added := taskByID(t, st.tasks, "S002")
	if !sameStringSet(added.DependencyIDs, []string{"S001"}) {
		t.Errorf("S002 dependencies = %v, want [S001]", added.DependencyIDs)
	}
}

func TestReconcileRemovesUnexecutedTask(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", reconcileDocTwo)
	write(t, dir, "PLAN.md", planDoc)

	res, err := reconcile(t, dir, "PLAN.md", st)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(res.Removed) != 1 || res.Removed[0] != "S002" {
		t.Errorf("removed = %v, want [S002]", res.Removed)
	}
	if len(st.tasks) != 1 || st.tasks[0].ID != "S001" {
		t.Errorf("tasks = %+v, want only S001", st.tasks)
	}
}

func TestReconcileRemovingExecutedTaskStops(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", reconcileDocTwo)
	markExecuted(taskByID(t, st.tasks, "S002"))
	write(t, dir, "PLAN.md", planDoc)

	_, err := reconcile(t, dir, "PLAN.md", st)
	if err == nil {
		t.Fatal("expected NEEDS_HUMAN for a removed executed task")
	}
	if !strings.Contains(err.Error(), "S002") || !strings.Contains(err.Error(), "absent from the requested plan") {
		t.Errorf("error = %q", err)
	}
	if len(st.tasks) != 2 {
		t.Errorf("tasks = %+v, want nothing removed", st.tasks)
	}
}

func TestReconcileInvalidPlanStops(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", planDoc)
	bad := "# Plan\n\n## Project\n\nP\n\n## Summary\n\nS\n\n## S001 — a\n\no\n\n### Dependencies\n\n- S999\n\n### Acceptance Criteria\n\n- x\n"
	write(t, dir, "PLAN.md", bad)

	_, err := reconcile(t, dir, "PLAN.md", st)
	if err == nil {
		t.Fatal("expected a validation failure")
	}
	if !strings.Contains(err.Error(), "Plan validation failed") || !strings.Contains(err.Error(), "unknown dependency") {
		t.Errorf("error = %q", err)
	}
	if len(st.tasks) != 1 || st.tasks[0].ID != "S001" {
		t.Errorf("tasks = %+v, want untouched S001", st.tasks)
	}
}

func TestReconcileWithoutPersistedPlanStops(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "PLAN.md", planDoc)
	st := &fakeStore{tasks: []*domain.Task{{ID: "S001", Status: domain.PLANNED}}}

	_, err := reconcile(t, dir, "PLAN.md", st)
	if err == nil {
		t.Fatal("expected an error when there is no persisted plan")
	}
	if !strings.Contains(err.Error(), "no persisted plan") {
		t.Errorf("error = %q", err)
	}
}

func TestReconcilePreservesHistoryOfUnchangedTask(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", reconcileDocTwo)
	executed := taskByID(t, st.tasks, "S001")
	markExecuted(executed)

	// Only the unexecuted S002 changes; the executed S001 is left exactly as it was.
	write(t, dir, "PLAN.md", strings.ReplaceAll(reconcileDocTwo, "Ingest it.", "Ingest it well."))

	res, err := reconcile(t, dir, "PLAN.md", st)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(res.Unchanged) != 1 || res.Unchanged[0] != "S001" {
		t.Errorf("unchanged = %v, want [S001]", res.Unchanged)
	}
	if len(res.Updated) != 1 || res.Updated[0] != "S002" {
		t.Errorf("updated = %v, want [S002]", res.Updated)
	}
	kept := taskByID(t, st.tasks, "S001")
	if kept.Attempt != 1 || len(kept.Attempts) != 1 {
		t.Errorf("history lost: %+v", kept)
	}
}

// TestReconcilePlanWriteFailureSurfacesAndRetries pins the graph-leading
// persistence order: the graph is committed before the machine plan file, so a
// machine-plan write failure surfaces as an error while plan.json is left intact
// rather than truncated, and a retry once the directory is writable finishes the
// reconciliation idempotently.
func TestReconcilePlanWriteFailureSurfacesAndRetries(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("directory permissions do not restrict root or Windows")
	}
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", planDoc)

	planPath := filepath.Join(dir, config.DirName, "plan.json")
	metaPath := filepath.Join(dir, config.DirName, "plan.meta.json")
	beforePlan := read(t, planPath)
	beforeMeta := read(t, metaPath)

	write(t, dir, "PLAN.md", strings.ReplaceAll(planDoc, "Application skeleton", "Renamed"))

	// The store is in-memory, so only the file writes can fail. A read-only
	// .agent-sdlc still permits reads but makes the atomic temp-file write fail,
	// exercising the window after the graph mutation.
	sdlc := filepath.Join(dir, config.DirName)
	if err := os.Chmod(sdlc, 0o555); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(sdlc, 0o755) })

	if _, err := reconcile(t, dir, "PLAN.md", st); err == nil {
		t.Fatal("expected the machine-plan write failure to surface")
	}

	if got := read(t, planPath); got != beforePlan {
		t.Error("plan.json must be left intact when its write fails")
	}
	if got := read(t, metaPath); got != beforeMeta {
		t.Error("plan.meta.json must be untouched when the plan write fails")
	}
	if taskByID(t, st.tasks, "S001").Title != "Renamed" {
		t.Error("the graph is persisted before the file writes")
	}

	if err := os.Chmod(sdlc, 0o755); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	if _, err := reconcile(t, dir, "PLAN.md", st); err != nil {
		t.Fatalf("retry must finish the reconciliation: %v", err)
	}
	if !strings.Contains(read(t, planPath), "Renamed") {
		t.Error("the retried reconciliation must persist the requested plan")
	}
	if got := read(t, metaPath); got == beforeMeta {
		t.Error("the retried reconciliation must refresh the metadata")
	}
}

func TestReconcilePersistenceFailureLeavesPlanAuthoritative(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", planDoc)

	planPath := filepath.Join(dir, config.DirName, "plan.json")
	metaPath := filepath.Join(dir, config.DirName, "plan.meta.json")
	beforePlan := read(t, planPath)
	beforeMeta := read(t, metaPath)

	write(t, dir, "PLAN.md", strings.ReplaceAll(planDoc, "Application skeleton", "Renamed"))
	st.replaceErr = errors.New("disk full")

	_, err := reconcile(t, dir, "PLAN.md", st)
	if err == nil {
		t.Fatal("expected the persistence failure to surface")
	}
	if !strings.Contains(err.Error(), "disk full") {
		t.Errorf("error = %q", err)
	}
	if got := read(t, planPath); got != beforePlan {
		t.Error("plan.json must be left unchanged when persistence fails")
	}
	if got := read(t, metaPath); got != beforeMeta {
		t.Error("plan.meta.json must be left unchanged when persistence fails")
	}
	if taskByID(t, st.tasks, "S001").Title != "Application skeleton" {
		t.Error("the active graph must be left unchanged when persistence fails")
	}
}

func TestReconcileAcceptsChangedExecutedTask(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", planDoc)
	cur := taskByID(t, st.tasks, "S001")
	markExecuted(cur)
	write(t, dir, "PLAN.md", strings.ReplaceAll(planDoc, "Application skeleton", "Renamed"))

	res, err := reconcileAccept(t, dir, "PLAN.md", st, "S001")
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(res.Accepted) != 1 || res.Accepted[0] != "S001" {
		t.Errorf("accepted = %v, want [S001]", res.Accepted)
	}
	got := taskByID(t, st.tasks, "S001")
	if got.Title != "Renamed" {
		t.Errorf("title = %q, want the requested definition", got.Title)
	}
	// Approval replaces the definition only: lifecycle state and history survive.
	if got.Status != domain.LOCAL_DONE || got.Attempt != 1 || len(got.Attempts) != 1 {
		t.Errorf("history or lifecycle state lost: %+v", got)
	}
}

func TestReconcileAcceptsMultipleChangedExecutedTasks(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", reconcileDocTwo)
	markExecuted(taskByID(t, st.tasks, "S001"))
	markExecuted(taskByID(t, st.tasks, "S002"))
	changed := strings.ReplaceAll(reconcileDocTwo, "Application skeleton", "Renamed")
	changed = strings.ReplaceAll(changed, "Ingestion", "Renamed ingestion")
	write(t, dir, "PLAN.md", changed)

	res, err := reconcileAccept(t, dir, "PLAN.md", st, "S001", "S002")
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if strings.Join(res.Accepted, ",") != "S001,S002" {
		t.Errorf("accepted = %v, want [S001 S002]", res.Accepted)
	}
	if taskByID(t, st.tasks, "S001").Title != "Renamed" || taskByID(t, st.tasks, "S002").Title != "Renamed ingestion" {
		t.Error("both requested definitions must be applied")
	}
}

func TestReconcilePartialApprovalStops(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", reconcileDocTwo)
	markExecuted(taskByID(t, st.tasks, "S001"))
	markExecuted(taskByID(t, st.tasks, "S002"))
	changed := strings.ReplaceAll(reconcileDocTwo, "Application skeleton", "Renamed")
	changed = strings.ReplaceAll(changed, "Ingestion", "Renamed ingestion")
	write(t, dir, "PLAN.md", changed)

	_, err := reconcileAccept(t, dir, "PLAN.md", st, "S001")
	if err == nil {
		t.Fatal("one approval must not cover another changed executed task")
	}
	if !strings.Contains(err.Error(), "S002") {
		t.Errorf("error %q does not name the unapproved task S002", err)
	}
	// Nothing is applied while an executed change remains unapproved.
	if taskByID(t, st.tasks, "S001").Title != "Application skeleton" {
		t.Error("the graph must be untouched on partial approval")
	}
}

func TestReconcileRejectsUnknownAccept(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", planDoc)

	_, err := reconcileAccept(t, dir, "PLAN.md", st, "NOPE")
	if err == nil {
		t.Fatal("expected an unknown --accept-changed id to be rejected")
	}
	if !strings.Contains(err.Error(), "NOPE") {
		t.Errorf("error = %q", err)
	}
}

func TestReconcileRejectsAcceptOfUnchangedTask(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", planDoc)
	markExecuted(taskByID(t, st.tasks, "S001"))

	_, err := reconcileAccept(t, dir, "PLAN.md", st, "S001")
	if err == nil {
		t.Fatal("approving an unchanged task must be rejected")
	}
	if !strings.Contains(err.Error(), "does not name an executed task whose definition changed") {
		t.Errorf("error = %q", err)
	}
}

func TestReconcileRejectsAcceptOfRemovedTask(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", reconcileDocTwo)
	markExecuted(taskByID(t, st.tasks, "S002"))
	write(t, dir, "PLAN.md", planDoc)

	_, err := reconcileAccept(t, dir, "PLAN.md", st, "S002")
	if err == nil {
		t.Fatal("approving a removed executed task must be rejected")
	}
	if !strings.Contains(err.Error(), "removes") {
		t.Errorf("error = %q", err)
	}
}

func TestReconcileAcceptRecordsApprovalAndSourceHash(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", planDoc)
	markExecuted(taskByID(t, st.tasks, "S001"))
	changed := strings.ReplaceAll(planDoc, "Application skeleton", "Renamed")
	write(t, dir, "PLAN.md", changed)

	if _, err := reconcileAccept(t, dir, "PLAN.md", st, "S001"); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	metaPath := filepath.Join(dir, config.DirName, metaFileName)
	meta := readMetadata(metaPath)
	if len(meta.ReconciledTasks) != 1 || meta.ReconciledTasks[0] != "S001" {
		t.Errorf("reconciled_tasks = %v, want [S001]", meta.ReconciledTasks)
	}
	if meta.SourceSHA256 != fingerprint([]byte(changed)) {
		t.Errorf("source_sha256 = %q, want the requested plan's hash", meta.SourceSHA256)
	}

	// A later no-op reconciliation must not erase the recorded approval.
	if _, err := reconcile(t, dir, "PLAN.md", st); err != nil {
		t.Fatalf("no-op reconcile: %v", err)
	}
	if got := readMetadata(metaPath).ReconciledTasks; len(got) != 1 || got[0] != "S001" {
		t.Errorf("reconciled_tasks after no-op = %v, want [S001]", got)
	}
}

func TestReconcileAcceptPersistenceFailureLeavesEverythingAuthoritative(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", planDoc)
	markExecuted(taskByID(t, st.tasks, "S001"))

	planPath := filepath.Join(dir, config.DirName, "plan.json")
	metaPath := filepath.Join(dir, config.DirName, "plan.meta.json")
	beforePlan := read(t, planPath)
	beforeMeta := read(t, metaPath)

	write(t, dir, "PLAN.md", strings.ReplaceAll(planDoc, "Application skeleton", "Renamed"))
	st.replaceErr = errors.New("disk full")

	_, err := reconcileAccept(t, dir, "PLAN.md", st, "S001")
	if err == nil {
		t.Fatal("expected the persistence failure to surface")
	}
	if !strings.Contains(err.Error(), "disk full") {
		t.Errorf("error = %q", err)
	}
	if got := read(t, planPath); got != beforePlan {
		t.Error("plan.json must be left unchanged when persistence fails")
	}
	if got := read(t, metaPath); got != beforeMeta {
		t.Error("plan.meta.json must be left unchanged when persistence fails")
	}
	if taskByID(t, st.tasks, "S001").Title != "Application skeleton" {
		t.Error("the active graph must be left unchanged when persistence fails")
	}
}
