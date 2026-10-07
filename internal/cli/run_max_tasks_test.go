package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/domain"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/store"
)

// The operator execution bound (--max-tasks) must guarantee that a bounded
// invocation never ENTERS task N+1. These tests use a three-stage linear plan
// (S001 -> S002 -> S003) and the existing fake/scripted agent, so they are
// deterministic, model-free, and provider-neutral.

const maxTasksPlanJSON = `{
  "project": "Bounded Run",
  "summary": "Three linear stages for the operator execution-bound tests.",
  "stages": [
    {
      "id": "S001",
      "title": "First",
      "objective": "First stage.",
      "dependencies": [],
      "deliverables": ["first"],
      "acceptance_criteria": ["first done"]
    },
    {
      "id": "S002",
      "title": "Second",
      "objective": "Second stage.",
      "dependencies": ["S001"],
      "deliverables": ["second"],
      "acceptance_criteria": ["second done"]
    },
    {
      "id": "S003",
      "title": "Third",
      "objective": "Third stage.",
      "dependencies": ["S002"],
      "deliverables": ["third"],
      "acceptance_criteria": ["third done"]
    }
  ]
}`

// threeStageGraph initializes a project whose persisted plan is the three-stage
// linear plan above, so a normal `sop run` would execute S001 -> S002 -> S003.
func threeStageGraph(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	initProject(t, dir)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
	writePlanJSON(t, dir, maxTasksPlanJSON)
	return dir
}

// threeStageAgent is a scripted model that completes each task with a clean review.
func threeStageAgent() *fakeCapabilityAgent {
	return &fakeCapabilityAgent{plan: maxTasksPlanJSON, impl: "changed", review: `{"summary":"clean","findings":[]}`}
}

func graphTaskStatus(t *testing.T, dir, id string) domain.TaskStatus {
	t.Helper()
	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	got, err := st.Get(id)
	if err != nil {
		t.Fatalf("get %s: %v", id, err)
	}
	return got.Status
}

func graphRunDirExists(dir, id string) bool {
	_, err := os.Stat(filepath.Join(dir, stateDirName, "runs", id))
	return err == nil
}

// 1 + 10. Without --max-tasks, existing unbounded behavior is unchanged.
func TestRunMaxTasksOmittedIsUnbounded(t *testing.T) {
	dir := threeStageGraph(t)
	code, stdout, stderr := runInjectedCLI(t, dir, "diff --git a/x b/x\n", threeStageAgent(), "run")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "all tasks done") {
		t.Errorf("stdout = %q, want all tasks done", stdout)
	}
	if strings.Contains(stdout, "execution bound reached") {
		t.Errorf("stdout = %q, a run without --max-tasks must not report a bound", stdout)
	}
	for _, id := range []string{"S001", "S002", "S003"} {
		if got := graphTaskStatus(t, dir, id); got != domain.LOCAL_DONE {
			t.Errorf("%s status = %s, want LOCAL_DONE", id, got)
		}
	}
}

// 2. --max-tasks 1: the first task may enter; the second never enters.
func TestRunMaxTasksOneStopsBeforeSecond(t *testing.T) {
	dir := threeStageGraph(t)
	code, stdout, stderr := runInjectedCLI(t, dir, "diff --git a/x b/x\n", threeStageAgent(), "run", "--max-tasks", "1")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "execution bound reached after 1 task") {
		t.Errorf("stdout = %q, want the bounded stop for 1 task", stdout)
	}
	if got := graphTaskStatus(t, dir, "S001"); got != domain.LOCAL_DONE {
		t.Errorf("S001 = %s, want LOCAL_DONE", got)
	}
	if got := graphTaskStatus(t, dir, "S002"); got != domain.PLANNED {
		t.Errorf("S002 = %s, want PLANNED (never entered)", got)
	}
	if graphRunDirExists(dir, "S002") {
		t.Error("S002 was entered despite --max-tasks 1")
	}
}

