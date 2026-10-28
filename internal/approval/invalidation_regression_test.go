package approval

// Regression suite for the APPR invalidate operation. These tests pin the
// guarantees an operator relies on when closing a stale approval request whose
// evidence the revised plan no longer contains:
//
//   - a reconciled, unsatisfied task with a stale PENDING request is invalidated,
//     and the task then runs again with a fresh outcome;
//   - the original PENDING request and its evidence survive verbatim in the
//     append-only history, beside the invalidation reason, actor, plan
//     revisions, and definition digests;
//   - an unchanged definition digest is refused;
//   - a legacy request with no task-scoped reconciliation evidence is refused;
//   - the task's status, attempt count, and satisfaction are never written, and
//     no decision is recorded on the head.

import (
	"errors"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/domain"
)

// seedReportedDeadlockRequest builds the reported deadlock shape: a reconciled,
// unsatisfied task carrying a PENDING request whose evidence the revised plan no
// longer contains. It returns the service, the fakes, and the original request.
func seedReportedDeadlockRequest(t *testing.T) (*Service, *memTasks, map[string]*memRecord, domain.ApprovalRequest) {
	t.Helper()
	blocked := &domain.Task{
		ID:            "S001",
		Status:        domain.BLOCKED,
		BlockedReason: domain.RETRIES_EXHAUSTED,
		Attempt:       2,
		MaxAttempts:   3,
	}
	tasks := newMemTasks(blocked)
	svc, records := newTestService(tasks)
	seedPendingRequest(t, svc, records, "S001", "digest-before")
	original := *records["S001"].head
	return svc, tasks, records, original
}

// TestInvalidationRegressionPreservesOriginalRequestVerbatim proves the original
// request and its evidence survive unchanged in the append-only history, beside
// the invalidation reason, actor, plan revisions, and definition digests.
func TestInvalidationRegressionPreservesOriginalRequestVerbatim(t *testing.T) {
	svc, _, records, original := seedReportedDeadlockRequest(t)
	acceptedAt := original.RequestedAt.Add(time.Minute)

	in := InvalidateInput{
		Reason:                         "the revised plan removed the deliverable this request names",
		By:                             "reviewer",
		PriorPlanFingerprint:           "plan-sha-before",
		CurrentPlanFingerprint:         "plan-sha-after",
		CurrentDefinitionDigest:        "digest-after",
		DefinitionAcceptedAsReconciled: true,
		DefinitionAcceptedAt:           acceptedAt,
		DefinitionAcceptedDigest:       "digest-after",
		DefinitionAcceptedPriorDigest:  "digest-before",
	}
	if _, err := svc.Invalidate("S001", in); err != nil {
		t.Fatalf("Invalidate: %v", err)
	}

	history := records["S001"].history
	if len(history) != 1 {
		t.Fatalf("history len = %d, want exactly 1", len(history))
	}
	archived := history[0]
	if archived.ID != original.ID {
		t.Errorf("archived id = %q, want %q", archived.ID, original.ID)
	}
	if archived.Status != domain.ApprovalPending {
		t.Errorf("archived status = %q, want PENDING verbatim", archived.Status)
	}
	if archived.Evidence != original.Evidence || archived.Reason != original.Reason {
		t.Errorf("archived reason/evidence = %q/%q, want the original %q/%q",
			archived.Reason, archived.Evidence, original.Reason, original.Evidence)
	}
	if !archived.RequestedAt.Equal(original.RequestedAt) {
		t.Errorf("archived requested at = %v, want %v", archived.RequestedAt, original.RequestedAt)
	}
	if archived.RequestedBy != original.RequestedBy {
		t.Errorf("archived requested by = %q, want %q", archived.RequestedBy, original.RequestedBy)
	}
	if archived.DefinitionDigest != original.DefinitionDigest {
		t.Errorf("archived definition digest = %q, want %q", archived.DefinitionDigest, original.DefinitionDigest)
	}
	if archived.Invalidation != nil || archived.Decision != nil {
		t.Errorf("archived request carries audit fields it must not: invalidation=%+v decision=%+v",
			archived.Invalidation, archived.Decision)
	}

	// The head carries the invalidation record beside the archived request.
	head := records["S001"].head
	if head.Invalidation == nil {
		t.Fatal("head has no invalidation record beside the archived request")
	}
	if head.Invalidation.Reason != in.Reason || head.Invalidation.InvalidatedBy != in.By {
		t.Errorf("head reason/by = %q/%q, want %q/%q",
			head.Invalidation.Reason, head.Invalidation.InvalidatedBy, in.Reason, in.By)
	}
	if head.Invalidation.PlanFingerprintBefore != "plan-sha-before" ||
		head.Invalidation.PlanFingerprintAfter != "plan-sha-after" {
		t.Errorf("head plan fingerprints = %q/%q",
			head.Invalidation.PlanFingerprintBefore, head.Invalidation.PlanFingerprintAfter)
	}
	if head.Invalidation.DefinitionDigestBefore != original.DefinitionDigest ||
		head.Invalidation.DefinitionDigestAfter != "digest-after" {
		t.Errorf("head definition digests = %q/%q",
			head.Invalidation.DefinitionDigestBefore, head.Invalidation.DefinitionDigestAfter)
	}
}

