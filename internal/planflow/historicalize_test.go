package planflow

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
)

// The planflow historicalization tests are deterministic and model-free. They
// cover the required invariants: a fully completed plan, each unresolved state,
// unresolved approvals, missing verification, the explicit-disposition path,
// idempotency, stale state, staged recovery, evidence preservation, and active
// plan selection.

// seedPassedRun writes a PASSED run state for a task, the verification evidence a
// satisfied task carries after its governed lifecycle.
func seedPassedRun(t *testing.T, dir, id string) {
	t.Helper()
	write(t, dir, filepath.Join(config.DirName, "runs", id, "state.json"),
		`{"id":"`+id+`","stage":"PASSED"}`+"\n")
}

// seedPendingApproval writes a pending approval request for a task.
func seedPendingApproval(t *testing.T, dir, id, requestID string) {
	t.Helper()
	write(t, dir, filepath.Join(config.DirName, "runs", id, "approval.json"),
		`{"id":"`+requestID+`","task_id":"`+id+`","kind":"NEEDS_HUMAN","target":"`+id+`","status":"PENDING"}`+"\n")
}

// completedPlan prepares a two-task plan and marks both tasks satisfied with
// verification evidence, so it is eligible for a normal historicalization.
func completedPlan(t *testing.T, dir string) *fakeStore {
	t.Helper()
	st := prepareForReconcile(t, dir, "PLAN.md", reconcileDocTwo)
	for _, task := range st.tasks {
		markExecuted(task)
		seedPassedRun(t, dir, task.ID)
	}
	return st
}

func archiveLifecyclePath(dir, planID string) string {
	return filepath.Join(dir, config.DirName, archiveDirName, archiveID(planID), lifecycleFile)
}

// TestHistoricalizeCompletedPlanArchives proves a fully satisfied, verified plan
// is archived as COMPLETE, released from ACTIVE selection, and preserved.
func TestHistoricalizeCompletedPlanArchives(t *testing.T) {
	dir := t.TempDir()
	st := completedPlan(t, dir)
	n := len(st.tasks)

	res, err := Historicalize(HistoricalizeOptions{Dir: dir, Store: st})
	if err != nil {
		t.Fatalf("historicalize: %v", err)
	}
	if res.Outcome != OutcomeHistoricalized {
		t.Fatalf("outcome = %s, want %s", res.Outcome, OutcomeHistoricalized)
	}
	if res.Tasks != n || res.Archived == "" {
		t.Fatalf("res = %+v", res)
	}
	if len(st.tasks) != 0 {
		t.Errorf("tasks were not cleared: %d remain", len(st.tasks))
	}
	if _, _, ok := ActivePlanID(dir); ok {
		t.Error("the historicalized plan is still ACTIVE")
	}
	if fileExists(filepath.Join(dir, config.DirName, planFileName)) {
		t.Error("plan.json was not removed")
	}
	life := readLifecycle(t, archiveLifecyclePath(dir, res.Readiness.PlanID))
	if life.Disposition != DispositionComplete {
		t.Errorf("disposition = %q, want COMPLETE", life.Disposition)
	}
	if res.Readiness.State != HistoricalizationStateHistoricalized || !res.Readiness.Eligible {
		t.Errorf("final readiness = %+v, want HISTORICALIZED/eligible", res.Readiness)
	}
	// The active-plan selection is released, so the plan cannot be re-activated
	// from a stale machine plan.
	archived := read(t, filepath.Join(dir, config.DirName, archiveDirName, archiveID(res.Readiness.PlanID), archivedTasksFile))
	if !strings.Contains(archived, "\"S001\"") || !strings.Contains(archived, "\"S002\"") {
		t.Errorf("archived tasks lost records: %s", archived)
	}
}

