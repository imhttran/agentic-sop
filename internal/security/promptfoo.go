package security

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// promptfooAdapter runs the pinned Promptfoo CLI through the existing
// command-execution layer. It never downloads or updates the tool: a missing or
// mismatched pin is an execution error, and installation is a separate,
// documented operator step.
type promptfooAdapter struct {
	version      string
	configPath   string
	target       string
	attack       string
	grading      string
	profile      string
	sevMode      string
	reportOnly   bool
	enabled      bool
	exec         Executor
	timeout      time.Duration
	maxOut       int
	configHash   string
	datasetHash  string
	requestLimit int
	concurrency  int
	remote       bool
	dataHandling string
}

func newPromptfooAdapter(e promptfooOptions, exec Executor, timeout time.Duration, maxOut int) *promptfooAdapter {
	return &promptfooAdapter{
		version: e.version, configPath: e.configPath, target: e.target,
		attack: e.attack, grading: e.grading, profile: e.profile,
		sevMode: e.sevMode, reportOnly: e.reportOnly, enabled: e.enabled,
		exec: exec, timeout: timeout, maxOut: maxOut, configHash: e.configHash,
		datasetHash: e.datasetHash, requestLimit: e.requestLimit,
		concurrency: e.concurrency, remote: e.remote, dataHandling: e.dataHandling,
	}
}

// promptfooOptions carries the resolved evaluation configuration the adapter
// needs. The config package maps SecurityEvalConfig onto it.
type promptfooOptions struct {
	version      string
	configPath   string
	target       string
	attack       string
	grading      string
	profile      string
	sevMode      string
	reportOnly   bool
	enabled      bool
	configHash   string
	datasetHash  string
	requestLimit int
	concurrency  int
	remote       bool
	dataHandling string
}

func (a *promptfooAdapter) Descriptor() Descriptor {
	locality := "local"
	// The adapter does not select or rewrite providers inside the Promptfoo
	// config, and it does not sandbox the Promptfoo process. Do not describe this
	// as network isolation; only the configured provider identities and the
	// telemetry-disable environment are enforced here.
	network := "config_path providers only; SOP adds no remote fallback and does not network-isolate the process"
	if a.remote {
		locality = "remote"
		network = "explicitly configured remote providers; telemetry and sharing are disabled"
	}
	return Descriptor{
		ProtocolVersion: ProtocolVersion,
		AdapterID:       "promptfoo",
		AdapterVersion:  a.version,
		Categories:      []Category{CategoryEvaluation},
		ScopeModes:      []string{"workspace"},
		Locality:        locality,
		Network:         network,
		RepositoryWide:  true,
	}
}

const (
	// promptfooFailedTestExitCode is Promptfoo's documented exit code for a run
	// with at least one failed test. It is a finding, not an adapter execution
	// error, so the results file is parsed normally.
	promptfooFailedTestExitCode = 100
)

// promptfooEnv is the fixed environment for every Promptfoo invocation. Telemetry,
// sharing, update checks, and hosted generation are disabled regardless of the
// configuration value; the adapter records that fact in its report. The
// environment is the effective policy, so telemetry_disabled reports true only
// because this environment was set.
func promptfooEnv() []string {
	env := []string{
		"PROMPTFOO_SELF_HOSTED=true",
		"PROMPTFOO_DISABLE_TELEMETRY=true",
		"PROMPTFOO_DISABLE_UPDATE=true",
		"PROMPTFOO_DISABLE_REMOTE_GENERATION=true",
		"PROMPTFOO_DISABLE_REDTEAM_REMOTE_GENERATION=true",
		"PROMPTFOO_EVAL_TIMEOUT_MS=120000",
		"PROMPTFOO_MAX_EVAL_TIME_MS=300000",
	}
	// The local Ollama endpoint is not a secret; pass it through because the
	// scanner environment intentionally strips provider-looking variables.
	if v := strings.TrimSpace(os.Getenv("OLLAMA_BASE_URL")); v != "" {
		env = append(env, "OLLAMA_BASE_URL="+v)
	}
	return env
}

