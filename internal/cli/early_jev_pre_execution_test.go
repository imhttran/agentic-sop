package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/jev"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
)

// These tests pin the P3-009 pre-execution analysis seam: the early JEV
// pre-execution checkpoint is invoked immediately before agent execution, it is
// analysis-only and read-only, it is a strict no-op when disabled or absent, and
// a provider failure is recorded distinctly from findings (never as a PASS).

// earlyPreExecEnabledConfig enables ONLY the pre-execution gate.
const earlyPreExecEnabledConfig = "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\nearly_jev:\n  enabled: true\n  mode: review\n  gates:\n    pre_execution: true\n"

// readEarlyArtifact loads .agent-sdlc/runs/<id>/early-jev.json for a task.
func readEarlyArtifact(t *testing.T, dir, id string) (runpkg.EarlyArtifact, bool) {
	t.Helper()
	path := filepath.Join(dir, ".agent-sdlc", "runs", id, "early-jev.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return runpkg.EarlyArtifact{}, false
		}
		t.Fatalf("read early artifact: %v", err)
	}
	var art runpkg.EarlyArtifact
	if err := json.Unmarshal(data, &art); err != nil {
		t.Fatalf("unmarshal early artifact: %v", err)
	}
	return art, true
}

// readRunState loads the persisted state.json bytes for a run, if it exists.
func readRunState(t *testing.T, dir string) []byte {
	t.Helper()
	path := filepath.Join(dir, ".agent-sdlc", "runs", "T001", "state.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("read state.json: %v", err)
	}
	return data
}

// TestPreExecutionDisabledIsNoOp proves that with the pre-execution gate
// disabled (the default), the checkpoint never runs, the analyzer is never
// consulted, and no early artifact is written: the run is unchanged.
func TestPreExecutionDisabledIsNoOp(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n")
	a := &earlyCountingAgent{}
	analyzer := &earlyFakeAnalyzer{result: earlyEvidenceResult(jev.PurposePreExecution, jev.CategorySecurity, jev.SeverityHigh)}

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, "diff --git a/x b/x\n", a,
		func(config.Config) (jev.Analyzer, error) { return analyzer, nil },
		"run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if analyzer.calls != 0 {
		t.Errorf("disabled pre-execution consulted the analyzer %d times, want 0", analyzer.calls)
	}
	if a.impl != 1 {
		t.Errorf("implementation ran %d times, want 1", a.impl)
	}
	if _, ok := readEarlyArtifact(t, dir, "T001"); ok {
		t.Errorf("disabled pre-execution must not write an early artifact")
	}
}

// TestPreExecutionAbsentAnalyzerIsNoOp proves that when the pre-execution gate
// is enabled but no analyzer is wired (nil factory result), the gate is a strict
// no-op: nothing runs, nothing is persisted, and the run proceeds unchanged.
func TestPreExecutionAbsentAnalyzerIsNoOp(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, earlyPreExecEnabledConfig)
	a := &earlyCountingAgent{}

	calls := 0
	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, "diff --git a/x b/x\n", a,
		func(config.Config) (jev.Analyzer, error) { calls++; return nil, nil },
		"run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if calls != 1 {
		t.Errorf("analyzer factory called %d times, want 1", calls)
	}
	if a.impl != 1 {
		t.Errorf("implementation ran %d times, want 1 (absent analyzer is a no-op)", a.impl)
	}
	if _, ok := readEarlyArtifact(t, dir, "T001"); ok {
		t.Errorf("an absent analyzer must not write an early artifact")
	}
}

