package domain

import "testing"

// TestApprovalStatusResolved pins the resolved semantics: an explicit decision
// (APPROVED/DECLINED) and an operator supersession (SUPERSEDED) are resolved; a
// still-pending request and the zero value are not.
func TestApprovalStatusResolved(t *testing.T) {
	cases := []struct {
		status ApprovalStatus
		want   bool
	}{
		{ApprovalPending, false},
		{ApprovalApproved, true},
		{ApprovalDeclined, true},
		{ApprovalSuperseded, true},
		{ApprovalStatus(""), false},
	}
	for _, tc := range cases {
		if got := tc.status.Resolved(); got != tc.want {
			t.Errorf("%q.Resolved() = %t, want %t", tc.status, got, tc.want)
		}
	}
}

// TestApprovalSupersededValue pins the explicit wire value so a persisted
// SUPERSEDED head is stable.
func TestApprovalSupersededValue(t *testing.T) {
	if got := string(ApprovalSuperseded); got != "SUPERSEDED" {
		t.Fatalf("ApprovalSuperseded = %q, want SUPERSEDED", got)
	}
}
