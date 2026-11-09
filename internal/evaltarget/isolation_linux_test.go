//go:build linux

package evaltarget

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// probeEndpoint performs a minimal HTTP request through the live endpoint socket.
// It proves the model endpoint is reachable through the only permitted channel,
// without issuing an inference request.
func probeEndpoint() error {
	socket := strings.TrimSpace(os.Getenv(LiveSocketEnv))
	if socket == "" {
		return fmt.Errorf("live endpoint socket is not configured")
	}
	conn, err := net.Dial("unix", socket)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	if _, err := fmt.Fprintf(conn, "GET /api/tags HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", LiveEndpointDialAddress); err != nil {
		return err
	}
	resp, err := http.ReadResponse(bufio.NewReader(conn), &http.Request{Method: http.MethodGet})
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("live endpoint returned status %d", resp.StatusCode)
	}
	return nil
}

func probeEndpointReachable() bool { return probeEndpoint() == nil }

func probeEndpointDetail() string {
	socket := strings.TrimSpace(os.Getenv(LiveSocketEnv))
	if socket == "" {
		return "unix socket not configured"
	}
	return "unix socket " + socket
}

// runIsolatedLinux runs argv under the default or opt-in live bwrap sandbox and
// returns its combined output and exit error. The live variant's proxy is
// released when the returned context is canceled.
func runIsolatedLinux(t *testing.T, live bool, root string, argv []string, env []string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var (
		cmd *exec.Cmd
		err error
	)
	if live {
		cmd, err = IsolateLiveCommand(ctx, root, argv, env, root)
	} else {
		cmd, err = IsolateCommand(ctx, root, argv, env, root)
	}
	if err != nil {
		t.Fatalf("build isolation command: %v", err)
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// hostDialable is the unsandboxed control for a network probe. A failed
// connection inside the sandbox is evidence only when the same destination is
// reachable outside it.
func hostDialable(host, port string) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 2*time.Second)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// TestLiveIsolationLinux proves the opt-in live sandbox at the OS boundary:
// the model endpoint is reachable only through the Unix-socket bridge, direct
// TCP to the endpoint and to another loopback port is denied, a non-loopback
// destination is denied, files outside the root are unreadable, and secret
// environment variables are scrubbed. Every denial that can have an
// unsandboxed positive control is paired with one.
func TestLiveIsolationLinux(t *testing.T) {
	if !IsolationAvailable() {
		t.Skip("bwrap is not available")
	}
	nc, err := exec.LookPath("nc")
	if err != nil {
		t.Skip("nc is not available")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh is not available")
	}
	if !hostDialable("127.0.0.1", "11434") {
		t.Skip("local model endpoint is not reachable from the host")
	}

	// A host listener on another loopback port is the positive control for the
	// "other loopback denied" probe.
	negative, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = negative.Close() }()
	negativePort := strconv.Itoa(negative.Addr().(*net.TCPAddr).Port)
	if !hostDialable("127.0.0.1", negativePort) {
		t.Fatalf("host listener on 127.0.0.1:%s is not reachable outside the sandbox", negativePort)
	}

	externalReachable := hostDialable("1.1.1.1", "443")
	t.Logf("unsandboxed controls: model endpoint reachable=true, loopback:%s reachable=true, 1.1.1.1:443 reachable=%v", negativePort, externalReachable)

	root := t.TempDir()
	env := IsolateEnv(append(os.Environ(),
		"OPENAI_API_KEY=sk-canary-should-not-appear",
		"SOP_LIVE_CANARY=visible",
	), "")

	// The endpoint is reachable only through the socket bridge.
	probe := fmt.Sprintf(`printf 'GET /api/tags HTTP/1.1\r\nHost: 127.0.0.1:11434\r\nConnection: close\r\n\r\n' | %s -U "$%s"`, nc, LiveSocketEnv)
	out, err := runIsolatedLinux(t, true, root, []string{sh, "-c", probe}, env)
	if err != nil {
		t.Fatalf("live endpoint socket probe failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, " 200 ") {
		t.Fatalf("live endpoint socket probe did not return HTTP 200:\n%s", out)
	}

	// Direct TCP to the endpoint is denied even though the endpoint is reachable
	// through the permitted bridge.
	if out, err := runIsolatedLinux(t, true, root, []string{nc, "-z", "-w", "1", "127.0.0.1", "11434"}, env); err == nil {
		t.Fatalf("direct TCP to the model endpoint succeeded inside the live sandbox\n%s", out)
	}

	// Another loopback port is denied while the host can reach it.
	if out, err := runIsolatedLinux(t, true, root, []string{nc, "-z", "-w", "1", "127.0.0.1", negativePort}, env); err == nil {
		t.Fatalf("another loopback port succeeded inside the live sandbox\n%s", out)
	}

	// A non-loopback destination is denied. The host control above records
	// whether the same destination was reachable outside the sandbox.
	if out, err := runIsolatedLinux(t, true, root, []string{nc, "-z", "-w", "2", "1.1.1.1", "443"}, env); err == nil {
		t.Fatalf("a non-loopback destination succeeded inside the live sandbox\n%s", out)
	}

	// The default sandbox still denies all network.
	if out, err := runIsolatedLinux(t, false, root, []string{nc, "-z", "-w", "1", "127.0.0.1", "11434"}, env); err == nil {
		t.Fatalf("the default isolation must deny all network\n%s", out)
	}

	// Filesystem restrictions: a file outside the sandbox root is unreadable.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(wd, "..", "..", "go.mod")
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside sentinel: %v", err)
	}
	if out, err := runIsolatedLinux(t, true, root, []string{"/bin/cat", outside}, env); err == nil {
		t.Fatalf("a file outside the root must be unreadable\n%s", out)
	}

	// Environment scrubbing: the secret is removed, a non-secret marker survives.
	out, err = runIsolatedLinux(t, true, root, []string{"/usr/bin/env"}, env)
	if err != nil {
		t.Fatalf("env probe: %v\n%s", err, out)
	}
	if strings.Contains(out, "sk-canary-should-not-appear") {
		t.Fatalf("secret environment variable reached the isolated child")
	}
	if !strings.Contains(out, "SOP_LIVE_CANARY=visible") {
		t.Fatalf("non-secret environment variable did not reach the isolated child")
	}
}

