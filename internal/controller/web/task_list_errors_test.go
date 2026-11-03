package web

import (
	"strings"
	"testing"
)

// A damaged run record is flagged in the polled task list instead of showing an
// empty stage.
func TestTaskListFlagsUnreadableRunRecord(t *testing.T) {
	srv, id, root := newTestServerRoot(t, "sop")
	writeRun(t, root, "t1", "state.json", "{ not json")

	code, body := get(t, srv.URL+"/projects/"+id+"/tasks")
	if code != 200 {
		t.Fatalf("GET tasks = %d, want 200", code)
	}
	if !strings.Contains(body, "run record could not be read") {
		t.Fatalf("task list did not flag the damaged run record: %q", body)
	}
}