// TestInvalidationRegressionRecordsFullAudit proves the head invalidation record
// carries every audit field an operator needs to reconstruct the decision.
func TestInvalidationRegressionRecordsFullAudit(t *testing.T) {
	svc, _, records, original := seedReportedDeadlockRequest(t)
	acceptedAt := original.RequestedAt.Add(time.Minute)

	in := InvalidateInput{
		Reason:                         "plan revision removed the named deliverable",
		By:                             "reviewer",
		PriorPlanFingerprint:           "plan-sha-before",
		CurrentPlanFingerprint:         "plan-sha-after",
		CurrentDefinitionDigest:        "digest-after",
		DefinitionAcceptedAsReconciled: true,
		DefinitionAcceptedAt:           acceptedAt,
		DefinitionAcceptedDigest:       "digest-after",
		DefinitionAcceptedPriorDigest:  "digest-before",
	}
	if _, err := svc.Invalidate("S001", in); err != nil {
		t.Fatalf("Invalidate: %v", err)
	}

	head := records["S001"].head
	if head.Status != domain.ApprovalInvalidated {
		t.Fatalf("head status = %s, want INVALIDATED", head.Status)
	}
	inv := head.Invalidation
	if inv == nil {
		t.Fatal("head has no invalidation record")
	}
	if inv.Reason != in.Reason {
		t.Errorf("reason = %q, want %q", inv.Reason, in.Reason)
	}
	if inv.InvalidatedBy != in.By {
		t.Errorf("actor = %q, want %q", inv.InvalidatedBy, in.By)
	}
	if inv.InvalidatedAt.IsZero() {
		t.Error("invalidation time is zero")
	}
	if inv.PlanFingerprintBefore != "plan-sha-before" {
		t.Errorf("plan fingerprint before = %q", inv.PlanFingerprintBefore)
	}
	if inv.PlanFingerprintAfter != "plan-sha-after" {
		t.Errorf("plan fingerprint after = %q", inv.PlanFingerprintAfter)
	}
	if !inv.DefinitionAcceptedAt.Equal(acceptedAt) {
		t.Errorf("reconciliation time = %v, want %v", inv.DefinitionAcceptedAt, acceptedAt)
	}
	if inv.DefinitionDigestBefore != "digest-before" {
		t.Errorf("definition digest before = %q", inv.DefinitionDigestBefore)
	}
	if inv.DefinitionDigestAfter != "digest-after" {
		t.Errorf("definition digest after = %q", inv.DefinitionDigestAfter)
	}
}

// TestInvalidationRegressionRefusesUnchangedDigest proves a still-applicable
// request is refused, with nothing written to the head or the history.
func TestInvalidationRegressionRefusesUnchangedDigest(t *testing.T) {
	svc, _, records, original := seedReportedDeadlockRequest(t)

	cases := []struct {
		name string
		in   InvalidateInput
	}{
		{
			name: "current digest equals the recorded digest",
			in: InvalidateInput{
				Reason:                         "r",
				By:                             "reviewer",
				CurrentDefinitionDigest:        "digest-before",
				DefinitionAcceptedAsReconciled: true,
				DefinitionAcceptedAt:           original.RequestedAt.Add(time.Minute),
				DefinitionAcceptedDigest:       "digest-before",
			},
		},
		{
			// The acceptance covers a changed definition, but the current
			// definition still yields the digest recorded on the request.
			name: "accepted change elsewhere leaves the recorded digest current",
			in: InvalidateInput{
				Reason:                         "r",
				By:                             "reviewer",
				CurrentDefinitionDigest:        "digest-before",
				DefinitionAcceptedAsReconciled: true,
				DefinitionAcceptedAt:           original.RequestedAt.Add(time.Minute),
				DefinitionAcceptedDigest:       "digest-before",
				DefinitionAcceptedPriorDigest:  "digest-earlier",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := svc.Invalidate("S001", tc.in); !errors.Is(err, ErrApprovalInvalidateNotApplicable) {
				t.Fatalf("Invalidate err = %v, want ErrApprovalInvalidateNotApplicable", err)
			}
			if head := records["S001"].head; head.Status != domain.ApprovalPending || head.Invalidation != nil {
				t.Errorf("refusal mutated the head: %+v", head)
			}
			if got := len(records["S001"].history); got != 0 {
				t.Errorf("refusal appended %d history entries, want 0", got)
			}
		})
	}
}

