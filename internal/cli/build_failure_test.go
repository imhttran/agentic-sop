package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/failure"
	"github.com/imhttran/agentic-sop/internal/jev"
)

// CTRL010 regression: a deterministic build failure introduced by an in-progress
// edit must classify as a compiler error (AUTO_FIX), never UNKNOWN, even when the
// agent's own outcome is a bare failed. Structured validation evidence outranks
// free-form agent prose, and JEV does not run while the basic build is red.

// brokenBuildConfig fails until the FIX agent writes fixed.marker, and reports the
// exact compiler error from the CTRL010 dogfood run on stderr.
const brokenBuildConfig = "project:\n  name: x\nvalidation:\n  build:\n" +
	"    - \"test -f fixed.marker || { echo 'internal/sopclient/checkpoint_read.go:100:16: undefined: checkpointFromDetail' >&2; exit 1; }\"\n"

const repairMarker = "fixed.marker"

// scriptedFix is one scripted FIX invocation: it optionally writes the repair
// marker (so the build passes afterward) and reports the given outcome.
type scriptedFix struct {
	repair  bool
	outcome *agent.Outcome
}

// scriptedFixAgent implements a task whose build stays red until a scripted FIX
// repairs it. The last scripted step repeats, so a sequence of non-repairing fixes
// exhausts the fix budget.
type scriptedFixAgent struct {
	dir         string
	steps       []scriptedFix
	implOutcome *agent.Outcome // defaults to a completed implementation
	fixCalls    int
}

func (a *scriptedFixAgent) Generate(_ context.Context, r agent.Request) (agent.Response, error) {
	switch r.Capability {
	case agent.Plan:
		return agent.Response{Content: validPlanJSON}, nil
	case agent.Implement:
		outcome := a.implOutcome
		if outcome == nil {
			outcome = &agent.Outcome{Status: agent.OutcomeCompleted, ChangesExpected: true}
		}
		return agent.Response{Content: "impl", Outcome: outcome}, nil
	case agent.Fix:
		i := a.fixCalls
		if i >= len(a.steps) {
			i = len(a.steps) - 1
		}
		step := a.steps[i]
		a.fixCalls++
		if step.repair {
			if err := os.WriteFile(filepath.Join(a.dir, repairMarker), []byte("ok"), 0o644); err != nil {
				return agent.Response{}, err
			}
		}
		return agent.Response{Content: "fix", Outcome: step.outcome}, nil
	case agent.Review:
		return agent.Response{Content: `{"summary":"clean","findings":[]}`}, nil
	default:
		return agent.Response{Content: "{}"}, nil
	}
}

// TestRunNonCompletedFixWithRedBuildIsAutoFixed is the CTRL010 dogfood regression:
// IMPLEMENT leaves the build red, a FIX returns a bare non-completed failed outcome,
// and SOP re-validates instead of trusting the prose. The deterministic compiler
// error drives another bounded fix, which repairs the tree, and the run completes
// with no human boundary.
func TestRunNonCompletedFixWithRedBuildIsAutoFixed(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, brokenBuildConfig)
	a := &scriptedFixAgent{dir: dir, steps: []scriptedFix{
		{outcome: &agent.Outcome{Status: agent.OutcomeFailed}}, // in-progress edit, bare failed
		{repair: true, outcome: &agent.Outcome{Status: agent.OutcomeCompleted, ChangesExpected: true}},
	}}

	code, stdout, stderr := runInjectedCLI(t, dir, "diff\n", a, "run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if strings.Contains(stdout, "NEEDS_HUMAN") || strings.Contains(stdout, "UNKNOWN") {
		t.Errorf("a repair-able build failure must not be a human boundary or UNKNOWN:\n%s", stdout)
	}
	if !strings.Contains(stdout, "fix cycles: 2/3") {
		t.Errorf("stdout = %q, want fix cycles: 2/3 (the bare failed fix was not the last word)", stdout)
	}
}

