package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/failure"
	"github.com/imhttran/agentic-sop/internal/review"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/security"
	"github.com/imhttran/agentic-sop/internal/taskfile"
	"github.com/imhttran/agentic-sop/internal/testrunner"
)

func securityScript(t *testing.T, dir string, response string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fixture adapter is a POSIX shell script")
	}
	path := filepath.Join(dir, "security-adapter.sh")
	script := "#!/bin/sh\ncat >/dev/null\ncat <<'EOF'\n" + response + "\nEOF\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func securityCLIConfig(script, mode, extra string) string {
	return "project:\n  name: x\nsecurity:\n  enabled: true\n  mode: " + mode + "\n" + extra +
		"  adapters:\n    - id: fixture-sast\n      category: sast\n      protocol: external\n      version: \"1.0.0\"\n      command: [\"" + script + "\"]\n"
}

func TestSecurityScanConfiglessEnvironmentEnables(t *testing.T) {
	t.Setenv("SOP_SECURITY_ENABLED", "true")
	t.Setenv("SOP_SECURITY_MODE", "report-only")
	dir := t.TempDir()
	code, stdout, stderr := runCLI(t, dir, "security", "scan", "--json", "--run-id", "scan-env")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "INDETERMINATE") {
		t.Errorf("stdout = %q, want an indeterminate scan without scanners", stdout)
	}
	if !stateExists(filepath.Join(dir, stateDirName, "runs", "scan-env", "security", "scan.json")) {
		t.Error("configless environment-enabled scan wrote no evidence")
	}
}

func TestSecurityScanDisabledIsNotApplicable(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "project:\n  name: x\n")
	code, stdout, stderr := runCLI(t, dir, "security", "scan")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "NOT_APPLICABLE") {
		t.Errorf("stdout = %q, want NOT_APPLICABLE", stdout)
	}
	if stateExists(filepath.Join(dir, stateDirName, "runs")) {
		t.Error("disabled add-on wrote run artifacts")
	}
}

