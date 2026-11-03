package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeDamagedPlanMeta replaces plan.meta.json with a truncated record, so the
// metadata exists but cannot be parsed.
func writeDamagedPlanMeta(t *testing.T, root string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, ".agent-sdlc", "plan.meta.json"), []byte(`{"source": `), 0o644); err != nil {
		t.Fatal(err)
	}
}

// The project page reports damaged plan metadata instead of presenting the
// project as one with no plan and no changed tasks.
func TestProjectPageReportsDamagedPlanMeta(t *testing.T) {
	srv, id, root := newRunServer(t, "PLANNED", nil)
	writeDamagedPlanMeta(t, root)

	code, body := get(t, srv.URL+"/projects/"+id)
	if code != 200 {
		t.Fatalf("GET project = %d, want 200", code)
	}
	if !strings.Contains(body, "plan metadata unreadable") {
		t.Error("project page should flag the unreadable plan metadata")
	}
	if !strings.Contains(body, "plan.meta.json") {
		t.Error("project page should name plan.meta.json")
	}
	if strings.Contains(body, "No active plan") {
		t.Error("project page should not present damaged metadata as an absent plan")
	}
}

// Reconcile against damaged metadata is an infrastructure failure (500), not the
// "nothing to reconcile" precondition (409).
func TestReconcileReportsDamagedPlanMeta(t *testing.T) {
	srv, id, root := newRunServer(t, "PLANNED", nil)
	writeDamagedPlanMeta(t, root)

	if code := postWithCSRF(t, srv, "/projects/"+id+"/commands/reconcile"); code != 500 {
		t.Fatalf("reconcile with damaged plan metadata = %d, want 500", code)
	}
}

// A project with no plan still refuses reconcile as a precondition (409).
func TestReconcileWithoutPlanIsStillAConflict(t *testing.T) {
	srv, id, _ := newRunServer(t, "PLANNED", nil)

	if code := postWithCSRF(t, srv, "/projects/"+id+"/commands/reconcile"); code != 409 {
		t.Fatalf("reconcile with no plan = %d, want 409", code)
	}
}