// TestHistoricalizeRefusesPlannedTask proves unresolved PLANNED work is refused.
func TestHistoricalizeRefusesPlannedTask(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", reconcileDocTwo) // both PLANNED

	res, err := Historicalize(HistoricalizeOptions{Dir: dir, Store: st})
	if !errors.Is(err, ErrHistoricalizationIneligible) {
		t.Fatalf("err = %v, want ineligible", err)
	}
	if res.Readiness.Eligible {
		t.Error("readiness must be ineligible")
	}
	if len(res.Readiness.UnresolvedTasks) != 2 {
		t.Errorf("unresolved = %v, want two", res.Readiness.UnresolvedTasks)
	}
	if len(st.tasks) == 0 {
		t.Error("tasks were cleared despite the refusal")
	}
	if _, _, ok := ActivePlanID(dir); !ok {
		t.Error("the plan lost its active association on a refused historicalization")
	}
}

// TestHistoricalizeRefusesRunningTask proves an in-flight task is unresolved work.
func TestHistoricalizeRefusesRunningTask(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", reconcileDocTwo)
	taskByID(t, st.tasks, "S001").Status = domain.IMPLEMENTING

	res, err := Historicalize(HistoricalizeOptions{Dir: dir, Store: st})
	if !errors.Is(err, ErrHistoricalizationIneligible) {
		t.Fatalf("err = %v, want ineligible", err)
	}
	if got := strings.Join(res.Readiness.UnresolvedTasks, ","); !strings.Contains(got, "S001 (IMPLEMENTING)") {
		t.Errorf("unresolved = %v, want S001 (IMPLEMENTING)", res.Readiness.UnresolvedTasks)
	}
}

// TestHistoricalizeRefusesBlockedTask proves an unresolved BLOCKED task is refused.
func TestHistoricalizeRefusesBlockedTask(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", reconcileDocTwo)
	blocked := taskByID(t, st.tasks, "S001")
	blocked.Status = domain.BLOCKED
	blocked.BlockedReason = domain.CI_FAILURE_UNACTIONABLE

	if _, err := Historicalize(HistoricalizeOptions{Dir: dir, Store: st}); !errors.Is(err, ErrHistoricalizationIneligible) {
		t.Fatalf("err = %v, want ineligible", err)
	}
	if len(st.tasks) == 0 {
		t.Error("a refused historicalization must not clear tasks")
	}
}

// TestHistoricalizeFailedWorkIsRefused documents that the domain has no FAILED
// task status: a failed task is a terminal BLOCKED task carrying its classified
// reason, and historicalization refuses it exactly like any other unresolved
// work. No new status is invented.
func TestHistoricalizeFailedWorkIsRefused(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", reconcileDocTwo)
	failed := taskByID(t, st.tasks, "S001")
	failed.Status = domain.BLOCKED
	failed.BlockedReason = domain.RETRIES_EXHAUSTED

	res, err := Historicalize(HistoricalizeOptions{Dir: dir, Store: st})
	if !errors.Is(err, ErrHistoricalizationIneligible) {
		t.Fatalf("err = %v, want ineligible", err)
	}
	if got := strings.Join(res.Readiness.UnresolvedTasks, ","); !strings.Contains(got, "RETRIES_EXHAUSTED") {
		t.Errorf("unresolved = %v, want the failure reason preserved", res.Readiness.UnresolvedTasks)
	}
}

// TestHistoricalizeRefusesUnresolvedApproval proves a pending human gate blocks a
// normal historicalization, so a dangling gate is never silently buried.
func TestHistoricalizeRefusesUnresolvedApproval(t *testing.T) {
	dir := t.TempDir()
	st := completedPlan(t, dir)
	seedPendingApproval(t, dir, "S001", "req-1")

	res, err := Historicalize(HistoricalizeOptions{Dir: dir, Store: st})
	if !errors.Is(err, ErrHistoricalizationIneligible) {
		t.Fatalf("err = %v, want ineligible", err)
	}
	if len(res.Readiness.UnresolvedApprovals) == 0 {
		t.Error("readiness must report the pending approval")
	}
	if !strings.Contains(res.Readiness.Reason, "approval") {
		t.Errorf("reason = %q, want an approval blocker", res.Readiness.Reason)
	}
}

