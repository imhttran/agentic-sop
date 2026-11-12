package dqharness

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// These tests use synthetic fixtures to verify the isolation controls. They are not
// historical evidence and are never used for the evaluation corpus.
func isolationFixture(cmd ...string) ReplayFixture {
	return ReplayFixture{
		CaseID:              "isolation-test",
		Revision:            "synthetic",
		TaskInput:           "synthetic isolation fixture",
		VerificationCommand: cmd,
		Provenance:          "synthetic test fixture, not historical evidence",
	}
}

func TestReplayFixtureRequiresProvenance(t *testing.T) {
	f := isolationFixture("true")
	f.Provenance = ""
	if err := f.Validate(); err == nil {
		t.Fatal("a fixture without provenance must be rejected")
	}
}

// requireIsolation skips a replay test on hosts without a usable OS-level
// isolation tool. Production never falls back to unsandboxed replay; it reports
// the blocker instead. Set SOP_REPLAY_ISOLATION_REQUIRED=1 to turn the skip into a
// failure where isolation must be present.
func requireIsolation(t *testing.T) {
	t.Helper()
	err := isolationBlocker()
	if err == nil {
		return
	}
	if v := strings.TrimSpace(os.Getenv("SOP_REPLAY_ISOLATION_REQUIRED")); v == "1" || strings.EqualFold(v, "true") {
		t.Fatalf("required replay isolation is unavailable: %v", err)
	}
	t.Skipf("OS-level isolation cannot run here: %v; set SOP_REPLAY_ISOLATION_REQUIRED=1 to require it", err)
}

// isolationBlocker reports why the OS sandbox cannot run a replay command on
// this host, or nil when it can. A backend that exists but cannot start is
// unusable: macOS Seatbelt refuses sandbox_apply inside another sandbox, and
// nested bubblewrap cannot mount /proc once a user and PID namespace are both
// unshared. The probe uses this package's own command builders, so it reflects
// the real argument list. Production stays fail-closed and never reuses the
// parent's boundary.
func isolationBlocker() error {
	isolationOnce.Do(func() { isolationProbeErr = probeIsolation() })
	return isolationProbeErr
}

var (
	isolationOnce     sync.Once
	isolationProbeErr error
)

// isolationProbeTimeout bounds the one-shot start probe.
const isolationProbeTimeout = 30 * time.Second

