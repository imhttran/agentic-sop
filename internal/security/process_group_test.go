//go:build unix

package security

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// sigtermIgnoringDescendantCommand starts a descendant shell that ignores
// SIGTERM, records its pid, and loops in the resolved sleep path. The direct
// command is the outer shell; the descendant ignores SIGTERM, so only a
// process-group SIGKILL reaches it.
func sigtermIgnoringDescendantCommand(sh, sleep string) string {
	inner := fmt.Sprintf(`trap "" TERM; while :; do %s 1; done`, shellQuote(sleep))
	return fmt.Sprintf(`%s -c %s & echo $! > descendant.pid; wait`, shellQuote(sh), shellQuote(inner))
}

// shellQuote single-quotes s for a POSIX shell.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func lookPathOrSkip(t *testing.T, name string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("process-group descendant test requires a POSIX shell")
	}
	path, err := exec.LookPath(name)
	if err != nil {
		t.Skipf("%s not found in PATH: %v", name, err)
	}
	return path
}

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

// cleanupOwnedPID signals the observed process only if the pid still names the
// same process, so a reused pid is never signaled. If the start time is
// unavailable it registers no signal and relies on the group kill.
func cleanupOwnedPID(t *testing.T, pid int) {
	t.Helper()
	if pid <= 0 {
		return
	}
	start, ok := pidStartTime(pid)
	if !ok {
		return
	}
	t.Cleanup(func() {
		current, ok := pidStartTime(pid)
		if !ok || current != start {
			return
		}
		_ = syscall.Kill(pid, syscall.SIGKILL)
	})
}

// TestProcessGroupKillsSigtermIgnoringDescendant proves execRunner.Run
// terminates a descendant the command started, not only the direct shell. The
// descendant ignores SIGTERM, so only a process-group kill reaches it.
func TestProcessGroupKillsSigtermIgnoringDescendant(t *testing.T) {
	sh := lookPathOrSkip(t, "sh")
	sleep := lookPathOrSkip(t, "sleep")

	root := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	script := sigtermIgnoringDescendantCommand(sh, sleep)
	_, _, _, err := execRunner{}.Run(ctx, root, []string{sh, "-c", script}, nil, 4096)
	if err == nil {
		t.Fatalf("exec err = nil, want a deadline error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("exec err = %v, want %v", err, context.DeadlineExceeded)
	}

	pid := waitForPIDFile(t, root, "descendant.pid")
	cleanupOwnedPID(t, pid)
	waitForProcessGone(t, pid)
}