// TestHistoricalizeRefusesMissingVerification proves a satisfied task with no
// verification evidence blocks a normal historicalization.
func TestHistoricalizeRefusesMissingVerification(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", reconcileDocTwo)
	for _, task := range st.tasks {
		markExecuted(task) // satisfied, but no run evidence seeded
	}

	res, err := Historicalize(HistoricalizeOptions{Dir: dir, Store: st})
	if !errors.Is(err, ErrHistoricalizationIneligible) {
		t.Fatalf("err = %v, want ineligible", err)
	}
	if len(res.Readiness.UnresolvedVerification) != 2 {
		t.Errorf("unresolved verification = %v, want two", res.Readiness.UnresolvedVerification)
	}
}

// TestHistoricalizeExplicitSupersededAllowsUnfinished proves the explicit
// disposal path preserves unfinished work rather than fabricating completion.
func TestHistoricalizeExplicitSupersededAllowsUnfinished(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", reconcileDocTwo) // both PLANNED
	markExecuted(taskByID(t, st.tasks, "S001"))                   // S001 done, S002 unfinished
	taskByID(t, st.tasks, "S002").BlockedReason = domain.RETRIES_EXHAUSTED

	res, err := Historicalize(HistoricalizeOptions{Dir: dir, Store: st, Disposition: DispositionSuperseded})
	if err != nil {
		t.Fatalf("historicalize superseded: %v", err)
	}
	if res.Outcome != OutcomeHistoricalized {
		t.Fatalf("outcome = %s", res.Outcome)
	}
	life := readLifecycle(t, archiveLifecyclePath(dir, res.Readiness.PlanID))
	if life.Disposition != DispositionSuperseded {
		t.Errorf("disposition = %q, want SUPERSEDED", life.Disposition)
	}
	archived := read(t, filepath.Join(dir, config.DirName, archiveDirName, archiveID(res.Readiness.PlanID), archivedTasksFile))
	if !strings.Contains(archived, "\"S002\"") || !strings.Contains(archived, "PLANNED") {
		t.Errorf("unfinished S002 was not preserved verbatim: %s", archived)
	}
}

// TestHistoricalizePreservesEvidence proves the archive keeps attempt history and
// the terminal disposition, so the audit trail is intact.
func TestHistoricalizePreservesEvidence(t *testing.T) {
	dir := t.TempDir()
	st := completedPlan(t, dir)

	if _, err := Historicalize(HistoricalizeOptions{Dir: dir, Store: st}); err != nil {
		t.Fatalf("historicalize: %v", err)
	}
	id, _, ok := ActivePlanID(dir)
	if ok {
		t.Fatalf("plan still active: %s", id)
	}
	// Re-read the archive directly from the recorded plan id.
	archived := read(t, filepath.Join(dir, config.DirName, archiveDirName, "plan", archivedTasksFile))
	if !strings.Contains(archived, "\"Attempts\"") || !strings.Contains(archived, "LOCAL_DONE") {
		t.Errorf("archived tasks lost attempt evidence: %s", archived)
	}
	if _, err := os.Stat(filepath.Join(dir, config.DirName, archiveDirName, "plan", metaFileName)); err != nil {
		t.Errorf("archived provenance missing: %v", err)
	}
}

// TestHistoricalizeIsIdempotent proves a repeated invocation is a deterministic
// no-op: it reports ALREADY_HISTORICALIZED and rewrites nothing.
func TestHistoricalizeIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	st := completedPlan(t, dir)

	first, err := Historicalize(HistoricalizeOptions{Dir: dir, Store: st})
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	before := readLifecycle(t, archiveLifecyclePath(dir, "plan"))

	second, err := Historicalize(HistoricalizeOptions{Dir: dir, Store: st, Plan: "PLAN.md"})
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if second.Outcome != OutcomeAlreadyHistoricalized {
		t.Fatalf("outcome = %s, want %s", second.Outcome, OutcomeAlreadyHistoricalized)
	}
	if second.Readiness.State != HistoricalizationStateHistoricalized {
		t.Errorf("state = %s, want HISTORICALIZED", second.Readiness.State)
	}
	after := readLifecycle(t, archiveLifecyclePath(dir, "plan"))
	if !before.RecordedAt.Equal(after.RecordedAt) {
		t.Errorf("idempotent run rewrote the archive: %v -> %v", before.RecordedAt, after.RecordedAt)
	}
	// Exactly one archive per plan: the fixed file set, not a duplicate directory.
	entries, err := os.ReadDir(filepath.Join(dir, config.DirName, archiveDirName))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("archive holds %d entries, want 1: %v", len(entries), entries)
	}
	if first.Archived != second.Archived {
		t.Errorf("archive path changed: %q -> %q", first.Archived, second.Archived)
	}
}

