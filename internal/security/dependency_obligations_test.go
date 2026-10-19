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

func rustPackages(t *testing.T, root string, names ...string) {
	t.Helper()
	for _, name := range names {
		cargoTestPackage(t, root, "packages/"+name, name)
	}
}

func rustLock(t *testing.T, root, name string) {
	t.Helper()
	cargoTestLock(t, root, "packages/"+name)
}

func cleanOSVExecutor(t *testing.T) *fakeEnvExecutor {
	t.Helper()
	return &fakeEnvExecutor{runEnv: func(ctx context.Context, dir string, argv []string, env []string, stdin []byte, maxOut int) ([]byte, []byte, int, error) {
		for _, arg := range argv {
			if arg == "--version" {
				return []byte("osv-scanner version: 2.7.0\n"), nil, 0, nil
			}
		}
		var lockfile string
		for i, arg := range argv {
			if arg == "--lockfile" && i+1 < len(argv) {
				lockfile = argv[i+1]
			}
		}
		report := `{"results":[{"source":{"path":` + jsonString(lockfile) + `,"type":"lockfile"},"packages":[{"package":{"name":"clean","version":"1.0.0","ecosystem":"crates.io"}}]}]}`
		return []byte(report), nil, 0, nil
	}}
}

func rustOSVAdapterForPlan(t *testing.T, root, database string, executor Executor, inputs []string) *osvAdapter {
	t.Helper()
	adapter := osvAdapterForTest(t, root, database, executor)
	adapter.languages = []Language{LanguageRust}
	adapter.inputs = append([]string(nil), inputs...)
	return adapter
}

func TestDependencyObligationIdentityChangesWithInventory(t *testing.T) {
	root := t.TempDir()
	rustPackages(t, root, "a")
	rustLock(t, root, "a")
	one, err := DetectRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	rustPackages(t, root, "b")
	rustLock(t, root, "b")
	two, err := DetectRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	if one.ObligationIdentity(LanguageRust) == two.ObligationIdentity(LanguageRust) {
		t.Fatal("adding a package did not change the obligation identity")
	}
	if len(one.SelectedObligations(LanguageRust)) != 1 || len(two.SelectedObligations(LanguageRust)) != 2 {
		t.Fatalf("obligations = %d/%d, want 1/2", len(one.SelectedObligations(LanguageRust)), len(two.SelectedObligations(LanguageRust)))
	}
}

func TestOSVRequiresEveryRustLockfile(t *testing.T) {
	root := t.TempDir()
	rustPackages(t, root, "a", "b")
	rustLock(t, root, "a")
	database := filepath.Join(t.TempDir(), "db")
	writeOSVDatabase(t, database, []string{"crates.io"}, time.Now().UTC())
	adapter := rustOSVAdapterForPlan(t, root, database, cleanOSVExecutor(t), nil)
	exec, err := adapter.Scan(context.Background(), osvRequest(root))
	if err != nil {
		t.Fatal(err)
	}
	if exec.Status != StatusFailed || exec.Coverage != CoveragePartial {
		t.Fatalf("status/coverage = %s/%s, want FAILED/PARTIAL (%v)", exec.Status, exec.Coverage, exec.Diagnostics)
	}
	if !diagnosticContains(exec.Diagnostics, "missing dependency input packages/b/Cargo.lock") {
		t.Fatalf("diagnostics = %v, want the missing package obligation", exec.Diagnostics)
	}
	if len(exec.Obligations) != 2 {
		t.Fatalf("obligations = %+v, want both packages", exec.Obligations)
	}
}

