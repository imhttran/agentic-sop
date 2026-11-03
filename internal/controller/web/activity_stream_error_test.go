package web

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A damaged activity artifact is reported as JSON on the poll endpoint, never as
// the HTML error page.
func TestActivityWindowReadFailureIsJSON(t *testing.T) {
	srv, id, root := newTestServerRoot(t, "sop")
	path := filepath.Join(root, ".agent-sdlc", "runs", "t1", "activity.jsonl")
	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}

	resp, err := http.Get(srv.URL + "/projects/" + id + "/activity/window")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode JSON error body: %v", err)
	}
	if body["error"] == "" {
		t.Error("JSON error body has no error message")
	}
}

// An unknown project is a 404 JSON response on the poll endpoint.
func TestActivityWindowUnknownProjectIsJSON404(t *testing.T) {
	srv, _, _ := newTestServerRoot(t, "sop")

	resp, err := http.Get(srv.URL + "/projects/nope/activity/window")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}
