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

// needSandbox skips unless this host can actually start the platform sandbox
// (sandbox-exec cannot nest; bwrap needs unprivileged user namespaces).
func needSandbox(t *testing.T) {
	t.Helper()
	probe := map[string][]string{
		"darwin": {"sandbox-exec", "-p", "(version 1)(allow default)", "true"},
		"linux":  {"bwrap", "--ro-bind", "/", "/", "--unshare-net", "true"},
	}[runtime.GOOS]
	if probe == nil {
		t.Skipf("no sandbox backend on %s", runtime.GOOS)
	}
	if err := exec.Command(probe[0], probe[1:]...).Run(); err != nil {
		t.Skipf("%s cannot start here: %v", probe[0], err)
	}
}

// ncTimeout is the netcat connect-timeout flag (BSD nc on macOS, OpenBSD nc on Linux).
func ncTimeout() string {
	if runtime.GOOS == "darwin" {
		return "-G 2"
	}
	return "-w 2"
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
	needSandbox(t)
	log := NewAuditLog(0)
	cfg := DefaultConfig()
	cfg.IsolateCommands = true
	h := New(t.TempDir(), cfg, log)
	out, err := h.Run(context.Background(), ToolRunCommand, map[string]any{"command": "go list ./..."})
	if err != nil || !strings.Contains(out, "exit") {
		t.Fatalf("isolated go version: %q, %v", out, err)
	}
	if d := log.Records()[0].Detail; (!strings.HasPrefix(d, "isolate=seatbelt backend=") && !strings.HasPrefix(d, "isolate=bubblewrap backend=")) || !strings.Contains(d, "profile=") {
		t.Fatalf("audit detail = %q, want the seatbelt backend and profile digest", d)
	}
}

func TestRunCommandIsolationDeniesStateDirWrite(t *testing.T) {
	needSandbox(t)
	root := t.TempDir()
	writeRepoFile(t, root, ".agent-sdlc/state.db", "state")
	if err := sandboxed(t, root, "echo x > ok.txt"); err != nil {
		t.Fatalf("root write was refused: %v", err)
	}
	// The write must land in the real root, not in a sandbox-only copy.
	if b, err := os.ReadFile(filepath.Join(root, "ok.txt")); err != nil || string(b) != "x\n" {
		t.Fatalf("root write did not reach the host: %q, %v", b, err)
	}
	if err := sandboxed(t, root, "echo forged > .agent-sdlc/state.db"); err == nil {
		t.Fatal("a subprocess write to .agent-sdlc succeeded")
	}
	if b, _ := os.ReadFile(filepath.Join(root, ".agent-sdlc/state.db")); string(b) != "state" {
		t.Fatalf("state.db changed: %q", b)
	}
	// Replacing the state dir itself (move it aside, plant a forged one) fails.
	if err := sandboxed(t, root, "mv .agent-sdlc moved && mkdir .agent-sdlc && echo forged > .agent-sdlc/state.db"); err == nil {
		t.Fatal("the state dir was replaced from inside the sandbox")
	}
	if b, _ := os.ReadFile(filepath.Join(root, ".agent-sdlc/state.db")); string(b) != "state" {
		t.Fatalf("state.db after replacement attempt: %q", b)
	}
	// A symlinked state dir is denied at its real path too.
	linked := t.TempDir()
	writeRepoFile(t, linked, "real-state/state.db", "state")
	if err := os.Symlink("real-state", filepath.Join(linked, ".agent-sdlc")); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "linux" {
		// bubblewrap refuses a symlinked state dir outright.
		if _, _, err := isolate(linked, []string{"true"}); !errors.Is(err, errIsolationUnavailable) {
			t.Fatalf("symlinked state dir: err = %v, want errIsolationUnavailable", err)
		}
	} else if err := sandboxed(t, linked, "echo forged > real-state/state.db"); err == nil {
		t.Fatal("a write to the symlinked state dir target succeeded")
	}
	// A cache dir that does not exist yet stays writable.
	fresh := filepath.Join(t.TempDir(), "new", "gocache")
	t.Setenv("GOCACHE", fresh)
	found := false
	for _, d := range writableDirs(root) {
		found = found || strings.HasSuffix(d, filepath.Join("new", "gocache"))
	}
	if !found {
		t.Fatalf("fresh GOCACHE %s dropped from writable dirs", fresh)
	}
	// A cache path through a planted symlink is dropped, not followed.
	planted := filepath.Join(t.TempDir(), "planted")
	if err := os.Symlink(t.TempDir(), planted); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOCACHE", filepath.Join(planted, "cache"))
	for _, d := range writableDirs(root) {
		if strings.HasSuffix(d, "cache") && !strings.Contains(d, "go-build") {
			t.Fatalf("cache path through a symlink kept as %s", d)
		}
	}
	home, _ := os.UserHomeDir()
	if err := sandboxed(t, root, "touch "+filepath.Join(home, ".sop-isolation-probe")); err == nil {
		os.Remove(filepath.Join(home, ".sop-isolation-probe"))
		t.Fatal("a write outside the writable dirs succeeded")
	}
}

