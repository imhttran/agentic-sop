package outcome

import (
	"errors"
	"testing"
)

func TestIsDesignatedApprover(t *testing.T) {
	cases := []struct {
		name       string
		designated []string
		want       bool
	}{
		{"operator", []string{"operator"}, true},
		{" operator ", []string{"operator"}, true},
		{"operator", nil, false},
		{"Operator", []string{"operator"}, false}, // case-sensitive
		{"", []string{"operator"}, false},
		{"mallory", []string{"operator"}, false},
	}
	for _, c := range cases {
		if got := IsDesignatedApprover(c.name, c.designated); got != c.want {
			t.Errorf("IsDesignatedApprover(%q, %v) = %v, want %v", c.name, c.designated, got, c.want)
		}
	}
}

func TestValidateGrantFailsClosed(t *testing.T) {
	designated := []string{"operator"}
	ok := GrantRequest{Task: "T1", CriterionID: "deliverable-1", Reason: "r", Approver: "operator"}
	if err := ValidateGrant(ok, designated, "2026-10-12T00:00:00Z"); err != nil {
		t.Fatalf("valid grant rejected: %v", err)
	}

	bad := []struct {
		name string
		req  GrantRequest
		ts   string
	}{
		{"no task", GrantRequest{CriterionID: "c", Reason: "r", Approver: "operator"}, "t"},
		{"no criterion", GrantRequest{Task: "T1", Reason: "r", Approver: "operator"}, "t"},
		{"no approver", GrantRequest{Task: "T1", CriterionID: "c", Reason: "r"}, "t"},
		{"not designated", GrantRequest{Task: "T1", CriterionID: "c", Reason: "r", Approver: "mallory"}, "t"},
		{"no reason", GrantRequest{Task: "T1", CriterionID: "c", Approver: "operator"}, "t"},
		{"no timestamp", ok, ""},
	}
	for _, c := range bad {
		if err := ValidateGrant(c.req, designated, c.ts); err == nil {
			t.Errorf("%s: ValidateGrant accepted an invalid grant", c.name)
		} else if (c.name == "no approver" || c.name == "not designated") && !errors.Is(err, ErrNoDesignatedApprover) {
			t.Errorf("%s: error should wrap ErrNoDesignatedApprover: %v", c.name, err)
		}
	}
}

func TestWaiverFromRecords(t *testing.T) {
	if WaiverFromRecords(nil) != nil {
		t.Fatal("no records should yield no waiver")
	}
	recs := []WaiverRecord{
		{Task: "T1", CriterionID: "deliverable-1", Reason: "first", Approver: "a", RecordedAt: "t1"},
		{Task: "T1", CriterionID: "deliverable-2", Reason: "second", Approver: "b", RecordedAt: "t2"},
		{Task: "T1", CriterionID: "deliverable-1", Reason: "dup", Approver: "c", RecordedAt: "t3"},
	}
	w := WaiverFromRecords(recs)
	if w == nil {
		t.Fatal("records should yield a waiver")
	}
	if len(w.Criteria) != 2 {
		t.Fatalf("waiver should name the union of criteria, got %v", w.Criteria)
	}
	// The most recent record supplies attribution/reason/timestamp.
	if w.Approver != "c" || w.Reason != "dup" || w.RecordedAt != "t3" {
		t.Fatalf("latest record should win: %+v", w)
	}
}
