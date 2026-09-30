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

// These tests bind the Phase 3.5 acceptance criteria S5: routing only selects a
// model, so the gate/approval decision flow must be byte-identical regardless of
// the routed class, and no gate-bypass path may be reachable from the routing
// decision.
//
// The strongest form of that claim is a differential regression: run the same
// task with the router OFF (the pre-integration selection) and with the router ON
// (a different class resolved), and assert the gate outcome, the reasons, the
// validation result, the review findings, the fix-cycle count, and the exit code
// are all identical. The ONLY permitted difference is the recorded model class,
// which is the model choice routing is allowed to make.

// gateSnapshot is the gate/approval-relevant projection of a run: everything the
// lifecycle decided about whether the change may proceed. It deliberately excludes
// the model class/provider/model, which routing is allowed to change.
type gateSnapshot struct {
	code       int
	decision   string
	reasons    []string
	validation []testrunnerResultLite
	findings   []string
	fixCycles  int
	stage      string
}

type testrunnerResultLite struct {
	Category string
	Command  string
	Status   string
}

// snapshotGate runs the task file once against the given config/environment and
// returns the gate-relevant projection plus the raw stdout for routing assertions.
func snapshotGate(t *testing.T, dir, cfg string, args ...string) (gateSnapshot, string) {
	t.Helper()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, cfg)

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, "diff --git a/x b/x\n", routingAgent(), fakeJEV(jev.NewClearFake()), args...)
	_ = stderr

	snap := gateSnapshot{code: code}
	for _, line := range strings.Split(stdout, "\n") {
		switch {
		case strings.HasPrefix(line, "run T001: "):
			snap.decision = strings.TrimPrefix(line, "run T001: ")
		case strings.HasPrefix(line, "fix cycles: "):
			snap.fixCycles = parseFixCycles(line)
		case strings.HasPrefix(line, "  - "):
			snap.reasons = append(snap.reasons, strings.TrimPrefix(line, "  - "))
		}
	}

	// The persisted report.json is the durable record of the gate decision, so the
	// differential reads the same artifact an auditor would.
	var doc runReportDoc
	readRunJSON(t, filepath.Join(dir, stateDirName, "runs", "T001", "report.json"), &doc)
	snap.stage = string(doc.Stage)
	if snap.decision == "" {
		snap.decision = string(doc.Decision)
	}
	if len(snap.reasons) == 0 {
		snap.reasons = doc.Reasons
	}
	for _, r := range doc.Validation {
		snap.validation = append(snap.validation, testrunnerResultLite{Category: string(r.Category), Command: r.Command, Status: string(r.Status)})
	}
	for _, f := range doc.Findings {
		snap.findings = append(snap.findings, string(f.Severity)+" "+f.Title)
	}
	return snap, stdout
}

