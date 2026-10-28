package approval

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/domain"
)

// attestedTask is a legacy pending request parked at an acceptance boundary, with the
// obsolete requirement present in its own evidence and absent from the current
// definition.
func attestedSetup(t *testing.T) (*Service, map[string]*memRecord, domain.ApprovalRequest) {
	t.Helper()
	task := pendingTask("S001")
	task.Status = domain.READY
	svc, records := newTestService(newMemTasks(task))
	seedPendingRequest(t, svc, records, "S001", "")
	head := records["S001"].head
	head.Stage = "WAITING_FOR_HUMAN"
	head.Reason = "required acceptance evidence is missing: deliverable \"domain.ApprovalRequest\" not present"
	head.Evidence = head.Reason
	if err := records["S001"].SaveApproval(*head); err != nil {
		t.Fatalf("save head: %v", err)
	}
	return svc, records, *head
}

func attestedInput(req domain.ApprovalRequest) InvalidateAttestedInput {
	return InvalidateAttestedInput{
		Reason:                  "the cited deliverable was removed by the reconciled plan",
		By:                      "operator",
		ExpectedRequestID:       req.ID,
		ObsoleteRequirement:     "domain.ApprovalRequest",
		ReplacementDefinition:   "internal/notify/event.go",
		ExpectDefinitionDigest:  "digest-current",
		CurrentDefinitionDigest: "digest-current",
		CurrentDefinitionText:   "internal/notify/event.go carries the event value\n",
		TaskReconciled:          true,
	}
}

func TestInvalidateAttestedSuccess(t *testing.T) {
	svc, records, head := attestedSetup(t)
	res, err := svc.InvalidateAttested("S001", attestedInput(head))
	if err != nil {
		t.Fatalf("InvalidateAttested: %v", err)
	}
	if res.Idempotent || res.View.Status != domain.ApprovalInvalidated {
		t.Fatalf("result = %+v", res)
	}
	got := records["S001"].head
	if got.Decision != nil {
		t.Error("attested invalidation must leave Decision nil")
	}
	if got.Invalidation == nil || got.Invalidation.Kind != domain.ApprovalInvalidationAttested {
		t.Fatalf("invalidation = %+v", got.Invalidation)
	}
	if got.Invalidation.Source != domain.InvalidationSourceUnknownLegacy {
		t.Errorf("source = %q, want the legacy-unknown marker", got.Invalidation.Source)
	}
	if got.Invalidation.InvalidatedAt.IsZero() {
		t.Error("attestation time missing")
	}
	if len(records["S001"].history) != 1 || records["S001"].history[0].ID != head.ID {
		t.Errorf("history = %+v, want the original request preserved", records["S001"].history)
	}
}

func TestInvalidateAttestedIdenticalReplayIsIdempotent(t *testing.T) {
	svc, records, head := attestedSetup(t)
	in := attestedInput(head)
	if _, err := svc.InvalidateAttested("S001", in); err != nil {
		t.Fatalf("first: %v", err)
	}
	before := len(records["S001"].history)

	again, err := svc.InvalidateAttested("S001", in)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if !again.Idempotent {
		t.Error("identical replay not reported idempotent")
	}
	if len(records["S001"].history) != before {
		t.Error("replay appended duplicate history")
	}
}

func TestInvalidateAttestedConflictingReplayIsRefused(t *testing.T) {
	svc, _, head := attestedSetup(t)
	in := attestedInput(head)
	if _, err := svc.InvalidateAttested("S001", in); err != nil {
		t.Fatalf("first: %v", err)
	}
	conflict := in
	conflict.Reason = "a different reason"
	if _, err := svc.InvalidateAttested("S001", conflict); !errors.Is(err, ErrApprovalResolved) {
		t.Fatalf("conflicting replay err = %v, want ErrApprovalResolved", err)
	}
}

func TestInvalidateAttestedRefusals(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*InvalidateAttestedInput)
		status domain.ApprovalStatus
		stage  string
	}{
		{"expected request mismatch", func(in *InvalidateAttestedInput) { in.ExpectedRequestID = "other-id" }, "", ""},
		{"stamped request", func(in *InvalidateAttestedInput) {}, domain.ApprovalPending, ""},
		{"accepted evidence exists", func(in *InvalidateAttestedInput) { in.DefinitionAcceptedAsReconciled = true }, "", ""},
		{"no reconciliation history", func(in *InvalidateAttestedInput) { in.TaskReconciled = false }, "", ""},
		{"provider failure stage", func(in *InvalidateAttestedInput) {}, domain.ApprovalPending, "FAILED"},
		{"obsolete requirement not in evidence", func(in *InvalidateAttestedInput) { in.ObsoleteRequirement = "something else entirely" }, "", ""},
		{"obsolete requirement still present", func(in *InvalidateAttestedInput) {
			in.CurrentDefinitionText = "domain.ApprovalRequest is still required\n"
		}, "", ""},
		{"replacement absent", func(in *InvalidateAttestedInput) { in.ReplacementDefinition = "internal/absent.go" }, "", ""},
		{"definition changed since review", func(in *InvalidateAttestedInput) { in.CurrentDefinitionDigest = "digest-newer" }, "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, records, head := attestedSetup(t)
			if tc.status != "" {
				records["S001"].head.Status = tc.status
			}
			if tc.stage != "" {
				records["S001"].head.Stage = tc.stage
			}
			if strings.Contains(tc.name, "stamped") {
				records["S001"].head.DefinitionDigest = "digest-at-request"
			}
			in := attestedInput(head)
			tc.mutate(&in)
			if _, err := svc.InvalidateAttested("S001", in); err == nil {
				t.Fatalf("%s: expected refusal", tc.name)
			}
			if records["S001"].head.Invalidation != nil {
				t.Fatalf("%s: invalidation written on refusal", tc.name)
			}
		})
	}
}

func TestInvalidateAttestedLeavesTaskUntouched(t *testing.T) {
	task := pendingTask("S001")
	task.Status = domain.BLOCKED
	task.Attempt = 2
	svc, records := newTestService(newMemTasks(task))
	seedPendingRequest(t, svc, records, "S001", "")
	head := records["S001"].head
	head.Stage = "WAITING_FOR_HUMAN"
	head.Reason = "deliverable \"domain.ApprovalRequest\" not present"
	head.Evidence = head.Reason
	_ = records["S001"].SaveApproval(*head)

	if _, err := svc.InvalidateAttested("S001", attestedInput(*head)); err != nil {
		t.Fatalf("InvalidateAttested: %v", err)
	}
	got, _ := svc.task("S001")
	if got.Status != domain.BLOCKED || got.Attempt != 2 {
		t.Errorf("task changed: status=%s attempt=%d", got.Status, got.Attempt)
	}
	if got.IsSatisfied() {
		t.Error("attested invalidation satisfied the task")
	}
}

func TestInvalidateAttestedMissingInputs(t *testing.T) {
	svc, _, head := attestedSetup(t)
	in := attestedInput(head)
	in.ReplacementDefinition = ""
	if _, err := svc.InvalidateAttested("S001", in); !errors.Is(err, ErrApprovalInvalidateInput) {
		t.Fatalf("err = %v, want ErrApprovalInvalidateInput", err)
	}
	_ = time.Now
}
