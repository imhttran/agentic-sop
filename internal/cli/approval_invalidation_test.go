package cli

// Regression suite for the APPR invalidation operation at the CLI boundary. It
// reuses the fixture helpers from approval_invalidate_test.go and pins:
//
//   - the reported deadlock: a reconciled, unsatisfied task with a stale PENDING
//     request is invalidated, and the execution gate then permits the next run;
//   - the original request and its evidence survive verbatim in the append-only
//     history, beside the invalidation reason, actor, plan revisions, and
//     definition digests;
//   - invalidation is refused when the current definition still yields the
//     recorded digest, and when a legacy request has no reconciliation evidence;
//   - an invalidated request leaves the task status, attempt count, and
//     satisfaction unchanged;
//   - pending and declined requests still block execution.

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/domain"
)

// TestInvalidationRegressionDeadlockThenFreshExecution reproduces the reported
// deadlock end to end: a reconciled task whose request is stale is invalidated,
// the gate then permits the next run, and the task records a fresh outcome.
func TestInvalidationRegressionDeadlockThenFreshExecution(t *testing.T) {
	f := newInvalidationFixture(t, invalidationFixtureOptions{
		taskStatus: domain.READY,
		accepted: &acceptedChange{
			TaskID:     "S001",
			AcceptedAt: time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC),
		},
	})

	// Before invalidation the pending request blocks execution: the reported
	// deadlock.
	var out, errb bytes.Buffer
	task := readTask(t, f.dir, "S001")
	if code := approvalExecutionGate(f.dir, task, &out, &errb); code != exitApprovalBlocked {
		t.Fatalf("gate before invalidation = %d, want blocked", code)
	}

	out.Reset()
	errb.Reset()
	if code := runApprovalInvalidate(
		[]string{"S001", "--reason", "the revised plan removed the named deliverable", "--by", "operator"},
		&out, &errb, invalidationDeps(f.dir)); code != exitOK {
		t.Fatalf("invalidate exit = %d, want OK; stderr=%s", code, errb.String())
	}

	// The task is still unsatisfied and no decision was recorded.
	task = readTask(t, f.dir, "S001")
	if task.IsSatisfied() || task.Status != domain.READY {
		t.Fatalf("task = %+v, want unsatisfied READY", task)
	}
	var head domain.ApprovalRequest
	readApprovalHead(t, f.runDir, &head)
	if head.Status != domain.ApprovalInvalidated {
		t.Fatalf("head status = %s, want INVALIDATED", head.Status)
	}
	if head.Decision != nil {
		t.Fatalf("invalidation recorded a decision: %+v", head.Decision)
	}

	// The next run then proceeds: the gate permits execution, and the task is
	// still eligible to record a fresh outcome.
	out.Reset()
	errb.Reset()
	if code := approvalExecutionGate(f.dir, task, &out, &errb); code != exitOK {
		t.Fatalf("gate after invalidation = %d, want OK; stderr=%s", code, errb.String())
	}
	if task.Attempt != 0 {
		t.Errorf("attempt = %d, want 0 (no attempt recorded by invalidation)", task.Attempt)
	}
}

