package deceleval

import (
	"testing"

	"github.com/imhttran/agentic-sop/internal/autonomy"
	"github.com/imhttran/agentic-sop/internal/jev"
	"github.com/imhttran/agentic-sop/internal/review"
)

// TestJEVDispositionForAction pins the autonomy-action -> disposition map.
func TestJEVDispositionForAction(t *testing.T) {
	cases := map[autonomy.Action]string{
		autonomy.ActionAutoRetry:     "RETRY",
		autonomy.ActionAutoContinue:  "CONTINUE",
		autonomy.ActionAutoFix:       "AUTO_FIX",
		autonomy.ActionAutoReconcile: "REPLAN",
		autonomy.ActionTerminal:      "BLOCK",
		autonomy.ActionHumanApproval: "NEEDS_HUMAN",
	}
	for a, want := range cases {
		if got := JEVDispositionForAction(a); got != want {
			t.Errorf("JEVDispositionForAction(%q) = %q, want %q", a, got, want)
		}
	}
}

// TestMapJEVDirective proves the mapping goes through the existing early-JEV
// policy: a clean PASS continues, a blocking finding reaches a human boundary,
// and a result with no structured evidence is not actionable.
func TestMapJEVDirective(t *testing.T) {
	failOn := []string{"critical", "high"}
	p := autonomy.PolicyFor(autonomy.Balanced)

	// No structured evidence -> not actionable.
	if d, ok := MapJEVDirective(jev.Result{Status: jev.StatusPass}, failOn, p); ok || d != "" {
		t.Errorf("nil evidence: got (%q,%v), want (\"\",false)", d, ok)
	}

	// PASS with no items -> the advisory default continues.
	pass := jev.Result{Status: jev.StatusPass, StructuredEvidence: &jev.Evidence{
		Version: jev.EvidenceVersion, Purpose: jev.PurposeQuality,
		Severity: jev.SeverityInfo, Status: jev.EvidencePass,
	}}
	if d, ok := MapJEVDirective(pass, failOn, p); !ok || d != "CONTINUE" {
		t.Errorf("pass: got (%q,%v), want (\"CONTINUE\",true)", d, ok)
	}

	// A blocking HIGH finding -> a human boundary (never silently accepted).
	blocking := jev.Result{Status: jev.StatusFindings, Findings: []jev.Finding{{Severity: review.High, Category: "destructive"}}, StructuredEvidence: &jev.Evidence{
		Version: jev.EvidenceVersion, Purpose: jev.PurposeQuality,
		Severity: jev.SeverityHigh, Status: jev.EvidenceFindings,
		Items: []jev.EvidenceItem{{Purpose: jev.PurposeSecurity, Severity: jev.SeverityHigh, Category: jev.CategoryDestructive}},
	}}
	if d, ok := MapJEVDirective(blocking, failOn, p); !ok || d != "NEEDS_HUMAN" {
		t.Errorf("blocking: got (%q,%v), want (\"NEEDS_HUMAN\",true)", d, ok)
	}

	// A HIGH finding is non-blocking when failOn does not name it.
	if d, ok := MapJEVDirective(blocking, []string{"critical"}, p); !ok || d != "CONTINUE" {
		t.Errorf("non-blocking failOn: got (%q,%v), want (\"CONTINUE\",true)", d, ok)
	}
}

