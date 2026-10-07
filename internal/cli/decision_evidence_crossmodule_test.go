package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/activity"
	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/autonomy"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/decision"
	"github.com/imhttran/agentic-sop/internal/decision/command"
	"github.com/imhttran/agentic-sop/internal/failure"
	"github.com/imhttran/agentic-sop/internal/taskfile"
)

// SEAM-006 cross-module / end-to-end proof.
//
// These tests drive the REAL out-of-module fixture (testdata/fake-decision-provider,
// its own Go module, stdlib only) through the REAL process adapter
// (internal/decision/command) and the REAL seam (applyDecisionEvidence), so the
// proof exercises the actual process boundary rather than an in-process double.
// The fixture imports no agentic-sop package, so it stands in for any external
// adapter; the same JSON protocol is all that crosses.

// crossModuleFixDir is the process-lifetime directory holding the built
// out-of-module fixture. TestMain builds it once and removes it after the run.
var crossModuleFixDir string

// TestMain builds the out-of-module fake provider once for the whole test binary.
// A missing Go toolchain is not fatal: the cross-module tests skip.
func TestMain(m *testing.M) {
	if dir, err := buildFakeProvider(); err == nil {
		crossModuleFixDir = dir
	}
	code := m.Run()
	if crossModuleFixDir != "" {
		_ = os.RemoveAll(crossModuleFixDir)
	}
	os.Exit(code)
}

// buildFakeProvider compiles testdata/fake-decision-provider — its own Go module,
// stdlib only — into a temporary directory for the test process. It uses the
// offline toolchain (GOTOOLCHAIN=local) so no download is required.
func buildFakeProvider() (string, error) {
	goBin, err := exec.LookPath("go")
	if err != nil {
		return "", err
	}
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "", fmt.Errorf("cannot locate the test source file")
	}
	root := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(root)
		if parent == root {
			return "", fmt.Errorf("no go.mod found above the test file")
		}
		root = parent
	}
	dir, err := os.MkdirTemp("", "fake-decision-provider-")
	if err != nil {
		return "", err
	}
	build := exec.Command(goBin, "build", "-o", filepath.Join(dir, "fake-decision-provider"), ".")
	build.Dir = filepath.Join(root, "testdata", "fake-decision-provider")
	build.Env = append(os.Environ(), "GOTOOLCHAIN=local")
	if out, err := build.CombinedOutput(); err != nil {
		_ = os.RemoveAll(dir)
		return "", fmt.Errorf("build fake provider: %v: %s", err, out)
	}
	return dir, nil
}

// crossModuleBinary returns the fixture executable path, skipping when unavailable.
func crossModuleBinary(t *testing.T) string {
	t.Helper()
	if crossModuleFixDir == "" {
		t.Skip("out-of-module fake provider unavailable (no Go toolchain)")
	}
	return filepath.Join(crossModuleFixDir, "fake-decision-provider")
}

// crossModuleAdapter constructs the real process adapter over the fixture.
func crossModuleAdapter(t *testing.T, name, mode string, extra ...string) decision.Provider {
	t.Helper()
	argv := append([]string{crossModuleBinary(t), mode}, extra...)
	p, err := command.New(name, argv)
	if err != nil {
		t.Fatalf("command.New: %v", err)
	}
	return p
}

// crossModuleRun applies the real seam with a real process provider.
func crossModuleRun(t *testing.T, cfg config.Config, p decision.Provider, spec *taskfile.Spec, cls failure.Classification) lifeResult {
	t.Helper()
	return applyDecisionEvidence(context.Background(), cfg, decisionEvidenceDeps(p, nil), spec, decisionEvidenceResult(cls))
}

