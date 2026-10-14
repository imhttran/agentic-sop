package sopclient

import (
	"context"
	"testing"
	"time"
)

// approvalsFixture is a representative SOP `approvals --json` listing covering a
// verbatim-field entry, an applicable gate, a completed/stale/resolved gate, and
// a free-form reason on each entry.
const approvalsFixture = `{
  "approvals": [
    {
      "task_id": "t-open",
      "kind": "AMBIGUOUS_CONTRACT",
      "target": "t-open",
      "reason": "Two valid contracts remain",
      "evidence": "classification NEEDS_HUMAN",
      "stage": "WAITING_FOR_HUMAN",
      "disposition": "NEEDS_HUMAN",
      "status": "PENDING",
      "requested_at": "2026-02-01T10:00:00Z",
      "task_status": "BLOCKED"
    },
    {
      "task_id": "t-done",
      "kind": "AMBIGUOUS_CONTRACT",
      "target": "t-done",
      "reason": "already answered",
      "evidence": "",
      "stage": "PASSED",
      "disposition": "APPLIED",
      "status": "COMPLETED",
      "requested_at": "2026-02-01T09:00:00Z",
      "task_status": "BLOCKED"
    },
    {
      "task_id": "t-stale",
      "kind": "CONTRACT",
      "target": "t-stale",
      "reason": "superseded by a newer plan",
      "evidence": "",
      "stage": "PASSED",
      "disposition": "RESOLVED",
      "status": "STALE",
      "requested_at": "2026-02-01T08:00:00Z",
      "task_status": "BLOCKED"
    }
  ]
}`

// Decoding SOP's listing yields entries whose field values equal the JSON
// verbatim (no renaming, no derivation).
func TestApprovalsDecodeVerbatim(t *testing.T) {
	listing, ok := decodeApprovals([]byte(approvalsFixture))
	if !ok {
		t.Fatal("decodeApprovals reported malformed for a valid listing")
	}
	if !listing.Reported {
		t.Fatal("listing.Reported = false, want true")
	}
	if len(listing.Entries) != 3 {
		t.Fatalf("entries = %d, want 3", len(listing.Entries))
	}
	e := listing.Entries[0]
	if e.TaskID != "t-open" || e.Kind != "AMBIGUOUS_CONTRACT" || e.Target != "t-open" {
		t.Errorf("task_id/kind/target not verbatim: %+v", e)
	}
	if e.Reason != "Two valid contracts remain" || e.Evidence != "classification NEEDS_HUMAN" {
		t.Errorf("reason/evidence not verbatim: %+v", e)
	}
	if e.Stage != "WAITING_FOR_HUMAN" || e.Disposition != "NEEDS_HUMAN" || e.Status != "PENDING" {
		t.Errorf("stage/disposition/status not verbatim: %+v", e)
	}
	if e.RequestedAt != "2026-02-01T10:00:00Z" || e.TaskStatus != "BLOCKED" {
		t.Errorf("requested_at/task_status not verbatim: %+v", e)
	}
	if !e.Applicable {
		t.Errorf("PENDING entry Applicable = false, want true")
	}
}

// A completed, stale, or resolved gate is not actionable.
func TestApprovalsClosedEntriesNotApplicable(t *testing.T) {
	listing, ok := decodeApprovals([]byte(approvalsFixture))
	if !ok {
		t.Fatal("decode failed")
	}
	byID := map[string]ApprovalEntry{}
	for _, e := range listing.Entries {
		byID[e.TaskID] = e
	}
	for _, id := range []string{"t-done", "t-stale"} {
		if byID[id].Applicable {
			t.Errorf("%s Applicable = true, want false (completed/stale)", id)
		}
		if _, ok := listing.Lookup(id); ok {
			t.Errorf("Lookup(%s) = found, want not found (closed gate is not actionable)", id)
		}
	}
	if _, ok := listing.Lookup("t-open"); !ok {
		t.Error("Lookup(t-open) = not found, want the applicable gate")
	}
}

// applicability reads only structured SOP fields: free-form reason/evidence
// never drive it, and a blank status never manufactures a gate.
func TestApprovalApplicabilityIgnoresProse(t *testing.T) {
	cases := []struct {
		name        string
		status      string
		disposition string
		want        bool
	}{
		{"pending", "PENDING", "", true},
		{"open", "OPEN", "", true},
		{"blank status", "", "", false},
		{"completed", "COMPLETED", "", false},
		{"resolved disposition", "PENDING", "RESOLVED", false},
		{"stale", "STALE", "", false},
		{"applied disposition", "", "APPLIED", false},
	}
	for _, tc := range cases {
		if got := applicableStatus(tc.status, tc.disposition); got != tc.want {
			t.Errorf("%s: applicableStatus(%q,%q) = %v, want %v", tc.name, tc.status, tc.disposition, got, tc.want)
		}
	}
}

