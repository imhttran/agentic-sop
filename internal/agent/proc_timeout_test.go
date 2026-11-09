//go:build unix

package agent

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

// TestCommandAgentTimeoutKillsDescendant proves the command agent's timeout
// terminates the shell's child too, not only the direct shell process.
func TestCommandAgentTimeoutKillsDescendant(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv(EnvAgentCommand, "sleep 30 & echo $! > child.pid; wait")
	t.Setenv(EnvAgentCommandTimeout, "300ms")

	a, err := NewCommandAgentFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Generate(context.Background(), validRequest(Implement)); err == nil {
		t.Fatal("expected a timeout")
	}

	data, err := os.ReadFile(filepath.Join(".", "child.pid"))
	if err != nil {
		t.Fatalf("read child pid: %v", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		t.Fatalf("child pid = %q", strings.TrimSpace(string(data)))
	}
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })

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
