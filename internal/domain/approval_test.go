package domain

import (
	"encoding/json"
	"testing"
	"time"
)

// TestApprovalStatusResolved pins the resolved semantics: an explicit decision
// (APPROVED/DECLINED) and an operator closure (SUPERSEDED/INVALIDATED) are
// resolved; a still-pending request and the zero value are not.
func TestApprovalStatusResolved(t *testing.T) {
	cases := []struct {
		status ApprovalStatus
		want   bool
	}{
		{ApprovalPending, false},
		{ApprovalApproved, true},
		{ApprovalDeclined, true},
		{ApprovalSuperseded, true},
		{ApprovalInvalidated, true},
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

// TestApprovalInvalidatedValue pins the explicit wire value so a persisted
// INVALIDATED head is stable.
func TestApprovalInvalidatedValue(t *testing.T) {
	if got := string(ApprovalInvalidated); got != "INVALIDATED" {
		t.Fatalf("ApprovalInvalidated = %q, want INVALIDATED", got)
	}
}

// TestApprovalInvalidatedIsDistinct pins that INVALIDATED is its own status: it
// must never collide with a human acceptance or any other resolved status, so no
// consumer can read it as APPROVED.
func TestApprovalInvalidatedIsDistinct(t *testing.T) {
	distinct := []ApprovalStatus{ApprovalPending, ApprovalApproved, ApprovalDeclined, ApprovalSuperseded}
	for _, other := range distinct {
		if ApprovalInvalidated == other {
			t.Fatalf("ApprovalInvalidated == %q, want a distinct status", other)
		}
	}
	if ApprovalInvalidated.Resolved() != true {
		t.Fatalf("ApprovalInvalidated.Resolved() = false, want true")
	}
}

// TestApprovalRequestNotInvalidated pins that a request that was never
// invalidated has a nil Invalidation and marshals no invalidation field.
func TestApprovalRequestNotInvalidated(t *testing.T) {
	req := ApprovalRequest{ID: "APR-1", TaskID: "T1", Status: ApprovalPending}
	if req.Invalidation != nil {
		t.Fatalf("fresh request has Invalidation = %+v, want nil", req.Invalidation)
	}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := fields["invalidation"]; ok {
		t.Fatalf("marshaled request has an invalidation field: %s", raw)
	}
}

// TestApprovalInvalidationRecordFields pins that the record carries the operator
// reason and identity, the invalidation time, the plan source fingerprints on
// both sides, the reconciliation time, and the definition digests on both sides.
func TestApprovalInvalidationRecordFields(t *testing.T) {
	at := time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)
	rec := ApprovalInvalidation{
		Reason:                 "plan revised after approval",
		InvalidatedBy:          "operator",
		InvalidatedAt:          at,
		PlanFingerprintBefore:  "plan-before",
		PlanFingerprintAfter:   "plan-after",
		DefinitionAcceptedAt:   at.Add(time.Minute),
		DefinitionDigestBefore: "digest-before",
		DefinitionDigestAfter:  "digest-after",
	}
	if rec.Reason != "plan revised after approval" || rec.InvalidatedBy != "operator" {
		t.Fatalf("operator reason/identity not carried: %+v", rec)
	}
	if rec.InvalidatedAt.IsZero() || rec.DefinitionAcceptedAt.IsZero() {
		t.Fatalf("timestamps not carried: %+v", rec)
	}
	if rec.PlanFingerprintBefore == "" || rec.PlanFingerprintAfter == "" {
		t.Fatalf("plan fingerprints not carried: %+v", rec)
	}
	if rec.DefinitionDigestBefore == "" || rec.DefinitionDigestAfter == "" {
		t.Fatalf("definition digests not carried: %+v", rec)
	}
}

// TestApprovalInvalidationDoesNotSetDecision pins that an invalidated request
// never sets the approval decision or lifecycle action fields.
func TestApprovalInvalidationDoesNotSetDecision(t *testing.T) {
	req := ApprovalRequest{
		ID:           "APR-1",
		TaskID:       "T1",
		Status:       ApprovalInvalidated,
		Invalidation: &ApprovalInvalidation{Reason: "stale", InvalidatedBy: "operator"},
	}
	if req.Decision != nil {
		t.Fatalf("invalidated request has Decision = %+v, want nil", req.Decision)
	}
	if req.Status == ApprovalApproved || req.Status == ApprovalDeclined || req.Status == ApprovalSuperseded {
		t.Fatalf("invalidated request status = %q, want a distinct status", req.Status)
	}
	if !req.Status.Resolved() {
		t.Fatalf("invalidated request is not resolved")
	}
}

// TestApprovalInvalidationRoundTrip pins that JSON round-trip preserves every
// invalidation field and the request DefinitionDigest.
func TestApprovalInvalidationRoundTrip(t *testing.T) {
	at := time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)
	req := ApprovalRequest{
		ID:               "APR-1",
		TaskID:           "T1",
		Status:           ApprovalInvalidated,
		DefinitionDigest: "digest-before",
		Invalidation: &ApprovalInvalidation{
			Reason:                 "plan revised",
			InvalidatedBy:          "operator",
			InvalidatedAt:          at,
			PlanFingerprintBefore:  "plan-before",
			PlanFingerprintAfter:   "plan-after",
			DefinitionAcceptedAt:   at.Add(time.Minute),
			DefinitionDigestBefore: "digest-before",
			DefinitionDigestAfter:  "digest-after",
		},
	}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got ApprovalRequest
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.DefinitionDigest != "digest-before" {
		t.Fatalf("DefinitionDigest = %q, want digest-before", got.DefinitionDigest)
	}
	if got.Invalidation == nil {
		t.Fatalf("Invalidation is nil after round-trip")
	}
	if got.Invalidation.Reason != req.Invalidation.Reason ||
		got.Invalidation.InvalidatedBy != req.Invalidation.InvalidatedBy ||
		!got.Invalidation.InvalidatedAt.Equal(req.Invalidation.InvalidatedAt) ||
		got.Invalidation.PlanFingerprintBefore != req.Invalidation.PlanFingerprintBefore ||
		got.Invalidation.PlanFingerprintAfter != req.Invalidation.PlanFingerprintAfter ||
		!got.Invalidation.DefinitionAcceptedAt.Equal(req.Invalidation.DefinitionAcceptedAt) ||
		got.Invalidation.DefinitionDigestBefore != req.Invalidation.DefinitionDigestBefore ||
		got.Invalidation.DefinitionDigestAfter != req.Invalidation.DefinitionDigestAfter {
		t.Fatalf("Invalidation changed across round-trip: %+v != %+v", got.Invalidation, req.Invalidation)
	}
}

// TestApprovalRequestDefinitionDigest pins that a request raised against a known
// task definition stores and reloads that definition's digest.
func TestApprovalRequestDefinitionDigest(t *testing.T) {
	req := ApprovalRequest{ID: "APR-1", TaskID: "T1", Status: ApprovalPending, DefinitionDigest: "sha256:abc"}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got ApprovalRequest
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.DefinitionDigest != "sha256:abc" {
		t.Fatalf("DefinitionDigest = %q, want sha256:abc", got.DefinitionDigest)
	}
}
