package security

import (
	"strings"
	"testing"
)

// These constants mirror the real Semgrep CE summary shape the adapter consumes.
// They are the conformance baseline: if the scanner stops emitting this contract,
// the parser must treat coverage as UNKNOWN instead of COMPLETE.
const semgrepCoverageSummaryNoSkip = `┌──────────────┐
│ Scan Summary │
└──────────────┘
✅ Scan completed successfully.
 • Findings: 0 (0 blocking)
 • Rules run: 1
 • Targets scanned: 1
 • Parsed lines: ~100.0%
 • No ignore information available
Ran 1 rule on 1 file: 0 findings.
`

const semgrepCoverageSummaryDeclaredExclude = `┌──────────────┐
│ Scan Summary │
└──────────────┘
 • Targets scanned: 1
 • Scan skipped:
   ◦ Matching --exclude patterns: 2
Ran 1 rule on 1 file: 0 findings.
`

func TestSemgrepCoverageSummaryContract(t *testing.T) {
	cases := []struct {
		name        string
		stderr      string
		detected    int
		coverage    Coverage
		wantSummary bool
		wantScanned int
		wantGaps    int
		wantDeclare int
		diagnostic  string
	}{
		{
			name: "no skips", stderr: semgrepCoverageSummaryNoSkip, detected: 1,
			coverage: CoverageComplete, wantSummary: true, wantScanned: 1,
		},
		{
			name: "semgrepignore skip", stderr: "Scan Summary\n • Targets scanned: 1\n • Scan skipped:\n   ◦ Files matching .semgrepignore patterns: 581\nRan 1 rule on 1 file: 0 findings.\n",
			detected: 1, coverage: CoveragePartial, wantSummary: true, wantScanned: 1, wantGaps: 1,
			diagnostic: ".semgrepignore patterns: 581",
		},
		{
			name: "large file skip", stderr: "Scan Summary\n • Targets scanned: 1\n • Scan skipped:\n   ◦ Files larger than  files 1.0 MB: 6\nRan 1 rule on 1 file: 0 findings.\n",
			detected: 1, coverage: CoveragePartial, wantSummary: true, wantScanned: 1, wantGaps: 1,
			diagnostic: "Files larger than  files 1.0 MB: 6",
		},
		{
			name: "declared exclude only", stderr: semgrepCoverageSummaryDeclaredExclude, detected: 1,
			coverage: CoverageComplete, wantSummary: true, wantScanned: 1, wantDeclare: 1,
			diagnostic: "declared out-of-scope",
		},
		{
			name: "declared exclude plus in-scope skip", stderr: "Scan Summary\n • Targets scanned: 1\n • Scan skipped:\n   ◦ Matching --exclude patterns: 2\n   ◦ Files matching .semgrepignore patterns: 3\nRan 1 rule on 1 file: 0 findings.\n",
			detected: 1, coverage: CoveragePartial, wantSummary: true, wantScanned: 1, wantGaps: 1, wantDeclare: 1,
			diagnostic: ".semgrepignore patterns: 3",
		},
		{
			name: "unrecognized skip reason", stderr: "Scan Summary\n • Targets scanned: 1\n • Scan skipped:\n   ◦ Some new scanner limit: 3\nRan 1 rule on 1 file: 0 findings.\n",
			detected: 1, coverage: CoveragePartial, wantSummary: true, wantScanned: 1, wantGaps: 1,
			diagnostic: "Some new scanner limit: 3",
		},
		{
			name: "skip count unavailable", stderr: "Scan Summary\n • Targets scanned: 1\n • Scan skipped:\n   ◦ Scan was limited to files tracked by git\nRan 1 rule on 1 file: 0 findings.\n",
			detected: 1, coverage: CoveragePartial, wantSummary: true, wantScanned: 1, wantGaps: 1,
			diagnostic: "count unavailable",
		},
		{
			name: "skip block without summary header", stderr: "Scan skipped:\n   ◦ Files matching .semgrepignore patterns: 4\n",
			detected: 1, coverage: CoveragePartial, wantSummary: false, wantGaps: 1,
		},
		{
			name: "missing summary", stderr: "", detected: 1,
			coverage: CoverageUnknown, wantSummary: false,
			diagnostic: "INDETERMINATE",
		},
		{
			name: "unrecognized summary", stderr: "Scan Summary\n ✅ Scan completed successfully.\n",
			detected: 1, coverage: CoverageUnknown, wantSummary: false,
			diagnostic: "unrecognized",
		},
		{
			name: "zero targets with detected source", stderr: "Scan Summary\n • Targets scanned: 0\n • No ignore information available\nRan 0 rules on 0 files: 0 findings.\n",
			detected: 1, coverage: CoverageUnknown, wantSummary: true, wantScanned: 0,
			diagnostic: "no scanned targets",
		},
		{
			name: "zero targets without detected source", stderr: "Scan Summary\n • Targets scanned: 0\n • No ignore information available\nRan 0 rules on 0 files: 0 findings.\n",
			detected: 0, coverage: CoverageComplete, wantSummary: true, wantScanned: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			evidence := parseSemgrepCoverageSummary(tc.stderr)
			if evidence.SummaryKnown != tc.wantSummary {
				t.Errorf("SummaryKnown = %v, want %v", evidence.SummaryKnown, tc.wantSummary)
			}
			if evidence.Scanned != tc.wantScanned {
				t.Errorf("Scanned = %d, want %d", evidence.Scanned, tc.wantScanned)
			}
			if got := len(evidence.gaps()); got != tc.wantGaps {
				t.Errorf("gaps = %d (%v), want %d", got, evidence.gaps(), tc.wantGaps)
			}
			if got := len(evidence.declared()); got != tc.wantDeclare {
				t.Errorf("declared = %d (%v), want %d", got, evidence.declared(), tc.wantDeclare)
			}
			coverage, diagnostics := semgrepCoverageDecision(evidence, tc.detected)
			if coverage != tc.coverage {
				t.Errorf("coverage = %s, want %s (%v)", coverage, tc.coverage, diagnostics)
			}
			if tc.diagnostic != "" && !strings.Contains(strings.Join(diagnostics, " "), tc.diagnostic) {
				t.Errorf("diagnostics = %v, want %q", diagnostics, tc.diagnostic)
			}
		})
	}
}

