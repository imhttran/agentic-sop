//go:build security_integration

package security

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/config"
)

const (
	integrationSemgrepVersion  = "1.136.0"
	integrationGitleaksVersion = "8.30.1"
)

func integrationBinary(t *testing.T, envName, name string) string {
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
	if integrationRequired() {
		t.Fatalf("required security integration prerequisite missing: %s (set %s or install it)", name, envName)
	}
	t.Skipf("%s is not installed; set %s or SOP_SECURITY_INTEGRATION_REQUIRED=1", name, envName)
	return ""
}

func integrationRequired() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("SOP_SECURITY_INTEGRATION_REQUIRED"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func toolVersion(t *testing.T, binary string, args ...string) string {
	t.Helper()
	out, err := exec.Command(binary, args...).Output()
	if err != nil {
		t.Fatalf("%s %s: %v", binary, strings.Join(args, " "), err)
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "A new version") {
			return line
		}
	}
	return ""
}

// integrationModuleRoot returns the module root so repository-relative
// integration prerequisites resolve consistently from the package directory.
func integrationModuleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("module root not found")
		}
		dir = parent
	}
}

func copyFixtureTree(t *testing.T, source, destination string) {
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

func integrationAdapters(t *testing.T, semgrepBin, gitleaksBin, semgrepVersion, gitleaksVersion string) []Adapter {
	t.Helper()
	rules, err := filepath.Abs(filepath.Join("testdata", "real", "rules.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	gitleaksConfig, err := filepath.Abs(filepath.Join("testdata", "real", "gitleaks.toml"))
	if err != nil {
		t.Fatal(err)
	}
	adapters, err := BuildAdapters([]config.SecurityAdapterConfig{
		{ID: "semgrep", Category: "sast", Protocol: "builtin", Command: []string{semgrepBin}, Rules: rules, Version: semgrepVersion},
		{ID: "gitleaks", Category: "secrets", Protocol: "builtin", Command: []string{gitleaksBin}, Rules: gitleaksConfig, Version: gitleaksVersion},
	}, BuildOptions{Timeout: 2 * time.Minute, MaxOutputBytes: 4 << 20})
	if err != nil {
		t.Fatal(err)
	}
	return adapters
}

func TestRealCLIsCleanAndSeededFixtures(t *testing.T) {
	semgrepBin := integrationBinary(t, "SOP_SECURITY_SEMGREP_BIN", "semgrep")
	gitleaksBin := integrationBinary(t, "SOP_SECURITY_GITLEAKS_BIN", "gitleaks")
	semgrepVersion := toolVersion(t, semgrepBin, "--version")
	gitleaksVersion := toolVersion(t, gitleaksBin, "version")
	t.Logf("semgrep=%s gitleaks=%s", semgrepVersion, gitleaksVersion)
	adapters := integrationAdapters(t, semgrepBin, gitleaksBin, semgrepVersion, gitleaksVersion)

	cleanRoot := t.TempDir()
	copyFixtureTree(t, filepath.Join("testdata", "real", "clean"), cleanRoot)
	clean, err := NewRunner(RunnerConfig{
		WorkspaceRoot: cleanRoot, RunID: "clean", Adapters: adapters,
		Policy:  Policy{Enabled: true, Mode: "required", Scope: "workspace", RequiredCategories: []string{"sast", "secrets"}, FailOn: []string{"critical", "high"}, UnknownSeverity: "indeterminate"},
		Timeout: 2 * time.Minute, MaxOutputBytes: 4 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	cleanScan, err := clean.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cleanScan.Verdict != VerdictPass || cleanScan.Coverage != CoverageComplete {
		t.Fatalf("clean scan = %s/%s, want PASS/COMPLETE: %v", cleanScan.Verdict, cleanScan.Coverage, cleanScan.Reasons)
	}
	if len(cleanScan.Findings) != 0 {
		t.Fatalf("clean fixture produced %d unexpected finding(s): %+v", len(cleanScan.Findings), cleanScan.Findings)
	}
	for _, adapter := range cleanScan.Adapters {
		if adapter.Status != StatusCompleted || adapter.Coverage != CoverageComplete {
			t.Errorf("clean adapter %s = %s/%s: %v", adapter.AdapterID, adapter.Status, adapter.Coverage, adapter.Diagnostics)
		}
	}

	seededRoot := t.TempDir()
	copyFixtureTree(t, filepath.Join("testdata", "real", "seeded"), seededRoot)
	seeded, err := NewRunner(RunnerConfig{
		WorkspaceRoot: seededRoot, RunID: "seeded", Adapters: adapters,
		Policy:  Policy{Enabled: true, Mode: "required", Scope: "workspace", RequiredCategories: []string{"sast", "secrets"}, FailOn: []string{"critical", "high"}, UnknownSeverity: "indeterminate"},
		Timeout: 2 * time.Minute, MaxOutputBytes: 4 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	seededScan, err := seeded.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if seededScan.Verdict != VerdictFail {
		t.Fatalf("seeded scan = %s, want FAIL: %v", seededScan.Verdict, seededScan.Reasons)
	}
	if seededScan.Coverage != CoverageComplete {
		t.Errorf("seeded coverage = %s, want COMPLETE", seededScan.Coverage)
	}
	var sast, secret bool
	secretValue := "SOP_FAKE_SECRET_DO_NOT_USE_1234567890"
	var serialized bytes.Buffer
	encoder := json.NewEncoder(&serialized)
	if err := encoder.Encode(seededScan); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(serialized.Bytes(), []byte(secretValue)) {
		t.Fatal("normalized scan JSON contains the synthetic secret")
	}
	for _, finding := range seededScan.Findings {
		switch finding.Category {
		case CategorySAST:
			sast = true
			if finding.Severity != SeverityHigh || finding.OriginalSeverity != "ERROR" {
				t.Errorf("semgrep finding severity = %s/%s, want HIGH/ERROR", finding.Severity, finding.OriginalSeverity)
			}
			if finding.Path != "insecure.go" {
				t.Errorf("semgrep path = %q, want insecure.go", finding.Path)
			}
			if strings.Contains(finding.Fingerprint, "requires login") {
				t.Errorf("semgrep fingerprint = %q, want a stable non-placeholder fingerprint", finding.Fingerprint)
			}
		case CategorySecrets:
			secret = true
			if finding.Severity != SeverityHigh {
				t.Errorf("gitleaks severity = %s, want HIGH", finding.Severity)
			}
			if finding.Path != "secret.txt" {
				t.Errorf("gitleaks path = %q, want secret.txt", finding.Path)
			}
			if strings.Contains(finding.Message, secretValue) {
				t.Error("normalized finding contains the synthetic secret")
			}
		}
	}
	if !sast {
		t.Error("Semgrep did not produce a SAST finding")
	}
	if !secret {
		t.Error("Gitleaks did not produce a secret finding")
	}
	for _, adapter := range seededScan.Adapters {
		if adapter.Provenance.RulesIdentity == "" {
			t.Errorf("adapter %s has no rules identity", adapter.AdapterID)
		}
		if adapter.Provenance.SanitizedReportDigest == "" {
			t.Errorf("adapter %s has no sanitized report digest", adapter.AdapterID)
		}
	}
	for _, adapter := range seededScan.Adapters {
		for _, diagnostic := range adapter.Diagnostics {
			if strings.Contains(diagnostic, secretValue) {
				t.Errorf("adapter %s diagnostic contains the synthetic secret", adapter.AdapterID)
			}
		}
		if bytes.Contains(adapter.NativeReport, []byte(secretValue)) {
			t.Errorf("adapter %s native report contains the synthetic secret", adapter.AdapterID)
		}
	}
	runDir := t.TempDir()
	if _, err := WriteArtifacts(runDir, seededScan); err != nil {
		t.Fatal(err)
	}
	err = filepath.WalkDir(runDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(data, []byte(secretValue)) {
			t.Errorf("persisted artifact %s contains the synthetic secret", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("seeded findings=%d duration=%dms adapter_durations=%v", len(seededScan.Findings), seededScan.Metrics.DurationMS, seededScan.Metrics.AdapterDurationMS)
}

func TestRealGitleaksExclusionsArePreScan(t *testing.T) {
	gitleaksBin := integrationBinary(t, "SOP_SECURITY_GITLEAKS_BIN", "gitleaks")
	gitleaksVersion := toolVersion(t, gitleaksBin, "version")
	configPath, err := filepath.Abs(filepath.Join("testdata", "real", "gitleaks.toml"))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	writeIntegrationFile(t, root, "keep/secret.txt", "SOP_FAKE_SECRET_KEEP_0001\n")
	var noise strings.Builder
	for i := 0; i < 2000; i++ {
		fmt.Fprintf(&noise, "SOP_FAKE_SECRET_NOISE_%04d\n", i)
	}
	writeIntegrationFile(t, root, "excluded/noise.txt", noise.String())

	build := func() Adapter {
		adapter, err := BuildAdapter(config.SecurityAdapterConfig{
			ID: "gitleaks", Category: "secrets", Protocol: "builtin",
			Command: []string{gitleaksBin}, Rules: configPath, Version: gitleaksVersion,
		}, BuildOptions{Timeout: 2 * time.Minute, MaxOutputBytes: 1024})
		if err != nil {
			t.Fatal(err)
		}
		return adapter
	}
	request := func(exclude []string) Request {
		req := requestFor(t, root)
		req.Scope.Excluded = exclude
		return req
	}

	excludedRun, err := build().Scan(context.Background(), request([]string{"excluded"}))
	if err != nil {
		t.Fatal(err)
	}
	if excludedRun.Status != StatusCompleted || excludedRun.Coverage != CoverageComplete {
		t.Fatalf("excluded run = %s/%s, want COMPLETED/COMPLETE: %v", excludedRun.Status, excludedRun.Coverage, excludedRun.Diagnostics)
	}
	if len(excludedRun.Findings) != 1 || excludedRun.Findings[0].Path != "keep/secret.txt" {
		t.Fatalf("excluded findings = %+v, want only keep/secret.txt", excludedRun.Findings)
	}
	if bytes.Contains(excludedRun.NativeReport, []byte("excluded/noise.txt")) {
		t.Error("native report contains the excluded path")
	}
	if bytes.Contains(excludedRun.NativeReport, []byte("sop-gitleaks-")) {
		t.Error("native report contains the temporary staged-view path")
	}
	if !strings.Contains(strings.Join(excludedRun.Diagnostics, " "), "outside the declared scope were not staged") {
		t.Errorf("diagnostics = %v, want staged-view accounting", excludedRun.Diagnostics)
	}

	// Control: without the exclusion, the same excluded content produces a report
	// larger than the adapter bound. This proves the staging fix is what prevents
	// excluded content from exhausting the scan limit.
	control, err := build().Scan(context.Background(), request(nil))
	if err != nil {
		t.Fatal(err)
	}
	if control.Status != StatusFailed {
		t.Fatalf("control status = %s, want FAILED at the 1024-byte report bound: %v", control.Status, control.Diagnostics)
	}
	if !strings.Contains(strings.Join(control.Diagnostics, " "), "exceeds the 1024-byte limit") {
		t.Errorf("control diagnostics = %v, want report-size failure", control.Diagnostics)
	}
}

func TestRealCLIExclusionConformance(t *testing.T) {
	semgrepBin := integrationBinary(t, "SOP_SECURITY_SEMGREP_BIN", "semgrep")
	gitleaksBin := integrationBinary(t, "SOP_SECURITY_GITLEAKS_BIN", "gitleaks")
	semgrepVersion := toolVersion(t, semgrepBin, "--version")
	gitleaksVersion := toolVersion(t, gitleaksBin, "version")
	adapters := integrationAdapters(t, semgrepBin, gitleaksBin, semgrepVersion, gitleaksVersion)

	root := t.TempDir()
	goSource := "package fixture\n\nimport \"os/exec\"\n\nfunc Run(arg string) error { return exec.Command(arg).Run() }\n"
	paths := []string{
		"keep",
		"internal/a/testdata/excluded",
		"internal/a/b/testdata/included",
		"nested/generated/x/excluded",
		"generated/root-excluded",
		"root-only/excluded",
		"sub/root-only/included",
	}
	for i, path := range paths {
		writeIntegrationFile(t, root, path+".go", goSource)
		writeIntegrationFile(t, root, path+".txt", fmt.Sprintf("SOP_FAKE_SECRET_CONFORMANCE_%02d\n", i))
	}
	excludes := []string{"/internal/*/testdata", "**/generated/**", "/root-only"}
	runner, err := NewRunner(RunnerConfig{
		WorkspaceRoot: root, RunID: "conformance", Adapters: adapters,
		Policy: Policy{
			Enabled: true, Mode: "report-only", Scope: "workspace",
			FailOn: []string{"high"}, UnknownSeverity: "indeterminate", Exclude: excludes,
		},
		Timeout: 2 * time.Minute, MaxOutputBytes: 4 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	scan, err := runner.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	included := map[string]bool{
		"keep":                           true,
		"internal/a/b/testdata/included": true,
		"sub/root-only/included":         true,
	}
	excluded := map[string]bool{
		"internal/a/testdata/excluded": true,
		"nested/generated/x/excluded":  true,
		"generated/root-excluded":      true,
		"root-only/excluded":           true,
	}
	found := make(map[string]bool)
	for _, finding := range scan.Findings {
		base := strings.TrimSuffix(finding.Path, filepath.Ext(finding.Path))
		found[finding.Path] = true
		if excluded[base] {
			t.Errorf("finding from excluded path %s (%s)", finding.Path, finding.AdapterID)
		}
		if !included[base] {
			t.Errorf("finding from unexpected path %s (%s)", finding.Path, finding.AdapterID)
		}
	}
	for base := range included {
		for _, ext := range []string{".go", ".txt"} {
			if !found[base+ext] {
				t.Errorf("missing expected finding %s", base+ext)
			}
		}
	}
	for _, adapter := range scan.Adapters {
		if adapter.Status != StatusCompleted || adapter.Coverage != CoverageComplete {
			t.Errorf("adapter %s = %s/%s: %v", adapter.AdapterID, adapter.Status, adapter.Coverage, adapter.Diagnostics)
		}
	}
	for _, pattern := range excludes {
		matched := false
		for _, got := range scan.Scope.Excluded {
			if got == pattern {
				matched = true
			}
		}
		if !matched {
			t.Errorf("scope.excluded = %v, missing %q", scan.Scope.Excluded, pattern)
		}
	}
}

func writeIntegrationFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRealCLIResumeIdentity(t *testing.T) {
	semgrepBin := integrationBinary(t, "SOP_SECURITY_SEMGREP_BIN", "semgrep")
	gitleaksBin := integrationBinary(t, "SOP_SECURITY_GITLEAKS_BIN", "gitleaks")
	semgrepVersion := toolVersion(t, semgrepBin, "--version")
	gitleaksVersion := toolVersion(t, gitleaksBin, "version")
	adapters := integrationAdapters(t, semgrepBin, gitleaksBin, semgrepVersion, gitleaksVersion)
	root := t.TempDir()
	copyFixtureTree(t, filepath.Join("testdata", "real", "seeded"), root)
	policy := Policy{Enabled: true, Mode: "report-only", Scope: "workspace", FailOn: []string{"critical", "high"}, UnknownSeverity: "indeterminate"}

	firstRunner, err := NewRunner(RunnerConfig{WorkspaceRoot: root, RunID: "resume", Adapters: adapters, Policy: policy, Timeout: 2 * time.Minute, MaxOutputBytes: 4 << 20})
	if err != nil {
		t.Fatal(err)
	}
	first, err := firstRunner.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Findings) == 0 {
		t.Fatal("first scan produced no findings")
	}
	priorNative := map[string][]byte{}
	for _, adapter := range first.Adapters {
		priorNative[adapter.AdapterID] = adapter.NativeReport
	}

	secondRunner, err := NewRunner(RunnerConfig{WorkspaceRoot: root, RunID: "resume", Adapters: adapters, Policy: policy, Timeout: 2 * time.Minute, MaxOutputBytes: 4 << 20, Prior: &first, PriorNativeReports: priorNative})
	if err != nil {
		t.Fatal(err)
	}
	second, err := secondRunner.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, adapter := range second.Adapters {
		if !adapter.Reused {
			t.Errorf("matching resume did not reuse adapter %s: %+v", adapter.AdapterID, adapter.Provenance)
		}
		if len(adapter.NativeReport) == 0 || DigestBytes(adapter.NativeReport) != adapter.Provenance.SanitizedReportDigest {
			t.Errorf("reused adapter %s has no verified native report", adapter.AdapterID)
		}
	}

	if err := os.WriteFile(filepath.Join(root, "insecure.go"), []byte("package seeded\n\n// changed input\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	thirdRunner, err := NewRunner(RunnerConfig{WorkspaceRoot: root, RunID: "resume", Adapters: adapters, Policy: policy, Timeout: 2 * time.Minute, MaxOutputBytes: 4 << 20, Prior: &first, PriorNativeReports: priorNative})
	if err != nil {
		t.Fatal(err)
	}
	third, err := thirdRunner.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, adapter := range third.Adapters {
		if adapter.Reused {
			t.Errorf("stale evidence was reused for adapter %s after a content change", adapter.AdapterID)
		}
	}
}

// TestRealCargoAuditPinnedDatabase runs the real cargo-audit adapter against a
// controlled local advisory database that contains a known advisory. It verifies
// the pinned executable version, the structured finding, and that the scan leaves
// the lockfile and the database byte-identical (no fetch, update, or lockfile
// mutation). A missing prerequisite is skipped, or fails when integration checks
// are required; it is never reported as a clean dependency result.
func TestRealCargoAuditPinnedDatabase(t *testing.T) {
	database := strings.TrimSpace(os.Getenv("SOP_SECURITY_CARGO_ADVISORY_DB"))
	if database == "" {
		if integrationRequired() {
			t.Fatal("required security integration prerequisite missing: SOP_SECURITY_CARGO_ADVISORY_DB")
		}
		t.Skip("SOP_SECURITY_CARGO_ADVISORY_DB is not set; real cargo-audit check not run")
	}
	absoluteDatabase, err := filepath.Abs(database)
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(database) {
		// The test runs with the package directory as its working directory, so a
		// repository-relative database path is resolved against the module root.
		absoluteDatabase = filepath.Join(integrationModuleRoot(t), database)
	}
	database = absoluteDatabase

	cargoBin := strings.TrimSpace(os.Getenv("SOP_SECURITY_CARGO_BIN"))
	if cargoBin == "" {
		for _, candidate := range []string{"cargo-audit", "cargo"} {
			if path, lookErr := exec.LookPath(candidate); lookErr == nil {
				cargoBin = path
				break
			}
		}
	}
	if cargoBin == "" {
		if integrationRequired() {
			t.Fatal("required security integration prerequisite missing: cargo-audit (set SOP_SECURITY_CARGO_BIN or install it)")
		}
		t.Skip("cargo-audit is not installed; real cargo-audit check not run")
	}

	rawVersion, err := exec.Command(cargoBin, "audit", "--version").Output()
	if err != nil {
		if integrationRequired() {
			t.Fatalf("cargo-audit version probe failed: %v", err)
		}
		t.Skipf("cargo-audit is not usable; real cargo-audit check not run: %v", err)
	}
	actualVersion := normalizeCargoAuditVersion(string(rawVersion))
	if actualVersion == "" {
		t.Fatalf("cargo-audit version output is unrecognized: %q", strings.TrimSpace(string(rawVersion)))
	}
	if pinned := normalizeCargoAuditVersion(os.Getenv("SOP_SECURITY_CARGO_AUDIT_VERSION")); pinned != "" && pinned != actualVersion {
		t.Fatalf("cargo-audit version = %s, want the pinned %s", actualVersion, pinned)
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Cargo.toml"), []byte("[package]\nname = \"real\"\nversion = \"0.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lockfileData, err := os.ReadFile(filepath.Join("testdata", "cargo-audit", "Cargo.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Cargo.lock"), lockfileData, 0o644); err != nil {
		t.Fatal(err)
	}
	databaseDigestBefore, err := rulesIdentity(database)
	if err != nil {
		t.Fatal(err)
	}

	adapter, err := BuildAdapter(config.SecurityAdapterConfig{
		ID: AdapterIDCargoAudit, Category: config.SecurityCategoryDependencies,
		Protocol: config.SecurityProtocolBuiltin, Command: []string{cargoBin, "audit"},
		Version: actualVersion,
	}, BuildOptions{
		Timeout: 2 * time.Minute, MaxOutputBytes: 4 << 20,
		DatabasePath: database, StalePolicy: StaleAllow,
	})
	if err != nil {
		t.Fatal(err)
	}
	execResult, err := adapter.Scan(context.Background(), requestFor(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if execResult.Status != StatusCompleted {
		t.Fatalf("status = %s, want COMPLETED: %v", execResult.Status, execResult.Diagnostics)
	}
	if execResult.Coverage == CoverageUnknown {
		t.Fatalf("coverage = UNKNOWN for a completed real scan: %v", execResult.Diagnostics)
	}
	if len(execResult.Findings) != 1 {
		t.Fatalf("findings = %d, want exactly one advisory: %+v", len(execResult.Findings), execResult.Findings)
	}
	finding := execResult.Findings[0]
	if finding.RuleID != "RUSTSEC-2020-0071" {
		t.Errorf("advisory id = %q, want RUSTSEC-2020-0071", finding.RuleID)
	}
	if finding.Category != CategoryDependencies {
		t.Errorf("category = %s, want dependencies", finding.Category)
	}
	for key, want := range map[string]string{
		"package":          "time",
		"version":          "0.1.44",
		"patched_versions": ">=0.2.23",
	} {
		if finding.Metadata[key] != want {
			t.Errorf("metadata[%s] = %q, want %q", key, finding.Metadata[key], want)
		}
	}
	if finding.Severity != SeverityHigh {
		t.Errorf("severity = %s, want HIGH from the advisory CVSS 7.5", finding.Severity)
	}
	if len(finding.CVE) == 0 || finding.CVE[0] != "CVE-2020-26235" {
		t.Errorf("CVE = %v, want CVE-2020-26235", finding.CVE)
	}
	if finding.Path != "Cargo.lock" {
		t.Errorf("path = %q, want the resolved lockfile", finding.Path)
	}
	if execResult.Provenance.ScannerVersion != actualVersion {
		t.Errorf("provenance scanner version = %q, want the verified %s", execResult.Provenance.ScannerVersion, actualVersion)
	}
	if execResult.Provenance.AdvisoryDatabaseIdentity == "" {
		t.Error("real cargo-audit run recorded no advisory database identity")
	}
	if !strings.HasPrefix(execResult.Provenance.LockfileIdentity, "path=Cargo.lock;digest=") {
		t.Errorf("lockfile identity = %q, want path and digest", execResult.Provenance.LockfileIdentity)
	}

	lockfileAfter, err := os.ReadFile(filepath.Join(root, "Cargo.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(lockfileData, lockfileAfter) {
		t.Error("cargo-audit modified the lockfile")
	}
	databaseDigestAfter, err := rulesIdentity(database)
	if err != nil {
		t.Fatal(err)
	}
	if databaseDigestBefore != databaseDigestAfter {
		t.Error("cargo-audit modified the local advisory database")
	}
	t.Logf("cargo-audit version=%s status=%s coverage=%s findings=%d database=%s lockfile=%s",
		actualVersion, execResult.Status, execResult.Coverage, len(execResult.Findings),
		execResult.Provenance.AdvisoryDatabaseIdentity, execResult.Provenance.LockfileIdentity)
}

func integrationCargoAuditPrerequisites(t *testing.T) (string, string, string) {
	t.Helper()
	database := strings.TrimSpace(os.Getenv("SOP_SECURITY_CARGO_ADVISORY_DB"))
	if database == "" {
		if integrationRequired() {
			t.Fatal("required security integration prerequisite missing: SOP_SECURITY_CARGO_ADVISORY_DB")
		}
		t.Skip("SOP_SECURITY_CARGO_ADVISORY_DB is not set; real cargo-audit check not run")
	}
	if !filepath.IsAbs(database) {
		database = filepath.Join(integrationModuleRoot(t), database)
	}
	cargoBin := strings.TrimSpace(os.Getenv("SOP_SECURITY_CARGO_BIN"))
	if cargoBin == "" {
		for _, candidate := range []string{"cargo-audit", "cargo"} {
			if path, err := exec.LookPath(candidate); err == nil {
				cargoBin = path
				break
			}
		}
	}
	if cargoBin == "" {
		if integrationRequired() {
			t.Fatal("required security integration prerequisite missing: cargo-audit")
		}
		t.Skip("cargo-audit is not installed; real cargo-audit check not run")
	}
	rawVersion, err := exec.Command(cargoBin, "audit", "--version").Output()
	if err != nil {
		if integrationRequired() {
			t.Fatalf("cargo-audit version probe failed: %v", err)
		}
		t.Skipf("cargo-audit is not usable: %v", err)
	}
	version := normalizeCargoAuditVersion(string(rawVersion))
	if version == "" {
		t.Fatalf("cargo-audit version output is unrecognized: %q", strings.TrimSpace(string(rawVersion)))
	}
	return cargoBin, database, version
}

func TestRealCargoAuditRequiresEveryRustLockfile(t *testing.T) {
	cargoBin, database, version := integrationCargoAuditPrerequisites(t)
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "packages", "a", "Cargo.toml"), "[package]\nname = \"a\"\nversion = \"0.1.0\"\n")
	writeFile(t, filepath.Join(root, "packages", "b", "Cargo.toml"), "[package]\nname = \"b\"\nversion = \"0.1.0\"\n")
	lockfileData, err := os.ReadFile(filepath.Join("testdata", "cargo-audit", "Cargo.lock"))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, "packages", "a", "Cargo.lock"), string(lockfileData))
	before, _ := rulesIdentity(filepath.Join(root, "packages", "a", "Cargo.lock"))

	adapter, err := BuildAdapter(config.SecurityAdapterConfig{
		ID: AdapterIDCargoAudit, Category: config.SecurityCategoryDependencies,
		Protocol: config.SecurityProtocolBuiltin, Command: []string{cargoBin, "audit"},
		Version: version,
	}, BuildOptions{
		Timeout: 2 * time.Minute, MaxOutputBytes: 4 << 20,
		DatabasePath: database, StalePolicy: StaleAllow,
	})
	if err != nil {
		t.Fatal(err)
	}
	execResult, err := adapter.Scan(context.Background(), Request{ProtocolVersion: ProtocolVersion, RunID: "cargo-obligation", WorkspaceRoot: root, Scope: Scope{Mode: "workspace"}})
	if err != nil {
		t.Fatal(err)
	}
	if execResult.Status != StatusFailed || execResult.Coverage != CoveragePartial {
		t.Fatalf("status/coverage = %s/%s, want FAILED/PARTIAL (%v)", execResult.Status, execResult.Coverage, execResult.Diagnostics)
	}
	if !diagnosticContains(execResult.Diagnostics, "missing dependency input packages/b/Cargo.lock") {
		t.Fatalf("diagnostics = %v, want the missing package obligation", execResult.Diagnostics)
	}
	after, _ := rulesIdentity(filepath.Join(root, "packages", "a", "Cargo.lock"))
	if before != after {
		t.Fatal("cargo-audit modified the present lockfile")
	}
	if _, err := os.Stat(filepath.Join(root, "packages", "b", "Cargo.lock")); err == nil {
		t.Fatal("cargo-audit generated a missing lockfile")
	}
}
