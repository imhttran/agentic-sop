package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/store"
)

// The read-only reconciliation listing (`sop reconcile --list-changed`): what a
// reconciliation would change, reported before anything is applied. A client (for
// example the controller) renders it instead of diffing the plan itself.

// TestRunReconcileListChangedIsReadOnly proves the listing names the changed
// executed task a reconciliation would stop on, and applies nothing - so a
// reconciliation run afterwards still stops for the same approval.
func TestRunReconcileListChangedIsReadOnly(t *testing.T) {
	dir := seedRAGProject(t)
	markTaskExecuted(t, dir, "S001")
	writeFile(t, dir, filepath.Join("docs", "PLAN.md"),
		strings.ReplaceAll(planRAGDoc, "Application skeleton", "Renamed skeleton"))

	code, stdout, stderr := runCLI(t, dir, "reconcile", filepath.Join("docs", "PLAN.md"), "--list-changed")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	for _, want := range []string{
		"nothing applied",
		"docs/PLAN.md",
		"changed (executed: approve each with --accept-changed): [S001]",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, stdout)
		}
	}

	// The executed task's definition and history are untouched.
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
		t.Errorf("the listing applied a change: %+v", task)
	}

	// A real reconciliation still stops, which proves the listing authorized nothing.
	if code, _, _ := runCLI(t, dir, "reconcile", filepath.Join("docs", "PLAN.md")); code != exitError {
		t.Errorf("reconcile after a listing: code=%d, want %d (the listing must apply nothing)", code, exitError)
	}
}

// TestRunReconcileListChangedJSON proves the structured listing a client consumes:
// a stable document with every category, and the two human-decision categories in
// their own fields.
func TestRunReconcileListChangedJSON(t *testing.T) {
	dir := seedRAGProject(t)
	markTaskExecuted(t, dir, "S001")
	writeFile(t, dir, filepath.Join("docs", "PLAN.md"),
		strings.ReplaceAll(planRAGDoc, "Application skeleton", "Renamed skeleton"))

	code, stdout, stderr := runCLI(t, dir, "reconcile", filepath.Join("docs", "PLAN.md"), "--list-changed", "--json")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}

	var doc struct {
		Version         int      `json:"version"`
		Source          string   `json:"source"`
		PlanChanged     bool     `json:"plan_changed"`
		Unchanged       []string `json:"unchanged"`
		ChangedExecuted []string `json:"changed_executed"`
		RemovedExecuted []string `json:"removed_executed"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("stdout is not the listing document: %v\n%s", err, stdout)
	}
	if doc.Version != 1 {
		t.Errorf("version = %d, want 1", doc.Version)
	}
	if doc.Source != "docs/PLAN.md" {
		t.Errorf("source = %q, want docs/PLAN.md", doc.Source)
	}
	if !doc.PlanChanged {
		t.Error("plan_changed = false, want true")
	}
	if len(doc.ChangedExecuted) != 1 || doc.ChangedExecuted[0] != "S001" {
		t.Errorf("changed_executed = %v, want [S001]", doc.ChangedExecuted)
	}
	// An empty category is [] in JSON, never null, so a client can iterate it blindly.
	if doc.RemovedExecuted == nil {
		t.Error("removed_executed = null, want []")
	}
	if !strings.Contains(stdout, `"removed_executed": []`) {
		t.Errorf("stdout does not render an empty category as []:\n%s", stdout)
	}
}

// TestRunReconcileListChangedRejectsApproval proves a listing authorizes nothing,
// so combining it with an approval is a usage error rather than a silently ignored
// flag (which would look like the approval had been applied).
func TestRunReconcileListChangedRejectsApproval(t *testing.T) {
	dir := seedRAGProject(t)

	code, _, stderr := runCLI(t, dir, "reconcile", filepath.Join("docs", "PLAN.md"), "--list-changed", "--accept-changed", "S001")
	if code != exitUsage {
		t.Fatalf("code=%d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr, "only reports") {
		t.Errorf("stderr = %q, want the usage error explaining a listing only reports", stderr)
	}
}

// TestRunReconcileJSONRequiresListing proves --json is not silently accepted on the
// applying path, where it would imply a structured report the command does not emit.
func TestRunReconcileJSONRequiresListing(t *testing.T) {
	dir := seedRAGProject(t)

	code, _, stderr := runCLI(t, dir, "reconcile", filepath.Join("docs", "PLAN.md"), "--json")
	if code != exitUsage {
		t.Fatalf("code=%d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr, "--json is available with --list-changed") {
		t.Errorf("stderr = %q", stderr)
	}
}
