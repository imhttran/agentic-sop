package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/executionlease"
)

// The default configuration acquires nothing: execution is exactly what it was
// before the lease existed.
func TestExecutionLeaseOffByDefault(t *testing.T) {
	dir := t.TempDir()

	held, err := executionLease(config.Config{}, dir, executionlease.Owner{Kind: executionlease.KindRun}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("executionLease(off) = %v, want no error", err)
	}
	if held != nil {
		t.Fatal("executionLease(off) acquired a lease")
	}
	if state, err := executionlease.Read(dir); err != nil || state.Present {
		t.Fatalf("read = %+v, %v; want no lease file", state, err)
	}
}

// required acquires a free project and releases it at the end of the run.
func TestExecutionLeaseRequiredAcquiresAFreeProject(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Config{Execution: config.ExecutionConfig{Lease: "required"}}

	held, err := executionLease(cfg, dir, executionlease.Owner{Kind: executionlease.KindRun, TaskID: "T1"}, &bytes.Buffer{})
	if err != nil || held == nil {
		t.Fatalf("executionLease(required) = %v, %v; want a lease", held, err)
	}
	if state, err := executionlease.Read(dir); err != nil || !state.Present || state.Owner.TaskID != "T1" {
		t.Fatalf("read = %+v, %v; want the T1 lease", state, err)
	}
	held.Release()
	if state, err := executionlease.Read(dir); err != nil || state.Present {
		t.Fatalf("read after release = %+v, %v; want no lease", state, err)
	}
}

// required refuses a contended project and names the holder; advisory records the
// contention and proceeds; a nil lease is always safe to release.
func TestExecutionLeaseContention(t *testing.T) {
	dir := t.TempDir()
	holder, err := executionlease.Acquire(dir, executionlease.Owner{Kind: executionlease.KindRun, TaskID: "OTHER"})
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Release()

	required := config.Config{Execution: config.ExecutionConfig{Lease: "required"}}
	held, err := executionLease(required, dir, executionlease.Owner{Kind: executionlease.KindRun}, &bytes.Buffer{})
	var busy *executionlease.BusyError
	if !errors.As(err, &busy) || held != nil {
		t.Fatalf("executionLease(required, contended) = %v, %v; want *BusyError", held, err)
	}
	if !strings.Contains(err.Error(), "OTHER") {
		t.Fatalf("error = %q, want it to name the holder", err)
	}

	var stderr bytes.Buffer
	advisory := config.Config{Execution: config.ExecutionConfig{Lease: "advisory"}}
	held, err = executionLease(advisory, dir, executionlease.Owner{Kind: executionlease.KindRun}, &stderr)
	if err != nil || held != nil {
		t.Fatalf("executionLease(advisory, contended) = %v, %v; want to proceed without a lease", held, err)
	}
	if !strings.Contains(stderr.String(), "advisory execution lease") {
		t.Fatalf("stderr = %q, want the contention recorded", stderr.String())
	}

	var nilHeld *executionlease.Held
	nilHeld.Release()
}

// An unknown mode is a configuration error, not a silent fallback to no protection.
func TestExecutionLeaseRejectsAnUnknownMode(t *testing.T) {
	cfg := config.Config{Execution: config.ExecutionConfig{Lease: "sometimes"}}
	if _, err := executionLease(cfg, t.TempDir(), executionlease.Owner{Kind: executionlease.KindRun}, &bytes.Buffer{}); err == nil {
		t.Fatal("executionLease accepted an unknown mode")
	}
}

// `sop run` refuses to drive a project another execution holds, and names the holder
// rather than starting a second run.
func TestRunRefusesWhileAnotherExecutionHoldsTheProject(t *testing.T) {
	dir := newDirtyRepo(t)
	initProject(t, dir)
	writeConfig(t, dir, manualRecoveryConfig+"execution:\n  lease: required\n")

	holder, err := executionlease.Acquire(dir, executionlease.Owner{Kind: executionlease.KindRun, JobID: "work-elsewhere"})
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Release()

	a := &recoveryPlanAgent{}
	code, _, stderr := runInjectedCLI(t, dir, "diff --git a/out.txt b/out.txt\n", a, "run")
	if code != exitError {
		t.Fatalf("run = %d, want %d; stderr=%s", code, exitError, stderr)
	}
	if !strings.Contains(stderr, "project is executing") || !strings.Contains(stderr, "work-elsewhere") {
		t.Fatalf("stderr = %q, want the refusal to name the holder", stderr)
	}
	if len(a.implementations) != 0 {
		t.Fatalf("agent ran while the lease was held: %v", a.implementations)
	}
}

// A run releases the lease it took, so the next invocation is free.
func TestRunReleasesTheLeaseOnExit(t *testing.T) {
	dir := newDirtyRepo(t)
	initProject(t, dir)
	writeConfig(t, dir, manualRecoveryConfig+"execution:\n  lease: required\n")

	a := &recoveryPlanAgent{}
	runInjectedCLI(t, dir, "diff --git a/out.txt b/out.txt\n", a, "run")

	if _, err := os.Stat(filepath.Join(dir, config.DirName, executionlease.FileName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lease file still present after the run: %v", err)
	}
}
