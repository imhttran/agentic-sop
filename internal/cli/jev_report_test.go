package cli

import (
	"strings"
	"testing"
)

// TestRenderJEVReportPass asserts the concise line a user reads when JEV ran and
// passed: 'JEV: PASS' and '0 blocking findings'. This is what lets a user tell
// whether JEV executed (JEV011 S2).
func TestRenderJEVReportPass(t *testing.T) {
	doc := &jevRunDoc{TaskID: "T011", RunID: "R1", Status: "PASS"}

	got := renderJEVReport(doc, []string{"HIGH", "CRITICAL"}, "")

	if !strings.Contains(got, "JEV: PASS") {
		t.Errorf("output = %q, want it to contain %q", got, "JEV: PASS")
	}
	if !strings.Contains(got, "0 blocking findings") {
		t.Errorf("output = %q, want it to contain %q", got, "0 blocking findings")
	}
}

// TestRenderJEVReportFindingsShowsBlockingWithLocation asserts blocking findings
// are visible with severity and source location, and the count line is accurate
// (JEV011 S3).
func TestRenderJEVReportFindingsShowsBlockingWithLocation(t *testing.T) {
	doc := &jevRunDoc{
		Status: "FINDINGS",
		Findings: []jevFindingDoc{
			{Severity: "HIGH", File: "internal/cli/run.go", Line: 88, Finding: "dead code"},
			{Severity: "CRITICAL", File: "internal/cli/jev.go", Line: 12, Finding: "unsafe mutation"},
		},
	}

	got := renderJEVReport(doc, []string{"HIGH", "CRITICAL"}, "")

	if !strings.Contains(got, "JEV: FINDINGS") {
		t.Errorf("output = %q, want it to contain %q", got, "JEV: FINDINGS")
	}
	if !strings.Contains(got, "2 blocking findings") {
		t.Errorf("output = %q, want it to contain %q", got, "2 blocking findings")
	}
	if !strings.Contains(got, "[HIGH] internal/cli/run.go:88 dead code") {
		t.Errorf("output = %q, want the HIGH finding with its location", got)
	}
	if !strings.Contains(got, "[CRITICAL] internal/cli/jev.go:12 unsafe mutation") {
		t.Errorf("output = %q, want the CRITICAL finding with its location", got)
	}
}

// TestRenderJEVReportFindingWithoutLocation asserts a blocking finding with no
// file/line renders without misleading location data and is still marked by its
// severity (JEV011 S3).
func TestRenderJEVReportFindingWithoutLocation(t *testing.T) {
	doc := &jevRunDoc{
		Status:   "FINDINGS",
		Findings: []jevFindingDoc{{Severity: "HIGH", Finding: "no location"}},
	}

	got := renderJEVReport(doc, []string{"HIGH"}, "")

	if !strings.Contains(got, "[HIGH] no location") {
		t.Errorf("output = %q, want a severity-marked finding without a location", got)
	}
	if strings.Contains(got, ":0") {
		t.Errorf("output = %q, want no fabricated line number", got)
	}
}

// TestRenderJEVReportNonBlockingAccessible asserts non-blocking findings are
// accessible and distinguishable from blocking findings, and do not imply quality
// failure (JEV011 S4).
func TestRenderJEVReportNonBlockingAccessible(t *testing.T) {
	doc := &jevRunDoc{
		Status: "FINDINGS",
		Findings: []jevFindingDoc{
			{Severity: "INFO", File: "a.go", Line: 1, Finding: "nit"},
			{Severity: "MEDIUM", File: "b.go", Line: 2, Finding: "style"},
		},
	}

	got := renderJEVReport(doc, []string{"HIGH", "CRITICAL"}, ".agent-sdlc/runs/T011/jev.json")

	if !strings.Contains(got, "0 blocking findings") {
		t.Errorf("output = %q, want 0 blocking findings", got)
	}
	if !strings.Contains(got, "2 non-blocking finding(s)") {
		t.Errorf("output = %q, want the non-blocking count line", got)
	}
	if !strings.Contains(got, "[INFO] a.go:1 nit") || !strings.Contains(got, "[MEDIUM] b.go:2 style") {
		t.Errorf("output = %q, want non-blocking findings with severity and location", got)
	}
}

