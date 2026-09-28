package cli

import (
	"context"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/store"
)

// seedRecoveryScenario creates the representative scenario state:
// odd-numbered tasks are BLOCKED (recoverable), even-numbered tasks are already
// LOCAL_DONE. It is the fixture TASK005-S2 requires.
func seedRecoveryScenario(t *testing.T, dir string) {
	t.Helper()
	initProject(t, dir)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
	for i := 1; i <= 8; i++ {
		id := recoveryID(i)
		if i%2 == 0 {
			seedTask(t, dir, &domain.Task{ID: id, Title: id, Status: domain.LOCAL_DONE, MaxAttempts: 3})
			continue
		}
		seedTask(t, dir, &domain.Task{ID: id, Title: id, Status: domain.BLOCKED, BlockedReason: domain.REVIEW_UNRESOLVED, Attempt: 1, MaxAttempts: 3})
	}
}

func recoveryID(i int) string {
	return "T00" + string(rune('0'+i))
}

// recoveryAgent makes every recovered BLOCKED task pass, so a scenario run can
// prove SOP continues through the plan within one invocation. failOn, when set,
// makes the task whose id it names fail (block).
type recoveryAgent struct {
	failOn string
}

func (a *recoveryAgent) Generate(_ context.Context, r agent.Request) (agent.Response, error) {
	switch r.Capability {
	case agent.Plan:
		return agent.Response{Content: validPlanJSON}, nil
	case agent.Implement:
		if a.failOn != "" && strings.Contains(r.Task, a.failOn) {
			return agent.Response{Content: "blocked", Outcome: &agent.Outcome{Status: agent.OutcomeFailed, Reason: "boom"}}, nil
		}
		return agent.Response{Content: "done"}, nil
	case agent.Review:
		return agent.Response{Content: `{"summary":"clean","findings":[]}`}, nil
	default:
		return agent.Response{Content: "{}"}, nil
	}
}

// TestRecoveryContinuesThroughPlan proves a successful automatic recovery lets
// the same `sop run` invocation continue: each blocked task is recovered to
// PASS, interleaved LOCAL_DONE tasks are skipped, and multiple historical blocked
// tasks are recovered sequentially in one invocation.
func TestRecoveryContinuesThroughPlan(t *testing.T) {
	dir := t.TempDir()
	seedRecoveryScenario(t, dir)
	a := &recoveryAgent{}

	code, stdout, stderr := runInjectedCLI(t, dir, "diff\n", a, "run")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}

	// Each blocked task is reported as being recovered and then runs to LOCAL_DONE.
	for _, id := range []string{"T001", "T003", "T005", "T007"} {
		if !strings.Contains(stdout, "Recovering: "+id) {
			t.Errorf("stdout missing recovery of %s:\n%s", id, stdout)
		}
		if !strings.Contains(stdout, id+" LOCAL_DONE") {
			t.Errorf("stdout missing completion of %s:\n%s", id, stdout)
		}
	}

	// Completed tasks are never selected.
	for _, id := range []string{"T002", "T004", "T006", "T008"} {
		if strings.Contains(stdout, "Running: "+id) {
			t.Errorf("completed task %s was re-run:\n%s", id, stdout)
		}
	}

	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	for _, id := range []string{"T001", "T002", "T003", "T004", "T005", "T006", "T007", "T008"} {
		task, err := st.Get(id)
		if err != nil {
			t.Fatalf("get %s: %v", id, err)
		}
		if task.Status != domain.LOCAL_DONE {
			t.Errorf("%s status = %s, want LOCAL_DONE", id, task.Status)
		}
	}
}

// TestRecoveryFailureStopsInvocation proves a recovery failure stops the run
// immediately: a later blocked task is neither executed nor recovered.
func TestRecoveryFailureStopsInvocation(t *testing.T) {
	dir := t.TempDir()
	seedRecoveryScenario(t, dir)
	// T003 recovers and passes; T005 fails, so T007 must not be touched.
	a := &recoveryAgent{failOn: "T005"}

	code, stdout, _ := runInjectedCLI(t, dir, "diff\n", a, "run")
	if code != exitError {
		t.Fatalf("code=%d, want %d; stdout=%s", code, exitError, stdout)
	}
	if !strings.Contains(stdout, "T005 BLOCKED") {
		t.Errorf("stdout = %q, want T005 reported BLOCKED", stdout)
	}
	if strings.Contains(stdout, "Recovering: T007") || strings.Contains(stdout, "Running: T007") {
		t.Errorf("T007 must not be executed or recovered after T005 failed:\n%s", stdout)
	}

	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	if task, err := st.Get("T001"); err != nil || task.Status != domain.LOCAL_DONE {
		t.Errorf("T001 = %v, %v; want LOCAL_DONE", task, err)
	}
	if task, err := st.Get("T007"); err != nil || task.Status != domain.BLOCKED {
		t.Errorf("T007 = %v, %v; want untouched BLOCKED", task, err)
	}
}

// TestRecoveryDoesNotRepeatSameTask proves the per-invocation one-attempt bound:
// a task that recovered successfully is not recovered again in the same run.
func TestRecoveryDoesNotRepeatSameTask(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "T001", Status: domain.BLOCKED, BlockedReason: domain.REVIEW_UNRESOLVED, Attempt: 1, MaxAttempts: 3})
	a := &recoveryAgent{}

	code, stdout, stderr := runInjectedCLI(t, dir, "diff\n", a, "run")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if got := strings.Count(stdout, "Recovering: T001"); got != 1 {
		t.Errorf("T001 recovered %d time(s), want exactly once:\n%s", got, stdout)
	}
}

// TestRecoverySkipsCompletedTasksAfterSuccess proves already-satisfied tasks are
// skipped and normal runnable work is preferred over recovery.
func TestRecoverySkipsCompletedTasksAfterSuccess(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n")
	seedTask(t, dir, &domain.Task{ID: "T001", Title: "T001", Status: domain.LOCAL_DONE, MaxAttempts: 3})
	seedTask(t, dir, &domain.Task{ID: "T002", Title: "T002", Status: domain.PLANNED, MaxAttempts: 3})
	seedTask(t, dir, &domain.Task{ID: "T003", Title: "T003", Status: domain.BLOCKED, BlockedReason: domain.REVIEW_UNRESOLVED, Attempt: 1, MaxAttempts: 3})
	a := &recoveryAgent{}

	code, stdout, stderr := runInjectedCLI(t, dir, "diff\n", a, "run")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	// Normal runnable work (T002) is preferred over recovery (T003).
	if strings.Contains(stdout, "Running: T001") {
		t.Errorf("completed T001 was re-run:\n%s", stdout)
	}
	for _, id := range []string{"T002", "T003"} {
		if !strings.Contains(stdout, "Running: "+id) {
			t.Errorf("stdout missing execution of %s:\n%s", id, stdout)
		}
	}
	if !strings.Contains(stdout, "Recovering: T003") {
		t.Errorf("stdout missing recovery of T003:\n%s", stdout)
	}
}
