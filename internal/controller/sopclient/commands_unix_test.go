//go:build !windows

package sopclient

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// killingSop writes an executable "sop" that starts a long-running grandchild,
// records its PID, and then waits for it, so a test can observe whether stopping
// the `sop` process also stops the work SOP started.
//
// The grandchild's stdout/stderr are detached from the command's pipes: without
// that, os/exec's pipe copiers would keep the surviving grandchild alive inside
// cmd.Wait and hide the orphan behind an unrelated wait.
func killingSop(t *testing.T) (bin, pidFile string) {
	t.Helper()
	dir := t.TempDir()
	bin = filepath.Join(dir, "sop")
	pidFile = filepath.Join(dir, "child.pid")
	sleep, err := exec.LookPath("sleep")
	if err != nil {
		t.Skipf("sleep not available: %v", err)
	}
	script := "#!/bin/sh\n" +
		sleep + " 30 >/dev/null 2>&1 &\n" +
		"echo $! > " + pidFile + "\n" +
		"wait\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, pidFile
}

// A timeout must not outlive the controller's report of it: when Exec gives up,
// every process the SOP command started must be gone too. Otherwise agents, git
// and validation commands keep changing the repo after the controller has already
// told the operator the run timed out.
func TestExecTimeoutKillsGrandchildren(t *testing.T) {
	bin, pidFile := killingSop(t)
	// The timeout must outlast the fake `sop` recording its grandchild, or under
	// load the kill can land before there is a grandchild to observe.
	cmd := NewCommander(bin, 2*time.Second)

	done := make(chan error, 1)
	go func() {
		_, err := cmd.Exec(context.Background(), t.TempDir(), "run")
		done <- err
	}()

	pid := readPID(t, pidFile) // the tree is up before the timeout fires
	select {
	case err := <-done:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err = %v, want errors.Is(err, context.DeadlineExceeded)", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Exec did not return after its timeout")
	}
	assertDead(t, pid)
}

// Explicit cancellation takes the same path as a timeout: the caller's context
// being done must still surface as context.Canceled, and must also stop the work
// SOP had started.
func TestExecCancellationKillsGrandchildren(t *testing.T) {
	bin, pidFile := killingSop(t)
	cmd := NewCommander(bin, time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := cmd.Exec(ctx, t.TempDir(), "run")
		done <- err
	}()

	pid := readPID(t, pidFile) // the tree is up before it is cancelled
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want errors.Is(err, context.Canceled)", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Exec did not return after cancellation")
	}
	assertDead(t, pid)
}

// readPID reads the grandchild PID the fake `sop` recorded before waiting.
func readPID(t *testing.T, pidFile string) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		raw, err := os.ReadFile(pidFile)
		if err == nil {
			pid, convErr := strconv.Atoi(strings.TrimSpace(string(raw)))
			if convErr == nil && pid > 0 {
				return pid
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("grandchild PID was never recorded in %s", pidFile)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// assertDead requires pid to disappear promptly, then and only then reports the
// surviving process as the failure and reaps it so a broken build does not leave a
// sleeper behind.
func assertDead(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for alive(pid) {
		if time.Now().After(deadline) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			t.Fatalf("process %d started by `sop` outlived the end of the command; the controller reported the run stopped while work continued", pid)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// alive reports whether pid still exists. Signal 0 performs the permission and
// existence checks without delivering a signal, so a reaped process reads as dead
// and an orphan that is still running (or still an unreaped zombie) reads as alive.
func alive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}
