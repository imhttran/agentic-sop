//go:build security_integration

package security

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// realOSVDatabase writes valid OSV advisory entries into the pinned offline
// database layout. Tests use a controlled fixture instead of downloading the
// production database, so the expected findings are deterministic.
func realOSVDatabase(t *testing.T, root string, ecosystems []string) {
	t.Helper()
	goAdvisory := `{
  "id": "GO-2024-0001",
  "modified": "2024-01-01T00:00:00Z",
  "published": "2024-01-01T00:00:00Z",
  "aliases": ["CVE-2024-0001"],
  "summary": "Vulnerable Go package",
  "details": "A controlled integration advisory.",
  "affected": [{"package": {"name": "example.com/vulnerable", "ecosystem": "Go"}, "ranges": [{"type": "SEMVER", "events": [{"introduced": "0"}, {"fixed": "1.0.1"}]}]}],
  "severity": [{"type": "CVSS_V3", "score": "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"}],
  "references": [{"type": "ADVISORY", "url": "https://example.com/go-advisory"}]
}`
	rustAdvisory := `{
  "id": "RUSTSEC-2024-0001",
  "modified": "2024-01-01T00:00:00Z",
  "published": "2024-01-01T00:00:00Z",
  "aliases": ["CVE-2024-0002"],
  "summary": "Vulnerable Rust crate",
  "details": "A controlled integration advisory.",
  "affected": [{"package": {"name": "vulnerable", "ecosystem": "crates.io"}, "ranges": [{"type": "SEMVER", "events": [{"introduced": "0"}, {"fixed": "0.2.1"}]}]}],
  "database_specific": {"severity": "HIGH"},
  "references": [{"type": "ADVISORY", "url": "https://example.com/rust-advisory"}]
}`
	for _, ecosystem := range ecosystems {
		dir := filepath.Join(root, "osv-scalibr", ecosystem)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "all.zip")
		file, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		archive := zip.NewWriter(file)
		body := goAdvisory
		if ecosystem == "crates.io" {
			body = rustAdvisory
		}
		entry, err := archive.Create("advisory.json")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
		if err := archive.Close(); err != nil {
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func realOSVWorkspace(t *testing.T, vulnerable bool) string {
	t.Helper()
	dir := t.TempDir()
	goDependency := "example.com/clean v1.2.3"
	rustDependency := "clean"
	rustVersion := "1.0.0"
	if vulnerable {
		goDependency = "example.com/vulnerable v1.0.0"
		rustDependency = "vulnerable"
		rustVersion = "0.2.0"
	}
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/app\n\ngo 1.21\n\nrequire "+goDependency+"\n")
	writeFile(t, filepath.Join(dir, "nested", "go.mod"), "module example.com/nested\n\ngo 1.21\n\nrequire example.com/clean v1.2.3\n")
	writeFile(t, filepath.Join(dir, "Cargo.toml"), "[workspace]\nmembers = [\"crates/app\"]\n")
	writeFile(t, filepath.Join(dir, "crates", "app", "Cargo.toml"), "[package]\nname = \"app\"\nversion = \"0.1.0\"\nedition = \"2021\"\n")
	writeFile(t, filepath.Join(dir, "crates", "app", "src", "main.rs"), "fn main() {}\n")
	writeFile(t, filepath.Join(dir, "Cargo.lock"), "version = 3\n\n[[package]]\nname = \""+rustDependency+"\"\nversion = \""+rustVersion+"\"\nsource = \"registry+https://github.com/rust-lang/crates.io-index\"\nchecksum = \"0000000000000000000000000000000000000000000000000000000000000000\"\n")
	return dir
}

func integrationOSVAdapter(t *testing.T, database string) *osvAdapter {
	t.Helper()
	bin := strings.TrimSpace(os.Getenv("SOP_SECURITY_OSV_BIN"))
	if bin == "" {
		if os.Getenv("SOP_SECURITY_INTEGRATION_REQUIRED") == "1" {
			t.Fatal("SOP_SECURITY_OSV_BIN is required for the security integration suite")
		}
		t.Skip("SOP_SECURITY_OSV_BIN is not set")
	}
	version := strings.TrimSpace(os.Getenv("SOP_SECURITY_OSV_VERSION"))
	if version == "" {
		version = "2.7.0"
	}
	return &osvAdapter{
		id:           AdapterIDOSVScanner,
		category:     CategoryDependencies,
		command:      []string{bin},
		databasePath: database,
		languages:    []Language{LanguageGo, LanguageRust},
		version:      version,
		configDigest: "integration",
		exec:         execRunner{},
		timeout:      2 * time.Minute,
		maxOutput:    4 << 20,
	}
}

func TestOSVRealCLIVulnerableAndClean(t *testing.T) {
	database := filepath.Join(t.TempDir(), "db")
	realOSVDatabase(t, database, []string{"Go", "crates.io"})
	adapter := integrationOSVAdapter(t, database)

	vulnerable := realOSVWorkspace(t, true)
	beforeGo, _ := rulesIdentity(filepath.Join(vulnerable, "go.mod"))
	beforeLock, _ := rulesIdentity(filepath.Join(vulnerable, "Cargo.lock"))
	exec, err := adapter.Scan(context.Background(), osvRequest(vulnerable))
	if err != nil {
		t.Fatal(err)
	}
	if exec.Status != StatusCompleted || exec.Coverage != CoverageComplete {
		t.Fatalf("vulnerable execution = %s/%s, diagnostics=%v", exec.Status, exec.Coverage, exec.Diagnostics)
	}
	byRule := map[string]Finding{}
	for _, finding := range exec.Findings {
		byRule[finding.RuleID] = finding
	}
	if finding, ok := byRule["GO-2024-0001"]; !ok || finding.Severity != SeverityCritical || finding.Path != "go.mod" {
		t.Errorf("Go finding = %+v", byRule["GO-2024-0001"])
	}
	if finding, ok := byRule["RUSTSEC-2024-0001"]; !ok || finding.Severity != SeverityHigh || finding.Path != "Cargo.lock" {
		t.Errorf("Rust finding = %+v", byRule["RUSTSEC-2024-0001"])
	}
	if exec.NativeReportDigest == "" {
		t.Error("native report digest is missing")
	}
	if !diagnosticContains(exec.Diagnostics, "Loaded Go local db") || !diagnosticContains(exec.Diagnostics, "Loaded crates.io local db") {
		t.Errorf("offline database loading was not evidenced: %v", exec.Diagnostics)
	}
	afterGo, _ := rulesIdentity(filepath.Join(vulnerable, "go.mod"))
	afterLock, _ := rulesIdentity(filepath.Join(vulnerable, "Cargo.lock"))
	if beforeGo != afterGo || beforeLock != afterLock {
		t.Fatal("OSV scan modified a repository input")
	}
	if _, err := os.Stat(filepath.Join(vulnerable, "go.sum")); err == nil {
		t.Fatal("OSV scan generated go.sum")
	}

	clean := realOSVWorkspace(t, false)
	exec, err = adapter.Scan(context.Background(), osvRequest(clean))
	if err != nil {
		t.Fatal(err)
	}
	if exec.Status != StatusCompleted || exec.Coverage != CoverageComplete {
		t.Fatalf("clean execution = %s/%s, diagnostics=%v", exec.Status, exec.Coverage, exec.Diagnostics)
	}
	if len(exec.Findings) != 0 {
		t.Fatalf("clean workspace produced findings: %+v", exec.Findings)
	}
}

func TestOSVRealCLIControlsScannerLocalSuppression(t *testing.T) {
	database := filepath.Join(t.TempDir(), "db")
	realOSVDatabase(t, database, []string{"Go", "crates.io"})
	adapter := integrationOSVAdapter(t, database)
	workspace := realOSVWorkspace(t, true)
	writeFile(t, filepath.Join(workspace, "osv-scanner.toml"), "[[IgnoredVulns]]\nid = \"GO-2024-0001\"\nreason = \"local suppression must not hide SOP findings\"\n")
	exec, err := adapter.Scan(context.Background(), osvRequest(workspace))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, finding := range exec.Findings {
		if finding.RuleID == "GO-2024-0001" {
			found = true
		}
	}
	if !found {
		t.Fatalf("scanner-local osv-scanner.toml suppressed a SOP finding: %+v", exec.Findings)
	}
}

func TestOSVRealCLIMissingEcosystemIsNotClean(t *testing.T) {
	database := filepath.Join(t.TempDir(), "db")
	realOSVDatabase(t, database, []string{"Go"})
	adapter := integrationOSVAdapter(t, database)
	workspace := realOSVWorkspace(t, true)
	exec, err := adapter.Scan(context.Background(), osvRequest(workspace))
	if err != nil {
		t.Fatal(err)
	}
	if exec.Status == StatusCompleted && exec.Coverage == CoverageComplete {
		t.Fatalf("missing crates.io database produced clean coverage: %+v", exec)
	}
	if !diagnosticContains(exec.Diagnostics, "crates.io") {
		t.Fatalf("diagnostics = %v, want missing crates.io", exec.Diagnostics)
	}
}
