package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/store"
)

// planRAGDoc is the markdown form of validPlanJSON, so reconciling it against a
// graph built from validPlanJSON is a true no-op.
const planRAGDoc = "# Implementation Plan\n\n## Project\n\nBook RAG\n\n## Summary\n\nBuild a retrieval augmented generation app.\n\n## S001 — Application skeleton\n\nCreate the Go application skeleton.\n\n### Deliverables\n\n- Go application\n\n### Acceptance Criteria\n\n- Application starts successfully\n\n## S002 — Ingestion\n\nIngest books into the index.\n\n### Dependencies\n\n- S001\n\n### Deliverables\n\n- Ingester\n\n### Acceptance Criteria\n\n- A book can be ingested\n"

// seedRAGProject initializes a project whose persisted plan.json and task graph
// come from validPlanJSON, with the matching human PLAN.md in docs/.
func seedRAGProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	initProject(t, dir)
	writePlanJSON(t, dir, validPlanJSON)
	writeFile(t, dir, filepath.Join("docs", "PLAN.md"), planRAGDoc)
	if code, _, stderr := runCLI(t, dir, "tasks"); code != exitOK {
		t.Fatalf("tasks: code=%d stderr=%s", code, stderr)
	}
	return dir
}

// markTaskExecuted gives a persisted task execution history via the store.
func markTaskExecuted(t *testing.T, dir, id string) {
	t.Helper()
	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	task, err := st.Get(id)
	if err != nil {
		t.Fatalf("get %s: %v", id, err)
	}
	task.Status = domain.LOCAL_DONE
	task.Attempt = 1
	task.Attempts = []domain.Attempt{{Number: 1, Status: domain.LOCAL_DONE, Reason: "done"}}
	if err := st.Save(task); err != nil {
		t.Fatalf("save %s: %v", id, err)
	}
}

func TestRunReconcileUsage(t *testing.T) {
	code, _, stderr := runCLI(t, t.TempDir(), "reconcile")
	if code != exitUsage {
		t.Fatalf("code=%d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr, "usage: sop reconcile") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestRunReconcileMissingPlan(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)

	code, _, stderr := runCLI(t, dir, "reconcile", "NOPE.md")
	if code != exitError {
		t.Fatalf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "PLAN not found") {
		t.Errorf("stderr = %q", stderr)
	}
}

func TestRunReconcileUnchangedPlan(t *testing.T) {
	dir := seedRAGProject(t)
	// An unrelated working-tree file must be preserved.
	writeFile(t, dir, "keep.txt", "user work\n")

	code, stdout, stderr := runCLI(t, dir, "reconcile", filepath.Join("docs", "PLAN.md"))
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	for _, want := range []string{"unchanged: 2", "matches the recorded plan"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout)
		}
	}
	if !stateExists(statePath(dir)) {
		t.Error("state.db must be preserved")
	}
}

func TestRunReconcileUpdatesUnexecutedTask(t *testing.T) {
	dir := seedRAGProject(t)
	writeFile(t, dir, filepath.Join("docs", "PLAN.md"),
		strings.ReplaceAll(planRAGDoc, "Application skeleton", "Renamed skeleton"))

	code, stdout, stderr := runCLI(t, dir, "reconcile", filepath.Join("docs", "PLAN.md"))
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "updated: [S001]") {
		t.Errorf("stdout = %q, want S001 updated", stdout)
	}

	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	task, err := st.Get("S001")
	if err != nil {
		t.Fatalf("get S001: %v", err)
	}
	if task.Title != "Renamed skeleton" {
		t.Errorf("title = %q, want the updated definition", task.Title)
	}
	if task.Status != domain.PLANNED {
		t.Errorf("status = %s, want PLANNED preserved", task.Status)
	}
}

