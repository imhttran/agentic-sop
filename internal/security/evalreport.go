package security

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// EvalCaseStatus is the per-case outcome of one AI evaluation case. It is limited
// to passed, failed, error, or skipped. A timeout, unavailable provider, missing
// artifact, parsing failure, or skip never becomes a pass.
type EvalCaseStatus string

const (
	EvalCasePassed  EvalCaseStatus = "passed"
	EvalCaseFailed  EvalCaseStatus = "failed"
	EvalCaseError   EvalCaseStatus = "error"
	EvalCaseSkipped EvalCaseStatus = "skipped"
)

// isKnownEvalCaseStatus reports whether s is one of the four allowed statuses.
func isKnownEvalCaseStatus(s EvalCaseStatus) bool {
	switch s {
	case EvalCasePassed, EvalCaseFailed, EvalCaseError, EvalCaseSkipped:
		return true
	}
	return false
}

// EvalUnavailable is the explicit marker for a required field that could not be
// populated. It never means empty or clean.
const EvalUnavailable = "UNAVAILABLE"

// EvalCase is one normalized evaluation case. Required identity and evidence
// fields are populated or explicitly marked EvalUnavailable.
type EvalCase struct {
	CaseID string         `json:"case_id"`
	Status EvalCaseStatus `json:"status"`
	// Finding is retained when the case produced a behavioral finding. A finding
	// is distinct from an adapter execution error.
	Finding  bool   `json:"finding"`
	RuleID   string `json:"rule_id,omitempty"`
	Message  string `json:"message,omitempty"`
	Duration int64  `json:"duration_ms"`
	// EvidenceLink points at the redacted evidence for this case. It is never a
	// raw secret.
	EvidenceLink string `json:"evidence_link"`
	// Severity is empty unless severity_mode is explicit. Behavioral failures get no
	// automatic CVSS severity.
	Severity string `json:"severity,omitempty"`
}

// EvalReport is the normalized evaluation report. It carries run identity, tool
// and model identifiers, hashes, per-case status, completion status, duration,
// and redacted evidence links.
type EvalReport struct {
	ProtocolVersion string `json:"protocol_version"`
	RunID           string `json:"run_id"`
	TaskID          string `json:"task_id"`
	// SOPRevision and WorkingTree identify the SOP revision and content under
	// test. They are populated or explicitly unavailable.
	SOPRevision      string          `json:"sop_revision"`
	WorkingTree      ContentIdentity `json:"working_tree"`
	PromptfooVersion string          `json:"promptfoo_version"`
	TargetModel      string          `json:"target_model"`
	AttackProvider   string          `json:"attack_provider"`
	GradingProvider  string          `json:"grading_provider"`
	ConfigHash       string          `json:"config_hash"`
	DatasetHash      string          `json:"dataset_hash"`
	// Profile, DataHandling, TelemetryDisabled, RequestLimit, and Concurrency
	// record the effective evaluation policy, not just the requested values.
	Profile           string    `json:"profile"`
	DataHandling      string    `json:"data_handling"`
	TelemetryDisabled bool      `json:"telemetry_disabled"`
	RequestLimit      int       `json:"request_limit,omitempty"`
	Concurrency       int       `json:"concurrency,omitempty"`
	StartedAt         time.Time `json:"started_at"`
	EndedAt           time.Time `json:"ended_at"`
	DurationMS        int64     `json:"duration_ms"`
	// Completion is COMPLETE, PARTIAL, FAILED, or SKIPPED. Any error case makes
	// it at least PARTIAL; an execution error that prevented parsing makes it
	// FAILED.
	Completion string `json:"completion"`
	// ReportOnly echoes the configuration: a finding does not block the workflow.
	ReportOnly bool       `json:"report_only"`
	Cases      []EvalCase `json:"cases"`
	// ExecutionError records an adapter execution error separately from findings.
	ExecutionError string   `json:"execution_error,omitempty"`
	Diagnostics    []string `json:"diagnostics,omitempty"`
	Verdict        Verdict  `json:"verdict"`
}

// Completion status values.
const (
	EvalCompletionComplete = "COMPLETE"
	EvalCompletionPartial  = "PARTIAL"
	EvalCompletionFailed   = "FAILED"
	// EvalCompletionSkipped records a deliberately disabled evaluation. It is
	// distinct from FAILED, which means an execution error.
	EvalCompletionSkipped = "SKIPPED"
)

// promptfooResults is the top-level results.json shape. Promptfoo 0.118 wraps the
// result list in an object with stats; older fixtures used a bare array. The
// decoder accepts both so the normalized report is independent of the envelope.
type promptfooResults struct {
	Results json.RawMessage `json:"results"`
}

type promptfooResultsEnvelope struct {
	Results []promptfooResult `json:"results"`
	Stats   struct {
		Successes int `json:"successes"`
		Failures  int `json:"failures"`
		Errors    int `json:"errors"`
	} `json:"stats"`
}

type promptfooResult struct {
	TestCase struct {
		Description string `json:"description"`
	} `json:"testCase"`
	TestIdx       int     `json:"testIdx"`
	Success       bool    `json:"success"`
	Error         string  `json:"error"`
	Score         float64 `json:"score"`
	LatencyMS     int64   `json:"latencyMs"`
	GradingResult struct {
		Reason string `json:"reason"`
	} `json:"gradingResult"`
	Metadata map[string]any `json:"metadata"`
	Skipped  bool           `json:"skipped"`
}

