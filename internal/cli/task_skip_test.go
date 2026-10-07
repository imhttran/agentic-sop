package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/domain"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/store"
)

const skipEvidence = "docs/reports/audit.md"

// setupSkipProject makes a two-stage project (S002 depends on S001) with S001 complete
// and an evidence file on disk.
func setupSkipProject(t *testing.T) (dir, head string) {
	t.Helper()
	dir = t.TempDir()
	head = setupExtProject(t, dir, extCompleteConfig, extPlanTwo)
	if code, _, errOut := runCLIWithAgent(t, dir, nil, "task", "complete", "S001", "--external", "--commit", head); code != exitOK {
		t.Fatalf("complete S001: %s", errOut)
	}
	writeFileT(t, dir, skipEvidence, "S002: NOT_REQUIRED\n")
	return dir, head
}

func loadTask(t *testing.T, dir, id string) (*domain.Task, map[string]*domain.Task) {
	t.Helper()
	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	all, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]*domain.Task{}
	for _, task := range all {
		byID[task.ID] = task
	}
	return byID[id], byID
}

func skip(t *testing.T, dir string, args ...string) (int, string, string) {
	t.Helper()
	return runCLIWithAgent(t, dir, nil, append([]string{"task", "skip"}, args...)...)
}

// TestTaskSkipPlanned proves PLANNED -> NOT_REQUIRED persists the reason in history,
// writes an evidence-pinned record, and mutates no tracked repository file.
func TestTaskSkipPlanned(t *testing.T) {
	dir, head := setupSkipProject(t)
	code, out, errOut := skip(t, dir, "S002", "--reason", "audit found no gap", "--evidence", skipEvidence)
	if code != exitOK {
		t.Fatalf("skip: code=%d stdout=%s stderr=%s", code, out, errOut)
	}
	task, _ := loadTask(t, dir, "S002")
	if task.Status != domain.NOT_REQUIRED {
		t.Fatalf("status = %s, want NOT_REQUIRED", task.Status)
	}
	if n := len(task.Attempts); n != 1 || task.Attempts[0].Reason != "audit found no gap" {
		t.Errorf("persisted history = %+v, want the disposition reason", task.Attempts)
	}

	data, err := os.ReadFile(filepath.Join(dir, stateDirName, "runs", "S002", runpkg.NotRequiredFile))
	if err != nil {
		t.Fatalf("record missing: %v", err)
	}
	var rec runpkg.NotRequired
	if err := json.Unmarshal(data, &rec); err != nil {
		t.Fatal(err)
	}
	if rec.PreviousStatus != "PLANNED" || rec.RepositoryHead != head || len(rec.Evidence) != 1 || rec.Evidence[0].Path != skipEvidence || len(rec.Evidence[0].SHA256) != 64 {
		t.Errorf("record = %+v", rec)
	}

	// No repository mutation: HEAD and every tracked file are unchanged.
	cmd := exec.Command("git", "diff", "--exit-code", "HEAD")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Errorf("tracked files changed: %s", out)
	}

	_, rep, _ := runCLIWithAgent(t, dir, nil, "report", "S002")
	if !strings.Contains(rep, "Disposition: NOT_REQUIRED") || !strings.Contains(rep, "audit found no gap") {
		t.Errorf("report does not show the disposition:\n%s", rep)
	}
	_, detail, _ := runCLIWithAgent(t, dir, nil, "task", "S002")
	if !strings.Contains(detail, "Status: NOT_REQUIRED") || !strings.Contains(detail, "Not required: audit found no gap") {
		t.Errorf("task detail does not show the disposition:\n%s", detail)
	}
}

