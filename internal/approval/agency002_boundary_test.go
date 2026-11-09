package approval

// AGENCY-002 approval-binding reproductions. Each case drives the real
// approval.Service — the production boundary — and records the exact decision
// fields it observed (request id, target, definition digest, requeue result).
// The decisions are compared by the service itself; nothing is re-implemented.

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/domain"
)

// agency002BlockedTask starts task as BLOCKED with an exhausted retry budget so a
// successful approve must show the requeue action without spending an attempt.
func agency002BlockedTask(id string) *domain.Task {
	return &domain.Task{ID: id, Status: domain.BLOCKED, BlockedReason: domain.RETRIES_EXHAUSTED, Attempt: 2, MaxAttempts: 2}
}

// TestAgency002MismatchedTargetCannotBeAuthorized proves an approval recorded
// for one gate cannot be replayed to authorize a different gate. The service
// binds a decision to the head request it is made against (RequestID, TaskID),
// refuses a request for another task, and creates a fresh request id once the
// previous gate is resolved, so a stale decision is never applied to a new target.
func TestAgency002MismatchedTargetCannotBeAuthorized(t *testing.T) {
	tasks := newMemTasks(agency002BlockedTask("S001"), agency002BlockedTask("S002"))
	svc, records := newTestService(tasks)

	first, err := svc.Request(RequestInput{TaskID: "S001", Target: "gate-A", Reason: gateReason, DefinitionDigest: "digest-A"})
	if err != nil {
		t.Fatalf("Request for gate-A: %v", err)
	}
	if first.Target != "gate-A" || first.DefinitionDigest != "digest-A" {
		t.Fatalf("recorded request = target %q digest %q, want gate-A/digest-A", first.Target, first.DefinitionDigest)
	}

	// A decision made against S001 must not authorize S002, which has no request.
	if _, err := svc.Approve("S002", DecisionInput{By: "human"}); !errors.Is(err, ErrApprovalNotRequested) {
		t.Fatalf("Approve on a task without a request err = %v, want ErrApprovalNotRequested", err)
	}
	if got := tasks.tasks["S002"]; got.Status != domain.BLOCKED {
		t.Errorf("S002 status = %s, want unchanged BLOCKED", got.Status)
	}

	// A decision for gate-A is recorded with gate-A's identity only.
	approved, err := svc.Approve("S001", DecisionInput{By: "human"})
	if err != nil {
		t.Fatalf("Approve gate-A: %v", err)
	}
	if approved.View.Status != domain.ApprovalApproved || approved.View.Target != "gate-A" {
		t.Fatalf("approved view = status %s target %q, want APPROVED/gate-A", approved.View.Status, approved.View.Target)
	}
	decision := records["S001"].head.Decision
	if decision == nil || decision.RequestID != first.ID || decision.TaskID != "S001" {
		t.Fatalf("decision = %+v, want RequestID=%s TaskID=S001", decision, first.ID)
	}

	// A new gate (changed target and definition digest) after the resolved one
	// gets a NEW request id and no decision: the earlier approval cannot
	// authorize it.
	second, err := svc.Request(RequestInput{TaskID: "S001", Target: "gate-B", Reason: gateReason, DefinitionDigest: "digest-B"})
	if err != nil {
		t.Fatalf("Request for gate-B: %v", err)
	}
	if second.ID == first.ID {
		t.Fatalf("a new gate reused request id %q", first.ID)
	}
	if second.Target != "gate-B" || second.DefinitionDigest != "digest-B" {
		t.Errorf("second request = target %q digest %q, want gate-B/digest-B", second.Target, second.DefinitionDigest)
	}
	if got := records["S001"].head; got.ID != second.ID || got.Status != domain.ApprovalPending || got.Decision != nil {
		t.Fatalf("new head = %+v, want PENDING with no decision for the new gate", got)
	}
}

