package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// An artifact that exists but cannot be reached is rendered as unreadable, never
// as "no report" and never as the absent literal.
func TestTaskPageReportsUnreachableReport(t *testing.T) {
	srv, id, root := newTestServerRoot(t, "sop")
	// A file where the run directory belongs makes os.Stat fail with ENOTDIR, an
	// I/O error that is not fs.ErrNotExist.
	runs := filepath.Join(root, ".agent-sdlc", "runs")
	if err := os.MkdirAll(runs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runs, "t2"), []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}

	code, body := get(t, srv.URL+"/projects/"+id+"/tasks/t2")
	if code != 200 {
		t.Fatalf("task page = %d, want 200", code)
	}
	for _, want := range []string{
		"unreadable",
		"SOP evidence could not be read",
		"report: report.json",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("task page missing %q", want)
		}
	}
}

// A config that cannot be read is surfaced on the projects list instead of
// silently presenting the project id as the name.
func TestProjectsPageReportsUnreadableConfig(t *testing.T) {
	srv, _, root := newTestServerRoot(t, "sop")
	cfg := filepath.Join(root, ".agent-sdlc", "config.yaml")
	if err := os.Remove(cfg); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(cfg, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}

	code, body := get(t, srv.URL+"/projects")
	if code != 200 {
		t.Fatalf("projects page = %d, want 200", code)
	}
	if !strings.Contains(body, "config unavailable") {
		t.Error("projects page should flag the unreadable project config")
	}
	if !strings.Contains(body, "config.yaml") {
		t.Error("projects page should name config.yaml in the badge title")
	}
}

// A readable config keeps the projects page clean.
func TestProjectsPageWithoutConfigError(t *testing.T) {
	srv, _, _ := newTestServerRoot(t, "sop")

	code, body := get(t, srv.URL+"/projects")
	if code != 200 {
		t.Fatalf("projects page = %d, want 200", code)
	}
	if strings.Contains(body, "config unavailable") {
		t.Error("projects page flagged a config error for a readable config")
	}
}
