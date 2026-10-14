package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writePlanSource records SOP's active plan provenance so the C2-003 listing
// read has a <PLAN.md> to resolve.
func writePlanSource(t *testing.T, root string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, ".agent-sdlc", "plan.meta.json"), []byte(`{"source":"docs/PLAN.md"}`), 0o644); err != nil {
		t.Fatal(err)
	}
}

// The project page reports a SOP-reported changed executed task before any
// reconcile mutation can happen, as pending until accepted. The changed set
// comes from SOP's authoritative listing (writeChangedListing), never from the
// retired reconcile.json artifact.
func TestProjectPageReportsChangedTasksBeforeMutation(t *testing.T) {
	srv, id, root := newRunServer(t, "PLANNED", nil)
	writePlanSource(t, root)
	writeChangedListing(t, root, "x1")

	code, body := get(t, srv.URL+"/projects/"+id)
	if code != 200 {
		t.Fatalf("GET project = %d", code)
	}
	if !strings.Contains(body, "x1") {
		t.Errorf("project page does not report the SOP changed task verbatim:\n%s", body)
	}
	if !strings.Contains(body, "pending") {
		t.Errorf("unapproved changed task should render as pending:\n%s", body)
	}
}

// When SOP reports an observed empty changed-executed-task listing, the project
// page shows an explicit absence, never an empty table presented as success.
func TestProjectPageShowsAbsentChangedTaskSet(t *testing.T) {
	srv, id, root := newRunServer(t, "PLANNED", nil)
	writePlanSource(t, root)
	// No changed_listing.json: the fake sop answers with an observed EMPTY
	// listing, so SOP reported a set with no changed tasks.

	code, body := get(t, srv.URL+"/projects/"+id)
	if code != 200 {
		t.Fatalf("GET project = %d", code)
	}
	if !strings.Contains(body, "SOP reports no changed executed tasks awaiting reconcile") {
		t.Errorf("project page should show an explicit absence state when SOP reports no changed tasks:\n%s", body)
	}
	if strings.Contains(body, "Changed executed tasks</h2></div>\n  \n  <table") {
		t.Errorf("project page must not render an empty table as success")
	}
}

// Reconcile is refused (409), not started, while SOP reports a changed
// executed task still pending approval.
func TestReconcileRefusedWhilePending(t *testing.T) {
	srv, id, root := newRunServer(t, "PLANNED", nil)
	writePlanSource(t, root)
	writeChangedListing(t, root, "x1")

	if code := postWithCSRF(t, srv, "/projects/"+id+"/commands/reconcile"); code != 409 {
		t.Fatalf("reconcile with a pending changed task = %d, want 409", code)
	}
}

// Once SOP reports no changed executed task still pending, reconcile proceeds
// to delegate to SOP as before.
func TestReconcileProceedsWhenNothingPending(t *testing.T) {
	srv, id, root := newRunServer(t, "PLANNED", nil)
	writePlanSource(t, root)
	writeChangedListing(t, root)

	if code := postWithCSRF(t, srv, "/projects/"+id+"/commands/reconcile"); code != 200 {
		t.Fatalf("reconcile with nothing pending = %d, want 200", code)
	}
}

// pollCommandDone polls a command status URL until it stops running (or the
// deadline passes) and returns the final fragment body.
func pollCommandDone(t *testing.T, srv string, path string) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		_, body := get(t, srv+path)
		if !strings.Contains(body, "cmd-running") {
			return body
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("command did not finish before deadline")
	return ""
}

// An accept-changed approval naming a task SOP did not report in its changed
// set is rejected with ErrTaskNotInChangedSet before any mutation, never a
// fabricated success.
func TestAcceptChangedTaskRejectsUnknownTask(t *testing.T) {
	srv, id, root := newRunServer(t, "PLANNED", nil)
	writePlanSource(t, root)
	writeChangedListing(t, root, "x1")

	code := postWithCSRF(t, srv, "/projects/"+id+"/tasks/unknown-task/commands/accept-changed")
	if code != 200 {
		// taskCommand always answers 200 with a "running"/terminal status fragment;
		// the rejection shows up in the polled command state, asserted below.
		t.Fatalf("POST accept-changed = %d", code)
	}
	body := pollCommandDone(t, srv.URL, "/projects/"+id+"/tasks/unknown-task/commands/accept-changed")
	if !strings.Contains(body, "cmd-error") || !strings.Contains(body, "reported changed executed task set") {
		t.Errorf("accept-changed for an unknown task should fail with ErrTaskNotInChangedSet:\n%s", body)
	}
}

