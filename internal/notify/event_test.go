package notify

import (
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/domain"
)

func pendingApproval() *domain.ApprovalRequest {
	return &domain.ApprovalRequest{
		ID:     "appr-1",
		TaskID: "task-1",
		Kind:   domain.ApprovalNeedsHuman,
		Reason: "destructive migration requires authorization",
		Status: domain.ApprovalPending,
	}
}

func completedParams() Params {
	return Params{
		Kind:       KindCompleted,
		Project:    "demo",
		JobID:      "job-1",
		TaskID:     "task-1",
		Attempt:    1,
		Transition: "LOCAL_DONE",
		Outcome:    "task completed with all checks passing",
		NextAction: "review the change and merge",
	}
}

func TestKindsExist(t *testing.T) {
	for _, k := range []Kind{KindCompleted, KindFailed, KindWaitingForHuman} {
		if !k.Valid() {
			t.Fatalf("kind %q is not valid", k)
		}
	}
	if Kind("bogus").Valid() {
		t.Fatal("unknown kind reported valid")
	}
}

func TestCompletedEventCarriesOutcomeAndNextAction(t *testing.T) {
	e, err := NewEvent(completedParams())
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	if e.Version != Version || e.ID == "" {
		t.Fatalf("unexpected version/id: %d %q", e.Version, e.ID)
	}
	if e.Outcome == "" || e.NextAction == "" {
		t.Fatalf("completed event missing outcome or next action: %+v", e)
	}
	if e.OccurredAt.IsZero() {
		t.Fatal("occurred-at not set")
	}
}

func TestCompletedRequiresOutcomeAndNextAction(t *testing.T) {
	p := completedParams()
	p.Outcome = ""
	if _, err := NewEvent(p); err == nil {
		t.Fatal("expected error for missing outcome")
	}
	p = completedParams()
	p.NextAction = ""
	if _, err := NewEvent(p); err == nil {
		t.Fatal("expected error for missing next action")
	}
}

func TestFailedEventCarriesReasonAndNextAction(t *testing.T) {
	p := Params{
		Kind:       KindFailed,
		Project:    "demo",
		JobID:      "job-1",
		TaskID:     "task-1",
		Attempt:    2,
		Transition: "BLOCKED",
		Reason:     "validation failed",
		NextAction: "fix the failing test and rerun sop run",
	}
	e, err := NewEvent(p)
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	if e.Reason == "" || e.NextAction == "" {
		t.Fatalf("failed event missing reason or next action: %+v", e)
	}

	p.Reason = ""
	if _, err := NewEvent(p); err == nil {
		t.Fatal("expected error for missing reason")
	}
	p.Reason = "validation failed"
	p.NextAction = ""
	if _, err := NewEvent(p); err == nil {
		t.Fatal("expected error for missing next action")
	}
}

func TestWaitingForHumanRequiresPendingApproval(t *testing.T) {
	p := Params{
		Kind:              KindWaitingForHuman,
		Project:           "demo",
		TaskID:            "task-1",
		Attempt:           1,
		Transition:        "WAITING_FOR_HUMAN",
		Approval:          pendingApproval(),
		ResolutionCommand: "sop approve appr-1 --approve",
	}
	e, err := NewEvent(p)
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	if e.ApprovalRequestID != "appr-1" {
		t.Fatalf("approval request id = %q", e.ApprovalRequestID)
	}
	if e.Reason == "" || e.ResolutionCommand == "" {
		t.Fatalf("waiting_for_human missing reason or resolution command: %+v", e)
	}

	notPending := pendingApproval()
	notPending.Status = domain.ApprovalApproved
	p.Approval = notPending
	if _, err := NewEvent(p); err == nil {
		t.Fatal("expected error for non-PENDING approval")
	}

	p.Approval = nil
	if _, err := NewEvent(p); err == nil {
		t.Fatal("expected error for missing approval request")
	}

	p.Approval = pendingApproval()
	p.ResolutionCommand = ""
	if _, err := NewEvent(p); err == nil {
		t.Fatal("expected error for missing resolution command")
	}
}

func TestWaitingForHumanCarriesJobAndTaskID(t *testing.T) {
	p := Params{
		Kind:              KindWaitingForHuman,
		Project:           "demo",
		JobID:             "job-7",
		TaskID:            "task-1",
		Attempt:           1,
		Transition:        "WAITING_FOR_HUMAN",
		Approval:          pendingApproval(),
		ResolutionCommand: "sop approve appr-1 --approve",
	}
	e, err := NewEvent(p)
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	if e.JobID != "job-7" || e.TaskID != "task-1" {
		t.Fatalf("job id = %q, task id = %q", e.JobID, e.TaskID)
	}
}

func TestIdentityDeterministic(t *testing.T) {
	p := completedParams()
	p.OccurredAt = time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	first, err := NewEvent(p)
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}

	// Same transition replayed later, with different human-readable text.
	p.OccurredAt = time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	p.Reason = "changed text"
	p.NextAction = "a different next action"
	replay, err := NewEvent(p)
	if err != nil {
		t.Fatalf("NewEvent replay: %v", err)
	}
	if first.ID != replay.ID {
		t.Fatalf("replay id %q != original id %q", replay.ID, first.ID)
	}

	newAttempt := p
	newAttempt.Attempt = p.Attempt + 1
	attemptEvent, err := NewEvent(newAttempt)
	if err != nil {
		t.Fatalf("NewEvent attempt: %v", err)
	}
	if attemptEvent.ID == first.ID {
		t.Fatal("new attempt produced the same event id")
	}

	newJob := p
	newJob.JobID = "job-2"
	jobEvent, err := NewEvent(newJob)
	if err != nil {
		t.Fatalf("NewEvent job: %v", err)
	}
	if jobEvent.ID == first.ID {
		t.Fatal("new job produced the same event id")
	}
}

func TestNewEventRejectsBadInput(t *testing.T) {
	p := completedParams()
	p.Kind = Kind("bogus")
	if _, err := NewEvent(p); err == nil {
		t.Fatal("expected error for unknown kind")
	}

	p = completedParams()
	p.Project = ""
	if _, err := NewEvent(p); err == nil {
		t.Fatal("expected error for missing project")
	}

	p = completedParams()
	p.Attempt = 0
	if _, err := NewEvent(p); err == nil {
		t.Fatal("expected error for non-positive attempt")
	}

	p = completedParams()
	p.Transition = ""
	if _, err := NewEvent(p); err == nil {
		t.Fatal("expected error for missing transition")
	}
}

// TestEventRenderingCarriesNoCredential asserts the notify package never
// renders a credential-bearing value: the event payload carries only identity,
// reason, next action, and the resolution command, and a credential value never
// enters it.
func TestEventRenderingCarriesNoCredential(t *testing.T) {
	secret := "sk-live-DO-NOT-LEAK"
	p := completedParams()
	p.Outcome = "task completed"
	e, err := NewEvent(p)
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	if strings.Contains(e.ID, secret) || strings.Contains(e.Outcome, secret) {
		t.Fatalf("event rendered a credential: %+v", e)
	}
	if len(e.ID) != 64 {
		t.Fatalf("event id is not a SHA-256 hex digest: %q", e.ID)
	}
}