func TestOSVPreservesFindingsWhenAnObligationIsMissing(t *testing.T) {
	root := t.TempDir()
	rustPackages(t, root, "a", "b")
	rustLock(t, root, "a")
	database := filepath.Join(t.TempDir(), "db")
	writeOSVDatabase(t, database, []string{"crates.io"}, time.Now().UTC())
	executor := &fakeEnvExecutor{runEnv: func(ctx context.Context, dir string, argv []string, env []string, stdin []byte, maxOut int) ([]byte, []byte, int, error) {
		for _, arg := range argv {
			if arg == "--version" {
				return []byte("osv-scanner version: 2.7.0\n"), nil, 0, nil
			}
		}
		var lockfile string
		for i, arg := range argv {
			if arg == "--lockfile" && i+1 < len(argv) {
				lockfile = argv[i+1]
			}
		}
		return []byte(osvReportFor(lockfile, false)), nil, 1, nil
	}}
	adapter := rustOSVAdapterForPlan(t, root, database, executor, nil)
	exec, err := adapter.Scan(context.Background(), osvRequest(root))
	if err != nil {
		t.Fatal(err)
	}
	if exec.Status != StatusFailed || exec.Coverage != CoveragePartial {
		t.Fatalf("status/coverage = %s/%s, want FAILED/PARTIAL", exec.Status, exec.Coverage)
	}
	if len(exec.Findings) != 1 {
		t.Fatalf("findings = %+v, want the present input's finding preserved", exec.Findings)
	}
}

func TestOSVAllRustLockfilesPermitCompleteCoverage(t *testing.T) {
	root := t.TempDir()
	rustPackages(t, root, "a", "b")
	rustLock(t, root, "a")
	rustLock(t, root, "b")
	database := filepath.Join(t.TempDir(), "db")
	writeOSVDatabase(t, database, []string{"crates.io"}, time.Now().UTC())
	adapter := rustOSVAdapterForPlan(t, root, database, cleanOSVExecutor(t), nil)
	exec, err := adapter.Scan(context.Background(), osvRequest(root))
	if err != nil {
		t.Fatal(err)
	}
	if exec.Status != StatusCompleted || exec.Coverage != CoverageComplete {
		t.Fatalf("status/coverage = %s/%s, want COMPLETED/COMPLETE (%v)", exec.Status, exec.Coverage, exec.Diagnostics)
	}
	if len(exec.Obligations) != 2 || len(exec.Languages) != 1 || exec.Languages[0] != LanguageRust {
		t.Fatalf("evidence = obligations=%v languages=%v", exec.Obligations, exec.Languages)
	}
}

func TestOSVExplicitSubsetReportsRemainingObligations(t *testing.T) {
	root := t.TempDir()
	rustPackages(t, root, "a", "b")
	rustLock(t, root, "a")
	rustLock(t, root, "b")
	database := filepath.Join(t.TempDir(), "db")
	writeOSVDatabase(t, database, []string{"crates.io"}, time.Now().UTC())
	adapter := rustOSVAdapterForPlan(t, root, database, cleanOSVExecutor(t), []string{"packages/a/Cargo.lock"})
	exec, err := adapter.Scan(context.Background(), osvRequest(root))
	if err != nil {
		t.Fatal(err)
	}
	if exec.Status != StatusFailed || exec.Coverage != CoveragePartial {
		t.Fatalf("status/coverage = %s/%s, want FAILED/PARTIAL", exec.Status, exec.Coverage)
	}
	if !diagnosticContains(exec.Diagnostics, "omitted by the explicit OSV inputs list") {
		t.Fatalf("diagnostics = %v, want the omitted obligation", exec.Diagnostics)
	}
}

