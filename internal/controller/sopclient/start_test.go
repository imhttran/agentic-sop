package sopclient

// This file is the MIG-011 regression for the controller's Run action: a
// started-but-incomplete plan must be executed through the supported execution
// command (`sop run`), never delegated to the read-only `sop resume` inspection.
//
// It records the argv the Client actually delegates, so the assertion is about the
// selected command (the one that executes a runnable task), not about a message.

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/controller/config"
)

// fakeRecordingSop writes an executable "sop" that appends its argv to a log file and
// exits 0, so a test can prove which verb the controller delegated.
func fakeRecordingSop(t *testing.T) (bin, logPath string) {
	t.Helper()
	dir := t.TempDir()
	logPath = filepath.Join(dir, "argv.log")
	bin = filepath.Join(dir, "sop")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + logPath + "\nexit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, logPath
}

// seedStartedIncompleteTask inserts one non-terminal, attempted task so the project
// detail is "started but incomplete".
func seedStartedIncompleteTask(t *testing.T, root string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+filepath.Join(root, ".agent-sdlc", "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`
		INSERT INTO tasks (id, title, objective, acceptance_criteria, status, blocked_reason, attempt, max_attempts, created_at, updated_at)
		VALUES ('T001', 't', 'o', 'c', 'BLOCKED', '', 1, 3, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
}

// TestStartOrContinueExecutesAStartedIncompletePlan proves the Run action delegates
// `sop run` (execution) for a started-but-incomplete plan, never the read-only
// `sop resume`.
func TestStartOrContinueExecutesAStartedIncompletePlan(t *testing.T) {
	root := newProject(t)
	seedStartedIncompleteTask(t, root)
	bin, logPath := fakeRecordingSop(t)

	c, err := New([]string{root}, bin, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	res, err := c.StartOrContinue(context.Background(), config.ProjectID(root))
	if err != nil {
		t.Fatalf("StartOrContinue: %v", err)
	}
	if res.Complete {
		t.Fatalf("Complete = true, want false for a started-incomplete plan")
	}
	if res.Verb != "run" {
		t.Errorf("Verb = %q, want \"run\" (execution)", res.Verb)
	}
	got, _ := os.ReadFile(logPath)
	lines := strings.Split(strings.TrimSpace(string(got)), "\n")
	delegated := lines[len(lines)-1]
	if delegated != "run" {
		t.Errorf("last delegated argv = %q, want \"run\": a started-incomplete plan must execute via `sop run`, not `sop resume`", delegated)
	}
	for _, l := range lines {
		if l == "resume" {
			t.Errorf("the Run action delegated `sop resume` (read-only) instead of executing: %q", string(got))
		}
	}
}

// TestStartOrContinueCompletePlanDispatchesNothing proves a complete plan dispatches
// no execution command.
func TestStartOrContinueCompletePlanDispatchesNothing(t *testing.T) {
	root := newProject(t)
	bin, logPath := fakeRecordingSop(t)

	c, err := New([]string{root}, bin, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	db, err := sql.Open("sqlite", "file:"+filepath.Join(root, ".agent-sdlc", "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`
		INSERT INTO tasks (id, title, objective, acceptance_criteria, status, blocked_reason, attempt, max_attempts, created_at, updated_at)
		VALUES ('T001', 't', 'o', 'c', 'LOCAL_DONE', '', 1, 3, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	_ = db.Close()

	res, err := c.StartOrContinue(context.Background(), config.ProjectID(root))
	if err != nil {
		t.Fatalf("StartOrContinue: %v", err)
	}
	if !res.Complete {
		t.Errorf("Complete = false, want true for a terminal plan")
	}
	if got, _ := os.ReadFile(logPath); strings.Contains(string(got), "\nrun\n") || strings.HasSuffix(strings.TrimSpace(string(got)), "\nrun") || strings.TrimSpace(string(got)) == "run" {
		t.Errorf("a complete plan dispatched an execution run, want nothing: %q", string(got))
	}
}