// TestHistoricalizeStaleStateRefused proves an explicit expected fingerprint that
// no longer matches refuses the mutation rather than overwriting newer state.
func TestHistoricalizeStaleStateRefused(t *testing.T) {
	dir := t.TempDir()
	st := completedPlan(t, dir)

	ready, err := EvaluateHistoricalization(dir, st, "", "")
	if err != nil {
		t.Fatalf("readiness: %v", err)
	}
	if !ready.Eligible {
		t.Fatalf("plan should be eligible: %+v", ready)
	}
	// A concurrent change moves the plan after readiness was decided.
	taskByID(t, st.tasks, "S001").UpdatedAt = taskByID(t, st.tasks, "S001").UpdatedAt.Add(time.Second)

	_, err = Historicalize(HistoricalizeOptions{Dir: dir, Store: st, ExpectedFingerprint: ready.Fingerprint})
	if !errors.Is(err, ErrStaleHistoricalizationState) {
		t.Fatalf("err = %v, want stale state", err)
	}
	if len(st.tasks) == 0 {
		t.Error("a stale refusal must not clear tasks")
	}
}

// flipStore changes a task between the readiness read and the pre-mutation read,
// modelling a concurrent writer within one process window.
type flipStore struct {
	calls int
	tasks []*domain.Task
}

func (f *flipStore) List() ([]*domain.Task, error) {
	f.calls++
	if f.calls == 2 {
		for _, task := range f.tasks {
			if task.ID == "S001" {
				task.Status = domain.PLANNED
				task.UpdatedAt = task.UpdatedAt.Add(time.Second)
			}
		}
	}
	return f.tasks, nil
}

func (f *flipStore) SaveTasks([]*domain.Task) error { return nil }
func (f *flipStore) ClearTasks() error              { f.tasks = nil; return nil }

// TestHistoricalizeDetectsStateChangeBetweenReads proves the pre-mutation re-read
// catches a plan that changed after readiness, even without an expected
// fingerprint.
func TestHistoricalizeDetectsStateChangeBetweenReads(t *testing.T) {
	dir := t.TempDir()
	base := completedPlan(t, dir)
	st := &flipStore{tasks: base.tasks}

	_, err := Historicalize(HistoricalizeOptions{Dir: dir, Store: st})
	if !errors.Is(err, ErrStaleHistoricalizationState) {
		t.Fatalf("err = %v, want stale state", err)
	}
	if st.tasks == nil {
		t.Error("a stale refusal must not release the active association")
	}
}

// TestHistoricalizeCompletesStagedRelease proves a partially released plan — an
// archive already written but the active association still present — is completed
// by a re-run instead of being reported historicalized while records stay active.
func TestHistoricalizeCompletesStagedRelease(t *testing.T) {
	dir := t.TempDir()
	st := completedPlan(t, dir)
	// Simulate a failure after the archive stage: the terminal record exists while
	// the plan is still ACTIVE.
	write(t, dir, filepath.Join(config.DirName, archiveDirName, "plan", lifecycleFile),
		`{"plan_id":"plan","source":"PLAN.md","disposition":"COMPLETE","recorded_at":"2026-01-01T00:00:00Z"}`+"\n")

	ready, err := EvaluateHistoricalization(dir, st, "", "")
	if err != nil {
		t.Fatalf("readiness: %v", err)
	}
	if ready.State != HistoricalizationStateInconsistent || !ready.Eligible {
		t.Fatalf("readiness = %+v, want INCONSISTENT/eligible", ready)
	}

	res, err := Historicalize(HistoricalizeOptions{Dir: dir, Store: st})
	if err != nil {
		t.Fatalf("historicalize staged: %v", err)
	}
	if res.Outcome != OutcomeHistoricalized {
		t.Fatalf("outcome = %s, want %s", res.Outcome, OutcomeHistoricalized)
	}
	if len(st.tasks) != 0 {
		t.Errorf("staged release left %d task(s) active", len(st.tasks))
	}
	if _, _, ok := ActivePlanID(dir); ok {
		t.Error("staged release did not release the active association")
	}
}