func TestOSVExcludedPackageHasNoObligation(t *testing.T) {
	root := t.TempDir()
	rustPackages(t, root, "a", "b")
	rustLock(t, root, "a")
	rustLock(t, root, "b")
	database := filepath.Join(t.TempDir(), "db")
	writeOSVDatabase(t, database, []string{"crates.io"}, time.Now().UTC())
	adapter := rustOSVAdapterForPlan(t, root, database, cleanOSVExecutor(t), nil)
	req := osvRequest(root)
	req.Scope.Excluded = []string{"packages/b"}
	exec, err := adapter.Scan(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if exec.Status != StatusCompleted || exec.Coverage != CoverageComplete {
		t.Fatalf("status/coverage = %s/%s, want COMPLETED/COMPLETE (%v)", exec.Status, exec.Coverage, exec.Diagnostics)
	}
	if len(exec.Obligations) != 1 {
		t.Fatalf("obligations = %+v, want only the in-scope package", exec.Obligations)
	}
}

func TestCargoAuditRequiresEveryRustLockfile(t *testing.T) {
	root := t.TempDir()
	rustPackages(t, root, "a", "b")
	rustLock(t, root, "a")
	adapter := cargoAdapter(t, cargoDatabaseFixture(t), "", func(context.Context, string, []string, []byte, int) ([]byte, []byte, int, error) {
		return fixture(t, "cargo-audit/report-clean.json"), nil, 0, nil
	})
	exec, err := adapter.Scan(context.Background(), requestFor(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if exec.Status != StatusFailed || exec.Coverage != CoveragePartial {
		t.Fatalf("status/coverage = %s/%s, want FAILED/PARTIAL (%v)", exec.Status, exec.Coverage, exec.Diagnostics)
	}
	if !diagnosticContains(exec.Diagnostics, "missing dependency input packages/b/Cargo.lock") {
		t.Fatalf("diagnostics = %v, want the missing package obligation", exec.Diagnostics)
	}
}

func TestGoSourceWithoutManifestIsNotClean(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "main.go"), "package main\n")
	adapter := &osvAdapter{
		id: AdapterIDOSVScanner, category: CategoryDependencies,
		languages: []Language{LanguageGo}, databasePath: "unused", version: "2.7.0",
		exec: &fakeEnvExecutor{runEnv: func(context.Context, string, []string, []string, []byte, int) ([]byte, []byte, int, error) {
			t.Fatal("OSV must not run when go.mod is missing")
			return nil, nil, 0, nil
		}},
		timeout: time.Second, maxOutput: 1 << 20,
	}
	execResult, err := adapter.Scan(context.Background(), osvRequest(root))
	if err != nil {
		t.Fatal(err)
	}
	if execResult.Status != StatusFailed || execResult.Coverage != CoverageUnknown {
		t.Fatalf("status/coverage = %s/%s, want FAILED/UNKNOWN (%v)", execResult.Status, execResult.Coverage, execResult.Diagnostics)
	}
	if !diagnosticContains(execResult.Diagnostics, "Go source detected without go.mod") {
		t.Fatalf("diagnostics = %v, want the unsupported Go input", execResult.Diagnostics)
	}
	if execResult.NoApplicableContent {
		t.Fatal("missing go.mod was treated as no applicable content")
	}
}

func TestRunnerDoesNotClaimCoverageForFailedEmptyAnalysis(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "main.go"), "package main\n")
	adapter := &osvAdapter{
		id: AdapterIDOSVScanner, category: CategoryDependencies,
		languages: []Language{LanguageGo}, databasePath: "unused", version: "2.7.0",
		exec: &fakeEnvExecutor{runEnv: func(context.Context, string, []string, []string, []byte, int) ([]byte, []byte, int, error) {
			t.Fatal("OSV must not run when go.mod is missing")
			return nil, nil, 0, nil
		}},
		timeout: time.Second, maxOutput: 1 << 20,
	}
	detection, err := DetectRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := NewRunner(RunnerConfig{
		WorkspaceRoot: root, RunID: "empty-analysis",
		Adapters: []Adapter{adapter},
		Policy:   Policy{Enabled: true, Mode: "required", Scope: "workspace", RequiredCategories: []string{"dependencies"}, FailOn: []string{"high"}, UnknownSeverity: "indeterminate"},
		Selection: map[string]AdapterSelection{
			AdapterIDOSVScanner: {Mode: SelectionExplicit, Languages: []Language{LanguageGo}},
		},
		Detection: &detection,
	})
	if err != nil {
		t.Fatal(err)
	}
	scan, err := runner.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if scan.Verdict == VerdictPass {
		t.Fatalf("required dependency gate passed without go.mod: %v", scan.Reasons)
	}
	for _, exec := range scan.Adapters {
		if exec.AdapterID == AdapterIDOSVScanner && len(exec.Languages) != 0 {
			t.Fatalf("failed empty analysis claimed languages: %v", exec.Languages)
		}
	}
	for _, category := range scan.CoverageReport {
		if category.Category == CategoryDependencies && category.State == CoverageStateCovered {
			t.Fatalf("dependency coverage reported covered without go.mod: %+v", category)
		}
	}
}

