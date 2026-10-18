package security

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// semgrepAdapter wraps the Semgrep Community Edition CLI. It invokes the
// executable directly with an argument array, parses Semgrep's JSON output, and
// records the rules identity. Rule downloads are never implicit.
type semgrepAdapter struct {
	id           string
	category     Category
	command      []string
	rules        string
	version      string
	configDigest string
	exec         Executor
	timeout      time.Duration
	maxOutput    int
}

func (a *semgrepAdapter) Descriptor() Descriptor {
	return Descriptor{
		ProtocolVersion: ProtocolVersion,
		AdapterID:       a.id,
		AdapterVersion:  a.version,
		Categories:      []Category{a.category},
		ScopeModes:      []string{"workspace", "changed-files"},
		Locality:        "local",
		Network:         "none required; local rules only (no implicit download or upload)",
	}
}

func (a *semgrepAdapter) currentProvenance(req Request) (Provenance, error) {
	prov := Provenance{
		ScannerVersion:      a.version,
		ConfigurationDigest: a.configDigest,
		ContentIdentity:     req.Content,
	}
	if strings.TrimSpace(a.rules) == "" {
		return prov, nil
	}
	rules := a.rules
	if !isAbsPath(rules) {
		rules = joinPath(req.WorkspaceRoot, rules)
	}
	identity, err := rulesIdentity(rules)
	if err != nil {
		return prov, err
	}
	prov.RulesIdentity = identity
	return prov, nil
}

type semgrepReport struct {
	Version string          `json:"version"`
	Results []semgrepResult `json:"results"`
	Errors  []semgrepError  `json:"errors"`
	Paths   struct {
		Scanned []string `json:"scanned"`
		Skipped []struct {
			Path   string `json:"path"`
			Reason string `json:"reason"`
		} `json:"skipped"`
	} `json:"paths"`
}

type semgrepResult struct {
	CheckID string `json:"check_id"`
	Path    string `json:"path"`
	Start   struct {
		Line int `json:"line"`
		Col  int `json:"col"`
	} `json:"start"`
	End struct {
		Line int `json:"line"`
		Col  int `json:"col"`
	} `json:"end"`
	Extra struct {
		Message     string         `json:"message"`
		Severity    string         `json:"severity"`
		Fingerprint string         `json:"fingerprint"`
		Metadata    map[string]any `json:"metadata"`
	} `json:"extra"`
}

type semgrepError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
}

