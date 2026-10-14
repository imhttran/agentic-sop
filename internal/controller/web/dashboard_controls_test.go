package web

import (
	"strings"
	"testing"
)

// TestStartContinueVisibility asserts Start/Continue is shown exactly when no
// task is actively running and the project is not complete (CTRL013).
func TestStartContinueVisibility(t *testing.T) {
	cases := []struct {
		name       string
		status     string
		artifacts  map[string]string
		wantButton bool
	}{
		{"idle", "PLANNED", nil, true},
		{"blocked-with-budget", "BLOCKED", nil, true},
		{"running", "IMPLEMENTING", map[string]string{"state.json": `{"id":"x1","stage":"IMPLEMENTING"}`}, false},
		{"complete", "DONE", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, id, _ := newRunServer(t, tc.status, tc.artifacts)
			code, body := get(t, srv.URL+"/projects/"+id)
			if code != 200 {
				t.Fatalf("project view: status %d", code)
			}
			got := strings.Contains(body, "Start / Continue")
			if got != tc.wantButton {
				t.Errorf("%s: Start/Continue present = %v, want %v", tc.name, got, tc.wantButton)
			}
		})
	}
}

// TestRetryAllBlockedVisibility asserts "Retry all blocked" is shown only when
// a BLOCKED task still has retry budget (CTRL013).
func TestRetryAllBlockedVisibility(t *testing.T) {
	cases := []struct {
		name       string
		status     string
		wantButton bool
	}{
		{"idle", "PLANNED", false},
		{"blocked-with-budget", "BLOCKED", true}, // newRunServer seeds attempt=2, max_attempts=3
		{"complete", "DONE", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, id, _ := newRunServer(t, tc.status, nil)
			code, body := get(t, srv.URL+"/projects/"+id)
			if code != 200 {
				t.Fatalf("project view: status %d", code)
			}
			got := strings.Contains(body, "/commands/retry-all")
			if got != tc.wantButton {
				t.Errorf("%s: Retry all blocked present = %v, want %v", tc.name, got, tc.wantButton)
			}
		})
	}
}
