package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A damaged activity artifact renders as unavailable instead of failing the page
// or claiming no activity was recorded.
func TestActivityPagesSurfaceReadFailure(t *testing.T) {
	srv, id, root := newTestServerRoot(t, "sop")
	path := filepath.Join(root, ".agent-sdlc", "runs", "t1", "activity.jsonl")
	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		url  string
	}{
		{"project", "/projects/" + id + "/activity"},
		{"task", "/projects/" + id + "/tasks/t1/activity"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, body := get(t, srv.URL+tc.url)
			if code != 200 {
				t.Fatalf("GET %s = %d, want 200", tc.url, code)
			}
			if !strings.Contains(body, "could not be read") {
				t.Fatalf("GET %s did not report the damaged activity: %q", tc.url, body)
			}
			if strings.Contains(body, "No activity recorded") {
				t.Errorf("GET %s rendered a damaged artifact as no activity", tc.url)
			}
		})
	}
}

// An unknown project still produces a 404, not an "unavailable" fragment.
func TestProjectActivityUnknownProjectIs404(t *testing.T) {
	srv, _, _ := newTestServerRoot(t, "sop")
	if code, _ := get(t, srv.URL+"/projects/nope/activity"); code != 404 {
		t.Fatalf("GET unknown project activity = %d, want 404", code)
	}
}
