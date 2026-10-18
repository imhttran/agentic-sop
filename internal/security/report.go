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

// Scan is one complete security stage result: independent adapter executions,
// normalized findings, content identity, policy, and verdict.
type Scan struct {
	ProtocolVersion string          `json:"protocol_version"`
	RunID           string          `json:"run_id"`
	TaskID          string          `json:"task_id,omitempty"`
	StartedAt       time.Time       `json:"started_at"`
	EndedAt         time.Time       `json:"ended_at"`
	Content         ContentIdentity `json:"content"`
	Scope           Scope           `json:"scope"`
	Policy          Policy          `json:"policy"`
	Adapters        []Execution     `json:"adapters"`
	Findings        []Finding       `json:"findings"`
	Verdict         Verdict         `json:"verdict"`
	Coverage        Coverage        `json:"coverage"`
	Reasons         []string        `json:"reasons"`
	Diagnostics     []string        `json:"diagnostics,omitempty"`
	Metrics         Metrics         `json:"metrics"`
}

// Metrics records measured duration and counts. It is diagnostic evidence; it
// never feeds the verdict.
type Metrics struct {
	DurationMS        int64            `json:"duration_ms"`
	AdapterDurationMS map[string]int64 `json:"adapter_duration_ms,omitempty"`
	Findings          int              `json:"findings"`
	Suppressed        int              `json:"suppressed"`
	ExecutionFailures int              `json:"execution_failures"`
}

// Identity returns the provenance identity for a policy plus its baseline.
func (p Policy) Identity(baseline Baseline) string {
	cats := append([]string(nil), p.RequiredCategories...)
	for i := range cats {
		cats[i] = strings.ToLower(strings.TrimSpace(cats[i]))
	}
	failOn := append([]string(nil), p.FailOn...)
	for i := range failOn {
		failOn[i] = strings.ToUpper(strings.TrimSpace(failOn[i]))
	}
	sort.Strings(cats)
	sort.Strings(failOn)
	enabled := "disabled"
	if p.Enabled {
		enabled = "enabled"
	}
	return DigestStrings(enabled, p.Mode, p.Scope, strings.Join(cats, ","), strings.Join(failOn, ","), p.UnknownSeverity, fmt.Sprintf("%d", p.TimeoutSeconds), fmt.Sprintf("%d", p.MaxOutputBytes), baseline.Identity())
}

// ArtifactRefs are the repository-relative artifact paths written for a scan.
type ArtifactRefs struct {
	ScanJSON     string            `json:"scan_json"`
	ScanMarkdown string            `json:"scan_markdown"`
	Native       map[string]string `json:"native,omitempty"`
}

// WriteArtifacts persists the normalized report, a Markdown rendering, and each
// sanitized native report under <runDir>/security. Raw reports are never written:
// an adapter has already sanitized NativeReport, and a nil native report is
// simply omitted.
func WriteArtifacts(runDir string, scan Scan) (ArtifactRefs, error) {
	dir := filepath.Join(runDir, "security")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ArtifactRefs{}, err
	}
	refs := ArtifactRefs{Native: map[string]string{}}
	used := map[string]int{}
	jsonPath := filepath.Join(dir, "scan.json")
	data, err := json.MarshalIndent(scan, "", "  ")
	if err != nil {
		return refs, err
	}
	data = append(data, '\n')
	if err := os.WriteFile(jsonPath, data, 0o644); err != nil {
		return refs, err
	}
	refs.ScanJSON = filepath.ToSlash(filepath.Join("security", "scan.json"))

	mdPath := filepath.Join(dir, "scan.md")
	if err := os.WriteFile(mdPath, []byte(RenderMarkdown(scan)), 0o644); err != nil {
		return refs, err
	}
	refs.ScanMarkdown = filepath.ToSlash(filepath.Join("security", "scan.md"))

	for _, adapter := range scan.Adapters {
		if len(adapter.NativeReport) == 0 {
			continue
		}
		base := safeArtifactName(adapter.AdapterID)
		name := base + ".native.json"
		if used[name] > 0 {
			name = fmt.Sprintf("%s-%d.native.json", base, used[name]+1)
		}
		used[name]++
		if err := os.WriteFile(filepath.Join(dir, name), adapter.NativeReport, 0o644); err != nil {
			return refs, err
		}
		refs.Native[adapter.AdapterID] = filepath.ToSlash(filepath.Join("security", name))
	}
	return refs, nil
}