// TestAgency002StaleReplayAfterSupersedeAndInvalidate proves a superseded or
// invalidated request cannot be decided and authorizes nothing.
func TestAgency002StaleReplayAfterSupersedeAndInvalidate(t *testing.T) {
	t.Run("superseded", func(t *testing.T) {
		satisfied := &domain.Task{ID: "S001", Status: domain.LOCAL_DONE}
		svc, records := newTestService(newMemTasks(satisfied))
		req, err := svc.Request(RequestInput{TaskID: "S001", Target: "gate", Reason: gateReason})
		if err != nil {
			t.Fatalf("Request: %v", err)
		}
		res, err := svc.Supersede("S001", SupersedeInput{Reason: "task already complete", By: "operator-1"})
		if err != nil {
			t.Fatalf("Supersede: %v", err)
		}
		if !res.View.Superseded || res.View.DecidedBy != "operator-1" || res.View.Note != "task already complete" {
			t.Fatalf("supersede view = %+v, want SUPERSEDED by operator-1", res.View)
		}
		if _, err := svc.Approve("S001", DecisionInput{By: "human"}); !errors.Is(err, ErrApprovalResolved) {
			t.Fatalf("Approve after Supersede err = %v, want ErrApprovalResolved", err)
		}
		if got := records["S001"].head.Status; got != domain.ApprovalSuperseded {
			t.Errorf("head status = %s, want SUPERSEDED (no decision applied)", got)
		}
		if got := records["S001"].head.Decision; got == nil || got.Approved {
			t.Errorf("superseded decision = %+v, want Approved=false and non-nil", got)
		}
		if req.ID != records["S001"].head.Decision.RequestID {
			t.Errorf("decision RequestID = %q, want %q", records["S001"].head.Decision.RequestID, req.ID)
		}
	})

	t.Run("invalidated", func(t *testing.T) {
		tasks := newMemTasks(agency002BlockedTask("S001"))
		svc, records := newTestService(tasks)
		req, err := svc.Request(RequestInput{TaskID: "S001", Target: "gate", Reason: gateReason, DefinitionDigest: "digest-a"})
		if err != nil {
			t.Fatalf("Request: %v", err)
		}
		in := InvalidateInput{
			Reason:                         "plan revised",
			By:                             "operator-2",
			CurrentDefinitionDigest:        "digest-b",
			DefinitionAcceptedAsReconciled: true,
			DefinitionAcceptedAt:           req.RequestedAt.Add(time.Minute),
			DefinitionAcceptedDigest:       "digest-b",
			DefinitionAcceptedPriorDigest:  "digest-a",
		}
		res, err := svc.Invalidate("S001", in)
		if err != nil {
			t.Fatalf("Invalidate: %v", err)
		}
		if !res.View.Invalidated || res.View.Invalidation == nil || res.View.Invalidation.InvalidatedBy != "operator-2" {
			t.Fatalf("invalidate view = %+v, want INVALIDATED by operator-2", res.View)
		}
		if got := records["S001"].head.Decision; got != nil {
			t.Errorf("invalidation recorded a decision: %+v", got)
		}
		if got := tasks.tasks["S001"]; got.Status != domain.BLOCKED {
			t.Errorf("task status = %s, want unchanged BLOCKED", got.Status)
		}

		// Replay of the same invalidation is idempotent: it never becomes an
		// approval and writes no duplicate history.
		replay, err := svc.Invalidate("S001", in)
		if err != nil {
			t.Fatalf("replay Invalidate: %v", err)
		}
		if !replay.Idempotent {
			t.Errorf("replay Invalidate not reported idempotent")
		}
		if got := records["S001"].head.Decision; got != nil {
			t.Errorf("replay recorded a decision: %+v", got)
		}

		// A fresh request after the invalidation is a NEW gate with a new id.
		fresh, err := svc.Request(RequestInput{TaskID: "S001", Target: "gate-2", Reason: gateReason, DefinitionDigest: "digest-b"})
		if err != nil {
			t.Fatalf("Request after invalidate: %v", err)
		}
		if fresh.ID == req.ID {
			t.Errorf("a request after invalidation reused id %q", req.ID)
		}
		if got := records["S001"].head; got.ID != fresh.ID || got.Decision != nil {
			t.Fatalf("head after a fresh request = %+v, want a PENDING request with no decision", got)
		}
	})
}

// TestAgency002PositiveApprovalStillWorks is the approval-boundary positive
// control: an authorized approve on a BLOCKED task requeues it without spending
// a retry.
func TestAgency002PositiveApprovalStillWorks(t *testing.T) {
	tasks := newMemTasks(agency002BlockedTask("S001"))
	svc, _ := newTestService(tasks)
	if _, err := svc.Request(RequestInput{TaskID: "S001", Target: "gate", Reason: gateReason}); err != nil {
		t.Fatalf("Request: %v", err)
	}
	res, err := svc.Approve("S001", DecisionInput{By: "human"})
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if res.View.Status != domain.ApprovalApproved || res.View.LifecycleAction != domain.ApprovalActionRequeued {
		t.Fatalf("view = %+v, want APPROVED/REQUEUED", res.View)
	}
	if got := tasks.tasks["S001"]; got.Status != domain.PLANNED || got.Attempt != 2 {
		t.Errorf("task = status %s attempt %d, want PLANNED attempt 2", got.Status, got.Attempt)
	}
}

// TestAgency002ApprovalDoesNotTouchRuntimeState proves the approval service does
// not reach the governed runtime-state sink: a fake state database under a temp
// directory is byte-identical after the whole decision cycle.
func TestAgency002ApprovalDoesNotTouchRuntimeState(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, ".agent-sdlc", "state.db")
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(statePath, []byte("state bytes\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tasks := newMemTasks(agency002BlockedTask("S001"))
	svc, _ := newTestService(tasks)
	if _, err := svc.Request(RequestInput{TaskID: "S001", Target: "gate", Reason: gateReason}); err != nil {
		t.Fatalf("Request: %v", err)
	}
	if _, err := svc.Approve("S001", DecisionInput{By: "human"}); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	b, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("state.db read back: %v", err)
	}
	if string(b) != "state bytes\n" {
		t.Errorf("state.db changed: %q", b)
	}
}