// parseFixCycles reads the "fix cycles: n/m" line.
func parseFixCycles(line string) int {
	v := strings.TrimPrefix(line, "fix cycles: ")
	if i := strings.IndexByte(v, '/'); i > 0 {
		v = v[:i]
	}
	n := 0
	for _, r := range v {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// assertSameGate fails when two gate snapshots differ in any gate-relevant field.
func assertSameGate(t *testing.T, disabled, enabled gateSnapshot) {
	t.Helper()
	if disabled.code != enabled.code {
		t.Errorf("exit code differs: disabled=%d enabled=%d", disabled.code, enabled.code)
	}
	if disabled.decision != enabled.decision {
		t.Errorf("gate decision differs: disabled=%q enabled=%q", disabled.decision, enabled.decision)
	}
	if strings.Join(disabled.reasons, "|") != strings.Join(enabled.reasons, "|") {
		t.Errorf("gate reasons differ:\n disabled=%v\n enabled=%v", disabled.reasons, enabled.reasons)
	}
	if disabled.fixCycles != enabled.fixCycles {
		t.Errorf("fix cycles differ: disabled=%d enabled=%d", disabled.fixCycles, enabled.fixCycles)
	}
	if disabled.stage != enabled.stage {
		t.Errorf("run stage differs: disabled=%q enabled=%q", disabled.stage, enabled.stage)
	}
	if len(disabled.validation) != len(enabled.validation) {
		t.Errorf("validation result count differs: disabled=%d enabled=%d", len(disabled.validation), len(enabled.validation))
	} else {
		for i := range disabled.validation {
			if disabled.validation[i] != enabled.validation[i] {
				t.Errorf("validation result %d differs: disabled=%+v enabled=%+v", i, disabled.validation[i], enabled.validation[i])
			}
		}
	}
	if strings.Join(disabled.findings, "|") != strings.Join(enabled.findings, "|") {
		t.Errorf("review findings differ:\n disabled=%v\n enabled=%v", disabled.findings, enabled.findings)
	}
}

// TestRoutingDoesNotAffectGateDecision is the core S5 regression: a passing task
// reaches the same PASS gate whether the router is off or on (and thus whether an
// implementation model was auto-selected). Routing changed the model class only.
func TestRoutingDoesNotAffectGateDecision(t *testing.T) {
	clearModelEnv(t)

	disabledDir := t.TempDir()
	disabled, disabledOut := snapshotGate(t, disabledDir, preExecRoutingConfig, "run", "--task", "TASK.md")
	if strings.Contains(disabledOut, "Task routing:") {
		t.Fatalf("router-off run must not record a routing decision: %q", disabledOut)
	}

	enabledDir := t.TempDir()
	t.Setenv(model.EnvRoutingEnabled, "true")
	enabled, enabledOut := snapshotGate(t, enabledDir, preExecRoutingConfig, "run", "--task", "TASK.md")
	if !strings.Contains(enabledOut, "Task routing: small (") {
		t.Fatalf("router-on run must record the routed class: %q", enabledOut)
	}

	// The model class differs (routing did its one job)...
	disabledSel, err := os.ReadFile(filepath.Join(disabledDir, stateDirName, "runs", "T001", "model-selection.json"))
	if err != nil {
		// A router-off run with routing inactive writes no model-selection artifact.
		disabledSel = nil
	}
	enabledSel, err := os.ReadFile(filepath.Join(enabledDir, stateDirName, "runs", "T001", "model-selection.json"))
	if err != nil {
		t.Fatalf("router-on run must persist the model selection: %v", err)
	}
	if string(disabledSel) == string(enabledSel) {
		t.Errorf("routing did not change the model class, so the differential proves nothing")
	}

	// ...but every gate-relevant outcome is identical.
	assertSameGate(t, disabled, enabled)
}

// TestRoutingDoesNotAffectGateDecisionOnFailure proves the same invariance when
// the gate does NOT pass: a task that exhausts its fix budget and stops at the
// human boundary reaches the identical NEEDS_HUMAN/FAIL outcome regardless of the
// routed class. Routing must never turn a blocked gate into a passing one.
func TestRoutingDoesNotAffectGateDecisionOnFailure(t *testing.T) {
	clearModelEnv(t)
	// A review that always reports a critical finding: the fix loop exhausts and
	// the gate stops at a human boundary. Same config for both runs.
	const failConfig = "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\nearly_jev:\n  enabled: true\n  mode: review\n  gates:\n    pre_execution: true\n"
	agentWithCritical := func() *fakeCapabilityAgent {
		return &fakeCapabilityAgent{
			plan:   validPlanJSON,
			impl:   "changed files",
			fix:    "still broken",
			review: `{"summary":"bad","findings":[{"severity":"CRITICAL","title":"boom","file":"a.go","line":1}]}`,
		}
	}

	disabledDir := t.TempDir()
	writeFile(t, disabledDir, "TASK.md", runTaskFile)
	writeConfig(t, disabledDir, failConfig)
	dCode, dOut, dErr := runInjectedCLIWithJEV(t, disabledDir, "diff --git a/x b/x\n", agentWithCritical(), fakeJEV(jev.NewClearFake()), "run", "--task", "TASK.md")
	if dCode != exitError {
		t.Fatalf("disabled run: code=%d stderr=%s stdout=%s", dCode, dErr, dOut)
	}

	t.Setenv(model.EnvRoutingEnabled, "true")
	enabledDir := t.TempDir()
	writeFile(t, enabledDir, "TASK.md", runTaskFile)
	writeConfig(t, enabledDir, failConfig)
	eCode, eOut, eErr := runInjectedCLIWithJEV(t, enabledDir, "diff --git a/x b/x\n", agentWithCritical(), fakeJEV(jev.NewClearFake()), "run", "--task", "TASK.md")
	if eCode != exitError {
		t.Fatalf("enabled run: code=%d stderr=%s stdout=%s", eCode, eErr, eOut)
	}

	// Both runs must stop at the human boundary; routing never converts it.
	if !strings.Contains(dOut, "NEEDS_HUMAN") || !strings.Contains(eOut, "NEEDS_HUMAN") {
		t.Fatalf("expected NEEDS_HUMAN in both runs:\ndisabled=%q\nenabled=%q", dOut, eOut)
	}

	var dDoc, eDoc runReportDoc
	readRunJSON(t, filepath.Join(disabledDir, stateDirName, "runs", "T001", "report.json"), &dDoc)
	readRunJSON(t, filepath.Join(enabledDir, stateDirName, "runs", "T001", "report.json"), &eDoc)
	if dDoc.Decision != eDoc.Decision {
		t.Errorf("gate decision differs on failure: disabled=%q enabled=%q", dDoc.Decision, eDoc.Decision)
	}
	if dDoc.FixCycles != eDoc.FixCycles {
		t.Errorf("fix cycles differ on failure: disabled=%d enabled=%d", dDoc.FixCycles, eDoc.FixCycles)
	}
	if dDoc.Stage != eDoc.Stage {
		t.Errorf("stage differs on failure: disabled=%q enabled=%q", dDoc.Stage, eDoc.Stage)
	}
}

// TestRoutingOutputNeverMutatesTaskState proves the routing decision is confined
// to the model choice: the task's terminal state and the run report's gate
// decision do not depend on whether routing ran. The routing artifact is written
// alongside, never instead of, the authoritative gate artifacts.
func TestRoutingOutputNeverMutatesTaskState(t *testing.T) {
	clearModelEnv(t)

	disabledDir := t.TempDir()
	disabled, _ := snapshotGate(t, disabledDir, preExecRoutingConfig, "run", "--task", "TASK.md")

	enabledDir := t.TempDir()
	t.Setenv(model.EnvRoutingEnabled, "true")
	enabled, _ := snapshotGate(t, enabledDir, preExecRoutingConfig, "run", "--task", "TASK.md")

	if disabled.stage != enabled.stage || disabled.decision != enabled.decision {
		t.Fatalf("routing changed the task's terminal state: disabled=%q/%q enabled=%q/%q",
			disabled.stage, disabled.decision, enabled.stage, enabled.decision)
	}

	// The routing artifact, when present, is evidence only: the authoritative gate
	// decision lives in report.json, which is unchanged by it.
	var doc runReportDoc
	readRunJSON(t, filepath.Join(enabledDir, stateDirName, "runs", "T001", "report.json"), &doc)
	if doc.Routing == nil {
		t.Fatal("router-on run should record a routing section in report.json")
	}
	if string(doc.Decision) != string(map[bool]string{true: string(doc.Decision)}[true]) {
		t.Fatal("unreachable")
	}
	if string(doc.Decision) == "" {
		t.Fatalf("routing run has no gate decision: %+v", doc)
	}
}

// TestRoutingDisabledIsStrictNoop binds the "disabled router is a strict no-op"
// criterion at the composition seam: with routing off, the lifecycle writes no
// routing artifact and no model-selection artifact, so the pre-integration
// selection is retained byte for byte.
func TestRoutingDisabledIsStrictNoop(t *testing.T) {
	clearModelEnv(t)
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, preExecRoutingConfig)

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, "diff --git a/x b/x\n", routingAgent(), fakeJEV(jev.NewClearFake()), "run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if strings.Contains(stdout, "Task routing:") {
		t.Errorf("router-off run must not print a routing line: %q", stdout)
	}
	runDir := filepath.Join(dir, stateDirName, "runs", "T001")
	for _, name := range []string{"routing.json", "model-selection.json"} {
		if _, err := os.Stat(filepath.Join(runDir, name)); err == nil {
			t.Errorf("router-off run wrote %s; a disabled router must be a strict no-op", name)
		}
	}

	var doc runReportDoc
	readRunJSON(t, filepath.Join(runDir, "report.json"), &doc)
	if doc.Routing != nil {
		t.Errorf("router-off report must omit the routing section: %+v", doc.Routing)
	}
	if doc.ModelSelection != nil {
		t.Errorf("router-off report must omit the model-selection section: %+v", doc.ModelSelection)
	}
}

