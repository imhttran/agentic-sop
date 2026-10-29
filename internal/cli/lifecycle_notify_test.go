package cli

import (
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/notify"
)

const notifyTestProject = "proj-1"

// pendingApproval builds a PENDING approval request for a task.
func pendingApproval(id, taskID string) *domain.ApprovalRequest {
	return &domain.ApprovalRequest{
		ID:     id,
		TaskID: taskID,
		Status: domain.ApprovalPending,
		Reason: "needs a security decision",
	}
}

func TestEmitCompletedProducesOneEvent(t *testing.T) {
	sink := NewMemoryEventSink()
	em := NewLifecycleEmitter(sink)
	event, recorded, err := em.EmitCompleted(notifyTestProject, "T1", "job-1", 1, "all checks pass", "merge the branch")
	if err != nil {
		t.Fatalf("EmitCompleted: %v", err)
	}
	if !recorded || sink.Len() != 1 {
		t.Fatalf("want 1 enqueued event, got %d", sink.Len())
	}
	if event.Kind != notify.KindCompleted {
		t.Fatalf("kind = %q, want completed", event.Kind)
	}
	if event.Project != notifyTestProject || event.TaskID != "T1" || event.JobID != "job-1" {
		t.Fatalf("identity not populated: %+v", event)
	}
	if event.Outcome == "" || event.NextAction == "" {
		t.Fatalf("outcome/next action not populated: %+v", event)
	}
}

func TestEmitFailedProducesOneEvent(t *testing.T) {
	sink := NewMemoryEventSink()
	em := NewLifecycleEmitter(sink)
	event, recorded, err := em.EmitFailed(notifyTestProject, "T1", "job-1", 1, false, "tests failed", "fix the failing test")
	if err != nil {
		t.Fatalf("EmitFailed: %v", err)
	}
	if !recorded || sink.Len() != 1 {
		t.Fatalf("want 1 enqueued event, got %d", sink.Len())
	}
	if event.Kind != notify.KindFailed {
		t.Fatalf("kind = %q, want failed", event.Kind)
	}
	if event.Reason == "" || event.NextAction == "" {
		t.Fatalf("reason/next action not populated: %+v", event)
	}
}

func TestBlockedWithoutApprovalIsFailed(t *testing.T) {
	sink := NewMemoryEventSink()
	em := NewLifecycleEmitter(sink)
	event, recorded, err := em.EmitFailed(notifyTestProject, "T1", "job-1", 1, true, "no runnable work", "review the plan")
	if err != nil {
		t.Fatalf("EmitFailed(blocked): %v", err)
	}
	if !recorded {
		t.Fatal("want the blocked transition enqueued")
	}
	if event.Kind != notify.KindFailed {
		t.Fatalf("blocked kind = %q, want failed", event.Kind)
	}
	for _, e := range sink.Events() {
		if e.Kind == notify.KindWaitingForHuman {
			t.Fatalf("blocked transition produced waiting_for_human: %+v", e)
		}
	}
}

func TestEmitApprovalProducesWaitingForHuman(t *testing.T) {
	sink := NewMemoryEventSink()
	em := NewLifecycleEmitter(sink)
	approval := pendingApproval("APR-1", "T1")
	event, recorded, err := em.EmitApproval(notifyTestProject, "T1", "job-1", 1, approval, "sop approve APR-1")
	if err != nil {
		t.Fatalf("EmitApproval: %v", err)
	}
	if !recorded || sink.Len() != 1 {
		t.Fatalf("want 1 enqueued event, got %d", sink.Len())
	}
	if event.Kind != notify.KindWaitingForHuman {
		t.Fatalf("kind = %q, want waiting_for_human", event.Kind)
	}
	if event.ApprovalRequestID != "APR-1" {
		t.Fatalf("approval request id = %q, want APR-1", event.ApprovalRequestID)
	}
	if event.Reason == "" || event.ResolutionCommand == "" {
		t.Fatalf("reason/resolution command not populated: %+v", event)
	}
}

