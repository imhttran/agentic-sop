package web

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

// A project whose state cannot be read renders as unavailable instead of
// failing the whole projects page.
func TestProjectsListSurfacesUnreadableProjectState(t *testing.T) {
	srv, id, root := newTestServerRoot(t, "sop")

	db, err := sql.Open("sqlite", "file:"+filepath.Join(root, ".agent-sdlc", "state.db")+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DROP TABLE tasks`); err != nil {
		db.Close()
		t.Fatalf("drop tasks: %v", err)
	}
	db.Close()

	code, body := get(t, srv.URL+"/projects")
	if code != 200 {
		t.Fatalf("GET /projects = %d, want 200 (one unreadable project must not blank the page)", code)
	}
	if !strings.Contains(body, "SOP state could not be read") {
		t.Fatalf("unavailable project not reported: %q", body)
	}
	if !strings.Contains(body, id) {
		t.Errorf("failed project id missing from the page")
	}
}