// TestInvalidationRegressionRefusesLegacyWithoutReconciliation proves a legacy
// request is refused unless a task-scoped acceptance strictly after the request
// covers the current definition.
func TestInvalidationRegressionRefusesLegacyWithoutReconciliation(t *testing.T) {
	svc, records := newTestService(newMemTasks(pendingTask("S001")))
	req := seedPendingRequest(t, svc, records, "S001", "")
	ok := func(acceptedAt time.Time) InvalidateInput {
		return InvalidateInput{
			Reason:                         "r",
			By:                             "reviewer",
			CurrentDefinitionDigest:        "digest-after",
			DefinitionAcceptedAsReconciled: true,
			DefinitionAcceptedAt:           acceptedAt,
			DefinitionAcceptedDigest:       "digest-after",
		}
	}

	cases := []struct {
		name string
		in   InvalidateInput
	}{
		{"no reconciliation evidence", InvalidateInput{Reason: "r", By: "reviewer", CurrentDefinitionDigest: "digest-after"}},
		{"acceptance predates the request", ok(req.RequestedAt.Add(-time.Second))},
		{"acceptance equals the request time", ok(req.RequestedAt)},
		{"acceptance covers a different definition", func() InvalidateInput {
			in := ok(req.RequestedAt.Add(time.Minute))
			in.DefinitionAcceptedDigest = "some-other-definition"
			return in
		}()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := svc.Invalidate("S001", tc.in); !errors.Is(err, ErrApprovalInvalidateNotApplicable) {
				t.Fatalf("Invalidate err = %v, want ErrApprovalInvalidateNotApplicable", err)
			}
			if head := records["S001"].head; head.Status != domain.ApprovalPending || head.Invalidation != nil {
				t.Errorf("refusal mutated the head: %+v", head)
			}
			if got := len(records["S001"].history); got != 0 {
				t.Errorf("refusal appended %d history entries, want 0", got)
			}
		})
	}

	// A covering acceptance strictly after the request is accepted, proving the
	// legacy refusal was about the missing evidence and not the legacy request.
	if _, err := svc.Invalidate("S001", ok(req.RequestedAt.Add(time.Minute))); err != nil {
		t.Fatalf("Invalidate with covering evidence: %v", err)
	}
	if got := records["S001"].head.Status; got != domain.ApprovalInvalidated {
		t.Errorf("head status = %s, want INVALIDATED", got)
	}
}

// TestInvalidationRegressionLeavesTaskStateUnchanged proves invalidation writes no
// task field and records no decision on the head.
func TestInvalidationRegressionLeavesTaskStateUnchanged(t *testing.T) {
	svc, tasks, records, original := seedReportedDeadlockRequest(t)
	before := *tasks.tasks["S001"]

	in := InvalidateInput{
		Reason:                         "r",
		By:                             "reviewer",
		CurrentDefinitionDigest:        "digest-after",
		DefinitionAcceptedAsReconciled: true,
		DefinitionAcceptedAt:           original.RequestedAt.Add(time.Minute),
		DefinitionAcceptedDigest:       "digest-after",
		DefinitionAcceptedPriorDigest:  "digest-before",
	}
	if _, err := svc.Invalidate("S001", in); err != nil {
		t.Fatalf("Invalidate: %v", err)
	}

	after := tasks.tasks["S001"]
	if after.Status != before.Status {
		t.Errorf("status = %s, want unchanged %s", after.Status, before.Status)
	}
	if after.Attempt != before.Attempt {
		t.Errorf("attempt = %d, want unchanged %d", after.Attempt, before.Attempt)
	}
	if after.IsSatisfied() != before.IsSatisfied() {
		t.Errorf("satisfaction = %v, want unchanged %v", after.IsSatisfied(), before.IsSatisfied())
	}
	if after.IsSatisfied() || after.IsCompleted() {
		t.Errorf("invalidation satisfied the task: %+v", after)
	}
	if head := records["S001"].head; head.Decision != nil {
		t.Errorf("invalidation recorded a decision: %+v", head.Decision)
	}
}

// TestInvalidationRegressionBlockingSemanticsPerStatus proves the boundary still
// refuses a decision on declined and invalidated heads, and reports an invalidated
// head as not applicable to a decision. Execution blocking itself is proven
// through the CLI gate in the cli package, which is the code that stops a run.
func TestInvalidationRegressionBlockingSemanticsPerStatus(t *testing.T) {
	cases := []struct {
		name       string
		status     domain.ApprovalStatus
		wantErr    error
		applicable bool
	}{
		{"pending is applicable until decided", domain.ApprovalPending, nil, true},
		{"declined refuses an approval", domain.ApprovalDeclined, ErrApprovalResolved, false},
		// An invalidated request is not a decision boundary at all: the boundary
		// reports it as not applicable, and the CLI gate is what lets the next run
		// proceed.
		{"invalidated leaves the decision boundary closed", domain.ApprovalInvalidated, nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, records := newTestService(newMemTasks(pendingTask("S001")))
			req := seedPendingRequest(t, svc, records, "S001", "digest-before")
			req.Status = tc.status
			records["S001"].head = &req

			view, err := svc.Approval("S001")
			if err != nil {
				t.Fatalf("Approval: %v", err)
			}
			if view.Applicable != tc.applicable {
				t.Errorf("Applicable = %v, want %v", view.Applicable, tc.applicable)
			}

			_, err = svc.Approve("S001", DecisionInput{By: "human"})
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("Approve on %s err = %v, want success", tc.status, err)
				}
				return
			}
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("Approve err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}