func TestNoApplicableContentIsExplicit(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "main.go"), "package main\n")
	adapter := rustOSVAdapterForPlan(t, root, "unused", cleanOSVExecutor(t), nil)
	exec, err := adapter.Scan(context.Background(), osvRequest(root))
	if err != nil {
		t.Fatal(err)
	}
	if exec.Status != StatusCompleted || !exec.NoApplicableContent {
		t.Fatalf("status/no-applicable = %s/%t, want completed no-op", exec.Status, exec.NoApplicableContent)
	}
	if len(exec.Languages) != 0 {
		t.Fatalf("languages = %v, want no claimed Rust coverage", exec.Languages)
	}
	if !diagnosticContains(exec.Diagnostics, "no applicable Rust dependency content") {
		t.Fatalf("diagnostics = %v, want an explicit no-applicable-content diagnostic", exec.Diagnostics)
	}
}

func TestOSVExplicitInputOutsideInventoryRejected(t *testing.T) {
	root := t.TempDir()
	rustPackages(t, root, "a")
	rustLock(t, root, "a")
	adapter := rustOSVAdapterForPlan(t, root, "unused", cleanOSVExecutor(t), []string{"packages/other/Cargo.lock"})
	exec, err := adapter.Scan(context.Background(), osvRequest(root))
	if err != nil {
		t.Fatal(err)
	}
	if exec.Status != StatusFailed || exec.Coverage != CoverageUnknown {
		t.Fatalf("status/coverage = %s/%s, want FAILED/UNKNOWN (%v)", exec.Status, exec.Coverage, exec.Diagnostics)
	}
	if !diagnosticContains(exec.Diagnostics, "is not an in-scope dependency obligation") {
		t.Fatalf("diagnostics = %v, want the out-of-inventory input reason", exec.Diagnostics)
	}
}

func TestDependencyObligationInventoryIsDeterministic(t *testing.T) {
	root := t.TempDir()
	rustPackages(t, root, "b", "a")
	rustLock(t, root, "a")
	first, err := DetectRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := DetectRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	if first.ObligationIdentity(LanguageRust) != second.ObligationIdentity(LanguageRust) {
		t.Fatal("obligation identity is not deterministic")
	}
	if len(first.Obligations) != 2 || !strings.Contains(first.Obligations[0].Input, "packages/") {
		t.Fatalf("obligations = %+v", first.Obligations)
	}
}

func TestMixedRepositoryCannotHideUnsupportedGoInput(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "main.go"), "package main\n")
	cargoTestPackage(t, root, ".", "rust-app")
	cargoTestLock(t, root, ".")
	database := filepath.Join(t.TempDir(), "db")
	writeOSVDatabase(t, database, []string{"crates.io"}, time.Now().UTC())
	adapter := rustOSVAdapterForPlan(t, root, database, cleanOSVExecutor(t), nil)
	adapter.languages = []Language{LanguageGo, LanguageRust}
	detection, err := DetectRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := NewRunner(RunnerConfig{
		WorkspaceRoot: root, RunID: "mixed-unsupported-go",
		Adapters: []Adapter{adapter},
		Policy:   Policy{Enabled: true, Mode: "required", Scope: "workspace", RequiredCategories: []string{"dependencies"}, FailOn: []string{"high"}, UnknownSeverity: "indeterminate"},
		Selection: map[string]AdapterSelection{
			AdapterIDOSVScanner: {Mode: SelectionExplicit, Languages: []Language{LanguageGo, LanguageRust}},
		},
		Detection: &detection,
	})
	if err != nil {
		t.Fatal(err)
	}
	scan, err := runner.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if scan.Verdict == VerdictPass {
		t.Fatalf("mixed repository passed with unsupported Go input: %v", scan.Reasons)
	}
	for _, category := range scan.CoverageReport {
		if category.Category != CategoryDependencies {
			continue
		}
		for _, language := range category.Languages {
			if language.Language == LanguageGo && language.State == CoverageStateCovered {
				t.Fatalf("unsupported Go dependency content reported covered: %+v", category)
			}
		}
	}
}

