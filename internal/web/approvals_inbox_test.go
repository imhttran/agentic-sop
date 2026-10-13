package web

import (
	"database/sql"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	assets "sop-controller"

	"sop-controller/internal/sopclient"

	_ "modernc.org/sqlite"
)

// GET /approvals renders SOP's applicable gates and delegates each decision to
// the existing task-scoped approve/decline routes (CSRF included). The page is
// read-only: it starts no command and manufactures no gate.
func TestApprovalsInboxRendersPendingGates(t *testing.T) {
	srv, id, _ := decisionServer(t, "")
	_, body := get(t, srv.URL+"/approvals")

	if !strings.Contains(body, "Pending approvals") {
		t.Fatalf("inbox heading missing: %q", body)
	}
	if !strings.Contains(body, "demo") {
		t.Errorf("project name missing from inbox")
	}
	if !strings.Contains(body, "x1") || !strings.Contains(body, "NEEDS_HUMAN") {
		t.Errorf("gate not rendered: %q", body)
	}
	if !strings.Contains(body, "needs a human") {
		t.Errorf("SOP-reported reason missing")
	}
	// The control must POST to the existing task-scoped route with a CSRF token.
	if want := "/projects/" + id + "/tasks/x1/commands/approve"; !strings.Contains(body, want) {
		t.Errorf("approve form not wired to %q", want)
	}
	if !strings.Contains(body, "name=\"csrf\"") {
		t.Errorf("approve form missing csrf input")
	}
}

// The inbox offers one shared decision field per approval, not one note per action: a
// single form carries the shared actor and decision note, and both decision buttons
// submit them to their existing endpoints.
func TestApprovalsInboxUsesOneSharedDecisionNote(t *testing.T) {
	srv, id, _ := decisionServer(t, "")
	_, body := get(t, srv.URL+"/approvals")

	if n := strings.Count(body, `name="by"`); n != 1 {
		t.Errorf("inbox should have exactly one shared actor field, got %d", n)
	}
	if n := strings.Count(body, `name="note"`); n != 1 {
		t.Errorf("inbox should have exactly one shared decision note field, got %d", n)
	}
	for _, want := range []string{
		"/projects/" + id + "/tasks/x1/commands/approve",
		`formaction="/projects/` + id + `/tasks/x1/commands/decline"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("inbox missing %q", want)
		}
	}
}

// A failed `sop approvals --json` is shown as an unread listing, never as "no
// gates": a failing SOP must be distinguishable from a quiet one.
func TestApprovalsInboxSurfacesUnreadListing(t *testing.T) {
	srv := unreadApprovalsServer(t)
	_, body := get(t, srv.URL+"/approvals")

	if !strings.Contains(body, "Could not be read") {
		t.Fatalf("unread panel missing: %q", body)
	}
	if strings.Contains(body, "SOP reports no applicable approval gate") {
		t.Errorf("an unread listing was rendered as 'no gates': %q", body)
	}
}

// The projects list surfaces the attention count derived from SOP's listing.
func TestProjectsListShowsAttentionBadge(t *testing.T) {
	srv, _, _ := decisionServer(t, "")
	_, body := get(t, srv.URL+"/projects")

	if !strings.Contains(body, "needs attention") {
		t.Fatalf("attention badge missing from projects list: %q", body)
	}
}

// The inbox gate list refreshes itself when a decision finishes, so a decided
// gate does not linger on screen as a stale card.
func TestApprovalsInboxWiresListRefresh(t *testing.T) {
	srv, _, _ := decisionServer(t, "")
	_, body := get(t, srv.URL+"/approvals")

	// The refresher carries the hx-* wiring and must be a SIBLING of the list.
	for _, want := range []string{
		`id="approvals-refresh"`,
		`hx-get="/approvals"`,
		`hx-select="#approvals-list"`,
		`hx-target="#approvals-list"`,
		"approvalsChanged from:body",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("inbox missing refresh wiring %q", want)
		}
	}
	// The forms' container must stay a plain div: htmx inherits hx-select from an
	// ancestor, which would select the approve/decline command fragment away and
	// leave the decision rendering empty.
	if !strings.Contains(body, `<div id="approvals-list">`) {
		t.Errorf("gate-list container must be a plain div so the decision forms inherit no hx-select")
	}
}

// A finished approve/decline command signals the inbox to refresh (HX-Trigger),
// so the decided gate is re-read from SOP and dropped; a still-running command
// does not signal.
func TestDecisionStatusSignalsInboxRefresh(t *testing.T) {
	srv, id, _ := decisionServer(t, "")
	if code, _ := postApproval(t, srv, id, "x1", "approve", "", ""); code != http.StatusOK {
		t.Fatalf("approve POST = %d, want 200", code)
	}
	statusURL := srv.URL + "/projects/" + id + "/tasks/x1/commands/approve"
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		req, err := http.NewRequest("GET", statusURL, nil)
		if err != nil {
			t.Fatal(err)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		hit := res.Header.Get("HX-Trigger")
		_, _ = io.Copy(io.Discard, res.Body)
		res.Body.Close()
		if hit == "approvalsChanged" {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("a finished decision did not signal the inbox to refresh (no HX-Trigger: approvalsChanged)")
}

// unreadApprovalsServer builds a project whose `sop approvals --json` fails (a
// binary that exits non-zero and no persisted listing), so the approval read is
// unreported rather than an empty success.
func unreadApprovalsServer(t *testing.T) *httptest.Server {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, ".agent-sdlc")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"),
		[]byte("project:\n  name: \"demo\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(testSchema); err != nil {
		t.Fatal(err)
	}
	db.Close()

	bin := filepath.Join(t.TempDir(), "sop")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	sop, err := sopclient.New([]string{root}, bin, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sop.Close() })
	views, err := NewViews(assets.FS)
	if err != nil {
		t.Fatalf("views: %v", err)
	}
	staticFS, err := fs.Sub(assets.FS, "static")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(NewServer(Options{SOP: sop, Views: views, StaticFS: staticFS, Poll: time.Second, CommandTimeout: time.Minute}))
	t.Cleanup(srv.Close)
	return srv
}
