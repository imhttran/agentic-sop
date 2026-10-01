package approval

import (
	"errors"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/store"
)

// memRecord is an in-memory Record for unit tests.
type memRecord struct {
	head    *domain.ApprovalRequest
	history []domain.ApprovalRequest
}

func (m *memRecord) Approval() (domain.ApprovalRequest, bool) {
	if m.head == nil {
		return domain.ApprovalRequest{}, false
	}
	return *m.head, true
}

func (m *memRecord) SaveApproval(req domain.ApprovalRequest) error {
	cp := req
	m.head = &cp
	return nil
}

func (m *memRecord) AppendApprovalHistory(req domain.ApprovalRequest) error {
	m.history = append(m.history, req)
	return nil
}

func (m *memRecord) ApprovalHistory() []domain.ApprovalRequest { return m.history }

// memTasks is an in-memory Tasks for unit tests.
type memTasks struct{ tasks map[string]*domain.Task }

func newMemTasks(tasks ...*domain.Task) *memTasks {
	m := &memTasks{tasks: map[string]*domain.Task{}}
	for _, t := range tasks {
		cp := *t
		m.tasks[t.ID] = &cp
	}
	return m
}

func (m *memTasks) Get(id string) (*domain.Task, error) {
	t, ok := m.tasks[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	return t, nil
}

func (m *memTasks) Save(task *domain.Task) error {
	cp := *task
	m.tasks[task.ID] = &cp
	return nil
}

// newTestService builds a Service over in-memory fakes with a deterministic,
// monotonically-advancing clock so request ids do not collide.
func newTestService(tasks *memTasks) (*Service, map[string]*memRecord) {
	records := map[string]*memRecord{}
	var tick int64
	svc := New(tasks, func(id string) Record {
		r, ok := records[id]
		if !ok {
			r = &memRecord{}
			records[id] = r
		}
		return r
	})
	svc.now = func() time.Time {
		tick++
		return time.Unix(0, tick).UTC()
	}
	return svc, records
}

const gateReason = "conflicting authoritative requirements"

func TestRequestIsReadableAndApplicable(t *testing.T) {
	tasks := newMemTasks(&domain.Task{ID: "S001", Status: domain.BLOCKED})
	svc, _ := newTestService(tasks)

	req, err := svc.Request(RequestInput{
		TaskID:      "S001",
		Target:      "S001",
		Reason:      gateReason,
		Evidence:    "PLAN.md S001 vs S002",
		Stage:       "WAITING_FOR_HUMAN",
		Disposition: "NEEDS_HUMAN",
	})
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if req.Status != domain.ApprovalPending || req.ID == "" {
		t.Fatalf("request = %+v, want a pending request with an id", req)
	}

	view, err := svc.Approval("S001")
	if err != nil {
		t.Fatalf("Approval: %v", err)
	}
	if !view.Present || !view.Applicable {
		t.Fatalf("view Present=%t Applicable=%t, want both true", view.Present, view.Applicable)
	}
	if view.Kind != domain.ApprovalNeedsHuman || view.Target != "S001" {
		t.Errorf("view Kind=%q Target=%q", view.Kind, view.Target)
	}
	if view.Reason != gateReason || view.Stage != "WAITING_FOR_HUMAN" || view.Disposition != "NEEDS_HUMAN" {
		t.Errorf("view = %+v, want the recorded reason/stage/disposition", view)
	}
}

func TestApprovalWithoutRequestIsAbsent(t *testing.T) {
	// A BLOCKED task carrying a human reason but NO recorded request is not an
	// approval boundary: SOP never manufactures one from a status.
	tasks := newMemTasks(&domain.Task{ID: "S001", Status: domain.BLOCKED, BlockedReason: domain.RETRIES_EXHAUSTED})
	svc, _ := newTestService(tasks)

	view, err := svc.Approval("S001")
	if err != nil {
		t.Fatalf("Approval: %v", err)
	}
	if view.Present {
		t.Fatalf("view.Present = true, want false (no request recorded)")
	}

	if _, err := svc.Approve("S001", DecisionInput{}); !errors.Is(err, ErrApprovalNotRequested) {
		t.Fatalf("Approve err = %v, want ErrApprovalNotRequested", err)
	}
	if _, err := svc.Decline("S001", DecisionInput{}); !errors.Is(err, ErrApprovalNotRequested) {
		t.Fatalf("Decline err = %v, want ErrApprovalNotRequested", err)
	}
}

func TestApprovePersistsDecisionAndHistory(t *testing.T) {
	tasks := newMemTasks(&domain.Task{ID: "S001", Status: domain.PLANNED})
	svc, records := newTestService(tasks)

	if _, err := svc.Request(RequestInput{TaskID: "S001", Reason: gateReason}); err != nil {
		t.Fatalf("Request: %v", err)
	}
	res, err := svc.Approve("S001", DecisionInput{By: "human", Note: "go ahead"})
	if err != nil {
		t.Fatalf("Approve: %v", err)
	}
	if res.Idempotent {
		t.Errorf("first Approve reported idempotent")
	}
	if res.View.Status != domain.ApprovalApproved || !res.View.Present {
		t.Fatalf("view = %+v, want APPROVED present", res.View)
	}
	if res.View.DecidedBy != "human" || res.View.Note != "go ahead" {
		t.Errorf("decision metadata = %q/%q", res.View.DecidedBy, res.View.Note)
	}
	if res.View.LifecycleAction != domain.ApprovalActionAlreadyRunnable {
		t.Errorf("lifecycle action = %q, want %q", res.View.LifecycleAction, domain.ApprovalActionAlreadyRunnable)
	}

	// The decision is persisted: a fresh read (same service, new read) sees it.
	head := records["S001"].head
	if head == nil || head.Status != domain.ApprovalApproved || head.Decision == nil {
		t.Fatalf("persisted head = %+v", head)
	}
	if len(records["S001"].history) != 1 {
		t.Fatalf("history len = %d, want 1", len(records["S001"].history))
	}
	if records["S001"].history[0].Decision == nil || records["S001"].history[0].Decision.RequestID != head.ID {
		t.Errorf("history entry missing the decision for request %s", head.ID)
	}
}

func TestApproveIsIdempotent(t *testing.T) {
	tasks := newMemTasks(&domain.Task{ID: "S001", Status: domain.PLANNED})
	svc, records := newTestService(tasks)
	if _, err := svc.Request(RequestInput{TaskID: "S001", Reason: gateReason}); err != nil {
		t.Fatalf("Request: %v", err)
	}
	if _, err := svc.Approve("S001", DecisionInput{By: "first"}); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	res, err := svc.Approve("S001", DecisionInput{By: "second"})
	if err != nil {
		t.Fatalf("second Approve: %v", err)
	}
	if !res.Idempotent {
		t.Errorf("second Approve not reported idempotent")
	}
	if res.View.DecidedBy != "first" {
		t.Errorf("idempotent Approve changed the decision to %q", res.View.DecidedBy)
	}
	if len(records["S001"].history) != 1 {
		t.Errorf("history len = %d, want 1 (no duplicate)", len(records["S001"].history))
	}
}

func TestDeclinePersistsAndDoesNotComplete(t *testing.T) {
	tasks := newMemTasks(&domain.Task{ID: "S001", Status: domain.BLOCKED, BlockedReason: domain.REVIEW_UNRESOLVED})
	svc, records := newTestService(tasks)
	if _, err := svc.Request(RequestInput{TaskID: "S001", Reason: gateReason}); err != nil {
		t.Fatalf("Request: %v", err)
	}

	res, err := svc.Decline("S001", DecisionInput{By: "human", Note: "not acceptable"})
	if err != nil {
		t.Fatalf("Decline: %v", err)
	}
	if res.View.Status != domain.ApprovalDeclined {
		t.Fatalf("status = %q, want DECLINED", res.View.Status)
	}
	if res.View.LifecycleAction != domain.ApprovalActionNone {
		t.Errorf("lifecycle action = %q, want NONE (decline never acts)", res.View.LifecycleAction)
	}
	if records["S001"].head.Status != domain.ApprovalDeclined {
		t.Errorf("persisted head status = %q", records["S001"].head.Status)
	}

	// Decline preserves the truthful lifecycle state: the task is unchanged and
	// definitely not completed.
	if got := tasks.tasks["S001"]; got.Status != domain.BLOCKED || got.IsSatisfied() {
		t.Fatalf("task after decline = %+v, want unchanged BLOCKED", got)
	}

	// Approving a declined request is an explicit resolved error.
	if _, err := svc.Approve("S001", DecisionInput{}); !errors.Is(err, ErrApprovalResolved) {
		t.Fatalf("Approve after Decline err = %v, want ErrApprovalResolved", err)
	}
}

func TestApproveResumesBlockedTaskWithoutSpendingRetry(t *testing.T) {
	tasks := newMemTasks(&domain.Task{ID: "S001", Status: domain.BLOCKED, BlockedReason: domain.RETRIES_EXHAUSTED, Attempt: 3, MaxAttempts: 3})
	svc, _ := newTestService(tasks)
	if _, err := svc.Request(RequestInput{TaskID: "S001", Reason: gateReason}); err != nil {
		t.Fatalf("Request: %v", err)
	}

	res, err := svc.Approve("S001", DecisionInput{})
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
	if got.Attempt != 3 {
		t.Errorf("approve spent a retry: attempt = %d, want 3", got.Attempt)
	}
}

func TestApproveIsScopedToItsTask(t *testing.T) {
	tasks := newMemTasks(
		&domain.Task{ID: "S001", Status: domain.BLOCKED},
		&domain.Task{ID: "S002", Status: domain.BLOCKED},
	)
	svc, _ := newTestService(tasks)
	if _, err := svc.Request(RequestInput{TaskID: "S001", Reason: gateReason}); err != nil {
		t.Fatalf("Request: %v", err)
	}

	if _, err := svc.Approve("S002", DecisionInput{}); !errors.Is(err, ErrApprovalNotRequested) {
		t.Fatalf("Approve on unrelated task err = %v, want ErrApprovalNotRequested", err)
	}
	// S002 is untouched.
	if got := tasks.tasks["S002"]; got.Status != domain.BLOCKED {
		t.Errorf("S002 status = %s, want BLOCKED", got.Status)
	}
}

func TestStaleRequestCannotBeApproved(t *testing.T) {
	// A pending request whose task completed after the fact is stale: it can no
	// longer authorize anything.
	completed := &domain.Task{ID: "S001", Status: domain.LOCAL_DONE}
	tasks := newMemTasks(completed)
	svc, _ := newTestService(tasks)
	if _, err := svc.Request(RequestInput{TaskID: "S001", Reason: gateReason}); err != nil {
		t.Fatalf("Request: %v", err)
	}

	if _, err := svc.Approve("S001", DecisionInput{}); !errors.Is(err, ErrApprovalNotApplicable) {
		t.Fatalf("Approve stale err = %v, want ErrApprovalNotApplicable", err)
	}
	if got := tasks.tasks["S001"]; got.Status != domain.LOCAL_DONE {
		t.Errorf("stale approve changed task status to %s", got.Status)
	}
}

// TestStaleRequestIsNotApplicable proves the read projection agrees with the decision
// boundary: a pending request whose task has already completed is reported as NOT
// applicable, for the same reason Approve refuses it (ErrApprovalNotApplicable). A
// client that trusted Applicable would otherwise advertise a decision that cannot be
// recorded.
func TestStaleRequestIsNotApplicable(t *testing.T) {
	completed := &domain.Task{ID: "S001", Status: domain.LOCAL_DONE}
	svc, _ := newTestService(newMemTasks(completed))
	if _, err := svc.Request(RequestInput{TaskID: "S001", Reason: gateReason}); err != nil {
		t.Fatalf("Request: %v", err)
	}

	view, err := svc.Approval("S001")
	if err != nil {
		t.Fatalf("Approval: %v", err)
	}
	if !view.Present {
		t.Fatal("the request is present")
	}
	if view.Applicable {
		t.Error("a pending request on a satisfied task must not be applicable")
	}
}

func TestRequestSupersedesResolvedGate(t *testing.T) {
	tasks := newMemTasks(&domain.Task{ID: "S001", Status: domain.BLOCKED})
	svc, records := newTestService(tasks)

	first, err := svc.Request(RequestInput{TaskID: "S001", Reason: "first gate"})
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if _, err := svc.Decline("S001", DecisionInput{}); err != nil {
		t.Fatalf("Decline: %v", err)
	}

	second, err := svc.Request(RequestInput{TaskID: "S001", Reason: "second gate"})
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if second.ID == first.ID {
		t.Fatalf("superseding request reused id %s", first.ID)
	}
	if second.Status != domain.ApprovalPending {
		t.Errorf("new request status = %q, want PENDING", second.Status)
	}
	// The declined request is preserved as provenance, and the old decision can
	// never be applied to the new gate.
	if len(records["S001"].history) != 1 || records["S001"].history[0].ID != first.ID {
		t.Fatalf("history = %+v, want the superseded request", records["S001"].history)
	}
	if records["S001"].history[0].Status != domain.ApprovalDeclined {
		t.Errorf("superseded request status = %q, want DECLINED", records["S001"].history[0].Status)
	}
}

func TestRequestIsIdempotentForSamePendingGate(t *testing.T) {
	tasks := newMemTasks(&domain.Task{ID: "S001", Status: domain.PLANNED})
	svc, records := newTestService(tasks)

	first, err := svc.Request(RequestInput{TaskID: "S001", Reason: "gate"})
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	second, err := svc.Request(RequestInput{TaskID: "S001", Reason: "gate, refreshed", Stage: "WAITING_FOR_HUMAN"})
	if err != nil {
		t.Fatalf("Request: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("re-park changed the request id: %s -> %s", first.ID, second.ID)
	}
	if second.Reason != "gate, refreshed" || second.Stage != "WAITING_FOR_HUMAN" {
		t.Errorf("re-park did not refresh context: %+v", second)
	}
	if len(records["S001"].history) != 0 {
		t.Errorf("re-park appended history: %+v", records["S001"].history)
	}
}

func TestUnknownTaskIsNotApprovable(t *testing.T) {
	svc, _ := newTestService(newMemTasks())
	if _, err := svc.Approval("S999"); !errors.Is(err, ErrApprovalNotRequested) {
		t.Fatalf("Approval err = %v, want ErrApprovalNotRequested", err)
	}
	if _, err := svc.Approve("S999", DecisionInput{}); !errors.Is(err, ErrApprovalNotRequested) {
		t.Fatalf("Approve err = %v, want ErrApprovalNotRequested", err)
	}
}
