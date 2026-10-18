package security

import (
	"archive/zip"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeEnvExecutor struct {
	mu     sync.Mutex
	calls  int
	runEnv func(ctx context.Context, dir string, argv []string, env []string, stdin []byte, maxOut int) ([]byte, []byte, int, error)
}

func (f *fakeEnvExecutor) Run(ctx context.Context, dir string, argv []string, stdin []byte, maxOut int) ([]byte, []byte, int, error) {
	return f.RunEnv(ctx, dir, argv, nil, stdin, maxOut)
}

func (f *fakeEnvExecutor) RunEnv(ctx context.Context, dir string, argv []string, env []string, stdin []byte, maxOut int) ([]byte, []byte, int, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	return f.runEnv(ctx, dir, argv, env, stdin, maxOut)
}

func (f *fakeEnvExecutor) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func osvWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), "module example.com/app\n\ngo 1.21\n\nrequire example.com/vulnerable v1.0.0\n")
	writeFile(t, filepath.Join(dir, "nested", "go.mod"), "module example.com/nested\n\ngo 1.21\n\nrequire example.com/clean v1.2.3\n")
	writeFile(t, filepath.Join(dir, "Cargo.toml"), "[workspace]\nmembers = [\"crates/app\"]\n")
	writeFile(t, filepath.Join(dir, "crates", "app", "Cargo.toml"), "[package]\nname = \"app\"\nversion = \"0.1.0\"\nedition = \"2021\"\n")
	writeFile(t, filepath.Join(dir, "Cargo.lock"), "version = 3\n\n[[package]]\nname = \"vulnerable\"\nversion = \"0.2.0\"\nsource = \"registry+https://github.com/rust-lang/crates.io-index\"\nchecksum = \"0000000000000000000000000000000000000000000000000000000000000000\"\n")
	writeFile(t, filepath.Join(dir, "crates", "app", "src", "main.rs"), "fn main() {}\n")
	return dir
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeOSVDatabase creates a controlled offline database with the OSV v2.7
// layout <root>/osv-scalibr/<ecosystem>/all.zip.
func writeOSVDatabase(t *testing.T, root string, ecosystems []string, modified time.Time) {
	t.Helper()
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
		entry, err := archive.Create("advisory.json")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(`{"id":"TEST-0001"}`)); err != nil {
			t.Fatal(err)
		}
		if err := archive.Close(); err != nil {
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		if !modified.IsZero() {
			if err := os.Chtimes(path, modified, modified); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func osvAdapterForTest(t *testing.T, workspace, database string, executor Executor) *osvAdapter {
	t.Helper()
	return &osvAdapter{
		id:           AdapterIDOSVScanner,
		category:     CategoryDependencies,
		command:      []string{"osv-scanner"},
		databasePath: database,
		languages:    []Language{LanguageGo, LanguageRust},
		version:      "2.7.0",
		configDigest: "test",
		exec:         executor,
		timeout:      30 * time.Second,
		maxOutput:    1 << 20,
	}
}

func osvRequest(workspace string) Request {
	return Request{
		ProtocolVersion: ProtocolVersion,
		RunID:           "test-run",
		WorkspaceRoot:   workspace,
		Scope:           Scope{Mode: "workspace"},
	}
}

func osvReportFor(abs string, goInput bool) string {
	if goInput {
		return `{
  "results": [{
    "source": {"path": ` + jsonString(abs) + `, "type": "lockfile"},
    "packages": [{
      "package": {"name": "example.com/vulnerable", "version": "1.0.0", "ecosystem": "Go", "purl": "pkg:golang/example.com/vulnerable@1.0.0"},
      "groups": [{"ids": ["GO-2024-0001"], "aliases": ["CVE-2024-0001", "GO-2024-0001"], "max_severity": "9.8"}],
      "vulnerabilities": [{
        "id": "GO-2024-0001",
        "aliases": ["CVE-2024-0001"],
        "summary": "Vulnerable Go package",
        "details": "A test advisory.",
        "severity": [{"type": "CVSS_V3", "score": "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"}],
        "affected": [{"package": {"name": "example.com/vulnerable", "ecosystem": "Go"}, "ranges": [{"type": "SEMVER", "events": [{"introduced": "0"}, {"fixed": "1.0.1"}]}]}],
        "references": [{"type": "ADVISORY", "url": "https://example.com/go-advisory"}]
      }]
    }]
  }]
}`
	}
	return `{
  "results": [{
    "source": {"path": ` + jsonString(abs) + `, "type": "lockfile"},
    "packages": [{
      "package": {"name": "vulnerable", "version": "0.2.0", "ecosystem": "crates.io", "purl": "pkg:cargo/vulnerable@0.2.0"},
      "groups": [{"ids": ["RUSTSEC-2024-0001"], "aliases": ["CVE-2024-0002", "RUSTSEC-2024-0001"], "max_severity": ""}],
      "vulnerabilities": [{
        "id": "RUSTSEC-2024-0001",
        "aliases": ["CVE-2024-0002"],
        "summary": "Vulnerable Rust crate",
        "details": "A test advisory.",
        "database_specific": {"severity": "HIGH"},
        "affected": [{"package": {"name": "vulnerable", "ecosystem": "crates.io"}, "ranges": [{"type": "SEMVER", "events": [{"introduced": "0"}, {"fixed": "0.2.1"}]}]}],
        "references": [{"type": "ADVISORY", "url": "https://example.com/rust-advisory"}]
      }]
    }]
  }]
}`
}

func osvCleanReportFor(abs string) string {
	return `{
  "results": [{
    "source": {"path": ` + jsonString(abs) + `, "type": "lockfile"},
    "packages": [{"package": {"name": "example.com/clean", "version": "1.2.3", "ecosystem": "Go"}}]
  }]
}`
}

func jsonString(value string) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func osvFakeExecutor(t *testing.T, workspace string) *fakeEnvExecutor {
	return &fakeEnvExecutor{
		runEnv: func(ctx context.Context, dir string, argv []string, env []string, stdin []byte, maxOut int) ([]byte, []byte, int, error) {
			for _, arg := range argv {
				if arg == "--version" {
					return []byte("osv-scanner version: 2.7.0\nosv-scalibr version: 0.5.3\n"), nil, 0, nil
				}
			}
			var lockfile string
			for i, arg := range argv {
				if arg == "--lockfile" && i+1 < len(argv) {
					lockfile = argv[i+1]
				}
			}
			if lockfile == "" {
				t.Fatalf("scan invocation has no --lockfile: %v", argv)
			}
			goInput := strings.HasSuffix(lockfile, "go.mod")
			if strings.Contains(lockfile, "nested") {
				return []byte(osvCleanReportFor(lockfile)), nil, 0, nil
			}
			return []byte(osvReportFor(lockfile, goInput)), nil, 1, nil
		},
	}
}

func TestOSVNormalizesGoAndRustFindings(t *testing.T) {
	workspace := osvWorkspace(t)
	database := filepath.Join(t.TempDir(), "db")
	writeOSVDatabase(t, database, []string{"Go", "crates.io"}, time.Now().UTC())
	executor := osvFakeExecutor(t, workspace)
	adapter := osvAdapterForTest(t, workspace, database, executor)
	exec, err := adapter.Scan(context.Background(), osvRequest(workspace))
	if err != nil {
		t.Fatal(err)
	}
	if exec.Status != StatusCompleted || exec.Coverage != CoverageComplete {
		t.Fatalf("execution = %s/%s, diagnostics=%v", exec.Status, exec.Coverage, exec.Diagnostics)
	}
	if len(exec.Languages) != 2 {
		t.Fatalf("languages = %v, want Go and Rust", exec.Languages)
	}
	if len(exec.Findings) != 2 {
		t.Fatalf("findings = %d, want 2: %+v", len(exec.Findings), exec.Findings)
	}
	byRule := map[string]Finding{}
	for _, finding := range exec.Findings {
		byRule[finding.RuleID] = finding
	}
	goFinding, ok := byRule["GO-2024-0001"]
	if !ok {
		t.Fatalf("missing Go finding: %+v", exec.Findings)
	}
	if goFinding.Severity != SeverityCritical || goFinding.Path != "go.mod" {
		t.Errorf("Go finding = %+v", goFinding)
	}
	if goFinding.Metadata["fixed_versions"] != "1.0.1" {
		t.Errorf("fixed versions = %q", goFinding.Metadata["fixed_versions"])
	}
	if goFinding.Metadata["reachability"] != "not-evaluated" {
		t.Errorf("reachability = %q", goFinding.Metadata["reachability"])
	}
	if len(goFinding.CVE) != 1 || goFinding.CVE[0] != "CVE-2024-0001" {
		t.Errorf("CVE = %v", goFinding.CVE)
	}
	rustFinding, ok := byRule["RUSTSEC-2024-0001"]
	if !ok {
		t.Fatalf("missing Rust finding: %+v", exec.Findings)
	}
	if rustFinding.Severity != SeverityHigh || rustFinding.Path != "Cargo.lock" {
		t.Errorf("Rust finding = %+v", rustFinding)
	}
	if rustFinding.Metadata["fixed_versions"] != "0.2.1" {
		t.Errorf("Rust fixed versions = %q", rustFinding.Metadata["fixed_versions"])
	}
	if exec.NativeReportDigest == "" || len(exec.NativeReport) == 0 {
		t.Fatal("native report was not preserved")
	}
	if !strings.Contains(string(exec.NativeReport), `"path": "go.mod"`) {
		t.Errorf("native report did not rewrite absolute paths: %s", exec.NativeReport)
	}
}

func TestOSVMissingEcosystemDatabaseIsNotClean(t *testing.T) {
	workspace := osvWorkspace(t)
	database := filepath.Join(t.TempDir(), "db")
	writeOSVDatabase(t, database, []string{"Go"}, time.Now().UTC())
	adapter := osvAdapterForTest(t, workspace, database, osvFakeExecutor(t, workspace))
	exec, err := adapter.Scan(context.Background(), osvRequest(workspace))
	if err != nil {
		t.Fatal(err)
	}
	if exec.Status == StatusCompleted && exec.Coverage == CoverageComplete {
		t.Fatalf("missing crates.io database produced clean coverage: %+v", exec)
	}
	if !diagnosticContains(exec.Diagnostics, "crates.io") {
		t.Fatalf("diagnostics = %v, want the missing ecosystem", exec.Diagnostics)
	}
}

func TestOSVMissingDatabasePathIsNotClean(t *testing.T) {
	workspace := osvWorkspace(t)
	adapter := osvAdapterForTest(t, workspace, "", osvFakeExecutor(t, workspace))
	exec, err := adapter.Scan(context.Background(), osvRequest(workspace))
	if err != nil {
		t.Fatal(err)
	}
	if exec.Status != StatusFailed || exec.Coverage != CoverageUnknown {
		t.Fatalf("execution = %s/%s, want FAILED/UNKNOWN", exec.Status, exec.Coverage)
	}
}

func TestOSVExitAndOutputFailuresAreNotClean(t *testing.T) {
	workspace := osvWorkspace(t)
	database := filepath.Join(t.TempDir(), "db")
	writeOSVDatabase(t, database, []string{"Go", "crates.io"}, time.Now().UTC())
	for _, tc := range []struct {
		name      string
		stdout    string
		stderr    string
		exit      int
		maxOutput int
		want      string
	}{
		{name: "malformed json", stdout: "not-json", exit: 0, want: "malformed JSON"},
		{name: "empty output", stdout: "", exit: 0, want: "empty output"},
		{name: "general error", stdout: `{"results":[]}`, exit: 127, want: "exited with code 127"},
		{name: "no packages", stdout: `{"results":[]}`, exit: 128, want: "exited with code 128"},
		{name: "exit 1 without vulnerabilities", stdout: `{"results":[]}`, exit: 1, want: "reported no vulnerabilities"},
		{name: "exit 0 with vulnerabilities", stdout: osvReportFor("go.mod", true), exit: 0, want: "reported vulnerabilities"},
		{name: "truncated", stdout: osvReportFor("go.mod", true), exit: 1, maxOutput: 10, want: "byte limit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			executor := &fakeEnvExecutor{runEnv: func(ctx context.Context, dir string, argv []string, env []string, stdin []byte, maxOut int) ([]byte, []byte, int, error) {
				for _, arg := range argv {
					if arg == "--version" {
						return []byte("osv-scanner version: 2.7.0\n"), nil, 0, nil
					}
				}
				return []byte(tc.stdout), []byte(tc.stderr), tc.exit, nil
			}}
			adapter := osvAdapterForTest(t, workspace, database, executor)
			if tc.maxOutput > 0 {
				adapter.maxOutput = tc.maxOutput
			}
			exec, err := adapter.Scan(context.Background(), osvRequest(workspace))
			if err != nil {
				t.Fatal(err)
			}
			if exec.Status == StatusCompleted && exec.Coverage == CoverageComplete {
				t.Fatalf("failure produced clean coverage: %+v", exec)
			}
			if !diagnosticContains(exec.Diagnostics, tc.want) {
				t.Fatalf("diagnostics = %v, want %q", exec.Diagnostics, tc.want)
			}
		})
	}
}

