//go:build security_integration

package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/security"
)

func realCLIBinary(t *testing.T, envName, name string) string {
	t.Helper()
	if path := strings.TrimSpace(os.Getenv(envName)); path != "" {
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			t.Fatalf("%s=%q is not an executable file", envName, path)
		}
		return path
	}
	path, err := exec.LookPath(name)
	if err == nil {
		return path
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("SOP_SECURITY_INTEGRATION_REQUIRED")), "true") ||
		strings.TrimSpace(os.Getenv("SOP_SECURITY_INTEGRATION_REQUIRED")) == "1" {
		t.Fatalf("required security integration prerequisite missing: %s", name)
	}
	t.Skipf("%s is not installed; set %s or SOP_SECURITY_INTEGRATION_REQUIRED=1", name, envName)
	return ""
}

func realCLIVersion(t *testing.T, binary string, args ...string) string {
	t.Helper()
	out, err := exec.Command(binary, args...).Output()
	if err != nil {
		t.Fatalf("%s: %v", binary, err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimSpace(line); line != "" && !strings.HasPrefix(line, "A new version") {
			return line
		}
	}
	return ""
}

func copyIntegrationFixtures(t *testing.T, source, destination string) {
	t.Helper()
	if err := os.MkdirAll(destination, 0o755); err != nil {
		t.Fatal(err)
	}
	err := filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(destination, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func writeRealCLIConfig(t *testing.T, dir, mode, semgrepBin, gitleaksBin, semgrepVersion, gitleaksVersion string) {
	t.Helper()
	rules, err := filepath.Abs(filepath.Join("..", "security", "testdata", "real", "rules.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	gitleaksConfig, err := filepath.Abs(filepath.Join("..", "security", "testdata", "real", "gitleaks.toml"))
	if err != nil {
		t.Fatal(err)
	}
	required := ""
	if mode == "required" {
		required = "  required_categories:\n    - sast\n    - secrets\n"
	}
	config := "version: 1\n" +
		"project:\n  name: security-integration\n" +
		"agent:\n  harness: command\n  provider: command\n" +
		"validation:\n  build:\n    - \"true\"\n  test:\n    - \"true\"\n" +
		"review:\n  engine: self\n" +
		"quality:\n  require_tests: true\n" +
		"security:\n  enabled: true\n  mode: " + mode + "\n  scope: workspace\n" + required +
		"  fail_on: [critical, high]\n  unknown_severity: indeterminate\n  timeout_seconds: 120\n  max_output_bytes: 4194304\n" +
		"  adapters:\n" +
		"    - id: semgrep\n      category: sast\n      protocol: builtin\n      rules: \"" + rules + "\"\n      version: \"" + semgrepVersion + "\"\n      command: [\"" + semgrepBin + "\"]\n" +
		"    - id: gitleaks\n      category: secrets\n      protocol: builtin\n      rules: \"" + gitleaksConfig + "\"\n      version: \"" + gitleaksVersion + "\"\n      command: [\"" + gitleaksBin + "\"]\n"
	writeConfig(t, dir, config)
}

func writeVerifyFirstPlan(t *testing.T, dir string) {
	t.Helper()
	plan := `# Security integration plan

## Project

security-integration

## Summary

Exercise the optional security stage without invoking an implementation agent.

## T001 — Verify security integration

The configured validation is the work.

### Dependencies

None

### Deliverables

- Security evidence under the task run directory.

### Acceptance Criteria

- The verify-first validation passes.

### Acceptance Checks

- ` + "`true`" + `

### Execution

- verify-first
`
	writeFile(t, dir, "PLAN.md", plan)
}

func prepareRealCLIProject(t *testing.T, fixture, mode, semgrepBin, gitleaksBin, semgrepVersion, gitleaksVersion string) string {
	t.Helper()
	dir := t.TempDir()
	copyIntegrationFixtures(t, filepath.Join("..", "security", "testdata", "real", fixture), dir)
	writeRealCLIConfig(t, dir, mode, semgrepBin, gitleaksBin, semgrepVersion, gitleaksVersion)
	return dir
}

func TestRealCLIScanReportOnlyAndRequired(t *testing.T) {
	semgrepBin := realCLIBinary(t, "SOP_SECURITY_SEMGREP_BIN", "semgrep")
	gitleaksBin := realCLIBinary(t, "SOP_SECURITY_GITLEAKS_BIN", "gitleaks")
	semgrepVersion := realCLIVersion(t, semgrepBin, "--version")
	gitleaksVersion := realCLIVersion(t, gitleaksBin, "version")
	t.Logf("semgrep=%s gitleaks=%s", semgrepVersion, gitleaksVersion)

	cases := []struct {
		name    string
		fixture string
		mode    string
		code    int
		verdict security.Verdict
	}{
		{"report-only seeded", "seeded", "report-only", exitOK, security.VerdictFail},
		{"required seeded", "seeded", "required", exitError, security.VerdictFail},
		{"required clean", "clean", "required", exitOK, security.VerdictPass},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := prepareRealCLIProject(t, tc.fixture, tc.mode, semgrepBin, gitleaksBin, semgrepVersion, gitleaksVersion)
			code, stdout, stderr := runCLI(t, dir, "security", "scan", "--json", "--run-id", "real-"+strings.ReplaceAll(tc.name, " ", "-"))
			if code != tc.code {
				t.Fatalf("code = %d, want %d\nstdout=%s\nstderr=%s", code, tc.code, stdout, stderr)
			}
			var out struct {
				Scan struct {
					Verdict  security.Verdict   `json:"verdict"`
					Coverage security.Coverage  `json:"coverage"`
					Findings []security.Finding `json:"findings"`
				} `json:"scan"`
			}
			if err := json.Unmarshal([]byte(stdout), &out); err != nil {
				t.Fatalf("stdout is not JSON: %v\n%s", err, stdout)
			}
			if out.Scan.Verdict != tc.verdict {
				t.Errorf("verdict = %s, want %s", out.Scan.Verdict, tc.verdict)
			}
			if out.Scan.Coverage != security.CoverageComplete {
				t.Errorf("coverage = %s, want COMPLETE", out.Scan.Coverage)
			}
			if tc.fixture == "clean" && len(out.Scan.Findings) != 0 {
				t.Errorf("clean fixture produced %d finding(s)", len(out.Scan.Findings))
			}
			if tc.fixture == "seeded" && len(out.Scan.Findings) < 2 {
				t.Errorf("seeded fixture produced %d finding(s), want at least 2", len(out.Scan.Findings))
			}
		})
	}
}

func TestRealCLIRunIntegration(t *testing.T) {
	semgrepBin := realCLIBinary(t, "SOP_SECURITY_SEMGREP_BIN", "semgrep")
	gitleaksBin := realCLIBinary(t, "SOP_SECURITY_GITLEAKS_BIN", "gitleaks")
	semgrepVersion := realCLIVersion(t, semgrepBin, "--version")
	gitleaksVersion := realCLIVersion(t, gitleaksBin, "version")

	cases := []struct {
		name string
		mode string
		code int
	}{
		{"report-only seeded", "report-only", exitOK},
		{"required seeded", "required", exitError},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := prepareRealCLIProject(t, "seeded", tc.mode, semgrepBin, gitleaksBin, semgrepVersion, gitleaksVersion)
			writeVerifyFirstPlan(t, dir)
			code, stdout, stderr := runCLIWithAgent(t, dir, &fakeAgent{}, "run", "PLAN.md")
			if code != tc.code {
				t.Fatalf("run code = %d, want %d\nstdout=%s\nstderr=%s", code, tc.code, stdout, stderr)
			}
			scanPath := filepath.Join(dir, stateDirName, "runs", "T001", "security", "scan.json")
			data, err := os.ReadFile(scanPath)
			if err != nil {
				t.Fatalf("run integration wrote no security evidence: %v", err)
			}
			var scan security.Scan
			if err := json.Unmarshal(data, &scan); err != nil {
				t.Fatalf("security evidence is not JSON: %v", err)
			}
			if scan.Verdict != security.VerdictFail {
				t.Errorf("security verdict = %s, want FAIL", scan.Verdict)
			}
			report, err := os.ReadFile(filepath.Join(dir, stateDirName, "runs", "T001", "report.md"))
			if err != nil {
				t.Fatalf("run integration wrote no report: %v", err)
			}
			if !strings.Contains(string(report), "## Security") || !strings.Contains(string(report), "Verdict: `FAIL`") {
				t.Errorf("run report does not include the security evidence:\n%s", report)
			}
			wantGate := "- Gate: `PASS`"
			if tc.mode == "required" {
				wantGate = "- Gate: `FAIL`"
			}
			if !strings.Contains(string(report), wantGate) {
				t.Errorf("run report gate does not match %s mode:\n%s", tc.mode, report)
			}
		})
	}
}
