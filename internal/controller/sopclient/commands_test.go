package sopclient

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/controller/config"
)

// fakeSop writes an executable "sop" that records its argv, one arg per line, and
// returns it. It lets a test assert exactly which SOP verb and arguments the
// controller drives, without invoking a real SOP.
func fakeSop(t *testing.T) (bin string, args func() []string) {
	t.Helper()
	dir := t.TempDir()
	record := filepath.Join(dir, "args")
	bin = filepath.Join(dir, "sop")
	script := "#!/bin/sh\n: > " + record + "\nfor a in \"$@\"; do printf '%s\\n' \"$a\" >> " + record + "; done\nexit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, func() []string {
		raw, err := os.ReadFile(record)
		if err != nil {
			return nil
		}
		return strings.Fields(string(raw))
	}
}

// fakeListingSop writes an executable "sop" that records its argv and emits the
// given stdout for every invocation, so a test can drive
// `sop reconcile <PLAN.md> --list-changed --json` and assert the decoded listing
// without a real SOP binary.
func fakeListingSop(t *testing.T, stdout string) (bin string, args func() []string) {
	t.Helper()
	dir := t.TempDir()
	record := filepath.Join(dir, "args")
	payload := filepath.Join(dir, "stdout")
	bin = filepath.Join(dir, "sop")
	if err := os.WriteFile(payload, []byte(stdout), 0o644); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\n: > " + record + "\nfor a in \"$@\"; do printf '%s\\n' \"$a\" >> " + record + "; done\ncat " + payload + "\nexit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, func() []string {
		raw, err := os.ReadFile(record)
		if err != nil {
			return nil
		}
		return strings.Fields(string(raw))
	}
}

// fakeFailingSop writes an executable "sop" that always exits non-zero, so a
// failed listing is surfaced as such rather than an empty success.
func fakeFailingSop(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "sop")
	script := "#!/bin/sh\necho 'sop: reconcile --list-changed unsupported' >&2\nexit 1\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

// Recovery operations must drive SOP's own commands, never mutate state directly.
func TestRecoveryCommandsDriveSop(t *testing.T) {
	root := newProject(t)
	now := time.Now().UTC().Format(time.RFC3339)
	seed(t, root, `INSERT INTO tasks VALUES ('t1','T','o','a','BLOCKED','X',3,3,'`+now+`','`+now+`')`)

	bin, args := fakeSop(t)
	c, err := New([]string{root}, bin, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	pid := config.ProjectID(root)

	cases := []struct {
		name string
		run  func() error
		want []string
	}{
		{"retry", func() error { return c.Retry(context.Background(), pid, "t1") }, []string{"retry", "t1"}},
		{"retry force", func() error { return c.RetryForce(context.Background(), pid, "t1") }, []string{"retry", "t1", "--force"}},
		{"retry all", func() error { return c.RetryAll(context.Background(), pid) }, []string{"retry", "--all"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); err != nil {
				t.Fatalf("run: %v", err)
			}
			if got := args(); !equal(got, tc.want) {
				t.Fatalf("sop argv = %v, want %v", got, tc.want)
			}
		})
	}

	out, err := c.Reconcile(context.Background(), pid, "docs/PLAN.md")
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	_ = out
	if got := args(); !equal(got, []string{"reconcile", "docs/PLAN.md"}) {
		t.Fatalf("reconcile argv = %v", got)
	}
}

// Retry must delegate the decision to SOP's CLI and never touch the retry
// budget itself: after Retry/RetryForce run against fakeSop (which performs no
// real mutation), the controller's own read path must still report exactly the
// attempt/max_attempts SOP's fixture recorded, proving the controller path
// writes nothing to state.db on its own.
func TestRetryDelegatesAndPreservesBudget(t *testing.T) {
	root := newProject(t)
	now := time.Now().UTC().Format(time.RFC3339)
	seed(t, root, `INSERT INTO tasks VALUES ('t1','T','o','a','BLOCKED','X',2,3,'`+now+`','`+now+`')`)

	bin, args := fakeSop(t)
	c, err := New([]string{root}, bin, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	pid := config.ProjectID(root)

	before, err := c.Project(context.Background(), pid)
	if err != nil {
		t.Fatal(err)
	}
	var beforeTask TaskSummary
	for _, task := range before.Tasks {
		if task.ID == "t1" {
			beforeTask = task
		}
	}
	if beforeTask.Attempt != 2 || beforeTask.MaxAttempts != 3 {
		t.Fatalf("seed mismatch: %+v", beforeTask)
	}

	if err := c.Retry(context.Background(), pid, "t1"); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if got := args(); !equal(got, []string{"retry", "t1"}) {
		t.Fatalf("retry argv = %v", got)
	}

	after, err := c.Project(context.Background(), pid)
	if err != nil {
		t.Fatal(err)
	}
	var afterTask TaskSummary
	for _, task := range after.Tasks {
		if task.ID == "t1" {
			afterTask = task
		}
	}
	if afterTask.Attempt != beforeTask.Attempt || afterTask.MaxAttempts != beforeTask.MaxAttempts {
		t.Fatalf("controller mutated retry budget: before=%+v after=%+v", beforeTask, afterTask)
	}
	if !afterTask.Retryable() {
		t.Fatalf("task should remain retryable after delegated retry: %+v", afterTask)
	}
}

// SOP_ACTIVITY is set for controller-driven commands so SOP persists the
// activity artifact the dashboard reads; an operator-set value is preserved.
func TestActivityEnvDefault(t *testing.T) {
	got := activityEnv([]string{"PATH=/bin"})
	if !contains(got, "SOP_ACTIVITY=on") {
		t.Fatalf("activityEnv = %v, want SOP_ACTIVITY=on added", got)
	}
	kept := activityEnv([]string{"SOP_ACTIVITY=off"})
	if contains(kept, "SOP_ACTIVITY=on") {
		t.Fatalf("activityEnv overrode an operator value: %v", kept)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
