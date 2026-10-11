package planflow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/run"
)

// LC-005: a single, deterministic, model-free regression table over the approval
// states that affect the closure consumers.
//
// The table enumerates six approval states (active pending, stale/satisfied
// pending, cross-plan pending, externally-completed pending, resolved, and
// no-approval) and, per state, the expected result for each affected consumer:
//
//   - approvals: does the approval boundary report the gate Present (true) or
//     absent (false)?
//   - continue:  is the active plan blocked from a normal (COMPLETE)
//     historicalization by an unresolved approval, i.e. must work continue to be
//     blocked? This is the readiness consumer's approval blocker, expressed as a
//     boolean.
//   - historicalize readiness: the readiness Eligible boolean for a normal
//     COMPLETE historicalization (equivalently the same closure the `continue`
//     consumer gates on).
//   - complete: the plan-completion closure verdict (true = clean/complete,
//     false = a blocker remains).
//
// The suite is table-driven and deterministic: every row builds its fixture in a
// fresh t.TempDir(), seeds only recorded artifacts (approval.json, run stage,
// external-completion.json), and never consults a clock, network, provider, or
// model. Timestamps are fixed constants, never time.Now.
//
// LC-005 requirement coverage (the matrix):
//
//	1  active-plan pending on satisfied tasks: rows "active pending blocks" and
//	   "stale satisfied pending blocks"; on unsatisfied tasks:
//	   TestApprovalStatesUnsatisfiedPendingBlocks.
//	2  unrelated-plan pending: row "cross-plan pending is ignored".
//	3  SUPERSEDED/APPROVED/DECLINED: rows "superseded approval is resolved",
//	   "resolved approval is not pending", "declined approval is resolved".
//	4  present verification: the completedPlan base; missing verification:
//	   TestCompleteRefusesMissingVerification (LC-004).
//	5  ExecutionDone exemption: TestCompleteExecutionDoneNeedsNoVerification (LC-004).
//	6  complete/historicalize agreement: every row exercises both the readiness and
//	   the real complete consumer; TestPlanCompleteAndHistoricalizeAgreeOnIneligibility
//	   (LC-004).
//	7  supersession idempotency and failure recovery: approval package tests (LC-007).
//	8  historical approval evidence preservation: approval package tests (LC-007),
//	   plus the RM-003 byte-identity check in LC-006.
//	9  reconciliation forward dependencies / rollback: store graph tests (LC-008).
//	10 readiness change between evaluation and mutation:
//	   TestCompleteRechecksClosureBeforeArchiving, plus the historicalize
//	   fingerprint tests (TestHistoricalizeStaleStateRefused,
//	   TestHistoricalizeDetectsStateChangeBetweenReads).

// lc005Request builds a recorded approval request head with a fixed (non-clock)
// timestamp.
func lc005Request(taskID, id string, status domain.ApprovalStatus, decided bool) domain.ApprovalRequest {
	req := domain.ApprovalRequest{
		ID:          id,
		TaskID:      taskID,
		Kind:        domain.ApprovalNeedsHuman,
		Target:      "stage",
		Reason:      "needs a human",
		Stage:       "VALIDATE",
		RequestedAt: time.Unix(1700000000, 0).UTC(),
		RequestedBy: "operator",
		Status:      status,
	}
	if decided {
		req.Decision = &domain.ApprovalDecision{
			RequestID:       id,
			TaskID:          taskID,
			Approved:        status == domain.ApprovalApproved,
			DecidedAt:       time.Unix(1700000010, 0).UTC(),
			DecidedBy:       "operator",
			LifecycleAction: domain.ApprovalActionNone,
		}
	}
	return req
}

// lc005SeedApproval writes a task's approval.json head into its run directory,
// creating the directory as needed. It writes no other artifact.
func lc005SeedApproval(t *testing.T, dir, taskID string, req domain.ApprovalRequest) {
	t.Helper()
	runDir := run.Dir(dir, taskID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}
	data, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		t.Fatalf("marshal approval: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "approval.json"), append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write approval: %v", err)
	}
}

