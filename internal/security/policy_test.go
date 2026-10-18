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
	changedOSV := base
	changedOSV.OSVDatabasePath = "osv-a"
	changedGate := base
	changedGate.RequiredGateActive = true
	if base.Identity(Baseline{}) == changedPath.Identity(Baseline{}) {
		t.Fatal("advisory database path did not change policy identity")
	}
	if base.Identity(Baseline{}) == changedStale.Identity(Baseline{}) {
		t.Fatal("stale policy did not change policy identity")
	}
	if base.Identity(Baseline{}) == changedOSV.Identity(Baseline{}) {
		t.Fatal("OSV database path did not change policy identity")
	}
	if base.Identity(Baseline{}) == changedGate.Identity(Baseline{}) {
		t.Fatal("required gate state did not change policy identity")
	}
}

func TestRequiredModeEvaluatesDetectedLanguagesSeparately(t *testing.T) {
	detection := Detection{Languages: []Language{LanguageGo, LanguageRust}}
	policy := Policy{Enabled: true, Mode: "required", RequiredCategories: []string{"dependencies"}, FailOn: []string{"high"}, UnknownSeverity: "indeterminate"}
	goOnly := []Execution{{
		ProtocolVersion: ProtocolVersion, AdapterID: AdapterIDOSVScanner, Category: CategoryDependencies,
		Status: StatusCompleted, Coverage: CoverageComplete, Languages: []Language{LanguageGo},
	}}
	missing := EvaluatePolicyWithDetection(goOnly, policy, &detection)
	if missing.Verdict == VerdictPass {
		t.Fatalf("Go-only required coverage passed for mixed content: %v", missing.Reasons)
	}
	if !strings.Contains(strings.Join(missing.Reasons, " "), "COMPLETE rust coverage") {
		t.Fatalf("reasons = %v, want missing Rust coverage", missing.Reasons)
	}
	both := append(append([]Execution(nil), goOnly...), Execution{
		ProtocolVersion: ProtocolVersion, AdapterID: AdapterIDCargoAudit, Category: CategoryDependencies,
		Status: StatusCompleted, Coverage: CoverageComplete, Languages: []Language{LanguageRust},
	})
	if evaluation := EvaluatePolicyWithDetection(both, policy, &detection); evaluation.Verdict != VerdictPass {
		t.Fatalf("complete mixed coverage = %s, want PASS (%v)", evaluation.Verdict, evaluation.Reasons)
	}
}

func TestCargoAuditCannotSatisfyGoDependencyCoverage(t *testing.T) {
	detection := Detection{Languages: []Language{LanguageGo, LanguageRust}}
	policy := Policy{Enabled: true, Mode: "required", RequiredCategories: []string{"dependencies"}, FailOn: []string{"high"}, UnknownSeverity: "indeterminate"}
	execs := []Execution{{
		ProtocolVersion: ProtocolVersion, AdapterID: AdapterIDCargoAudit, Category: CategoryDependencies,
		Status: StatusCompleted, Coverage: CoverageComplete, Languages: []Language{LanguageRust},
	}}
	evaluation := EvaluatePolicyWithDetection(execs, policy, &detection)
	if evaluation.Verdict == VerdictPass {
		t.Fatalf("cargo-audit satisfied Go dependency coverage: %v", evaluation.Reasons)
	}
	if !strings.Contains(strings.Join(evaluation.Reasons, " "), "COMPLETE go coverage") {
		t.Fatalf("reasons = %v, want missing Go coverage", evaluation.Reasons)
	}
}

func TestOneSuccessfulAdapterCannotHideAFailedRequiredObligation(t *testing.T) {
	detection := Detection{Languages: []Language{LanguageGo, LanguageRust}}
	policy := Policy{Enabled: true, Mode: "required", RequiredCategories: []string{"dependencies"}, FailOn: []string{"high"}, UnknownSeverity: "indeterminate"}
	execs := []Execution{
		{ProtocolVersion: ProtocolVersion, AdapterID: AdapterIDOSVScanner, Category: CategoryDependencies, Status: StatusCompleted, Coverage: CoverageComplete, Languages: []Language{LanguageGo, LanguageRust}},
		{ProtocolVersion: ProtocolVersion, AdapterID: "private-deps", Category: CategoryDependencies, Status: StatusFailed, Coverage: CoverageUnknown, Languages: []Language{LanguageRust}},
	}
	evaluation := EvaluatePolicyWithDetection(execs, policy, &detection)
	if evaluation.Verdict != VerdictIndeterminate {
		t.Fatalf("verdict = %s, want INDETERMINATE (%v)", evaluation.Verdict, evaluation.Reasons)
	}
}

func TestGlobalDisableIsNotARequiredGateExemption(t *testing.T) {
	evaluation := EvaluatePolicyWithDetection(nil, Policy{
		Enabled: false, Mode: "required", RequiredCategories: []string{"sast"},
		RequiredGateActive: true, FailOn: []string{"high"}, UnknownSeverity: "indeterminate",
	}, nil)
	if evaluation.Verdict != VerdictIndeterminate {
		t.Fatalf("verdict = %s, want INDETERMINATE", evaluation.Verdict)
	}
}

func TestDisabledRequiredCategoryIsNotClean(t *testing.T) {
	detection := Detection{Languages: []Language{LanguageRust}}
	policy := Policy{Enabled: true, Mode: "required", RequiredCategories: []string{"dependencies"}, FailOn: []string{"high"}, UnknownSeverity: "indeterminate"}
	execs := []Execution{{
		ProtocolVersion: ProtocolVersion, AdapterID: AdapterIDCargoAudit, Category: CategoryDependencies,
		Status: StatusSkipped, Coverage: CoverageUnknown, Disabled: true, Languages: []Language{LanguageRust},
	}}
	evaluation := EvaluatePolicyWithDetection(execs, policy, &detection)
	if evaluation.Verdict == VerdictPass {
		t.Fatalf("disabled required category passed: %v", evaluation.Reasons)
	}
}
