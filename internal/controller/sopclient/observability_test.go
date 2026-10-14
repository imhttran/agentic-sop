package sopclient

import (
	"context"
	"testing"
	"time"
)

// HARD007: absence of activity is never a lifecycle state. A task SOP reports
// as READY, with no run directory and no activity at all, must stay READY — the
// read layer must not infer FAILED, BLOCKED, or STUCK from silence or from how
// long ago the task last changed. The check is deterministic: no sleeps and no
// wall-clock dependency beyond a fixed "stale" timestamp.
func TestInactivityDoesNotInferFailure(t *testing.T) {
	root := newProject(t)
	// A quiet, never-run task: no runs/<id>/ artifacts exist for it, and its
	// last update is hours old.
	stale := time.Now().Add(-6 * time.Hour).UTC().Format(time.RFC3339)
	seed(t, root,
		`INSERT INTO tasks VALUES ('quiet','Quiet','o','a','READY',NULL,0,3,'`+stale+`','`+stale+`')`,
	)

	st, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	tasks, err := st.Tasks(context.Background(), readApprovals(t, st))
	if err != nil {
		t.Fatal(err)
	}
	var q TaskSummary
	found := false
	for _, task := range tasks {
		if task.ID == "quiet" {
			q, found = task, true
			break
		}
	}
	if !found {
		t.Fatal("task 'quiet' missing from Tasks()")
	}
	if got := q.State(); got != "READY" {
		t.Fatalf("State() = %q, want READY: absence of activity must not become a failed/blocked state", got)
	}
	if q.Stage != "" {
		t.Fatalf("Stage = %q, want empty: no run artifact exists, so no stage may be fabricated", q.Stage)
	}
	if q.Recovery != "" {
		t.Fatalf("Recovery = %q, want empty: SOP reported no recovery disposition", q.Recovery)
	}
	if q.NeedsHuman {
		t.Fatalf("NeedsHuman = true for a quiet READY task: no human boundary was reported by SOP")
	}

	// The read layer reports absence as absence: no activity events, not an
	// error and not a synthesized event.
	if events := st.activity("quiet"); len(events) != 0 {
		t.Fatalf("activity('quiet') = %d events, want 0", len(events))
	}
}
