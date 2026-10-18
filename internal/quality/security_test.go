package quality

import (
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
)

func passingInput() Input {
	return Input{BuildPassed: true, TestPassed: true}
}

func qualityPolicy() config.Quality {
	return config.Default().Quality
}

func TestSecurityNilEvidenceDoesNotChangeGate(t *testing.T) {
	got := Evaluate(qualityPolicy(), passingInput())
	if got.Decision != Pass {
		t.Errorf("decision = %s, want PASS", got.Decision)
	}
}

func TestSecurityReportOnlyNeverBlocks(t *testing.T) {
	for _, verdict := range []string{"PASS", "FAIL", "INDETERMINATE", "NOT_APPLICABLE"} {
		in := passingInput()
		in.Security = &SecurityEvidence{Enabled: true, Required: false, Configured: true, Verdict: verdict}
		got := Evaluate(qualityPolicy(), in)
		if got.Decision != Pass {
			t.Errorf("report-only %s blocked with %s: %v", verdict, got.Decision, got.Reasons)
		}
		if !strings.Contains(strings.Join(got.Advisories, " "), "security (report-only): "+verdict) {
			t.Errorf("report-only %s advisory = %v", verdict, got.Advisories)
		}
	}
}

func TestSecurityRequiredBlocksFailAndIndeterminate(t *testing.T) {
	for _, verdict := range []string{"FAIL", "INDETERMINATE", "NOT_APPLICABLE", ""} {
		in := passingInput()
		in.Security = &SecurityEvidence{Enabled: true, Required: true, Configured: true, Verdict: verdict}
		got := Evaluate(qualityPolicy(), in)
		if got.Decision != Fail {
			t.Errorf("required %q decision = %s, want FAIL", verdict, got.Decision)
		}
	}
}

func TestSecurityRequiredPassesOnlyOnPass(t *testing.T) {
	in := passingInput()
	in.Security = &SecurityEvidence{Enabled: true, Required: true, Configured: true, Verdict: "PASS"}
	got := Evaluate(qualityPolicy(), in)
	if got.Decision != Pass {
		t.Errorf("decision = %s, want PASS: %v", got.Decision, got.Reasons)
	}
	if !strings.Contains(strings.Join(got.Reasons, " "), "required security scan passed") {
		t.Errorf("reasons = %v", got.Reasons)
	}
}

func TestSecurityRequiredUnconfiguredFailsClosed(t *testing.T) {
	in := passingInput()
	in.Security = &SecurityEvidence{Enabled: true, Required: true, Configured: false, Verdict: "PASS"}
	got := Evaluate(qualityPolicy(), in)
	if got.Decision != Fail || !strings.Contains(strings.Join(got.Reasons, " "), "not configured") {
		t.Errorf("decision/reasons = %s/%v, want FAIL with configuration reason", got.Decision, got.Reasons)
	}
}

func TestSecurityRequiredRespectsFixBudget(t *testing.T) {
	policy := qualityPolicy()
	in := passingInput()
	in.FixCycles = policy.MaxFixCycles
	in.Security = &SecurityEvidence{Enabled: true, Required: true, Configured: true, Verdict: "FAIL"}
	got := Evaluate(policy, in)
	if got.Decision != NeedsHuman {
		t.Errorf("decision = %s, want NEEDS_HUMAN at the fix-loop limit", got.Decision)
	}
}

func TestHumanApprovalStillOutranksSecurityFailure(t *testing.T) {
	in := passingInput()
	in.HumanRequired = true
	in.Security = &SecurityEvidence{Enabled: true, Required: true, Configured: true, Verdict: "FAIL"}
	got := Evaluate(qualityPolicy(), in)
	if got.Decision != NeedsHuman {
		t.Fatalf("decision = %s, want NEEDS_HUMAN", got.Decision)
	}
	if len(got.Reasons) != 1 || got.Reasons[0] != "human approval required" {
		t.Errorf("reasons = %v, want only the existing human-approval boundary", got.Reasons)
	}
}