// TestPreExecutionProviderFailureRecordedSeparately proves that a provider
// failure at the pre-execution seam is recorded distinctly from findings: the
// artifact carries provider_failed=true and zero evidence, and the run does not
// treat the failure as a PASS or a finding (the advisory path continues).
func TestPreExecutionProviderFailureRecordedSeparately(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, earlyPreExecEnabledConfig)
	a := &earlyCountingAgent{}
	analyzer := &earlyFakeAnalyzer{err: jev.ErrProviderFailure}

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, "diff --git a/x b/x\n", a,
		func(config.Config) (jev.Analyzer, error) { return analyzer, nil },
		"run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("an advisory provider failure must not block: code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if analyzer.calls != 1 {
		t.Errorf("pre-execution analyzer calls = %d, want 1", analyzer.calls)
	}
	if a.impl != 1 {
		t.Errorf("implementation ran %d times, want 1", a.impl)
	}

	art, ok := readEarlyArtifact(t, dir, "T001")
	if !ok {
		t.Fatalf("provider failure must be recorded as an early artifact")
	}
	if art.Checkpoint != runpkg.CheckpointPreExecution {
		t.Errorf("checkpoint = %q, want %q", art.Checkpoint, runpkg.CheckpointPreExecution)
	}
	if !art.ProviderFailed {
		t.Errorf("provider_failed = false, want true")
	}
	if len(art.Evidence.Items) != 0 {
		t.Errorf("provider failure carried %d evidence items, want 0", len(art.Evidence.Items))
	}
	if strings.Contains(stdout, "NEEDS_HUMAN") {
		t.Errorf("a provider failure must not escalate: %q", stdout)
	}
}

// TestPreExecutionFindingsRecordedWithEvidence proves that a successful analysis
// with findings is recorded as evidence with provider_failed=false, and that the
// invocation happens before implementation.
func TestPreExecutionFindingsRecordedWithEvidence(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, earlyPreExecEnabledConfig)
	a := &earlyCountingAgent{}
	analyzer := &earlyFakeAnalyzer{result: earlyEvidenceResult(jev.PurposePreExecution, jev.CategoryScope, jev.SeverityLow)}

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, "diff --git a/x b/x\n", a,
		func(config.Config) (jev.Analyzer, error) { return analyzer, nil },
		"run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if analyzer.calls != 1 {
		t.Errorf("pre-execution analyzer calls = %d, want 1", analyzer.calls)
	}
	if a.impl != 1 {
		t.Errorf("implementation ran %d times, want 1", a.impl)
	}

	art, ok := readEarlyArtifact(t, dir, "T001")
	if !ok {
		t.Fatalf("a findings result must be recorded as an early artifact")
	}
	if art.ProviderFailed {
		t.Errorf("provider_failed = true, want false")
	}
	if len(art.Evidence.Items) == 0 {
		t.Errorf("findings result recorded no evidence items")
	}
}

// TestPreExecutionSeamDoesNotMutateState proves the seam is read-only with
// respect to SOP persistence: state.json is preserved across a run in which
// the pre-execution checkpoint fires, and only the diagnostic early artifact is
// added.
func TestPreExecutionSeamDoesNotMutateState(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, earlyPreExecEnabledConfig)
	a := &earlyCountingAgent{}
	analyzer := &earlyFakeAnalyzer{result: earlyClearResult(jev.PurposePreExecution)}

	code, _, stderr := runInjectedCLIWithJEV(t, dir, "diff --git a/x b/x\n", a,
		func(config.Config) (jev.Analyzer, error) { return analyzer, nil },
		"run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if analyzer.calls != 1 {
		t.Errorf("pre-execution analyzer calls = %d, want 1", analyzer.calls)
	}

	// The seam's persistence is add-only: the diagnostic artifact exists and is
	// valid, and it never replaced the run's authoritative state.
	art, ok := readEarlyArtifact(t, dir, "T001")
	if !ok {
		t.Fatalf("expected the diagnostic early artifact")
	}
	if err := art.Validate(); err != nil {
		t.Errorf("early artifact invalid: %v", err)
	}
	if state := readRunState(t, dir); state == nil {
		t.Errorf("state.json missing; the seam must not delete or skip state persistence")
	}
}