func TestResumeInvalidatesWhenObligationChanges(t *testing.T) {
	root := t.TempDir()
	rustPackages(t, root, "a")
	rustLock(t, root, "a")
	database := filepath.Join(t.TempDir(), "db")
	writeOSVDatabase(t, database, []string{"crates.io"}, time.Now().UTC())
	executor := cleanOSVExecutor(t)
	adapter := rustOSVAdapterForPlan(t, root, database, executor, nil)
	detection, err := DetectRepository(root)
	if err != nil {
		t.Fatal(err)
	}
	config := RunnerConfig{
		WorkspaceRoot: root, RunID: "obligation-resume",
		Adapters: []Adapter{adapter},
		Policy:   Policy{Enabled: true, Mode: "report-only", Scope: "workspace", FailOn: []string{"high"}, UnknownSeverity: "indeterminate"},
		Selection: map[string]AdapterSelection{
			AdapterIDOSVScanner: {Mode: SelectionExplicit, Languages: []Language{LanguageRust}},
		},
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
	if len(first.Adapters) != 1 || first.Adapters[0].Status != StatusCompleted {
		t.Fatalf("first scan = %+v", first.Adapters)
	}

	rustPackages(t, root, "b")
	runner, err = NewRunner(RunnerConfig{
		WorkspaceRoot: config.WorkspaceRoot, RunID: config.RunID,
		Adapters: config.Adapters, Policy: config.Policy, Selection: config.Selection,
		Detection: config.Detection, Prior: &first,
		PriorNativeReports: map[string][]byte{AdapterIDOSVScanner: first.Adapters[0].NativeReport},
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := runner.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Adapters) != 1 || second.Adapters[0].Reused {
		t.Fatalf("added obligation reused prior evidence: %+v", second.Adapters)
	}
}

func TestCargoAuditExplicitSubsetReportsRemainingObligations(t *testing.T) {
	root := t.TempDir()
	rustPackages(t, root, "a", "b")
	rustLock(t, root, "a")
	rustLock(t, root, "b")
	adapter := cargoAdapterWithLockfile(t, cargoDatabaseFixture(t), "packages/a/Cargo.lock", func(context.Context, string, []string, []byte, int) ([]byte, []byte, int, error) {
		return fixture(t, "cargo-audit/report-clean.json"), nil, 0, nil
	})
	exec, err := adapter.Scan(context.Background(), requestFor(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if exec.Status != StatusFailed || exec.Coverage != CoveragePartial {
		t.Fatalf("status/coverage = %s/%s, want FAILED/PARTIAL", exec.Status, exec.Coverage)
	}
	if !diagnosticContains(exec.Diagnostics, "omitted by the explicit lockfile setting") {
		t.Fatalf("diagnostics = %v, want the omitted obligation", exec.Diagnostics)
	}
}

func TestDependencyCapabilityReflectsIncompleteInventory(t *testing.T) {
	command, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	findCandidate := func(t *testing.T, candidates []AdapterCandidate, id string) AdapterCandidate {
		t.Helper()
		for _, candidate := range candidates {
			if candidate.ID == id {
				return candidate
			}
		}
		t.Fatalf("candidate %s not found in %+v", id, candidates)
		return AdapterCandidate{}
	}

	t.Run("missing obligation", func(t *testing.T) {
		root := t.TempDir()
		rustPackages(t, root, "a", "b")
		rustLock(t, root, "a")
		detection, err := DetectRepository(root)
		if err != nil {
			t.Fatal(err)
		}
		cargo := config.SecurityAdapterConfig{
			ID: AdapterIDCargoAudit, Category: config.SecurityCategoryDependencies,
			Command: []string{command}, Version: "0.21.2",
		}
		candidates := ClassifyCapabilitiesWithDatabases(detection, []config.SecurityAdapterConfig{cargo}, cargoDatabaseFixture(t), "")
		candidate := findCandidate(t, candidates, AdapterIDCargoAudit)
		if candidate.Status != CapabilityUnsupported || !strings.Contains(candidate.Reason, "missing dependency input packages/b/Cargo.lock") {
			t.Fatalf("cargo-audit candidate = %+v, want unsupported with the missing obligation", candidate)
		}

		database := filepath.Join(t.TempDir(), "db")
		writeOSVDatabase(t, database, []string{"crates.io"}, time.Now().UTC())
		osv := config.SecurityAdapterConfig{
			ID: AdapterIDOSVScanner, Category: config.SecurityCategoryDependencies,
			Command: []string{command}, Version: "2.7.0", Languages: []string{"rust"},
		}
		candidates = ClassifyCapabilitiesWithDatabases(detection, []config.SecurityAdapterConfig{osv}, "", database)
		candidate = findCandidate(t, candidates, AdapterIDOSVScanner)
		if candidate.Status != CapabilityUnsupported || !strings.Contains(candidate.Reason, "missing dependency input packages/b/Cargo.lock") {
			t.Fatalf("osv candidate = %+v, want unsupported with the missing obligation", candidate)
		}

		proposed := SelectCandidatesWithDatabases(detection, nil, cargoDatabaseFixture(t), "")
		candidate = findCandidate(t, proposed, AdapterIDCargoAudit)
		if candidate.Status != CapabilityUnsupported || !strings.Contains(candidate.Reason, "missing dependency input packages/b/Cargo.lock") {
			t.Fatalf("proposed cargo-audit = %+v, want unsupported with the missing obligation", candidate)
		}
	})

	t.Run("explicit subset", func(t *testing.T) {
		root := t.TempDir()
		rustPackages(t, root, "a", "b")
		rustLock(t, root, "a")
		rustLock(t, root, "b")
		detection, err := DetectRepository(root)
		if err != nil {
			t.Fatal(err)
		}
		cargo := config.SecurityAdapterConfig{
			ID: AdapterIDCargoAudit, Category: config.SecurityCategoryDependencies,
			Command: []string{command}, Version: "0.21.2", Lockfile: "packages/a/Cargo.lock",
		}
		candidates := ClassifyCapabilitiesWithDatabases(detection, []config.SecurityAdapterConfig{cargo}, cargoDatabaseFixture(t), "")
		candidate := findCandidate(t, candidates, AdapterIDCargoAudit)
		if candidate.Status != CapabilityUnsupported || !strings.Contains(candidate.Reason, "omitted by the explicit dependency input list") {
			t.Fatalf("cargo-audit candidate = %+v, want unsupported with the omitted obligation", candidate)
		}
	})

	t.Run("invalid layout", func(t *testing.T) {
		root := t.TempDir()
		cargoTestFile(t, root, "Cargo.toml", "[workspace\nmembers = [\"packages/a\"]\n")
		cargoTestPackage(t, root, "packages/a", "a")
		cargoTestLock(t, root, "packages/a")
		detection, err := DetectRepository(root)
		if err != nil {
			t.Fatal(err)
		}
		if len(detection.Issues) == 0 {
			t.Fatal("invalid TOML produced no detection issue")
		}
		cargo := config.SecurityAdapterConfig{
			ID: AdapterIDCargoAudit, Category: config.SecurityCategoryDependencies,
			Command: []string{command}, Version: "0.21.2",
		}
		candidates := ClassifyCapabilitiesWithDatabases(detection, []config.SecurityAdapterConfig{cargo}, cargoDatabaseFixture(t), "")
		candidate := findCandidate(t, candidates, AdapterIDCargoAudit)
		if candidate.Status != CapabilityUnsupported || !strings.Contains(candidate.Reason, "unsupported or ambiguous") {
			t.Fatalf("cargo-audit candidate = %+v, want unsupported with the invalid layout", candidate)
		}
	})
}

func cargoAdapterWithLockfile(t *testing.T, database, lockfile string, run func(context.Context, string, []string, []byte, int) ([]byte, []byte, int, error)) Adapter {
	t.Helper()
	wrapped := func(ctx context.Context, dir string, argv []string, stdin []byte, max int) ([]byte, []byte, int, error) {
		if containsArg(argv, "--version") {
			return []byte("cargo-audit 0.21.2\n"), nil, 0, nil
		}
		return run(ctx, dir, argv, stdin, max)
	}
	adapter, err := BuildAdapter(config.SecurityAdapterConfig{
		ID: AdapterIDCargoAudit, Category: config.SecurityCategoryDependencies,
		Protocol: config.SecurityProtocolBuiltin, Command: []string{"cargo", "audit"},
		Version: "0.21.2", Lockfile: lockfile,
	}, BuildOptions{
		Executor: fakeExecutor{run: wrapped}, Timeout: time.Second, MaxOutputBytes: 1 << 20,
		DatabasePath: database,
	})
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}
