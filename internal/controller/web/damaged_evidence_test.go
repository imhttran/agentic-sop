package web

import (
	"strings"
	"testing"
)

// Damaged evidence is rendered as damage, never as the absence the page would
// otherwise report: a corrupt validation.json/state.json is not "SOP persisted no
// evidence".
func TestTaskPageReportsDamagedEvidence(t *testing.T) {
	srv, id, root := newTestServerRoot(t, "sop")
	writeRun(t, root, "t2", "validation.json", `{"truncated": `)
	writeRun(t, root, "t2", "review.json", `{"truncated": `)
	writeRun(t, root, "t2", "state.json", `{"truncated": `)

	code, body := get(t, srv.URL+"/projects/"+id+"/tasks/t2")
	if code != 200 {
		t.Fatalf("task page = %d, want 200", code)
	}
	for _, want := range []string{
		"Validation evidence could not be read",
		"Review evidence could not be read",
		"validation.json",
		"review.json",
		"state.json",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("task page missing %q", want)
		}
	}
	for _, unwanted := range []string{
		"SOP persisted no validation evidence",
		"SOP persisted no review evidence",
	} {
		if strings.Contains(body, unwanted) {
			t.Errorf("task page claims absence despite damaged evidence: %q", unwanted)
		}
	}
}

// Activity lines SOP wrote but could not be parsed are counted on the page, so a
// partial history is not presented as the whole history.
func TestTaskPageReportsSkippedActivity(t *testing.T) {
	srv, id, root := newTestServerRoot(t, "sop")
	writeRun(t, root, "t2", "activity.jsonl",
		`{"stage":"IMPLEMENTING","action":"mutate","detail":"edited a file","timestamp":"2026-11-02T10:00:00Z"}`+"\n"+`{"stage":"BROKEN"`+"\n")

	code, body := get(t, srv.URL+"/projects/"+id+"/tasks/t2")
	if code != 200 {
		t.Fatalf("task page = %d, want 200", code)
	}
	if !strings.Contains(body, "could not be parsed") {
		t.Errorf("task page should report the skipped activity line")
	}
	if !strings.Contains(body, "edited a file") {
		t.Errorf("task page should still show the readable activity")
	}
}
