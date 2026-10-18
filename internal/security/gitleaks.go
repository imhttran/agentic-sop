package security

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// gitleaksAdapter wraps the Gitleaks CLI. It scans local files (no Git history),
// asks Gitleaks to redact, and additionally sanitizes the native report before it
// can be retained. Raw secret values never enter SOP's normalized evidence.
type gitleaksAdapter struct {
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

func (a *gitleaksAdapter) Descriptor() Descriptor {
	return Descriptor{
		ProtocolVersion: ProtocolVersion,
		AdapterID:       a.id,
		AdapterVersion:  a.version,
		Categories:      []Category{a.category},
		ScopeModes:      []string{"workspace", "changed-files"},
		Locality:        "local",
		Network:         "none required for local detection; no verified-secret service is used",
	}
}

func (a *gitleaksAdapter) currentProvenance(req Request) (Provenance, error) {
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

type gitleaksFinding struct {
	Description string   `json:"Description"`
	File        string   `json:"File"`
	StartLine   int      `json:"StartLine"`
	EndLine     int      `json:"EndLine"`
	StartColumn int      `json:"StartColumn"`
	EndColumn   int      `json:"EndColumn"`
	Match       string   `json:"Match"`
	Secret      string   `json:"Secret"`
	Entropy     float64  `json:"Entropy"`
	Tags        []string `json:"Tags"`
	RuleID      string   `json:"RuleID"`
	Fingerprint string   `json:"Fingerprint"`
}

func (a *gitleaksAdapter) Scan(ctx context.Context, req Request) (Execution, error) {
	exec := newExecution(a.id, a.category, a.version, a.configDigest, "", req)
	if req.Scope.Mode == "changed-files" && len(req.Scope.Files) == 0 {
		exec.Coverage = CoverageComplete
		exec.Diagnostics = []string{"no changed files to scan"}
		return finish(exec, nil)
	}

	tempDir, err := os.MkdirTemp("", "sop-gitleaks-*")
	if err != nil {
		exec.Status = StatusFailed
		exec.Diagnostics = []string{"create restricted report directory: " + SanitizeText(err.Error(), MaxDiagnosticBytes)}
		return finish(exec, nil)
	}
	defer os.RemoveAll(tempDir)
	reportPath := filepath.Join(tempDir, "report.json")
	viewDir := filepath.Join(tempDir, "view")
	matcher := NewExcludeMatcher(req.Scope.Excluded)
	if patterns := matcher.Patterns(); len(patterns) > 0 {
		exec.Diagnostics = append(exec.Diagnostics, "configured exclusions: "+SanitizeText(strings.Join(patterns, ", "), MaxDiagnosticBytes))
	}
	view, verr := stageGitleaksView(req.WorkspaceRoot, viewDir, matcher)
	if verr != nil {
		exec.Status = StatusFailed
		exec.Coverage = CoverageUnknown
		exec.Diagnostics = append(exec.Diagnostics, "gitleaks exclusions could not be staged: "+SanitizeText(verr.Error(), MaxDiagnosticBytes))
		return finish(exec, nil)
	}
	if view.excludedFiles > 0 || view.excludedDirs > 0 {
		exec.Diagnostics = append(exec.Diagnostics, fmt.Sprintf("gitleaks scanned a staged view: %d file(s) and %d director(ies) outside the declared scope were not staged", view.excludedFiles, view.excludedDirs))
	}
	if view.skippedSymlinks > 0 || view.skippedOther > 0 {
		exec.Diagnostics = append(exec.Diagnostics, fmt.Sprintf("gitleaks staged view skipped %d symbolic link(s) and %d other non-regular entry(ies); coverage is partial", view.skippedSymlinks, view.skippedOther))
	}
	roots := viewRoots(viewDir)

	argv := append([]string(nil), a.command...)
	if len(argv) == 0 {
		argv = []string{"gitleaks"}
	}
	argv = append(argv,
		"detect",
		"--source", viewDir,
		"--no-git",
		"--redact",
		"--report-format", "json",
		"--report-path", reportPath,
		"--exit-code", "0",
		"--no-banner",
	)
	if strings.TrimSpace(a.rules) != "" {
		rules := a.rules
		if !isAbsPath(rules) {
			rules = joinPath(req.WorkspaceRoot, rules)
		}
		argv = append(argv, "--config", rules)
		identity, err := rulesIdentity(rules)
		if err != nil {
			exec.Status = StatusFailed
			exec.Diagnostics = []string{"gitleaks configuration is unavailable: " + SanitizeText(err.Error(), MaxDiagnosticBytes)}
			return finish(exec, nil)
		}
		exec.Provenance.RulesIdentity = identity
	}

	runCtx, cancel := scanContext(ctx, a.timeout)
	defer cancel()
	stdout, stderr, exitCode, runErr := a.exec.Run(runCtx, viewDir, argv, nil, a.maxOutput)
	exec.ExitCode = exitCode
	if runErr != nil {
		switch {
		case errors.Is(runErr, ErrUnavailable):
			exec.Status = StatusUnavailable
			exec.Diagnostics = []string{"gitleaks executable is unavailable"}
		case errors.Is(runErr, context.DeadlineExceeded):
			exec.Status = StatusTimedOut
			exec.Diagnostics = []string{"gitleaks exceeded the scan deadline"}
		case errors.Is(runErr, context.Canceled):
			exec.Status = StatusCancelled
			exec.Diagnostics = []string{"gitleaks was cancelled"}
		default:
			exec.Status = StatusFailed
			exec.Diagnostics = []string{"gitleaks execution failed: " + SanitizeText(runErr.Error(), MaxDiagnosticBytes)}
		}
		return finish(exec, nil)
	}
	if len(stderr) > 0 {
		exec.Diagnostics = append(exec.Diagnostics, SanitizeText(string(sanitizeViewPaths(stderr, roots)), MaxDiagnosticBytes))
	}
	if len(stdout) > 0 {
		exec.Diagnostics = append(exec.Diagnostics, SanitizeText(string(sanitizeViewPaths(stdout, roots)), MaxDiagnosticBytes))
	}
	if exitCode != 0 && exitCode != 1 {
		exec.Status = StatusFailed
		exec.Diagnostics = append(exec.Diagnostics, fmt.Sprintf("gitleaks exited with code %d", exitCode))
		return finish(exec, nil)
	}

	raw, err := readBoundedFile(reportPath, a.maxOutput)
	if err != nil {
		exec.Status = StatusFailed
		exec.Coverage = CoverageUnknown
		exec.Diagnostics = append(exec.Diagnostics, "gitleaks report is unavailable: "+SanitizeText(err.Error(), MaxDiagnosticBytes))
		return finish(exec, nil)
	}
	findings, secrets, err := parseGitleaksReport(raw)
	if err != nil {
		exec.Status = StatusFailed
		exec.Coverage = CoverageUnknown
		exec.Diagnostics = append(exec.Diagnostics, "gitleaks returned a malformed report: "+SanitizeText(err.Error(), MaxDiagnosticBytes))
		return finish(exec, nil)
	}

	exec.Coverage = CoverageComplete
	if view.skippedSymlinks > 0 || view.skippedOther > 0 {
		exec.Coverage = CoveragePartial
	}
	allowed := changedSet(req.Scope)
	omitted := 0
	excludedFindings := 0
	for _, gf := range findings {
		path, ok := viewRelativePath(gf.File, roots)
		var reason string
		if !ok {
			path, reason = NormalizePath(gf.File)
			if filepath.IsAbs(gf.File) {
				path, reason = relativeFindingPath(req.WorkspaceRoot, gf.File)
			}
		}
		if path == "" {
			exec.Diagnostics = append(exec.Diagnostics, "gitleaks finding path rejected: "+reason)
		}
		if path != "" && matcher.Excluded(path) {
			// The staged view should already omit excluded paths. Keep this guard so
			// a scanner-side path form change cannot leak an excluded finding.
			excludedFindings++
			continue
		}
		if allowed != nil && path != "" && !allowed[path] {
			omitted++
			continue
		}
		ruleID := firstNonEmpty(gf.RuleID, "generic-secret")
		// Gitleaks fingerprints can contain the absolute source path. Use a stable
		// repository-relative fingerprint instead, so the identifier is reproducible
		// and never leaks host paths.
		fingerprint := StableFingerprint("gitleaks", ruleID, path, gf.StartLine, gf.StartColumn)
		message := firstNonEmpty(gf.Description, "detected secret")
		message = RedactValues(message, secrets)
		metadata := map[string]string{}
		if len(gf.Tags) > 0 {
			metadata["tags"] = SanitizeText(strings.Join(gf.Tags, ","), MaxMessageBytes)
		}
		if gf.Entropy != 0 {
			metadata["entropy"] = strconv.FormatFloat(gf.Entropy, 'f', -1, 64)
		}
		if len(metadata) == 0 {
			metadata = nil
		}
		exec.Findings = append(exec.Findings, Finding{
			AdapterID:        a.id,
			Provider:         "gitleaks",
			Category:         CategorySecrets,
			RuleID:           SanitizeText(ruleID, MaxMessageBytes),
			OriginalSeverity: "secret",
			Severity:         SeverityHigh,
			Message:          SanitizeText(message, MaxMessageBytes),
			Path:             path,
			Line:             gf.StartLine,
			EndLine:          gf.EndLine,
			Column:           gf.StartColumn,
			Fingerprint:      fingerprint,
			Metadata:         metadata,
		})
	}
	if req.Scope.Mode == "changed-files" {
		exec.Coverage = CoveragePartial
		exec.Diagnostics = append(exec.Diagnostics, fmt.Sprintf("changed-files scope: gitleaks scanned the workspace and %d finding(s) outside the changed set were omitted", omitted))
	}
	if excludedFindings > 0 {
		exec.Diagnostics = append(exec.Diagnostics, fmt.Sprintf("gitleaks: %d finding(s) in declared excluded paths were omitted from the normalized report", excludedFindings))
	}

	sanitized, digest, serr := SanitizeNativeReport(sanitizeViewPaths(raw, roots))
	if serr == nil {
		exec.NativeReport = sanitized
		exec.NativeReportDigest = digest
		exec.Provenance.SanitizedReportDigest = digest
	} else {
		exec.Coverage = CoveragePartial
		exec.Diagnostics = append(exec.Diagnostics, "native report sanitization failed: "+SanitizeText(serr.Error(), MaxDiagnosticBytes))
	}
	exec.Diagnostics = sanitizeList(exec.Diagnostics)
	// Remove every exact secret value from captured diagnostics as a final guard.
	for i := range exec.Diagnostics {
		exec.Diagnostics[i] = RedactValues(exec.Diagnostics[i], secrets)
	}
	sanitizeExecution(&exec)
	return finish(exec, nil)
}

// parseGitleaksReport accepts the two report shapes distributed by Gitleaks
// versions: a top-level array, or an object with a findings array.
func parseGitleaksReport(raw []byte) ([]gitleaksFinding, []string, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil, nil, fmt.Errorf("report is empty")
	}
	var findings []gitleaksFinding
	if strings.HasPrefix(trimmed, "[") {
		if err := json.Unmarshal([]byte(trimmed), &findings); err != nil {
			return nil, nil, err
		}
	} else {
		var wrapper struct {
			Findings []gitleaksFinding `json:"findings"`
		}
		if err := json.Unmarshal([]byte(trimmed), &wrapper); err != nil {
			return nil, nil, err
		}
		findings = wrapper.Findings
	}
	var secrets []string
	for _, finding := range findings {
		if strings.TrimSpace(finding.Secret) != "" {
			secrets = append(secrets, finding.Secret)
		}
		if strings.TrimSpace(finding.Match) != "" {
			secrets = append(secrets, finding.Match)
		}
	}
	return findings, secrets, nil
}

func readBoundedFile(path string, limit int) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if limit > 0 && info.Size() > int64(limit) {
		return nil, fmt.Errorf("report size %d exceeds the %d-byte limit", info.Size(), limit)
	}
	return os.ReadFile(path)
}