func TestRunReconcileChangedExecutedStops(t *testing.T) {
	dir := seedRAGProject(t)
	markTaskExecuted(t, dir, "S001")
	// An unrelated working-tree file must survive the reconciliation.
	writeFile(t, dir, "keep.txt", "user work\n")
	writeFile(t, dir, filepath.Join("docs", "PLAN.md"),
		strings.ReplaceAll(planRAGDoc, "Application skeleton", "Renamed skeleton"))

	code, _, stderr := runCLI(t, dir, "reconcile", filepath.Join("docs", "PLAN.md"))
	if code != exitError {
		t.Fatalf("code=%d, want %d", code, exitError)
	}
	for _, want := range []string{"execution history", "S001"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr missing %q:\n%s", want, stderr)
		}
	}

	// The database and the executed task's history are preserved untouched.
	if !stateExists(statePath(dir)) {
		t.Error("state.db must be preserved")
	}
	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	task, err := st.Get("S001")
	if err != nil {
		t.Fatalf("get S001: %v", err)
	}
	if task.Title != "Application skeleton" || task.Status != domain.LOCAL_DONE || task.Attempt != 1 {
		t.Errorf("executed task must be untouched: %+v", task)
	}
	// A dirty working-tree file is never touched.
	if data, err := os.ReadFile(filepath.Join(dir, "keep.txt")); err != nil || string(data) != "user work\n" {
		t.Errorf("working-tree file changed: %q err=%v", data, err)
	}
}

func TestRunReconcileAcceptChangedExecuted(t *testing.T) {
	dir := seedRAGProject(t)
	markTaskExecuted(t, dir, "S001")
	writeFile(t, dir, filepath.Join("docs", "PLAN.md"),
		strings.ReplaceAll(planRAGDoc, "Application skeleton", "Renamed skeleton"))

	code, stdout, stderr := runCLI(t, dir, "reconcile", filepath.Join("docs", "PLAN.md"), "--accept-changed=S001")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "accepted (executed): [S001]") {
		t.Errorf("stdout = %q, want S001 accepted", stdout)
	}
	if !stateExists(statePath(dir)) {
		t.Error("state.db must be preserved")
	}

	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	task, err := st.Get("S001")
	if err != nil {
		t.Fatalf("get S001: %v", err)
	}
	if task.Title != "Renamed skeleton" {
		t.Errorf("title = %q, want the approved definition", task.Title)
	}
	if task.Status != domain.LOCAL_DONE || task.Attempt != 1 || len(task.Attempts) != 1 {
		t.Errorf("history or lifecycle state lost: %+v", task)
	}
}

func TestRunReconcileAcceptUnknownTask(t *testing.T) {
	dir := seedRAGProject(t)

	code, _, stderr := runCLI(t, dir, "reconcile", filepath.Join("docs", "PLAN.md"), "--accept-changed", "NOPE")
	if code != exitError {
		t.Fatalf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "NOPE") {
		t.Errorf("stderr = %q, want the rejected id", stderr)
	}
}

func TestRunReconcilePartialApprovalStops(t *testing.T) {
	dir := seedRAGProject(t)
	markTaskExecuted(t, dir, "S001")
	markTaskExecuted(t, dir, "S002")
	changed := strings.ReplaceAll(planRAGDoc, "Application skeleton", "Renamed skeleton")
	changed = strings.ReplaceAll(changed, "Ingestion", "Renamed ingestion")
	writeFile(t, dir, filepath.Join("docs", "PLAN.md"), changed)

	code, _, stderr := runCLI(t, dir, "reconcile", filepath.Join("docs", "PLAN.md"), "--accept-changed", "S001")
	if code != exitError {
		t.Fatalf("code=%d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "S002") {
		t.Errorf("stderr = %q, want the unapproved task named", stderr)
	}

	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	if task, err := st.Get("S001"); err != nil || task.Title != "Application skeleton" {
		t.Errorf("S001 must be untouched on partial approval: %+v err=%v", task, err)
	}
}

func TestRunReconcileAcceptMissingValue(t *testing.T) {
	code, _, stderr := runCLI(t, t.TempDir(), "reconcile", "docs/PLAN.md", "--accept-changed")
	if code != exitUsage {
		t.Fatalf("code=%d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr, "--accept-changed requires a task id") {
		t.Errorf("stderr = %q", stderr)
	}
}
