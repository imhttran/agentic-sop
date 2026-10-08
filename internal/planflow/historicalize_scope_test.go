package planflow

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
)

// The LC-003 tests scope the historicalization readiness approval scan to the
// active plan. They are deterministic and model-free: they seed only
// approval.json/state.json fixtures under .agent-sdlc/runs/ and never invoke a
// provider. They assert that readiness mutates no run or archive artifact.

// snapshotRunArtifacts records the byte content (and existence) of every file
// under the runs directory and the whole archive directory, so a readiness-only
// evaluation can be proven to leave them byte-unchanged.
func snapshotRunArtifacts(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, root := range []string{
		filepath.Join(dir, config.DirName, "runs"),
		filepath.Join(dir, config.DirName, archiveDirName),
	} {
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.IsDir() {
				return nil
			}
			data, rerr := os.ReadFile(path)
			if rerr != nil {
				t.Fatalf("read %s: %v", path, rerr)
			}
			rel, _ := filepath.Rel(dir, path)
			out[rel] = string(data)
			return nil
		})
	}
	return out
}

// assertArtifactsUnchanged compares the snapshot taken before and after a
// readiness-only evaluation, proving no run or archive artifact moved.
func assertArtifactsUnchanged(t *testing.T, want, got map[string]string) {
	t.Helper()
	keys := make([]string, 0, len(want))
	for k := range want {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if got[k] != want[k] {
			t.Errorf("artifact %s changed during readiness: %q -> %q", k, want[k], got[k])
		}
	}
	for k := range got {
		if _, ok := want[k]; !ok {
			t.Errorf("artifact %s was created during readiness", k)
		}
	}
}

// TestHistoricalizeCrossPlanPendingApprovalStaysEligible proves a PENDING approval
// whose task is not in the active plan does not make the active plan ineligible,
// even though the request is on disk. This is the cross-plan case under I2. It
// fails against the pre-LC-003 global scan, which counted any PENDING request.
func TestHistoricalizeCrossPlanPendingApprovalStaysEligible(t *testing.T) {
	dir := t.TempDir()
	st := completedPlan(t, dir) // S001, S002 both satisfied and verified
	// A pending gate on a task that is not part of the active plan (another
	// plan's task whose run dir still carries a pending gate).
	seedPendingApproval(t, dir, "RM-003", "req-cross")

	before := snapshotRunArtifacts(t, dir)
	ready, err := EvaluateHistoricalization(dir, st, "", "")
	if err != nil {
		t.Fatalf("readiness: %v", err)
	}
	assertArtifactsUnchanged(t, before, snapshotRunArtifacts(t, dir))
	if !ready.Eligible {
		t.Fatalf("cross-plan approval must not block: %+v", ready)
	}
	if len(ready.UnresolvedApprovals) != 0 {
		t.Errorf("cross-plan approval leaked into blockers: %v", ready.UnresolvedApprovals)
	}

	res, err := Historicalize(HistoricalizeOptions{Dir: dir, Store: st})
	if err != nil {
		t.Fatalf("historicalize: %v", err)
	}
	if res.Outcome != OutcomeHistoricalized {
		t.Fatalf("outcome = %s, want %s", res.Outcome, OutcomeHistoricalized)
	}
}

// TestHistoricalizeActivePlanPendingApprovalStillBlocks proves a PENDING approval
// on a non-satisfied active-plan task still makes the plan ineligible, with the
// task id in the readiness reason (a regression of the existing
// TestHistoricalizeRefusesUnresolvedApproval).
func TestHistoricalizeActivePlanPendingApprovalStillBlocks(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", reconcileDocTwo) // both PLANNED
	markExecuted(taskByID(t, st.tasks, "S002"))                   // S002 satisfied, S001 not
	seedPassedRun(t, dir, "S002")
	seedPendingApproval(t, dir, "S001", "req-active") // active-plan, unsatisfied

	before := snapshotRunArtifacts(t, dir)
	ready, err := EvaluateHistoricalization(dir, st, "", "")
	if err != nil {
		t.Fatalf("readiness: %v", err)
	}
	assertArtifactsUnchanged(t, before, snapshotRunArtifacts(t, dir))
	if ready.Eligible {
		t.Fatal("an active-plan pending approval must make the plan ineligible")
	}
	if !strings.Contains(strings.Join(ready.UnresolvedApprovals, ","), "S001") {
		t.Errorf("unresolved approvals = %v, want S001", ready.UnresolvedApprovals)
	}
	if !strings.Contains(ready.Reason, "S001") {
		t.Errorf("reason = %q, want the active task id", ready.Reason)
	}

	res, err := Historicalize(HistoricalizeOptions{Dir: dir, Store: st})
	if !errors.Is(err, ErrHistoricalizationIneligible) {
		t.Fatalf("err = %v, want ineligible", err)
	}
	if !strings.Contains(res.Readiness.Reason, "S001") {
		t.Errorf("readiness reason = %q, want S001", res.Readiness.Reason)
	}
}

// TestHistoricalizeSatisfiedActivePlanPendingApprovalStillBlocks proves that a
// PENDING approval on an already-satisfied active-plan task (a stale gate) still
// blocks a normal historicalization under the canonical contract: an unresolved
// approval must be explicitly resolved or superseded (for example by external
// completion) before the plan is historicalized, and the historical evidence must
// never be silently discarded. Only a request on a task outside the active plan is
// out of scope.
func TestHistoricalizeSatisfiedActivePlanPendingApprovalStillBlocks(t *testing.T) {
	dir := t.TempDir()
	st := completedPlan(t, dir) // S001, S002 both satisfied and verified
	seedPendingApproval(t, dir, "S001", "req-stale")

	before := snapshotRunArtifacts(t, dir)
	ready, err := EvaluateHistoricalization(dir, st, "", "")
	if err != nil {
		t.Fatalf("readiness: %v", err)
	}
	assertArtifactsUnchanged(t, before, snapshotRunArtifacts(t, dir))
	if ready.Eligible {
		t.Fatal("an active-plan pending approval must make the plan ineligible, even when the task is satisfied")
	}
	if !strings.Contains(strings.Join(ready.UnresolvedApprovals, ","), "S001") {
		t.Errorf("unresolved approvals = %v, want S001", ready.UnresolvedApprovals)
	}
	if !strings.Contains(ready.Reason, "S001") {
		t.Errorf("reason = %q, want the active task id", ready.Reason)
	}

	res, err := Historicalize(HistoricalizeOptions{Dir: dir, Store: st})
	if !errors.Is(err, ErrHistoricalizationIneligible) {
		t.Fatalf("err = %v, want ineligible", err)
	}
	if !strings.Contains(res.Readiness.Reason, "S001") {
		t.Errorf("readiness reason = %q, want S001", res.Readiness.Reason)
	}
}

// TestHistoricalizationApprovalInPlan pins the pure scoping predicate: a pending
// approval is a blocker when its task is present in the active plan, regardless of
// task satisfaction; only a task absent from the plan is out of scope.
func TestHistoricalizationApprovalInPlan(t *testing.T) {
	tasks := []*domain.Task{
		{ID: "S001", Status: domain.PLANNED},
		{ID: "S002", Status: domain.LOCAL_DONE},
	}
	cases := []struct {
		name   string
		taskID string
		want   bool
	}{
		{"active unsatisfied", "S001", true},
		{"active satisfied", "S002", true},
		{"cross-plan", "RM-003", false},
		{"empty", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := historicalizationApprovalInPlan(tc.taskID, tasks); got != tc.want {
				t.Errorf("inPlan(%q) = %v, want %v", tc.taskID, got, tc.want)
			}
		})
	}
}