// RenderMarkdown renders the human-readable security report. Report text is
// bounded, but it remains untrusted data: it is never interpreted as instructions.
func RenderMarkdown(scan Scan) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Security scan: %s\n\n", scan.RunID)
	fmt.Fprintf(&b, "- Verdict: `%s`\n", scan.Verdict)
	fmt.Fprintf(&b, "- Coverage: `%s`\n", scan.Coverage)
	fmt.Fprintf(&b, "- Mode: `%s`\n", scan.Policy.Mode)
	fmt.Fprintf(&b, "- Scope: `%s`\n", scan.Scope.Mode)
	if scan.Content.Commit != "" {
		fmt.Fprintf(&b, "- Commit: `%s`\n", scan.Content.Commit)
	}
	if scan.Content.Tree != "" {
		fmt.Fprintf(&b, "- Tree: `%s`\n", scan.Content.Tree)
	}
	if scan.Content.Dirty {
		fmt.Fprintf(&b, "- Working tree: DIRTY (content digest `%s`)\n", scan.Content.Digest)
	} else {
		b.WriteString("- Working tree: CLEAN\n")
	}
	fmt.Fprintf(&b, "- Duration: %dms\n", scan.Metrics.DurationMS)

	b.WriteString("\n## Adapters\n\n")
	if len(scan.Adapters) == 0 {
		b.WriteString("No adapters ran.\n")
	}
	for _, adapter := range scan.Adapters {
		fmt.Fprintf(&b, "- `%s` (%s): `%s`, coverage `%s`, exit `%d`, duration `%dms`",
			adapter.AdapterID, adapter.Category, adapter.Status, adapter.Coverage, adapter.ExitCode,
			scan.Metrics.AdapterDurationMS[adapter.AdapterID])
		if adapter.Reused {
			b.WriteString(", reused")
		}
		b.WriteString("\n")
		for _, diagnostic := range adapter.Diagnostics {
			fmt.Fprintf(&b, "  - Diagnostic: %s\n", SanitizeText(diagnostic, MaxDiagnosticBytes))
		}
	}

	b.WriteString("\n## Findings\n\n")
	if len(scan.Findings) == 0 {
		b.WriteString("No findings.\n")
	}
	for _, finding := range scan.Findings {
		location := finding.Path
		if finding.Line > 0 {
			location = fmt.Sprintf("%s:%d", finding.Path, finding.Line)
		}
		if location == "" {
			location = "(no location)"
		}
		status := ""
		if finding.Suppressed {
			status = " [suppressed]"
		} else if finding.Suppression != nil && finding.Suppression.Expired {
			status = " [expired suppression]"
		}
		fmt.Fprintf(&b, "- %s %s `%s` — %s%s\n", finding.Severity, location, finding.RuleID, SanitizeText(finding.Message, MaxMessageBytes), status)
	}

	b.WriteString("\n## Verdict\n\n")
	for _, reason := range scan.Reasons {
		fmt.Fprintf(&b, "- %s\n", SanitizeText(reason, MaxDiagnosticBytes))
	}
	for _, diagnostic := range scan.Diagnostics {
		fmt.Fprintf(&b, "- Diagnostic: %s\n", SanitizeText(diagnostic, MaxDiagnosticBytes))
	}
	return b.String()
}

func safeArtifactName(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return "adapter"
	}
	var b strings.Builder
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}
