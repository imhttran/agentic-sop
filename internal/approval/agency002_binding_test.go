package approval

import (
	"errors"
	"testing"

	"github.com/imhttran/agentic-sop/internal/domain"
)

// AGENCY-002 approval-binding tests drive the production approval.Service. They
// prove the existing request-identity binding: a decision recorded for one
// request id or task can never resolve a different request or task, and a stale
// or superseded head is refused.

func TestAgency002DecisionRequiresRecordedRequest(t *testing.T) {
	tasks := newMemTasks(&domain.Task{ID: "S001", Status: domain.BLOCKED})
	svc, _ := newTestService(tasks)

	if _, err := svc.Approve("S001", DecisionInput{}); !errors.Is(err, ErrApprovalNotRequested) {
		t.Fatalf("Approve err = %v, want ErrApprovalNotRequested", err)
	}
	if _, err := svc.Decline("S001", DecisionInput{}); !errors.Is(err, ErrApprovalNotRequested) {
		t.Fatalf("Decline err = %v, want ErrApprovalNotRequested", err)
	}
	if got := tasks.tasks["S001"]; got.Status != domain.BLOCKED {
		t.Fatalf("refused decision changed status to %s, want BLOCKED", got.Status)
	}
}

func TestAgency002ResolvedHeadRefusesOppositeDecision(t *testing.T) {
	t.Run("approve-after-decline", func(t *testing.T) {
		tasks := newMemTasks(&domain.Task{ID: "S001", Status: domain.BLOCKED})
		svc, records := newTestService(tasks)
		if _, err := svc.Request(RequestInput{TaskID: "S001", Reason: gateReason}); err != nil {
			t.Fatalf("Request: %v", err)
		}
		if _, err := svc.Decline("S001", DecisionInput{}); err != nil {
			t.Fatalf("Decline: %v", err)
		}
		headBefore := *records["S001"].head
		if _, err := svc.Approve("S001", DecisionInput{}); !errors.Is(err, ErrApprovalResolved) {
			t.Fatalf("Approve after Decline err = %v, want ErrApprovalResolved", err)
		}
		if head := records["S001"].head; head.Status != headBefore.Status || head.Decision == nil || head.Decision.Approved {
			t.Fatalf("refused approve changed the head: %+v", head)
		}
	})

	t.Run("decline-after-approve", func(t *testing.T) {
		tasks := newMemTasks(&domain.Task{ID: "S001", Status: domain.BLOCKED})
		svc, records := newTestService(tasks)
		if _, err := svc.Request(RequestInput{TaskID: "S001", Reason: gateReason}); err != nil {
			t.Fatalf("Request: %v", err)
		}
		if _, err := svc.Approve("S001", DecisionInput{}); err != nil {
			t.Fatalf("Approve: %v", err)
		}
		if _, err := svc.Decline("S001", DecisionInput{}); !errors.Is(err, ErrApprovalResolved) {
			t.Fatalf("Decline after Approve err = %v, want ErrApprovalResolved", err)
		}
		if head := records["S001"].head; head.Status != domain.ApprovalApproved || head.Decision == nil || !head.Decision.Approved {
			t.Fatalf("refused decline changed the head: %+v", head)
		}
	})
}

func TestAgency002SatisfiedTaskIsNotApplicable(t *testing.T) {
	tasks := newMemTasks(&domain.Task{ID: "S001", Status: domain.LOCAL_DONE})
	svc, records := newTestService(tasks)
	if _, err := svc.Request(RequestInput{TaskID: "S001", Reason: gateReason}); err != nil {
		t.Fatalf("Request: %v", err)
	}
	if _, err := svc.Approve("S001", DecisionInput{}); !errors.Is(err, ErrApprovalNotApplicable) {
		t.Fatalf("Approve err = %v, want ErrApprovalNotApplicable", err)
	}
	if head := records["S001"].head; head.Status != domain.ApprovalPending {
		t.Fatalf("refused decision changed head status to %s, want PENDING", head.Status)
	}
	if got := tasks.tasks["S001"]; got.Status != domain.LOCAL_DONE {
		t.Fatalf("refused decision changed task status to %s", got.Status)
	}
}

// TestAgency002ApproveIsScopedToItsRequestAndTask proves criterion 3: an approval
// recorded for request R on task S001 cannot resolve a different task's request,
// and an old request's identity cannot be reused to authorize a newer gate.
func TestAgency002ApproveIsScopedToItsRequestAndTask(t *testing.T) {
	tasks := newMemTasks(
		&domain.Task{ID: "S001", Status: domain.BLOCKED},
		&domain.Task{ID: "S002", Status: domain.BLOCKED},
	)
	svc, records := newTestService(tasks)

	first, err := svc.Request(RequestInput{TaskID: "S001", Reason: "gate one"})
	if err != nil {
		t.Fatalf("Request S001: %v", err)
	}
	if _, err := svc.Request(RequestInput{TaskID: "S002", Reason: "gate two"}); err != nil {
		t.Fatalf("Request S002: %v", err)
	}

	// A decision on S002 must not resolve S001, and vice versa.
	if _, err := svc.Approve("S002", DecisionInput{}); err != nil {
		t.Fatalf("Approve S002: %v", err)
	}
	if head := records["S001"].head; head.Status != domain.ApprovalPending {
		t.Fatalf("approving S002 resolved S001 (status %s)", head.Status)
	}
	if got := tasks.tasks["S001"]; got.Status != domain.BLOCKED {
		t.Fatalf("approving S002 requeued S001 (status %s)", got.Status)
	}

	// Resolve S001, then start a fresh gate on S001: the old request id can never
	// authorize the new one, and a decision for the old id is not applied.
	if _, err := svc.Decline("S001", DecisionInput{}); err != nil {
		t.Fatalf("Decline S001: %v", err)
	}
	second, err := svc.Request(RequestInput{TaskID: "S001", Reason: "gate three"})
	if err != nil {
		t.Fatalf("re-Request S001: %v", err)
	}
	if second.ID == first.ID {
		t.Fatalf("re-request reused request id %s", first.ID)
	}
	if head := records["S001"].head; head.ID != second.ID || head.Status != domain.ApprovalPending {
		t.Fatalf("new gate head = %+v, want the fresh pending request", head)
	}
}

// TestAgency002PositiveControlApproveRequeues is the authorized-operation positive
// control: an approval on a PENDING request of a BLOCKED task succeeds and makes
// the task runnable again without spending a retry.
func TestAgency002PositiveControlApproveRequeues(t *testing.T) {
	tasks := newMemTasks(&domain.Task{ID: "S001", Status: domain.BLOCKED, Attempt: 2, MaxAttempts: 3})
	svc, _ := newTestService(tasks)
	if _, err := svc.Request(RequestInput{TaskID: "S001", Reason: gateReason}); err != nil {
		t.Fatalf("Request: %v", err)
	}
	res, err := svc.Approve("S001", DecisionInput{By: "human"})
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if res.View.LifecycleAction != domain.ApprovalActionRequeued {
		t.Fatalf("lifecycle action = %q, want REQUEUED", res.View.LifecycleAction)
	}
	got := tasks.tasks["S001"]
	if got.Status != domain.PLANNED {
		t.Fatalf("task status = %s, want PLANNED", got.Status)
	}
	if got.Attempt != 2 {
		t.Errorf("approve spent a retry: attempt = %d, want 2", got.Attempt)
	}
}