func (a *semgrepAdapter) Scan(ctx context.Context, req Request) (Execution, error) {
	exec := newExecution(a.id, a.category, a.version, a.configDigest, "", req)
	if strings.TrimSpace(a.rules) == "" {
		exec.Status = StatusFailed
		exec.Coverage = CoverageUnknown
		exec.Diagnostics = []string{"semgrep rules are not configured; implicit rule download is disabled"}
		return finish(exec, nil)
	}
	rules := a.rules
	if !isAbsPath(rules) {
		rules = joinPath(req.WorkspaceRoot, rules)
	}
	identity, err := rulesIdentity(rules)
	if err != nil {
		exec.Status = StatusFailed
		exec.Diagnostics = []string{"semgrep rules are unavailable: " + SanitizeText(err.Error(), MaxDiagnosticBytes)}
		return finish(exec, nil)
	}
	exec.Provenance.RulesIdentity = identity

	if req.Scope.Mode == "changed-files" && len(req.Scope.Files) == 0 {
		exec.Coverage = CoverageComplete
		exec.Diagnostics = []string{"no changed files to scan"}
		return finish(exec, nil)
	}

	argv := append([]string(nil), a.command...)
	if len(argv) == 0 {
		argv = []string{"semgrep"}
	}
	timeoutSeconds := int(a.timeout.Seconds())
	if timeoutSeconds <= 0 {
		timeoutSeconds = 300
	}
	argv = append(argv,
		"scan",
		"--json",
		"--metrics=off",
		"--disable-version-check",
		"--no-git-ignore",
		// Keep the rule ID the ruleset declares. Without this, Semgrep prefixes
		// IDs with the local rules path, so fingerprints and baselines would
		// change when the rules live in a different directory.
		"--no-rewrite-rule-ids",
		"--config", rules,
		"--timeout", strconv.Itoa(timeoutSeconds),
	)
	// Declared exclusions are part of the requested scope. Semgrep is told about
	// them so it does not spend time reading excluded trees; the same list is
	// recorded in scan.json under scope.excluded.
	if excludes := NormalizeExcludes(req.Scope.Excluded); len(excludes) > 0 {
		for _, pattern := range excludes {
			argv = append(argv, "--exclude", pattern)
		}
		exec.Diagnostics = append(exec.Diagnostics, "configured exclusions: "+SanitizeText(strings.Join(excludes, ", "), MaxDiagnosticBytes))
	}
	// A changed-files scope is an optimization and is recorded as such. When it
	// is requested, only the named files are targets.
	targets := []string{"."}
	if req.Scope.Mode == "changed-files" {
		targets = append([]string(nil), req.Scope.Files...)
	}
	argv = append(argv, targets...)

	runCtx, cancel := scanContext(ctx, a.timeout)
	defer cancel()
	stdout, stderr, exitCode, runErr := a.exec.Run(runCtx, req.WorkspaceRoot, argv, nil, a.maxOutput)
	exec.ExitCode = exitCode
	if runErr != nil {
		switch {
		case errors.Is(runErr, ErrUnavailable):
			exec.Status = StatusUnavailable
			exec.Diagnostics = []string{"semgrep executable is unavailable"}
		case errors.Is(runErr, context.DeadlineExceeded):
			exec.Status = StatusTimedOut
			exec.Coverage = CoverageUnknown
			exec.Diagnostics = []string{"semgrep exceeded the scan deadline"}
		case errors.Is(runErr, context.Canceled):
			exec.Status = StatusCancelled
			exec.Coverage = CoverageUnknown
			exec.Diagnostics = []string{"semgrep was cancelled"}
		default:
			exec.Status = StatusFailed
			exec.Diagnostics = []string{"semgrep execution failed: " + SanitizeText(runErr.Error(), MaxDiagnosticBytes)}
		}
		return finish(exec, nil)
	}
	if len(stderr) > 0 {
		exec.Diagnostics = append(exec.Diagnostics, SanitizeText(string(stderr), MaxDiagnosticBytes))
	}
	if len(stdout) == 0 {
		exec.Status = StatusFailed
		exec.Diagnostics = append(exec.Diagnostics, "semgrep returned empty output")
		return finish(exec, nil)
	}
	if a.maxOutput > 0 && len(stdout) >= a.maxOutput {
		exec.Status = StatusFailed
		exec.Diagnostics = append(exec.Diagnostics, fmt.Sprintf("semgrep output reached the %d-byte limit", a.maxOutput))
		return finish(exec, nil)
	}
	var report semgrepReport
	if err := json.Unmarshal(stdout, &report); err != nil {
		exec.Status = StatusFailed
		exec.Diagnostics = append(exec.Diagnostics, "semgrep returned malformed JSON: "+SanitizeText(err.Error(), MaxDiagnosticBytes))
		return finish(exec, nil)
	}
	// Semgrep's ordinary exit status is 1 when findings are present; a valid JSON
	// report is what distinguishes findings from a scanner failure.
	if exitCode != 0 && exitCode != 1 {
		exec.Status = StatusFailed
		exec.Diagnostics = append(exec.Diagnostics, fmt.Sprintf("semgrep exited with code %d", exitCode))
		return finish(exec, nil)
	}
	if report.Version != "" {
		exec.AdapterVersion = report.Version
		exec.Provenance.ScannerVersion = report.Version
	}

	exec.Coverage = CoverageComplete
	// Semgrep CE reports some coverage limits only in its human summary rather than
	// in the JSON paths object. Treat those explicit limits as PARTIAL so a required
	// gate never claims a full scan when files were skipped or the target set was
	// restricted to git-tracked files. A skip caused solely by a declared
	// --exclude is part of the requested scope, not a coverage gap.
	if semgrepCoverageGap(string(stderr)) {
		exec.Coverage = CoveragePartial
		reason := "semgrep reported skipped or restricted targets; coverage is partial"
		if detail := semgrepCoverageDetail(string(stderr)); detail != "" {
			reason += ": " + detail
		}
		exec.Diagnostics = append(exec.Diagnostics, reason)
	}
	for _, e := range report.Errors {
		exec.Diagnostics = append(exec.Diagnostics, SanitizeText("semgrep error: "+e.Type+": "+e.Message, MaxDiagnosticBytes))
		exec.Coverage = CoveragePartial
	}
	for _, skipped := range report.Paths.Skipped {
		path, reason := NormalizePath(skipped.Path)
		if path == "" {
			path = "."
		}
		exec.Skipped = append(exec.Skipped, SkippedFile{Path: path, Reason: SanitizeText(reason+" "+skipped.Reason, MaxDiagnosticBytes)})
		exec.Coverage = CoveragePartial
	}
	if req.Scope.Mode == "changed-files" {
		// The changed-file list is complete for the files it names, but it is not a
		// full-scan result. The limitation is explicit in coverage and diagnostics.
		if exec.Coverage == CoverageComplete {
			exec.Coverage = CoveragePartial
		}
		exec.Diagnostics = append(exec.Diagnostics, "changed-files scope: only the named changed files were inspected")
	}

	allowed := changedSet(req.Scope)
	for _, result := range report.Results {
		path, reason := NormalizePath(result.Path)
		if path == "" {
			exec.Diagnostics = append(exec.Diagnostics, "semgrep finding path rejected: "+reason)
			path = ""
		}
		if allowed != nil && path != "" && !allowed[path] {
			continue
		}
		fingerprint := strings.TrimSpace(result.Extra.Fingerprint)
		if fingerprint == "" || strings.EqualFold(fingerprint, "requires login") || strings.EqualFold(fingerprint, "unknown") {
			fingerprint = StableFingerprint("semgrep", result.CheckID, path, result.Start.Line, result.Start.Col)
		} else if !strings.HasPrefix(fingerprint, "semgrep:") {
			fingerprint = "semgrep:" + fingerprint
		}
		finding := Finding{
			AdapterID:        a.id,
			Provider:         "semgrep",
			Category:         CategorySAST,
			RuleID:           SanitizeText(result.CheckID, MaxMessageBytes),
			OriginalSeverity: SanitizeText(result.Extra.Severity, 64),
			Severity:         NormalizeSeverity("semgrep", result.Extra.Severity),
			Message:          SanitizeText(result.Extra.Message, MaxMessageBytes),
			Path:             path,
			Line:             result.Start.Line,
			EndLine:          result.End.Line,
			Column:           result.Start.Col,
			Fingerprint:      fingerprint,
			Metadata:         metadataStrings(result.Extra.Metadata),
		}
		if cwes := stringList(result.Extra.Metadata["cwe"]); len(cwes) > 0 {
			finding.CWE = cwes
		}
		if cves := stringList(result.Extra.Metadata["cve"]); len(cves) > 0 {
			finding.CVE = cves
		}
		if confidence, ok := result.Extra.Metadata["confidence"].(string); ok {
			finding.Confidence = SanitizeText(confidence, 64)
		}
		exec.Findings = append(exec.Findings, finding)
	}

	sanitized, digest, serr := SanitizeNativeReport(stdout)
	if serr == nil {
		exec.NativeReport = sanitized
		exec.NativeReportDigest = digest
		exec.Provenance.SanitizedReportDigest = digest
	} else {
		exec.Coverage = CoveragePartial
		exec.Diagnostics = append(exec.Diagnostics, "native report sanitization failed: "+SanitizeText(serr.Error(), MaxDiagnosticBytes))
	}
	sanitizeExecution(&exec)
	return finish(exec, nil)
}