func TestRunCommandIsolationDeniesNetwork(t *testing.T) {
	needSandbox(t)
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
	// On Linux the sandbox has its own network namespace, so the host's
	// loopback listener is unreachable by design and only macOS checks it.
	if runtime.GOOS == "darwin" {
		if err := sandboxed(t, root, "nc -z "+ncTimeout()+" 127.0.0.1 "+strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)); err != nil {
			t.Fatalf("loopback connect refused: %v", err)
		}
	}
	if err := sandboxed(t, root, "nc -z "+ncTimeout()+" 1.1.1.1 443"); err == nil {
		t.Fatal("an outbound connection succeeded")
	}
	// A host Unix socket (e.g. a Docker or agent socket) is not reachable.
	sock := filepath.Join(os.TempDir(), "sop-iso-"+strconv.Itoa(os.Getpid())+".sock")
	ul, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ul.Close()
	if err := sandboxed(t, root, "nc -z -U "+sock); err == nil {
		t.Fatal("a Unix-socket connection succeeded")
	}
}

// The bubblewrap view: host read-only first, private /tmp and /run, the state
// dir mounted read-only after the writable root bind (so it wins), and a
// private network namespace. Deterministic, so it runs on every platform.
func TestBubblewrapArgsOrder(t *testing.T) {
	root := t.TempDir()
	writeRepoFile(t, root, ".agent-sdlc/state.db", "state")
	canon, _ := filepath.EvalSymlinks(root)
	got := strings.Join(bubblewrapArgs(canon, []string{canon, "/tmp/x", "/run/user/1/cache"}), " ")
	state, _ := filepath.EvalSymlinks(filepath.Join(canon, ".agent-sdlc"))
	order := []string{"--ro-bind / /", "--tmpfs /tmp", "--tmpfs /run", "--bind-try " + canon + " " + canon, "--ro-bind " + state + " " + state, "--unshare-net", "--chdir " + canon, "--"}
	at := 0
	for _, want := range order {
		i := strings.Index(got[at:], want)
		if i < 0 {
			t.Fatalf("missing or out of order %q in:\n%s", want, got)
		}
		at += i + len(want)
	}
	if strings.Contains(got, "/tmp/x") || strings.Contains(got, "/run/user") {
		t.Fatalf("a /tmp or /run path must not be bound over the private tmpfs:\n%s", got)
	}
	// A symlinked state dir is refused: the writable root would let the
	// command replace the link.
	linked := t.TempDir()
	writeRepoFile(t, linked, "real/state.db", "state")
	if err := os.Symlink("real", filepath.Join(linked, ".agent-sdlc")); err != nil {
		t.Fatal(err)
	}
	if err := refuseSymlinkedState(linked); !errors.Is(err, errIsolationUnavailable) {
		t.Fatalf("symlinked state dir: err = %v, want errIsolationUnavailable", err)
	}
	if err := refuseSymlinkedState(root); err != nil {
		t.Fatalf("real state dir refused: %v", err)
	}
	missing := strings.Join(bubblewrapArgs(t.TempDir(), nil), " ")
	if !strings.Contains(missing, "--remount-ro") {
		t.Fatalf("a missing state dir must get a read-only tmpfs:\n%s", missing)
	}
}
