package sopclient

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/controller/config"
)

// Projects discovered beneath a workspace root enter the same registry as
// explicit ones: identical routes and model, with no discovery-specific handling.
func TestDiscoveredProjectsEnterRegistry(t *testing.T) {
	ws := t.TempDir()
	initProjectAt(t, filepath.Join(ws, "alpha"), "alpha")
	initProjectAt(t, filepath.Join(ws, "group", "beta"), "beta")

	projects, diags, err := config.ResolveProjects(nil, []string{ws}, config.DefaultMaxDepth)
	if err != nil {
		t.Fatal(err)
	}
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	roots := make([]string, 0, len(projects))
	for _, p := range projects {
		roots = append(roots, p.Root)
	}

	c, err := New(roots, "sop", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	list, err := c.Projects(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("got %d projects, want 2", len(list))
	}
	got := map[string]bool{}
	for _, p := range list {
		got[p.ID] = true
		if p.Name == "" {
			t.Errorf("project %s has no display name", p.ID)
		}
	}
	for _, id := range []string{"alpha", "beta"} {
		if !got[id] {
			t.Errorf("discovered project %q missing from registry", id)
		}
	}
}