// runPromptfoo runs the pinned binary with an explicit environment. An executor
// without environment support is a fail-closed execution error: the adapter
// cannot claim telemetry or sharing was disabled otherwise.
func runPromptfoo(ctx context.Context, exec Executor, dir string, argv, env []string, maxOut int) ([]byte, []byte, int, error) {
	ee, ok := exec.(EnvExecutor)
	if !ok {
		return nil, nil, -1, errors.New("promptfoo requires an executor with explicit environment support")
	}
	return ee.RunEnv(ctx, dir, argv, env, nil, maxOut)
}

// promptfooVersionCheck confirms the pinned Promptfoo version is already
// installed locally. It runs the same binary the evaluation runs, so a version
// check can never pass for a different executable. It never installs anything.
func promptfooVersionCheck(ctx context.Context, exec Executor, dir string, version string, maxOut int) (bool, string, int) {
	stdout, _, exitCode, err := runPromptfoo(ctx, exec, dir, []string{"promptfoo", "--version"}, promptfooEnv(), maxOut)
	if err != nil {
		return false, "promptfoo is not installed locally; run the documented install step for " + version + ": " + SanitizeText(err.Error(), MaxDiagnosticBytes), exitCode
	}
	if exitCode != 0 {
		return false, fmt.Sprintf("promptfoo --version exited with code %d", exitCode), exitCode
	}
	got := strings.TrimSpace(string(stdout))
	if !strings.Contains(got, strings.TrimSpace(version)) {
		return false, fmt.Sprintf("promptfoo version %q does not match pinned %q", got, version), exitCode
	}
	return true, "promptfoo " + version + " is installed locally", exitCode
}