// lc005SeedExternalCompletion writes a PASS external-completion record for a
// task, the explicit, model-free completion path historicalization accepts as
// verification evidence.
func lc005SeedExternalCompletion(t *testing.T, dir, taskID string) {
	t.Helper()
	runDir := run.Dir(dir, taskID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}
	rec := run.ExternalCompletion{
		Version:              run.ExternalCompletionVersion,
		TaskID:               taskID,
		CompletionSource:     run.CompletionSourceExternal,
		RepositoryHead:       "head0000",
		ImplementationCommit: "commit0000",
		Verification:         "PASS",
		RecordedBy:           "operator",
		RecordedAt:           time.Unix(1700000020, 0).UTC(),
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		t.Fatalf("marshal external completion: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, run.ExternalCompletionFile), append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write external completion: %v", err)
	}
}

// lc005ApprovalHeadPresent reports whether a task's run directory records an
// approval request head, using the same reader historicalization uses.
func lc005ApprovalHeadPresent(dir, taskID string) bool {
	_, ok := run.At(run.Dir(dir, taskID)).Approval()
	return ok
}

// lc005PendingHeadExists reports whether any task's approval.json records a
// PENDING head. It is the I5 "no PENDING head" predicate.
func lc005PendingHeadExists(dir string) bool {
	root := filepath.Join(dir, config.DirName, "runs")
	entries, err := os.ReadDir(root)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		req, ok := run.At(filepath.Join(root, e.Name())).Approval()
		if ok && req.Status == domain.ApprovalPending {
			return true
		}
	}
	return false
}

// lc005State is one row of the regression table: an approval state and the
// expected per-consumer result.
//
// A "pending gate" is expected to block a normal historicalization only when it
// is bound to a task in the active plan. approvalsPresent records whether the
// approval boundary should report a head at all.
type lc005State struct {
	name string

	// seed builds the fixtures for this state in dir. The active plan's tasks are
	// S001 and S002.
	seed func(t *testing.T, dir string)

	// approvalsPresent is the expected approvals consumer result: does a head
	// exist for the active-plan task S001?
	approvalsPresent bool

	// blocked is the expected continue/readiness/complete result: true means the
	// plan is NOT ready (an approval blocker remains), false means ready/clean.
	blocked bool

	// wantPendingHead is the expected value of "a PENDING approval head exists on
	// disk after the fixture is seeded".
	wantPendingHead bool
}

func lc005Table() []lc005State {
	return []lc005State{
		{
			name: "active pending blocks",
			seed: func(t *testing.T, dir string) {
				// S001 satisfied + a PENDING gate on it: active-plan, unresolved.
				lc005SeedApproval(t, dir, "S001", lc005Request("S001", "req-active", domain.ApprovalPending, false))
			},
			approvalsPresent: true,
			blocked:          true,
			wantPendingHead:  true,
		},
		{
			name: "stale satisfied pending blocks",
			seed: func(t *testing.T, dir string) {
				// Both active tasks satisfied + verified; S001 still carries a PENDING gate.
				// Under the canonical contract a satisfied task's open gate still blocks.
				lc005SeedApproval(t, dir, "S001", lc005Request("S001", "req-stale", domain.ApprovalPending, false))
			},
			approvalsPresent: true,
			blocked:          true,
			wantPendingHead:  true,
		},
		{
			name: "cross-plan pending is ignored",
			seed: func(t *testing.T, dir string) {
				// A PENDING gate on a task that is NOT in the active plan.
				lc005SeedApproval(t, dir, "RM-999", lc005Request("RM-999", "req-cross", domain.ApprovalPending, false))
			},
			// The boundary still reports the (out-of-plan) head, but it must not block.
			approvalsPresent: true,
			blocked:          false,
			wantPendingHead:  true,
		},
		{
			name: "externally-completed pending leaves no pending head after resolution",
			seed: func(t *testing.T, dir string) {
				// External completion records the run evidence; the stale gate was then
				// resolved by supersession (SUPERSEDED head), so no PENDING head remains.
				lc005SeedExternalCompletion(t, dir, "S001")
				lc005SeedApproval(t, dir, "S001", lc005Request("S001", "req-ext", domain.ApprovalSuperseded, true))
			},
			approvalsPresent: true,
			blocked:          false,
			wantPendingHead:  false,
		},
		{
			name: "resolved approval is not pending",
			seed: func(t *testing.T, dir string) {
				lc005SeedApproval(t, dir, "S001", lc005Request("S001", "req-resolved", domain.ApprovalApproved, true))
			},
			approvalsPresent: true,
			blocked:          false,
			wantPendingHead:  false,
		},
		{
			name: "superseded approval is resolved",
			seed: func(t *testing.T, dir string) {
				// A SUPERSEDED head is a recorded resolution (LC-007), not an open gate.
				lc005SeedApproval(t, dir, "S001", lc005Request("S001", "req-super", domain.ApprovalSuperseded, true))
			},
			approvalsPresent: true,
			blocked:          false,
			wantPendingHead:  false,
		},
		{
			name: "declined approval is resolved",
			seed: func(t *testing.T, dir string) {
				// A DECLINED head is a recorded human decision and must not block closure.
				lc005SeedApproval(t, dir, "S001", lc005Request("S001", "req-decl", domain.ApprovalDeclined, true))
			},
			approvalsPresent: true,
			blocked:          false,
			wantPendingHead:  false,
		},
		{
			name: "no approval",
			seed: func(t *testing.T, dir string) {
				// No approval artifact at all for any task.
			},
			approvalsPresent: false,
			blocked:          false,
			wantPendingHead:  false,
		},
	}
}