// TestCrossModuleProtocolShape proves the request that crosses the process boundary
// is exactly the documented provider-neutral DTO, and the fixture returns a result
// the seam validates. It captures the raw bytes the fixture received on stdin.
func TestCrossModuleProtocolShape(t *testing.T) {
	capture := filepath.Join(t.TempDir(), "request.json")
	p := crossModuleAdapter(t, "fake-external", "decide", capture)

	spec := &taskfile.Spec{ID: "T1", Title: "Security hardening", Description: "harden auth", AcceptanceCriteria: []string{"a", "b"}}
	cls := autoFixClassification()
	req := decisionEvidenceRequest(spec, cls)

	dec, err := p.Decide(context.Background(), req)
	if err != nil {
		t.Fatalf("fixture decide: %v", err)
	}
	if err := decision.Validate(dec); err != nil {
		t.Fatalf("fixture must return valid, SOP-recognized evidence: %v", err)
	}
	if dec.Choice != decision.High {
		t.Fatalf("adapter-like fixture chose %q, want HIGH for a risky subject", dec.Choice)
	}

	raw, err := os.ReadFile(capture)
	if err != nil {
		t.Fatalf("fixture did not capture the request: %v", err)
	}
	var wire struct {
		ContractVersion int                `json:"contract_version"`
		Kind            string             `json:"kind"`
		Question        string             `json:"question"`
		Signals         map[string]float64 `json:"signals"`
		Choices         []string           `json:"choices"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatalf("the request that crossed the boundary is not the documented JSON: %v (%s)", err, raw)
	}
	if wire.ContractVersion != decision.ContractVersion {
		t.Errorf("contract_version = %d, want %d", wire.ContractVersion, decision.ContractVersion)
	}
	if wire.Kind != decisionEvidenceKind {
		t.Errorf("kind = %q, want %q", wire.Kind, decisionEvidenceKind)
	}
	if !strings.Contains(wire.Question, "Security hardening") {
		t.Errorf("question = %q, want the bounded subject", wire.Question)
	}
	if len(wire.Choices) != 4 {
		t.Errorf("choices = %v, want the closed four-choice set", wire.Choices)
	}
	if _, ok := wire.Signals["criteria"]; !ok {
		t.Errorf("signals = %v, want the bounded criteria signal", wire.Signals)
	}
	// The protocol carries no lifecycle, approval, commit, or merge field.
	for _, forbidden := range []string{"task", "state", "approval", "commit", "merge", "action"} {
		if strings.Contains(strings.ToLower(string(raw)), `"`+forbidden) {
			t.Errorf("request DTO leaked a %q field across the boundary: %s", forbidden, raw)
		}
	}
}

// TestCrossModuleEndToEndScenarios runs every fixture behavior through the real
// process boundary and the real seam, pinning the governed effect.
func TestCrossModuleEndToEndScenarios(t *testing.T) {
	cfg := decisionEvidenceConfig(true)
	benign := benignSpec()
	risky := &taskfile.Spec{ID: "T1", Title: "Security hardening of auth"}

	for _, tc := range []struct {
		name      string
		mode      string
		spec      *taskfile.Spec
		wantHuman bool
	}{
		{"valid benign LOW", "low", benign, false},
		{"valid adverse HIGH", "high", benign, true},
		{"valid adverse HUMAN", "human", benign, true},
		{"valid confidence zero (benign)", "conf-zero", benign, false},
		{"valid confidence one (benign)", "conf-one", benign, false},
		{"adapter-like benign subject", "decide", benign, false},
		{"adapter-like risky subject", "decide", risky, true},
		{"unsupported capability", "unsupported", benign, false},
		{"indeterminate", "indeterminate", benign, false},
		{"unknown choice", "unknown-choice", benign, false},
		{"invalid confidence", "bad-confidence", benign, false},
		{"malformed response", "malformed", benign, false},
		{"empty response", "empty", benign, false},
		{"non-zero exit", "fail", benign, false},
		{"crash", "panic", benign, false},
		{"hostile diagnostics", "hostile", benign, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := crossModuleAdapter(t, "fake-external", tc.mode)
			in := decisionEvidenceResult(autoFixClassification())
			out := crossModuleRun(t, cfg, p, tc.spec, autoFixClassification())

			if out.classification != in.classification {
				t.Fatalf("classification must be unchanged: %+v", out.classification)
			}
			if tc.wantHuman {
				if !out.decision.RequiresHuman || out.decision.Action != autonomy.ActionHumanApproval {
					t.Fatalf("expected added attention, got %+v", out.decision)
				}
			} else if !reflect.DeepEqual(out.decision, in.decision) {
				t.Fatalf("expected the governed baseline unchanged:\n got %+v\nwant %+v", out.decision, in.decision)
			}
			if permitsExecution(out.decision) && !permitsExecution(in.decision) {
				t.Fatalf("evidence made a governed result permitting: %+v", out.decision)
			}
		})
	}
}

