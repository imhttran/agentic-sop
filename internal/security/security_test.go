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

func TestSemgrepAdapterParsesFixture(t *testing.T) {
	root := tempWorkspace(t)
	rules := filepath.Join(root, "rules.yaml")
	if err := os.WriteFile(rules, []byte("rules: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
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
	rules := filepath.Join(root, "rules.yaml")
	if err := os.WriteFile(rules, []byte("rules: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
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
		"adapter_id":"private-sast",
		"category":"sast",
		"status":"COMPLETED",
		"exit_code":0,
		"coverage":"COMPLETE",
		"findings":[{"adapter_id":"private-sast","provider":"private-sast","category":"sast","rule_id":"R1","severity":"HIGH","message":"finding","path":"main.go","line":1,"fingerprint":"private-sast:1"}],
		"provenance":{"scanner_version":"1.0.0","configuration_digest":"cfg"}
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
		{"report-only pass", []Execution{completedSAST}, Policy{Enabled: true, Mode: "report-only", Scope: "workspace", FailOn: []string{"high"}, UnknownSeverity: "indeterminate"}, VerdictPass},
		{"required changed scope", []Execution{completedSAST}, Policy{Enabled: true, Mode: "required", Scope: "changed-files", RequiredCategories: []string{"sast"}, FailOn: []string{"high"}, UnknownSeverity: "indeterminate"}, VerdictIndeterminate},
		{"blocking finding", []Execution{withFindings(completedSAST, Finding{Severity: SeverityHigh, Fingerprint: "x"})}, requiredPolicy("sast"), VerdictFail},
		{"suppressed finding", []Execution{withFindings(completedSAST, Finding{Severity: SeverityHigh, Fingerprint: "x", Suppressed: true})}, requiredPolicy("sast"), VerdictPass},
		{"unknown indeterminate", []Execution{withFindings(completedSAST, Finding{Severity: SeverityUnknown, Fingerprint: "x"})}, requiredPolicy("sast"), VerdictIndeterminate},
		{"unknown fail", []Execution{withFindings(completedSAST, Finding{Severity: SeverityUnknown, Fingerprint: "x"})}, withUnknown(requiredPolicy("sast"), "fail"), VerdictFail},
		{"unknown ignore", []Execution{withFindings(completedSAST, Finding{Severity: SeverityUnknown, Fingerprint: "x"})}, withUnknown(requiredPolicy("sast"), "ignore"), VerdictPass},
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
