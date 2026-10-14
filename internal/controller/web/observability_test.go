package web

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// HARD007: deterministic observability tests. They prove that an in-flight
// command is observable without waiting for completion, that a failure is
// surfaced promptly and truthfully, and that the controller never invents a
// lifecycle state. Every interval here is short and bounded, and the only
// sleeps are for goroutine/command synchronization — never a multi-minute wait.

// A long-running command is reported as "running" immediately, before it has
// finished, and only reflects completion once it actually returns.
func TestRunningCommandObservableWithoutWaiting(t *testing.T) {
	r := NewCommandRunner(time.Minute)
	release := make(chan struct{})
	if ok, err := r.Start("proj", "run", func(ctx context.Context) (string, error) {
		<-release
		return "done", nil
	}); !ok || err != nil {
		t.Fatalf("Start: ok=%v err=%v", ok, err)
	}

	// Observe immediately: the command is still in flight and no wait was
	// required to learn that.
	st, ok := r.Status("proj", "run")
	if !ok || !st.Running() || st.State != "running" {
		t.Fatalf("immediately after Start: state=%q ok=%v, want running", st.State, ok)
	}

	close(release)
	if got := waitState(t, r, "proj", "run"); got.State != "done" {
		t.Fatalf("after the command returned: state=%q, want done", got.State)
	}
}

// A command failure is surfaced promptly with the underlying message, and the
// runner records it as an error rather than a success or an invented state.
func TestCommandFailureSurfacesPromptly(t *testing.T) {
	r := NewCommandRunner(time.Minute)
	r.Start("proj", "run", func(ctx context.Context) (string, error) {
		return "", context.DeadlineExceeded
	})
	st := waitState(t, r, "proj", "run")
	if st.State != "error" {
		t.Fatalf("state = %q, want error", st.State)
	}
	if !strings.Contains(st.Error, "deadline exceeded") {
		t.Fatalf("error = %q, want the underlying failure message", st.Error)
	}
}

// slowSopBin writes an executable that ignores its SOP arguments and sleeps for
// about a second, so a command started against it is genuinely in flight long
// enough to observe without waiting for it to finish.
func slowSopBin(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "slow-sop")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nsleep 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

// Starting a genuinely long-running SOP command returns from the starting
// request immediately, and the command is observable as "running" while it is
// still in flight. Completion is then reflected without any arbitrary
// multi-minute wait.
func TestProjectCommandObservedWhileRunning(t *testing.T) {
	srv, id, _ := newTestServerRoot(t, slowSopBin(t))
	client := &http.Client{Jar: mustJar(t)}

	elapsed, body := postCommand(t, client,
		srv.URL+"/projects/"+id, srv.URL+"/projects/"+id+"/commands/validate")
	if elapsed > 3*time.Second {
		t.Fatalf("starting POST took %s; it must return without waiting for the command", elapsed)
	}
	if !strings.Contains(body, "running") {
		t.Fatalf("first response should report the command as running, got: %s", body)
	}

	// Observe it while in flight: poll the status endpoint on a short, bounded
	// cadence (never a fixed multi-minute sleep) and assert it is running.
	deadline := time.Now().Add(6 * time.Second)
	sawRunning := false
	for time.Now().Before(deadline) {
		if _, b := get(t, srv.URL+"/projects/"+id+"/commands/validate"); strings.Contains(b, "running") {
			sawRunning = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !sawRunning {
		t.Fatal("the command was never observable as running while in flight")
	}

	// And completion is reflected as soon as it happens, with no arbitrary wait.
	for time.Now().Before(deadline) {
		if _, b := get(t, srv.URL+"/projects/"+id+"/commands/validate"); !strings.Contains(b, "running") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("the command never left the running state")
}
