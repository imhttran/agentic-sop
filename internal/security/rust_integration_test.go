//go:build security_integration

package security

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/config"
)

// rustRuleIDs are the rules the repository-owned Rust ruleset declares. Each one
// has a matching positive fixture item and a safe counterpart.
var rustRuleIDs = []string{
	"sop-rust-command-injection-env",
	"sop-rust-tls-verification-disabled",
	"sop-rust-unsafe-block",
}

func integrationRulesPath(t *testing.T, name string) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("testdata", "real", name))
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func integrationFixtureRoot(t *testing.T, name string) string {
	t.Helper()
	root := t.TempDir()
	copyFixtureTree(t, filepath.Join("testdata", "real", name), root)
	return root
}

// runRealSemgrep executes the real Semgrep CLI through the production adapter
// and runner, so coverage, provenance, and normalization are exercised end to
// end rather than through a fake executor.
func runRealSemgrep(t *testing.T, root, rules, runID string) Scan {
	t.Helper()
	semgrepBin := integrationBinary(t, "SOP_SECURITY_SEMGREP_BIN", "semgrep")
	version := toolVersion(t, semgrepBin, "--version")
	adapters, err := BuildAdapters([]config.SecurityAdapterConfig{{
		ID: "semgrep", Category: "sast", Protocol: "builtin",
		Command: []string{semgrepBin}, Rules: rules, Version: version,
	}}, BuildOptions{Timeout: 2 * time.Minute, MaxOutputBytes: 4 << 20})
	if err != nil {
		t.Fatal(err)
	}
	runner, err := NewRunner(RunnerConfig{
		WorkspaceRoot: root, RunID: runID, Adapters: adapters,
		Policy:  Policy{Enabled: true, Mode: "report-only", Scope: "workspace", FailOn: []string{"critical", "high"}, UnknownSeverity: "indeterminate"},
		Timeout: 2 * time.Minute, MaxOutputBytes: 4 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	scan, err := runner.Scan(context.Background())
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(scan.Adapters) != 1 {
		t.Fatalf("executions = %d, want 1", len(scan.Adapters))
	}
	return scan
}

func findingRuleIDs(findings []Finding) []string {
	out := make([]string, 0, len(findings))
	for _, finding := range findings {
		out = append(out, finding.RuleID)
	}
	sort.Strings(out)
	return out
}

func sha256File(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// TestRealSemgrepRustRulesetDetectsAndCleans proves the repository-owned Rust
// ruleset is usable by the pinned Semgrep CE version: every intended rule
// produces a real finding on the seeded fixture, the safe fixture stays clean,
// and the rules digest is recorded as provenance.
func TestRealSemgrepRustRulesetDetectsAndCleans(t *testing.T) {
	rules := integrationRulesPath(t, "rules-rust.yaml")

	seeded := runRealSemgrep(t, integrationFixtureRoot(t, "rust-seeded"), rules, "rust-seeded")
	exec := seeded.Adapters[0]
	if exec.Status != StatusCompleted {
		t.Fatalf("seeded status = %s, want COMPLETED: %v", exec.Status, exec.Diagnostics)
	}
	if exec.Coverage != CoverageComplete {
		t.Fatalf("seeded coverage = %s, want COMPLETE: %v", exec.Coverage, exec.Diagnostics)
	}
	if got := findingRuleIDs(exec.Findings); strings.Join(got, ",") != strings.Join(rustRuleIDs, ",") {
		t.Fatalf("seeded rule IDs = %v, want %v", got, rustRuleIDs)
	}
	for _, finding := range exec.Findings {
		if !strings.HasSuffix(finding.Path, ".rs") {
			t.Errorf("finding %s path = %q, want Rust source", finding.RuleID, finding.Path)
		}
		if finding.Severity == SeverityUnknown {
			t.Errorf("finding %s has unknown severity (original %q)", finding.RuleID, finding.OriginalSeverity)
		}
	}
	if want := sha256File(t, rules); exec.Provenance.RulesIdentity != want {
		t.Errorf("rules identity = %q, want %q", exec.Provenance.RulesIdentity, want)
	}
	if exec.Provenance.RulesIdentity == "" {
		t.Error("rules identity is empty; provenance does not identify the Rust ruleset")
	}

	clean := runRealSemgrep(t, integrationFixtureRoot(t, "rust-clean"), rules, "rust-clean")
	cleanExec := clean.Adapters[0]
	if cleanExec.Status != StatusCompleted {
		t.Fatalf("clean status = %s, want COMPLETED: %v", cleanExec.Status, cleanExec.Diagnostics)
	}
	if cleanExec.Coverage != CoverageComplete {
		t.Fatalf("clean coverage = %s, want COMPLETE: %v", cleanExec.Coverage, cleanExec.Diagnostics)
	}
	if len(cleanExec.Findings) != 0 {
		t.Fatalf("clean fixture produced %d finding(s): %+v", len(cleanExec.Findings), cleanExec.Findings)
	}
}

// TestRealSemgrepRustUnsupportedWithoutRustRules proves a Go-only ruleset never
// yields a clean Rust result and never applies a Go rule to Rust source.
func TestRealSemgrepRustUnsupportedWithoutRustRules(t *testing.T) {
	scan := runRealSemgrep(t, integrationFixtureRoot(t, "rust-seeded"), integrationRulesPath(t, "rules.yaml"), "rust-go-only")
	exec := scan.Adapters[0]
	if exec.Status != StatusFailed || exec.Coverage != CoverageUnknown {
		t.Fatalf("status/coverage = %s/%s, want FAILED/UNKNOWN: %v", exec.Status, exec.Coverage, exec.Diagnostics)
	}
	diagnostics := strings.Join(exec.Diagnostics, " ")
	if !strings.Contains(diagnostics, "rust") || !strings.Contains(diagnostics, "UNSUPPORTED") {
		t.Errorf("diagnostics = %v, want explicit unsupported Rust analysis", exec.Diagnostics)
	}
	if len(exec.Findings) != 0 {
		t.Errorf("Go-only rules produced %d finding(s) against Rust source: %+v", len(exec.Findings), exec.Findings)
	}
}

// TestRealSemgrepMixedLanguageCoverage proves Go and Rust coverage stay
// independent: a mixed ruleset is complete, and a Go-only ruleset is explicitly
// partial while still analyzing the Go half.
func TestRealSemgrepMixedLanguageCoverage(t *testing.T) {
	mixed := integrationFixtureRoot(t, "rust-mixed")

	full := runRealSemgrep(t, mixed, integrationRulesPath(t, "rules-mixed.yaml"), "mixed-full")
	fullExec := full.Adapters[0]
	if fullExec.Status != StatusCompleted || fullExec.Coverage != CoverageComplete {
		t.Fatalf("mixed rules status/coverage = %s/%s, want COMPLETED/COMPLETE: %v", fullExec.Status, fullExec.Coverage, fullExec.Diagnostics)
	}
	var goFinding, rustFinding bool
	for _, finding := range fullExec.Findings {
		switch {
		case strings.HasSuffix(finding.Path, ".go"):
			goFinding = true
		case strings.HasSuffix(finding.Path, ".rs"):
			rustFinding = true
		}
	}
	if !goFinding || !rustFinding {
		t.Fatalf("mixed findings = %+v, want one Go and one Rust finding", fullExec.Findings)
	}

	partial := runRealSemgrep(t, mixed, integrationRulesPath(t, "rules.yaml"), "mixed-go-only")
	partialExec := partial.Adapters[0]
	if partialExec.Status != StatusCompleted || partialExec.Coverage != CoveragePartial {
		t.Fatalf("go-only status/coverage = %s/%s, want COMPLETED/PARTIAL: %v", partialExec.Status, partialExec.Coverage, partialExec.Diagnostics)
	}
	for _, finding := range partialExec.Findings {
		if strings.HasSuffix(finding.Path, ".rs") {
			t.Errorf("Go-only rules produced a Rust finding: %+v", finding)
		}
	}
	if !strings.Contains(strings.Join(partialExec.Diagnostics, " "), "rust") {
		t.Errorf("diagnostics = %v, want an unsupported Rust note", partialExec.Diagnostics)
	}
}

// TestRealSemgrepInvalidRulesNeverClean proves malformed rules cannot produce a
// clean, complete result: both unparseable YAML and a rule Semgrep rejects are
// execution failures.
func TestRealSemgrepInvalidRulesNeverClean(t *testing.T) {
	root := integrationFixtureRoot(t, "rust-seeded")
	dir := t.TempDir()

	cases := []struct {
		name    string
		content string
	}{
		{"unparseable yaml", "rules: [ this is not: valid: yaml\n"},
		{"invalid rust pattern", "rules:\n  - id: broken-pattern\n    languages: [rust]\n    message: rejected rule\n    severity: WARNING\n    pattern: \"(((\"\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rules := filepath.Join(dir, strings.ReplaceAll(tc.name, " ", "-")+".yaml")
			if err := os.WriteFile(rules, []byte(tc.content), 0o644); err != nil {
				t.Fatal(err)
			}
			scan := runRealSemgrep(t, root, rules, "invalid-"+strings.ReplaceAll(tc.name, " ", "-"))
			exec := scan.Adapters[0]
			if exec.Status != StatusFailed {
				t.Fatalf("status = %s, want FAILED: %v", exec.Status, exec.Diagnostics)
			}
			if exec.Coverage == CoverageComplete {
				t.Fatalf("coverage = COMPLETE for invalid rules: %v", exec.Diagnostics)
			}
			if len(exec.Findings) != 0 {
				t.Errorf("invalid rules produced findings: %+v", exec.Findings)
			}
		})
	}
}

// TestPinnedProductionRulesCoverRust verifies the provisioned, pinned production
// ruleset (not a test fixture) actually covers Rust. It runs only when
// SOP_SECURITY_RULES_DIR points at a provisioned rules directory, which is what
// scripts/checks/security-integration.sh provides. An unprovisioned run is
// skipped, never reported as a pass.
func TestPinnedProductionRulesCoverRust(t *testing.T) {
	rules := strings.TrimSpace(os.Getenv("SOP_SECURITY_RULES_DIR"))
	if rules == "" {
		t.Skip("SOP_SECURITY_RULES_DIR is not set; pinned production rules were not provisioned")
	}
	languages, err := rulesLanguageSet(rules)
	if err != nil {
		t.Fatalf("read production rules: %v", err)
	}
	if !languages["rust"] {
		t.Fatalf("pinned production rules do not declare Rust: %v", languages)
	}
	if !languages["go"] {
		t.Fatalf("pinned production rules do not declare Go: %v", languages)
	}

	scan := runRealSemgrep(t, integrationFixtureRoot(t, "rust-seeded"), rules, "pinned-rust")
	exec := scan.Adapters[0]
	if exec.Status != StatusCompleted || exec.Coverage != CoverageComplete {
		t.Fatalf("pinned rules status/coverage = %s/%s, want COMPLETED/COMPLETE: %v", exec.Status, exec.Coverage, exec.Diagnostics)
	}
	if len(exec.Findings) == 0 {
		t.Fatalf("pinned production rules produced no Rust finding: %v", exec.Diagnostics)
	}
	for _, finding := range exec.Findings {
		if !strings.HasSuffix(finding.Path, ".rs") {
			t.Errorf("pinned finding %s path = %q, want Rust source", finding.RuleID, finding.Path)
		}
	}
	if exec.Provenance.RulesIdentity == "" {
		t.Error("pinned rules produced no rules identity in provenance")
	}
	t.Logf("pinned rules identity=%s findings=%d", exec.Provenance.RulesIdentity, len(exec.Findings))
}