// TestSemgrepCoverageContractFailsClosed proves parser uncertainty never falls
// back to COMPLETE: an unrecognized or absent summary is INDETERMINATE even when
// the scanner exited successfully with no findings.
func TestSemgrepCoverageContractFailsClosed(t *testing.T) {
	for name, stderr := range map[string]string{
		"empty":             "",
		"summary only":      "Scan Summary\n",
		"truncated summary": "Scan Summary\n • Targets scanned: 1\n • Scan skip",
		"renamed summary":   "Coverage Report\n • Targets scanned: 1\n • No ignore information available\n",
		"no targets line":   "Scan Summary\n ✅ Scan completed successfully.\n • Findings: 0\n",
	} {
		t.Run(name, func(t *testing.T) {
			evidence := parseSemgrepCoverageSummary(stderr)
			if evidence.SummaryKnown {
				t.Fatalf("SummaryKnown = true for %q", stderr)
			}
			coverage, diagnostics := semgrepCoverageDecision(evidence, 1)
			if coverage != CoverageUnknown {
				t.Fatalf("coverage = %s, want UNKNOWN (%v)", coverage, diagnostics)
			}
		})
	}
}

func TestSemgrepCoverageDeclaredExcludesAreNotGaps(t *testing.T) {
	evidence := parseSemgrepCoverageSummary(semgrepCoverageSummaryDeclaredExclude)
	coverage, diagnostics := semgrepCoverageDecision(evidence, 1)
	if coverage != CoverageComplete {
		t.Fatalf("coverage = %s, want COMPLETE for declared exclusions (%v)", coverage, diagnostics)
	}
	detail := strings.Join(diagnostics, " ")
	if !strings.Contains(detail, "declared out-of-scope") || !strings.Contains(detail, "Matching --exclude patterns: 2") {
		t.Fatalf("diagnostics = %v, want declared exclusion accounting", diagnostics)
	}
	if strings.Contains(detail, "PARTIAL") {
		t.Fatalf("diagnostics = %v, declared exclusions must not be reported as partial coverage", diagnostics)
	}
}
