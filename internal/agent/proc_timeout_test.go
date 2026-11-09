//go:build unix

package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// agentGrandchildCommand starts an inner shell that starts a grandchild and
// records both pids. The command agent wraps this in its own shell, so the
// outer shell is the direct process, the inner shell is its child, and the
// sleep is its grandchild.
const agentGrandchildCommand = `sh -c 'sleep 30 & echo $! > grandchild.pid; echo $$ > child.pid; wait' & wait`

func waitForAgentPIDFile(t *testing.T, name string) int {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		data, err := os.ReadFile(filepath.Join(".", name))
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

func waitForAgentProcessGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if err := syscall.Kill(pid, 0); err != nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("descendant %d survived the command-agent timeout", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func agentPIDStartTime(pid int) (string, bool) {
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
	if len(fields) <= 19 {
		return "", false
	}
	return fields[19], true
}

func agentKillIfMatching(pid int, start string, readStart func(int) (string, bool), kill func(int) error) bool {
	if pid <= 0 || strings.TrimSpace(start) == "" {
		return false
	}
	current, ok := readStart(pid)
	if !ok || current != start {
		return false
	}
	return kill(pid) == nil
}

func cleanupAgentPID(t *testing.T, pid int) {
	start, ok := agentPIDStartTime(pid)
	if !ok {
		return
	}
	t.Cleanup(func() {
		_ = agentKillIfMatching(pid, start, agentPIDStartTime, func(p int) error {
			return syscall.Kill(p, syscall.SIGKILL)
		})
	})
}

// TestCommandAgentTimeoutKillsDescendant proves the command agent's timeout
// terminates the shell's child and grandchild, not only the direct shell.
func TestCommandAgentTimeoutKillsDescendant(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv(EnvAgentCommand, agentGrandchildCommand)
	t.Setenv(EnvAgentCommandTimeout, "500ms")

	a, err := NewCommandAgentFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Generate(context.Background(), validRequest(Implement)); err == nil {
		t.Fatal("expected a timeout")
	}

	child := waitForAgentPIDFile(t, "child.pid")
	grandchild := waitForAgentPIDFile(t, "grandchild.pid")
	cleanupAgentPID(t, child)
	cleanupAgentPID(t, grandchild)
	waitForAgentProcessGone(t, child)
	waitForAgentProcessGone(t, grandchild)
}
