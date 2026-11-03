package hipcompare

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/imhttran/agentic-sop/internal/redact"
)

// ResultsFileName is the machine-readable comparison result. It is written beside
// the comparison directory following the evidence artifact layout.
const ResultsFileName = "comparison.json"

// ComparisonMarkdownFileName is the short Markdown comparison.
const ComparisonMarkdownFileName = "comparison.md"

// WriteArtifacts writes the machine-readable comparison and the short Markdown
// comparison into dir. It redacts credential-like values before writing, so a
// credential present in the process environment never reaches an artifact.
func WriteArtifacts(dir string, cmp Comparison) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create comparison dir: %w", err)
	}
	redacted := redactComparison(cmp)
	data, err := json.MarshalIndent(redacted, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal comparison: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, ResultsFileName), append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", ResultsFileName, err)
	}
	md := compareMarkdown(redacted)
	if err := os.WriteFile(filepath.Join(dir, ComparisonMarkdownFileName), []byte(md), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", ComparisonMarkdownFileName, err)
	}
	return nil
}

// redactComparison scrubs any value that looks like a credential from the
// comparison. Credentials never belong in an artifact; only secrets are removed.
func redactComparison(cmp Comparison) Comparison {
	cmp.TaskID = redactText(cmp.TaskID)
	cmp.StartingCommit = redactText(cmp.StartingCommit)
	for i := range cmp.Arms {
		cmp.Arms[i].Failures = redactAll(cmp.Arms[i].Failures)
		cmp.Arms[i].EvidencePaths = redactAll(cmp.Arms[i].EvidencePaths)
		cmp.Arms[i].ChangedPaths = redactAll(cmp.Arms[i].ChangedPaths)
		cmp.Arms[i].InstructionsApplied = redactAll(cmp.Arms[i].InstructionsApplied)
		cmp.Arms[i].Output = redactText(cmp.Arms[i].Output)
		cmp.Arms[i].LimitFailureReason = redactText(cmp.Arms[i].LimitFailureReason)
	}
	return cmp
}

func redactAll(in []string) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = redactText(s)
	}
	return out
}

// credentialEnvNames maps clearly-secret variable name prefixes to true. Only
// these names are treated as credentials, so innocuous values such as SOP_LOG or
// AWS_REGION are never mistaken for a secret and corrupted in an artifact.
var credentialEnvPrefixes = []string{
	"SOP_API_KEY", "SOP_TOKEN", "SOP_SECRET", "SOP_PASSWORD",
	"OPENAI_API_KEY", "ANTHROPIC_API_KEY", "AZURE_OPENAI_KEY",
	"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY",
	"GOOGLE_API_KEY", "GEMINI_API_KEY", "GITHUB_TOKEN", "GH_TOKEN",
	"API_KEY", "SECRET", "PASSWORD", "ACCESS_KEY",
}

// redactText removes credential-shaped values, and any environment value supplied by a
// clearly-secret variable name, from s. The shapes are decided by internal/redact, so a
// shape one surface learns is a shape this artifact redacts too.
//
// The environment list stays local: it selects which variables to scrub from an
// experimental agent's own environment (isSecretEnvName), which is a narrower question
// than "does this text look like a credential".
func redactText(s string) string {
	if s == "" {
		return s
	}
	s = redact.ShapesValues(s, redact.Fixed("<redacted>"))
	for _, envValue := range credentialEnvValues() {
		s = strings.ReplaceAll(s, envValue, "<redacted>")
	}
	return s
}

// isSecretEnvName reports whether name is a clearly-secret environment variable.
// It is the source of truth for the arm executor's environment scrubber, so a
// credential is never handed to an experimental agent. Artifact text is redacted by
// shape instead (internal/redact), which also catches a credential whose name this
// list does not know.
func isSecretEnvName(name string) bool {
	upper := strings.ToUpper(name)
	for _, p := range credentialEnvPrefixes {
		if upper == p || strings.HasPrefix(upper, p+"_") || strings.HasPrefix(upper, p) {
			return true
		}
	}
	return false
}

// credentialEnvValues returns the value of every environment variable whose name is
// on the clearly-secret list. A short value is skipped to avoid false matches on
// common small strings; a real credential is longer.
func credentialEnvValues() []string {
	var out []string
	seen := map[string]bool{}
	for _, kv := range os.Environ() {
		i := strings.IndexByte(kv, '=')
		if i < 0 {
			continue
		}
		name, value := kv[:i], kv[i+1:]
		if len(value) < 4 || seen[value] {
			continue
		}
		if isSecretEnvName(name) {
			seen[value] = true
			out = append(out, value)
		}
	}
	// Longest first, so a credential that is a substring of another is still removed
	// completely.
	sort.Slice(out, func(i, j int) bool { return len(out[i]) > len(out[j]) })
	return out
}

