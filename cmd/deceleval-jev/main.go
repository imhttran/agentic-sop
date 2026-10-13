// Command deceleval-jev runs the JEV shadow-mode evaluation (DME follow-up).
//
// It loads the frozen 61-scenario corpus, runs the deterministic Go baseline, then
// invokes the REAL JEV analyzer (internal/jev.OllamaAnalyzer, via run.RunJEV) for
// every scenario and maps each result to a failure disposition through the
// existing deterministic early-JEV policy (autonomy.DecideEarly). JEV is observed
// only: the deterministic decision is never changed, nothing is consulted for
// authority, and no production routing/threshold/governance behavior is touched.
//
// It stores results under evals/decision-model/ with a distinct JEV experiment
// name and never overwrites the DME-003/004/005 artifacts.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/imhttran/agentic-sop/internal/autonomy"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/deceleval"
	"github.com/imhttran/agentic-sop/internal/jev"
	"github.com/imhttran/agentic-sop/internal/review"
	"github.com/imhttran/agentic-sop/internal/run"
)

func main() {
	corpusPath := flag.String("corpus", "evals/decision-model/corpus.json", "frozen corpus")
	freezePath := flag.String("freeze", "evals/decision-model/CORPUS.sha256", "corpus freeze")
	out := flag.String("out", "evals/decision-model/jev-shadow.results.json", "results output")
	limit := flag.Int("limit", 0, "evaluate only the first N scenarios (0 = all)")
	split := flag.String("split", "", "restrict to a split: eval | tuning (default all)")
	timeout := flag.Duration("timeout", 180*time.Second, "per-scenario JEV timeout")
	flag.Parse()

	if err := deceleval.VerifyFreeze(*corpusPath, *freezePath); err != nil {
		log.Fatalf("corpus freeze check failed: %v", err)
	}
	corpusSHA, err := deceleval.FreezeOf(*corpusPath)
	if err != nil {
		log.Fatal(err)
	}
	c, err := deceleval.Load(*corpusPath)
	if err != nil {
		log.Fatalf("load corpus: %v", err)
	}
	if err := c.Validate(); err != nil {
		log.Fatalf("validate corpus: %v", err)
	}

	baseline := deceleval.RunBaseline(c, corpusSHA)

	// The REAL JEV implementation, selected exactly as production does.
	analyzer, err := jev.NewOllamaAnalyzerFromEnv()
	if err != nil {
		log.Fatalf("BLOCKER: real JEV analyzer unavailable: %v", err)
	}
	provider, model := analyzer.JEVProvider(), analyzer.JEVModel()

	cfg := config.Default()
	failOn := cfg.Quality.JEVFailOn()
	policy := cfg.AutonomyPolicy()

	fmt.Printf("corpus_sha256=%s\n", corpusSHA)
	fmt.Printf("jev_provider=%s jev_model=%s\n", provider, model)
	fmt.Printf("fail_on=%v revision=%s input=%s\n", failOn, deceleval.JEVEvaluationRevision, deceleval.JEVInputMode)
	fmt.Printf("scope=HYPOTHETICAL failure-disposition mapping (JEV used as a classifier); NOT production early-JEV behavior\n")

	ctx := context.Background()
	obs := make([]deceleval.JEVObservation, 0, len(c.Scenarios))
	n := 0
	for _, s := range c.Scenarios {
		if *split != "" && s.Split != *split {
			continue
		}
		if *limit > 0 && n >= *limit {
			break
		}
		n++
		obs = append(obs, runOne(ctx, analyzer, failOn, policy, *timeout, s))
	}

	res := deceleval.BuildJEV(baseline, obs, provider, model, failOn)
	data, err := deceleval.MarshalJEV(res)
	if err != nil {
		log.Fatal(err)
	}
	sum := sha256.Sum256(data)
	sha := hex.EncodeToString(sum[:])

	report(res, sha)

	if err := os.WriteFile(*out, data, 0o644); err != nil {
		log.Fatalf("write %s: %v", *out, err)
	}
	if err := os.WriteFile(*out+".sha256", []byte(sha+"  jev-shadow.results.json\n"), 0o644); err != nil {
		log.Fatalf("write results hash: %v", err)
	}
	fmt.Printf("wrote %s\n", *out)
}

func runOne(ctx context.Context, analyzer jev.Analyzer, failOn []string, policy autonomy.Policy, timeout time.Duration, s deceleval.Scenario) deceleval.JEVObservation {
	rec := deceleval.JEVObservation{ScenarioID: s.ID, Category: s.Category, Split: s.Split, Expected: string(s.Expected.Action)}
	// Only the observable failure text is shared with JEV; no label or category.
	callCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	outcome := run.RunJEV(callCtx, analyzer, run.JEVInvocation{Task: s.Observable})
	rec.Ran = outcome.Ran
	rec.LatencyMS = int(outcome.DurationMS)
	rec.Provider = outcome.Provider
	rec.Model = outcome.Model
	rec.ToolCalls = outcome.ToolCalls
	if outcome.Error != nil {
		rec.Error = outcome.Error.Error()
		return rec
	}
	rec.Available = true
	rec.Status = string(outcome.Result.Status)
	rec.FindingCount = len(outcome.Result.Findings)
	rec.MaxSeverity = maxSeverity(outcome.Result.Findings)
	directive, ok := deceleval.MapJEVHypotheticalDirective(outcome.Result, failOn, policy)
	rec.Directive = directive
	rec.Actionable = ok
	return rec
}

// maxSeverity returns the highest finding severity as a string ("" when none).
func maxSeverity(findings []jev.Finding) string {
	best := review.Severity("")
	rank := map[review.Severity]int{"INFO": 0, "LOW": 1, "MEDIUM": 2, "HIGH": 3, "CRITICAL": 4}
	for _, f := range findings {
		if best == "" || rank[f.Severity] > rank[best] {
			best = f.Severity
		}
	}
	return string(best)
}

func report(res deceleval.JEVResult, sha string) {
	m := res.Metrics
	fmt.Printf("result_sha256=%s\n", sha)
	fmt.Printf("n=%d available=%d unavailable=%d not_actionable=%d\n", m.N, m.Available, m.Unavailable, m.NotActionable)
	fmt.Printf("exact_agree=%d operational_agree=%d\n", m.ExactAgree, m.OperationalAgree)
	fmt.Printf("useful=%d false=%d unsafe_missed=%d\n", m.UsefulEscalations, m.FalseEscalations, m.UnsafeMissed)
	fmt.Printf("improved=%d worsened=%d agree_with_deterministic=%d\n", m.Improved, m.Worsened, m.AgreeWithDeterminism)
	fmt.Printf("terminal_total=%d terminal_evaluated=%d\n", m.TerminalTotal, m.TerminalEvaluated)
	fmt.Printf("by_directive=%v by_status=%v\n", m.ByDirective, m.ByStatus)
	fmt.Printf("note: by_directive is the HYPOTHETICAL mapping, not production early-JEV disposition\n")
	fmt.Printf("latency_median_ms=%.0f latency_p95_ms=%.0f\n", m.LatencyMedianMS, m.LatencyP95MS)
	// Surface the terminal scenario the task calls out explicitly.
	for _, r := range res.Records {
		if r.ScenarioID == "dme-real-close-004" {
			fmt.Printf("dme-real-close-004: expected=%s det=%s status=%s directive=%s actionable=%v err=%q\n",
				r.Expected, r.DeterministicAction, r.Status, r.Directive, r.Actionable, r.Error)
		}
	}
}
