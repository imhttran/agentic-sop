package decisionquality

import (
	"fmt"
	"os"
	"strings"
)

// CoverageReport is the report-facing retrieval coverage.
type CoverageReport struct {
	Cases        int
	WithEvidence int
	Without      int
}

// ReportInput is everything a final report is rendered from.
type ReportInput struct {
	Title    string
	Corpus   Corpus
	Rules    ScoringRules
	A, B     ArmSummary
	Paired   PairedResult
	Gate     Gate
	Decision string
	// Coverage is the deterministic arm-B retrieval coverage measured after the run with
	// the corrected query. Nil means it was not measured and is reported as unavailable.
	Coverage *CoverageReport
	// OriginalCoverageNote describes the coverage of the run as executed. It is a note
	// rather than a number because the original run did not persist retrieval IDs.
	OriginalCoverageNote string
	// PromptEquality states whether byte/hash equality of the original arm prompts was
	// verified. Empty means "not assessed".
	PromptEquality string
	// CoverageGateFailure names the frozen coverage-gate failure, if any.
	CoverageGateFailure string
	// DecisionNote is an explicit qualifier printed with the decision.
	DecisionNote string
	// SafetyAvailable is false when no safety observation was recorded; the zero
	// counters are then reported as unavailable rather than as a passing result.
	SafetyAvailable bool
	// Insufficient is non-empty when the evidence cannot support a conclusion.
	Insufficient string
	// NotEvaluated lists arms that were not run (for example C and P).
	NotEvaluated []string
	RunA, RunB   RunRecord
}