// TestLiveIsolationLinuxCancelsProcessGroup proves a canceled live sandbox does
// not leave the sandboxed shell or its descendant behind. The marker appears in
// both command lines, so a survivor is detected by scanning /proc on the host.
func TestLiveIsolationLinuxCancelsProcessGroup(t *testing.T) {
	if !IsolationAvailable() {
		t.Skip("bwrap is not available")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh is not available")
	}
	const marker = "987654321"
	root := t.TempDir()
	env := IsolateEnv(os.Environ(), "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// The marker is both in the shell's -c argument and in the sleep child's
	// argv, so scanning /proc detects either process if it survives.
	script := `: > started; sleep 987654321 & wait`
	cmd, err := IsolateLiveCommand(ctx, root, []string{sh, "-c", script}, env, root)
	if err != nil {
		t.Fatalf("build live isolation command: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start live sandbox: %v", err)
	}
	started := filepath.Join(root, "started")
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(started); err == nil {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			_ = cmd.Wait()
			t.Fatal("live sandbox did not start")
		}
		time.Sleep(20 * time.Millisecond)
	}

	cancel()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("canceled live sandbox did not exit")
	}

	// A killed process can take a moment to disappear from /proc.
	deadline = time.Now().Add(3 * time.Second)
	for {
		survivors := processesWithMarker(marker)
		if len(survivors) == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("processes survived live-sandbox cancellation: %v", survivors)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// processesWithMarker returns the command lines under /proc that contain marker.
// It is used only to observe that a canceled sandbox and its descendants exited.
func processesWithMarker(marker string) []string {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	var survivors []string
	for _, e := range entries {
		if _, err := strconv.Atoi(e.Name()); err != nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join("/proc", e.Name(), "cmdline"))
		if err != nil || len(data) == 0 {
			continue
		}
		cmdline := strings.ReplaceAll(string(data), "\x00", " ")
		if strings.Contains(cmdline, marker) {
			survivors = append(survivors, e.Name()+": "+strings.TrimSpace(cmdline))
		}
	}
	return survivors
}
