//go:build unix

package toolharness

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// childCommand starts one descendant and records its pid, then waits.
const childCommand = "sleep 30 & echo $! > child.pid; wait"

// grandchildCommand starts an inner shell that starts a grandchild, records
// both pids, and waits. The outer shell is the direct command; the inner shell
// is its child and the sleep is its grandchild.
const grandchildCommand = `sh -c 'sleep 30 & echo $! > grandchild.pid; echo $$ > child.pid; wait' & wait`

// sigtermIgnoringCommand starts a grandchild shell that ignores SIGTERM. The
// supported contract is a process-group SIGKILL, so the group kill must still
// terminate it without a graceful escalation phase.
const sigtermIgnoringCommand = `sh -c 'sh -c "trap \"\" TERM; while :; do sleep 1; done" & echo $! > grandchild.pid; echo $$ > child.pid; wait' & wait`

// normalGrandchildCommand completes normally after starting and waiting for a
// short-lived grandchild.
const normalGrandchildCommand = `sh -c 'sleep 0.2 & echo $! > grandchild.pid; wait'`

func waitForPIDFile(t *testing.T, root, name string) int {
	t.Helper()
	path := filepath.Join(root, name)
	deadline := time.Now().Add(2 * time.Second)
	for {
		data, err := os.ReadFile(path)
		if err == nil {
			pid, perr := strconv.Atoi(strings.TrimSpace(string(data)))
			if perr == nil && pid > 0 {
				return pid
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s did not appear", name)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func waitForProcessGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if err := syscall.Kill(pid, 0); err != nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("descendant %d survived", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// pidStartTime reads the kernel start time of pid from /proc on Linux. It
// returns false where /proc is unavailable; callers then avoid signaling by
// bare pid and rely on the harness group kill.
func pidStartTime(pid int) (string, bool) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return "", false
	}
	s := string(data)
	end := strings.LastIndexByte(s, ')')
	if end < 0 {
		return "", false
	}
	fields := strings.Fields(s[end+1:])
	// Field 3 is state; starttime is field 22 overall, index 19 here.
	if len(fields) <= 19 {
		return "", false
	}
	return fields[19], true
}

// killIfMatching signals pid only when its current start time still matches the
// identity observed earlier, so a reused pid is never signaled.
func killIfMatching(pid int, start string, readStart func(int) (string, bool), kill func(int) error) bool {
	if pid <= 0 || strings.TrimSpace(start) == "" {
		return false
	}
	current, ok := readStart(pid)
	if !ok || current != start {
		return false
	}
	return kill(pid) == nil
}

// cleanupOwnedPID registers a cleanup that signals the observed process only if
// the pid still names the same process. If the start time is unavailable, it
// registers no signal and relies on the harness group kill.
func cleanupOwnedPID(t *testing.T, pid int) {
	start, ok := pidStartTime(pid)
	if !ok {
		return
	}
	t.Cleanup(func() {
		_ = killIfMatching(pid, start, pidStartTime, func(p int) error {
			return syscall.Kill(p, syscall.SIGKILL)
		})
	})
}

// TestExecTimeoutKillsProcessGroup proves the configured timeout terminates a
// descendant the command started, not only the direct shell.
func TestExecTimeoutKillsProcessGroup(t *testing.T) {
	root := t.TempDir()
	h := New(root, Config{CommandTimeout: 300 * time.Millisecond, MaxOutputBytes: 4096}, nil)
	_, err := h.exec(context.Background(), []string{"sh", "-c", childCommand})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("exec err = %v, want a timeout", err)
	}
	pid := waitForPIDFile(t, root, "child.pid")
	cleanupOwnedPID(t, pid)
	waitForProcessGone(t, pid)
}

// TestExecCancellationKillsProcessGroup proves context cancellation terminates
// the same descendant set, so the CLI signal context can stop owned work.
func TestExecCancellationKillsProcessGroup(t *testing.T) {
	root := t.TempDir()
	h := New(root, Config{CommandTimeout: 30 * time.Second, MaxOutputBytes: 4096}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_, _ = h.exec(ctx, []string{"sh", "-c", childCommand})
		close(done)
	}()

	pid := waitForPIDFile(t, root, "child.pid")
	cleanupOwnedPID(t, pid)
	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("exec did not return after cancellation")
	}
	waitForProcessGone(t, pid)
}