// TestRunNonCompletedFixExhaustsBudgetBoundedNotUnknown proves the bounded terminal
// case: when the deterministic build failure survives the fix budget, the
// classification is AUTO_FIX_EXHAUSTED with the compiler diagnostic preserved — not
// UNKNOWN, and the fix-cycle accounting is the invocation's own count, not zero.
func TestRunNonCompletedFixExhaustsBudgetBoundedNotUnknown(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, brokenBuildConfig)
	a := &scriptedFixAgent{dir: dir, steps: []scriptedFix{
		{outcome: &agent.Outcome{Status: agent.OutcomeFailed}}, // never repairs
	}}

	code, stdout, _ := runInjectedCLI(t, dir, "diff\n", a, "run", "--task", "TASK.md")
	if code != exitError {
		t.Fatalf("code=%d, want %d", code, exitError)
	}
	if strings.Contains(stdout, "UNKNOWN") {
		t.Errorf("stdout = %q, a deterministic build failure must not be UNKNOWN", stdout)
	}
	if !strings.Contains(stdout, "fix cycles: 3/3") {
		t.Errorf("stdout = %q, want fix cycles: 3/3", stdout)
	}
	if !strings.Contains(stdout, "checkpointFromDetail") {
		t.Errorf("stdout = %q, want the compiler diagnostic preserved", stdout)
	}

	cls := readClassification(t, dir, "T001")
	if cls.Kind != failure.AutoFixExhausted {
		t.Errorf("classification kind = %s, want AUTO_FIX_EXHAUSTED (never UNKNOWN)", cls.Kind)
	}
	if !strings.Contains(cls.Reason, "checkpointFromDetail") {
		t.Errorf("classification reason = %q, want the compiler diagnostic", cls.Reason)
	}

	// The fix-cycle count is the same authoritative value in the summary, the report,
	// and the performance record: the non-completed-outcome path must not drop it.
	if got := readReportFixCycles(t, dir, "T001"); got != 3 {
		t.Errorf("report fix_cycles = %d, want 3", got)
	}
	if !strings.Contains(stdout, "fix cycles 3") {
		t.Errorf("stdout = %q, want the performance line to report 3 fix cycles", stdout)
	}
}

// TestRunBuildFailureLowAutonomyClassifiesCompilerError proves a deterministic build
// failure is classified by the evidence (COMPILER_ERROR / AUTO_FIX) even when the
// autonomy level withholds the fix: a conservative policy stops for a human but
// never mislabels the failure UNKNOWN.
func TestRunBuildFailureLowAutonomyClassifiesCompilerError(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, failingBuildLow)
	a := &fakeCapabilityAgent{plan: validPlanJSON, impl: "x", fix: "x", review: `{"summary":"clean","findings":[]}`}

	code, stdout, _ := runInjectedCLI(t, dir, "diff\n", a, "run", "--task", "TASK.md")
	if code != exitError {
		t.Fatalf("code=%d, want %d", code, exitError)
	}
	if strings.Contains(stdout, "UNKNOWN") {
		t.Errorf("stdout = %q, want no UNKNOWN for a deterministic build failure", stdout)
	}
	if !strings.Contains(stdout, "classification: AUTO_FIX (COMPILER_ERROR") {
		t.Errorf("stdout = %q, want AUTO_FIX (COMPILER_ERROR", stdout)
	}
	if !strings.Contains(stdout, "decision=HUMAN_APPROVAL_REQUIRED") {
		t.Errorf("stdout = %q, want the conservative level to withhold the fix for a human", stdout)
	}
	cls := readClassification(t, dir, "T001")
	if cls.Disposition != failure.AutoFix {
		t.Errorf("classification disposition = %s, want AUTO_FIX", cls.Disposition)
	}
}

