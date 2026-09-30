package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/jev"
	"github.com/imhttran/agentic-sop/internal/model"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
)

// These tests pin the Phase 3.5 per-task model routing integration: the router
// selects a class from the early-JEV checkpoints' typed evidence, resolves it to a
// concrete model, records the decision, and is a strict no-op unless enabled. It
// never transitions state or bypasses a gate.

// preExecRoutingConfig enables ONLY the pre-execution checkpoint (so routing has
// evidence) without enabling triage.
const preExecRoutingConfig = "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\nearly_jev:\n  enabled: true\n  mode: review\n  gates:\n    pre_execution: true\n"

// plainRoutingConfig has no early_jev block: routing is enabled but no evidence is
// available, so the router falls back to the safe default.
const plainRoutingConfig = "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n"

// clearRoutingConfig enables the pre-execution gate but keeps high-severity
// findings non-blocking, so a routing escalation can be observed without the
// policy stopping the task at a human boundary.
const clearRoutingConfig = "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\nearly_jev:\n  enabled: true\n  mode: review\n  fail_on:\n    - critical\n  gates:\n    pre_execution: true\n"

func routingAgent() *fakeCapabilityAgent {
	return &fakeCapabilityAgent{plan: validPlanJSON, impl: "changed files", review: `{"summary":"clean","findings":[]}`}
}

func fakeJEV(a jev.Analyzer) func(config.Config) (jev.Analyzer, error) {
	return func(config.Config) (jev.Analyzer, error) { return a, nil }
}

func readRoutingArtifact(t *testing.T, dir string) runpkg.RoutingArtifact {
	t.Helper()
	var art runpkg.RoutingArtifact
	readRunJSON(t, filepath.Join(dir, stateDirName, "runs", "T001", "routing.json"), &art)
	return art
}

// TestRoutingDisabledIsUnchanged proves that with the router off (the default),
// no routing line is printed and no routing artifact is written: existing behavior
// is unchanged.
func TestRoutingDisabledIsUnchanged(t *testing.T) {
	clearModelEnv(t)
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, preExecRoutingConfig)

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, "diff --git a/x b/x\n", routingAgent(), fakeJEV(jev.NewClearFake()), "run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if strings.Contains(stdout, "Task routing:") {
		t.Errorf("routing must be silent when disabled: %q", stdout)
	}
	if _, err := os.Stat(filepath.Join(dir, stateDirName, "runs", "T001", "routing.json")); err == nil {
		t.Error("routing.json written while the router is disabled")
	}
}

// TestRoutingSmallFromClearEvidence proves a clear, small task routes to the
// smallest configured class from typed JEV evidence.
func TestRoutingSmallFromClearEvidence(t *testing.T) {
	clearModelEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, preExecRoutingConfig)

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, "diff --git a/x b/x\n", routingAgent(), fakeJEV(jev.NewClearFake()), "run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "Task routing: small (qwen3:4b; isolated low-risk task)") {
		t.Errorf("stdout missing small routing line: %q", stdout)
	}

	art := readRoutingArtifact(t, dir)
	if art.Class != "small" || art.Source != runpkg.RoutingSourcePolicy {
		t.Fatalf("routing = %+v, want small/policy", art)
	}
	if art.Model != "qwen3:4b" {
		t.Errorf("model = %q, want qwen3:4b", art.Model)
	}
	if art.Signals == nil || !art.Signals.JEVAvailable {
		t.Errorf("signals should record available JEV evidence: %+v", art.Signals)
	}
	if len(art.Checkpoints) != 1 || art.Checkpoints[0] != "pre_execution" {
		t.Errorf("checkpoints = %v, want [pre_execution]", art.Checkpoints)
	}

	// The decision is reported too.
	if _, out, _ := runCLI(t, dir, "report", "T001"); !strings.Contains(out, "Model routing:") || !strings.Contains(out, "qwen3:4b") {
		t.Errorf("sop report missing routing: %q", out)
	}
}

// triageRoutingConfig enables ONLY the task-triage checkpoint, so routing is fed
// from triage evidence; findings below fail_on stay non-blocking.
const triageRoutingConfig = "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\nearly_jev:\n  enabled: true\n  mode: review\n  fail_on:\n    - critical\n  gates:\n    task_triage: true\n"

// TestRoutingLargeFromTriageEvidence proves the triage checkpoint's typed evidence
// can drive routing (triage evidence is used when pre-execution evidence is
// absent), and the informing checkpoint is recorded.
func TestRoutingLargeFromTriageEvidence(t *testing.T) {
	clearModelEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, triageRoutingConfig)

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, "diff --git a/x b/x\n", routingAgent(), fakeJEV(jev.NewScopeConcernFake()), "run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "cross-cutting change") {
		t.Errorf("stdout missing cross-cutting routing: %q", stdout)
	}
	art := readRoutingArtifact(t, dir)
	if art.Class != "large" {
		t.Fatalf("routing class = %q, want large", art.Class)
	}
	if len(art.Checkpoints) != 1 || art.Checkpoints[0] != "task_triage" {
		t.Errorf("checkpoints = %v, want [task_triage]", art.Checkpoints)
	}
}

