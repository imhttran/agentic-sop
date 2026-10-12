package web

import (
	"database/sql"
	"io/fs"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	assets "sop-controller"

	"sop-controller/internal/config"
	"sop-controller/internal/sopclient"

	_ "modernc.org/sqlite"
)

// The task detail view reports SOP's recorded model selection verbatim: the
// class, locality, and source appear when SOP persisted a record, and an
// explicit absence appears when it did not. The controller never selects a model
// itself (docs/specs/AGENT-PROVIDER.md).
func TestTaskPageShowsModelSelection(t *testing.T) {
	srv, id, root := modelSelectionServer(t)
	_ = root

	_, body := get(t, srv.URL+"/projects/"+id+"/tasks/m1")
	if !strings.Contains(body, "Model class") {
		t.Fatalf("model-class row missing: %q", body)
	}
	if !strings.Contains(body, "large") || !strings.Contains(body, "cloud") {
		t.Errorf("SOP model class/locality not shown verbatim: %q", body)
	}

	// A task with no model-selection record shows an explicit absence.
	_, body = get(t, srv.URL+"/projects/"+id+"/tasks/m2")
	if !strings.Contains(body, "SOP persisted no model-selection record") {
		t.Errorf("absent model selection not shown as an absence: %q", body)
	}
}

// modelSelectionServer builds a project with m1 carrying a persisted
// model_selection record and m2 carrying none.
func modelSelectionServer(t *testing.T) (*httptest.Server, string, string) {
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
	now := time.Now().UTC().Format(time.RFC3339)
	stmts := []string{
		testSchema,
		`INSERT INTO tasks VALUES ('m1','Task M1','obj','ac','IMPLEMENTING',NULL,1,3,'` + now + `','` + now + `')`,
		`INSERT INTO tasks VALUES ('m2','Task M2','obj','ac','PLANNED',NULL,0,3,'` + now + `','` + now + `')`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	db.Close()

	writeRun(t, root, "m1", "report.json", `{
	  "id":"m1","stage":"PASSED","decision":"PASS","provider":"ollama","model":"deepseek-v4.1-flash:cloud",
	  "model_selection":{"class":"large","provider":"ollama","model":"deepseek-v4.1-flash:cloud","locality":"cloud","source":"env","fallback":false,"reason":"environment default class"},
	  "generated_at":"2026-01-01T00:00:00Z"
	}`)

	bin := filepath.Join(t.TempDir(), "sop")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
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
	return srv, config.ProjectID(root), root
}