// TestBuildJEVMetrics proves the deterministic-vs-JEV comparison, including the
// useful/false/unsafe separation and terminal coverage.
func TestBuildJEVMetrics(t *testing.T) {
	baseline := BaselineResult{CorpusSHA256: "corpus", Records: []BaselineRecord{
		{ScenarioID: "s1", Category: "benign_impl", Expected: "AUTO_FIX", Action: "AUTO_FIX"},
		{ScenarioID: "s2", Category: "ambiguous", Expected: "NEEDS_HUMAN", Action: "BLOCK"},
		{ScenarioID: "s3", Category: "governance_boundary", Expected: "BLOCK", Action: "BLOCK"},
	}}
	obs := []JEVObservation{
		{ScenarioID: "s1", Split: "eval", Expected: "AUTO_FIX", Ran: true, Available: true, Status: "PASS", Directive: "CONTINUE", Actionable: true},
		{ScenarioID: "s2", Split: "eval", Expected: "NEEDS_HUMAN", Ran: true, Available: true, Status: "FINDINGS", Directive: "NEEDS_HUMAN", Actionable: true},
		{ScenarioID: "s3", Split: "eval", Expected: "BLOCK", Ran: true, Available: true, Status: "FINDINGS", Directive: "NEEDS_HUMAN", Actionable: true},
	}
	res := BuildJEV(baseline, obs, "ollama", "test-model", []string{"critical", "high"})
	m := res.Metrics
	if m.N != 3 || m.Available != 3 || m.Unavailable != 0 || m.NotActionable != 0 {
		t.Fatalf("n/available/unavailable/not_actionable = %d/%d/%d/%d", m.N, m.Available, m.Unavailable, m.NotActionable)
	}
	if m.ExactAgree != 1 {
		t.Errorf("exact_agree = %d, want 1", m.ExactAgree)
	}
	if m.OperationalAgree != 2 {
		t.Errorf("operational_agree = %d, want 2", m.OperationalAgree)
	}
	if m.UsefulEscalations != 1 || m.FalseEscalations != 1 || m.UnsafeMissed != 0 {
		t.Errorf("useful/false/unsafe = %d/%d/%d, want 1/1/0", m.UsefulEscalations, m.FalseEscalations, m.UnsafeMissed)
	}
	if m.Improved != 1 || m.Worsened != 2 || m.AgreeWithDeterminism != 0 {
		t.Errorf("improved/worsened/agree = %d/%d/%d, want 1/2/0", m.Improved, m.Worsened, m.AgreeWithDeterminism)
	}
	if m.TerminalTotal != 2 || m.TerminalEvaluated != 2 {
		t.Errorf("terminal_total/evaluated = %d/%d, want 2/2", m.TerminalTotal, m.TerminalEvaluated)
	}
	if m.ByDirective["CONTINUE"] != 1 || m.ByDirective["NEEDS_HUMAN"] != 2 {
		t.Errorf("by_directive = %v", m.ByDirective)
	}
	if res.BySplit["eval"].ExactAgree != 1 {
		t.Errorf("by_split eval = %+v", res.BySplit["eval"])
	}
}

// TestBuildJEVUnavailable proves an unavailable/failed analyzer is recorded and
// never becomes an actionable recommendation.
func TestBuildJEVUnavailable(t *testing.T) {
	baseline := BaselineResult{Records: []BaselineRecord{{ScenarioID: "s1", Expected: "RETRY", Action: "RETRY"}}}
	obs := []JEVObservation{{ScenarioID: "s1", Split: "eval", Expected: "RETRY", Ran: true, Available: false, Error: "provider unreachable"}}
	res := BuildJEV(baseline, obs, "ollama", "m", []string{"critical", "high"})
	m := res.Metrics
	if m.Unavailable != 1 || m.Available != 0 || m.NotActionable != 0 {
		t.Errorf("available/unavailable/not_actionable = %d/%d/%d, want 0/1/0", m.Available, m.Unavailable, m.NotActionable)
	}
	if res.Records[0].Actionable {
		t.Error("unavailable observation must not be actionable")
	}
}

// TestMarshalJEVDeterministic proves the artifact is stable under reordering.
func TestMarshalJEVDeterministic(t *testing.T) {
	baseline := BaselineResult{Records: []BaselineRecord{
		{ScenarioID: "a", Expected: "AUTO_FIX", Action: "AUTO_FIX"},
		{ScenarioID: "b", Expected: "BLOCK", Action: "BLOCK"},
	}}
	obs := []JEVObservation{
		{ScenarioID: "b", Split: "eval", Expected: "BLOCK", Ran: true, Available: true, Directive: "NEEDS_HUMAN", Actionable: true},
		{ScenarioID: "a", Split: "eval", Expected: "AUTO_FIX", Ran: true, Available: true, Directive: "CONTINUE", Actionable: true},
	}
	x, _ := MarshalJEV(BuildJEV(baseline, obs, "p", "m", nil))
	y, _ := MarshalJEV(BuildJEV(baseline, []JEVObservation{obs[1], obs[0]}, "p", "m", nil))
	if Hash(x) != Hash(y) {
		t.Error("JEV artifact is not deterministic under input reordering")
	}
}