// compareMarkdown renders a short side-by-side comparison of both arms.
func compareMarkdown(cmp Comparison) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# HIP-003 comparison: %s\n\n", cmp.TaskID)
	fmt.Fprintf(&b, "- Starting commit: `%s`\n", cmp.StartingCommit)
	fmt.Fprintf(&b, "- Fingerprint: `%s`\n", cmp.Fingerprint.Digest)
	fmt.Fprintf(&b, "- Budget: %d pair(s) planned (max %d), %d held-out pair(s) (max %d)\n",
		cmp.Budget.Pairs, MaxPairs, cmp.Budget.HeldOutPairs, MaxHeldOutPairs)
	fmt.Fprintf(&b, "- Executions: %d used of this budget's ceiling %d (absolute maximum %d)\n",
		cmp.Executions, cmp.Budget.ExecutionCeiling(), MaxExecutions)
	fmt.Fprintf(&b, "- Per run: %ds time limit (enforced), %d token limit (enforced only when usage is reported); %d automatic retries\n\n",
		cmp.Budget.PerRunTimeSeconds, cmp.Budget.PerRunTokenLimit, cmp.Budget.AutomaticRetries)

	b.WriteString("| Metric | Baseline | Candidate |\n")
	b.WriteString("| --- | --- | --- |\n")
	base, cand := findArm(cmp.Arms, ArmBaseline), findArm(cmp.Arms, ArmCandidate)
	writeRow := func(label, baseVal, candVal string) {
		fmt.Fprintf(&b, "| %s | %s | %s |\n", label, baseVal, candVal)
	}
	writeRow("Completed", boolText(base, func(a ArmResult) string { return boolString(a.Completed) }), boolText(cand, func(a ArmResult) string { return boolString(a.Completed) }))
	writeRow("Acceptance", boolText(base, func(a ArmResult) string { return boolString(a.AcceptancePassed) }), boolText(cand, func(a ArmResult) string { return boolString(a.AcceptancePassed) }))
	writeRow("Human interventions", boolText(base, func(a ArmResult) string { return fmt.Sprintf("%d", a.HumanInterventions) }), boolText(cand, func(a ArmResult) string { return fmt.Sprintf("%d", a.HumanInterventions) }))
	writeRow("Tokens", boolText(base, func(a ArmResult) string { return tokensText(a) }), boolText(cand, func(a ArmResult) string { return tokensText(a) }))
	writeRow("Elapsed (ms)", boolText(base, func(a ArmResult) string { return fmt.Sprintf("%d", a.ElapsedMillis) }), boolText(cand, func(a ArmResult) string { return fmt.Sprintf("%d", a.ElapsedMillis) }))
	writeRow("Failures", boolText(base, func(a ArmResult) string { return failuresText(a) }), boolText(cand, func(a ArmResult) string { return failuresText(a) }))
	writeRow("Evidence", boolText(base, func(a ArmResult) string { return evidenceText(a) }), boolText(cand, func(a ArmResult) string { return evidenceText(a) }))

	b.WriteString("\n## Result\n\n")
	b.WriteString(verdict(cmp))
	b.WriteString("\n")
	return b.String()
}

func findArm(arms []ArmResult, arm Arm) *ArmResult {
	for i := range arms {
		if arms[i].Arm == arm {
			return &arms[i]
		}
	}
	return nil
}

func boolText(a *ArmResult, f func(ArmResult) string) string {
	if a == nil {
		return "NOT RUN"
	}
	return f(*a)
}

func boolString(v bool) string {
	if v {
		return "yes"
	}
	return "no"
}

func failuresText(a ArmResult) string {
	if len(a.Failures) == 0 {
		return "none"
	}
	return strings.Join(a.Failures, "; ")
}

func tokensText(a ArmResult) string {
	if !a.TokensReported {
		return "UNAVAILABLE"
	}
	return fmt.Sprintf("%d", a.TokensUsed)
}

func evidenceText(a ArmResult) string {
	if len(a.EvidencePaths) == 0 {
		return "none"
	}
	return strings.Join(a.EvidencePaths, "<br>")
}

// verdict summarizes the comparison outcome in one short line. Missing telemetry
// makes the comparison inconclusive; it is never substituted with zero.
func verdict(cmp Comparison) string {
	base, cand := findArm(cmp.Arms, ArmBaseline), findArm(cmp.Arms, ArmCandidate)
	switch {
	case base == nil || cand == nil:
		return "Inconclusive: one arm did not run, so no pair is comparable."
	case base.LimitFailure || cand.LimitFailure:
		return "Inconclusive: an arm hit a per-run limit, so the pair is incomplete."
	case !base.AcceptancePassed || !cand.AcceptancePassed:
		return "Candidate not eligible: at least one arm failed the fixed acceptance checks."
	default:
		return "Both arms passed the fixed acceptance checks; review the cost metrics above before promoting the candidate."
	}
}
