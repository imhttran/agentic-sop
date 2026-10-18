package security

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/config"
)

func cargoWorkspace(t *testing.T, withLock bool) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Cargo.toml"), []byte("[package]\nname = \"fixture\"\nversion = \"0.1.0\"\nedition = \"2021\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if withLock {
		if err := os.WriteFile(filepath.Join(root, "Cargo.lock"), []byte("version = 3\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func cargoDatabaseFixture(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("testdata", "cargo-audit", "advisory-db"))
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func cargoAdapter(t *testing.T, database string, stale string, run func(context.Context, string, []string, []byte, int) ([]byte, []byte, int, error)) Adapter {
	t.Helper()
	// Every scan first verifies the pinned tool version. Answer that probe with the
	// pinned value so the test body only observes the real scan invocation.
	wrapped := func(ctx context.Context, dir string, argv []string, stdin []byte, max int) ([]byte, []byte, int, error) {
		if containsArg(argv, "--version") {
			return []byte("cargo-audit 0.21.2\n"), nil, 0, nil
		}
		return run(ctx, dir, argv, stdin, max)
	}
	adapter, err := BuildAdapter(config.SecurityAdapterConfig{
		ID: AdapterIDCargoAudit, Category: config.SecurityCategoryDependencies,
		Protocol: config.SecurityProtocolBuiltin, Command: []string{"cargo", "audit"},
		Version: "0.21.2",
	}, BuildOptions{
		Executor: fakeExecutor{run: wrapped}, Timeout: time.Second, MaxOutputBytes: 1 << 20,
		DatabasePath: database, StalePolicy: stale,
	})
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

func TestCargoAuditCleanFixture(t *testing.T) {
	root := cargoWorkspace(t, true)
	database := cargoDatabaseFixture(t)
	var argv []string
	adapter := cargoAdapter(t, database, "", func(_ context.Context, _ string, got []string, _ []byte, _ int) ([]byte, []byte, int, error) {
		argv = append([]string(nil), got...)
		return fixture(t, "cargo-audit/report-clean.json"), nil, 0, nil
	})
	exec, err := adapter.Scan(context.Background(), requestFor(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if exec.Status != StatusCompleted || exec.Coverage != CoverageComplete {
		t.Fatalf("status/coverage = %s/%s, want COMPLETED/COMPLETE (%v)", exec.Status, exec.Coverage, exec.Diagnostics)
	}
	if len(exec.Findings) != 0 {
		t.Fatalf("findings = %+v, want none", exec.Findings)
	}
	for _, want := range []string{"--file", "--json", "--no-fetch", "--db"} {
		if !containsArg(argv, want) {
			t.Errorf("argv = %v, want %s", argv, want)
		}
	}
	if containsArg(argv, "--stale") {
		t.Errorf("argv = %v, deny policy must not pass --stale", argv)
	}
	for _, forbidden := range []string{"fix", "fetch", "update"} {
		for _, arg := range argv {
			if arg == forbidden {
				t.Errorf("argv = %v, must not contain %q", argv, forbidden)
			}
		}
	}
	if exec.Provenance.AdvisoryDatabaseIdentity == "" || exec.Provenance.AdvisoryDatabaseMetadata == "" {
		t.Errorf("provenance = %+v, want advisory database identity", exec.Provenance)
	}
	if len(exec.NativeReport) == 0 || exec.Provenance.SanitizedReportDigest == "" {
		t.Errorf("native report was not retained")
	}
}

func TestCargoAuditSeededFixture(t *testing.T) {
	root := cargoWorkspace(t, true)
	adapter := cargoAdapter(t, cargoDatabaseFixture(t), "", func(context.Context, string, []string, []byte, int) ([]byte, []byte, int, error) {
		return fixture(t, "cargo-audit/report-vulnerable.json"), nil, 1, nil
	})
	exec, err := adapter.Scan(context.Background(), requestFor(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if exec.Status != StatusCompleted || exec.Coverage != CoverageComplete {
		t.Fatalf("status/coverage = %s/%s, want COMPLETED/COMPLETE (%v)", exec.Status, exec.Coverage, exec.Diagnostics)
	}
	if len(exec.Findings) != 1 {
		t.Fatalf("findings = %d, want 1", len(exec.Findings))
	}
	finding := exec.Findings[0]
	if finding.RuleID != "RUSTSEC-2020-0071" {
		t.Errorf("rule = %q", finding.RuleID)
	}
	if finding.Severity != SeverityHigh {
		t.Errorf("severity = %s, want HIGH from CVSS 7.5", finding.Severity)
	}
	if finding.Path != "Cargo.lock" {
		t.Errorf("path = %q, want Cargo.lock", finding.Path)
	}
	if len(finding.CVE) != 1 || finding.CVE[0] != "CVE-2020-26235" {
		t.Errorf("CVE = %v, want CVE-2020-26235", finding.CVE)
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
	if !strings.Contains(finding.Message, "patched: >=0.2.23") {
		t.Errorf("message = %q, want remediation", finding.Message)
	}
	if !strings.HasPrefix(finding.Fingerprint, "cargo-audit:") {
		t.Errorf("fingerprint = %q", finding.Fingerprint)
	}
}

func TestCargoAuditMissingLockfile(t *testing.T) {
	root := cargoWorkspace(t, false)
	adapter := cargoAdapter(t, cargoDatabaseFixture(t), "", func(context.Context, string, []string, []byte, int) ([]byte, []byte, int, error) {
		t.Fatal("cargo-audit must not run when the lockfile is missing")
		return nil, nil, 0, nil
	})
	exec, err := adapter.Scan(context.Background(), requestFor(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if exec.Status != StatusFailed || exec.Coverage == CoverageComplete {
		t.Fatalf("status/coverage = %s/%s, want failed and not complete", exec.Status, exec.Coverage)
	}
	if !strings.Contains(strings.Join(exec.Diagnostics, " "), "no Cargo.lock") {
		t.Errorf("diagnostics = %v, want missing lockfile", exec.Diagnostics)
	}
}

func TestCargoAuditMissingDatabase(t *testing.T) {
	root := cargoWorkspace(t, true)
	adapter := cargoAdapter(t, filepath.Join(root, "does-not-exist"), "", func(context.Context, string, []string, []byte, int) ([]byte, []byte, int, error) {
		t.Fatal("cargo-audit must not run when the database is unavailable")
		return nil, nil, 0, nil
	})
	exec, err := adapter.Scan(context.Background(), requestFor(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if exec.Status != StatusFailed || exec.Coverage == CoverageComplete {
		t.Fatalf("status/coverage = %s/%s, want failed and not complete", exec.Status, exec.Coverage)
	}
	if !strings.Contains(strings.Join(exec.Diagnostics, " "), "advisory database is unavailable") {
		t.Errorf("diagnostics = %v, want unavailable database", exec.Diagnostics)
	}
}

func TestCargoAuditMalformedAndTimeout(t *testing.T) {
	root := cargoWorkspace(t, true)
	database := cargoDatabaseFixture(t)
	for name, run := range map[string]func(context.Context, string, []string, []byte, int) ([]byte, []byte, int, error){
		"malformed": func(context.Context, string, []string, []byte, int) ([]byte, []byte, int, error) {
			return fixture(t, "cargo-audit/report-malformed.json"), nil, 0, nil
		},
		"no vulnerability evidence": func(context.Context, string, []string, []byte, int) ([]byte, []byte, int, error) {
			return fixture(t, "cargo-audit/report-no-vulnerabilities.json"), nil, 0, nil
		},
		"timeout": func(context.Context, string, []string, []byte, int) ([]byte, []byte, int, error) {
			return nil, nil, -1, context.DeadlineExceeded
		},
		"unavailable": func(context.Context, string, []string, []byte, int) ([]byte, []byte, int, error) {
			return nil, nil, -1, ErrUnavailable
		},
	} {
		t.Run(name, func(t *testing.T) {
			exec, err := cargoAdapter(t, database, "", run).Scan(context.Background(), requestFor(t, root))
			if err != nil {
				t.Fatal(err)
			}
			if exec.Coverage == CoverageComplete || exec.Status == StatusCompleted {
				t.Fatalf("status/coverage = %s/%s, want non-clean", exec.Status, exec.Coverage)
			}
			if len(exec.Findings) != 0 {
				t.Fatalf("findings = %+v, want none", exec.Findings)
			}
		})
	}
}

func TestCargoAuditNoDatabaseIdentityFailsClosed(t *testing.T) {
	root := cargoWorkspace(t, true)
	adapter := cargoAdapter(t, cargoDatabaseFixture(t), "", func(context.Context, string, []string, []byte, int) ([]byte, []byte, int, error) {
		return fixture(t, "cargo-audit/report-no-database.json"), nil, 0, nil
	})
	exec, err := adapter.Scan(context.Background(), requestFor(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if exec.Status != StatusFailed || exec.Coverage == CoverageComplete {
		t.Fatalf("status/coverage = %s/%s, want failed and not complete", exec.Status, exec.Coverage)
	}
	if !strings.Contains(strings.Join(exec.Diagnostics, " "), "no advisory database identity") {
		t.Errorf("diagnostics = %v", exec.Diagnostics)
	}
}

func TestCargoAuditStalePolicy(t *testing.T) {
	root := cargoWorkspace(t, true)
	database := cargoDatabaseFixture(t)
	report := fixture(t, "cargo-audit/report-stale.json")

	denyArgv := []string{}
	deny := cargoAdapter(t, database, "", func(_ context.Context, _ string, got []string, _ []byte, _ int) ([]byte, []byte, int, error) {
		denyArgv = append([]string(nil), got...)
		return report, nil, 0, nil
	})
	denyExec, err := deny.Scan(context.Background(), requestFor(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if !denyExec.Provenance.AdvisoryDatabaseStale {
		t.Fatalf("deny result did not record staleness: %+v", denyExec.Provenance)
	}
	if containsArg(denyArgv, "--stale") {
		t.Errorf("deny argv = %v, must not pass --stale", denyArgv)
	}
	evaluation := EvaluatePolicy([]Execution{denyExec}, Policy{Enabled: true, Mode: "report-only", FailOn: []string{"high"}, UnknownSeverity: "indeterminate"})
	if evaluation.Verdict == VerdictPass {
		t.Fatalf("stale deny must never pass: %+v", evaluation)
	}

	allowArgv := []string{}
	allow := cargoAdapter(t, database, StaleAllow, func(_ context.Context, _ string, got []string, _ []byte, _ int) ([]byte, []byte, int, error) {
		allowArgv = append([]string(nil), got...)
		return report, nil, 0, nil
	})
	allowExec, err := allow.Scan(context.Background(), requestFor(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if !containsArg(allowArgv, "--stale") {
		t.Errorf("allow argv = %v, want --stale", allowArgv)
	}
	if allowExec.Coverage != CoveragePartial {
		t.Errorf("allow coverage = %s, want PARTIAL", allowExec.Coverage)
	}
	if !strings.Contains(strings.Join(allowExec.Diagnostics, " "), "stale") {
		t.Errorf("allow diagnostics = %v, want stale note", allowExec.Diagnostics)
	}
}

func TestCargoAuditResumeInvalidatesOnDatabaseIdentity(t *testing.T) {
	root := cargoWorkspace(t, true)
	database := cargoDatabaseFixture(t)
	adapter := cargoAdapter(t, database, "", func(context.Context, string, []string, []byte, int) ([]byte, []byte, int, error) {
		return fixture(t, "cargo-audit/report-clean.json"), nil, 0, nil
	})
	req := requestFor(t, root)
	policy := Policy{Enabled: true, Mode: "report-only", Scope: "workspace", FailOn: []string{"high"}, UnknownSeverity: "indeterminate"}
	current, err := adapter.(provenanceProvider).currentProvenance(req)
	if err != nil {
		t.Fatal(err)
	}
	current.PolicyIdentity = policy.Identity(Baseline{})
	prior := Scan{
		ProtocolVersion: ProtocolVersion,
		RunID:           "prior",
		Policy:          policy,
		Adapters: []Execution{{
			ProtocolVersion: ProtocolVersion,
			AdapterID:       AdapterIDCargoAudit,
			Category:        CategoryDependencies,
			Status:          StatusCompleted,
			Coverage:        CoverageComplete,
			Provenance:      current,
		}},
	}
	runner, err := NewRunner(RunnerConfig{WorkspaceRoot: root, RunID: "run", Adapters: []Adapter{adapter}, Policy: policy, Prior: &prior})
	if err != nil {
		t.Fatal(err)
	}
	if _, reused := runner.maybeReuse(req, adapter.Descriptor()); !reused {
		t.Fatal("matching advisory database identity was not reused")
	}
	prior.Adapters[0].Provenance.AdvisoryDatabaseIdentity += "-changed"
	if _, reused := runner.maybeReuse(req, adapter.Descriptor()); reused {
		t.Fatal("changed advisory database identity reused stale evidence")
	}
}

func TestCargoAuditTimeoutUsesBoundedContext(t *testing.T) {
	root := cargoWorkspace(t, true)
	adapter, err := BuildAdapter(config.SecurityAdapterConfig{
		ID: AdapterIDCargoAudit, Category: config.SecurityCategoryDependencies,
		Protocol: config.SecurityProtocolBuiltin, Command: []string{"cargo", "audit"},
		Version: "0.21.2",
	}, BuildOptions{
		Executor: fakeExecutor{run: func(ctx context.Context, _ string, argv []string, _ []byte, _ int) ([]byte, []byte, int, error) {
			if containsArg(argv, "--version") {
				return []byte("cargo-audit 0.21.2\n"), nil, 0, nil
			}
			<-ctx.Done()
			return nil, nil, -1, ctx.Err()
		}},
		Timeout: 50 * time.Millisecond, MaxOutputBytes: 1 << 20,
		DatabasePath: cargoDatabaseFixture(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	exec, err := adapter.Scan(context.Background(), requestFor(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if exec.Status != StatusTimedOut {
		t.Fatalf("status = %s, want TIMED_OUT", exec.Status)
	}
}

func TestCargoAuditRejectsLockfileSymlink(t *testing.T) {
	root := cargoWorkspace(t, false)
	outside := filepath.Join(t.TempDir(), "outside.lock")
	if err := os.WriteFile(outside, []byte("version = 3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "Cargo.lock")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	adapter := cargoAdapter(t, cargoDatabaseFixture(t), "", func(context.Context, string, []string, []byte, int) ([]byte, []byte, int, error) {
		t.Fatal("cargo-audit must not run on a symlinked lockfile")
		return nil, nil, 0, nil
	})
	exec, err := adapter.Scan(context.Background(), requestFor(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if exec.Status != StatusFailed || exec.Coverage == CoverageComplete {
		t.Fatalf("status/coverage = %s/%s, want failed and not complete", exec.Status, exec.Coverage)
	}
}

func TestCargoAuditVersionMismatchFailsClosed(t *testing.T) {
	root := cargoWorkspace(t, true)
	scanCalls := 0
	adapter, err := BuildAdapter(config.SecurityAdapterConfig{
		ID: AdapterIDCargoAudit, Category: config.SecurityCategoryDependencies,
		Protocol: config.SecurityProtocolBuiltin, Command: []string{"cargo", "audit"},
		Version: "0.22.2",
	}, BuildOptions{
		Executor: fakeExecutor{run: func(_ context.Context, _ string, argv []string, _ []byte, _ int) ([]byte, []byte, int, error) {
			if containsArg(argv, "--version") {
				return []byte("cargo-audit 0.21.2\n"), nil, 0, nil
			}
			scanCalls++
			return fixture(t, "cargo-audit/report-clean.json"), nil, 0, nil
		}},
		Timeout: time.Second, MaxOutputBytes: 1 << 20,
		DatabasePath: cargoDatabaseFixture(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	execResult, err := adapter.Scan(context.Background(), requestFor(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if execResult.Status != StatusFailed || execResult.Coverage == CoverageComplete {
		t.Fatalf("status/coverage = %s/%s, want failed and not complete", execResult.Status, execResult.Coverage)
	}
	if scanCalls != 0 {
		t.Errorf("cargo-audit ran %d scan invocation(s) after a version mismatch", scanCalls)
	}
	if !strings.Contains(strings.Join(execResult.Diagnostics, " "), "version mismatch") {
		t.Errorf("diagnostics = %v, want a version mismatch", execResult.Diagnostics)
	}
}

func TestCargoAuditUnpinnedVersionFailsClosed(t *testing.T) {
	root := cargoWorkspace(t, true)
	scanCalls := 0
	adapter, err := BuildAdapter(config.SecurityAdapterConfig{
		ID: AdapterIDCargoAudit, Category: config.SecurityCategoryDependencies,
		Protocol: config.SecurityProtocolBuiltin, Command: []string{"cargo", "audit"},
	}, BuildOptions{
		Executor: fakeExecutor{run: func(_ context.Context, _ string, argv []string, _ []byte, _ int) ([]byte, []byte, int, error) {
			if containsArg(argv, "--version") {
				return []byte("cargo-audit 0.22.2\n"), nil, 0, nil
			}
			scanCalls++
			return fixture(t, "cargo-audit/report-clean.json"), nil, 0, nil
		}},
		Timeout: time.Second, MaxOutputBytes: 1 << 20,
		DatabasePath: cargoDatabaseFixture(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	execResult, err := adapter.Scan(context.Background(), requestFor(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if execResult.Status != StatusFailed || execResult.Coverage == CoverageComplete {
		t.Fatalf("status/coverage = %s/%s, want failed and not complete", execResult.Status, execResult.Coverage)
	}
	if scanCalls != 0 {
		t.Errorf("cargo-audit ran %d scan invocation(s) without a pinned version", scanCalls)
	}
	if !strings.Contains(strings.Join(execResult.Diagnostics, " "), "not pinned") {
		t.Errorf("diagnostics = %v, want an unpinned-version diagnostic", execResult.Diagnostics)
	}
}

func TestCargoAuditVersionProbeUnavailable(t *testing.T) {
	root := cargoWorkspace(t, true)
	adapter, err := BuildAdapter(config.SecurityAdapterConfig{
		ID: AdapterIDCargoAudit, Category: config.SecurityCategoryDependencies,
		Protocol: config.SecurityProtocolBuiltin, Command: []string{"cargo", "audit"},
		Version: "0.22.2",
	}, BuildOptions{
		Executor: fakeExecutor{run: func(context.Context, string, []string, []byte, int) ([]byte, []byte, int, error) {
			return nil, nil, -1, ErrUnavailable
		}},
		Timeout: time.Second, MaxOutputBytes: 1 << 20,
		DatabasePath: cargoDatabaseFixture(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	execResult, err := adapter.Scan(context.Background(), requestFor(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if execResult.Status != StatusUnavailable || execResult.Coverage == CoverageComplete {
		t.Fatalf("status/coverage = %s/%s, want unavailable and not complete", execResult.Status, execResult.Coverage)
	}
}

func TestCargoAuditUnsupportedTarget(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	adapter := cargoAdapter(t, cargoDatabaseFixture(t), "", func(context.Context, string, []string, []byte, int) ([]byte, []byte, int, error) {
		t.Fatal("cargo-audit must not run for a repository with no Rust content")
		return nil, nil, 0, nil
	})
	execResult, err := adapter.Scan(context.Background(), requestFor(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if execResult.Status != StatusFailed || execResult.Coverage == CoverageComplete {
		t.Fatalf("status/coverage = %s/%s, want failed and not complete", execResult.Status, execResult.Coverage)
	}
	if !strings.Contains(strings.Join(execResult.Diagnostics, " "), "unsupported") {
		t.Errorf("diagnostics = %v, want an unsupported-target diagnostic", execResult.Diagnostics)
	}
}

func TestCargoAuditLockfileIdentityRecordedAndResumed(t *testing.T) {
	root := cargoWorkspace(t, true)
	adapter := cargoAdapter(t, cargoDatabaseFixture(t), "", func(context.Context, string, []string, []byte, int) ([]byte, []byte, int, error) {
		return fixture(t, "cargo-audit/report-clean.json"), nil, 0, nil
	})
	policy := Policy{Enabled: true, Mode: "report-only", Scope: "workspace", FailOn: []string{"high"}, UnknownSeverity: "indeterminate"}

	execResult, err := adapter.Scan(context.Background(), requestFor(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(execResult.Provenance.LockfileIdentity, "path=Cargo.lock;digest=") {
		t.Fatalf("lockfile identity = %q, want path and digest", execResult.Provenance.LockfileIdentity)
	}
	if execResult.Provenance.ScannerVersion != "0.21.2" {
		t.Errorf("scanner version = %q, want the verified executable version 0.21.2", execResult.Provenance.ScannerVersion)
	}

	req := requestFor(t, root)
	current, err := adapter.(provenanceProvider).currentProvenance(req)
	if err != nil {
		t.Fatal(err)
	}
	if current.LockfileIdentity != execResult.Provenance.LockfileIdentity {
		t.Fatalf("currentProvenance lockfile identity = %q, want %q", current.LockfileIdentity, execResult.Provenance.LockfileIdentity)
	}
	current.PolicyIdentity = policy.Identity(Baseline{})
	prior := Scan{
		ProtocolVersion: ProtocolVersion,
		RunID:           "prior",
		Policy:          policy,
		Adapters: []Execution{{
			ProtocolVersion: ProtocolVersion,
			AdapterID:       AdapterIDCargoAudit,
			Category:        CategoryDependencies,
			Status:          StatusCompleted,
			Coverage:        CoverageComplete,
			Provenance:      current,
		}},
	}
	runner, err := NewRunner(RunnerConfig{WorkspaceRoot: root, RunID: "run", Adapters: []Adapter{adapter}, Policy: policy, Prior: &prior})
	if err != nil {
		t.Fatal(err)
	}
	if _, reused := runner.maybeReuse(req, adapter.Descriptor()); !reused {
		t.Fatal("matching lockfile identity was not reused")
	}

	if err := os.WriteFile(filepath.Join(root, "Cargo.lock"), []byte("version = 3\n\n[[package]]\nname = \"changed\"\nversion = \"1.0.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	changedReq := requestFor(t, root)
	current, err = adapter.(provenanceProvider).currentProvenance(changedReq)
	if err != nil {
		t.Fatal(err)
	}
	if current.LockfileIdentity == execResult.Provenance.LockfileIdentity {
		t.Fatal("changed lockfile content produced an unchanged lockfile identity")
	}
	if _, reused := runner.maybeReuse(changedReq, adapter.Descriptor()); reused {
		t.Fatal("changed lockfile content reused stale dependency evidence")
	}
}
