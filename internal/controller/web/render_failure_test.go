package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// A template failure must produce a clean 500, not a partial 200 body: the
// handler renders to a buffer before writing headers.
func TestRenderFailureIsA500NotAPartial200(t *testing.T) {
	views, err := NewViews(fstest.MapFS{
		"templates/bad.html":                  &fstest.MapFile{Data: []byte(`before {{.Missing.Field}} after`)},
		"templates/partials/placeholder.html": &fstest.MapFile{Data: []byte("")},
	})
	if err != nil {
		t.Fatalf("NewViews: %v", err)
	}
	h := &Handlers{views: views}

	rec := httptest.NewRecorder()
	h.render(rec, http.StatusOK, "bad.html", struct{}{})

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "before") {
		t.Errorf("partial template output leaked into the response: %q", rec.Body.String())
	}
}