// An accept-changed approval submitted when SOP reports no changed set at all
// is rejected with ErrChangedTasksNotReported before any mutation.
func TestAcceptChangedTaskRejectsWhenSetNotReported(t *testing.T) {
	srv, id, root := newRunServer(t, "PLANNED", nil)
	writePlanSource(t, root)
	// No changed_listing.json: an observed EMPTY listing. A task not listed is
	// therefore rejected with ErrTaskNotInChangedSet.

	postWithCSRF(t, srv, "/projects/"+id+"/tasks/x1/commands/accept-changed")
	body := pollCommandDone(t, srv.URL, "/projects/"+id+"/tasks/x1/commands/accept-changed")
	if !strings.Contains(body, "cmd-error") || !strings.Contains(body, "reported changed executed task set") {
		t.Errorf("accept-changed with no reported changed task should fail with ErrTaskNotInChangedSet:\n%s", body)
	}
}

// An accept-changed approval submitted when no active plan is recorded is
// rejected with ErrNoActivePlan before any mutation.
func TestAcceptChangedTaskRejectsWithNoActivePlan(t *testing.T) {
	srv, id, _ := newRunServer(t, "PLANNED", nil)

	postWithCSRF(t, srv, "/projects/"+id+"/tasks/x1/commands/accept-changed")
	body := pollCommandDone(t, srv.URL, "/projects/"+id+"/tasks/x1/commands/accept-changed")
	if !strings.Contains(body, "cmd-error") || !strings.Contains(body, "no active plan recorded") {
		t.Errorf("accept-changed with no active plan should fail with ErrNoActivePlan:\n%s", body)
	}
}

// A valid accept-changed approval (active plan, reported set, task present)
// delegates to SOP as `sop reconcile <PLAN.md> --accept-changed <id>`. Against
// the fake sop (which exits 0) that surfaces as a completed command, never a
// fabricated success on a real failure and never the old unsupported gap.
func TestAcceptChangedTaskDelegatesWhenValid(t *testing.T) {
	srv, id, root := newRunServer(t, "PLANNED", nil)
	writePlanSource(t, root)
	writeChangedListing(t, root, "x1")

	postWithCSRF(t, srv, "/projects/"+id+"/tasks/x1/commands/accept-changed")
	body := pollCommandDone(t, srv.URL, "/projects/"+id+"/tasks/x1/commands/accept-changed")
	if strings.Contains(body, "cmd-error") {
		t.Errorf("a valid accept-changed approval should delegate to SOP (fake exits 0), not error:\n%s", body)
	}
	if !strings.Contains(body, "cmd-done") {
		t.Errorf("a valid accept-changed approval should render a completed state:\n%s", body)
	}
}

// A GET of the changed-task panel performs no mutation: no accept-changed
// command is started by viewing.
func TestViewingChangedTasksDoesNotAccept(t *testing.T) {
	srv, id, root := newRunServer(t, "PLANNED", nil)
	writePlanSource(t, root)
	writeChangedListing(t, root, "x1")

	code, body := get(t, srv.URL+"/projects/"+id)
	if code != 200 {
		t.Fatalf("GET project = %d", code)
	}
	if !strings.Contains(body, "x1") {
		t.Fatalf("expected the changed task to be listed:\n%s", body)
	}
	// The per-task accept-changed control is now offered (boundary reports it
	// supported); viewing must not have started any accept-changed command, so the
	// status fragment still resolves to idle for that task.
	_, st := get(t, srv.URL+"/projects/"+id+"/tasks/x1/commands/accept-changed")
	if !strings.Contains(st, "cmd-idle") {
		t.Errorf("viewing the changed-task panel started a mutation (status not idle):\n%s", st)
	}
}
