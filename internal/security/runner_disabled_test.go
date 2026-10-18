package security

import (
	"context"
	"strings"
	"testing"
)

type countingAdapter struct {
	id    string
	calls int
}

func (a *countingAdapter) Descriptor() Descriptor {
	return Descriptor{
		ProtocolVersion: ProtocolVersion,
		AdapterID:       a.id,
		AdapterVersion:  "1.0.0",
		Categories:      []Category{CategorySAST},
		ScopeModes:      []string{"workspace"},
		Locality:        "local",
		Languages:       []Language{LanguageGo},
	}
}

func (a *countingAdapter) Scan(ctx context.Context, req Request) (Execution, error) {
	a.calls++
	return Execution{
		ProtocolVersion: ProtocolVersion,
		AdapterID:       a.id,
		AdapterVersion:  "1.0.0",
		Category:        CategorySAST,
		Status:          StatusCompleted,
		Coverage:        CoverageComplete,
		Languages:       []Language{LanguageGo},
	}, nil
}

func disabledEvidence(id string, category Category, languages []Language, repositoryWide bool, reason string) DisabledAdapter {
	return DisabledAdapter{
		ID: id, Category: category, Reason: reason, Languages: languages,
		RepositoryWide: repositoryWide, Selection: SelectionExplicit,
	}
}

func TestRunnerRecordsAllDisabledAdapters(t *testing.T) {
	workspace := t.TempDir()
	detection := Detection{Languages: []Language{LanguageGo, LanguageRust}}
	disabled := []DisabledAdapter{
		disabledEvidence(AdapterIDSemgrep, CategorySAST, []Language{LanguageGo, LanguageRust}, false, "adapter is disabled by configuration"),
		disabledEvidence(AdapterIDGitleaks, CategorySecrets, nil, true, "adapter is disabled by configuration"),
		disabledEvidence(AdapterIDOSVScanner, CategoryDependencies, []Language{LanguageGo, LanguageRust}, false, "adapter is disabled by configuration"),
	}
	runner, err := NewRunner(RunnerConfig{
		WorkspaceRoot: workspace, RunID: "all-disabled",
		Policy:   Policy{Enabled: true, Mode: "report-only", Scope: "workspace", FailOn: []string{"high"}, UnknownSeverity: "indeterminate"},
		Disabled: disabled, Detection: &detection,
	})
	if err != nil {
		t.Fatal(err)
	}
	scan, err := runner.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if scan.Verdict != VerdictIndeterminate {
		t.Fatalf("verdict = %s, want INDETERMINATE (%v)", scan.Verdict, scan.Reasons)
	}
	if !reasonListContains(scan.Reasons, "every adapter is disabled") {
		t.Fatalf("reasons = %v, want all-disabled explanation", scan.Reasons)
	}
	if len(scan.Adapters) != len(disabled) {
		t.Fatalf("adapters = %d, want %d disabled evidence records", len(scan.Adapters), len(disabled))
	}
	for _, exec := range scan.Adapters {
		if !exec.Disabled || exec.Status != StatusSkipped || exec.Coverage != CoverageUnknown {
			t.Fatalf("disabled execution = %+v, want SKIPPED/UNKNOWN", exec)
		}
		if !reasonListContains(exec.Diagnostics, "disabled") {
			t.Fatalf("execution diagnostics = %v, want a disabled reason", exec.Diagnostics)
		}
	}
	if scan.Metrics.ExecutionFailures != 0 {
		t.Fatalf("execution failures = %d, want 0 for intentional disables", scan.Metrics.ExecutionFailures)
	}
	states := coverageStates(scan.CoverageReport)
	for _, category := range []Category{CategorySAST, CategorySecrets, CategoryDependencies} {
		if states[category] != CoverageStateDisabled {
			t.Errorf("%s coverage = %s, want disabled", category, states[category])
		}
	}
}

func TestRunnerNoAdaptersConfigured(t *testing.T) {
	workspace := t.TempDir()
	runner, err := NewRunner(RunnerConfig{
		WorkspaceRoot: workspace, RunID: "none-configured",
		Policy: Policy{Enabled: true, Mode: "report-only", Scope: "workspace", FailOn: []string{"high"}, UnknownSeverity: "indeterminate"},
	})
	if err != nil {
		t.Fatal(err)
	}
	scan, err := runner.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if scan.Verdict != VerdictIndeterminate {
		t.Fatalf("verdict = %s, want INDETERMINATE", scan.Verdict)
	}
	if !reasonListContains(scan.Reasons, "no adapter is configured") {
		t.Fatalf("reasons = %v, want none-configured explanation", scan.Reasons)
	}
	if reasonListContains(scan.Reasons, "every adapter is disabled") {
		t.Fatalf("none-configured scan was reported as all-disabled: %v", scan.Reasons)
	}
	if len(scan.Adapters) != 0 {
		t.Fatalf("adapters = %+v, want none", scan.Adapters)
	}
}