// Scan runs the pinned Promptfoo binary directly, normalizes results.json into a
// per-case report, and derives a SOP-owned verdict. Report-only mode records
// findings without blocking; an execution error always keeps the verdict
// non-successful.
func (a *promptfooAdapter) Scan(ctx context.Context, req Request) (Execution, error) {
	execOut := newExecution("promptfoo", CategoryEvaluation, a.version, a.configHash, "", req)
	execOut.Provenance.ScannerVersion = a.version
	execOut.RepositoryWide = true

	report := EvalReport{
		ProtocolVersion:   ProtocolVersion,
		RunID:             req.RunID,
		TaskID:            req.TaskID,
		SOPRevision:       firstNonEmptyEval(req.Configuration["sop_revision"]),
		WorkingTree:       req.Content,
		PromptfooVersion:  a.version,
		TargetModel:       firstNonEmptyEval(a.target),
		AttackProvider:    firstNonEmptyEval(a.attack),
		GradingProvider:   firstNonEmptyEval(a.grading),
		ConfigHash:        firstNonEmptyEval(a.configHash),
		DatasetHash:       firstNonEmptyEval(a.datasetHash),
		Profile:           firstNonEmptyEval(a.profile),
		DataHandling:      firstNonEmptyEval(a.dataHandling),
		TelemetryDisabled: true,
		RequestLimit:      a.requestLimit,
		Concurrency:       a.concurrency,
		ReportOnly:        a.reportOnly,
		Cases:             []EvalCase{},
	}
	setReportUnavailable(&report)

	if !a.enabled {
		execOut.Disabled = true
		execOut.Status = StatusSkipped
		execOut.Coverage = CoverageUnknown
		execOut.Diagnostics = []string{"promptfoo evaluation is disabled by configuration"}
		report.Completion = EvalCompletionSkipped
		report.Verdict = VerdictNotApplicable
		return finishPromptfoo(execOut, report, nil, req)
	}

	report.StartedAt = time.Now().UTC()
	runCtx, cancel := scanContext(ctx, a.timeout)
	defer cancel()

	available, note, verExit := promptfooVersionCheck(runCtx, a.exec, req.WorkspaceRoot, a.version, a.maxOut)
	execOut.Diagnostics = append(execOut.Diagnostics, note)
	execOut.Provenance.ScannerVersion = a.version
	execOut.Provenance.RulesIdentity = "promptfoo-pin:" + a.version
	if !available {
		execOut.Status = StatusUnavailable
		execOut.ExitCode = verExit
		execOut.Coverage = CoverageUnknown
		report.ExecutionError = note
		return finishPromptfoo(execOut, report, nil, req)
	}

	// The Promptfoo process writes to a private scratch directory, never straight
	// into the workspace artifact path: an unsanitized file must not persist when
	// the run times out, crashes, or exits non-zero. The adapter reads, sanitizes,
	// and only then persists the report.
	scratch, err := os.MkdirTemp("", "sop-promptfoo-*")
	if err != nil {
		execOut.Status = StatusFailed
		execOut.Coverage = CoverageUnknown
		report.ExecutionError = "promptfoo scratch directory: " + SanitizeText(err.Error(), MaxDiagnosticBytes)
		return finishPromptfoo(execOut, report, nil, req)
	}
	defer func() { _ = os.RemoveAll(scratch) }()
	outPath := filepath.Join(scratch, "promptfoo.raw.json")

	// The pinned binary is invoked directly, never through npx: ordinary
	// execution cannot download or update it. Sharing is not requested: the
	// installed release has no --no-share flag, and --share is never passed.
	argv := []string{"promptfoo", "eval", "-c", a.configPath, "--output", outPath, "--no-progress-bar"}
	if a.concurrency > 0 {
		argv = append(argv, "--max-concurrency", strconv.Itoa(a.concurrency))
	}
	if a.requestLimit > 0 {
		argv = append(argv, "--filter-first-n", strconv.Itoa(a.requestLimit))
	}
	_, stderr, exitCode, runErr := runPromptfoo(runCtx, a.exec, req.WorkspaceRoot, argv, promptfooEnv(), a.maxOut)
	execOut.ExitCode = exitCode
	if len(stderr) > 0 {
		execOut.Diagnostics = append(execOut.Diagnostics, SanitizeText(string(stderr), MaxDiagnosticBytes))
	}
	// A raw report may exist even after a non-zero exit or a timeout. Preserve it,
	// sanitized, so the failure stays inspectable, but never treat it as a pass.
	var raw []byte
	if data, readErr := os.ReadFile(outPath); readErr == nil {
		raw = sanitizeBytes(data, a.maxOut)
	}
	if runErr != nil {
		switch {
		case errors.Is(runErr, ErrUnavailable):
			execOut.Status = StatusUnavailable
			execOut.Diagnostics = append(execOut.Diagnostics, "promptfoo executable is unavailable")
			report.ExecutionError = "promptfoo executable is unavailable"
		case errors.Is(runErr, context.DeadlineExceeded), errors.Is(runErr, context.Canceled):
			execOut.Status = StatusTimedOut
			execOut.Diagnostics = append(execOut.Diagnostics, "promptfoo exceeded the process timeout")
			report.ExecutionError = "promptfoo exceeded the process timeout"
		default:
			execOut.Status = StatusFailed
			execOut.Diagnostics = append(execOut.Diagnostics, "promptfoo execution failed: "+SanitizeText(runErr.Error(), MaxDiagnosticBytes))
			report.ExecutionError = "promptfoo execution failed: " + SanitizeText(runErr.Error(), MaxDiagnosticBytes)
		}
		execOut.Coverage = CoverageUnknown
		return finishPromptfoo(execOut, report, raw, req)
	}
	if exitCode != 0 && exitCode != promptfooFailedTestExitCode {
		execOut.Status = StatusFailed
		execOut.Diagnostics = append(execOut.Diagnostics, fmt.Sprintf("promptfoo exited with code %d", exitCode))
		report.ExecutionError = fmt.Sprintf("promptfoo exited with code %d", exitCode)
		execOut.Coverage = CoverageUnknown
		return finishPromptfoo(execOut, report, nil, req)
	}

	if len(raw) == 0 {
		execOut.Status = StatusFailed
		execOut.Diagnostics = append(execOut.Diagnostics, "promptfoo report is missing: "+EvalUnavailable)
		report.ExecutionError = "promptfoo report is missing: " + EvalUnavailable
		execOut.Coverage = CoverageUnknown
		return finishPromptfoo(execOut, report, nil, req)
	}
	execOut.NativeReport = raw
	execOut.NativeReportDigest = DigestBytes(raw)
	execOut.Provenance.SanitizedReportDigest = execOut.NativeReportDigest

	cases, err := ParsePromptfooResults(raw)
	if err != nil {
		execOut.Status = StatusFailed
		execOut.Diagnostics = append(execOut.Diagnostics, "promptfoo report parsing failure: "+SanitizeText(err.Error(), MaxDiagnosticBytes))
		report.ExecutionError = "promptfoo report parsing failure: " + SanitizeText(err.Error(), MaxDiagnosticBytes)
		execOut.Coverage = CoverageUnknown
		return finishPromptfoo(execOut, report, raw, req)
	}

	execOut.Coverage = CoverageComplete
	// The CLI already applies --filter-first-n. A case beyond the limit is
	// recorded as skipped only when the CLI did not cap it, so a case is never
	// counted twice. Case skips are kept in the evaluation report, not mixed into
	// the file-skip list.
	if a.requestLimit > 0 && len(cases) > a.requestLimit {
		for i := a.requestLimit; i < len(cases); i++ {
			if cases[i].Status == EvalCaseSkipped {
				continue
			}
			cases[i].Status = EvalCaseSkipped
			cases[i].Finding = false
			cases[i].Message = "not evaluated: request_limit"
		}
		execOut.Coverage = CoveragePartial
		execOut.Diagnostics = append(execOut.Diagnostics, fmt.Sprintf("request_limit capped %d case(s)", len(cases)-a.requestLimit))
	}

	execErr := false
	for _, c := range cases {
		// Behavioral failures become findings; error cases are execution errors
		// and are never converted into findings or passes.
		if c.Status == EvalCaseFailed {
			ruleID := c.RuleID
			if ruleID == "" {
				ruleID = "promptfoo/" + c.CaseID
			}
			severity := SeverityUnknown
			if a.sevMode == "explicit" && c.Severity != "" {
				severity = Severity(c.Severity)
			}
			f := Finding{
				AdapterID:   "promptfoo",
				Provider:    "promptfoo",
				Category:    CategoryEvaluation,
				RuleID:      ruleID,
				Severity:    severity,
				Message:     c.Message,
				Fingerprint: DigestStrings("promptfoo", c.CaseID),
				ReportOnly:  a.reportOnly,
			}
			execOut.Findings = append(execOut.Findings, f)
		}
		if c.Status == EvalCaseError {
			execErr = true
			execOut.Diagnostics = append(execOut.Diagnostics, "case "+c.CaseID+" errored")
		}
	}
	// Keep the case order deterministic before it enters the report.
	sortEvalCases(cases)
	report.Cases = cases
	if execErr {
		// A case error is an execution error: it is visible and can never yield a
		// successful verdict, in either mode.
		report.ExecutionError = "one or more promptfoo cases errored"
		execOut.Status = StatusFailed
		execOut.Coverage = CoveragePartial
	} else {
		execOut.Status = StatusCompleted
	}
	return finishPromptfoo(execOut, report, raw, req)
}