// TestTaskSkipBlocked proves BLOCKED -> NOT_REQUIRED keeps the earlier attempts and
// the stale run report, which the disposition supersedes.
func TestTaskSkipBlocked(t *testing.T) {
	dir, _ := setupSkipProject(t)
	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	task, _ := st.Get("S002")
	_ = task.AddAttempt(domain.BLOCKED, "IMPLEMENT_NO_PROGRESS")
	if err := task.Block(domain.NO_PROGRESS); err != nil {
		t.Fatal(err)
	}
	if err := st.Save(task); err != nil {
		t.Fatal(err)
	}
	st.Close()
	writeFileT(t, dir, stateDirName+"/runs/S002/trace.json", "{}\n")

	if code, _, errOut := skip(t, dir, "S002", "--reason", "gate", "--evidence", skipEvidence); code != exitOK {
		t.Fatalf("skip: %s", errOut)
	}
	task, _ = loadTask(t, dir, "S002")
	if task.Status != domain.NOT_REQUIRED || task.BlockedReason != domain.NO_REASON {
		t.Fatalf("status=%s reason=%s", task.Status, task.BlockedReason)
	}
	if len(task.Attempts) != 2 || task.Attempts[0].Reason != "IMPLEMENT_NO_PROGRESS" || task.Attempts[1].Status != domain.NOT_REQUIRED {
		t.Errorf("history = %+v", task.Attempts)
	}
	if _, err := os.Stat(filepath.Join(dir, stateDirName, "runs", "S002", "trace.json")); err != nil {
		t.Errorf("prior run evidence was not preserved: %v", err)
	}
}

// TestTaskSkipProgressionNotSuccess proves a NOT_REQUIRED task satisfies its dependants
// and plan completion, but is never counted as completed work.
func TestTaskSkipProgressionNotSuccess(t *testing.T) {
	dir := t.TempDir()
	setupExtProject(t, dir, extCompleteConfig, extPlanTwo)
	writeFileT(t, dir, skipEvidence, "x\n")
	if code, _, errOut := skip(t, dir, "S001", "--reason", "not needed", "--evidence", skipEvidence); code != exitOK {
		t.Fatalf("skip: %s", errOut)
	}
	next, byID := loadTask(t, dir, "S002")
	if _, ok := next.ResolveDependencies(byID); !ok {
		t.Error("S002 must become runnable once S001 is NOT_REQUIRED")
	}
	done, notRequired, _, _ := statusCounts([]*domain.Task{byID["S001"], byID["S002"]})
	if done != 0 || notRequired != 1 {
		t.Errorf("counts done=%d notRequired=%d, want 0 and 1", done, notRequired)
	}
	if code, _, _ := runCLIWithAgent(t, dir, nil, "retry", "S001"); code == exitOK {
		t.Error("a NOT_REQUIRED task must not be retried")
	}
}

func TestTaskSkipRejects(t *testing.T) {
	cases := []struct {
		name string
		id   string
		args []string
		gate bool
		code int
	}{
		{"missing reason", "S002", []string{"--evidence", skipEvidence}, false, exitUsage},
		{"missing evidence", "S002", []string{"--reason", "r"}, false, exitUsage},
		{"evidence outside repo", "S002", []string{"--reason", "r", "--evidence", "../x.md"}, false, exitError},
		{"missing evidence file", "S002", []string{"--reason", "r", "--evidence", "docs/nope.md"}, false, exitError},
		{"completed task", "S001", []string{"--reason", "r", "--evidence", skipEvidence}, false, exitError},
		{"pending human gate", "S002", []string{"--reason", "r", "--evidence", skipEvidence}, true, exitError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir, _ := setupSkipProject(t)
			if tc.gate {
				seedGate(t, dir, tc.id)
			}
			before, _ := loadTask(t, dir, tc.id)
			if code, _, _ := skip(t, dir, append([]string{tc.id}, tc.args...)...); code != tc.code {
				t.Fatalf("code = %d, want %d", code, tc.code)
			}
			after, _ := loadTask(t, dir, tc.id)
			if after.Status != before.Status || len(after.Attempts) != len(before.Attempts) {
				t.Errorf("task changed on a rejected skip: %s -> %s", before.Status, after.Status)
			}
			if _, err := os.Stat(filepath.Join(dir, stateDirName, "runs", tc.id, runpkg.NotRequiredFile)); err == nil {
				t.Error("a rejected skip must not write a record")
			}
		})
	}
}

func TestTaskSkipRejectsUnsatisfiedDependency(t *testing.T) {
	dir := t.TempDir()
	setupExtProject(t, dir, extCompleteConfig, extPlanTwo)
	writeFileT(t, dir, skipEvidence, "x\n")
	if code, _, _ := skip(t, dir, "S002", "--reason", "r", "--evidence", skipEvidence); code == exitOK {
		t.Fatal("skipping a task with an unsatisfied dependency must be rejected")
	}
	if taskStatus(t, dir, "S002") != domain.PLANNED {
		t.Error("S002 must be unchanged")
	}
}
