package security

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeEvalExecutor records the exact argv/environment and returns scripted
// Promptfoo output. It implements EnvExecutor so the adapter can enforce the
// telemetry-disabled environment.
type fakeEvalExecutor struct {
	version string
	results []byte
	exit    int
	runErr  error
	calls   [][]string
	envs    [][]string
}

func (f *fakeEvalExecutor) Run(ctx context.Context, dir string, argv []string, stdin []byte, maxOut int) ([]byte, []byte, int, error) {
	return f.RunEnv(ctx, dir, argv, nil, stdin, maxOut)
}

func (f *fakeEvalExecutor) RunEnv(_ context.Context, _ string, argv, env []string, _ []byte, _ int) ([]byte, []byte, int, error) {
	f.calls = append(f.calls, append([]string(nil), argv...))
	f.envs = append(f.envs, append([]string(nil), env...))
	if len(argv) >= 2 && argv[1] == "--version" {
		return []byte(f.version), nil, 0, nil
	}
	for i, arg := range argv {
		if arg == "--output" && i+1 < len(argv) {
			_ = os.MkdirAll(filepath.Dir(argv[i+1]), 0o755)
			_ = os.WriteFile(argv[i+1], f.results, 0o644)
		}
	}
	if f.runErr != nil {
		return nil, nil, f.exit, f.runErr
	}
	if f.exit != 0 {
		return nil, nil, f.exit, nil
	}
	return f.results, nil, 0, nil
}

func evalOptions() promptfooOptions {
	return promptfooOptions{
		version:      "0.118.0",
		configPath:   "promptfooconfig.yaml",
		target:       "local-model",
		attack:       "local-attack",
		grading:      "local-grading",
		profile:      "local",
		sevMode:      "none",
		enabled:      true,
		requestLimit: 10,
		concurrency:  4,
		dataHandling: "local-only",
	}
}

func runEvalScan(t *testing.T, a *promptfooAdapter, ex *fakeEvalExecutor) (Execution, string) {
	t.Helper()
	workspace := t.TempDir()
	out := t.TempDir()
	res, err := a.Scan(context.Background(), Request{WorkspaceRoot: workspace, RunID: "run-1", OutputPath: out})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	return res, out
}

func readEvalReport(t *testing.T, out string) EvalReport {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(out, "security", "promptfoo.case.json"))
	if err != nil {
		t.Fatal(err)
	}
	var rep EvalReport
	if err := json.Unmarshal(data, &rep); err != nil {
		t.Fatal(err)
	}
	return rep
}

const twoCaseResults = `{"results":[
 {"testCase":{"description":"AISEC-001"},"success":false,"gradingResult":{"reason":"approval bypass"}},
 {"testCase":{"description":"AISEC-006"},"success":true}
]}`

