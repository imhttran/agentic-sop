package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/store"
)

// gitRepo initializes a git repository with one commit and returns its HEAD.
func gitRepo(t *testing.T, dir string) string {
	t.Helper()
	run := func(args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	run("init", "-q")
	run("config", "user.email", "t@e")
	run("config", "user.name", "t")
	writeFile(t, dir, "README.md", "x\n")
	run("add", "-A")
	run("commit", "-q", "-m", "init")
	return run("rev-parse", "HEAD")
}

const extCompleteConfig = "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n"
const extFailConfig = "project:\n  name: x\nvalidation:\n  build:\n    - \"false\"\n"
const extPlanTwo = "# Implementation Plan\n\n## Project\n\nWidget\n\n## Summary\n\nTwo stages.\n\n## S001 — First\n\nDo it.\n\n### Acceptance Criteria\n\n- a\n\n## S002 — Second\n\nDo it.\n\n### Dependencies\n\n- S001\n\n### Acceptance Criteria\n\n- b\n"

// setupExtProject makes a git repo + initialized SOP project with a plan activated, and
// returns HEAD.
func setupExtProject(t *testing.T, dir, cfg, planDoc string) string {
	t.Helper()
	head := gitRepo(t, dir)
	initProject(t, dir)
	writeConfig(t, dir, cfg)
	writeFile(t, dir, "PLAN.md", planDoc)
	if code, _, errOut := runCLIWithAgent(t, dir, &planCountAgent{}, "plan", "activate", "PLAN.md"); code != exitOK {
		t.Fatalf("activate: code=%d stderr=%s", code, errOut)
	}
	return head
}

func taskStatus(t *testing.T, dir, id string) domain.TaskStatus {
	t.Helper()
	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	task, err := st.Get(id)
	if err != nil {
		t.Fatalf("get %s: %v", id, err)
	}
	return task.Status
}

// TestTaskCompleteExternal proves an explicit external completion records the task in the
// local terminal state from repository + validation evidence, fabricates no attempt, and
// writes provenance.
func TestTaskCompleteExternal(t *testing.T) {
	dir := t.TempDir()
	head := setupExtProject(t, dir, extCompleteConfig, autoPlanDoc)
	if taskStatus(t, dir, "S001") != domain.PLANNED {
		t.Fatalf("S001 before = %s, want PLANNED", taskStatus(t, dir, "S001"))
	}

	code, out, errOut := runCLIWithAgent(t, dir, nil, "task", "complete", "S001", "--external", "--commit", head)
	if code != exitOK {
		t.Fatalf("complete: code=%d stdout=%s stderr=%s", code, out, errOut)
	}
	if got := taskStatus(t, dir, "S001"); got != domain.LOCAL_DONE {
		t.Errorf("S001 after = %s, want LOCAL_DONE", got)
	}
	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	task, _ := st.Get("S001")
	if task.Attempt != 0 || len(task.Attempts) != 0 {
		t.Errorf("external completion fabricated an attempt: %+v", task)
	}
	artifact := filepath.Join(dir, stateDirName, "runs", "S001", "external-completion.json")
	if _, err := os.Stat(artifact); err != nil {
		t.Errorf("provenance artifact missing: %v", err)
	}
}

func TestTaskCompleteExternalRejects(t *testing.T) {
	cases := []struct{ name, cfg, commit, id string }{
		{"unknown task", extCompleteConfig, "", "NOPE"},
		{"missing implementation evidence", extCompleteConfig, "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef", "S001"},
		{"failed validation", extFailConfig, "", "S001"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			head := setupExtProject(t, dir, tc.cfg, autoPlanDoc)
			commit := tc.commit
			if commit == "" {
				commit = head
			}
			code, _, _ := runCLIWithAgent(t, dir, nil, "task", "complete", tc.id, "--external", "--commit", commit)
			if code == exitOK {
				t.Fatalf("%s: expected a failure, got OK", tc.name)
			}
			if tc.id == "S001" && taskStatus(t, dir, "S001") != domain.PLANNED {
				t.Errorf("%s: status changed on a rejected completion", tc.name)
			}
		})
	}
}

func TestTaskCompleteExternalRejectsAlreadyComplete(t *testing.T) {
	dir := t.TempDir()
	head := setupExtProject(t, dir, extCompleteConfig, autoPlanDoc)
	if code, _, errOut := runCLIWithAgent(t, dir, nil, "task", "complete", "S001", "--external", "--commit", head); code != exitOK {
		t.Fatalf("first complete: stderr=%s", errOut)
	}
	if code, _, _ := runCLIWithAgent(t, dir, nil, "task", "complete", "S001", "--external", "--commit", head); code == exitOK {
		t.Fatal("a second completion must be rejected")
	}
}

func TestTaskCompleteExternalRejectsUnsatisfiedDependency(t *testing.T) {
	dir := t.TempDir()
	head := setupExtProject(t, dir, extCompleteConfig, extPlanTwo)
	// S002 depends on S001, which is still PLANNED.
	if code, _, _ := runCLIWithAgent(t, dir, nil, "task", "complete", "S002", "--external", "--commit", head); code == exitOK {
		t.Fatal("completing a task with an unsatisfied dependency must be rejected")
	}
	if taskStatus(t, dir, "S002") != domain.PLANNED {
		t.Error("S002 must be unchanged after a rejected completion")
	}
}

func TestTaskCompleteExternalUnblocksDependent(t *testing.T) {
	dir := t.TempDir()
	head := setupExtProject(t, dir, extCompleteConfig, extPlanTwo)
	if code, _, errOut := runCLIWithAgent(t, dir, nil, "task", "complete", "S001", "--external", "--commit", head); code != exitOK {
		t.Fatalf("complete S001: stderr=%s", errOut)
	}
	st, err := store.Open(statePath(dir))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	all, _ := st.List()
	byID := map[string]*domain.Task{}
	for _, task := range all {
		byID[task.ID] = task
	}
	if _, ok := byID["S002"].ResolveDependencies(byID); !ok {
		t.Error("S002 must become runnable once S001 is externally complete")
	}
}

func TestTaskCompleteExternalRequiresExternalFlag(t *testing.T) {
	dir := t.TempDir()
	setupExtProject(t, dir, extCompleteConfig, autoPlanDoc)
	if code, _, _ := runCLIWithAgent(t, dir, nil, "task", "complete", "S001"); code != exitUsage {
		t.Fatalf("completion without --external must be a usage error, got %d", code)
	}
}
