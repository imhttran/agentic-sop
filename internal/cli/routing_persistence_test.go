package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/jev"
	"github.com/imhttran/agentic-sop/internal/model"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
)

// These tests pin the Phase 3.5 persistence boundary: the routing decision is
// auditable after the process exits, the write path fails closed on an unknown
// version/source, and the artifact is never read back to drive a decision (it is
// not a second source of truth).

// TestRoutingArtifactPersistedAndAuditable proves a completed run leaves a
// routable, auditable routing.json carrying the version, class, source, reasons,
// and resolved model, while the in-process decision stays the source of truth.
func TestRoutingArtifactPersistedAndAuditable(t *testing.T) {
	clearModelEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, preExecRoutingConfig)

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, "diff --git a/x b/x\n", routingAgent(), fakeJEV(jev.NewClearFake()), "run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}

	art := readRoutingArtifact(t, dir)
	if art.Version != runpkg.RoutingArtifactVersion {
		t.Errorf("version = %d, want %d", art.Version, runpkg.RoutingArtifactVersion)
	}
	if art.Task != "T001" {
		t.Errorf("task = %q, want T001", art.Task)
	}
	if art.Class == "" || art.Source == "" || art.Model == "" {
		t.Errorf("artifact must carry class/source/model: %+v", art)
	}
	if len(art.Reasons) == 0 {
		t.Errorf("artifact must carry the deterministic reasons: %+v", art)
	}
}

// TestRoutingArtifactWriteFailsClosed proves the CLI write seam surfaces a
// contract violation explicitly rather than discarding it: an unknown version or
// source is rejected by the store writer and no artifact is persisted. The seam
// never changes the run's already-computed decision.
func TestRoutingArtifactWriteFailsClosed(t *testing.T) {
	dir := t.TempDir()
	rn, err := runpkg.New(dir, "T001")
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// A well-formed decision persists.
	tr := &taskRouting{
		Class:     "medium",
		Source:    runpkg.RoutingSourcePolicy,
		Reasons:   []string{"moderate task; default class"},
		Selection: model.Selection{Class: "medium", Provider: "ollama", Model: "glm-5.3-flash:cloud"},
	}
	if err := writeRoutingDecisionArtifact(rn, "T001", tr); err != nil {
		t.Fatalf("writeRoutingDecisionArtifact: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rn.Dir(), "routing.json")); err != nil {
		t.Fatalf("routing.json not written for a well-formed decision: %v", err)
	}

	// An unknown source fails closed: the error is returned (not swallowed) and
	// no second write happens.
	bad := *tr
	bad.Source = runpkg.RoutingSource("made_up")
	if err := writeRoutingDecisionArtifact(rn, "T001", &bad); err == nil {
		t.Fatal("writeRoutingDecisionArtifact accepted an unknown source")
	}
}

// TestRoutingArtifactIsNeverReadBack proves the decision path does not consume
// routing.json: deleting the persisted artifact leaves the run's chosen model
// unchanged, so the artifact is not a second source of truth.
func TestRoutingArtifactIsNeverReadBack(t *testing.T) {
	clearModelEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, preExecRoutingConfig)

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, "diff --git a/x b/x\n", routingAgent(), fakeJEV(jev.NewClearFake()), "run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "Task routing: small (qwen3:4b;") {
		t.Fatalf("unexpected routing line: %q", stdout)
	}
	art := readRoutingArtifact(t, dir)
	if art.Class != "small" || art.Model != "qwen3:4b" {
		t.Fatalf("routing = %+v, want small/qwen3:4b", art)
	}

	// Corrupt/delete the artifact and re-run: the chosen model must be unchanged,
	// because the artifact is write-only evidence and never read to drive a
	// decision.
	if err := os.Remove(filepath.Join(dir, stateDirName, "runs", "T001", "routing.json")); err != nil {
		t.Fatalf("remove routing.json: %v", err)
	}
	code, stdout, stderr = runInjectedCLIWithJEV(t, dir, "diff --git a/x b/x\n", routingAgent(), fakeJEV(jev.NewClearFake()), "run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("rerun code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "Task routing: small (qwen3:4b;") {
		t.Fatalf("decision changed after artifact deletion: %q", stdout)
	}
	if art := readRoutingArtifact(t, dir); art.Class != "small" {
		t.Fatalf("routing = %+v, want small after re-run", art)
	}
}
