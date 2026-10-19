// Command rdm004-eval runs the frozen RDM-004 A/B comparison.
//
// It is the executable evaluation entry point: it verifies the frozen inputs, enforces
// explicit call/retry/time/token limits, checkpoints every completed decision, resumes
// without repeating one, verifies the run deterministically, and writes the measured
// report.
//
// A real run requires explicit operator authorization (--authorize) because it spends
// external model calls. Without it the command prints the plan and estimate only.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/decisionquality"
	"github.com/imhttran/agentic-sop/internal/dqharness"
	"github.com/imhttran/agentic-sop/internal/ollamaagent"
)

const promptTemplate = "rdm004-armblind-v1"

func main() {
	corpusPath := flag.String("corpus", "evals/decision-quality/corpus-heldout.json", "frozen held-out corpus")
	root := flag.String("root", "evals/decision-quality", "frozen input directory")
	profile := flag.String("profile", "rdm004", "corpus contract: rdm004 or pilot")
	outDir := flag.String("out", "evals/decision-quality/run", "run artifact directory")
	model := flag.String("model", "", "model name (required for a real run)")
	baseURL := flag.String("base-url", "http://localhost:11434", "provider base URL")
	timeout := flag.Duration("timeout", 30*time.Second, "per-decision timeout")
	maxDecisions := flag.Int("max-decisions", 180, "scheduled decisions allowed")
	maxRequests := flag.Int("max-requests", 360, "actual provider requests allowed, including retries")
	maxRetries := flag.Int("max-retries", 180, "retries allowed (each retry also consumes a request)")
	maxElapsed := flag.Duration("max-elapsed", 90*time.Minute, "wall-clock limit")
	maxTokens := flag.Int("max-tokens", 400000, "prompt+output token limit")
	authorize := flag.Bool("authorize", false, "authorize spending the model calls")
	verifyOnly := flag.Bool("verify-only", false, "verify an existing measured run without calling a model")
	auditRetrieval := flag.Bool("audit-retrieval", false, "record deterministic retrieval coverage and prompt inclusion without calling a model")
	renderOnly := flag.Bool("render-only", false, "re-render REPORT.md from existing run artifacts without calling a model")
	flag.Parse()

	corpus, err := decisionquality.LoadCorpus(*corpusPath)
	if err != nil {
		fail(err)
	}
	if *profile == "pilot" {
		if err := decisionquality.ValidatePilot(*corpus); err != nil {
			fail(err)
		}
	} else if err := corpus.Validate(); err != nil {
		fail(err)
	}
	if err := dqharness.VerifyFrozen(*root); err != nil {
		fail(fmt.Errorf("frozen inputs do not match their sidecars: %w", err))
	}
	blocker := dqharness.ReplayBlocker(*corpus)

	rules := decisionquality.FrozenRules()
	limits := dqharness.Limits{MaxDecisions: *maxDecisions, MaxRequests: *maxRequests, MaxRetries: *maxRetries, MaxElapsed: *maxElapsed, MaxTokens: *maxTokens}
	required := len(corpus.Cases) * rules.RepeatsPerCase * 2

	fmt.Printf("RDM-004 evaluation\n")
	fmt.Printf("  corpus:      %s (%d cases)\n", *corpusPath, len(corpus.Cases))
	fmt.Printf("  arms:        A (no retrieval), B (local SQLite FTS5)\n")
	fmt.Printf("  repeats:     %d per case per arm\n", rules.RepeatsPerCase)
	fmt.Printf("  decisions:   %d scheduled, limit %d\n", required, limits.MaxDecisions)
	fmt.Printf("  requests:    limit %d (every provider call, including retries)\n", limits.MaxRequests)
	fmt.Printf("  retries:     limit %d (also counted as requests)\n", limits.MaxRetries)
	fmt.Printf("  elapsed:     limit %s\n", limits.MaxElapsed)
	fmt.Printf("  tokens:      limit %d\n", limits.MaxTokens)
	fmt.Printf("  checkpoint:  %s\n", filepath.Join(*outDir, "checkpoint.jsonl"))
	if blocker != "" {
		fmt.Printf("  replay:      BLOCKED - %s\n", blocker)
	}
	if *model == "" {
		fmt.Printf("  model:       NOT SET (required for a real run)\n")
	}

	if *verifyOnly {
		verifyArtifacts(*outDir, *corpus, rules, *profile == "pilot")
		return
	}
	if *auditRetrieval {
		auditRetrievalCoverage(*outDir, *corpus)
		return
	}
	if *renderOnly {
		renderArtifacts(*outDir, *corpus, rules, *profile == "pilot")
		return
	}

	if !*authorize {
		fmt.Printf("\nNo model calls were made. A real run requires explicit authorization:\n")
		fmt.Printf("  rdm004-eval --authorize --model <model> --corpus %s --out %s\n", *corpusPath, *outDir)
		fmt.Printf("Estimated model time: %d decisions at 5-15s each = %d-%d minutes, excluding replay.\n",
			required, required*5/60, required*15/60)
		fmt.Printf("Cost: unavailable (no provider pricing is configured in this repository).\n")
		return
	}
	if *model == "" {
		fail(fmt.Errorf("--model is required with --authorize"))
	}
	if blocker != "" {
		fail(fmt.Errorf("refusing to run a reduced experiment: %s", blocker))
	}

	binding, err := buildBinding(*root, *corpusPath, *model)
	if err != nil {
		fail(err)
	}

	chat, err := ollamaagent.NewChatProvider(ollamaagent.Config{
		BaseURL: *baseURL,
		Model:   *model,
		Timeout: *timeout,
	})
	if err != nil {
		fail(err)
	}
	decisions := dqharness.DecisionSet(*corpus)
	decide := dqharness.ModelDecision(func(ctx context.Context, m, p string) (string, error) {
		return chat.Chat(ctx, m, p)
	}, *model, decisions)

	sum := sha256.Sum256([]byte(promptTemplate))
	runCfg := dqharness.RunConfig{
		Model:          *model,
		PromptTemplate: promptTemplate,
		PromptHash:     hex.EncodeToString(sum[:]),
		EvidenceTokens: dqharness.DefaultEvidenceTokens,
	}
	opts := dqharness.RunOptions{
		Corpus: *corpus, Rules: rules, Config: runCfg, Binding: binding, Decisions: decisions,
		DecideA: decide, DecideB: decide,
		Retriever:      dqharness.KeywordRetriever{},
		Sandbox:        nil,
		Limits:         limits,
		CheckpointPath: filepath.Join(*outDir, "checkpoint.jsonl"),
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fail(err)
	}
	out, err := dqharness.RunComparison(context.Background(), opts)
	if err != nil {
		fail(err)
	}
	// The matched conditions are stamped by RunComparison from runCfg.
	if out.Stopped != "" {
		fmt.Printf("Stopped cleanly: %s\n", out.Stopped)
	}
	if err := dqharness.VerifyRun(*corpus, rules, out.A, out.B); err != nil {
		// A partial run is not a result. Preserve it and stop.
		writeJSON(filepath.Join(*outDir, "run-A.json"), out.A)
		writeJSON(filepath.Join(*outDir, "run-B.json"), out.B)
		fail(fmt.Errorf("run is not verifiable (partial or unmatched): %w", err))
	}
	writeJSON(filepath.Join(*outDir, "run-A.json"), out.A)
	writeJSON(filepath.Join(*outDir, "run-B.json"), out.B)

	in, rerr := computeReport(*outDir, *corpus, rules, *profile == "pilot")
	if rerr != nil {
		fail(rerr)
	}
	if err := decisionquality.WriteReport(filepath.Join(*outDir, "REPORT.md"), in); err != nil {
		fail(err)
	}
	fmt.Print(decisionquality.RenderReport(in))
}