// finishPromptfoo stamps the report timing and verdict, persists the raw and
// normalized reports, and finalizes the execution contract. In report-only mode
// a finding does not block the workflow; an execution error still keeps the
// verdict non-successful. A failed artifact write is itself an execution error,
// so a run never reports COMPLETE without its stored evidence.
func finishPromptfoo(exec Execution, report EvalReport, raw []byte, req Request) (Execution, error) {
	report.EndedAt = time.Now().UTC()
	if !report.StartedAt.IsZero() {
		report.DurationMS = report.EndedAt.Sub(report.StartedAt).Milliseconds()
	} else {
		// No process ran, so the duration is explicitly unavailable rather than a
		// misleading zero.
		report.DurationMS = -1
	}
	if report.ExecutionError != "" {
		// An execution error keeps the verdict non-successful and marks the run
		// FAILED; it is never a pass even when some cases passed.
		report.Completion = EvalCompletionFailed
		report.Verdict = VerdictIndeterminate
	} else if report.Completion == "" || report.Completion == EvalUnavailable {
		// Derive completion and verdict from the cases before any field is marked
		// unavailable, so a report-only run with findings is not mistaken for a
		// missing completion status.
		completion, verdict := SummarizeEvalCases(report.Cases, report.ReportOnly)
		report.Completion = completion
		report.Verdict = verdict
	}
	setReportUnavailable(&report)
	// Point every case at the redacted normalized evidence file BEFORE the report
	// is serialized, so the persisted JSON and the in-memory report agree.
	const normLink = "security/promptfoo.case.json"
	for i := range report.Cases {
		report.Cases[i].EvidenceLink = normLink
	}
	if runDir := strings.TrimSpace(req.OutputPath); runDir != "" {
		rawPath, normPath, err := WriteEvalArtifacts(runDir, report, raw)
		if err != nil {
			// A missing artifact can never convert into a pass: record it as an
			// execution error and keep the verdict non-successful.
			exec.Status = StatusFailed
			exec.Coverage = CoverageUnknown
			exec.Diagnostics = append(exec.Diagnostics, "failed to persist evaluation artifacts: "+SanitizeText(err.Error(), MaxDiagnosticBytes))
			report.Completion = EvalCompletionFailed
			report.Verdict = VerdictIndeterminate
			report.ExecutionError = "failed to persist evaluation artifacts: " + SanitizeText(err.Error(), MaxDiagnosticBytes)
		} else {
			exec.Diagnostics = append(exec.Diagnostics, "normalized report: "+normPath+"; raw report: "+rawPath)
		}
	}
	return finish(exec, nil)
}