func TestSecurityScanReportOnlyRecordsFindingWithoutBlocking(t *testing.T) {
	dir := t.TempDir()
	response := `{"protocol_version":"security-scan/v1","adapter_id":"fixture-sast","category":"sast","status":"COMPLETED","exit_code":0,"coverage":"COMPLETE","findings":[{"rule_id":"R1","severity":"HIGH","message":"fixture finding","path":"main.go","line":1,"fingerprint":"fixture:1"}]}`
	script := securityScript(t, dir, response)
	writeConfig(t, dir, securityCLIConfig(script, "report-only", ""))
	writeFile(t, dir, "main.go", "package main\n")

	code, stdout, stderr := runCLI(t, dir, "security", "scan", "--json", "--run-id", "scan-1")
	if code != exitOK {
		t.Fatalf("report-only exited %d: %s", code, stderr)
	}
	var out struct {
		Scan struct {
			Verdict  security.Verdict  `json:"verdict"`
			Coverage security.Coverage `json:"coverage"`
			Content  struct {
				Dirty  bool   `json:"dirty"`
				Digest string `json:"digest"`
			} `json:"content"`
			Findings []security.Finding `json:"findings"`
		} `json:"scan"`
		Artifacts struct {
			ScanJSON string `json:"scan_json"`
		} `json:"artifacts"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout)
	}
	if out.Scan.Verdict != security.VerdictFail {
		t.Errorf("verdict = %s, want FAIL", out.Scan.Verdict)
	}
	if !out.Scan.Content.Dirty || out.Scan.Content.Digest == "" {
		t.Errorf("content identity = %+v, want dirty digest", out.Scan.Content)
	}
	if len(out.Scan.Findings) != 1 {
		t.Errorf("findings = %d, want 1", len(out.Scan.Findings))
	}
	if out.Artifacts.ScanJSON != "security/scan.json" {
		t.Errorf("artifact = %q", out.Artifacts.ScanJSON)
	}
	if !stateExists(filepath.Join(dir, stateDirName, "runs", "scan-1", "security", "scan.json")) {
		t.Error("security scan artifact was not written")
	}
}

func TestSecurityScanRequiredBlocksOnFinding(t *testing.T) {
	dir := t.TempDir()
	response := `{"protocol_version":"security-scan/v1","adapter_id":"fixture-sast","category":"sast","status":"COMPLETED","exit_code":0,"coverage":"COMPLETE","findings":[{"rule_id":"R1","severity":"HIGH","message":"fixture finding","path":"main.go","line":1,"fingerprint":"fixture:1"}]}`
	script := securityScript(t, dir, response)
	writeConfig(t, dir, securityCLIConfig(script, "required", "  required_categories:\n    - sast\n"))
	code, _, stderr := runCLI(t, dir, "security", "scan", "--run-id", "scan-2")
	if code != exitError {
		t.Fatalf("required scan exited %d, want %d (stderr=%s)", code, exitError, stderr)
	}
}

func TestSecurityScanRequiredUnavailableIsIndeterminate(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "project:\n  name: x\nsecurity:\n  enabled: true\n  mode: required\n  required_categories:\n    - sast\n  adapters:\n    - id: fixture-sast\n      category: sast\n      protocol: external\n      version: \"1.0.0\"\n      command: [\""+filepath.Join(dir, "missing-adapter")+"\"]\n")
	code, stdout, stderr := runCLI(t, dir, "security", "scan", "--json", "--run-id", "scan-3")
	if code != exitError {
		t.Fatalf("required unavailable scan exited %d, want %d", code, exitError)
	}
	var out struct {
		Scan struct {
			Verdict  security.Verdict `json:"verdict"`
			Adapters []struct {
				Status security.ExecutionStatus `json:"status"`
			} `json:"adapters"`
		} `json:"scan"`
	}
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s\nstderr=%s", err, stdout, stderr)
	}
	if out.Scan.Verdict != security.VerdictIndeterminate {
		t.Errorf("verdict = %s, want INDETERMINATE", out.Scan.Verdict)
	}
	if len(out.Scan.Adapters) != 1 || out.Scan.Adapters[0].Status != security.StatusUnavailable {
		t.Errorf("adapters = %+v, want UNAVAILABLE", out.Scan.Adapters)
	}
}

func TestRunSecurityGateWritesEvidence(t *testing.T) {
	dir := t.TempDir()
	response := `{"protocol_version":"security-scan/v1","adapter_id":"fixture-sast","category":"sast","status":"COMPLETED","exit_code":0,"coverage":"COMPLETE"}`
	script := securityScript(t, dir, response)
	writeConfig(t, dir, securityCLIConfig(script, "report-only", ""))
	writeFile(t, dir, "main.go", "package main\n")

	cfg, err := loadConfigOrDefault(dir)
	if err != nil {
		t.Fatal(err)
	}
	rn, err := runpkg.New(dir, "task-1")
	if err != nil {
		t.Fatal(err)
	}
	scan, err := runSecurityGate(context.Background(), cfg, dir, &taskfile.Spec{ID: "T1"}, rn)
	if err != nil {
		t.Fatal(err)
	}
	if scan.Verdict != security.VerdictPass {
		t.Errorf("verdict = %s, want PASS: %v", scan.Verdict, scan.Reasons)
	}
	if !stateExists(filepath.Join(rn.Dir(), "security", "scan.json")) {
		t.Error("security evidence was not written into the run directory")
	}
}

func TestVerificationEvidenceCarriesRequiredSecurityFailure(t *testing.T) {
	enabled := true
	cfg := config.Config{}
	cfg.Security.Enabled = &enabled
	cfg.Security.Mode = config.SecurityModeRequired
	cfg.Security.Adapters = []config.SecurityAdapterConfig{{ID: "semgrep"}}
	scan := &security.Scan{
		Verdict:  security.VerdictFail,
		Adapters: []security.Execution{{AdapterID: "semgrep", Status: security.StatusCompleted, Coverage: security.CoverageComplete}},
		Reasons:  []string{"1 unsuppressed finding at a blocking severity"},
	}
	ev := verificationEvidence(cfg, testrunner.SuiteResult{}, review.Report{}, 0, &jevRunEvidence{}, failure.ApprovalNone, scan)
	if !ev.SecurityFailed || ev.SecurityVerdict != "FAIL" {
		t.Fatalf("security evidence = %+v, want required FAIL", ev)
	}
	class := failure.Classify(ev)
	if class.Kind != failure.SecurityScanFailure || class.Disposition != failure.Block {
		t.Fatalf("classification = %+v, want SECURITY_SCAN_FAILURE/BLOCK", class)
	}
	cfg.Security.Mode = config.SecurityModeReportOnly
	ev = verificationEvidence(cfg, testrunner.SuiteResult{}, review.Report{}, 0, &jevRunEvidence{}, failure.ApprovalNone, scan)
	if ev.SecurityFailed {
		t.Fatalf("report-only evidence must not classify as a security failure: %+v", ev)
	}
}

func TestLoadPriorSecurityScanLoadsVerifiedNativeReport(t *testing.T) {
	dir := t.TempDir()
	runID := "prior-run"
	runDir := filepath.Join(dir, config.DirName, "runs", runID)
	native := []byte(`{"adapter":"stub","ok":true}`)
	scan := security.Scan{
		ProtocolVersion: security.ProtocolVersion,
		RunID:           runID,
		Adapters: []security.Execution{{
			ProtocolVersion: security.ProtocolVersion, AdapterID: "stub", Category: security.CategorySAST,
			Status: security.StatusCompleted, Coverage: security.CoverageComplete, NativeReport: native,
			Provenance: security.Provenance{SanitizedReportDigest: security.DigestBytes(native)},
		}},
	}
	if _, err := security.WriteArtifacts(runDir, scan); err != nil {
		t.Fatal(err)
	}
	got, natives, err := loadPriorSecurityScan(dir, runID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.RunID != runID {
		t.Fatalf("prior scan = %+v, want run %s", got, runID)
	}
	if string(natives["stub"]) != string(native) {
		t.Fatalf("native report = %q, want verified report", natives["stub"])
	}
	path := filepath.Join(runDir, "security", "stub.native.json")
	if err := os.WriteFile(path, []byte(`{"tampered":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, natives, err = loadPriorSecurityScan(dir, runID)
	if err != nil {
		t.Fatal(err)
	}
	if len(natives) != 0 {
		t.Fatalf("tampered native report was accepted: %v", natives)
	}
}
