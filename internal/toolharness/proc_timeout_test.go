//go:build unix

package toolharness

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// childCommand starts a descendant and records its pid, then waits. It is used
// to prove the harness stops owned work, not only the direct shell.
const childCommand = "sleep 30 & echo $! > child.pid; wait"

func waitForPIDFile(t *testing.T, root string) int {
	t.Helper()
	path := filepath.Join(root, "child.pid")
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
			t.Fatalf("child pid file did not appear")
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

// TestExecTimeoutKillsProcessGroup proves the configured timeout terminates a
// descendant the command started, not only the direct shell.
func TestExecTimeoutKillsProcessGroup(t *testing.T) {
	root := t.TempDir()
	h := New(root, Config{CommandTimeout: 300 * time.Millisecond, MaxOutputBytes: 4096}, nil)
	_, err := h.exec(context.Background(), []string{"sh", "-c", childCommand})
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("exec err = %v, want a timeout", err)
	}
	pid := waitForPIDFile(t, root)
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
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

	pid := waitForPIDFile(t, root)
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("exec did not return after cancellation")
	}
	waitForProcessGone(t, pid)
}