// 3. --max-tasks 2: exactly two tasks may enter; the third never enters.
func TestRunMaxTasksTwoStopsBeforeThird(t *testing.T) {
	dir := threeStageGraph(t)
	code, stdout, stderr := runInjectedCLI(t, dir, "diff --git a/x b/x\n", threeStageAgent(), "run", "--max-tasks", "2")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "execution bound reached after 2 task") {
		t.Errorf("stdout = %q, want the bounded stop for 2 tasks", stdout)
	}
	for _, id := range []string{"S001", "S002"} {
		if got := graphTaskStatus(t, dir, id); got != domain.LOCAL_DONE {
			t.Errorf("%s = %s, want LOCAL_DONE", id, got)
		}
	}
	if got := graphTaskStatus(t, dir, "S003"); got != domain.PLANNED {
		t.Errorf("S003 = %s, want PLANNED (never entered)", got)
	}
	if graphRunDirExists(dir, "S003") {
		t.Error("S003 was entered despite --max-tasks 2")
	}
}

// The --max-tasks=N spelling is accepted and equivalent.
func TestRunMaxTasksEqualsForm(t *testing.T) {
	dir := threeStageGraph(t)
	code, stdout, stderr := runInjectedCLI(t, dir, "diff --git a/x b/x\n", threeStageAgent(), "run", "--max-tasks=2")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "execution bound reached after 2 task") {
		t.Errorf("stdout = %q, want the bounded stop for 2 tasks", stdout)
	}
	if got := graphTaskStatus(t, dir, "S003"); got != domain.PLANNED {
		t.Errorf("S003 = %s, want PLANNED", got)
	}
}

// 4. A failure before the bound preserves existing failure behavior: the run does
// not continue merely to consume N tasks, and the bound is never reported.
func TestRunMaxTasksFailureWins(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"exit 1\"\n")
	writePlanJSON(t, dir, maxTasksPlanJSON)
	code, stdout, _ := runInjectedCLI(t, dir, "diff --git a/x b/x\n", threeStageAgent(), "run", "--max-tasks", "2")
	if code == exitOK {
		t.Fatalf("code = exitOK, want a non-OK outcome for a failing first task")
	}
	if strings.Contains(stdout, "execution bound reached") {
		t.Errorf("stdout = %q, the bound must not be reported when a task failed", stdout)
	}
	if got := graphTaskStatus(t, dir, "S002"); got != domain.PLANNED {
		t.Errorf("S002 = %s, want PLANNED (a failure stops the graph)", got)
	}
}

// 5. A blocking review before the bound preserves existing block behavior.
func TestRunMaxTasksBlockWins(t *testing.T) {
	dir := threeStageGraph(t)
	blocking := &fakeCapabilityAgent{plan: maxTasksPlanJSON, impl: "changed", review: `{"summary":"blocked","findings":[{"severity":"HIGH","title":"boom","file":"x","line":1}]}`}
	code, stdout, _ := runInjectedCLI(t, dir, "diff --git a/x b/x\n", blocking, "run", "--max-tasks", "2")
	if code == exitOK {
		t.Fatalf("code = exitOK, want a non-OK outcome for a blocking review")
	}
	if strings.Contains(stdout, "execution bound reached") {
		t.Errorf("stdout = %q, the bound must not mask a block", stdout)
	}
	if got := graphTaskStatus(t, dir, "S002"); got != domain.PLANNED {
		t.Errorf("S002 = %s, want PLANNED", got)
	}
}

// 6. An approval/human boundary before the bound preserves existing approval
// behavior: the run parks at the gate and the bound does not apply.
func TestRunMaxTasksApprovalBoundaryWins(t *testing.T) {
	dir := threeStageGraph(t)
	rn, err := runpkg.New(dir, "S001")
	if err != nil {
		t.Fatal(err)
	}
	if err := rn.SetStage(runpkg.WaitingForHuman); err != nil {
		t.Fatal(err)
	}
	a := outcomeAgent{outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: "the implementation is incomplete"}}
	code, stdout, _ := runInjectedCLI(t, dir, "diff\n", a, "run", "--max-tasks", "2")
	if code == exitOK {
		t.Fatalf("code = exitOK, want the human boundary to stop the run")
	}
	if !strings.Contains(stdout, "NEEDS_HUMAN") {
		t.Errorf("stdout = %q, want NEEDS_HUMAN", stdout)
	}
	if strings.Contains(stdout, "execution bound reached") {
		t.Errorf("stdout = %q, the bound must not mask an approval boundary", stdout)
	}
	if got := graphTaskStatus(t, dir, "S002"); got != domain.PLANNED {
		t.Errorf("S002 = %s, want PLANNED", got)
	}
}