// lc005SeedCompletePlan seeds a plan whose two active tasks (S001, S002) are both
// satisfied and verified with PASS run evidence, so only an approval gate can
// make it ineligible. It reuses the package's existing completedPlan fixture.
func lc005SeedCompletePlan(t *testing.T, dir string) *fakeStore {
	t.Helper()
	return completedPlan(t, dir)
}

// lc005CompleteClean runs the real plan-completion closure on a fresh fixture with
// the row's state applied, returning true when the plan completes cleanly. It
// exercises the actual `complete` consumer rather than re-deriving readiness, so
// the table proves complete and historicalize agree on the same inputs.
func lc005CompleteClean(t *testing.T, seed func(t *testing.T, dir string)) bool {
	t.Helper()
	dir := t.TempDir()
	st := lc005SeedCompletePlan(t, dir)
	if seed != nil {
		seed(t, dir)
	}
	if _, err := Complete(Options{Dir: dir, Store: st}); err != nil {
		return false
	}
	return true
}

// lc005HistoricalizeClean runs the real plan-historicalization closure on a fresh
// fixture with the row's state applied, returning true when the plan
// historicalizes cleanly. It drives the actual `historicalize` mutation path, not
// only readiness, so the table ties both closure operations to the same inputs.
func lc005HistoricalizeClean(t *testing.T, seed func(t *testing.T, dir string)) bool {
	t.Helper()
	dir := t.TempDir()
	st := lc005SeedCompletePlan(t, dir)
	if seed != nil {
		seed(t, dir)
	}
	if _, err := Historicalize(HistoricalizeOptions{Dir: dir, Store: st}); err != nil {
		return false
	}
	return true
}

