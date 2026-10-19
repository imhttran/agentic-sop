package dqharness

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

// requireIsolation skips a replay test on hosts without an OS-level isolation tool.
// Production never falls back to unsandboxed replay; it reports the blocker instead.
// Set SOP_REPLAY_ISOLATION_REQUIRED=1 to turn the skip into a failure where isolation
// must be present.
func requireIsolation(t *testing.T) {
	t.Helper()
	iso := DetectIsolation()
	if iso.Available {
		return
	}
	if v := strings.TrimSpace(os.Getenv("SOP_REPLAY_ISOLATION_REQUIRED")); v == "1" || strings.EqualFold(v, "true") {
		t.Fatalf("required replay isolation is unavailable: %s", iso.Reason)
	}
	t.Skipf("no OS-level isolation available: %s; set SOP_REPLAY_ISOLATION_REQUIRED=1 to require it", iso.Reason)
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
	f := isolationFixture("python3", "-c", "import socket; s=socket.socket(); s.settimeout(1); s.connect(('1.1.1.1',80))")
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
