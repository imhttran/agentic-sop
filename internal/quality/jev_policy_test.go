package quality

import (
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/jev"
	"github.com/imhttran/agentic-sop/internal/review"
)

// JEV012 deterministic policy coverage: the severity policy, the report/blocking
// split, fail-closed handling, and the way JEV evidence folds into the gate. None
// of these require a model, the network, or timing.

func jevFinding(sev review.Severity, category, msg string) jev.Finding {
	return jev.Finding{Severity: sev, Category: category, Message: msg}
}

// TestJEVSeverityPriority pins the severity policy: HIGH/CRITICAL block under the
// default (quality.fail_on) list, INFO/LOW/MEDIUM never do, and an empty fail_on
// blocks nothing.
func TestJEVSeverityPriority(t *testing.T) {
	failOn := []string{"critical", "high"}
	cases := []struct {
		sev  string
		want JEVPriority
	}{
		{"CRITICAL", JEVBlocking},
		{"critical", JEVBlocking},
		{"HIGH", JEVBlocking},
		{"High", JEVBlocking},
		{" high ", JEVBlocking},
		{"MEDIUM", JEVReport},
		{"LOW", JEVReport},
		{"INFO", JEVReport},
		{"", JEVReport},
		{"UNKNOWN", JEVReport},
	}
	for _, tc := range cases {
		if got := JEVSeverityPriority(tc.sev, failOn); got != tc.want {
			t.Errorf("JEVSeverityPriority(%q) = %s, want %s", tc.sev, got, tc.want)
		}
	}

	// An empty fail_on list blocks nothing, even for the highest severities.
	for _, sev := range []string{"CRITICAL", "HIGH"} {
		if got := JEVSeverityPriority(sev, nil); got != JEVReport {
			t.Errorf("JEVSeverityPriority(%q, empty) = %s, want REPORT", sev, got)
		}
	}
}

// TestJEVReportFindingsSurfacesNonBlocking proves the report-only findings are
// surfaced (with their category and message) and that blocking findings, a nil
// evidence, and a fail-closed evidence are not reported here.
func TestJEVReportFindingsSurfacesNonBlocking(t *testing.T) {
	ev := jevEvidence(t, jev.Result{Status: jev.StatusFindings, Findings: []jev.Finding{
		jevFinding(review.Medium, "quality", "style nit"),
		jevFinding(review.High, "quality", "real bug"),
		jevFinding(review.Info, "", ""),
	}})

	reasons := JEVReportFindings([]string{"critical", "high"}, ev)
	joined := strings.Join(reasons, "\n")
	if !strings.Contains(joined, "style nit") {
		t.Errorf("reasons = %v, want the MEDIUM finding", reasons)
	}
	if !strings.Contains(joined, "quality: style nit") {
		t.Errorf("reasons = %v, want the category preserved", reasons)
	}
	if strings.Contains(joined, "real bug") {
		t.Errorf("reasons = %v, must not report the blocking HIGH finding", reasons)
	}
	if len(reasons) != 2 {
		t.Errorf("reasons = %v, want 2 report-only findings (MEDIUM and INFO)", reasons)
	}

	if got := JEVReportFindings(nil, nil); got != nil {
		t.Errorf("nil evidence = %v, want nil", got)
	}
	failClosed := &JEVEvidence{FailClosed: true, Reason: "x"}
	if got := JEVReportFindings([]string{"high"}, failClosed); got != nil {
		t.Errorf("fail-closed evidence = %v, want no report reasons", got)
	}
}