// semgrepCoverageGap reports whether Semgrep's human summary names a
// scanner-side limitation. The generic "Scan skipped" heading is deliberately
// not a marker: it is also emitted for declared --exclude patterns.
func semgrepCoverageGap(stderr string) bool {
	for _, marker := range []string{
		"semgrepignore",
		"Files larger than",
		"Scan was limited to files tracked by git",
		"Partially analyzed",
		"couldn't be accessed",
	} {
		if strings.Contains(stderr, marker) {
			return true
		}
	}
	return false
}

// semgrepCoverageDetail extracts the human-summary skip reasons and counts so
// the persisted evidence says why coverage is partial even when the raw stderr
// diagnostic is truncated by the diagnostic bound.
func semgrepCoverageDetail(stderr string) string {
	var parts []string
	for _, line := range strings.Split(stderr, "\n") {
		t := strings.TrimSpace(line)
		t = strings.TrimPrefix(t, "• ")
		t = strings.TrimPrefix(t, "◦ ")
		switch {
		case strings.Contains(t, ".semgrepignore"),
			strings.Contains(t, "Files larger than"),
			strings.Contains(t, "Scan was limited to files tracked by git"),
			strings.Contains(t, "Partially analyzed"),
			strings.Contains(t, "couldn't be accessed"):
			parts = append(parts, SanitizeText(t, 256))
		}
	}
	return strings.Join(parts, "; ")
}

func changedSet(scope Scope) map[string]bool {
	if scope.Mode != "changed-files" {
		return nil
	}
	out := make(map[string]bool, len(scope.Files))
	for _, file := range scope.Files {
		if path, _ := NormalizePath(file); path != "" {
			out[path] = true
		}
	}
	return out
}

func metadataStrings(metadata map[string]any) map[string]string {
	if len(metadata) == 0 {
		return nil
	}
	out := make(map[string]string, len(metadata))
	for key, value := range metadata {
		switch v := value.(type) {
		case string:
			out[SanitizeText(key, 128)] = SanitizeText(v, MaxMessageBytes)
		case bool:
			out[SanitizeText(key, 128)] = strconv.FormatBool(v)
		case float64:
			out[SanitizeText(key, 128)] = strconv.FormatFloat(v, 'f', -1, 64)
		case []any:
			if values := stringList(v); len(values) > 0 {
				out[SanitizeText(key, 128)] = SanitizeText(strings.Join(values, ","), MaxMessageBytes)
			}
		}
	}
	return out
}

func stringList(value any) []string {
	switch v := value.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		return []string{SanitizeText(v, 256)}
	case []any:
		var out []string
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, SanitizeText(s, 256))
			}
		}
		return out
	case []string:
		out := make([]string, 0, len(v))
		for _, s := range v {
			if strings.TrimSpace(s) != "" {
				out = append(out, SanitizeText(s, 256))
			}
		}
		return out
	default:
		return nil
	}
}