// firstNonEmptyEval returns the first non-empty trimmed value, or EvalUnavailable
// so a required field is explicitly marked rather than left blank.
func firstNonEmptyEval(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return EvalUnavailable
}

// setReportUnavailable marks every required-but-unset identity field as
// EvalUnavailable, so the report never leaves a required field blank.
func setReportUnavailable(report *EvalReport) {
	if strings.TrimSpace(report.SOPRevision) == "" {
		report.SOPRevision = EvalUnavailable
	}
	if strings.TrimSpace(report.PromptfooVersion) == "" {
		report.PromptfooVersion = EvalUnavailable
	}
	if strings.TrimSpace(report.TargetModel) == "" {
		report.TargetModel = EvalUnavailable
	}
	if strings.TrimSpace(report.AttackProvider) == "" {
		report.AttackProvider = EvalUnavailable
	}
	if strings.TrimSpace(report.GradingProvider) == "" {
		report.GradingProvider = EvalUnavailable
	}
	if strings.TrimSpace(report.ConfigHash) == "" {
		report.ConfigHash = EvalUnavailable
	}
	if strings.TrimSpace(report.DatasetHash) == "" {
		report.DatasetHash = EvalUnavailable
	}
	if strings.TrimSpace(report.RunID) == "" {
		report.RunID = EvalUnavailable
	}
	if strings.TrimSpace(report.TaskID) == "" {
		report.TaskID = EvalUnavailable
	}
	if report.WorkingTree.Commit == "" && report.WorkingTree.Tree == "" && report.WorkingTree.Digest == "" {
		// No working-tree identity was supplied; mark it explicitly instead of a
		// zero struct that reads as a clean tree.
		report.WorkingTree.Digest = EvalUnavailable
	}
	if strings.TrimSpace(report.Profile) == "" {
		report.Profile = EvalUnavailable
	}
	if strings.TrimSpace(report.DataHandling) == "" {
		report.DataHandling = EvalUnavailable
	}
	if strings.TrimSpace(report.Completion) == "" {
		report.Completion = EvalUnavailable
	}
	if strings.TrimSpace(string(report.Verdict)) == "" {
		report.Verdict = VerdictIndeterminate
	}
	if report.StartedAt.IsZero() {
		report.StartedAt = time.Time{}.UTC()
	}
}

// SanitizeTextWithLimit is the shared redaction entry point for evaluation text.
// It never truncates JSON in a way that would invalidate evidence.
func sanitizeBytes(data []byte, _ int) []byte {
	return []byte(SanitizeText(string(data), len(data)+1))
}

// digestBytesSHA256 returns a stable sha256 hex digest of data.
func digestBytesSHA256(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// HashFile returns the sha256 hex digest of one file, or EvalUnavailable when the
// file cannot be read. It never returns an empty string as if it were clean.
func HashFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return EvalUnavailable
	}
	return digestBytesSHA256(data)
}