func buildBinding(root, corpusPath, model string) (dqharness.Binding, error) {
	h := func(p string) (string, error) {
		data, err := os.ReadFile(p)
		if err != nil {
			return "", err
		}
		return dqharness.HashBytes(data), nil
	}
	corpus, err := h(corpusPath)
	if err != nil {
		return dqharness.Binding{}, err
	}
	protocol, err := h(filepath.Join(root, "PROTOCOL.md"))
	if err != nil {
		return dqharness.Binding{}, err
	}
	rubric, err := h(filepath.Join(root, "RUBRIC.json"))
	if err != nil {
		return dqharness.Binding{}, err
	}
	return dqharness.Binding{
		Corpus: corpus, Protocol: protocol, Rubric: rubric,
		Harness: dqharness.HarnessRevision, Model: model,
		Sampling: dqharness.SamplingLabel(dqharness.RunConfig{Model: model}),
	}, nil
}

func auditRetrievalCoverage(outDir string, corpus decisionquality.Corpus) {
	retriever := dqharness.KeywordRetriever{}
	decisions := dqharness.DecisionSet(corpus)
	type caseCov struct {
		CaseID    string   `json:"case_id"`
		Retrieved []string `json:"retrieved"`
		Included  []string `json:"included"`
		Items     int      `json:"evidence_items"`
		Tokens    int      `json:"tokens"`
	}
	var out []caseCov
	var with, without int
	for _, c := range corpus.Cases {
		got, err := retriever.Retrieve(c, dqharness.DefaultEvidenceTokens)
		if err != nil {
			fail(err)
		}
		prompt := dqharness.BuildPrompt(c, got, decisions)
		row := caseCov{CaseID: c.ID, Items: len(c.PreDecisionEvidence)}
		for _, ev := range got {
			row.Retrieved = append(row.Retrieved, ev.RecordID)
			// Actual prompt inclusion: the citation header must be present in the built prompt.
			if strings.Contains(prompt, "["+ev.RecordID+"]") {
				row.Included = append(row.Included, ev.RecordID)
			}
			row.Tokens += len([]rune(ev.Text))/4 + 1
		}
		if len(got) == 0 {
			without++
		} else {
			with++
		}
		out = append(out, row)
	}
	if err := os.MkdirAll(filepath.Join(outDir, "audit"), 0o755); err != nil {
		fail(err)
	}
	writeJSON(filepath.Join(outDir, "audit", "retrieval-coverage.json"), map[string]any{
		"corpus_cases": len(corpus.Cases), "with_evidence": with, "without_evidence": without, "cases": out,
	})
	fmt.Printf("retrieval audit: %d cases, %d with evidence, %d without; wrote %s\n",
		len(corpus.Cases), with, without, filepath.Join(outDir, "audit", "retrieval-coverage.json"))
}

