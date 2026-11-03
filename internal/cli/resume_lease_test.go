package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/executionlease"
)

// writeExecutionLease writes a lease record as a crashed or running execution
// leaves it, without holding it.
func writeExecutionLease(t *testing.T, dir string, owner executionlease.Owner) string {
	t.Helper()
	raw, err := json.Marshal(owner)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, stateDirName, executionlease.FileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// A lease left behind by a crashed run is reported and released by resume, and the
// next start succeeds: the proposal's crash/stale acceptance case.
func TestRunResumeReleasesStaleExecutionLease(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	writeExecutionLease(t, dir, executionlease.Owner{
		Kind:        executionlease.KindRun,
		PID:         999999,
		Host:        executionlease.Hostname(),
		StartedAt:   time.Now().Add(-time.Hour),
		HeartbeatAt: time.Now().Add(-2 * executionlease.HeartbeatLimit),
	})

	code, out, stderr := runCLIWithResources(t, dir, &fakeResources{}, "resume")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stderr, "stale") || !strings.Contains(stderr, "pid 999999") {
		t.Errorf("stderr = %q, want the stale holder reported", stderr)
	}
	if !strings.Contains(out, "nothing to resume") {
		t.Errorf("stdout = %q, want resume to carry on", out)
	}

	state, err := executionlease.Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if state.Present {
		t.Fatalf("stale lease still present: %+v", state)
	}

	held, err := executionlease.Acquire(dir, executionlease.Owner{Kind: executionlease.KindRun})
	if err != nil {
		t.Fatalf("Acquire after stale release: %v", err)
	}
	held.Release()
}

// Resume must never release a live lease: it reports the holder and leaves it in
// place, so the running execution keeps the project.
func TestRunResumeLeavesLiveExecutionLease(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	now := time.Now().UTC()
	writeExecutionLease(t, dir, executionlease.Owner{
		Kind:        executionlease.KindRun,
		PID:         os.Getpid(),
		Host:        executionlease.Hostname(),
		StartedAt:   now,
		HeartbeatAt: now,
	})

	code, out, stderr := runCLIWithResources(t, dir, &fakeResources{}, "resume")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stderr, "held by") || !strings.Contains(stderr, fmt.Sprintf("pid %d", os.Getpid())) {
		t.Errorf("stderr = %q, want the live holder reported", stderr)
	}
	if strings.Contains(stderr, "stale") {
		t.Errorf("stderr = %q, want a live lease not described as stale", stderr)
	}
	if !strings.Contains(out, "nothing to resume") {
		t.Errorf("stdout = %q, want resume to carry on", out)
	}

	state, err := executionlease.Read(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Present {
		t.Fatal("live lease was removed")
	}

	_, err = executionlease.Acquire(dir, executionlease.Owner{Kind: executionlease.KindPrompt})
	var busy *executionlease.BusyError
	if !errors.As(err, &busy) {
		t.Fatalf("Acquire after resume = %v, want the live lease still held", err)
	}
}