// TestRenderJEVReportShowsReportPath asserts the persisted report path is shown
// only when one exists (JEV011 S5).
func TestRenderJEVReportShowsReportPath(t *testing.T) {
	doc := &jevRunDoc{Status: "PASS"}

	withPath := renderJEVReport(doc, []string{"HIGH"}, ".agent-sdlc/runs/T011/jev.json")
	if !strings.Contains(withPath, "JEV report: .agent-sdlc/runs/T011/jev.json") {
		t.Errorf("output = %q, want the report path", withPath)
	}

	withoutPath := renderJEVReport(doc, []string{"HIGH"}, "")
	if strings.Contains(withoutPath, "JEV report:") {
		t.Errorf("output = %q, want no report path when none exists", withoutPath)
	}
}

// TestRenderJEVReportFailClosedNeverPass asserts a fail-closed artifact is never
// rendered as PASS and that an unrecognized/empty status is reported fail-closed
// as ERROR (JEV011 S2, fail-closed reporting).
func TestRenderJEVReportFailClosedNeverPass(t *testing.T) {
	cases := []struct {
		name string
		doc  *jevRunDoc
	}{
		{"fail-closed PASS", &jevRunDoc{Status: "PASS", FailClosed: true, Reason: "malformed"}},
		{"incomplete", &jevRunDoc{Status: "INCOMPLETE", FailClosed: true}},
		{"error", &jevRunDoc{Status: "ERROR", FailClosed: true}},
		{"empty", &jevRunDoc{Status: ""}},
		{"unrecognized", &jevRunDoc{Status: "OK"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := renderJEVReport(tc.doc, []string{"HIGH"}, "")
			if strings.Contains(got, "JEV: PASS") {
				t.Errorf("output = %q, must not be rendered as PASS", got)
			}
		})
	}
}

// TestRenderJEVReportDisabledAddsNoNoise asserts a disabled/absent JEV (nil
// document) renders nothing, so the CLI output is unchanged (JEV011 S6).
func TestRenderJEVReportDisabledAddsNoNoise(t *testing.T) {
	if got := renderJEVReport(nil, []string{"HIGH"}, ".agent-sdlc/runs/T011/jev.json"); got != "" {
		t.Errorf("renderJEVReport(nil) = %q, want empty", got)
	}

	var b strings.Builder
	writeJEVReport(&b, nil, []string{"HIGH"}, ".agent-sdlc/runs/T011/jev.json")
	if b.String() != "" {
		t.Errorf("writeJEVReport(nil) wrote %q, want nothing", b.String())
	}
}

// TestRenderJEVReportDeterministic asserts the rendering depends only on the
// artifact and policy (JEV011 S2).
func TestRenderJEVReportDeterministic(t *testing.T) {
	doc := &jevRunDoc{
		Status: "FINDINGS",
		Findings: []jevFindingDoc{
			{Severity: "HIGH", File: "a.go", Line: 1, Finding: "one"},
			{Severity: "LOW", File: "b.go", Line: 2, Finding: "two"},
		},
	}

	first := renderJEVReport(doc, []string{"HIGH"}, "p")
	for i := 0; i < 5; i++ {
		if got := renderJEVReport(doc, []string{"HIGH"}, "p"); got != first {
			t.Fatalf("render %d = %q, want %q", i, got, first)
		}
	}
}

// TestRenderJEVReportBoundsFindings asserts a long finding list is bounded but
// never silently drops blocking findings: a remainder line points at the report
// (JEV011 S3).
func TestRenderJEVReportBoundsFindings(t *testing.T) {
	const total = jevMaxBlockingRendered + 3
	findings := make([]jevFindingDoc, 0, total)
	for i := 0; i < total; i++ {
		findings = append(findings, jevFindingDoc{Severity: "HIGH", File: "f.go", Line: i + 1, Finding: "boom"})
	}
	doc := &jevRunDoc{Status: "FINDINGS", Findings: findings}

	got := renderJEVReport(doc, []string{"HIGH"}, "")

	if !strings.Contains(got, "3 more blocking finding(s)") {
		t.Errorf("output = %q, want the remainder line", got)
	}
	if strings.Count(got, "[HIGH]") != jevMaxBlockingRendered {
		t.Errorf("rendered %d findings, want %d", strings.Count(got, "[HIGH]"), jevMaxBlockingRendered)
	}
}