func TestOSVVersionMismatchIsNotClean(t *testing.T) {
	workspace := osvWorkspace(t)
	database := filepath.Join(t.TempDir(), "db")
	writeOSVDatabase(t, database, []string{"Go", "crates.io"}, time.Now().UTC())
	executor := &fakeEnvExecutor{runEnv: func(ctx context.Context, dir string, argv []string, env []string, stdin []byte, maxOut int) ([]byte, []byte, int, error) {
		return []byte("osv-scanner version: 2.6.0\n"), nil, 0, nil
	}}
	adapter := osvAdapterForTest(t, workspace, database, executor)
	exec, err := adapter.Scan(context.Background(), osvRequest(workspace))
	if err != nil {
		t.Fatal(err)
	}
	if exec.Status != StatusFailed {
		t.Fatalf("status = %s, want FAILED", exec.Status)
	}
	if !diagnosticContains(exec.Diagnostics, "version mismatch") {
		t.Fatalf("diagnostics = %v", exec.Diagnostics)
	}
}

func TestOSVRequiresEnvironmentCapableExecutor(t *testing.T) {
	workspace := osvWorkspace(t)
	database := filepath.Join(t.TempDir(), "db")
	writeOSVDatabase(t, database, []string{"Go", "crates.io"}, time.Now().UTC())
	executor := fakeExecutor{run: func(ctx context.Context, dir string, argv []string, stdin []byte, maxOut int) ([]byte, []byte, int, error) {
		return []byte("osv-scanner version: 2.7.0\n"), nil, 0, nil
	}}
	adapter := osvAdapterForTest(t, workspace, database, executor)
	exec, err := adapter.Scan(context.Background(), osvRequest(workspace))
	if err != nil {
		t.Fatal(err)
	}
	if exec.Status != StatusFailed {
		t.Fatalf("status = %s, want FAILED", exec.Status)
	}
	if !diagnosticContains(exec.Diagnostics, "explicit environment") {
		t.Fatalf("diagnostics = %v", exec.Diagnostics)
	}
}