func computeReport(outDir string, corpus decisionquality.Corpus, rules decisionquality.ScoringRules, pilot bool) (decisionquality.ReportInput, error) {
	a, err := decisionquality.ReadRun(filepath.Join(outDir, "run-A.json"))
	if err != nil {
		return decisionquality.ReportInput{}, fmt.Errorf("no measured arm A artifact: %w", err)
	}
	b, err := decisionquality.ReadRun(filepath.Join(outDir, "run-B.json"))
	if err != nil {
		return decisionquality.ReportInput{}, fmt.Errorf("no measured arm B artifact: %w", err)
	}
	if err := dqharness.VerifyRun(corpus, rules, a, b); err != nil {
		return decisionquality.ReportInput{}, err
	}
	sa, sb := decisionquality.Summarize(a), decisionquality.Summarize(b)
	paired := decisionquality.PairedRun(a, b, 20261019)
	gate := decisionquality.EvaluateGate(rules, sa, sb, paired)
	insufficient := decisionquality.InsufficientEvidence(corpus, a, b)
	title := "RDM-004 — Decision-quality comparison"
	dec := decisionquality.Decision(gate.Pass, false, false)
	if insufficient != "" {
		dec = ""
	}
	var gateFailure, promptEq, note string
	if pilot {
		title = "Decision-memory pilot — planning-only"
		dec = decisionquality.PilotDecision(gate, insufficient)
		gateFailure = decisionquality.InsufficientCoverage(corpus)
		promptEq = "unverified: prompts were not persisted; equal estimated token totals are consistent with, but do not prove, identical prompts"
		note = "NO-GO for rollout; retrieval benefit was not evaluated."
	}
	cov, cerr := dqharness.MeasureCoverage(corpus, dqharness.KeywordRetriever{})
	var coverage *decisionquality.CoverageReport
	if cerr == nil {
		coverage = &decisionquality.CoverageReport{Cases: cov.Cases, WithEvidence: cov.WithEvidence, Without: cov.Without}
	}
	return decisionquality.ReportInput{
		Title: title, Corpus: corpus, Rules: rules, A: sa, B: sb, Paired: paired, Gate: gate,
		Decision: dec, Insufficient: insufficient, NotEvaluated: []string{"C", "P"},
		Coverage: coverage, SafetyAvailable: false,
		OriginalCoverageNote: originalCoverageNote(pilot, len(corpus.Cases)),
		PromptEquality:       promptEq,
		CoverageGateFailure:  gateFailure,
		DecisionNote:         note,
		RunA:                 a, RunB: b,
	}, nil
}

func originalCoverageNote(pilot bool, cases int) string {
	if !pilot {
		return ""
	}
	return fmt.Sprintf("0 of %d cases (audit-reconstructed: the legacy AND query matched no evidence and both arms had identical estimated token totals)", cases)
}

func verifyArtifacts(outDir string, corpus decisionquality.Corpus, rules decisionquality.ScoringRules, pilot bool) {
	in, err := computeReport(outDir, corpus, rules, pilot)
	if err != nil {
		fail(err)
	}
	onDisk, err := os.ReadFile(filepath.Join(outDir, "REPORT.md"))
	if err != nil {
		fail(fmt.Errorf("no measured report: %w", err))
	}
	if strings.TrimSpace(string(onDisk)) != strings.TrimSpace(decisionquality.RenderReport(in)) {
		fail(fmt.Errorf("REPORT.md does not match the recomputed metrics for these run artifacts"))
	}
	fmt.Printf("verified: %d cases, %d repeats/arm, primary A=%.2f%% B=%.2f%%, paired=%.2f pp [%.2f, %.2f]\n",
		len(corpus.Cases), rules.RepeatsPerCase, in.A.Primary, in.B.Primary, in.Paired.MeanDiffPP, in.Paired.LowerPP, in.Paired.UpperPP)
}

func renderArtifacts(outDir string, corpus decisionquality.Corpus, rules decisionquality.ScoringRules, pilot bool) {
	in, err := computeReport(outDir, corpus, rules, pilot)
	if err != nil {
		fail(err)
	}
	if err := decisionquality.WriteReport(filepath.Join(outDir, "REPORT.md"), in); err != nil {
		fail(err)
	}
	fmt.Print(decisionquality.RenderReport(in))
}

func writeJSON(path string, v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err == nil {
		_ = os.WriteFile(path, append(b, '\n'), 0o644)
	}
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "rdm004-eval: %v\n", err)
	os.Exit(1)
}
