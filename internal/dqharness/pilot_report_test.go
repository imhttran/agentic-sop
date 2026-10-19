package dqharness

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/decisionquality"
)

// TestPilotMeasuredReportVerifies is the deterministic proof that the planning-only pilot
// actually ran: it reloads the frozen corpus and raw run artifacts, re-verifies coverage
// and matched conditions, recomputes every metric, and requires REPORT.md to match.
func TestPilotMeasuredReportVerifies(t *testing.T) {
	dir := "../../evals/decision-memory-pilot"
	corpus, err := decisionquality.LoadCorpus(filepath.Join(dir, "corpus-heldout.json"))
	if err != nil {
		t.Fatalf("held-out corpus: %v", err)
	}
	if err := decisionquality.ValidatePilot(*corpus); err != nil {
		t.Fatalf("pilot contract: %v", err)
	}
	a, err := decisionquality.ReadRun(filepath.Join(dir, "run", "run-A.json"))
	if err != nil {
		t.Fatalf("arm A artifact: %v", err)
	}
	b, err := decisionquality.ReadRun(filepath.Join(dir, "run", "run-B.json"))
	if err != nil {
		t.Fatalf("arm B artifact: %v", err)
	}
	rules := decisionquality.FrozenRules()
	if err := VerifyRun(*corpus, rules, a, b); err != nil {
		t.Fatalf("run verification: %v", err)
	}
	sa, sb := decisionquality.Summarize(a), decisionquality.Summarize(b)
	paired := decisionquality.PairedRun(a, b, 20261019)
	gate := decisionquality.EvaluateGate(rules, sa, sb, paired)
	insufficient := decisionquality.InsufficientEvidence(*corpus, a, b)
	cov, err := MeasureCoverage(*corpus, KeywordRetriever{})
	if err != nil {
		t.Fatalf("coverage: %v", err)
	}
	in := decisionquality.ReportInput{
		Title: "Decision-memory pilot — planning-only", Corpus: *corpus, Rules: rules,
		A: sa, B: sb, Paired: paired, Gate: gate,
		Decision:     decisionquality.PilotDecision(gate, insufficient),
		Insufficient: insufficient, NotEvaluated: []string{"C", "P"},
		Coverage:             &decisionquality.CoverageReport{Cases: cov.Cases, WithEvidence: cov.WithEvidence, Without: cov.Without},
		SafetyAvailable:      false,
		OriginalCoverageNote: fmt.Sprintf("0 of %d cases (audit-reconstructed: the legacy AND query matched no evidence and both arms had identical estimated token totals)", len(corpus.Cases)),
		PromptEquality:       "unverified: prompts were not persisted; equal estimated token totals are consistent with, but do not prove, identical prompts",
		CoverageGateFailure:  decisionquality.InsufficientCoverage(*corpus),
		DecisionNote:         "NO-GO for rollout; retrieval benefit was not evaluated.",
		RunA:                 a, RunB: b,
	}
	onDisk, err := os.ReadFile(filepath.Join(dir, "run", "REPORT.md"))
	if err != nil {
		t.Fatalf("no measured report: %v", err)
	}
	if strings.TrimSpace(string(onDisk)) != strings.TrimSpace(decisionquality.RenderReport(in)) {
		t.Fatal("REPORT.md does not match the recomputed metrics from the raw run artifacts")
	}
	if sa.CaseCount != len(corpus.Cases) || sb.CaseCount != len(corpus.Cases) {
		t.Fatalf("coverage A=%d B=%d, want %d", sa.CaseCount, sb.CaseCount, len(corpus.Cases))
	}
}
