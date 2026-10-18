package security

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/config"
)

type fakeExecutor struct {
	run func(ctx context.Context, dir string, argv []string, stdin []byte, maxOut int) ([]byte, []byte, int, error)
}

func (f fakeExecutor) Run(ctx context.Context, dir string, argv []string, stdin []byte, maxOut int) ([]byte, []byte, int, error) {
	return f.run(ctx, dir, argv, stdin, maxOut)
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

func tempWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// goRules writes a minimal local rules file that declares Go coverage. A
// Semgrep scan may only claim Go coverage when the rules actually declare it.
func goRules(t *testing.T, root string) string {
	t.Helper()
	path := filepath.Join(root, "rules.yaml")
	body := "rules:\n  - id: test-go-rule\n    languages: [go]\n    message: test\n    severity: WARNING\n    pattern: $X\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func requestFor(t *testing.T, root string) Request {
	t.Helper()
	content, err := ComputeContentIdentity(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	return Request{
		ProtocolVersion: ProtocolVersion,
		RunID:           "run-1",
		WorkspaceRoot:   root,
		Content:         content,
		Scope:           Scope{Mode: "workspace"},
	}
}

func TestExcludedPathMatchesDeclaredScope(t *testing.T) {
	patterns := NormalizeExcludes([]string{
		"./internal/security/testdata/",
		"internal/legacy/*.go",
		"generated/**",
		"exact.txt",
		"internal/security/testdata",
		"/sop",
		"/sop-*",
	})
	for _, path := range []string{
		"internal/security/testdata/fixtures/secret.txt",
		"internal/legacy/old.go",
		"generated/deep/nested/file.go",
		"exact.txt",
		"sop",
		"sop-agent",
	} {
		if !ExcludedPath(path, patterns) {
			t.Errorf("%q was not excluded by %v", path, patterns)
		}
	}
	for _, path := range []string{"internal/legacy/nested/old.go", "internal/security/real.go", "notexact.txt", "cmd/sop-controller/main.go", "cmd/sop/main.go"} {
		if ExcludedPath(path, patterns) {
			t.Errorf("%q was unexpectedly excluded by %v", path, patterns)
		}
	}
}

func TestSemgrepAdapterPassesConfiguredExcludes(t *testing.T) {
	root := tempWorkspace(t)
	rules := goRules(t, root)
	var argv []string
	adapter, err := BuildAdapter(config.SecurityAdapterConfig{
		ID: "semgrep", Category: "sast", Protocol: "builtin",
		Command: []string{"semgrep"}, Rules: rules, Version: "1.99.0",
	}, BuildOptions{Executor: fakeExecutor{run: func(_ context.Context, _ string, got []string, _ []byte, _ int) ([]byte, []byte, int, error) {
		argv = append([]string(nil), got...)
		return []byte(`{"version":"1.99.0","results":[],"errors":[]}`), nil, 0, nil
	}}, Timeout: time.Second, MaxOutputBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	req := requestFor(t, root)
	req.Scope.Excluded = []string{"internal/security/testdata", ".git"}
	if _, err := adapter.Scan(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	var excludes []string
	for i := 0; i+1 < len(argv); i++ {
		if argv[i] == "--exclude" {
			excludes = append(excludes, argv[i+1])
		}
	}
	if len(excludes) != 2 || excludes[0] != ".git" || excludes[1] != "internal/security/testdata" {
		t.Errorf("excludes = %v, argv = %v", excludes, argv)
	}
	if !containsArg(argv, "--no-rewrite-rule-ids") {
		t.Errorf("argv = %v, want --no-rewrite-rule-ids for stable rule IDs", argv)
	}
}

func TestSemgrepAdapterChangedFilesTargetsAreSafe(t *testing.T) {
	root := tempWorkspace(t)
	rules := goRules(t, root)
	var argv []string
	report := `{"version":"1.99.0","errors":[],"results":[
		{"check_id":"r","path":"fixtures/bad.go","start":{"line":1,"col":1},"end":{"line":1,"col":2},"extra":{"severity":"ERROR","message":"m"}},
		{"check_id":"r","path":"--output=x.go","start":{"line":1,"col":1},"end":{"line":1,"col":2},"extra":{"severity":"ERROR","message":"m"}}
	]}`
	adapter, err := BuildAdapter(config.SecurityAdapterConfig{
		ID: "semgrep", Category: "sast", Protocol: "builtin",
		Command: []string{"semgrep"}, Rules: rules, Version: "1.99.0",
	}, BuildOptions{Executor: fakeExecutor{run: func(_ context.Context, _ string, got []string, _ []byte, _ int) ([]byte, []byte, int, error) {
		argv = append([]string(nil), got...)
		return []byte(report), nil, 0, nil
	}}, Timeout: time.Second, MaxOutputBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	req := requestFor(t, root)
	req.Scope = Scope{Mode: "changed-files", Files: []string{"--output=x.go", "fixtures/bad.go"}, Excluded: []string{"fixtures"}}
	exec, err := adapter.Scan(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(argv); n < 2 || argv[n-2] != "--" || argv[n-1] != "--output=x.go" {
		t.Errorf("argv tail = %v, want [-- --output=x.go] (option terminator, excluded target dropped)", argv)
	}
	if len(exec.Findings) != 1 || exec.Findings[0].Path != "--output=x.go" {
		t.Errorf("findings = %+v, want only the non-excluded file", exec.Findings)
	}
	if !strings.Contains(strings.Join(exec.Diagnostics, " "), "declared excluded paths") {
		t.Errorf("diagnostics = %v, want exclusion accounting", exec.Diagnostics)
	}

	// Every changed file excluded: semgrep must not run, since no target means ".".
	argv = nil
	req.Scope.Files = []string{"fixtures/bad.go"}
	exec, err = adapter.Scan(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if argv != nil || !strings.Contains(strings.Join(exec.Diagnostics, " "), "no changed files to scan") {
		t.Errorf("argv = %v, diagnostics = %v; want no semgrep run", argv, exec.Diagnostics)
	}
}

func containsArg(argv []string, want string) bool {
	for _, arg := range argv {
		if arg == want {
			return true
		}
	}
	return false
}

func TestGitleaksAdapterOmitsFindingsInExcludedPaths(t *testing.T) {
	root := tempWorkspace(t)
	report := []byte(`[
		{"Description":"synthetic fixture","File":"internal/security/testdata/secret.txt","StartLine":1,"RuleID":"fixture-rule","Secret":"REDACTED","Match":"REDACTED","Fingerprint":"fixture"},
		{"Description":"real finding","File":"keep.go","StartLine":2,"RuleID":"real-rule","Secret":"REDACTED","Match":"REDACTED","Fingerprint":"real"}
	]`)
	adapter, err := BuildAdapter(config.SecurityAdapterConfig{
		ID: "gitleaks", Category: "secrets", Protocol: "builtin",
		Command: []string{"gitleaks"}, Version: "8.99.0",
	}, BuildOptions{Executor: fakeExecutor{run: func(_ context.Context, _ string, argv []string, _ []byte, _ int) ([]byte, []byte, int, error) {
		path := argument(argv, "--report-path")
		if path == "" {
			t.Fatal("gitleaks invocation has no --report-path")
		}
		if err := os.WriteFile(path, report, 0o644); err != nil {
			t.Fatal(err)
		}
		return nil, nil, 0, nil
	}}, Timeout: time.Second, MaxOutputBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	req := requestFor(t, root)
	req.Scope.Excluded = []string{"internal/security/testdata"}
	exec, err := adapter.Scan(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(exec.Findings) != 1 || exec.Findings[0].Path != "keep.go" {
		t.Fatalf("findings = %+v, want only keep.go", exec.Findings)
	}
	if !strings.Contains(strings.Join(exec.Diagnostics, " "), "declared excluded paths") {
		t.Errorf("diagnostics = %v, want exclusion accounting", exec.Diagnostics)
	}
}

func TestPolicyIdentityIncludesExcludes(t *testing.T) {
	base := Policy{Enabled: true, Mode: "report-only", Scope: "workspace", FailOn: []string{"high"}, UnknownSeverity: "indeterminate", TimeoutSeconds: 300, MaxOutputBytes: 1 << 20}
	withExclude := base
	withExclude.Exclude = []string{"internal/security/testdata"}
	if base.Identity(Baseline{}) == withExclude.Identity(Baseline{}) {
		t.Fatal("policy identity did not change when exclusions changed")
	}
}

func TestSemgrepCoverageGapIgnoresDeclaredExcludes(t *testing.T) {
	cases := []struct {
		name    string
		stderr  string
		partial bool
	}{
		{"declared exclude only", "Scan skipped:\n   ◦ Matching --exclude patterns: 4\n", false},
		{"semgrepignore", "Scan skipped:\n   ◦ Files matching .semgrepignore patterns: 581\n", true},
		{"size limit", "Scan skipped:\n   ◦ Files larger than 1.0 MB: 6\n", true},
		{"git restriction", "Scan skipped:\n   ◦ Scan was limited to files tracked by git\n", true},
		{"unknown reason", "Scan skipped:\n   ◦ Some new scanner limit: 3\n", true},
		{"unknown bullet reason", "• Scan skipped:\n• Some new scanner limit: 3\n• Findings: 0\n", true},
	}
	for _, tc := range cases {
		if got := semgrepCoverageGap(tc.stderr); got != tc.partial {
			t.Errorf("%s: semgrepCoverageGap = %v, want %v", tc.name, got, tc.partial)
		}
	}
}

func TestSemgrepCoverageDetailRecordsSkipCounts(t *testing.T) {
	stderr := "Scan skipped:\n   ◦ Matching --exclude patterns: 4\n   ◦ Files matching .semgrepignore patterns: 581\n   ◦ Files larger than 1.0 MB: 6\n"
	detail := semgrepCoverageDetail(stderr)
	if !strings.Contains(detail, ".semgrepignore patterns: 581") || !strings.Contains(detail, "Files larger than 1.0 MB: 6") {
		t.Fatalf("detail = %q, want both scanner-side limits", detail)
	}
	if strings.Contains(detail, "--exclude") {
		t.Fatalf("detail = %q, must not report declared exclusions as a gap", detail)
	}
}

func TestGitleaksSkipNoticesForcePartial(t *testing.T) {
	files, findings, unknown := gitleaksSkipNotices("7:00PM INF skipping file: global allowlist\n7:00PM INF skipping finding: low entropy\n7:00PM INF new skip mode\n")
	if files != 1 || findings != 1 || unknown != 1 {
		t.Fatalf("skip notices = %d/%d/%d, want 1/1/1", files, findings, unknown)
	}
	root := tempWorkspace(t)
	adapter, err := BuildAdapter(config.SecurityAdapterConfig{
		ID: "gitleaks", Category: "secrets", Protocol: "builtin",
		Command: []string{"gitleaks"}, Version: "8.99.0",
	}, BuildOptions{Executor: fakeExecutor{run: func(_ context.Context, _ string, argv []string, _ []byte, _ int) ([]byte, []byte, int, error) {
		return nil, []byte("INF skipping file: global allowlist\n"), 0, os.WriteFile(argument(argv, "--report-path"), []byte("[]"), 0o644)
	}}, Timeout: time.Second, MaxOutputBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	exec, err := adapter.Scan(context.Background(), requestFor(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if exec.Coverage != CoveragePartial {
		t.Fatalf("coverage = %s, want PARTIAL for a scanner-side skip", exec.Coverage)
	}
	if !strings.Contains(strings.Join(exec.Diagnostics, " "), "scanner-side file skip") {
		t.Errorf("diagnostics = %v, want scanner skip accounting", exec.Diagnostics)
	}
}

func TestSemgrepAdapterDeclaredExcludesStayComplete(t *testing.T) {
	root := tempWorkspace(t)
	rules := goRules(t, root)
	adapter, err := BuildAdapter(config.SecurityAdapterConfig{
		ID: "semgrep", Category: "sast", Protocol: "builtin",
		Command: []string{"semgrep"}, Rules: rules, Version: "1.99.0",
	}, BuildOptions{Executor: fakeExecutor{run: func(context.Context, string, []string, []byte, int) ([]byte, []byte, int, error) {
		return []byte(`{"version":"1.99.0","results":[],"errors":[]}`), []byte("Scan skipped:\n   ◦ Matching --exclude patterns: 2\n"), 0, nil
	}}, Timeout: time.Second, MaxOutputBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	req := requestFor(t, root)
	req.Scope.Excluded = []string{"internal/security/testdata"}
	exec, err := adapter.Scan(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if exec.Coverage != CoverageComplete {
		t.Fatalf("coverage = %s, want COMPLETE for declared exclusions (%v)", exec.Coverage, exec.Diagnostics)
	}
}

func TestExcludedPathGitignoreSemantics(t *testing.T) {
	patterns := []string{
		"fixtures",
		"/root-only",
		"/internal/*/testdata",
		"**/generated/**",
		"docs/*.md",
		"**/cache",
		"*.tmp",
	}
	excluded := []string{
		"a/b/fixtures/file.go",
		"root-only/file.go",
		"internal/a/testdata/file.go",
		"nested/generated/x/file.go",
		"generated/file.go",
		"nested/cache/file",
		"docs/readme.md",
		"a/b/x.tmp",
	}
	for _, path := range excluded {
		if !ExcludedPath(path, patterns) {
			t.Errorf("%q should be excluded by %v", path, patterns)
		}
	}
	included := []string{
		"cmd/sop/main.go",
		"bar/root-only/file.go",
		"internal/a/b/testdata/file.go",
		"internal/testdata/file.go",
		"docs/sub/readme.md",
		"generatedness/file.go",
	}
	for _, path := range included {
		if ExcludedPath(path, patterns) {
			t.Errorf("%q should not be excluded by %v", path, patterns)
		}
	}
}

func TestGitleaksAdapterStagesOnlyIncludedFiles(t *testing.T) {
	root := tempWorkspace(t)
	for _, rel := range []string{"keep/secret.txt", "excluded/secret.txt", "nested/keep.go", ".env"} {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("content\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var viewDir, source, reportPath string
	var staged []string
	adapter, err := BuildAdapter(config.SecurityAdapterConfig{
		ID: "gitleaks", Category: "secrets", Protocol: "builtin",
		Command: []string{"gitleaks"}, Version: "8.99.0",
	}, BuildOptions{Executor: fakeExecutor{run: func(_ context.Context, dir string, argv []string, _ []byte, _ int) ([]byte, []byte, int, error) {
		viewDir = dir
		source = argument(argv, "--source")
		reportPath = argument(argv, "--report-path")
		staged = stagedFiles(t, dir)
		return nil, nil, 0, os.WriteFile(reportPath, []byte("[]"), 0o644)
	}}, Timeout: time.Second, MaxOutputBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	req := requestFor(t, root)
	req.Scope.Excluded = []string{"excluded"}
	exec, err := adapter.Scan(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if exec.Status != StatusCompleted {
		t.Fatalf("status = %s, want COMPLETED (%v)", exec.Status, exec.Diagnostics)
	}
	if viewDir == "" || viewDir == root {
		t.Fatalf("gitleaks did not run in a staged view: dir=%q root=%q", viewDir, root)
	}
	if source != viewDir {
		t.Errorf("--source = %q, want staged view %q", source, viewDir)
	}
	if reportPath == "" || strings.HasPrefix(reportPath, viewDir+string(filepath.Separator)) {
		t.Errorf("--report-path = %q, want a path outside the staged view", reportPath)
	}
	for _, present := range []string{"keep/secret.txt", "nested/keep.go", ".env"} {
		if !contains(staged, present) {
			t.Errorf("staged view is missing %s: %v", present, staged)
		}
	}
	if contains(staged, "excluded/secret.txt") {
		t.Errorf("excluded file was staged: %v", staged)
	}
	if !strings.Contains(strings.Join(exec.Diagnostics, " "), "outside the declared scope were not staged") {
		t.Errorf("diagnostics = %v, want staged-view accounting", exec.Diagnostics)
	}
}

func stagedFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestGitleaksAdapterSymlinkMakesCoveragePartial(t *testing.T) {
	root := tempWorkspace(t)
	if err := os.WriteFile(filepath.Join(root, "target.txt"), []byte("content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target.txt", filepath.Join(root, "link.txt")); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	adapter, err := BuildAdapter(config.SecurityAdapterConfig{
		ID: "gitleaks", Category: "secrets", Protocol: "builtin",
		Command: []string{"gitleaks"}, Version: "8.99.0",
	}, BuildOptions{Executor: fakeExecutor{run: func(_ context.Context, _ string, argv []string, _ []byte, _ int) ([]byte, []byte, int, error) {
		return nil, nil, 0, os.WriteFile(argument(argv, "--report-path"), []byte("[]"), 0o644)
	}}, Timeout: time.Second, MaxOutputBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	exec, err := adapter.Scan(context.Background(), requestFor(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if exec.Coverage != CoveragePartial {
		t.Fatalf("coverage = %s, want PARTIAL for an unfollowed symlink", exec.Coverage)
	}
	if !strings.Contains(strings.Join(exec.Diagnostics, " "), "symbolic link") {
		t.Errorf("diagnostics = %v, want symlink coverage note", exec.Diagnostics)
	}
}

func TestSemgrepAdapterParsesFixture(t *testing.T) {
	root := tempWorkspace(t)
	rules := goRules(t, root)
	report := fixture(t, "semgrep-report.json")
	adapter, err := BuildAdapter(config.SecurityAdapterConfig{
		ID: "semgrep", Category: "sast", Protocol: "builtin",
		Command: []string{"semgrep"}, Rules: rules, Version: "1.99.0",
	}, BuildOptions{
		Executor: fakeExecutor{run: func(context.Context, string, []string, []byte, int) ([]byte, []byte, int, error) {
			return report, nil, 1, nil
		}},
		Timeout: 10 * time.Second, MaxOutputBytes: 1 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
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
	if finding.Severity != SeverityHigh || finding.OriginalSeverity != "ERROR" {
		t.Errorf("severity = %s/%s, want HIGH/ERROR", finding.Severity, finding.OriginalSeverity)
	}
	if finding.Provider != "semgrep" || finding.Category != CategorySAST {
		t.Errorf("provider/category = %s/%s", finding.Provider, finding.Category)
	}
	if !strings.HasPrefix(finding.Fingerprint, "semgrep:") {
		t.Errorf("fingerprint = %q, want semgrep namespace", finding.Fingerprint)
	}
	if len(finding.CWE) != 1 || finding.CWE[0] != "CWE-78" {
		t.Errorf("CWE = %v, want CWE-78", finding.CWE)
	}
	if exec.Provenance.RulesIdentity == "" || exec.Provenance.SanitizedReportDigest == "" {
		t.Errorf("provenance = %+v, want rules and report identity", exec.Provenance)
	}
}

func TestSemgrepAdapterCleanFixture(t *testing.T) {
	root := tempWorkspace(t)
	rules := goRules(t, root)
	adapter, err := BuildAdapter(config.SecurityAdapterConfig{
		ID: "semgrep", Category: "sast", Protocol: "builtin",
		Command: []string{"semgrep"}, Rules: rules, Version: "1.99.0",
	}, BuildOptions{Executor: fakeExecutor{run: func(context.Context, string, []string, []byte, int) ([]byte, []byte, int, error) {
		return []byte(`{"version":"1.99.0","results":[],"errors":[],"paths":{"scanned":["main.go"],"skipped":[]}}`), nil, 0, nil
	}}, Timeout: time.Second, MaxOutputBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	exec, err := adapter.Scan(context.Background(), requestFor(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if len(exec.Findings) != 0 || exec.Status != StatusCompleted {
		t.Errorf("clean fixture produced %d findings status %s", len(exec.Findings), exec.Status)
	}
}

func TestSemgrepAdapterRequiresExplicitRules(t *testing.T) {
	root := tempWorkspace(t)
	adapter, err := BuildAdapter(config.SecurityAdapterConfig{
		ID: "semgrep", Category: "sast", Protocol: "builtin",
		Command: []string{"semgrep"},
	}, BuildOptions{Executor: fakeExecutor{run: func(context.Context, string, []string, []byte, int) ([]byte, []byte, int, error) {
		return fixture(t, "semgrep-report.json"), nil, 1, nil
	}}, Timeout: time.Second, MaxOutputBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	exec, err := adapter.Scan(context.Background(), requestFor(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if exec.Status != StatusFailed || exec.Coverage == CoverageComplete {
		t.Errorf("status/coverage = %s/%s, want FAILED and not COMPLETE", exec.Status, exec.Coverage)
	}
	if !strings.Contains(strings.Join(exec.Diagnostics, " "), "rules are not configured") {
		t.Errorf("diagnostics = %v, want explicit rules diagnostic", exec.Diagnostics)
	}
}

func TestGitleaksAdapterRedactsSeededSecret(t *testing.T) {
	root := tempWorkspace(t)
	report := fixture(t, "gitleaks-report.json")
	secret := "SOP_FAKE_SECRET_DO_NOT_USE_1234567890"
	rules := filepath.Join(root, "gitleaks.toml")
	if err := os.WriteFile(rules, []byte("title = \"fixture\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	adapter, err := BuildAdapter(config.SecurityAdapterConfig{
		ID: "gitleaks", Category: "secrets", Protocol: "builtin",
		Command: []string{"gitleaks"}, Rules: rules, Version: "8.99.0",
	}, BuildOptions{Executor: fakeExecutor{run: func(_ context.Context, _ string, argv []string, _ []byte, _ int) ([]byte, []byte, int, error) {
		path := argument(argv, "--report-path")
		if path == "" {
			t.Fatal("gitleaks invocation has no --report-path")
		}
		if err := os.WriteFile(path, report, 0o644); err != nil {
			t.Fatal(err)
		}
		return nil, nil, 0, nil
	}}, Timeout: time.Second, MaxOutputBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	exec, err := adapter.Scan(context.Background(), requestFor(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if exec.Status != StatusCompleted || len(exec.Findings) != 1 {
		t.Fatalf("status/findings = %s/%d, want COMPLETED/1 (%v)", exec.Status, len(exec.Findings), exec.Diagnostics)
	}
	if exec.Findings[0].Severity != SeverityHigh || exec.Findings[0].Category != CategorySecrets {
		t.Errorf("finding = %+v, want HIGH secrets", exec.Findings[0])
	}
	if strings.Contains(exec.Findings[0].Message, secret) {
		t.Error("normalized message contains the seeded secret")
	}
	if len(exec.NativeReport) == 0 || bytes.Contains(exec.NativeReport, []byte(secret)) {
		t.Errorf("native report missing or contains the seeded secret")
	}
	if exec.Provenance.RulesIdentity == "" {
		t.Error("gitleaks rules identity is empty")
	}

	// Persist every artifact and verify the secret is absent from all of them.
	runDir := t.TempDir()
	if _, err := WriteArtifacts(runDir, Scan{
		ProtocolVersion: ProtocolVersion, RunID: "run-1", Adapters: []Execution{exec},
		Findings: exec.Findings, Verdict: VerdictPass, Coverage: exec.Coverage,
		Metrics: Metrics{AdapterDurationMS: map[string]int64{}},
	}); err != nil {
		t.Fatal(err)
	}
	err = filepath.WalkDir(runDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(data, []byte(secret)) {
			t.Errorf("persisted artifact %s contains the seeded secret", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestGitleaksAdapterOversizedReportFails(t *testing.T) {
	root := tempWorkspace(t)
	adapter, err := BuildAdapter(config.SecurityAdapterConfig{
		ID: "gitleaks", Category: "secrets", Protocol: "builtin",
		Command: []string{"gitleaks"}, Version: "8.99.0",
	}, BuildOptions{Executor: fakeExecutor{run: func(_ context.Context, _ string, argv []string, _ []byte, maxOut int) ([]byte, []byte, int, error) {
		path := argument(argv, "--report-path")
		if err := os.WriteFile(path, bytes.Repeat([]byte("x"), maxOut+1), 0o644); err != nil {
			t.Fatal(err)
		}
		return nil, nil, 0, nil
	}}, Timeout: time.Second, MaxOutputBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	exec, err := adapter.Scan(context.Background(), requestFor(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if exec.Status != StatusFailed || exec.Coverage == CoverageComplete {
		t.Errorf("status/coverage = %s/%s, want FAILED and not COMPLETE", exec.Status, exec.Coverage)
	}
}

func argument(argv []string, name string) string {
	for i, arg := range argv {
		if arg == name && i+1 < len(argv) {
			return argv[i+1]
		}
	}
	return ""
}

func TestExternalAdapterValidResponseAndReplacement(t *testing.T) {
	root := tempWorkspace(t)
	response := []byte(`{
		"protocol_version":"security-scan/v1",
		"adapter_id":"private-sast",
		"category":"sast",
		"status":"COMPLETED",
		"exit_code":0,
		"coverage":"COMPLETE",
		"findings":[{"adapter_id":"private-sast","provider":"private-sast","category":"sast","rule_id":"R1","severity":"HIGH","message":"finding","path":"main.go","line":1,"fingerprint":"private-sast:1"}],
		"provenance":{"scanner_version":"1.0.0","configuration_digest":"provider-cfg","content_identity":{"commit":"provider-commit","tree":"provider-tree","digest":"provider-digest","dirty":true}}
	}`)
	fake := fakeExecutor{run: func(_ context.Context, _ string, _ []string, stdin []byte, _ int) ([]byte, []byte, int, error) {
		var req Request
		if err := json.Unmarshal(stdin, &req); err != nil {
			t.Errorf("request is not JSON: %v", err)
		}
		return response, nil, 0, nil
	}}
	adapters, err := BuildAdapters([]config.SecurityAdapterConfig{{
		ID: "private-sast", Category: "sast", Protocol: "external",
		Command: []string{"/approved/private-sast"}, Version: "1.0.0",
	}}, BuildOptions{Executor: fake, Timeout: time.Second, MaxOutputBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	runner, err := NewRunner(RunnerConfig{
		WorkspaceRoot: root, RunID: "run-1", Adapters: adapters,
		Policy:  Policy{Enabled: true, Mode: "required", Scope: "workspace", RequiredCategories: []string{"sast"}, FailOn: []string{"high"}, UnknownSeverity: "indeterminate"},
		Timeout: time.Second, MaxOutputBytes: 1 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	scan, err := runner.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if scan.Verdict != VerdictFail {
		t.Errorf("verdict = %s, want FAIL from the blocking finding", scan.Verdict)
	}
	if len(scan.Adapters) != 1 || scan.Adapters[0].Status != StatusCompleted {
		t.Fatalf("adapters = %+v", scan.Adapters)
	}
	if len(scan.Findings) != 1 || scan.Findings[0].Provider != "private-sast" {
		t.Errorf("findings = %+v", scan.Findings)
	}
	prov := scan.Adapters[0].Provenance
	if prov.ContentIdentity.Commit == "provider-commit" || prov.ContentIdentity.Tree == "provider-tree" || prov.ContentIdentity.Digest == "provider-digest" {
		t.Errorf("provider content identity was trusted: %+v", prov.ContentIdentity)
	}
	if prov.ContentIdentity.Dirty != scan.Content.Dirty {
		t.Errorf("content identity = %+v, want SOP-observed %+v", prov.ContentIdentity, scan.Content)
	}
	if prov.ConfigurationDigest == "provider-cfg" {
		t.Errorf("provider configuration digest was trusted: %+v", prov)
	}
}

func TestExternalAdapterFailureStatuses(t *testing.T) {
	root := tempWorkspace(t)
	tests := []struct {
		name   string
		run    func(context.Context, string, []string, []byte, int) ([]byte, []byte, int, error)
		status ExecutionStatus
	}{
		{"unavailable", func(context.Context, string, []string, []byte, int) ([]byte, []byte, int, error) {
			return nil, nil, -1, ErrUnavailable
		}, StatusUnavailable},
		{"timeout", func(context.Context, string, []string, []byte, int) ([]byte, []byte, int, error) {
			return nil, nil, -1, context.DeadlineExceeded
		}, StatusTimedOut},
		{"cancelled", func(context.Context, string, []string, []byte, int) ([]byte, []byte, int, error) {
			return nil, nil, -1, context.Canceled
		}, StatusCancelled},
		{"malformed", func(context.Context, string, []string, []byte, int) ([]byte, []byte, int, error) {
			return []byte("{"), nil, 0, nil
		}, StatusFailed},
		{"oversized", func(_ context.Context, _ string, _ []string, _ []byte, maxOut int) ([]byte, []byte, int, error) {
			return bytes.Repeat([]byte("x"), maxOut), nil, 0, nil
		}, StatusFailed},
		{"nonzero", func(context.Context, string, []string, []byte, int) ([]byte, []byte, int, error) {
			return []byte(`{}`), nil, 2, nil
		}, StatusFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter, err := BuildAdapter(config.SecurityAdapterConfig{
				ID: "private-sast", Category: "sast", Protocol: "external",
				Command: []string{"/approved/private-sast"}, Version: "1.0.0",
			}, BuildOptions{Executor: fakeExecutor{run: tt.run}, Timeout: time.Second, MaxOutputBytes: 1024})
			if err != nil {
				t.Fatal(err)
			}
			exec, err := adapter.Scan(context.Background(), requestFor(t, root))
			if err != nil {
				t.Fatal(err)
			}
			if exec.Status != tt.status {
				t.Errorf("status = %s, want %s (%v)", exec.Status, tt.status, exec.Diagnostics)
			}
			if exec.Status == StatusCompleted {
				t.Errorf("failure case must not be COMPLETED")
			}
		})
	}
}

func TestExternalAdapterRejectsWrongProtocolAndMissingProtocol(t *testing.T) {
	root := tempWorkspace(t)
	for _, response := range []string{
		`{"protocol_version":"security-scan/v2","adapter_id":"private-sast","category":"sast","status":"COMPLETED","coverage":"COMPLETE"}`,
		`{"adapter_id":"private-sast","category":"sast","status":"COMPLETED","coverage":"COMPLETE"}`,
	} {
		adapter, err := BuildAdapter(config.SecurityAdapterConfig{
			ID: "private-sast", Category: "sast", Protocol: "external",
			Command: []string{"/approved/private-sast"}, Version: "1.0.0",
		}, BuildOptions{Executor: fakeExecutor{run: func(context.Context, string, []string, []byte, int) ([]byte, []byte, int, error) {
			return []byte(response), nil, 0, nil
		}}, Timeout: time.Second, MaxOutputBytes: 1 << 20})
		if err != nil {
			t.Fatal(err)
		}
		exec, err := adapter.Scan(context.Background(), requestFor(t, root))
		if err != nil {
			t.Fatal(err)
		}
		if exec.Status != StatusFailed {
			t.Errorf("response %s status = %s, want FAILED", response, exec.Status)
		}
	}
}

func TestPolicyMatrix(t *testing.T) {
	completedSAST := execFor(CategorySAST, StatusCompleted, CoverageComplete)
	tests := []struct {
		name   string
		execs  []Execution
		policy Policy
		want   Verdict
	}{
		{"disabled", nil, Policy{Enabled: false}, VerdictNotApplicable},
		{"required pass", []Execution{completedSAST}, requiredPolicy("sast"), VerdictPass},
		{"required partial", []Execution{execFor(CategorySAST, StatusCompleted, CoveragePartial)}, requiredPolicy("sast"), VerdictIndeterminate},
		{"required failed", []Execution{execFor(CategorySAST, StatusFailed, CoverageUnknown)}, requiredPolicy("sast"), VerdictIndeterminate},
		{"required missing category", []Execution{execFor(CategorySecrets, StatusCompleted, CoverageComplete)}, requiredPolicy("sast"), VerdictIndeterminate},
		{"wrong category does not satisfy", []Execution{execFor(CategorySecrets, StatusCompleted, CoverageComplete)}, requiredPolicy("sast"), VerdictIndeterminate},
		{"report-only incomplete", []Execution{execFor(CategorySAST, StatusUnavailable, CoverageUnknown)}, Policy{Enabled: true, Mode: "report-only", Scope: "workspace", FailOn: []string{"high"}, UnknownSeverity: "indeterminate"}, VerdictIndeterminate},
		{"report-only partial blocking", []Execution{withFindings(execFor(CategorySAST, StatusCompleted, CoveragePartial), Finding{Severity: SeverityHigh, Fingerprint: "x"})}, Policy{Enabled: true, Mode: "report-only", Scope: "workspace", FailOn: []string{"high"}, UnknownSeverity: "indeterminate"}, VerdictFail},
		{"required failed secrets with blocking sast", []Execution{withFindings(execFor(CategorySAST, StatusCompleted, CoverageComplete), Finding{Severity: SeverityHigh, Fingerprint: "x"}), execFor(CategorySecrets, StatusUnavailable, CoverageUnknown)}, requiredPolicy("sast", "secrets"), VerdictFail},
		{"required missing category with blocking finding", []Execution{withFindings(execFor(CategorySAST, StatusCompleted, CoverageComplete), Finding{Severity: SeverityHigh, Fingerprint: "x"})}, requiredPolicy("sast", "secrets"), VerdictFail},
		{"report-only pass", []Execution{completedSAST}, Policy{Enabled: true, Mode: "report-only", Scope: "workspace", FailOn: []string{"high"}, UnknownSeverity: "indeterminate"}, VerdictPass},
		{"required changed scope", []Execution{completedSAST}, Policy{Enabled: true, Mode: "required", Scope: "changed-files", RequiredCategories: []string{"sast"}, FailOn: []string{"high"}, UnknownSeverity: "indeterminate"}, VerdictIndeterminate},
		{"blocking finding", []Execution{withFindings(completedSAST, Finding{Severity: SeverityHigh, Fingerprint: "x"})}, requiredPolicy("sast"), VerdictFail},
		{"suppressed finding", []Execution{withFindings(completedSAST, Finding{Severity: SeverityHigh, Fingerprint: "x", Suppressed: true})}, requiredPolicy("sast"), VerdictPass},
		{"unknown indeterminate", []Execution{withFindings(completedSAST, Finding{Severity: SeverityUnknown, Fingerprint: "x"})}, requiredPolicy("sast"), VerdictIndeterminate},
		{"unknown fail", []Execution{withFindings(completedSAST, Finding{Severity: SeverityUnknown, Fingerprint: "x"})}, withUnknown(requiredPolicy("sast"), "fail"), VerdictFail},
		{"unknown ignore", []Execution{withFindings(completedSAST, Finding{Severity: SeverityUnknown, Fingerprint: "x"})}, withUnknown(requiredPolicy("sast"), "ignore"), VerdictPass},
		{"unknown ignore with incomplete coverage", []Execution{withFindings(execFor(CategorySAST, StatusCompleted, CoveragePartial), Finding{Severity: SeverityUnknown, Fingerprint: "x"})}, withUnknown(requiredPolicy("sast"), "ignore"), VerdictIndeterminate},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := EvaluatePolicy(tt.execs, tt.policy).Verdict; got != tt.want {
				t.Errorf("verdict = %s, want %s", got, tt.want)
			}
		})
	}
}

func requiredPolicy(categories ...string) Policy {
	return Policy{Enabled: true, Mode: "required", Scope: "workspace", RequiredCategories: categories, FailOn: []string{"high", "critical"}, UnknownSeverity: "indeterminate"}
}

func withUnknown(p Policy, value string) Policy {
	p.UnknownSeverity = value
	return p
}

func execFor(category Category, status ExecutionStatus, coverage Coverage) Execution {
	return Execution{
		ProtocolVersion: ProtocolVersion, AdapterID: string(category) + "-adapter", Category: category,
		Status: status, Coverage: coverage,
		Findings: nil,
	}
}

func withFindings(exec Execution, findings ...Finding) Execution {
	exec.Findings = findings
	return exec
}

func TestBaselineSuppressionAndExpiry(t *testing.T) {
	now := time.Date(2026, 10, 17, 0, 0, 0, 0, time.UTC)
	fingerprint := "semgrep:one"
	baseline := Baseline{Version: 1, Entries: []BaselineEntry{
		{Fingerprint: fingerprint, Reason: "reviewed", Owner: "team", ExpiresAt: now.Add(24 * time.Hour)},
		{Fingerprint: "semgrep:expired", Reason: "old", Owner: "team", ExpiresAt: now.Add(-time.Hour)},
		{Fingerprint: "semgrep:invalid", Reason: "", Owner: "team", ExpiresAt: now.Add(time.Hour)},
	}}
	findings := []Finding{
		{RuleID: "R1", Fingerprint: fingerprint, Severity: SeverityHigh},
		{RuleID: "R2", Fingerprint: "semgrep:expired", Severity: SeverityHigh},
		{RuleID: "R3", Fingerprint: "semgrep:new", Severity: SeverityHigh},
	}
	out, diagnostics := baseline.Apply(findings, now)
	if !out[0].Suppressed || out[0].Suppression == nil || out[0].Suppression.Expired {
		t.Errorf("active suppression = %+v, want suppressing", out[0])
	}
	if out[1].Suppressed || out[1].Suppression == nil || !out[1].Suppression.Expired {
		t.Errorf("expired suppression = %+v, want visible and not suppressing", out[1])
	}
	if out[2].Suppression != nil {
		t.Errorf("new finding was unexpectedly suppressed: %+v", out[2])
	}
	if len(diagnostics) < 2 {
		t.Errorf("diagnostics = %v, want invalid and expired entries reported", diagnostics)
	}
	if !strings.Contains(strings.Join(diagnostics, " "), "expired") {
		t.Errorf("diagnostics = %v, want expired suppression visible", diagnostics)
	}
}

func TestLoadBaselineRejectsEscape(t *testing.T) {
	root := t.TempDir()
	if _, _, err := LoadBaseline(root, "../outside.json"); err == nil {
		t.Fatal("expected an escaping baseline path to be rejected")
	}
}

type stubAdapter struct {
	calls        int
	category     Category
	version      string
	configDigest string
	rules        string
}

func (s *stubAdapter) Descriptor() Descriptor {
	return Descriptor{ProtocolVersion: ProtocolVersion, AdapterID: "stub", AdapterVersion: s.version, Categories: []Category{s.category}, ScopeModes: []string{"workspace"}}
}

func (s *stubAdapter) Scan(context.Context, Request) (Execution, error) {
	s.calls++
	return Execution{
		ProtocolVersion: ProtocolVersion, AdapterID: "stub", AdapterVersion: s.version, Category: s.category,
		Status: StatusCompleted, Coverage: CoverageComplete, StartedAt: time.Now().UTC(), EndedAt: time.Now().UTC(),
		Provenance: Provenance{ScannerVersion: s.version, ConfigurationDigest: s.configDigest, RulesIdentity: s.rules},
	}, nil
}

func (s *stubAdapter) currentProvenance(req Request) (Provenance, error) {
	return Provenance{ScannerVersion: s.version, ConfigurationDigest: s.configDigest, RulesIdentity: s.rules, ContentIdentity: req.Content}, nil
}

func TestRunnerResumeUsesOnlyMatchingIdentities(t *testing.T) {
	root := tempWorkspace(t)
	policy := requiredPolicy("sast")
	baseline := Baseline{}
	content, err := ComputeContentIdentity(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	policyID := policy.Identity(baseline)
	prior := &Scan{ProtocolVersion: ProtocolVersion, Content: content, Adapters: []Execution{{
		ProtocolVersion: ProtocolVersion, AdapterID: "stub", Category: CategorySAST, Status: StatusCompleted, Coverage: CoverageComplete,
		Provenance: Provenance{
			ScannerVersion: "1.0.0", ConfigurationDigest: "cfg", RulesIdentity: "rules",
			ContentIdentity: content, PolicyIdentity: policyID,
		},
	}}}

	tests := []struct {
		name      string
		mutate    func(*Execution)
		wantReuse bool
	}{
		{"matching", func(*Execution) {}, true},
		{"stale content", func(e *Execution) { e.Provenance.ContentIdentity.Digest = "other" }, false},
		{"stale scanner", func(e *Execution) { e.Provenance.ScannerVersion = "0.9.0" }, false},
		{"stale rules", func(e *Execution) { e.Provenance.RulesIdentity = "old" }, false},
		{"stale config", func(e *Execution) { e.Provenance.ConfigurationDigest = "old" }, false},
		{"stale policy", func(e *Execution) { e.Provenance.PolicyIdentity = "old" }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := &stubAdapter{category: CategorySAST, version: "1.0.0", configDigest: "cfg", rules: "rules"}
			priorCopy := *prior
			priorCopy.Adapters = append([]Execution(nil), prior.Adapters...)
			tt.mutate(&priorCopy.Adapters[0])
			runner, err := NewRunner(RunnerConfig{
				WorkspaceRoot: root, RunID: "run-1", Adapters: []Adapter{adapter},
				Policy: policy, Timeout: time.Second, MaxOutputBytes: 1 << 20,
				Baseline: baseline, Prior: &priorCopy,
			})
			if err != nil {
				t.Fatal(err)
			}
			scan, err := runner.Scan(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			reused := len(scan.Adapters) == 1 && scan.Adapters[0].Reused
			if reused != tt.wantReuse {
				t.Errorf("reused = %t, want %t (calls=%d)", reused, tt.wantReuse, adapter.calls)
			}
			if tt.wantReuse && adapter.calls != 0 {
				t.Errorf("matching resume executed the adapter %d time(s)", adapter.calls)
			}
			if !tt.wantReuse && adapter.calls != 1 {
				t.Errorf("mismatched resume executed the adapter %d time(s), want 1", adapter.calls)
			}
		})
	}
}

func TestLoadNativeReportsVerifiesDigest(t *testing.T) {
	runDir := t.TempDir()
	native := []byte(`{"adapter":"stub","ok":true}`)
	scan := Scan{
		ProtocolVersion: ProtocolVersion, RunID: "run-1",
		Adapters: []Execution{{
			ProtocolVersion: ProtocolVersion, AdapterID: "stub", Category: CategorySAST,
			Status: StatusCompleted, Coverage: CoverageComplete, NativeReport: native,
			Provenance: Provenance{SanitizedReportDigest: DigestBytes(native)},
		}},
	}
	if _, err := WriteArtifacts(runDir, scan); err != nil {
		t.Fatal(err)
	}
	loaded := LoadNativeReports(runDir, &scan)
	if !bytes.Equal(loaded["stub"], native) {
		t.Fatalf("loaded native report = %q, want verified report", loaded["stub"])
	}
	path := filepath.Join(runDir, "security", "stub.native.json")
	if err := os.WriteFile(path, []byte(`{"tampered":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := LoadNativeReports(runDir, &scan); len(got) != 0 {
		t.Fatalf("tampered native report was accepted: %v", got)
	}
}

func TestResumeUsesVerifiedNativeReport(t *testing.T) {
	root := tempWorkspace(t)
	policy := requiredPolicy("sast")
	baseline := Baseline{}
	content, err := ComputeContentIdentity(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	policyID := policy.Identity(baseline)
	native := []byte(`{"adapter":"stub","ok":true}`)
	prior := &Scan{ProtocolVersion: ProtocolVersion, Content: content, Adapters: []Execution{{
		ProtocolVersion: ProtocolVersion, AdapterID: "stub", Category: CategorySAST, Status: StatusCompleted, Coverage: CoverageComplete,
		Provenance: Provenance{
			ScannerVersion: "1.0.0", ConfigurationDigest: "cfg", RulesIdentity: "rules",
			ContentIdentity: content, PolicyIdentity: policyID, SanitizedReportDigest: DigestBytes(native),
		},
	}}}
	tests := []struct {
		name      string
		native    map[string][]byte
		wantReuse bool
		wantCalls int
	}{
		{"verified report", map[string][]byte{"stub": native}, true, 0},
		{"missing report", map[string][]byte{}, false, 1},
		{"tampered report", map[string][]byte{"stub": []byte(`{"tampered":true}`)}, false, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := &stubAdapter{category: CategorySAST, version: "1.0.0", configDigest: "cfg", rules: "rules"}
			runner, err := NewRunner(RunnerConfig{
				WorkspaceRoot: root, RunID: "run-1", Adapters: []Adapter{adapter},
				Policy: policy, Timeout: time.Second, MaxOutputBytes: 1 << 20,
				Baseline: baseline, Prior: prior, PriorNativeReports: tt.native,
			})
			if err != nil {
				t.Fatal(err)
			}
			scan, err := runner.Scan(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			reused := len(scan.Adapters) == 1 && scan.Adapters[0].Reused
			if reused != tt.wantReuse || adapter.calls != tt.wantCalls {
				t.Fatalf("reused=%t calls=%d, want reused=%t calls=%d", reused, adapter.calls, tt.wantReuse, tt.wantCalls)
			}
			if tt.wantReuse && !bytes.Equal(scan.Adapters[0].NativeReport, native) {
				t.Fatalf("reused native report = %q, want verified report", scan.Adapters[0].NativeReport)
			}
		})
	}
}

func TestRunnerDisabledIsNotApplicable(t *testing.T) {
	root := tempWorkspace(t)
	runner, err := NewRunner(RunnerConfig{WorkspaceRoot: root, RunID: "run-1", Policy: Policy{Enabled: false}})
	if err != nil {
		t.Fatal(err)
	}
	scan, err := runner.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if scan.Verdict != VerdictNotApplicable {
		t.Errorf("verdict = %s, want NOT_APPLICABLE", scan.Verdict)
	}
	if len(scan.Adapters) != 0 {
		t.Errorf("disabled add-on executed adapters")
	}
}

func TestContentIdentityDirtyForNonGitWorkspace(t *testing.T) {
	root := tempWorkspace(t)
	content, err := ComputeContentIdentity(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if !content.Dirty || content.Digest == "" {
		t.Errorf("identity = %+v, want dirty with a digest", content)
	}
}

func TestExecutionValidationRejectsUnsafePath(t *testing.T) {
	exec := Execution{
		ProtocolVersion: ProtocolVersion, AdapterID: "a", Category: CategorySAST,
		Status: StatusCompleted, Coverage: CoverageComplete,
		Findings: []Finding{{AdapterID: "a", RuleID: "R", Severity: SeverityHigh, Fingerprint: "a:1", Path: "../escape"}},
	}
	if err := exec.Validate(); err == nil {
		t.Fatal("expected an unsafe path to fail contract validation")
	}
}

func TestExternalAdapterSanitizesUnsafePath(t *testing.T) {
	root := tempWorkspace(t)
	response := []byte(`{
		"protocol_version":"security-scan/v1",
		"adapter_id":"private-sast","category":"sast","status":"COMPLETED","coverage":"COMPLETE",
		"findings":[{"rule_id":"R1","severity":"HIGH","message":"finding","path":"../escape","fingerprint":"private-sast:1"}]
	}`)
	adapter, err := BuildAdapter(config.SecurityAdapterConfig{
		ID: "private-sast", Category: "sast", Protocol: "external",
		Command: []string{"/approved/private-sast"}, Version: "1.0.0",
	}, BuildOptions{Executor: fakeExecutor{run: func(context.Context, string, []string, []byte, int) ([]byte, []byte, int, error) {
		return response, nil, 0, nil
	}}, Timeout: time.Second, MaxOutputBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	exec, err := adapter.Scan(context.Background(), requestFor(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if len(exec.Findings) != 1 || exec.Findings[0].Path != "" {
		t.Errorf("findings = %+v, want a finding with the unsafe path removed", exec.Findings)
	}
	if !strings.Contains(strings.Join(exec.Diagnostics, " "), "unsafe path rejected") {
		t.Errorf("diagnostics = %v, want an unsafe-path diagnostic", exec.Diagnostics)
	}
}

func TestFinishNeverTurnsContractFailureIntoSuccess(t *testing.T) {
	exec := Execution{ProtocolVersion: "wrong", AdapterID: "a", Category: CategorySAST, Status: StatusCompleted, Coverage: CoverageComplete}
	out, err := finish(exec, nil)
	if err != nil {
		t.Fatal(err)
	}
	if out.Status == StatusCompleted {
		t.Errorf("invalid contract stayed COMPLETED")
	}
}

func TestExternalAdapterTimeoutFromContext(t *testing.T) {
	root := tempWorkspace(t)
	adapter, err := BuildAdapter(config.SecurityAdapterConfig{
		ID: "private-sast", Category: "sast", Protocol: "external",
		Command: []string{"/approved/private-sast"}, Version: "1.0.0",
	}, BuildOptions{Executor: fakeExecutor{run: func(ctx context.Context, _ string, _ []string, _ []byte, _ int) ([]byte, []byte, int, error) {
		<-ctx.Done()
		return nil, nil, -1, ctx.Err()
	}}, Timeout: 10 * time.Millisecond, MaxOutputBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	exec, err := adapter.Scan(context.Background(), requestFor(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if exec.Status != StatusTimedOut {
		t.Errorf("status = %s, want TIMED_OUT", exec.Status)
	}
}

func TestErrUnavailableIsStable(t *testing.T) {
	if !errors.Is(ErrUnavailable, ErrUnavailable) {
		t.Fatal("sentinel is not stable")
	}
}

func TestPolicyIdentityIncludesExecutionBounds(t *testing.T) {
	base := requiredPolicy("sast")
	base.TimeoutSeconds = 10
	base.MaxOutputBytes = 1024
	changed := base
	changed.TimeoutSeconds = 20
	if base.Identity(Baseline{}) == changed.Identity(Baseline{}) {
		t.Error("policy identity did not change with the execution timeout")
	}
	changed = base
	changed.MaxOutputBytes = 2048
	if base.Identity(Baseline{}) == changed.Identity(Baseline{}) {
		t.Error("policy identity did not change with the output bound")
	}
}

func TestWriteArtifactsAvoidsNativeNameCollisions(t *testing.T) {
	runDir := t.TempDir()
	scan := Scan{
		ProtocolVersion: ProtocolVersion, RunID: "run-1",
		Metrics: Metrics{AdapterDurationMS: map[string]int64{}},
		Adapters: []Execution{
			{AdapterID: "a/b", NativeReport: []byte("{}\n")},
			{AdapterID: "a_b", NativeReport: []byte("{}\n")},
		},
	}
	refs, err := WriteArtifacts(runDir, scan)
	if err != nil {
		t.Fatal(err)
	}
	if refs.Native["a/b"] == refs.Native["a_b"] {
		t.Fatalf("native artifact paths collide: %v", refs.Native)
	}
	if len(refs.Native) != 2 {
		t.Fatalf("native artifacts = %v, want 2", refs.Native)
	}
}

func TestRedactValuesHandlesShortSecrets(t *testing.T) {
	if got := RedactValues("value=ab and ab", []string{"ab"}); strings.Contains(got, "ab") {
		t.Errorf("short secret was not redacted: %q", got)
	}
}

func TestRequiredGateRejectsPartialCoverage(t *testing.T) {
	exec := Execution{
		ProtocolVersion: ProtocolVersion, AdapterID: "semgrep", Category: CategorySAST,
		Status: StatusCompleted, Coverage: CoveragePartial,
	}
	eval := EvaluatePolicy([]Execution{exec}, Policy{
		Enabled: true, Mode: "required", Scope: "workspace",
		RequiredCategories: []string{"sast"}, FailOn: []string{"high"}, UnknownSeverity: "indeterminate",
	})
	if eval.Verdict != VerdictIndeterminate {
		t.Fatalf("verdict = %s, want INDETERMINATE for partial/unknown-skip coverage: %v", eval.Verdict, eval.Reasons)
	}
}