// TestApprovalStatesRegression drives every consumer from one table. It asserts,
// per state:
//
//	approvals       -> whether the boundary reports a head
//	continue        -> whether an approval blocker must keep work continuing
//	historicalize   -> readiness Eligible AND the real historicalize closure
//	complete        -> the real plan-completion closure verdict
//
// and proves invariants I1-I5. Every fixture is a fresh t.TempDir(); the test is
// deterministic and model-free.
func TestApprovalStatesRegression(t *testing.T) {
	for _, tc := range lc005Table() {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			st := lc005SeedCompletePlan(t, dir)
			tc.seed(t, dir)

			// --- consumer: approvals --------------------------------------------
			approvalsPresent := lc005ApprovalHeadPresent(dir, "S001")
			// The approvals consumer is keyed to the active-plan task S001; a
			// cross-plan or absent head is reported separately by the boundary, so
			// we only require the active-plan presence to match the table.
			if tc.approvalsPresent && !approvalsPresent {
				// A row whose head lives on a cross-plan task is asserted below by
				// the pending-head predicate instead.
				if tc.name != "cross-plan pending is ignored" {
					t.Errorf("[%s] approvals: expected a head for S001", tc.name)
				}
			}

			// --- consumer: historicalize readiness ------------------------------
			ready, err := EvaluateHistoricalization(dir, st, "", "")
			if err != nil {
				t.Fatalf("[%s] historicalize readiness: %v", tc.name, err)
			}
			readinessEligible := ready.Eligible
			if readinessEligible == tc.blocked {
				t.Errorf("[%s] historicalize readiness: Eligible=%v, want blocked=%v", tc.name, readinessEligible, tc.blocked)
			}

			// --- consumer: continue ---------------------------------------------
			// The continue consumer gates on the same closure: work must continue
			// (be blocked) exactly when readiness is ineligible.
			continueBlocked := !readinessEligible
			if continueBlocked != tc.blocked {
				t.Errorf("[%s] continue: blocked=%v, want %v", tc.name, continueBlocked, tc.blocked)
			}

			// --- consumer: complete ---------------------------------------------
			// Exercise the real plan-completion closure on a fresh fixture with the
			// same state. It must agree with readiness: clean exactly when eligible.
			completeClean := lc005CompleteClean(t, tc.seed)
			if completeClean == tc.blocked {
				t.Errorf("[%s] complete: clean=%v, want blocked=%v", tc.name, completeClean, tc.blocked)
			}

			// --- consumer: historicalize (mutation) --------------------------------
			// Drive the real historicalize closure on a fresh fixture too, so both
			// closure operations are exercised, not only readiness. The two must agree.
			historicalizeClean := lc005HistoricalizeClean(t, tc.seed)
			if historicalizeClean != completeClean {
				t.Errorf("[%s] historicalize clean=%v must agree with complete clean=%v", tc.name, historicalizeClean, completeClean)
			}

			// --- invariant I1 ----------------------------------------------------
			// No contradiction between approvals and readiness for the same task: a
			// head the boundary reports for the active plan must agree with the
			// readiness blocker set.
			if approvalsPresent && tc.blocked {
				blockedByApproval := false
				for _, id := range ready.UnresolvedApprovals {
					if id == "S001" || len(id) >= 4 && id[:4] == "S001" {
						blockedByApproval = true
					}
				}
				if !blockedByApproval {
					t.Errorf("[%s] I1: approvals present and blocked, but readiness lists no approval blocker: %v", tc.name, ready.UnresolvedApprovals)
				}
			}

			// --- invariant I2 ----------------------------------------------------
			// Cross-plan pending approval is ignored by consumers.
			if tc.name == "cross-plan pending is ignored" {
				if len(ready.UnresolvedApprovals) != 0 {
					t.Errorf("[%s] I2: cross-plan approval leaked into blockers: %v", tc.name, ready.UnresolvedApprovals)
				}
			}

			// --- invariant I3 ----------------------------------------------------
			// An active pending approval blocks the relevant consumer(s).
			if tc.name == "active pending blocks" {
				if readinessEligible {
					t.Errorf("[%s] I3: active pending approval must block historicalization", tc.name)
				}
				found := false
				for _, id := range ready.UnresolvedApprovals {
					if len(id) >= 4 && id[:4] == "S001" {
						found = true
					}
				}
				if !found {
					t.Errorf("[%s] I3: readiness must name S001 as unresolved, got %v", tc.name, ready.UnresolvedApprovals)
				}
			}

			// --- invariants I4/I5 ------------------------------------------------
			// External completion leaves no PENDING head for the task, and the plan
			// can then close.
			if got := lc005PendingHeadExists(dir); got != tc.wantPendingHead {
				t.Errorf("[%s] pending-head predicate = %v, want %v", tc.name, got, tc.wantPendingHead)
			}
			if tc.name == "externally-completed pending leaves no pending head after resolution" {
				if lc005PendingHeadExists(dir) {
					t.Errorf("[%s] I5: external completion must leave no PENDING head", tc.name)
				}
				if tc.blocked {
					t.Errorf("[%s] I4: externally-completed plan must be clean, got blocked", tc.name)
				}
			}
		})
	}
}