// TestEvaluateWithJEV pins how JEV evidence folds into the gate: a PASS adds
// nothing, blocking findings fail, report-only findings pass, and a fail-closed
// result never passes.
func TestEvaluateWithJEV(t *testing.T) {
	failOn := []string{"critical", "high"}
	passing := Input{BuildPassed: true, TestPassed: true}

	cases := []struct {
		name       string
		in         Input
		want       Decision
		wantReason string
	}{
		{
			name: "nil JEV leaves a pass unchanged",
			in:   withJEV(passing, nil),
			want: Pass,
		},
		{
			name: "JEV PASS adds nothing",
			in:   withJEV(passing, jevEvidence(t, jev.Result{Status: jev.StatusPass})),
			want: Pass,
		},
		{
			name: "JEV HIGH fails the gate",
			in: withJEV(passing, jevEvidence(t, jev.Result{Status: jev.StatusFindings, Findings: []jev.Finding{
				jevFinding(review.High, "quality", "bug"),
			}})),
			want:       Fail,
			wantReason: "JEV finding(s) at a failing severity",
		},
		{
			name: "JEV CRITICAL fails the gate",
			in: withJEV(passing, jevEvidence(t, jev.Result{Status: jev.StatusFindings, Findings: []jev.Finding{
				jevFinding(review.Critical, "quality", "critical bug"),
			}})),
			want:       Fail,
			wantReason: "JEV finding(s) at a failing severity",
		},
		{
			name: "JEV MEDIUM alone does not block",
			in: withJEV(passing, jevEvidence(t, jev.Result{Status: jev.StatusFindings, Findings: []jev.Finding{
				jevFinding(review.Medium, "quality", "nit"),
			}})),
			want:       Pass,
			wantReason: "JEV report (MEDIUM)",
		},
		{
			name: "JEV LOW alone does not block",
			in: withJEV(passing, jevEvidence(t, jev.Result{Status: jev.StatusFindings, Findings: []jev.Finding{
				jevFinding(review.Low, "quality", "nibble"),
			}})),
			want:       Pass,
			wantReason: "JEV report (LOW)",
		},
		{
			name: "JEV INFO alone does not block",
			in: withJEV(passing, jevEvidence(t, jev.Result{Status: jev.StatusFindings, Findings: []jev.Finding{
				jevFinding(review.Info, "quality", "fyi"),
			}})),
			want:       Pass,
			wantReason: "JEV report (INFO)",
		},
		{
			name: "fail-closed JEV never passes",
			in:   withJEV(passing, &JEVEvidence{FailClosed: true, Reason: "JEV analysis failed (fail closed): provider down"}),
			want: Fail,
		},
		{
			name: "blocking JEV with exhausted budget needs a human",
			in: withJEV(Input{BuildPassed: true, TestPassed: true, FixCycles: 3},
				jevEvidence(t, jev.Result{Status: jev.StatusFindings, Findings: []jev.Finding{
					jevFinding(review.High, "quality", "bug"),
				}})),
			want:       NeedsHuman,
			wantReason: "fix-loop limit",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Evaluate(policy(true, 3, failOn...), tc.in)
			if got.Decision != tc.want {
				t.Errorf("Decision = %s, want %s (reasons: %v)", got.Decision, tc.want, got.Reasons)
			}
			if tc.wantReason != "" && !strings.Contains(strings.Join(got.Reasons, "; "), tc.wantReason) {
				t.Errorf("reasons = %v, want one containing %q", got.Reasons, tc.wantReason)
			}
		})
	}
}

// TestNewJEVEvidenceValidation pins the fail-closed shaping: valid PASS/FINDINGS
// are accepted, while a malformed result and the INCOMPLETE/ERROR statuses are
// fail-closed and never a pass.
func TestNewJEVEvidenceValidation(t *testing.T) {
	cases := []struct {
		name       string
		res        jev.Result
		failClosed bool
	}{
		{"valid pass", jev.Result{Status: jev.StatusPass}, false},
		{"valid findings", jev.Result{Status: jev.StatusFindings, Findings: []jev.Finding{
			{Severity: review.High, Message: "x"},
		}}, false},
		{"unknown status is malformed", jev.Result{Status: "WHATEVER"}, true},
		{"pass with findings is malformed", jev.Result{Status: jev.StatusPass, Findings: []jev.Finding{
			{Severity: review.High},
		}}, true},
		{"findings without findings is malformed", jev.Result{Status: jev.StatusFindings}, true},
		{"invalid severity is malformed", jev.Result{Status: jev.StatusFindings, Findings: []jev.Finding{
			{Severity: "NOPE"},
		}}, true},
		{"incomplete is fail-closed", jev.Result{Status: jev.StatusIncomplete}, true},
		{"error is fail-closed", jev.Result{Status: jev.StatusError}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev := NewJEVEvidence(tc.res)
			if ev.FailClosed != tc.failClosed {
				t.Errorf("FailClosed = %v, want %v (reason: %q)", ev.FailClosed, tc.failClosed, ev.Reason)
			}
			if tc.failClosed && strings.TrimSpace(ev.Reason) == "" {
				t.Error("a fail-closed evidence must explain why")
			}
			// A fail-closed evidence must never read as a pass.
			if tc.failClosed && ev.Status == jev.StatusPass {
				t.Error("a fail-closed evidence must not carry PASS")
			}
		})
	}
}

// jevEvidence builds the gate-shaped evidence from a raw result, the same way the
// lifecycle does.
func jevEvidence(t *testing.T, res jev.Result) *JEVEvidence {
	t.Helper()
	if err := res.Validate(); err != nil {
		t.Fatalf("test fixture result is invalid: %v", err)
	}
	ev := NewJEVEvidence(res)
	return &ev
}

// withJEV returns in with its JEV evidence set.
func withJEV(in Input, ev *JEVEvidence) Input {
	in.JEV = ev
	return in
}