// TestRunRedBuildSkipsJEVUntilGreen proves the JEV ordering: while the basic build is
// red, the (expensive) analysis does not run; once a fix makes the build green, JEV
// runs exactly once at the quality seam.
func TestRunRedBuildSkipsJEVUntilGreen(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, brokenBuildConfig+"quality:\n  jev:\n    enabled: true\n    mode: review\n")
	a := &scriptedFixAgent{dir: dir, steps: []scriptedFix{
		{repair: true, outcome: &agent.Outcome{Status: agent.OutcomeCompleted, ChangesExpected: true}},
	}}
	analyzer := &sequencedJEV{results: []jev.Result{{Status: jev.StatusPass}}}

	code, stdout, stderr := runInjectedCLIWithJEV(t, dir, jevDiff, a,
		func(config.Config) (jev.Analyzer, error) { return analyzer, nil },
		"run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if analyzer.calls != 1 {
		t.Errorf("JEV invocations = %d, want 1 (only after the build turned green)", analyzer.calls)
	}
	if !strings.Contains(stdout, "fix cycles: 1/3") {
		t.Errorf("stdout = %q, want fix cycles: 1/3", stdout)
	}
}

// TestRunNonCompletedImplementWithRedBuildIsAutoFixed is the task's stated dogfood
// regression: IMPLEMENT itself leaves the build red and reports a bare non-completed
// failed outcome. SOP classifies from the compiler evidence, fixes it, and passes.
func TestRunNonCompletedImplementWithRedBuildIsAutoFixed(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, brokenBuildConfig)
	a := &scriptedFixAgent{
		dir:         dir,
		implOutcome: &agent.Outcome{Status: agent.OutcomeFailed},
		steps: []scriptedFix{
			{repair: true, outcome: &agent.Outcome{Status: agent.OutcomeCompleted, ChangesExpected: true}},
		},
	}

	code, stdout, stderr := runInjectedCLI(t, dir, "diff\n", a, "run", "--task", "TASK.md")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s stdout=%s", code, stderr, stdout)
	}
	if strings.Contains(stdout, "NEEDS_HUMAN") || strings.Contains(stdout, "UNKNOWN") {
		t.Errorf("a deterministic build failure must be auto-fixed, not a human boundary or UNKNOWN:\n%s", stdout)
	}
	if !strings.Contains(stdout, "fix cycles: 1/3") {
		t.Errorf("stdout = %q, want fix cycles: 1/3", stdout)
	}
}

// TestRunNonCompletedFixExhaustionIsTerminalUnderHigh is dogfood scenario C: a
// repeated non-progressing repair is bounded and terminates — under HIGH as a
// terminal automation state, never NEEDS_HUMAN.
func TestRunNonCompletedFixExhaustionIsTerminalUnderHigh(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "TASK.md", runTaskFile)
	writeConfig(t, dir, brokenBuildConfig+"autonomy:\n  level: high\n")
	a := &scriptedFixAgent{dir: dir, steps: []scriptedFix{
		{outcome: &agent.Outcome{Status: agent.OutcomeFailed}}, // never repairs
	}}

	code, stdout, _ := runInjectedCLI(t, dir, "diff\n", a, "run", "--task", "TASK.md")
	if code != exitError {
		t.Fatalf("code=%d, want %d", code, exitError)
	}
	if strings.Contains(stdout, "NEEDS_HUMAN") {
		t.Errorf("HIGH exhaustion must not require a human:\n%s", stdout)
	}
	if !strings.Contains(stdout, "decision=TERMINAL") {
		t.Errorf("stdout = %q, want a terminal automation decision", stdout)
	}
	cls := readClassification(t, dir, "T001")
	if cls.Kind != failure.AutoFixExhausted {
		t.Errorf("classification kind = %s, want AUTO_FIX_EXHAUSTED", cls.Kind)
	}
}

// readReportFixCycles reads the fix-cycle count from a run's report.json.
func readReportFixCycles(t *testing.T, dir, id string) int {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, stateDirName, "runs", id, "report.json"))
	if err != nil {
		t.Fatalf("read report.json: %v", err)
	}
	var doc struct {
		FixCycles int `json:"fix_cycles"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse report.json: %v", err)
	}
	return doc.FixCycles
}
