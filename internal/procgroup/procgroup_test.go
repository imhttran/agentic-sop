//go:build unix

package procgroup

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// TestBindKillsGrandchild cancels a shell that backgrounds a child. The child is a
// grandchild of the context-bound process, so it survives unless the whole group is
// killed; this is the behavior every bound caller depends on.
func TestBindKillsGrandchild(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pid")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", `sleep 30 & echo $! > "$0"; wait`, pidFile)
	Bind(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	pid := waitForPID(t, pidFile)
	cancel()
	_ = cmd.Wait() // the group kill makes Wait return with an error; that is expected

	deadline := time.Now().Add(5 * time.Second)
	for syscall.Kill(pid, 0) != syscall.ESRCH {
		if time.Now().After(deadline) {
			t.Fatalf("grandchild %d still running after the group was killed", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func waitForPID(t *testing.T, path string) int {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if data, err := os.ReadFile(path); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && pid > 0 {
				return pid
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("shell never reported the grandchild pid")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
