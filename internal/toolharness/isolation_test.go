package toolharness

import (
	"context"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func needSeatbelt(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Skip("Seatbelt backend is macOS only")
	}
	if _, err := exec.LookPath("sandbox-exec"); err != nil {
		t.Skip("sandbox-exec not available")
	}
	if exec.Command("sandbox-exec", "-p", "(version 1)(allow default)", "true").Run() != nil {
		t.Skip("sandbox-exec cannot nest (already inside a sandbox)")
	}
}

// sandboxed runs a shell snippet inside the run_command sandbox for root.
func sandboxed(t *testing.T, root, script string) error {
	t.Helper()
	argv, _, err := isolate(root, []string{"sh", "-c", script})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = root
	return cmd.Run()
}

func TestRunCommandIsolationOffKeepsExecCapturedPath(t *testing.T) {
	t.Setenv(IsolationEnv, "")
	log := NewAuditLog(0)
	h := New(t.TempDir(), DefaultConfig(), log)
	if _, err := h.Run(context.Background(), ToolRunCommand, map[string]any{"command": "go list ./..."}); err != nil {
		t.Fatal(err)
	}
	if d := log.Records()[0].Detail; d != "" {
		t.Fatalf("isolation off: audit detail = %q, want empty", d)
	}
}

func TestRunCommandIsolationOnFailsClosedWithoutBackend(t *testing.T) {
	t.Setenv(IsolationEnv, "1")
	t.Setenv("PATH", t.TempDir()) // no sandbox-exec on PATH
	log := NewAuditLog(0)
	h := New(t.TempDir(), DefaultConfig(), log)
	_, err := h.Run(context.Background(), ToolRunCommand, map[string]any{"command": "go list ./..."})
	if !errors.Is(err, errIsolationUnavailable) {
		t.Fatalf("err = %v, want errIsolationUnavailable", err)
	}
	if r := log.Records()[0]; r.Action != ActionDeny {
		t.Fatalf("audit action = %s, want deny", r.Action)
	}
}

func TestRunCommandIsolationOnWrapsArgvWithSandboxExec(t *testing.T) {
	needSeatbelt(t)
	log := NewAuditLog(0)
	cfg := DefaultConfig()
	cfg.IsolateCommands = true
	h := New(t.TempDir(), cfg, log)
	out, err := h.Run(context.Background(), ToolRunCommand, map[string]any{"command": "go list ./..."})
	if err != nil || !strings.Contains(out, "exit") {
		t.Fatalf("isolated go version: %q, %v", out, err)
	}
	if d := log.Records()[0].Detail; !strings.HasPrefix(d, "isolate=seatbelt backend=") || !strings.Contains(d, "profile=") {
		t.Fatalf("audit detail = %q, want the seatbelt backend and profile digest", d)
	}
}

func TestRunCommandIsolationDeniesStateDirWrite(t *testing.T) {
	needSeatbelt(t)
	root := t.TempDir()
	writeRepoFile(t, root, ".agent-sdlc/state.db", "state")
	if err := sandboxed(t, root, "echo x > ok.txt"); err != nil {
		t.Fatalf("root write was refused: %v", err)
	}
	if err := sandboxed(t, root, "echo forged > .agent-sdlc/state.db"); err == nil {
		t.Fatal("a subprocess write to .agent-sdlc succeeded")
	}
	if b, _ := os.ReadFile(filepath.Join(root, ".agent-sdlc/state.db")); string(b) != "state" {
		t.Fatalf("state.db changed: %q", b)
	}
	home, _ := os.UserHomeDir()
	if err := sandboxed(t, root, "touch "+filepath.Join(home, ".sop-isolation-probe")); err == nil {
		os.Remove(filepath.Join(home, ".sop-isolation-probe"))
		t.Fatal("a write outside the writable dirs succeeded")
	}
}

func TestRunCommandIsolationDeniesNetwork(t *testing.T) {
	needSeatbelt(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	root := t.TempDir()
	// Loopback stays allowed (go test servers); a public address does not.
	if err := sandboxed(t, root, "nc -z -G 2 127.0.0.1 "+strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)); err != nil {
		t.Fatalf("loopback connect refused: %v", err)
	}
	if err := sandboxed(t, root, "nc -z -G 2 1.1.1.1 443"); err == nil {
		t.Fatal("an outbound connection succeeded")
	}
}
