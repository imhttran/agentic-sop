package security

import "testing"

func TestEvaluatePolicyIgnoresReportOnlyFindings(t *testing.T) {
	exec := Execution{
		AdapterID: "promptfoo",
		Category:  CategoryEvaluation,
		Status:    StatusCompleted,
		Coverage:  CoverageComplete,
		Findings: []Finding{{
			AdapterID:  "promptfoo",
			Category:   CategoryEvaluation,
			Severity:   SeverityCritical,
			ReportOnly: true,
		}},
	}
	evaluation := EvaluatePolicy([]Execution{exec}, Policy{Enabled: true, FailOn: []string{"critical"}, UnknownSeverity: "indeterminate"})
	if evaluation.Verdict != VerdictPass {
		t.Fatalf("verdict = %s, want PASS for report-only", evaluation.Verdict)
	}
	if len(evaluation.Findings) != 1 {
		t.Fatalf("findings = %+v, want the finding retained", evaluation.Findings)
	}
}

func TestEvaluatePolicyBlocksNonReportOnlyEvaluationFinding(t *testing.T) {
	exec := Execution{
		AdapterID: "promptfoo",
		Category:  CategoryEvaluation,
		Status:    StatusCompleted,
		Coverage:  CoverageComplete,
		Findings: []Finding{{
			AdapterID: "promptfoo",
			Category:  CategoryEvaluation,
			Severity:  SeverityCritical,
		}},
	}
	evaluation := EvaluatePolicy([]Execution{exec}, Policy{Enabled: true, FailOn: []string{"critical"}, UnknownSeverity: "indeterminate"})
	if evaluation.Verdict != VerdictFail {
		t.Fatalf("verdict = %s, want FAIL", evaluation.Verdict)
	}
}
