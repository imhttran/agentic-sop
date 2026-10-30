package dqharness

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// requireBubblewrapRuntime skips the test unless the detected backend is the Linux
// bubblewrap sandbox. It reuses requireIsolation, so SOP_REPLAY_ISOLATION_REQUIRED=1
// turns a missing backend into a hard failure rather than a skip.
func requireBubblewrapRuntime(t *testing.T) {
	t.Helper()
	requireIsolation(t)
	if iso := DetectIsolation(); iso.Backend != BackendBubblewrap {
		t.Skipf("unix-socket boundary test requires the Linux bubblewrap backend, got %q", iso.Backend)
	}
}

// unixSocketClient is the Python client. It connects to the socket path passed as the
// first argument, sends one byte, and prints CONNECTED on success. On failure it prints
// DENIED:<error-type> and exits non-zero.
const unixSocketClient = `import socket, sys
path = sys.argv[1]
s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
s.settimeout(2)
try:
    s.connect(path)
except OSError as e:
    print("DENIED:" + type(e).__name__)
    sys.exit(1)
s.sendall(b"x")
print("CONNECTED")
s.close()
`

// awaitConnection waits up to d for a connection on the channel and reports whether one
// arrived.
func awaitConnection(ch <-chan struct{}, d time.Duration) bool {
	select {
	case <-ch:
		return true
	case <-time.After(d):
		return false
	}
}

// TestIsolatedReplayUnixSocket proves a host Unix-domain socket outside the replay
// directory is unreachable from inside the bubblewrap sandbox. The same client succeeds
// outside isolation and is denied inside it.
func TestIsolatedReplayUnixSocket(t *testing.T) {
	requireBubblewrapRuntime(t)

	if _, err := exec.LookPath("python3"); err != nil {
		t.Fatalf("python3 client is required but missing: %v", err)
	}

	// The socket lives in a host temp directory outside the replay directory, so the
	// restricted view hides it.
	sockDir, err := os.MkdirTemp("", "unixsock-")
	if err != nil {
		t.Fatalf("temp dir for listener: %v", err)
	}
	defer os.RemoveAll(sockDir)
	socketPath := filepath.Join(sockDir, "probe.sock")

	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		t.Fatalf("listen unix socket: %v", err)
	}
	defer ln.Close()

	accepted := make(chan struct{}, 16)
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
			accepted <- struct{}{}
		}
	}()

	// Step 3: run the client outside isolation and require success plus one connection.
	outside := exec.Command("python3", "-c", unixSocketClient, socketPath)
	out, err := outside.CombinedOutput()
	if err != nil {
		t.Fatalf("outside client failed: err=%v out=%q", err, out)
	}
	if !strings.Contains(string(out), "CONNECTED") {
		t.Fatalf("outside client did not connect: out=%q", out)
	}
	if !awaitConnection(accepted, 2*time.Second) {
		t.Fatal("listener did not receive the outside connection")
	}
	// Drain any duplicate from the outside run.
	if awaitConnection(accepted, 100*time.Millisecond) {
		t.Fatal("outside run produced more than one connection")
	}

	// Step 4: run the same client through the sandbox. Require a non-timeout, non-startup
	// failure that carries the DENIED marker.
	f := isolationFixture("python3", "-c", unixSocketClient, socketPath)
	code, isoOut, isoErr := IsolatedReplay(context.Background(), t.TempDir(), f, 10*time.Second)
	if isoErr != nil {
		t.Fatalf("sandbox startup failure (not a denial): %v", isoErr)
	}
	if strings.Contains(isoOut, "timed out") {
		t.Fatalf("isolated run timed out instead of being denied: %q", isoOut)
	}
	if !strings.Contains(isoOut, "DENIED:") {
		t.Fatalf("isolated run lacks the DENIED marker: code=%d out=%q", code, isoOut)
	}
	if code == 0 {
		t.Fatalf("isolated client reported success: out=%q", isoOut)
	}

	// Step 5: no further connection arrives after the isolated run.
	if awaitConnection(accepted, 500*time.Millisecond) {
		t.Fatalf("the isolated client reached the host Unix socket: out=%q", fmt.Sprintf("%q", isoOut))
	}
}

// TestReplayIsolationRequiredEnv is the deterministic guard that SOP acceptance
// commands inherit the required-isolation environment. It skips when the variable is
// unset, and passes when it is 1.
func TestReplayIsolationRequiredEnv(t *testing.T) {
	v := strings.TrimSpace(os.Getenv("SOP_REPLAY_ISOLATION_REQUIRED"))
	if v != "1" {
		t.Skipf("SOP_REPLAY_ISOLATION_REQUIRED=%q, want 1; skipping required-env check", v)
	}
}

// TestIsolatedReplayToolchainProcAndTemp proves the sandbox provides a usable /proc and
// temp storage inside the replay directory, and that a real Go toolchain command runs.
func TestIsolatedReplayToolchainProcAndTemp(t *testing.T) {
	requireBubblewrapRuntime(t)

	if _, err := exec.LookPath("go"); err != nil {
		t.Fatalf("go toolchain is required but missing: %v", err)
	}

	f := isolationFixture("go", "test", "./...")
	f.Files = map[string]string{
		"go.mod": "module replaytest\n\ngo 1.22\n",
		"replay_test.go": `package replaytest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplay(t *testing.T) {
	if _, err := os.Stat("/proc/self/status"); err != nil {
		t.Fatalf("/proc/self/status: %v", err)
	}
	f, err := os.CreateTemp("", "replay-temp-*")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	name := f.Name()
	if _, err := f.WriteString("ok"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	defer os.Remove(name)

	tmp := os.Getenv("TMPDIR")
	if tmp == "" {
		t.Fatal("TMPDIR is empty")
	}
	abs, err := filepath.Abs(name)
	if err != nil {
		t.Fatal(err)
	}
	rel, err := filepath.Rel(tmp, abs)
	if err != nil || strings.HasPrefix(rel, "..") {
		t.Fatalf("temp file %q is outside TMPDIR %q", name, tmp)
	}
	if _, err := os.Stat(name); err != nil {
		t.Fatalf("temp stat: %v", err)
	}
}
`,
	}

	code, out, err := IsolatedReplay(context.Background(), t.TempDir(), f, 120*time.Second)
	if err != nil {
		t.Fatalf("IsolatedReplay error: %v (out=%q)", err, out)
	}
	if code != 0 {
		t.Fatalf("toolchain/proc/temp check failed: code=%d out=%q", code, out)
	}
}
