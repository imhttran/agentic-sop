package web

import (
	"io/fs"
	"net/http/httptest"
	"strings"
	"testing"

	assets "github.com/imhttran/agentic-sop/internal/controller/assets"

	"github.com/imhttran/agentic-sop/internal/controller/config"
)

// newDiscoveryOnlyServer builds a server that serves the /discovery view without
// a SOP client: the handler reads only the discovery report, so no state is
// needed.
func newDiscoveryOnlyServer(t *testing.T, rep config.DiscoveryReport) *httptest.Server {
	t.Helper()
	views, err := NewViews(assets.FS)
	if err != nil {
		t.Fatal(err)
	}
	staticFS, err := fs.Sub(assets.FS, "static")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(NewServer(Options{Views: views, StaticFS: staticFS, Discovery: rep}))
	t.Cleanup(srv.Close)
	return srv
}

// The discovery view makes the allowlist boundary visible: configured roots, the
// depth bound, registered projects, and skipped candidates.
func TestDiscoveryPageRenders(t *testing.T) {
	rep := config.DiscoveryReport{
		Explicit:   []string{"/explicit/one"},
		Workspaces: []string{"/workspace"},
		MaxDepth:   4,
		Projects:   []config.Project{{Root: "/workspace/alpha", ID: "alpha"}},
		Diagnostics: []config.Diagnostic{
			{Path: "/workspace/bad", Reason: "invalid SOP config: missing project section"},
		},
	}
	srv := newDiscoveryOnlyServer(t, rep)

	code, body := get(t, srv.URL+"/discovery")
	if code != 200 {
		t.Fatalf("discovery page: %d", code)
	}
	for _, want := range []string{
		"Project discovery", "/workspace", "/explicit/one", "Maximum discovery depth: 4",
		"alpha", "/workspace/alpha", "/workspace/bad", "invalid SOP config",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("discovery page missing %q", want)
		}
	}
}

// With nothing configured the view explains the empty state instead of erroring.
func TestDiscoveryPageEmptyState(t *testing.T) {
	srv := newDiscoveryOnlyServer(t, config.DiscoveryReport{})
	code, body := get(t, srv.URL+"/discovery")
	if code != 200 {
		t.Fatalf("discovery page: %d", code)
	}
	for _, want := range []string{"None configured", "No SOP projects found", "None — every configured root was usable."} {
		if !strings.Contains(body, want) {
			t.Errorf("empty discovery page missing %q", want)
		}
	}
}

// The discovery view is linked from the navigation so it is discoverable.
func TestDiscoveryNavLinkPresent(t *testing.T) {
	srv := newDiscoveryOnlyServer(t, config.DiscoveryReport{})
	_, body := get(t, srv.URL+"/discovery")
	if !strings.Contains(body, `href="/discovery"`) {
		t.Error("navigation is missing the Discovery link")
	}
}