func TestOSVUsesControlledConfigAndExplicitOfflineFlags(t *testing.T) {
	workspace := osvWorkspace(t)
	writeFile(t, filepath.Join(workspace, "osv-scanner.toml"), "[[IgnoredVulns]]\nid = \"GO-2024-0001\"\nreason = \"probe\"\n")
	database := filepath.Join(t.TempDir(), "db")
	writeOSVDatabase(t, database, []string{"Go", "crates.io"}, time.Now().UTC())
	var configBody []byte
	var configPath string
	var envSeen []string
	executor := &fakeEnvExecutor{runEnv: func(ctx context.Context, dir string, argv []string, env []string, stdin []byte, maxOut int) ([]byte, []byte, int, error) {
		for _, arg := range argv {
			if arg == "--version" {
				return []byte("osv-scanner version: 2.7.0\n"), nil, 0, nil
			}
		}
		envSeen = append([]string(nil), env...)
		for i, arg := range argv {
			if arg == "--config" && i+1 < len(argv) {
				configPath = argv[i+1]
				configBody, _ = os.ReadFile(configPath)
			}
		}
		if !containsArg(argv, "--offline") || !containsArg(argv, "--no-call-analysis=all") {
			t.Fatalf("offline/no-call-analysis flags missing: %v", argv)
		}
		for _, arg := range argv {
			if strings.Contains(arg, "osv-scanner.toml") {
				t.Fatalf("repository osv-scanner.toml was passed to the scanner: %v", argv)
			}
		}
		var lockfile string
		for i, arg := range argv {
			if arg == "--lockfile" && i+1 < len(argv) {
				lockfile = argv[i+1]
			}
		}
		if strings.Contains(lockfile, "nested") {
			return []byte(osvCleanReportFor(lockfile)), nil, 0, nil
		}
		return []byte(osvReportFor(lockfile, strings.HasSuffix(lockfile, "go.mod"))), nil, 1, nil
	}}
	adapter := osvAdapterForTest(t, workspace, database, executor)
	exec, err := adapter.Scan(context.Background(), osvRequest(workspace))
	if err != nil {
		t.Fatal(err)
	}
	if exec.Status != StatusCompleted {
		t.Fatalf("status = %s, diagnostics=%v", exec.Status, exec.Diagnostics)
	}
	if configPath == "" || len(configBody) != 0 {
		t.Fatalf("controlled config was not empty: path=%q body=%q", configPath, string(configBody))
	}
	wantEnv := "OSV_SCANNER_LOCAL_DB_CACHE_DIRECTORY=" + database
	if !containsArg(envSeen, wantEnv) {
		t.Fatalf("explicit OSV cache environment missing: %v", envSeen)
	}
}