// TestCrossModuleAuthorityLattice is the end-to-end monotonic-authority proof
// through the real process adapter: Block and Human baselines are never weakened to
// Continue, and a failure never authorizes.
func TestCrossModuleAuthorityLattice(t *testing.T) {
	cfg := decisionEvidenceConfig(true)
	baselines := map[string]failure.Classification{
		"continue": {Kind: failure.IncompleteImplementation, Disposition: failure.Continue},
		"block":    {Kind: failure.NoProgress, Disposition: failure.Retry},
		"human":    {Kind: failure.ApprovalRequired, Disposition: failure.NeedsHuman},
	}
	for bname, cls := range baselines {
		base := decisionEvidenceResult(cls).decision
		for _, mode := range []string{"low", "high", "human", "fail", "malformed", "unknown-choice"} {
			out := crossModuleRun(t, cfg, crossModuleAdapter(t, "fake-external", mode), benignSpec(), cls)
			if permitsExecution(out.decision) && !permitsExecution(base) {
				t.Fatalf("[%s/%s] governed boundary weakened through the process boundary: %+v", bname, mode, out.decision)
			}
			if bname != "continue" && out.decision.Action == autonomy.ActionAutoContinue {
				t.Fatalf("[%s/%s] became Continue: %+v", bname, mode, out.decision)
			}
			if out.classification != cls {
				t.Fatalf("[%s/%s] classification rewritten: %+v", bname, mode, out.classification)
			}
		}
	}
}

// TestCrossModuleProviderIdentityIndependence proves two differently-identified
// providers returning identical evidence yield identical policy, and that provider
// metadata/diagnostic variation with the same choice also yields identical policy.
func TestCrossModuleProviderIdentityIndependence(t *testing.T) {
	cfg := decisionEvidenceConfig(true)
	a := crossModuleRun(t, cfg, crossModuleAdapter(t, "provider-a", "high"), benignSpec(), autoFixClassification())
	b := crossModuleRun(t, cfg, crossModuleAdapter(t, "provider-b", "high"), benignSpec(), autoFixClassification())
	if !reflect.DeepEqual(a.decision, b.decision) {
		t.Fatalf("provider identity changed policy:\n a=%+v\n b=%+v", a.decision, b.decision)
	}
	if !a.decision.RequiresHuman {
		t.Fatalf("expected an added human boundary: %+v", a.decision)
	}

	// The same choice (HIGH) with different provider metadata/diagnostics must yield
	// the same policy: diagnostics are advisory data, never policy input.
	risky := &taskfile.Spec{ID: "T1", Title: "Security hardening of auth"}
	c := crossModuleRun(t, cfg, crossModuleAdapter(t, "provider-c", "decide"), risky, autoFixClassification())
	if !reflect.DeepEqual(a.decision, c.decision) {
		t.Fatalf("provider metadata changed policy:\n a=%+v\n c=%+v", a.decision, c.decision)
	}
}

// TestCrossModuleModelIndependence proves the same evidence yields the same decision
// regardless of the execution-model routing context.
func TestCrossModuleModelIndependence(t *testing.T) {
	cfg := decisionEvidenceConfig(true)
	plain := decisionEvidenceDeps(crossModuleAdapter(t, "fake-external", "high"), nil)
	modeled := plain
	modeled.modelClass = "large"
	modeled.routingEnabled = true

	a := applyDecisionEvidence(context.Background(), cfg, plain, benignSpec(), decisionEvidenceResult(autoFixClassification()))
	b := applyDecisionEvidence(context.Background(), cfg, modeled, benignSpec(), decisionEvidenceResult(autoFixClassification()))
	if !reflect.DeepEqual(a.decision, b.decision) {
		t.Fatalf("execution-model context changed decision-provider policy:\n a=%+v\n b=%+v", a.decision, b.decision)
	}
}

// TestCrossModuleProviderFailureIsRecorded proves a real external process failure
// is recorded distinctly (secret-free) while the governed baseline stands.
func TestCrossModuleProviderFailureIsRecorded(t *testing.T) {
	var events []activity.Event
	rec := activity.New("T1", activity.Func(func(e activity.Event) { events = append(events, e) }))
	ctx := activity.WithRecorder(context.Background(), rec)

	in := decisionEvidenceResult(autoFixClassification())
	out := applyDecisionEvidence(ctx, decisionEvidenceConfig(true),
		decisionEvidenceDeps(crossModuleAdapter(t, "fake-external", "fail"), nil), benignSpec(), in)

	if !reflect.DeepEqual(out, in) {
		t.Fatalf("a process failure must not change the governed result: %+v", out)
	}
	found := false
	for _, e := range events {
		if e.Action == "decision-evidence-unavailable" && e.Detail == "provider unavailable" {
			found = true
		}
	}
	if !found {
		t.Fatalf("a real provider failure must be recorded distinctly; events=%+v", events)
	}
}