func TestPromptfooReportOnlyFindingsDoNotBlock(t *testing.T) {
	ex := &fakeEvalExecutor{version: "0.118.0", results: []byte(twoCaseResults)}
	opts := evalOptions()
	opts.reportOnly = true
	a := newPromptfooAdapter(opts, ex, time.Second, 1<<20)
	res, out := runEvalScan(t, a, ex)

	if res.Status != StatusCompleted {
		t.Fatalf("status = %s", res.Status)
	}
	if len(res.Findings) != 1 || !res.Findings[0].ReportOnly {
		t.Fatalf("findings = %+v, want one report-only finding", res.Findings)
	}
	rep := readEvalReport(t, out)
	if rep.Verdict != VerdictReportOnly {
		t.Fatalf("verdict = %s, want REPORT_ONLY for report-only findings", rep.Verdict)
	}
	if rep.Completion != EvalCompletionComplete {
		t.Fatalf("completion = %s, want COMPLETE", rep.Completion)
	}
	if !rep.TelemetryDisabled || rep.Profile != "local" || rep.DataHandling != "local-only" {
		t.Fatalf("policy evidence = %+v", rep)
	}
	for _, c := range rep.Cases {
		if c.EvidenceLink != "security/promptfoo.case.json" {
			t.Fatalf("evidence link = %q, want the normalized report", c.EvidenceLink)
		}
	}
	raw, err := os.ReadFile(filepath.Join(out, "security", "promptfoo.raw.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(raw) {
		t.Fatalf("raw report is not valid JSON: %q", raw)
	}
}

func TestPromptfooDisabledIsSkippedNotFailed(t *testing.T) {
	ex := &fakeEvalExecutor{version: "0.118.0"}
	opts := evalOptions()
	opts.enabled = false
	a := newPromptfooAdapter(opts, ex, time.Second, 1<<20)
	res, out := runEvalScan(t, a, ex)
	if len(ex.calls) != 0 {
		t.Fatalf("disabled evaluation executed %d command(s)", len(ex.calls))
	}
	if !res.Disabled || res.Status != StatusSkipped {
		t.Fatalf("execution = %+v", res)
	}
	rep := readEvalReport(t, out)
	if rep.Completion != EvalCompletionSkipped || rep.Verdict != VerdictNotApplicable {
		t.Fatalf("report = completion %s verdict %s", rep.Completion, rep.Verdict)
	}
}

func TestPromptfooExecutionErrorNeverPasses(t *testing.T) {
	ex := &fakeEvalExecutor{version: "0.118.0", results: []byte(twoCaseResults), exit: 1}
	a := newPromptfooAdapter(evalOptions(), ex, time.Second, 1<<20)
	res, out := runEvalScan(t, a, ex)
	if res.Status != StatusFailed {
		t.Fatalf("status = %s, want failed", res.Status)
	}
	rep := readEvalReport(t, out)
	if rep.Completion != EvalCompletionFailed || rep.Verdict != VerdictIndeterminate {
		t.Fatalf("report = completion %s verdict %s", rep.Completion, rep.Verdict)
	}
	if strings.TrimSpace(rep.ExecutionError) == "" {
		t.Fatal("missing execution error")
	}
}

func TestPromptfooCaseErrorIsAnExecutionError(t *testing.T) {
	ex := &fakeEvalExecutor{version: "0.118.0", results: []byte(`{"results":[{"testCase":{"description":"AISEC-002"},"success":false,"error":"provider unavailable"}]}`)}
	a := newPromptfooAdapter(evalOptions(), ex, time.Second, 1<<20)
	res, out := runEvalScan(t, a, ex)
	if res.Status != StatusFailed {
		t.Fatalf("status = %s, want failed", res.Status)
	}
	if len(res.Findings) != 0 {
		t.Fatalf("case error became a finding: %+v", res.Findings)
	}
	rep := readEvalReport(t, out)
	if rep.Cases[0].Status != EvalCaseError || rep.Verdict != VerdictIndeterminate {
		t.Fatalf("report = %+v", rep)
	}
}

func TestPromptfooUsesPinnedBinaryAndBoundedEnv(t *testing.T) {
	ex := &fakeEvalExecutor{version: "0.118.0", results: []byte(twoCaseResults)}
	opts := evalOptions()
	opts.concurrency = 2
	opts.requestLimit = 3
	a := newPromptfooAdapter(opts, ex, time.Second, 1<<20)
	runEvalScan(t, a, ex)

	var evalCall, evalEnv []string
	for i, call := range ex.calls {
		if len(call) >= 2 && call[1] == "eval" {
			evalCall, evalEnv = call, ex.envs[i]
		}
	}
	if len(evalCall) == 0 {
		t.Fatal("no eval invocation")
	}
	joined := strings.Join(evalCall, " ")
	for _, want := range []string{"promptfoo eval", "--max-concurrency 2", "--filter-first-n 3"} {
		if !strings.Contains(joined, want) {
			t.Errorf("argv %q missing %q", joined, want)
		}
	}
	if strings.Contains(joined, "--share") || strings.Contains(joined, "--no-share") {
		t.Errorf("argv %q must not request sharing", joined)
	}
	if strings.Contains(joined, "npx") {
		t.Errorf("argv %q still uses npx", joined)
	}
	env := strings.Join(evalEnv, "\n")
	for _, want := range []string{"PROMPTFOO_SELF_HOSTED=true", "PROMPTFOO_DISABLE_TELEMETRY=true", "PROMPTFOO_DISABLE_REMOTE_GENERATION=true"} {
		if !strings.Contains(env, want) {
			t.Errorf("environment missing %q", want)
		}
	}
}

func TestPromptfooExplicitSeverityIsNormalized(t *testing.T) {
	ex := &fakeEvalExecutor{version: "0.118.0", results: []byte(`{"results":[
 {"testCase":{"description":"AISEC-001"},"success":false,"metadata":{"severity":"high"},"gradingResult":{"reason":"x"}},
 {"testCase":{"description":"AISEC-004"},"success":false,"metadata":{"severity":"CVSS:9.8"},"gradingResult":{"reason":"x"}}
]}`)}
	opts := evalOptions()
	opts.sevMode = "explicit"
	a := newPromptfooAdapter(opts, ex, time.Second, 1<<20)
	res, _ := runEvalScan(t, a, ex)
	if len(res.Findings) != 2 {
		t.Fatalf("findings = %+v", res.Findings)
	}
	if res.Findings[0].Severity != SeverityHigh {
		t.Fatalf("explicit severity = %s, want HIGH", res.Findings[0].Severity)
	}
	if res.Findings[1].Severity != SeverityUnknown {
		t.Fatalf("CVSS-like severity = %s, want UNKNOWN", res.Findings[1].Severity)
	}
}

func TestPromptfooRequestLimitMarksSkippedCases(t *testing.T) {
	ex := &fakeEvalExecutor{version: "0.118.0", results: []byte(`{"results":[
 {"testCase":{"description":"AISEC-001"},"success":true},
 {"testCase":{"description":"AISEC-002"},"success":true},
 {"testCase":{"description":"AISEC-003"},"success":true}
]}`)}
	opts := evalOptions()
	opts.requestLimit = 2
	a := newPromptfooAdapter(opts, ex, time.Second, 1<<20)
	res, out := runEvalScan(t, a, ex)
	if res.Coverage != CoveragePartial {
		t.Fatalf("coverage = %s, want PARTIAL", res.Coverage)
	}
	rep := readEvalReport(t, out)
	if rep.Cases[2].Status != EvalCaseSkipped {
		t.Fatalf("capped case = %s, want skipped", rep.Cases[2].Status)
	}
	if rep.Verdict == VerdictPass {
		t.Fatal("a capped run must not report a clean pass")
	}
}

func TestPromptfooFailedTestExitIsAFinding(t *testing.T) {
	ex := &fakeEvalExecutor{version: "0.118.0", results: []byte(twoCaseResults), exit: promptfooFailedTestExitCode}
	a := newPromptfooAdapter(evalOptions(), ex, time.Second, 1<<20)
	res, out := runEvalScan(t, a, ex)
	if res.Status != StatusCompleted {
		t.Fatalf("status = %s, want completed", res.Status)
	}
	if len(res.Findings) != 1 {
		t.Fatalf("findings = %+v, want one behavioral finding", res.Findings)
	}
	rep := readEvalReport(t, out)
	if rep.Verdict != VerdictFail {
		t.Fatalf("verdict = %s, want FAIL for a non-report-only failed case", rep.Verdict)
	}
}

func TestPromptfooOperatingGuide(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "docs", "security-promptfoo.md"))
	if err != nil {
		t.Fatal(err)
	}
	guide := string(data)
	for _, want := range []string{
		"## Configuration keys",
		"## Normalized report",
		"## Command exit behavior",
		"## Outbound behavior and telemetry",
		"## Isolation boundary",
		"## Initial AISEC suite",
		"## Live verification",
		"REPORT_ONLY",
	} {
		if !strings.Contains(guide, want) {
			t.Errorf("operating guide is missing %q", want)
		}
	}
}

func TestPromptfooRequiresEnvExecutor(t *testing.T) {
	a := newPromptfooAdapter(evalOptions(), plainExecutor{}, time.Second, 1<<20)
	_, err := a.Scan(context.Background(), Request{WorkspaceRoot: t.TempDir(), RunID: "r"})
	if err != nil {
		t.Fatalf("scan returned an adapter error instead of an execution failure: %v", err)
	}
}

type plainExecutor struct{}

func (plainExecutor) Run(context.Context, string, []string, []byte, int) ([]byte, []byte, int, error) {
	return nil, nil, 0, errors.New("plain executor cannot run promptfoo")
}