// RenderReport renders the RDM-004 go/no-go report. It records measured values only;
// a missing measurement stays visibly missing.
func RenderReport(in ReportInput) string {
	var b strings.Builder
	title := strings.TrimSpace(in.Title)
	if title == "" {
		title = "RDM-004 — Decision-quality comparison"
	}
	fmt.Fprintf(&b, "# %s\n\n", title)
	fmt.Fprintf(&b, "**Corpus:** %s (%d cases)\n\n", in.Corpus.Description, len(in.Corpus.Cases))
	fmt.Fprintf(&b, "**Frozen rules:** version %d, %d repeats per case, primary threshold %.1f pp, p95 added latency <= %d ms, token ceiling %.1f%%\n\n",
		in.Rules.Version, in.Rules.RepeatsPerCase, in.Rules.PrimaryThresholdPP, in.Rules.MaxP95AddedLatencyMS, in.Rules.TokenCeilingPercent)

	counts := in.Corpus.Counts()
	fmt.Fprintf(&b, "## Corpus\n\n| Class | Cases |\n| --- | --- |\n")
	for _, class := range []string{ClassPlanning, ClassRecovery, ClassNegative} {
		fmt.Fprintf(&b, "| %s | %d |\n", class, counts[class])
	}

	fmt.Fprintf(&b, "\n## Matched conditions\n\n| Field | Arm A | Arm B |\n| --- | --- | --- |\n")
	row := func(name, a, bb string) { fmt.Fprintf(&b, "| %s | %s | %s |\n", name, a, bb) }
	row("model", in.RunA.Model, in.RunB.Model)
	row("model version", in.RunA.ModelVersion, in.RunB.ModelVersion)
	row("prompt hash", in.RunA.PromptHash, in.RunB.PromptHash)
	row("sampling", in.RunA.Sampling, in.RunB.Sampling)
	row("retrieval mode", in.RunA.RetrievalMode, in.RunB.RetrievalMode)
	row("evidence token budget", fmt.Sprint(in.RunA.EvidenceTokens), fmt.Sprint(in.RunB.EvidenceTokens))

	fmt.Fprintf(&b, "\n## Results\n\n| Metric | Arm A | Arm B |\n| --- | --- | --- |\n")
	rowf := func(name string, a, bb interface{}) { fmt.Fprintf(&b, "| %s | %v | %v |\n", name, a, bb) }
	rowf("independent cases", in.A.CaseCount, in.B.CaseCount)
	rowf("repeat records", in.A.RepeatCount, in.B.RepeatCount)
	rowf("classification accuracy (%)", fmt.Sprintf("%.2f", in.A.Primary), fmt.Sprintf("%.2f", in.B.Primary))
	rowf("estimated tokens", in.A.Tokens, in.B.Tokens)
	rowf("p50 latency (ms)", in.A.P50LatencyMS, in.B.P50LatencyMS)
	rowf("p95 latency (ms)", in.A.P95LatencyMS, in.B.P95LatencyMS)
	safety := func(name string, a, b int) {
		if !in.SafetyAvailable {
			rowf(name, "unavailable", "unavailable")
			return
		}
		rowf(name, a, b)
	}
	safety("approval bypasses", in.A.ApprovalBypasses, in.B.ApprovalBypasses)
	safety("cross-corpus disclosures", in.A.CrossCorpusDisclosures, in.B.CrossCorpusDisclosures)
	safety("required-evidence bypasses", in.A.RequiredEvidenceBypasses, in.B.RequiredEvidenceBypasses)
	rate := func(name string, a, b float64) {
		if !in.SafetyAvailable {
			rowf(name, "unavailable", "unavailable")
			return
		}
		rowf(name, fmt.Sprintf("%.3f", a), fmt.Sprintf("%.3f", b))
	}
	rate("unsupported-claim rate", in.A.UnsupportedClaimRate, in.B.UnsupportedClaimRate)
	rate("unnecessary-escalation rate", in.A.UnnecessaryEscalationRate, in.B.UnnecessaryEscalationRate)
	falseEsc := func(s ArmSummary) any {
		if s.FalseEscalationRate < 0 {
			return "unavailable"
		}
		return fmt.Sprintf("%.3f (%d)", s.FalseEscalationRate, s.FalseEscalations)
	}
	rowf("false-escalation rate", falseEsc(in.A), falseEsc(in.B))

	fmt.Fprintf(&b, "\n## Retrieval coverage\n\n")
	fmt.Fprintf(&b, "| Run | Cases | With evidence | Without evidence |\n| --- | --- | --- | --- |\n")
	note := strings.TrimSpace(in.OriginalCoverageNote)
	if note == "" {
		note = "not recorded"
	}
	fmt.Fprintf(&b, "| Original measured run (as executed) | %d | %s | %s |\n", len(in.Corpus.Cases), note, "\u2014")
	if in.Coverage == nil {
		fmt.Fprintf(&b, "| Post-run audit (corrected query) | - | unavailable | unavailable |\n")
	} else {
		fmt.Fprintf(&b, "| Post-run audit (corrected query) | %d | %d | %d |\n", in.Coverage.Cases, in.Coverage.WithEvidence, in.Coverage.Without)
	}
	fmt.Fprintf(&b, "\nThe post-run audit is deterministic and makes no model call; it is not a re-run of the original experiment.\n")
	if in.PromptEquality != "" {
		fmt.Fprintf(&b, "\n**Prompt equality:** %s\n", in.PromptEquality)
	}

	fmt.Fprintf(&b, "\n## Paired comparison\n\n")
	fmt.Fprintf(&b, "- Paired cases: %d (repeats are clustered within a case and are never treated as independent)\n", in.Paired.CaseCount)
	fmt.Fprintf(&b, "- Mean difference: %.2f pp (B - A)\n", in.Paired.MeanDiffPP)
	fmt.Fprintf(&b, "- Per-case spread: %.2f pp\n", in.Paired.VariabilityPP)
	fmt.Fprintf(&b, "- 95%% bootstrap interval (case-clustered): [%.2f, %.2f] pp\n", in.Paired.LowerPP, in.Paired.UpperPP)

	fmt.Fprintf(&b, "\n## Frozen gate\n\n| Check | Value | Pass |\n| --- | --- | --- |\n")
	gateRow := func(name string, v interface{}, ok bool) { fmt.Fprintf(&b, "| %s | %v | %v |\n", name, v, ok) }
	gateRow("classification accuracy gain >= threshold", fmt.Sprintf("%.2f pp (>= %.2f)", in.Paired.MeanDiffPP, in.Gate.ThresholdPP), in.Paired.MeanDiffPP >= in.Gate.ThresholdPP)
	if in.SafetyAvailable {
		gateRow("zero safety violations", in.B.ApprovalBypasses+in.B.CrossCorpusDisclosures+in.B.RequiredEvidenceBypasses, in.Gate.ZeroSafetyViolations)
	} else {
		fmt.Fprintf(&b, "| zero safety violations | unavailable (not observed) | unavailable |\n")
	}
	if in.SafetyAvailable {
		gateRow("secondary not worse", "-", in.Gate.SecondaryNotWorse)
	} else {
		fmt.Fprintf(&b, "| secondary not worse | unavailable (not observed) | unavailable |\n")
	}
	gateRow("p95 added latency", fmt.Sprintf("%d ms (<= %d)", in.B.P95LatencyMS-in.A.P95LatencyMS, in.Gate.MaxP95AddedMS), in.Gate.LatencyOK)
	gateRow("token ceiling", fmt.Sprintf("%d tokens (<= %.0f%%)", in.B.Tokens, in.Gate.TokenCeiling), in.Gate.TokensOK)

	classCounts := in.Corpus.Counts()
	_ = classCounts
	cases := len(in.Corpus.Cases)
	labels := map[string]int{}
	for _, cs := range in.Corpus.Cases {
		if d := strings.TrimSpace(cs.Expected.Decision); d != "" {
			labels[d]++
		}
	}
	minLabel, minN := "", 0
	for name, n := range labels {
		if minLabel == "" || n < minN {
			minLabel, minN = name, n
		}
	}
	fmt.Fprintf(&b, "\n## Coverage gate (pilot, disposition labels)\n\n| Check | Value | Pass |\n| --- | --- | --- |\n")
	gateRow("held-out cases >= 20", cases, cases >= 20)
	gateRow("distinct disposition labels >= 2", len(labels), len(labels) >= 2)
	gateRow(fmt.Sprintf("minimum cases per label >= 3 (lowest: %s)", minLabel), minN, minN >= 3)
	if in.CoverageGateFailure != "" {
		fmt.Fprintf(&b, "\n**Coverage gate failed:** %s. The sample was not enlarged; no passing result is claimed.\n", in.CoverageGateFailure)
	}

	fmt.Fprintf(&b, "\n## Decision\n\n")
	if in.Insufficient != "" {
		fmt.Fprintf(&b, "**Insufficient evidence:** %s\n\nNo go/no-go conclusion is drawn.\n\n", in.Insufficient)
	} else {
		fmt.Fprintf(&b, "Frozen decision: `%s` (gate pass: %v).\n\n", in.Decision, in.Gate.Pass)
	}
	if in.DecisionNote != "" {
		fmt.Fprintf(&b, "%s\n\n", in.DecisionNote)
	}
	if len(in.NotEvaluated) > 0 {
		fmt.Fprintf(&b, "Not evaluated: %s. Unimplemented or unavailable arms are never imputed.\n\n", strings.Join(in.NotEvaluated, ", "))
	}
	fmt.Fprintf(&b, "A small pilot supports a limited trial, not a general reliability claim.\n")
	fmt.Fprintf(&b, "Defaults remain disabled unless an explicit rollout decision authorizes a change.\n")
	return b.String()
}

// WriteReport writes a rendered report atomically.
func WriteReport(path string, in ReportInput) error {
	data := []byte(RenderReport(in))
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
