package config

import (
	"os"
	"path/filepath"
	"testing"
)

// A malformed .agent-sdlc layout is reported, not silently treated as "not a
// project".
func TestConsiderProjectReportsMalformedLayout(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".agent-sdlc"), []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}

	project, isProject, diag := considerProject(dir)
	if isProject {
		t.Fatalf("considerProject = %+v, true; want not a project", project)
	}
	if diag == "" {
		t.Fatal("a malformed .agent-sdlc layout must produce a diagnostic")
	}
}

// A directory with no .agent-sdlc at all stays a silent non-project.
func TestConsiderProjectIgnoresOrdinaryDirectory(t *testing.T) {
	if _, isProject, diag := considerProject(t.TempDir()); isProject || diag != "" {
		t.Fatalf("ordinary directory = (isProject=%v, %q), want false with no diagnostic", isProject, diag)
	}
}