// 7. Invalid values fail before any task is entered/created.
func TestRunMaxTasksInvalidValuesEnterNoTask(t *testing.T) {
	for _, arg := range []string{"0", "-1", "abc", "", "2.5"} {
		t.Run(arg, func(t *testing.T) {
			dir := threeStageGraph(t)
			code, _, stderr := runInjectedCLI(t, dir, "diff\n", threeStageAgent(), "run", "--max-tasks", arg)
			if code != exitUsage {
				t.Fatalf("code=%d, want %d (usage)", code, exitUsage)
			}
			if !strings.Contains(stderr, "invalid --max-tasks") {
				t.Errorf("stderr = %q, want an invalid --max-tasks message", stderr)
			}
			st, err := store.Open(statePath(dir))
			if err != nil {
				t.Fatalf("open store: %v", err)
			}
			defer st.Close()
			list, err := st.List()
			if err != nil {
				t.Fatal(err)
			}
			if len(list) != 0 {
				t.Errorf("tasks = %d, want 0 (no task created or entered)", len(list))
			}
		})
	}
}

// 8. A bounded stop fabricates no FAILED/BLOCKED/NO_PROGRESS state: the entered
// tasks are LOCAL_DONE, and the un-entered task is untouched.
func TestRunMaxTasksBoundedStopFabricatesNoState(t *testing.T) {
	dir := threeStageGraph(t)
	code, _, stderr := runInjectedCLI(t, dir, "diff --git a/x b/x\n", threeStageAgent(), "run", "--max-tasks", "2")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	untouched, err := st.Get("S003")
	if err != nil {
		t.Fatal(err)
	}
	if untouched.Status != domain.PLANNED || untouched.Attempt != 0 || len(untouched.Attempts) != 0 {
		t.Errorf("S003 = status=%s attempt=%d attempts=%d, want PLANNED/0/0 (no fabricated history)",
			untouched.Status, untouched.Attempt, len(untouched.Attempts))
	}
	if got := graphRunDirExists(dir, "S003"); got {
		t.Error("S003 must have no run artifacts (it was never entered)")
	}
	for _, id := range []string{"S001", "S002"} {
		g, err := st.Get(id)
		if err != nil {
			t.Fatal(err)
		}
		if g.Status != domain.LOCAL_DONE {
			t.Errorf("%s = %s, want LOCAL_DONE (the bound is not a failure)", id, g.Status)
		}
	}
}

// 9. A subsequent bounded invocation resumes from the correct next task.
func TestRunMaxTasksResumeContinuesFromNextTask(t *testing.T) {
	dir := threeStageGraph(t)
	if code, _, stderr := runInjectedCLI(t, dir, "diff --git a/x b/x\n", threeStageAgent(), "run", "--max-tasks", "1"); code != exitOK {
		t.Fatalf("first bounded run: code=%d stderr=%s", code, stderr)
	}
	if got := graphTaskStatus(t, dir, "S002"); got != domain.PLANNED {
		t.Fatalf("S002 = %s after the first run, want PLANNED", got)
	}
	code, stdout, stderr := runInjectedCLI(t, dir, "diff --git a/x b/x\n", threeStageAgent(), "run", "--max-tasks", "1")
	if code != exitOK {
		t.Fatalf("second bounded run: code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "execution bound reached after 1 task") {
		t.Errorf("stdout = %q, want a bounded stop after 1 task", stdout)
	}
	if got := graphTaskStatus(t, dir, "S002"); got != domain.LOCAL_DONE {
		t.Errorf("S002 = %s, want LOCAL_DONE after resume", got)
	}
	if got := graphTaskStatus(t, dir, "S003"); got != domain.PLANNED {
		t.Errorf("S003 = %s, want PLANNED", got)
	}
}