// TestInvalidationRegressionHistoryVerbatimBesideAudit proves the original
// PENDING request survives verbatim in the append-only history, beside the
// invalidation reason, actor, plan revisions, and definition digests.
func TestInvalidationRegressionHistoryVerbatimBesideAudit(t *testing.T) {
	acceptedAt := time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)
	f := newInvalidationFixture(t, invalidationFixtureOptions{
		planFP: "original-plan-fingerprint",
		digest: "stale-definition-digest",
		accepted: &acceptedChange{
			TaskID:     "S001",
			AcceptedAt: acceptedAt,
		},
	})

	var original domain.ApprovalRequest
	readApprovalHead(t, f.runDir, &original)

	var out, errb bytes.Buffer
	if code := runApprovalInvalidate(
		[]string{"S001", "--reason", "evidence names a removed deliverable", "--by", "operator"},
		&out, &errb, invalidationDeps(f.dir)); code != exitOK {
		t.Fatalf("exit = %d, want OK; stderr=%s", code, errb.String())
	}

	history := readApprovalHistory(t, f.runDir)
	if len(history) != 1 {
		t.Fatalf("history entries = %d, want exactly 1", len(history))
	}
	archived := history[0]
	if archived.ID != original.ID {
		t.Errorf("archived id = %q, want %q", archived.ID, original.ID)
	}
	if archived.Status != domain.ApprovalPending {
		t.Errorf("archived status = %q, want PENDING verbatim", archived.Status)
	}
	if archived.Evidence != original.Evidence || archived.Reason != original.Reason {
		t.Errorf("archived reason/evidence = %q/%q, want %q/%q",
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

	var head domain.ApprovalRequest
	readApprovalHead(t, f.runDir, &head)
	if head.Invalidation == nil {
		t.Fatal("head has no invalidation record beside the archived request")
	}
	if head.Invalidation.Reason != "evidence names a removed deliverable" {
		t.Errorf("recorded reason = %q", head.Invalidation.Reason)
	}
	if head.Invalidation.InvalidatedBy != "operator" {
		t.Errorf("recorded actor = %q", head.Invalidation.InvalidatedBy)
	}
	if head.Invalidation.PlanFingerprintBefore != "original-plan-fingerprint" ||
		head.Invalidation.PlanFingerprintAfter != f.planFP {
		t.Errorf("plan fingerprints = %q/%q, want %q/%q",
			head.Invalidation.PlanFingerprintBefore, head.Invalidation.PlanFingerprintAfter,
			"original-plan-fingerprint", f.planFP)
	}
	if head.Invalidation.DefinitionDigestBefore != "stale-definition-digest" ||
		head.Invalidation.DefinitionDigestAfter != f.digest {
		t.Errorf("definition digests = %q/%q, want %q/%q",
			head.Invalidation.DefinitionDigestBefore, head.Invalidation.DefinitionDigestAfter,
			"stale-definition-digest", f.digest)
	}
	if !head.Invalidation.DefinitionAcceptedAt.Equal(acceptedAt) {
		t.Errorf("acceptance time = %v, want %v", head.Invalidation.DefinitionAcceptedAt, acceptedAt)
	}
	if head.Invalidation.InvalidatedAt.IsZero() {
		t.Error("invalidation time is zero")
	}
}

// TestInvalidationRegressionRefusesStillApplicable proves invalidation is refused
// when the current definition still yields the recorded digest, and when a legacy
// request has no reconciliation evidence covering it. A refusal writes nothing.
func TestInvalidationRegressionRefusesStillApplicable(t *testing.T) {
	afterRequest := time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)

	t.Run("current digest equals the recorded digest", func(t *testing.T) {
		f := newInvalidationFixture(t, invalidationFixtureOptions{
			digest: "will-be-replaced",
			accepted: &acceptedChange{
				TaskID:     "S001",
				AcceptedAt: afterRequest,
			},
		})
		// Stamp the request with the CURRENT digest: the definition is unchanged.
		var head domain.ApprovalRequest
		readApprovalHead(t, f.runDir, &head)
		head.DefinitionDigest = f.digest
		writeJSONDoc(t, f.runDir+"/approval.json", head)

		var out, errb bytes.Buffer
		if code := runApprovalInvalidate([]string{"S001", "--reason", "r", "--by", "operator"}, &out, &errb, invalidationDeps(f.dir)); code == exitOK {
			t.Fatal("expected refusal when the definition is unchanged")
		}
		assertHeadPending(t, f.runDir)
		if got := len(readApprovalHistory(t, f.runDir)); got != 0 {
			t.Errorf("refusal appended %d history entries, want 0", got)
		}
	})

	t.Run("legacy request without reconciliation evidence", func(t *testing.T) {
		f := newInvalidationFixture(t, invalidationFixtureOptions{})
		var out, errb bytes.Buffer
		if code := runApprovalInvalidate([]string{"S001", "--reason", "r", "--by", "operator"}, &out, &errb, invalidationDeps(f.dir)); code == exitOK {
			t.Fatal("a legacy request with no acceptance evidence must be refused")
		}
		if !strings.Contains(errb.String(), "accepted definition change") {
			t.Errorf("stderr = %q, want the accepted-change refusal", errb.String())
		}
		assertHeadPending(t, f.runDir)
		if got := len(readApprovalHistory(t, f.runDir)); got != 0 {
			t.Errorf("refusal appended %d history entries, want 0", got)
		}
	})
}

// TestInvalidationRegressionTaskStateUnchanged proves an invalidated request
// leaves the task status, attempt count, and satisfaction unchanged.
func TestInvalidationRegressionTaskStateUnchanged(t *testing.T) {
	f := newInvalidationFixture(t, invalidationFixtureOptions{
		taskStatus: domain.READY,
		accepted: &acceptedChange{
			TaskID:     "S001",
			AcceptedAt: time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC),
		},
	})
	before := *readTask(t, f.dir, "S001")

	var out, errb bytes.Buffer
	if code := runApprovalInvalidate([]string{"S001", "--reason", "r", "--by", "operator"}, &out, &errb, invalidationDeps(f.dir)); code != exitOK {
		t.Fatalf("exit = %d, want OK; stderr=%s", code, errb.String())
	}

	after := readTask(t, f.dir, "S001")
	if after.Status != before.Status {
		t.Errorf("status = %s, want unchanged %s", after.Status, before.Status)
	}
	if after.Attempt != before.Attempt {
		t.Errorf("attempt = %d, want unchanged %d", after.Attempt, before.Attempt)
	}
	if after.IsSatisfied() != before.IsSatisfied() {
		t.Errorf("satisfaction = %v, want unchanged %v", after.IsSatisfied(), before.IsSatisfied())
	}
}

// TestInvalidationRegressionBlockingSemantics proves pending and declined
// requests still block execution exactly as before, while approved and
// invalidated requests permit it.
func TestInvalidationRegressionBlockingSemantics(t *testing.T) {
	cases := []struct {
		name   string
		status domain.ApprovalStatus
		want   int
	}{
		{"pending blocks", domain.ApprovalPending, exitApprovalBlocked},
		{"declined blocks", domain.ApprovalDeclined, exitApprovalBlocked},
		{"approved permits execution", domain.ApprovalApproved, exitOK},
		{"invalidated permits execution", domain.ApprovalInvalidated, exitOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newInvalidationFixture(t, invalidationFixtureOptions{headStatus: tc.status})
			var out, errb bytes.Buffer
			task := readTask(t, f.dir, "S001")
			if code := approvalExecutionGate(f.dir, task, &out, &errb); code != tc.want {
				t.Fatalf("gate = %d, want %d", code, tc.want)
			}
		})
	}
}