// TestHistoricalizeReadinessReportsCounts proves readiness reports task counts by
// state, unresolved work, and the current lifecycle state.
func TestHistoricalizeReadinessReportsCounts(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", reconcileDocTwo)
	markExecuted(taskByID(t, st.tasks, "S001"))

	ready, err := EvaluateHistoricalization(dir, st, "", "")
	if err != nil {
		t.Fatalf("readiness: %v", err)
	}
	if ready.State != HistoricalizationStateActive {
		t.Errorf("state = %s, want ACTIVE", ready.State)
	}
	if ready.TotalTasks != 2 || ready.TaskCounts[string(domain.LOCAL_DONE)] != 1 || ready.TaskCounts[string(domain.PLANNED)] != 1 {
		t.Errorf("counts = %+v (total %d)", ready.TaskCounts, ready.TotalTasks)
	}
	if ready.Eligible {
		t.Errorf("plan with a PLANNED task must be ineligible: %+v", ready)
	}
}

// TestHistoricalizeRejectsUnknownDisposition proves an unknown disposition is a
// usage error, not a silently accepted value.
func TestHistoricalizeRejectsUnknownDisposition(t *testing.T) {
	dir := t.TempDir()
	st := completedPlan(t, dir)
	if _, err := Historicalize(HistoricalizeOptions{Dir: dir, Store: st, Disposition: "DEFERRED"}); err == nil {
		t.Fatal("an unknown disposition must be rejected")
	}
	if len(st.tasks) == 0 {
		t.Error("a rejected disposition must not mutate")
	}
}

// TestHistoricalizeRefusesWithoutActivePlan proves a no-op project is refused.
func TestHistoricalizeRefusesWithoutActivePlan(t *testing.T) {
	dir := t.TempDir()
	if _, err := Historicalize(HistoricalizeOptions{Dir: dir, Store: &fakeStore{}}); !errors.Is(err, ErrNoActivePlan) {
		t.Fatalf("err = %v, want no active plan", err)
	}
}

// TestHistoricalizeNamedPlanMustBeActive proves a named plan that is not the
// active plan is refused, so the wrong identity is never archived.
func TestHistoricalizeNamedPlanMustBeActive(t *testing.T) {
	dir := t.TempDir()
	st := completedPlan(t, dir)
	write(t, dir, "PLAN-2.md", reconcileDocTwo)

	res, err := Historicalize(HistoricalizeOptions{Dir: dir, Store: st, Plan: "PLAN-2.md"})
	if !errors.Is(err, ErrHistoricalizationIneligible) {
		t.Fatalf("err = %v, want ineligible", err)
	}
	if !strings.Contains(res.Readiness.Reason, "not the active plan") {
		t.Errorf("reason = %q, want a mismatch reason", res.Readiness.Reason)
	}
	if len(st.tasks) == 0 {
		t.Error("a mismatch refusal must not mutate")
	}
}

// TestHistoricalizeMissingPlanPathIsRefused proves a plan argument that looks
// like a path but does not resolve is a clear error, never silently treated as a
// plan id.
func TestHistoricalizeMissingPlanPathIsRefused(t *testing.T) {
	dir := t.TempDir()
	st := completedPlan(t, dir)
	if _, err := EvaluateHistoricalization(dir, st, "docs/PLAN-Missing.md", ""); err == nil {
		t.Fatal("a missing plan path must be refused, not treated as an id")
	}
	if len(st.tasks) == 0 {
		t.Error("readiness must not mutate")
	}
}