func probeIsolation() error {
	iso := DetectIsolation()
	if !iso.Available {
		return errors.New(iso.Reason)
	}
	dir, err := os.MkdirTemp("", "sop-replay-probe-")
	if err != nil {
		return fmt.Errorf("create probe directory: %w", err)
	}
	defer os.RemoveAll(dir)

	var tool string
	var args []string
	switch iso.Backend {
	case BackendSeatbelt:
		bound, err := filepath.EvalSymlinks(dir)
		if err != nil {
			return err
		}
		tool, args = "sandbox-exec", seatbeltCommand(bound, []string{"true"})
	case BackendBubblewrap:
		_, a, err := bubblewrapCommand(dir, []string{"true"})
		if err != nil {
			return err
		}
		tool, args = "bwrap", a
	default:
		return fmt.Errorf("unknown isolation backend %q", iso.Backend)
	}

	ctx, cancel := context.WithTimeout(context.Background(), isolationProbeTimeout)
	defer cancel()
	if out, err := exec.CommandContext(ctx, tool, args...).CombinedOutput(); err != nil {
		return fmt.Errorf("%s cannot run a replay sandbox here: %v: %s (a nested OS sandbox is the usual cause, and it is not supported)",
			tool, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// TestIsolatedReplayFailsClosedWithoutIsolation runs on every host: with no isolation
// tool on PATH, IsolatedReplay refuses and the verification command never runs
// unsandboxed.
func TestIsolatedReplayFailsClosedWithoutIsolation(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if iso := DetectIsolation(); iso.Available {
		t.Fatalf("isolation detected with an empty PATH: %+v", iso)
	}
	marker := filepath.Join(t.TempDir(), "ran.txt")
	f := isolationFixture("/bin/sh", "-c", "echo ran > "+marker)
	code, _, err := IsolatedReplay(context.Background(), t.TempDir(), f, 10*time.Second)
	if err == nil || code != -1 || !strings.Contains(err.Error(), "replay isolation unavailable") {
		t.Fatalf("code=%d err=%v, want a refusal for unavailable isolation", code, err)
	}
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Fatal("the verification command ran without isolation")
	}
}

func TestIsolatedReplayRunsVerificationInFreshDir(t *testing.T) {
	requireIsolation(t)
	parent := t.TempDir()
	f := isolationFixture("sh", "-c", "test -f in.txt && test \"$(cat in.txt)\" = ok")
	f.Files = map[string]string{"in.txt": "ok"}
	code, out, err := IsolatedReplay(context.Background(), parent, f, 10*time.Second)
	if err != nil || code != 0 {
		t.Fatalf("code=%d out=%q err=%v", code, out, err)
	}
	entries, _ := os.ReadDir(parent)
	if len(entries) != 0 {
		t.Fatalf("replay directory was not cleaned up: %v", entries)
	}
}

func TestIsolatedReplayScrubsCredentials(t *testing.T) {
	requireIsolation(t)
	t.Setenv("OLLAMA_API_KEY", "secret-value")
	t.Setenv("SOP_CONTROLLER_TOKEN", "secret-value")
	f := isolationFixture("sh", "-c", "test -z \"$OLLAMA_API_KEY\" && test -z \"$SOP_CONTROLLER_TOKEN\" && test -z \"$SOP_OMLX_API_KEY\"")
	code, out, err := IsolatedReplay(context.Background(), t.TempDir(), f, 10*time.Second)
	if err != nil || code != 0 {
		t.Fatalf("child saw a credential: code=%d out=%q err=%v", code, out, err)
	}
}

func TestIsolatedReplayDeniesWritesOutside(t *testing.T) {
	requireIsolation(t)
	parent := t.TempDir()
	escape := filepath.Join(parent, "escape.txt")
	f := isolationFixture("sh", "-c", "echo escaped > "+escape)
	code, out, err := IsolatedReplay(context.Background(), parent, f, 10*time.Second)
	if err == nil && code == 0 {
		t.Fatalf("write outside the replay directory succeeded: out=%q", out)
	}
	if _, statErr := os.Stat(escape); statErr == nil {
		t.Fatal("a file was written outside the replay directory")
	}
}

func TestIsolatedReplayDeniesNetwork(t *testing.T) {
	requireIsolation(t)
	if _, err := exec.LookPath("python3"); err != nil {
		t.Fatalf("python3 client is required but missing: %v", err)
	}

	// A host listener gives the denial a deterministic positive control: the same
	// client connects without the sandbox, so a failure below is the sandbox rather
	// than a host with no network. This also keeps the test offline.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	host, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("split %q: %v", ln.Addr(), err)
	}
	conn, err := net.DialTimeout("tcp", ln.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatalf("unsandboxed positive control cannot reach %s: %v", ln.Addr(), err)
	}
	_ = conn.Close()

	client := fmt.Sprintf("import socket; s=socket.socket(); s.settimeout(2); s.connect((%q, %s))", host, port)
	f := isolationFixture("python3", "-c", client)
	code, out, err := IsolatedReplay(context.Background(), t.TempDir(), f, 10*time.Second)
	if err == nil && code == 0 {
		t.Fatalf("network access succeeded inside the replay: out=%q", out)
	}
}

func TestIsolatedReplayTimesOut(t *testing.T) {
	requireIsolation(t)
	f := isolationFixture("sh", "-c", "sleep 5")
	_, _, err := IsolatedReplay(context.Background(), t.TempDir(), f, 300*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err = %v, want a timeout", err)
	}
}
