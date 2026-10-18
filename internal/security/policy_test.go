package security

import (
	"strings"
	"testing"
)

func TestStaleDatabasePolicy(t *testing.T) {
	exec := Execution{
		ProtocolVersion: ProtocolVersion,
		AdapterID:       AdapterIDCargoAudit,
		Category:        CategoryDependencies,
		Status:          StatusCompleted,
		Coverage:        CoverageComplete,
		Provenance:      Provenance{AdvisoryDatabaseStale: true},
	}
	base := Policy{Enabled: true, Mode: "report-only", FailOn: []string{"high"}, UnknownSeverity: "indeterminate"}

	denied := EvaluatePolicy([]Execution{exec}, base)
	if denied.Verdict != VerdictIndeterminate {
		t.Fatalf("stale deny verdict = %s, want INDETERMINATE (%v)", denied.Verdict, denied.Reasons)
	}

	allowed := base
	allowed.StaleAdvisoryDatabase = StaleAllow
	allowedEval := EvaluatePolicy([]Execution{exec}, allowed)
	if allowedEval.Coverage != CoveragePartial {
		t.Fatalf("stale allow coverage = %s, want PARTIAL", allowedEval.Coverage)
	}
	if !strings.Contains(strings.Join(allowedEval.Reasons, " "), "stale") {
		t.Fatalf("stale allow reasons = %v, want stale diagnostic", allowedEval.Reasons)
	}
	if allowedEval.Verdict == VerdictPass {
		t.Fatalf("stale allow must not produce a PASS with partial coverage")
	}
}

func TestRequiredModeRejectsUnsupportedCoverage(t *testing.T) {
	exec := Execution{
		ProtocolVersion: ProtocolVersion,
		AdapterID:       AdapterIDSemgrep,
		Category:        CategorySAST,
		Status:          StatusFailed,
		Coverage:        CoverageUnknown,
	}
	evaluation := EvaluatePolicy([]Execution{exec}, Policy{
		Enabled: true, Mode: "required", RequiredCategories: []string{"sast"},
		FailOn: []string{"high"}, UnknownSeverity: "indeterminate",
	})
	if evaluation.Verdict != VerdictIndeterminate {
		t.Fatalf("verdict = %s, want INDETERMINATE", evaluation.Verdict)
	}
}

func TestPolicyIdentityIncludesAdvisoryDatabase(t *testing.T) {
	base := Policy{Enabled: true, Mode: "report-only", Scope: "workspace", FailOn: []string{"high"}, UnknownSeverity: "indeterminate", AdvisoryDatabasePath: "db-a", StaleAdvisoryDatabase: StaleDeny}
	changedPath := base
	changedPath.AdvisoryDatabasePath = "db-b"
	changedStale := base
	changedStale.StaleAdvisoryDatabase = StaleAllow
	if base.Identity(Baseline{}) == changedPath.Identity(Baseline{}) {
		t.Fatal("advisory database path did not change policy identity")
	}
	if base.Identity(Baseline{}) == changedStale.Identity(Baseline{}) {
		t.Fatal("stale policy did not change policy identity")
	}
}