func TestRunnerAllDisabledRequiredOutcome(t *testing.T) {
	workspace := t.TempDir()
	detection := Detection{Languages: []Language{LanguageGo, LanguageRust}}
	runner, err := NewRunner(RunnerConfig{
		WorkspaceRoot: workspace, RunID: "required-disabled",
		Policy: Policy{
			Enabled: true, Mode: "required", Scope: "workspace",
			RequiredCategories: []string{"dependencies"},
			FailOn:             []string{"high"}, UnknownSeverity: "indeterminate",
		},
		Disabled: []DisabledAdapter{
			disabledEvidence(AdapterIDOSVScanner, CategoryDependencies, []Language{LanguageGo, LanguageRust}, false, "adapter is disabled by configuration"),
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
	if scan.Verdict != VerdictIndeterminate {
		t.Fatalf("verdict = %s, want INDETERMINATE (%v)", scan.Verdict, scan.Reasons)
	}
	if scan.RequiredMode == nil || scan.RequiredMode.Ready {
		t.Fatalf("required readiness = %+v, want a blocked projection", scan.RequiredMode)
	}
	if !reasonListContains(scan.Reasons, "dependencies") {
		t.Fatalf("reasons = %v, want a required dependency reason", scan.Reasons)
	}
}

func TestRunnerGlobalDisableRequiredGatePreservesBoundary(t *testing.T) {
	workspace := t.TempDir()
	runner, err := NewRunner(RunnerConfig{
		WorkspaceRoot: workspace, RunID: "global-disabled-required",
		Policy: Policy{
			Enabled: false, Mode: "required", Scope: "workspace",
			RequiredCategories: []string{"sast"}, RequiredGateActive: true,
			FailOn: []string{"high"}, UnknownSeverity: "indeterminate",
		},
		Disabled: []DisabledAdapter{
			disabledEvidence(AdapterIDSemgrep, CategorySAST, []Language{LanguageGo}, false, "global security.enabled is false"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	scan, err := runner.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if scan.Verdict != VerdictIndeterminate {
		t.Fatalf("verdict = %s, want INDETERMINATE", scan.Verdict)
	}
	if !reasonListContains(scan.Reasons, "globally disabled") {
		t.Fatalf("reasons = %v, want the global-disable reason", scan.Reasons)
	}
	if len(scan.Adapters) != 0 {
		t.Fatalf("global disable recorded adapter evidence: %+v", scan.Adapters)
	}
}

func TestRunnerDoesNotExecuteDisabledAdapter(t *testing.T) {
	workspace := t.TempDir()
	enabled := &countingAdapter{id: "enabled-sast"}
	runner, err := NewRunner(RunnerConfig{
		WorkspaceRoot: workspace, RunID: "mixed-disabled",
		Adapters: []Adapter{enabled},
		Policy:   Policy{Enabled: true, Mode: "report-only", Scope: "workspace", FailOn: []string{"high"}, UnknownSeverity: "indeterminate"},
		Disabled: []DisabledAdapter{
			disabledEvidence("disabled-sast", CategorySAST, []Language{LanguageGo}, false, "adapter is disabled by configuration"),
		},
		Selection: map[string]AdapterSelection{
			"enabled-sast": {Mode: SelectionExplicit, Languages: []Language{LanguageGo}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	scan, err := runner.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if enabled.calls != 1 {
		t.Fatalf("enabled adapter calls = %d, want 1", enabled.calls)
	}
	seenDisabled := false
	for _, exec := range scan.Adapters {
		if exec.AdapterID == "disabled-sast" {
			seenDisabled = exec.Disabled && exec.Status == StatusSkipped
		}
	}
	if !seenDisabled {
		t.Fatalf("disabled adapter evidence missing: %+v", scan.Adapters)
	}
	if scan.Verdict != VerdictPass {
		t.Fatalf("verdict = %s, want PASS when enabled coverage is complete (%v)", scan.Verdict, scan.Reasons)
	}
}

func coverageStates(report []CategoryCoverage) map[Category]CoverageState {
	states := map[Category]CoverageState{}
	for _, category := range report {
		states[category.Category] = category.State
	}
	return states
}

func reasonListContains(values []string, want string) bool {
	for _, value := range values {
		if strings.Contains(value, want) {
			return true
		}
	}
	return false
}