// TestExecNormalCompletionNoDescendants proves a normal command that starts and
// waits for a grandchild completes without leaving either process behind.
func TestExecNormalCompletionNoDescendants(t *testing.T) {
	root := t.TempDir()
	h := New(root, Config{CommandTimeout: 5 * time.Second, MaxOutputBytes: 4096}, nil)
	if _, err := h.exec(context.Background(), []string{"sh", "-c", normalGrandchildCommand}); err != nil {
		t.Fatalf("exec: %v", err)
	}
	pid := waitForPIDFile(t, root, "grandchild.pid")
	cleanupOwnedPID(t, pid)
	waitForProcessGone(t, pid)
}

// TestExecTimeoutKillsGrandchild proves the timeout group kill reaches a child
// that started a grandchild, not only the direct process.
func TestExecTimeoutKillsGrandchild(t *testing.T) {
	root := t.TempDir()
	h := New(root, Config{CommandTimeout: 500 * time.Millisecond, MaxOutputBytes: 4096}, nil)
	_, err := h.exec(context.Background(), []string{"sh", "-c", grandchildCommand})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("exec err = %v, want a timeout", err)
	}
	child := waitForPIDFile(t, root, "child.pid")
	grandchild := waitForPIDFile(t, root, "grandchild.pid")
	cleanupOwnedPID(t, child)
	cleanupOwnedPID(t, grandchild)
	waitForProcessGone(t, child)
	waitForProcessGone(t, grandchild)
}

// TestExecCancellationKillsGrandchild proves explicit cancellation terminates
// both the child and the grandchild.
func TestExecCancellationKillsGrandchild(t *testing.T) {
	root := t.TempDir()
	h := New(root, Config{CommandTimeout: 30 * time.Second, MaxOutputBytes: 4096}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_, _ = h.exec(ctx, []string{"sh", "-c", grandchildCommand})
		close(done)
	}()

	child := waitForPIDFile(t, root, "child.pid")
	grandchild := waitForPIDFile(t, root, "grandchild.pid")
	cleanupOwnedPID(t, child)
	cleanupOwnedPID(t, grandchild)
	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("exec did not return after cancellation")
	}
	waitForProcessGone(t, child)
	waitForProcessGone(t, grandchild)
}

// TestExecTimeoutKillsSigtermIgnoringDescendant proves the existing group
// SIGKILL terminates a descendant that ignores SIGTERM. There is no graceful
// escalation phase in the contract; SIGKILL is the contract.
func TestExecTimeoutKillsSigtermIgnoringDescendant(t *testing.T) {
	root := t.TempDir()
	h := New(root, Config{CommandTimeout: 500 * time.Millisecond, MaxOutputBytes: 4096}, nil)
	_, err := h.exec(context.Background(), []string{"sh", "-c", sigtermIgnoringCommand})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("exec err = %v, want a timeout", err)
	}
	child := waitForPIDFile(t, root, "child.pid")
	grandchild := waitForPIDFile(t, root, "grandchild.pid")
	cleanupOwnedPID(t, child)
	cleanupOwnedPID(t, grandchild)
	waitForProcessGone(t, child)
	waitForProcessGone(t, grandchild)
}

// TestCleanupOwnedPIDDoesNotSignalDifferentIdentity proves the cleanup helper
// refuses a mismatched identity and signals only the matching process.
func TestCleanupOwnedPIDDoesNotSignalDifferentIdentity(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := cmd.Process.Pid
	start, ok := pidStartTime(pid)
	if !ok {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Skip("process start time is unavailable")
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	kill := func(p int) error { return syscall.Kill(p, syscall.SIGKILL) }
	if killIfMatching(pid, "different-"+start, pidStartTime, kill) {
		t.Fatal("killIfMatching signaled a different process identity")
	}
	if err := syscall.Kill(pid, 0); err != nil {
		t.Fatalf("process was signaled despite the identity mismatch: %v", err)
	}
	if !killIfMatching(pid, start, pidStartTime, kill) {
		t.Fatal("killIfMatching did not signal the matching process")
	}
	_ = cmd.Wait()
}