// TestRoutingProviderFailureFallsBack proves a provider failure is never a
// finding: routing falls back to the safe default and records no evidence.
func TestRoutingProviderFailureFallsBack(t *testing.T) {
	clearModelEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, preExecRoutingConfig)

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, "diff --git a/x b/x\n", routingAgent(), fakeJEV(jev.NewProviderFailureFake()), "run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "JEV evidence unavailable; defaulting to medium") {
		t.Errorf("stdout missing fallback reason: %q", stdout)
	}
	art := readRoutingArtifact(t, dir)
	if art.Class != "medium" || len(art.Checkpoints) != 0 {
		t.Fatalf("routing = %+v, want medium with no evidence", art)
	}
	if art.Signals == nil || art.Signals.JEVAvailable {
		t.Errorf("signals = %+v, want JEVAvailable false", art.Signals)
	}
}

// TestRoutingMalformedEvidenceFallsBack proves malformed/invalid evidence fails
// closed to the safe default rather than being read as a clean result.
func TestRoutingMalformedEvidenceFallsBack(t *testing.T) {
	clearModelEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, preExecRoutingConfig)

	code, _, stderr := runInjectedCLIWithJEV(t, dir, "diff --git a/x b/x\n", routingAgent(), fakeJEV(jev.NewMalformedFake()), "run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if art := readRoutingArtifact(t, dir); art.Class != "medium" || len(art.Checkpoints) != 0 {
		t.Fatalf("routing = %+v, want medium with no usable evidence", art)
	}
}

// TestRoutingLargeFromCrossCuttingEvidence proves a cross-cutting finding routes to
// the largest class (escalation), and the task still completes normally.
func TestRoutingLargeFromCrossCuttingEvidence(t *testing.T) {
	clearModelEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, clearRoutingConfig)

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, "diff --git a/x b/x\n", routingAgent(), fakeJEV(jev.NewScopeConcernFake()), "run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "Task routing: large (deepseek-v4.1-flash:cloud; cross-cutting change)") {
		t.Errorf("stdout missing large routing line: %q", stdout)
	}
	if art := readRoutingArtifact(t, dir); art.Class != "large" {
		t.Fatalf("routing class = %q, want large", art.Class)
	}
}

// TestRoutingMediumWithoutEvidence proves the safe default: routing enabled but no
// early evidence routes to MEDIUM and records the fallback reason.
func TestRoutingMediumWithoutEvidence(t *testing.T) {
	clearModelEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, plainRoutingConfig)

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, "diff --git a/x b/x\n", routingAgent(), nil, "run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "Task routing: medium (glm-5.3-flash:cloud; JEV evidence unavailable; defaulting to medium)") {
		t.Errorf("stdout missing medium fallback line: %q", stdout)
	}
	art := readRoutingArtifact(t, dir)
	if art.Signals == nil || art.Signals.JEVAvailable {
		t.Errorf("signals should report unavailable JEV: %+v", art.Signals)
	}
	if len(art.Checkpoints) != 0 {
		t.Errorf("checkpoints = %v, want none", art.Checkpoints)
	}
}

// TestRoutingManualOverrideWins proves a manual --model-class override beats the
// automatic router and is recorded as a manual_override decision.
func TestRoutingManualOverrideWins(t *testing.T) {
	clearModelEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, preExecRoutingConfig)

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, "diff --git a/x b/x\n", routingAgent(), fakeJEV(jev.NewClearFake()), "run", "--task", "TASK.md", "--model-class", "large")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if !strings.Contains(stdout, "Task routing: large (deepseek-v4.1-flash:cloud; manual model-class override)") {
		t.Errorf("stdout missing manual-override line: %q", stdout)
	}
	art := readRoutingArtifact(t, dir)
	if art.Source != runpkg.RoutingSourceManual || art.Class != "large" {
		t.Fatalf("routing = %+v, want large/manual_override", art)
	}
}

// TestRoutingArtifactCarriesNoCredential proves the routing evidence stays
// non-secret, exactly like the model-selection evidence.
func TestRoutingArtifactCarriesNoCredential(t *testing.T) {
	clearModelEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	t.Setenv("SOP_LLAMACPP_API_KEY", "super-secret-token")
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, preExecRoutingConfig)

	code, _, stderr := runInjectedCLIWithJEV(t, dir, "diff --git a/x b/x\n", routingAgent(), fakeJEV(jev.NewClearFake()), "run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	data, err := os.ReadFile(filepath.Join(dir, stateDirName, "runs", "T001", "routing.json"))
	if err != nil {
		t.Fatal(err)
	}
	low := strings.ToLower(string(data))
	if strings.Contains(low, "secret") || strings.Contains(low, "api_key") || strings.Contains(low, "token") {
		t.Errorf("routing.json appears to carry a credential: %s", data)
	}
}
