package sopclient

import (
	"context"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/controller/config"
)

// TestClientProjectionsConsumeSopApprovalsListing is the regression test for the
// wiring defect the C2-009 dogfood uncovered: the controller's gate projections
// must consume SOP's authoritative `sop approvals --json` listing.
//
// The project carries NO persisted approvals.json artifact, because real SOP
// never writes one - `sop approvals --json` prints the listing to stdout only. A
// projection that read an artifact instead of asking SOP would therefore surface
// no gate at all, even when SOP reports one. This proves the live listing is the
// source for both the list/project projection and the task detail.
func TestClientProjectionsConsumeSopApprovalsListing(t *testing.T) {
	root := newProject(t)
	now := time.Now().UTC().Format(time.RFC3339)
	seed(t, root, `INSERT INTO tasks VALUES ('t9','Gated','o','a','PLANNED','',0,3,'`+now+`','`+now+`')`)

	listing := `{"version":1,"approvals":[{"task_id":"t9","kind":"NEEDS_HUMAN","target":"t9","reason":"needs a human","evidence":"","stage":"WAITING_FOR_HUMAN","disposition":"NEEDS_HUMAN","status":"PENDING","requested_at":"2026-01-01T00:00:00Z","task_status":"PLANNED"}]}`
	bin, args := fakeListingSop(t, listing)
	c, err := New([]string{root}, bin, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	pid := config.ProjectID(root)
	ctx := context.Background()

	proj, err := c.Project(ctx, pid)
	if err != nil {
		t.Fatalf("Project: %v", err)
	}
	if proj.Summary.NeedsAttention != 1 {
		t.Errorf("Project.Summary.NeedsAttention = %d, want 1 (the gate from sop approvals --json)", proj.Summary.NeedsAttention)
	}
	var listGate bool
	for _, ts := range proj.Tasks {
		if ts.ID == "t9" {
			listGate = ts.NeedsHuman
			if ts.ApprovalKind != "NEEDS_HUMAN" {
				t.Errorf("TaskSummary.ApprovalKind = %q, want NEEDS_HUMAN", ts.ApprovalKind)
			}
		}
	}
	if !listGate {
		t.Errorf("task t9 not projected as NeedsHuman from SOP's listing")
	}

	detail, err := c.Task(ctx, pid, "t9")
	if err != nil {
		t.Fatalf("Task: %v", err)
	}
	if !detail.Approval.Present {
		t.Errorf("TaskDetail.Approval.Present = false, want true (gate from sop approvals --json)")
	}
	if detail.Approval.Kind != "NEEDS_HUMAN" {
		t.Errorf("Approval.Kind = %q, want NEEDS_HUMAN", detail.Approval.Kind)
	}
	if detail.Approval.TaskStatus != "PLANNED" {
		t.Errorf("Approval.TaskStatus = %q, want SOP's reported PLANNED", detail.Approval.TaskStatus)
	}

	// The controller must have asked SOP for the listing (the fake truncates its
	// argv record per invocation, so the last recorded invocation is the read that
	// produced the projection).
	if got := args(); len(got) == 0 || got[0] != "approvals" {
		t.Errorf("sop argv = %v, want an `approvals --json` invocation", got)
	}
}
