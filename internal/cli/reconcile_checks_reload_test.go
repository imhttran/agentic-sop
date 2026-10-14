package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// planChecksDoc / planChecksJSON are a plan whose stages declare operator
// acceptance checks, in both the markdown and machine forms, so a task reloaded
// from the store can be reconciled against it.
const planChecksJSON = `{
  "project": "Book RAG",
  "summary": "Build a retrieval augmented generation app.",
  "stages": [
    {
      "id": "S001",
      "title": "Application skeleton",
      "objective": "Create the Go application skeleton.",
      "dependencies": [],
      "deliverables": ["Go application"],
      "acceptance_criteria": ["Application starts successfully"],
      "acceptance_checks": ["go build ./..."]
    },
    {
      "id": "S002",
      "title": "Ingestion",
      "objective": "Ingest books into the index.",
      "dependencies": ["S001"],
      "deliverables": ["Ingester"],
      "acceptance_criteria": ["A book can be ingested"],
      "acceptance_checks": ["go test ./..."]
    }
  ]
}`

const planChecksDoc = "# Implementation Plan\n\n## Project\n\nBook RAG\n\n## Summary\n\nBuild a retrieval augmented generation app.\n\n" +
	"## S001 — Application skeleton\n\nCreate the Go application skeleton.\n\n" +
	"### Deliverables\n\n- Go application\n\n" +
	"### Acceptance Criteria\n\n- Application starts successfully\n\n" +
	"### Acceptance Checks\n\n- `go build ./...`\n\n" +
	"### Execution\n\n- implement\n\n" +
	"## S002 — Ingestion\n\nIngest books into the index.\n\n" +
	"### Dependencies\n\n- S001\n\n" +
	"### Deliverables\n\n- Ingester\n\n" +
	"### Acceptance Criteria\n\n- A book can be ingested\n\n" +
	"### Acceptance Checks\n\n- `go test ./...`\n\n" +
	"### Execution\n\n- implement\n"

type reconcileListing struct {
	PlanChanged     bool     `json:"plan_changed"`
	Unchanged       []string `json:"unchanged"`
	Updated         []string `json:"updated"`
	Added           []string `json:"added"`
	Removed         []string `json:"removed"`
	ChangedExecuted []string `json:"changed_executed"`
	RemovedExecuted []string `json:"removed_executed"`
}

func listChanged(t *testing.T, dir string) reconcileListing {
	t.Helper()
	code, stdout, stderr := runCLI(t, dir, "reconcile", filepath.Join("docs", "PLAN.md"), "--list-changed", "--json")
	if code != exitOK {
		t.Fatalf("reconcile --list-changed: code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	var res reconcileListing
	if err := json.Unmarshal([]byte(stdout), &res); err != nil {
		t.Fatalf("unmarshal listing %q: %v", stdout, err)
	}
	return res
}

// TestReconcileUnchangedCheckedPlanAfterReloadHasNoChanges is the regression: a
// task reloaded from the store carries no acceptance checks (the store does not
// persist them), so reconciling an UNCHANGED plan that declares checks must still
// report no change — not a spurious update for every checked task. A real
// acceptance-check change must still be reported.
func TestReconcileUnchangedCheckedPlanAfterReloadHasNoChanges(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	initProject(t, dir)
	writePlanJSON(t, dir, planChecksJSON)
	writeFile(t, dir, filepath.Join("docs", "PLAN.md"), planChecksDoc)
	if code, _, stderr := runCLI(t, dir, "tasks"); code != exitOK {
		t.Fatalf("tasks: code=%d stderr=%s", code, stderr)
	}

	res := listChanged(t, dir)
	if res.PlanChanged {
		t.Errorf("plan_changed = true, want false for an unchanged plan")
	}
	if n := len(res.Updated) + len(res.Added) + len(res.Removed) + len(res.ChangedExecuted) + len(res.RemovedExecuted); n != 0 {
		t.Errorf("spurious reconciliation change(s) after reload: %+v", res)
	}
	if len(res.Unchanged) != 2 {
		t.Errorf("unchanged = %v, want [S001 S002]", res.Unchanged)
	}

	// A real acceptance-check change is still a change (never suppressed).
	changed := strings.Replace(planChecksDoc, "`go build ./...`", "`go build ./cmd/...`", 1)
	if changed == planChecksDoc {
		t.Fatal("test setup: acceptance-check edit did not apply")
	}
	writeFile(t, dir, filepath.Join("docs", "PLAN.md"), changed)

	res = listChanged(t, dir)
	if len(res.Updated)+len(res.ChangedExecuted) == 0 {
		t.Errorf("a real acceptance-check change was suppressed: %+v", res)
	}
	if !contains(res.Updated, "S001") && !contains(res.ChangedExecuted, "S001") {
		t.Errorf("S001's changed acceptance check was not reported: %+v", res)
	}
	if contains(res.Unchanged, "S001") {
		t.Errorf("S001 reported unchanged despite a changed acceptance check: %+v", res)
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
