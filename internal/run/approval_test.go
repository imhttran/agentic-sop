package run

import (
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/domain"
)

func TestApprovalArtifactRoundTrips(t *testing.T) {
	dir := t.TempDir()
	r, err := New(dir, "S001")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	req := domain.ApprovalRequest{
		ID:          "S001-1",
		TaskID:      "S001",
		Kind:        domain.ApprovalNeedsHuman,
		Target:      "S001",
		Reason:      "conflicting requirements",
		Stage:       "WAITING_FOR_HUMAN",
		Disposition: "NEEDS_HUMAN",
		RequestedAt: time.Unix(1, 0).UTC(),
		Status:      domain.ApprovalPending,
	}
	if err := r.SaveApproval(req); err != nil {
		t.Fatalf("SaveApproval: %v", err)
	}

	// A read through a handle bound without touching state.json (the CLI/controller
	// path) sees the request.
	got, ok := At(Dir(dir, "S001")).Approval()
	if !ok {
		t.Fatalf("Approval: no request found")
	}
	if got.ID != req.ID || got.Status != domain.ApprovalPending || got.Reason != req.Reason {
		t.Fatalf("approval = %+v, want %+v", got, req)
	}

	// The decision survives a reload.
	resolved := got
	resolved.Status = domain.ApprovalApproved
	resolved.Decision = &domain.ApprovalDecision{RequestID: got.ID, TaskID: got.TaskID, Approved: true, DecidedAt: time.Unix(2, 0).UTC(), DecidedBy: "human"}
	if err := r.SaveApproval(resolved); err != nil {
		t.Fatalf("SaveApproval resolved: %v", err)
	}
	reloaded, ok := At(Dir(dir, "S001")).Approval()
	if !ok || reloaded.Status != domain.ApprovalApproved || reloaded.Decision == nil {
		t.Fatalf("reloaded approval = %+v (ok=%t), want APPROVED with a decision", reloaded, ok)
	}
	if reloaded.Decision.DecidedBy != "human" {
		t.Errorf("decision provenance lost: %+v", reloaded.Decision)
	}
}

func TestApprovalHistoryAppends(t *testing.T) {
	dir := t.TempDir()
	r := At(dir)
	first := domain.ApprovalRequest{ID: "a", TaskID: "S001", Status: domain.ApprovalDeclined}
	second := domain.ApprovalRequest{ID: "b", TaskID: "S001", Status: domain.ApprovalApproved}
	if err := r.AppendApprovalHistory(first); err != nil {
		t.Fatalf("AppendApprovalHistory: %v", err)
	}
	if err := r.AppendApprovalHistory(second); err != nil {
		t.Fatalf("AppendApprovalHistory: %v", err)
	}

	history := At(dir).ApprovalHistory()
	if len(history) != 2 || history[0].ID != "a" || history[1].ID != "b" {
		t.Fatalf("history = %+v, want [a b]", history)
	}
}

func TestApprovalAbsentReadsFalse(t *testing.T) {
	dir := t.TempDir()
	if _, ok := At(dir).Approval(); ok {
		t.Fatalf("Approval on empty dir reported a request")
	}
	if history := At(dir).ApprovalHistory(); history != nil {
		t.Fatalf("ApprovalHistory on empty dir = %+v, want nil", history)
	}
}