// TestCrossModuleLivePathEndToEnd proves the whole live lifecycle path: a real
// external process supplies evidence to a real `run --task`, and only an adverse
// choice adds a human boundary.
func TestCrossModuleLivePathEndToEnd(t *testing.T) {
	cfgEnabled := "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\nautonomy:\n  level: high\ndecision:\n  enabled: true\n  provider: command\n  command:\n    - placeholder\n"
	cfgDisabled := "project:\n  name: x\nvalidation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\nautonomy:\n  level: high\n"

	for _, tc := range []struct {
		name     string
		config   string
		mode     string
		off      bool
		wantAuto bool
	}{
		{"provider off is unchanged", cfgDisabled, "", true, true},
		{"valid benign keeps Continue", cfgEnabled, "low", false, true},
		{"valid adverse adds attention", cfgEnabled, "high", false, false},
		{"provider failure keeps Continue", cfgEnabled, "fail", false, true},
		{"malformed keeps Continue", cfgEnabled, "malformed", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "TASK.md", runTaskFile)
			writeConfig(t, dir, tc.config)

			var factory func(config.Config) (decision.Provider, error)
			if tc.off {
				factory = decisionProviderFromConfig
			} else {
				p := crossModuleAdapter(t, "fake-external", tc.mode)
				factory = func(config.Config) (decision.Provider, error) { return p, nil }
			}

			a := outcomeAgent{outcome: &agent.Outcome{Status: agent.OutcomeNeedsHuman, Reason: budgetExhaustedReason}}
			code, stdout, _ := runInjectedCLIWithDecision(t, dir, "diff\n", a, factory, "run", "--task", "TASK.md")
			if code != exitError {
				t.Fatalf("code=%d, want a non-pass result; stdout=%s", code, stdout)
			}
			gotAuto := strings.Contains(stdout, "decision=AUTO_CONTINUE")
			if gotAuto != tc.wantAuto {
				t.Fatalf("decision=AUTO_CONTINUE is %v, want %v:\n%s", gotAuto, tc.wantAuto, stdout)
			}
			if !tc.wantAuto && !strings.Contains(stdout, "decision=HUMAN_APPROVAL_REQUIRED") {
				t.Fatalf("adverse evidence must record a human-approval decision:\n%s", stdout)
			}
		})
	}
}

// TestCrossModuleTimeoutAndCancellation proves SOP owns the deadline across the real
// process boundary: a timeout/cancellation terminates the fixture, is classified as
// a provider failure, never authorizes, and leaves no live process.
func TestCrossModuleTimeoutAndCancellation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		cancel bool
	}{
		{"timeout", false},
		{"cancellation", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pidPath := filepath.Join(t.TempDir(), "pid")
			p := crossModuleAdapter(t, "fake-external", "sleep", "", pidPath)

			ctx := context.Background()
			var cancel context.CancelFunc
			if tc.cancel {
				ctx, cancel = context.WithCancel(ctx)
			} else {
				ctx, cancel = context.WithTimeout(ctx, 300*time.Millisecond)
			}
			defer cancel()

			// Cancel only once the fixture is provably running.
			if tc.cancel {
				go func() {
					deadline := time.Now().Add(5 * time.Second)
					for time.Now().Before(deadline) {
						if _, err := os.Stat(pidPath); err == nil {
							cancel()
							return
						}
						time.Sleep(5 * time.Millisecond)
					}
				}()
			}

			start := time.Now()
			dec, err := p.Decide(ctx, reqForCrossModule())
			elapsed := time.Since(start)

			if err == nil || !strings.Contains(err.Error(), "provider failure") {
				t.Fatalf("expected a classified provider failure, got %v", err)
			}
			if dec.Choice != "" || dec.Confidence != 0 {
				t.Fatalf("timeout/cancellation must not yield a decision: %+v", dec)
			}
			if elapsed > 8*time.Second {
				t.Fatalf("the provider must not stall the caller (took %s)", elapsed)
			}

			// The direct child must have been killed and reaped: no live process remains.
			if runtime.GOOS != "windows" {
				waitForPIDFile(t, pidPath)
				pid := readPID(t, pidPath)
				waitForProcessExit(t, pid, 5*time.Second)
			}
		})
	}
}

func reqForCrossModule() decision.Request {
	return decision.Request{UseCase: decisionEvidenceKind, Subject: "x"}
}

func waitForPIDFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("fixture never wrote its pid file %s", path)
}

func readPID(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read pid: %v", err)
	}
	var pid int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(data)), "%d", &pid); err != nil {
		t.Fatalf("parse pid %q: %v", data, err)
	}
	return pid
}

// waitForProcessExit asserts the process is no longer alive within the deadline.
func waitForProcessExit(t *testing.T, pid int, within time.Duration) {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if !processAlive(pid) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("process %d is still alive after %s (orphan child)", pid, within)
}

// processAlive reports whether pid names a live process. On non-Unix platforms the
// signal probe is not meaningful, so callers guard it.
func processAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}
