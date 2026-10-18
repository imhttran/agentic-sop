package skill

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestSecurityScanSkillContract proves the shipped /sop-security-scan skill
// orchestrates detection, selection, report-only scanning, triage, and evidence
// without executing a scanner directly or weakening a gate.
func TestSecurityScanSkillContract(t *testing.T) {
	body := normalizeSpaces(read(t, filepath.Join(skillsRoot, "sop-security-scan", "SKILL.md")))
	for _, want := range []string{
		"disable-model-invocation: true",
		"## PREFLIGHT", "## DETECT", "## CONFIGURE", "## SELECT", "## SCAN",
		"## TRIAGE", "## REPORT", "## RUST REQUIREMENTS", "## BOUNDARIES", "## EXTENSION",
		"sop security inspect --json", "sop security scan --auto --report-only --json",
		"sop approvals --json",
		"available", "unavailable", "unsupported", "proposed",
		"Gitleaks", "Semgrep", "cargo-audit",
		"Cargo.lock", "advisory database", "stale", "--no-fetch",
		"report-only", "required_mode", "required mode",
		"Never reuse Go rules for Rust source",
		"Do not quote a detected secret value",
		"Do not suppress findings", "Do not install tools",
		"Do not launch automatic fixes", "Do not alter shell authorization",
		"Never merge, delete branches, or delete worktrees",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("security-scan contract missing %q", want)
		}
	}

	for _, line := range bashCommandLines(read(t, filepath.Join(skillsRoot, "sop-security-scan", "SKILL.md"))) {
		if !strings.HasPrefix(line, "sop ") {
			t.Errorf("security-scan shell example does not invoke sop: %q", line)
		}
	}
	for _, forbidden := range append([]string{
		"fail_on", "max_fix_cycles", "sed -i", "rm -rf", "git commit", "git push", "git add",
	}, ModelLiterals...) {
		if strings.Contains(strings.ToLower(body), forbidden) {
			t.Errorf("security-scan skill contains %q", forbidden)
		}
	}
}