func TestOSVStaleDatabaseFollowsPolicy(t *testing.T) {
	workspace := osvWorkspace(t)
	database := filepath.Join(t.TempDir(), "db")
	writeOSVDatabase(t, database, []string{"Go", "crates.io"}, time.Now().UTC().Add(-180*24*time.Hour))
	adapter := osvAdapterForTest(t, workspace, database, osvFakeExecutor(t, workspace))
	adapter.stalePolicy = StaleAllow
	exec, err := adapter.Scan(context.Background(), osvRequest(workspace))
	if err != nil {
		t.Fatal(err)
	}
	if !exec.Provenance.AdvisoryDatabaseStale {
		t.Fatal("stale database was not recorded")
	}
	if exec.Coverage != CoveragePartial {
		t.Fatalf("coverage = %s, want PARTIAL under allow", exec.Coverage)
	}
	adapter.stalePolicy = StaleDeny
	exec, err = adapter.Scan(context.Background(), osvRequest(workspace))
	if err != nil {
		t.Fatal(err)
	}
	if !exec.Provenance.AdvisoryDatabaseStale {
		t.Fatal("stale database was not recorded under deny")
	}
}

func TestOSVResumeInvalidation(t *testing.T) {
	workspace := osvWorkspace(t)
	database := filepath.Join(t.TempDir(), "db")
	writeOSVDatabase(t, database, []string{"Go", "crates.io"}, time.Now().UTC())
	executor := osvFakeExecutor(t, workspace)
	adapter := osvAdapterForTest(t, workspace, database, executor)
	baseline := Baseline{Version: 1}
	policy := Policy{Enabled: true, Mode: "report-only", Scope: "workspace", FailOn: []string{"high"}, UnknownSeverity: "indeterminate"}
	detection, err := DetectRepositoryScoped(workspace, nil)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := NewRunner(RunnerConfig{
		WorkspaceRoot: workspace, RunID: "resume",
		Adapters: []Adapter{adapter}, Policy: policy, Baseline: baseline,
		Selection: map[string]AdapterSelection{AdapterIDOSVScanner: {Mode: SelectionExplicit, Languages: []Language{LanguageGo, LanguageRust}}},
		Detection: &detection,
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := runner.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Adapters) != 1 || first.Adapters[0].Status != StatusCompleted {
		t.Fatalf("first scan = %+v", first.Adapters)
	}
	native := first.Adapters[0].NativeReport

	executor.runEnv = func(ctx context.Context, dir string, argv []string, env []string, stdin []byte, maxOut int) ([]byte, []byte, int, error) {
		t.Fatal("adapter executed instead of reusing valid prior evidence")
		return nil, nil, 0, nil
	}
	runner, err = NewRunner(RunnerConfig{
		WorkspaceRoot: workspace, RunID: "resume",
		Adapters: []Adapter{adapter}, Policy: policy, Baseline: baseline,
		Selection: map[string]AdapterSelection{AdapterIDOSVScanner: {Mode: SelectionExplicit, Languages: []Language{LanguageGo, LanguageRust}}},
		Detection: &detection, Prior: &first,
		PriorNativeReports: map[string][]byte{AdapterIDOSVScanner: native},
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := runner.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Adapters) != 1 || !second.Adapters[0].Reused {
		t.Fatalf("valid prior evidence was not reused: %+v", second.Adapters)
	}

	writeFile(t, filepath.Join(workspace, "Cargo.lock"), "version = 3\n\n[[package]]\nname = \"vulnerable\"\nversion = \"0.2.1\"\nsource = \"registry+https://github.com/rust-lang/crates.io-index\"\nchecksum = \"0000000000000000000000000000000000000000000000000000000000000000\"\n")
	executor.runEnv = osvFakeExecutor(t, workspace).runEnv
	executor.calls = 0
	runner, err = NewRunner(RunnerConfig{
		WorkspaceRoot: workspace, RunID: "resume",
		Adapters: []Adapter{adapter}, Policy: policy, Baseline: baseline,
		Selection: map[string]AdapterSelection{AdapterIDOSVScanner: {Mode: SelectionExplicit, Languages: []Language{LanguageGo, LanguageRust}}},
		Detection: &detection, Prior: &second,
		PriorNativeReports: map[string][]byte{AdapterIDOSVScanner: second.Adapters[0].NativeReport},
	})
	if err != nil {
		t.Fatal(err)
	}
	third, err := runner.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(third.Adapters) != 1 || third.Adapters[0].Reused {
		t.Fatalf("changed lockfile reused prior evidence: %+v", third.Adapters)
	}
	if executor.callCount() == 0 {
		t.Fatal("changed lockfile did not re-execute the adapter")
	}
}

func TestOSVResumeRechecksFreshnessAndSelection(t *testing.T) {
	workspace := osvWorkspace(t)
	database := filepath.Join(t.TempDir(), "db")
	writeOSVDatabase(t, database, []string{"Go", "crates.io"}, time.Now().UTC())
	executor := osvFakeExecutor(t, workspace)
	adapter := osvAdapterForTest(t, workspace, database, executor)
	policy := Policy{Enabled: true, Mode: "report-only", Scope: "workspace", FailOn: []string{"high"}, UnknownSeverity: "indeterminate"}
	detection, err := DetectRepositoryScoped(workspace, nil)
	if err != nil {
		t.Fatal(err)
	}
	config := RunnerConfig{
		WorkspaceRoot: workspace, RunID: "freshness",
		Adapters: []Adapter{adapter}, Policy: policy, Baseline: Baseline{Version: 1},
		Selection: map[string]AdapterSelection{AdapterIDOSVScanner: {Mode: SelectionExplicit, Languages: []Language{LanguageGo, LanguageRust}}},
		Detection: &detection,
	}
	runner, err := NewRunner(config)
	if err != nil {
		t.Fatal(err)
	}
	first, err := runner.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// Age the unchanged database. The digest is unchanged but reuse must be
	// refused because freshness is rechecked.
	old := time.Now().UTC().Add(-180 * 24 * time.Hour)
	for _, ecosystem := range []string{"Go", "crates.io"} {
		if err := os.Chtimes(filepath.Join(database, "osv-scalibr", ecosystem, "all.zip"), old, old); err != nil {
			t.Fatal(err)
		}
	}
	aged := config
	aged.Prior = &first
	aged.PriorNativeReports = map[string][]byte{AdapterIDOSVScanner: first.Adapters[0].NativeReport}
	runner, err = NewRunner(aged)
	if err != nil {
		t.Fatal(err)
	}
	second, err := runner.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if second.Adapters[0].Reused {
		t.Fatal("aged database reused stale evidence")
	}

	// Restore a fresh database but change the selection mode. Selection is part of
	// the resume identity, so evidence is re-executed rather than reused.
	writeOSVDatabase(t, database, []string{"Go", "crates.io"}, time.Now().UTC())
	changedSelection := config
	changedSelection.Prior = &second
	changedSelection.PriorNativeReports = map[string][]byte{AdapterIDOSVScanner: second.Adapters[0].NativeReport}
	changedSelection.Selection = map[string]AdapterSelection{AdapterIDOSVScanner: {Mode: SelectionAutomatic, Languages: []Language{LanguageGo, LanguageRust}}}
	runner, err = NewRunner(changedSelection)
	if err != nil {
		t.Fatal(err)
	}
	third, err := runner.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if third.Adapters[0].Reused {
		t.Fatal("changed selection mode reused prior evidence")
	}
}

func TestOSVRequiredCoverageIsPerLanguage(t *testing.T) {
	detection := mixedDetection(t)
	policy := Policy{Enabled: true, Mode: "required", RequiredCategories: []string{"dependencies"}, FailOn: []string{"high"}, UnknownSeverity: "indeterminate"}
	execs := []Execution{
		{ProtocolVersion: ProtocolVersion, AdapterID: AdapterIDOSVScanner, Category: CategoryDependencies, Status: StatusCompleted, Coverage: CoverageComplete, Languages: []Language{LanguageGo}},
	}
	evaluation := EvaluatePolicyWithDetection(execs, policy, &detection)
	if evaluation.Verdict == VerdictPass {
		t.Fatalf("required gate passed without Rust dependency coverage: %+v", evaluation.Reasons)
	}
	if !reasonContains(evaluation.Reasons, "COMPLETE rust coverage") {
		t.Fatalf("reasons = %v, want missing Rust coverage", evaluation.Reasons)
	}
}

func TestOSVCoverageReportSeparatesLanguages(t *testing.T) {
	detection := mixedDetection(t)
	execs := []Execution{
		{ProtocolVersion: ProtocolVersion, AdapterID: AdapterIDOSVScanner, Category: CategoryDependencies, Status: StatusCompleted, Coverage: CoverageComplete, Languages: []Language{LanguageGo}},
	}
	report := CoverageByCategory(execs, &detection, Policy{Enabled: true})
	for _, category := range report {
		if category.Category != CategoryDependencies {
			continue
		}
		states := map[Language]CoverageState{}
		for _, language := range category.Languages {
			states[language.Language] = language.State
		}
		if states[LanguageGo] != CoverageStateCovered {
			t.Errorf("Go state = %s", states[LanguageGo])
		}
		if states[LanguageRust] != CoverageStateUnconfigured {
			t.Errorf("Rust state = %s, want unconfigured", states[LanguageRust])
		}
		if category.State != CoverageStatePartial {
			t.Errorf("category state = %s, want partial for covered+unconfigured", category.State)
		}
		return
	}
	t.Fatal("dependency coverage was not reported")
}

func diagnosticContains(diagnostics []string, want string) bool {
	for _, diagnostic := range diagnostics {
		if strings.Contains(diagnostic, want) {
			return true
		}
	}
	return false
}

func reasonContains(reasons []string, want string) bool {
	for _, reason := range reasons {
		if strings.Contains(reason, want) {
			return true
		}
	}
	return false
}
