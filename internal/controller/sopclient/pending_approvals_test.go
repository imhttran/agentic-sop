package sopclient

import (
	"context"
	"testing"
	"time"
)

// PendingApprovals aggregates SOP's applicable gates across every observed
// project, keeping only entries SOP reports as applicable (the fixture's
// completed/stale entries are omitted) and preserving the client's project
// order.
func TestClientPendingApprovalsAggregates(t *testing.T) {
	rootA := t.TempDir()
	initProjectAt(t, rootA, "alpha")
	rootB := t.TempDir()
	initProjectAt(t, rootB, "beta")

	c, err := New([]string{rootA, rootB}, fakeStdoutSop(t, approvalsFixture), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	got := c.PendingApprovals(context.Background())
	if len(got) != 2 {
		t.Fatalf("projects = %d, want 2", len(got))
	}
	if got[0].ProjectName != "alpha" || got[1].ProjectName != "beta" {
		t.Errorf("order/names = %q,%q want alpha,beta", got[0].ProjectName, got[1].ProjectName)
	}
	for _, pa := range got {
		if !pa.Reported {
			t.Errorf("%s Reported = false, want true", pa.ProjectID)
		}
		if len(pa.Entries) != 1 || pa.Entries[0].TaskID != "t-open" {
			t.Errorf("%s entries = %+v, want only the applicable t-open gate", pa.ProjectID, pa.Entries)
		}
	}
}

// A failed `sop approvals --json` is an unread listing (Reported=false), never
// an empty success: a failing SOP must be distinguishable from a quiet one.
func TestClientPendingApprovalsUnreadIsNotQuiet(t *testing.T) {
	root := newProject(t)
	c, err := New([]string{root}, fakeFailingSop(t), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	got := c.PendingApprovals(context.Background())
	if len(got) != 1 {
		t.Fatalf("projects = %d, want 1", len(got))
	}
	if got[0].Reported {
		t.Error("Reported = true for a failed listing; a failing SOP must not look like no gates")
	}
	if len(got[0].Entries) != 0 {
		t.Errorf("entries = %+v, want none for an unread listing", got[0].Entries)
	}
}

// A well-formed empty listing is Reported with no entries (SOP affirmatively
// reported no gates), distinct from an unread listing.
func TestClientPendingApprovalsEmptyListingReported(t *testing.T) {
	root := newProject(t)
	c, err := New([]string{root}, fakeStdoutSop(t, `{"approvals":[]}`), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	got := c.PendingApprovals(context.Background())
	if len(got) != 1 || !got[0].Reported {
		t.Fatalf("got = %+v, want one reported project", got)
	}
	if len(got[0].Entries) != 0 {
		t.Errorf("entries = %+v, want none", got[0].Entries)
	}
}
