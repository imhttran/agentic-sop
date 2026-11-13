package security

import (
	"strings"
	"testing"
	"time"
)

func driftBaseline(fingerprints ...string) Baseline {
	entries := make([]BaselineEntry, 0, len(fingerprints))
	for _, fp := range fingerprints {
		entries = append(entries, BaselineEntry{
			Fingerprint: fp,
			RuleID:      "dangerous-exec-command",
			Path:        "internal/example.go",
			Reason:      "reviewed",
			Owner:       "team",
			ExpiresAt:   time.Date(2027, 6, 30, 0, 0, 0, 0, time.UTC),
		})
	}
	return Baseline{Version: 1, Entries: entries}
}

func completeExec(id string) Execution {
	return Execution{AdapterID: id, Status: StatusCompleted, Coverage: CoverageComplete}
}

func driftNow() time.Time { return time.Date(2026, 11, 13, 0, 0, 0, 0, time.UTC) }

// A successful full-workspace scan that observed every relevant adapter still
// reports entries that matched nothing.
func TestBaselineDriftDiagnosticsCompleteWorkspaceReportsDrift(t *testing.T) {
	b := driftBaseline("semgrep:drifted")
	scan := Scan{
		Scope:    Scope{Mode: workspaceScopeMode},
		Adapters: []Execution{completeExec(AdapterIDSemgrep)},
	}
	got := BaselineDriftDiagnostics(b, scan, driftNow())
	if len(got) != 1 || !strings.Contains(got[0], "1 reviewed entry") {
		t.Fatalf("diagnostics = %v, want one drift warning", got)
	}
}

// A changed-files scan with no findings must stay silent: every entry is absent
// because unchanged files were not scanned, not because the entry drifted.
func TestBaselineDriftDiagnosticsChangedFilesIsSilent(t *testing.T) {
	b := driftBaseline("semgrep:one", "semgrep:two")
	scan := Scan{
		Scope:    Scope{Mode: "changed-files"},
		Adapters: []Execution{completeExec(AdapterIDSemgrep)},
	}
	if got := BaselineDriftDiagnostics(b, scan, driftNow()); got != nil {
		t.Fatalf("diagnostics = %v, want none for a changed-files scan", got)
	}
}

func TestBaselineDriftDiagnosticsIncompleteCoverageIsSilent(t *testing.T) {
	b := driftBaseline("semgrep:drifted")
	for _, coverage := range []Coverage{CoveragePartial, CoverageUnknown} {
		scan := Scan{
			Scope: Scope{Mode: workspaceScopeMode},
			Adapters: []Execution{{
				AdapterID: AdapterIDSemgrep, Status: StatusCompleted, Coverage: coverage,
			}},
		}
		if got := BaselineDriftDiagnostics(b, scan, driftNow()); got != nil {
			t.Fatalf("coverage %s: diagnostics = %v, want none", coverage, got)
		}
	}
}

func TestBaselineDriftDiagnosticsFailedAdapterIsSilent(t *testing.T) {
	b := driftBaseline("semgrep:drifted")
	scan := Scan{
		Scope: Scope{Mode: workspaceScopeMode},
		Adapters: []Execution{{
			AdapterID: AdapterIDSemgrep, Status: StatusFailed, Coverage: CoverageUnknown,
		}},
	}
	if got := BaselineDriftDiagnostics(b, scan, driftNow()); got != nil {
		t.Fatalf("diagnostics = %v, want none when the relevant adapter failed", got)
	}
}

func TestBaselineDriftDiagnosticsDisabledOrNoOpAdapterIsSilent(t *testing.T) {
	b := driftBaseline("semgrep:drifted")
	cases := map[string]Execution{
		"disabled": {AdapterID: AdapterIDSemgrep, Status: StatusCompleted, Coverage: CoverageComplete, Disabled: true},
		"no applicable content": {
			AdapterID: AdapterIDSemgrep, Status: StatusCompleted,
			Coverage: CoverageComplete, NoApplicableContent: true,
		},
		"absent": {},
	}
	for name, exec := range cases {
		adapters := []Execution{}
		if exec.AdapterID != "" {
			adapters = append(adapters, exec)
		}
		scan := Scan{Scope: Scope{Mode: workspaceScopeMode}, Adapters: adapters}
		if got := BaselineDriftDiagnostics(b, scan, driftNow()); got != nil {
			t.Fatalf("%s: diagnostics = %v, want none", name, got)
		}
	}
}

// Only adapters the baseline actually relies on gate the warning. A failure in an
// adapter with no reviewed entries must not hide drift in another adapter.
func TestBaselineDriftDiagnosticsIgnoresIrrelevantAdapters(t *testing.T) {
	b := driftBaseline("semgrep:drifted")
	scan := Scan{
		Scope: Scope{Mode: workspaceScopeMode},
		Adapters: []Execution{
			completeExec(AdapterIDSemgrep),
			{AdapterID: AdapterIDGitleaks, Status: StatusFailed, Coverage: CoverageUnknown},
		},
	}
	got := BaselineDriftDiagnostics(b, scan, driftNow())
	if len(got) != 1 {
		t.Fatalf("diagnostics = %v, want the semgrep drift warning despite an unrelated failure", got)
	}

	// The reverse case must stay silent: the baseline now also relies on the
	// adapter that failed.
	b = driftBaseline("semgrep:drifted", "gitleaks:drifted")
	if got := BaselineDriftDiagnostics(b, scan, driftNow()); got != nil {
		t.Fatalf("diagnostics = %v, want none when a relevant adapter failed", got)
	}
}

func TestBaselineDriftDiagnosticsEmptyBaselineIsSilent(t *testing.T) {
	scan := Scan{
		Scope:    Scope{Mode: workspaceScopeMode},
		Adapters: []Execution{completeExec(AdapterIDSemgrep)},
	}
	if got := BaselineDriftDiagnostics(Baseline{Version: 1}, scan, driftNow()); got != nil {
		t.Fatalf("diagnostics = %v, want none for an empty baseline", got)
	}
}

// The gate is advisory plumbing only: it must not suppress a finding or alter the
// verdict it was computed alongside.
func TestBaselineDriftDiagnosticsDoesNotChangeFindingsOrVerdict(t *testing.T) {
	b := driftBaseline("semgrep:drifted")
	execs := []Execution{{
		AdapterID: AdapterIDSemgrep, Status: StatusCompleted, Coverage: CoverageComplete,
		Findings: []Finding{{
			Fingerprint: "semgrep:live", RuleID: "dangerous-exec-command",
			Severity: SeverityHigh, Path: "internal/example.go",
		}},
	}}
	policy := Policy{Enabled: true, Mode: "report-only", Scope: workspaceScopeMode,
		FailOn: []string{"critical", "high"}, UnknownSeverity: "indeterminate"}
	before := EvaluatePolicyWithDetection(execs, policy, nil)

	scan := Scan{Scope: Scope{Mode: workspaceScopeMode}, Adapters: execs, Findings: execs[0].Findings}
	if got := BaselineDriftDiagnostics(b, scan, driftNow()); len(got) != 1 {
		t.Fatalf("diagnostics = %v, want one drift warning", got)
	}
	after := EvaluatePolicyWithDetection(execs, policy, nil)
	if before.Verdict != after.Verdict || len(after.Findings) != 1 || after.Findings[0].Suppressed {
		t.Fatalf("verdict or findings changed: before=%+v after=%+v", before, after)
	}
}