// ParsePromptfooResults normalizes a Promptfoo results.json document into a case
// list. A parse failure returns an error so the caller records an execution
// error and never a pass.
func ParsePromptfooResults(raw []byte) ([]EvalCase, error) {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil, fmt.Errorf("promptfoo results are empty")
	}
	var doc promptfooResults
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse promptfoo results: %w", err)
	}
	var results []promptfooResult
	trimmed := strings.TrimSpace(string(doc.Results))
	if strings.HasPrefix(trimmed, "[") {
		if err := json.Unmarshal(doc.Results, &results); err != nil {
			return nil, fmt.Errorf("parse promptfoo results: %w", err)
		}
	} else {
		var envelope promptfooResultsEnvelope
		if err := json.Unmarshal(doc.Results, &envelope); err != nil {
			return nil, fmt.Errorf("parse promptfoo results: %w", err)
		}
		results = envelope.Results
	}
	cases := make([]EvalCase, 0, len(results))
	for i, r := range results {
		id := strings.TrimSpace(r.TestCase.Description)
		if id == "" {
			if r.TestIdx > 0 {
				id = fmt.Sprintf("case-%d", r.TestIdx+1)
			} else {
				id = fmt.Sprintf("case-%d", i+1)
			}
		}
		c := EvalCase{CaseID: id, Duration: r.LatencyMS}
		switch {
		case r.Skipped:
			c.Status = EvalCaseSkipped
		case strings.TrimSpace(r.Error) != "":
			c.Status = EvalCaseError
			c.Message = SanitizeText(r.Error, MaxMessageBytes)
		case r.Success:
			c.Status = EvalCasePassed
		default:
			c.Status = EvalCaseFailed
			c.Finding = true
			c.Message = SanitizeText(r.GradingResult.Reason, MaxMessageBytes)
		}
		if rawSeverity, ok := r.Metadata["severity"].(string); ok {
			c.Severity = normalizeExplicitSeverity(rawSeverity)
		}
		c.EvidenceLink = "security/promptfoo.case.json#" + id
		cases = append(cases, c)
	}
	return cases, nil
}

// SummarizeEvalCases computes the completion status and evaluation verdict from
// cases and a report-only policy. An execution error or an error case can never
// yield a successful verdict in either mode. In report-only mode a finding is
// recorded without blocking the ordinary workflow, so the verdict is
// VerdictReportOnly rather than VerdictPass; the finding itself stays recorded in
// the report and in the execution Findings, so a reader can still tell the run
// had behavioral failures. An execution error stays non-successful in both modes.
func SummarizeEvalCases(cases []EvalCase, reportOnly bool) (string, Verdict) {
	if len(cases) == 0 {
		return EvalCompletionFailed, VerdictIndeterminate
	}
	hasError := false
	hasFail := false
	hasSkip := false
	allPass := true
	for _, c := range cases {
		switch c.Status {
		case EvalCaseError:
			hasError = true
			allPass = false
		case EvalCaseFailed:
			hasFail = true
			allPass = false
		case EvalCaseSkipped:
			hasSkip = true
			allPass = false
		case EvalCasePassed:
		default:
			// An unknown status is an execution error, never a pass.
			hasError = true
			allPass = false
		}
	}
	completion := EvalCompletionComplete
	if hasError || hasSkip {
		completion = EvalCompletionPartial
	}
	// An execution error can never yield a successful verdict.
	if hasError {
		return completion, VerdictIndeterminate
	}
	if hasFail {
		if reportOnly {
			// The finding is recorded in the report and no execution error occurred.
			// Report-only mode does not block the ordinary workflow, but the verdict
			// is VerdictReportOnly so a run with findings is never indistinguishable
			// from a clean VerdictPass run. Execution errors are handled above and
			// never reach this branch.
			return completion, VerdictReportOnly
		}
		return completion, VerdictFail
	}
	if hasSkip {
		return completion, VerdictIndeterminate
	}
	if allPass {
		return completion, VerdictPass
	}
	return completion, VerdictIndeterminate
}

// WriteEvalArtifacts persists the sanitized raw Promptfoo report and the
// normalized report under <runDir>/security. It reuses the existing artifact
// directory and never writes a secret value.
func WriteEvalArtifacts(runDir string, report EvalReport, rawSanitized []byte) (string, string, error) {
	dir := filepath.Join(runDir, "security")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	rawName := "promptfoo.raw.json"
	if len(rawSanitized) == 0 {
		rawSanitized = []byte("{\"status\":\"" + EvalUnavailable + "\"}")
	}
	if err := os.WriteFile(filepath.Join(dir, rawName), rawSanitized, 0o644); err != nil {
		return "", "", err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", "", err
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(dir, "promptfoo.case.json"), data, 0o644); err != nil {
		return "", "", err
	}
	return filepath.ToSlash(filepath.Join("security", rawName)),
		filepath.ToSlash(filepath.Join("security", "promptfoo.case.json")), nil
}

// sortEvalCases orders cases deterministically by case id.
func sortEvalCases(cases []EvalCase) {
	sort.SliceStable(cases, func(i, j int) bool { return cases[i].CaseID < cases[j].CaseID })
}

// normalizeExplicitSeverity accepts only the normalized severity labels. A
// CVSS score or arbitrary string is never promoted into a severity.
func normalizeExplicitSeverity(raw string) string {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "CRITICAL", "HIGH", "MEDIUM", "LOW", "INFO":
		return strings.ToUpper(strings.TrimSpace(raw))
	default:
		return ""
	}
}