// A BLOCKED task with no applicable listing entry reports no approval at all,
// and a non-empty blocked_reason does not create one. A stale/completed entry
// for a BLOCKED task also yields no approval.
func TestApprovalNeverFromStatusOrProse(t *testing.T) {
	root := newProject(t)
	now := time.Now().UTC().Format(time.RFC3339)
	seed(t, root,
		`INSERT INTO tasks VALUES ('blocked','B','o','a','BLOCKED','waiting on a human decision',1,3,'`+now+`','`+now+`')`,
		`INSERT INTO tasks VALUES ('done-gate','D','o','a','BLOCKED','resolved',1,3,'`+now+`','`+now+`')`,
	)
	writeArtifact(t, root, "blocked", "classification.json",
		`{"kind":"AMBIGUOUS_CONTRACT","disposition":"NEEDS_HUMAN","confidence":"HIGH","reason":"needs a human"}`)
	writeArtifact(t, root, "blocked", "state.json", `{"id":"blocked","stage":"WAITING_FOR_HUMAN"}`)
	writeApprovals(t, root, `{"approvals":[{"task_id":"done-gate","kind":"CONTRACT","target":"done-gate","reason":"already answered","evidence":"","stage":"PASSED","disposition":"APPLIED","status":"COMPLETED","requested_at":"2026-01-01T00:00:00Z","task_status":"BLOCKED"}]}`)

	st, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	d, err := st.Task(context.Background(), "blocked", readApprovals(t, st))
	if err != nil {
		t.Fatal(err)
	}
	if d.Approval.Present {
		t.Fatalf("Approval.Present = true for a BLOCKED/NEEDS_HUMAN task with no listing entry: %+v", d.Approval)
	}
	if d.NeedsHuman() {
		t.Fatal("NeedsHuman() = true for a task with no listing entry")
	}

	done, err := st.Task(context.Background(), "done-gate", readApprovals(t, st))
	if err != nil {
		t.Fatal(err)
	}
	if done.Approval.Present {
		t.Fatalf("Approval.Present = true for a completed listing entry: %+v", done.Approval)
	}

	tasks, err := st.Tasks(context.Background(), readApprovals(t, st))
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range tasks {
		if task.NeedsHuman {
			t.Errorf("task %s NeedsHuman = true, want false", task.ID)
		}
	}
}

// Attempt counts and inactivity never produce a gate.
func TestApprovalNeverFromAttemptsOrInactivity(t *testing.T) {
	root := newProject(t)
	stale := time.Now().Add(-72 * time.Hour).UTC().Format(time.RFC3339)
	seed(t, root,
		`INSERT INTO tasks VALUES ('retried','R','o','a','BLOCKED','many attempts',5,5,'`+stale+`','`+stale+`')`,
	)
	writeArtifact(t, root, "retried", "report.json", `{"id":"retried","stage":"FAILED","decision":"FAIL","fix_cycles":5}`)

	st, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	d, err := st.Task(context.Background(), "retried", readApprovals(t, st))
	if err != nil {
		t.Fatal(err)
	}
	if d.Approval.Present {
		t.Fatalf("Approval.Present = true from attempts/inactivity: %+v", d.Approval)
	}
}

// The listing is the single source for BOTH projections: a task marked
// NeedsHuman by Store.Tasks also reports Approval.Present in Store.Task, with
// matching kind/target, over the same fixture.
func TestApprovalsListAndDetailAgree(t *testing.T) {
	root := newProject(t)
	now := time.Now().UTC().Format(time.RFC3339)
	seed(t, root, `INSERT INTO tasks VALUES ('t-open','T','o','a','BLOCKED','x',1,3,'`+now+`','`+now+`')`)
	writeApprovals(t, root, approvalsFixture)

	st, err := OpenStore(root)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	tasks, err := st.Tasks(context.Background(), readApprovals(t, st))
	if err != nil {
		t.Fatal(err)
	}
	var summary TaskSummary
	for _, task := range tasks {
		if task.ID == "t-open" {
			summary = task
		}
	}
	if !summary.NeedsHuman || summary.ApprovalKind != "AMBIGUOUS_CONTRACT" {
		t.Fatalf("summary = %+v, want NeedsHuman/AMBIGUOUS_CONTRACT", summary)
	}

	detail, err := st.Task(context.Background(), "t-open", readApprovals(t, st))
	if err != nil {
		t.Fatal(err)
	}
	if !detail.Approval.Present || detail.Approval.Kind != "AMBIGUOUS_CONTRACT" {
		t.Fatalf("detail.Approval = %+v, want Present/AMBIGUOUS_CONTRACT", detail.Approval)
	}
	if detail.Approval.Target != "t-open" {
		t.Errorf("detail.Approval.Target = %q, want t-open", detail.Approval.Target)
	}
	if summary.NeedsHuman != detail.Approval.Present {
		t.Errorf("list NeedsHuman=%v disagrees with detail Present=%v", summary.NeedsHuman, detail.Approval.Present)
	}
}

// A malformed listing document yields Reported=false (explicit absence), never
// a partial gate list.
func TestApprovalsMalformedIsAbsent(t *testing.T) {
	if _, ok := decodeApprovals([]byte(`{"approvals": not json`)); ok {
		t.Fatal("decodeApprovals accepted malformed JSON")
	}
}
