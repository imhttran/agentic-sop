package web

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The decisions route is a read-only projection of SOP's reported decision
// state: it offers approve/decline only for gates SOP reports applicable and
// accept only for changed-executed tasks SOP reports pending.
func TestProjectDecisionsRouteRendersState(t *testing.T) {
	srv, id, root := newRunServer(t, "BLOCKED", map[string]string{
		"approvals.json": approvalListing("needs a human"),
	})
	// Record an active plan and a changed-executed set with one pending task so
	// the accept control has a SOP-reported source.
	if err := os.WriteFile(filepath.Join(root, ".agent-sdlc", "plan.meta.json"),
		[]byte(`{"source":"docs/PLAN.md"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	writeChangedListing(t, root, "x1")

	code, body := get(t, srv.URL+"/projects/"+id+"/decisions")
	if code != http.StatusOK {
		t.Fatalf("decisions route = %d, want 200", code)
	}
	for _, want := range []string{
		"/tasks/x1/commands/approve",
		"/tasks/x1/commands/decline",
		"/tasks/x1/commands/accept-changed",
		`name="csrf"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("decisions route missing %q", want)
		}
	}
	if n := strings.Count(body, `name="note"`); n != 1 {
		t.Errorf("decisions panel should offer one shared decision note per gate, got %d", n)
	}
}

// An unknown project is a 404, not a phantom empty panel.
func TestProjectDecisionsUnknownProjectIs404(t *testing.T) {
	srv, _, _ := newRunServer(t, "PLANNED", nil)
	if code, _ := get(t, srv.URL+"/projects/nope/decisions"); code != http.StatusNotFound {
		t.Fatalf("unknown project decisions = %d, want 404", code)
	}
}

// With no SOP-reported gate and no SOP-reported changed-executed set, the panel
// shows an explicit empty state and offers no mutation control.
func TestProjectDecisionsEmptyState(t *testing.T) {
	srv, id, _ := newRunServer(t, "PLANNED", nil)
	code, body := get(t, srv.URL+"/projects/"+id) // no approvals.json, no plan
	if code != http.StatusOK {
		t.Fatalf("project = %d", code)
	}
	if !strings.Contains(body, "Human decisions") {
		t.Fatalf("decisions panel not present on project page")
	}
	if strings.Contains(body, "/commands/approve") {
		t.Error("no approve control may be offered without a SOP-reported gate")
	}
	if strings.Contains(body, "/commands/accept-changed") {
		t.Error("no accept control may be offered without a SOP-reported pending changed task")
	}
}

// The project page hosts the same decisions panel as the GET route, backed by
// the same SOP reads (no contradictory eligibility).
func TestProjectPageHostsDecisionsPanel(t *testing.T) {
	srv, id, _ := newRunServer(t, "BLOCKED", map[string]string{
		"approvals.json": approvalListing("needs a human"),
	})
	code, body := get(t, srv.URL+"/projects/"+id)
	if code != http.StatusOK {
		t.Fatalf("project = %d", code)
	}
	if !strings.Contains(body, "Human decisions") {
		t.Error("project page missing the decisions panel")
	}
	if !strings.Contains(body, "/tasks/x1/commands/approve") {
		t.Error("project page decisions panel missing the SOP-backed approve control")
	}
}

// Hostile values SOP reports (a task id carrying HTML) are HTML-escaped in the
// decisions panel, never rendered as raw markup.
func TestProjectDecisionsEscapesUntrustedIDs(t *testing.T) {
	srv, id, root := newRunServer(t, "BLOCKED", nil)
	if err := os.WriteFile(filepath.Join(root, ".agent-sdlc", "plan.meta.json"),
		[]byte(`{"source":"docs/PLAN.md"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	writeChangedListing(t, root, "x<script>alert(1)</script>")

	code, body := get(t, srv.URL+"/projects/"+id+"/decisions")
	if code != http.StatusOK {
		t.Fatalf("decisions route = %d", code)
	}
	if strings.Contains(body, "<script>alert(1)</script>") {
		t.Error("hostile task id rendered unescaped")
	}
}

// Every mutation in the decisions surface is POST + CSRF: an unauthenticated
// POST is rejected by csrfProtect, and viewing the panel (GET) never mutates.
func TestProjectDecisionsMutationsRequireCSRF(t *testing.T) {
	srv, id, _ := newRunServer(t, "BLOCKED", map[string]string{
		"approvals.json": approvalListing("needs a human"),
	})
	for _, path := range []string{
		"/projects/" + id + "/tasks/x1/commands/approve",
		"/projects/" + id + "/tasks/x1/commands/decline",
		"/projects/" + id + "/tasks/x1/commands/accept-changed",
	} {
		resp, err := http.Post(srv.URL+path, "application/x-www-form-urlencoded", strings.NewReader(""))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			t.Errorf("POST %s without CSRF = 200, want rejection", path)
		}
	}
}

// A GET on the decisions route performs no mutation: no command is recorded and
// no accept/approve is started.
func TestProjectDecisionsGETPerformsNoMutation(t *testing.T) {
	srv, id, root := newRunServer(t, "BLOCKED", map[string]string{
		"approvals.json": approvalListing("needs a human"),
	})
	if err := os.WriteFile(filepath.Join(root, ".agent-sdlc", "plan.meta.json"),
		[]byte(`{"source":"docs/PLAN.md"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	writeChangedListing(t, root, "x1")

	for i := 0; i < 2; i++ {
		if code, _ := get(t, srv.URL+"/projects/"+id+"/decisions"); code != http.StatusOK {
			t.Fatalf("decisions GET = %d", code)
		}
	}
	// After reading the panel the command-status for approve is still idle, i.e.
	// no command was ever started by the GET.
	_, status := get(t, srv.URL+"/projects/"+id+"/tasks/x1/commands/accept-changed")
	if !strings.Contains(status, "idle") {
		t.Errorf("GET decisions started a command: status = %q", status)
	}
}