// TestManualOverrideBeatsRouterAndSelectsConcreteModel binds two criteria at the
// composition seam: a manual override wins over the automatic router, and the
// resolved class is a concrete provider/model. With the router on and evidence
// that would route to SMALL, an explicit --model-class large must yield LARGE
// resolved to the built-in large model (not the routed class).
func TestManualOverrideBeatsRouterAndSelectsConcreteModel(t *testing.T) {
	clearModelEnv(t)
	t.Setenv(model.EnvRoutingEnabled, "true")
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, preExecRoutingConfig)

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, "diff --git a/x b/x\n", routingAgent(), fakeJEV(jev.NewClearFake()), "run", "--task", "TASK.md", "--model-class", "large")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	art := readRoutingArtifact(t, dir)
	if art.Class != "large" {
		t.Errorf("class = %q, want large (the manual override)", art.Class)
	}
	if art.Source != runpkg.RoutingSourceManual {
		t.Errorf("source = %q, want manual_override", art.Source)
	}
	if art.Model == "" || art.Provider == "" {
		t.Errorf("a routed class must resolve to a concrete model: %+v", art)
	}
}

// TestRouterEmitsOnlyKnownClasses proves every class the router can emit resolves
// to a concrete provider/model, so no routed class can strand the lifecycle
// without a usable model. It uses the model layer directly for all three classes.
func TestRouterEmitsOnlyKnownClasses(t *testing.T) {
	for _, c := range model.Classes {
		res, err := model.Resolve(model.Inputs{Config: config.Config{}.Models, RoutedClass: c, RoutedReason: "test"})
		if err != nil {
			t.Errorf("class %s: Resolve error: %v", c, err)
			continue
		}
		if !res.Active {
			t.Errorf("class %s: routing must be active when a routed class is set", c)
			continue
		}
		if res.Selection.Provider == "" || res.Selection.Model == "" {
			t.Errorf("class %s: resolved to an incomplete model: %+v", c, res.Selection)
		}
		if res.Selection.Source != model.SourceRouter {
			t.Errorf("class %s: source = %q, want router", c, res.Selection.Source)
		}
	}
}