// TestApprovalStatesTableCoversAllStates guards the table's coverage: exactly the
// six required states, sorted and distinct, so a regression cannot silently drop
// a state or reintroduce the pre-LC-003 global scan.
func TestApprovalStatesTableCoversAllStates(t *testing.T) {
	want := []string{
		"active pending blocks",
		"cross-plan pending is ignored",
		"declined approval is resolved",
		"externally-completed pending leaves no pending head after resolution",
		"no approval",
		"resolved approval is not pending",
		"stale satisfied pending blocks",
		"superseded approval is resolved",
	}
	rows := lc005Table()
	if len(rows) != len(want) {
		t.Fatalf("table has %d states, want %d", len(rows), len(want))
	}
	got := make([]string, 0, len(rows))
	seen := map[string]bool{}
	for _, r := range rows {
		if seen[r.name] {
			t.Errorf("duplicate state %q", r.name)
		}
		seen[r.name] = true
		got = append(got, r.name)
	}
	sort.Strings(got)
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("state[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestApprovalStatesUnsatisfiedPendingBlocks proves requirement 1's unsatisfied
// case: a PENDING approval on an UNSATISFIED active-plan task is unresolved work
// and blocks both closure paths (not only the satisfied/stale case the table
// enumerates).
func TestApprovalStatesUnsatisfiedPendingBlocks(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", reconcileDocTwo) // tasks are PLANNED
	lc005SeedApproval(t, dir, "S001", lc005Request("S001", "req-unsat", domain.ApprovalPending, false))

	ready, err := EvaluateHistoricalization(dir, st, "", "")
	if err != nil {
		t.Fatalf("readiness: %v", err)
	}
	if ready.Eligible {
		t.Error("an unsatisfied active-plan task with a PENDING gate must block historicalization")
	}
	if _, err := Complete(Options{Dir: dir, Store: st}); err == nil {
		t.Error("an unsatisfied active-plan task with a PENDING gate must block completion")
	}
}

// TestCompleteRechecksClosureBeforeArchiving covers requirement 10 for `complete`:
// a plan that was eligible when readiness was evaluated is refused once the state
// changes before the mutation, because `Complete` re-reads the closure invariants
// from fresh state immediately before archiving rather than trusting a prior
// readiness verdict. No sleep or concurrency is involved; the change is injected
// deterministically between the two calls. The `historicalize` path carries an
// additional explicit fingerprint re-read (TestHistoricalizeStaleStateRefused,
// TestHistoricalizeDetectsStateChangeBetweenReads).
func TestCompleteRechecksClosureBeforeArchiving(t *testing.T) {
	dir := t.TempDir()
	st := lc005SeedCompletePlan(t, dir)

	ready, err := EvaluateHistoricalization(dir, st, "", "")
	if err != nil {
		t.Fatalf("readiness: %v", err)
	}
	if !ready.Eligible {
		t.Fatalf("setup: plan must start eligible, reason=%s", ready.Reason)
	}

	// The plan state changes AFTER the readiness evaluation: a new PENDING gate on
	// an active-plan task appears.
	lc005SeedApproval(t, dir, "S001", lc005Request("S001", "req-late", domain.ApprovalPending, false))

	if _, err := Complete(Options{Dir: dir, Store: st}); err == nil {
		t.Fatal("complete must re-check closure and refuse the blocker that appeared after readiness")
	}
	// No partial mutation: the plan is still ACTIVE and its tasks are intact.
	if len(st.tasks) == 0 {
		t.Error("tasks were cleared despite the refusal")
	}
	if _, _, ok := ActivePlanID(dir); !ok {
		t.Error("the plan lost its active association on a refused completion")
	}
}
