package sopclient

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/controller/config"
)

// writePlan records SOP's active plan provenance so AcceptChangedTasks can
// resolve the plan path from SOP's recorded plan.meta.json.
func writePlan(t *testing.T, root, src string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, ".agent-sdlc", "plan.meta.json"),
		[]byte(`{"source":"`+src+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}
}

// hasAcceptChanged reports whether a recorded SOP invocation was a mutation
// (`--accept-changed`), as opposed to the pure `--list-changed` read.
func hasAcceptChanged(args []string) bool {
	for _, a := range args {
		if a == "--accept-changed" {
			return true
		}
	}
	return false
}

// fakeSuccessSop records argv and always exits 0, so a test can assert the exact
// invocation a delegated accept-changed sends to SOP.
func fakeSuccessSop(t *testing.T) (bin string, args func() []string) {
	t.Helper()
	dir := t.TempDir()
	record := filepath.Join(dir, "args")
	bin = filepath.Join(dir, "sop")
	listing := listingDocJSON("t1", "t2", "t3")
	payload := filepath.Join(dir, "stdout")
	if err := os.WriteFile(payload, []byte(listing), 0o644); err != nil {
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

// Accepting N explicitly selected ids issues exactly ONE SOP invocation with the
// resolved plan path and repeated `--accept-changed <id>`, and no other ids.
func TestAcceptChangedTasksSingleInvocationExplicitIds(t *testing.T) {
	root := newProject(t)
	writePlan(t, root, "docs/PLAN.md")
	bin, args := fakeSuccessSop(t)
	c, err := New([]string{root}, bin, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	pid := config.ProjectID(root)

	if err := c.AcceptChangedTasks(context.Background(), pid, []string{"t1", "t3"}); err != nil {
		t.Fatalf("AcceptChangedTasks: %v", err)
	}
	// The fake records argv for the LAST invocation only; assert it is a single,
	// correct reconcile invocation carrying exactly the two selected ids.
	got := args()
	want := []string{"reconcile", "docs/PLAN.md", "--accept-changed", "t1", "--accept-changed", "t3"}
	if !equal(got, want) {
		t.Fatalf("sop argv = %v, want %v", got, want)
	}
	if strings.Contains(strings.Join(got, " "), "t2") {
		t.Errorf("invocation must not carry an id the caller did not select: %v", got)
	}
}

// The one-id convenience delegates to the batch form and issues exactly one
// `--accept-changed <id>` pair.
func TestAcceptChangedTaskSingleIDArgv(t *testing.T) {
	root := newProject(t)
	writePlan(t, root, "docs/PLAN.md")
	bin, args := fakeSuccessSop(t)
	c, err := New([]string{root}, bin, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	pid := config.ProjectID(root)

	if err := c.AcceptChangedTask(context.Background(), pid, "t2"); err != nil {
		t.Fatalf("AcceptChangedTask: %v", err)
	}
	want := []string{"reconcile", "docs/PLAN.md", "--accept-changed", "t2"}
	if got := args(); !equal(got, want) {
		t.Fatalf("sop argv = %v, want %v", got, want)
	}
}

// Empty/duplicate selections are normalized so each id appears exactly once; an
// all-empty selection is rejected with ErrNoExplicitTaskIDs and makes no mutation.
func TestAcceptChangedTasksNormalizesIDs(t *testing.T) {
	root := newProject(t)
	writePlan(t, root, "docs/PLAN.md")
	bin, args := fakeSuccessSop(t)
	c, err := New([]string{root}, bin, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	pid := config.ProjectID(root)
	ctx := context.Background()

	if err := c.AcceptChangedTasks(ctx, pid, []string{" t1 ", "t1", "", "t1"}); err != nil {
		t.Fatalf("AcceptChangedTasks: %v", err)
	}
	want := []string{"reconcile", "docs/PLAN.md", "--accept-changed", "t1"}
	if got := args(); !equal(got, want) {
		t.Fatalf("sop argv = %v, want %v", got, want)
	}

	if err := c.AcceptChangedTasks(ctx, pid, []string{"", "  "}); !errors.Is(err, ErrNoExplicitTaskIDs) {
		t.Errorf("AcceptChangedTasks(empty selection) err = %v, want ErrNoExplicitTaskIDs", err)
	}
	// The empty-selection rejection must make no mutation.
	if hasAcceptChanged(args()) {
		t.Errorf("empty selection issued a mutation: %v", args())
	}
}

// A task not in SOP's reported changed set is rejected with ErrTaskNotInChangedSet
// BEFORE any accept-changed mutation; likewise unknown-project, no-active-plan,
// and an unreported/failed listing make no mutation at all.
func TestAcceptChangedTasksRejectsBeforeAnyCall(t *testing.T) {
	ctx := context.Background()

	t.Run("unknown project", func(t *testing.T) {
		root := newProject(t)
		bin, args := fakeSuccessSop(t)
		c, _ := New([]string{root}, bin, time.Minute)
		defer c.Close()
		if err := c.AcceptChangedTask(ctx, "nope", "t1"); !errors.Is(err, ErrProjectNotFound) {
			t.Errorf("err = %v, want ErrProjectNotFound", err)
		}
		if got := args(); got != nil {
			t.Errorf("SOP was invoked for an unknown project: %v", got)
		}
	})

	t.Run("no active plan", func(t *testing.T) {
		root := newProject(t)
		bin, args := fakeSuccessSop(t)
		c, _ := New([]string{root}, bin, time.Minute)
		defer c.Close()
		if err := c.AcceptChangedTask(ctx, config.ProjectID(root), "t1"); !errors.Is(err, ErrNoActivePlan) {
			t.Errorf("err = %v, want ErrNoActivePlan", err)
		}
		if got := args(); got != nil {
			t.Errorf("SOP was invoked with no active plan: %v", got)
		}
	})

	t.Run("task not in changed set", func(t *testing.T) {
		root := newProject(t)
		writePlan(t, root, "docs/PLAN.md")
		bin, args := fakeSuccessSop(t)
		c, _ := New([]string{root}, bin, time.Minute)
		defer c.Close()
		if err := c.AcceptChangedTask(ctx, config.ProjectID(root), "unrelated"); !errors.Is(err, ErrTaskNotInChangedSet) {
			t.Errorf("err = %v, want ErrTaskNotInChangedSet", err)
		}
		// The listing read (a pure `--list-changed` read) may run, but NO
		// accept-changed mutation may be issued for an unlisted task.
		if hasAcceptChanged(args()) {
			t.Errorf("SOP was invoked to accept an unlisted task: %v", args())
		}
	})

	t.Run("unreported listing", func(t *testing.T) {
		root := newProject(t)
		writePlan(t, root, "docs/PLAN.md")
		bin := fakeFailingSop(t)
		c, _ := New([]string{root}, bin, time.Minute)
		defer c.Close()
		if err := c.AcceptChangedTask(ctx, config.ProjectID(root), "t1"); err == nil {
			t.Errorf("err = nil, want a surfaced listing failure")
		}
	})
}

// A non-zero SOP result is reported truthfully as a *ReconcileRejection carrying
// SOP's own message verbatim and matching ErrReconcileRejected; it is never a
// fabricated success, and it is distinct from the precondition sentinels.
func TestAcceptChangedTasksReportsFailedReconcileTruthfully(t *testing.T) {
	root := newProject(t)
	writePlan(t, root, "docs/PLAN.md")
	dir := t.TempDir()
	bin := filepath.Join(dir, "sop")
	// Emit a valid listing for `--list-changed`, then fail for the mutation with a
	// message so the classification carries SOP's own output verbatim.
	listing := filepath.Join(dir, "listing")
	if err := os.WriteFile(listing, []byte(listingDocJSON("t1")), 0o644); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\ncase \"$*\" in\n  *--list-changed*) cat " + listing + "; exit 0;;\nesac\necho 'sop: unknown flag --accept-changed' >&2\nexit 2\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	c, err := New([]string{root}, bin, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	pid := config.ProjectID(root)

	err = c.AcceptChangedTask(context.Background(), pid, "t1")
	if err == nil {
		t.Fatal("err = nil, want a truthful failure")
	}
	if !errors.Is(err, ErrReconcileRejected) {
		t.Errorf("err = %v, want ErrReconcileRejected", err)
	}
	var rej *ReconcileRejection
	if !errors.As(err, &rej) {
		t.Fatalf("err = %v, want *ReconcileRejection", err)
	}
	if !strings.Contains(rej.Message, "unknown flag --accept-changed") {
		t.Errorf("rejection message = %q, want SOP's own output verbatim", rej.Message)
	}
	if len(rej.TaskIDs) != 1 || rej.TaskIDs[0] != "t1" {
		t.Errorf("rejection TaskIDs = %v, want [t1]", rej.TaskIDs)
	}
	// A reconcile rejection is not a precondition sentinel.
	if errors.Is(err, ErrTaskNotInChangedSet) {
		t.Errorf("reconcile rejection misclassified: %v", err)
	}
}