func TestEmitApprovalRejectsNonPending(t *testing.T) {
	sink := NewMemoryEventSink()
	em := NewLifecycleEmitter(sink)
	approval := pendingApproval("APR-1", "T1")
	approval.Status = domain.ApprovalApproved
	_, _, err := em.EmitApproval(notifyTestProject, "T1", "job-1", 1, approval, "sop approve APR-1")
	if err == nil {
		t.Fatal("want an error for a non-PENDING approval request")
	}
	if sink.Len() != 0 {
		t.Fatalf("want no enqueued event, got %d", sink.Len())
	}
}

func TestRepeatedEmissionEnqueuesOnce(t *testing.T) {
	sink := NewMemoryEventSink()
	em := NewLifecycleEmitter(sink)
	first, recorded, err := em.EmitCompleted(notifyTestProject, "T1", "job-1", 1, "done", "merge")
	if err != nil || !recorded {
		t.Fatalf("first emit: recorded=%v err=%v", recorded, err)
	}
	for i := 0; i < 3; i++ {
		again, recorded, err := em.EmitCompleted(notifyTestProject, "T1", "job-1", 1, "done", "merge")
		if err != nil {
			t.Fatalf("repeat emit: %v", err)
		}
		if recorded {
			t.Fatal("repeated emission must not record a second event")
		}
		if again.ID != first.ID {
			t.Fatalf("repeated emission changed the event id: %q != %q", again.ID, first.ID)
		}
	}
	if sink.Len() != 1 {
		t.Fatalf("want exactly 1 event, got %d", sink.Len())
	}
}

func TestNewAttemptProducesNewEvent(t *testing.T) {
	sink := NewMemoryEventSink()
	em := NewLifecycleEmitter(sink)
	first, _, err := em.EmitCompleted(notifyTestProject, "T1", "job-1", 1, "done", "merge")
	if err != nil {
		t.Fatalf("attempt 1: %v", err)
	}
	second, recorded, err := em.EmitCompleted(notifyTestProject, "T1", "job-1", 2, "done", "merge")
	if err != nil {
		t.Fatalf("attempt 2: %v", err)
	}
	if !recorded {
		t.Fatal("a new attempt must enqueue a new event")
	}
	if first.ID == second.ID {
		t.Fatalf("a new attempt reused the event id %q", first.ID)
	}
	if sink.Len() != 2 {
		t.Fatalf("want 2 events, got %d", sink.Len())
	}
}

func TestJobRerunProducesNewEvent(t *testing.T) {
	sink := NewMemoryEventSink()
	em := NewLifecycleEmitter(sink)
	first, _, err := em.EmitFailed(notifyTestProject, "", "job-1", 1, false, "process died", "inspect the job log")
	if err != nil {
		t.Fatalf("job-1: %v", err)
	}
	second, recorded, err := em.EmitFailed(notifyTestProject, "", "job-2", 1, false, "process died", "inspect the job log")
	if err != nil {
		t.Fatalf("job-2: %v", err)
	}
	if !recorded || first.ID == second.ID {
		t.Fatalf("job re-run must produce its own event: recorded=%v ids=%q,%q", recorded, first.ID, second.ID)
	}
	if sink.Len() != 2 {
		t.Fatalf("want 2 events, got %d", sink.Len())
	}
}

func TestUnknownTransitionIsRejected(t *testing.T) {
	sink := NewMemoryEventSink()
	em := NewLifecycleEmitter(sink)
	_, _, err := em.Emit(EmitRequest{
		Project:    notifyTestProject,
		Attempt:    1,
		Transition: "NOT_A_TRANSITION",
	})
	if err == nil {
		t.Fatal("want an error for an unknown transition")
	}
	if !strings.Contains(err.Error(), "unknown transition") {
		t.Fatalf("error = %v, want it to name the unknown transition", err)
	}
	if sink.Len() != 0 {
		t.Fatalf("want no enqueued event, got %d", sink.Len())
	}
}

func TestEmitWithNilSinkFails(t *testing.T) {
	em := NewLifecycleEmitter(nil)
	_, _, err := em.EmitCompleted(notifyTestProject, "T1", "job-1", 1, "done", "merge")
	if err == nil {
		t.Fatal("want an error when no sink is configured")
	}
}

func TestMemoryEventSinkRejectsEmptyID(t *testing.T) {
	sink := NewMemoryEventSink()
	if _, err := sink.Enqueue(notify.Event{}); err == nil {
		t.Fatal("want an error for an event with no id")
	}
}
