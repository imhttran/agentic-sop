// Command deceleval-advisory runs the advisory (Experimental B) evaluation over
// the frozen decision-model corpus and writes the paired A/B artifact.
//
// For each scenario it computes the same deterministic classification the
// baseline uses, then runs the REAL SEAM-004 advisory checkpoint once via
// internal/cli.EvaluateAdvisoryDecision (CLEF primary, Nimble backup via the
// configured adapter; deterministic fallback on failure). No prompt or model is
// tuned here; malformed/unavailable model results are recorded, never excluded.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/imhttran/agentic-sop/internal/cli"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/deceleval"
	"github.com/imhttran/agentic-sop/internal/failure"
	"github.com/imhttran/agentic-sop/internal/taskfile"
)

func main() {
	corpusPath := flag.String("corpus", "evals/decision-model/corpus.json", "frozen corpus")
	freezePath := flag.String("freeze", "evals/decision-model/CORPUS.sha256", "corpus freeze")
	adapter := flag.String("adapter", "/tmp/sop-decision-adapter", "decision adapter executable argv[0]")
	adapterProvider := flag.String("adapter-provider", "clef", "adapter profile (clef)")
	out := flag.String("out", "evals/decision-model/paired-A-B.results.json", "paired results output")
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

	// Baseline A (deterministic only).
	baseline := deceleval.RunBaseline(c, corpusSHA)

	// Experimental B: advisory decisions enabled via the real checkpoint.
	cfg := config.Default()
	enabled := true
	cfg.Decision.Enabled = &enabled
	cfg.Decision.Provider = "adapter"
	cfg.Decision.AdapterCommand = []string{*adapter}
	cfg.Decision.AdapterProvider = *adapterProvider

	ctx := context.Background()
	adv := make([]deceleval.AdvisoryRecord, 0, len(c.Scenarios))
	for _, s := range c.Scenarios {
		rec := deceleval.AdvisoryRecord{ScenarioID: s.ID, Category: s.Category, Expected: string(s.Expected.Action)}
		ev, err := deceleval.EvidenceFor(s)
		if err != nil {
			rec.AdvisoryError = err.Error()
			adv = append(adv, rec)
			continue
		}
		cls := failure.Classify(ev)
		spec := &taskfile.Spec{Title: s.Observable}
		start := time.Now()
		oc := cli.EvaluateAdvisoryDecision(ctx, cfg, cls, spec)
		rec.LatencyMS = int(time.Since(start).Milliseconds())
		rec.BaselineAction = oc.BaselineAction
		rec.Choice = oc.Choice
		rec.Confidence = oc.Confidence
		rec.FinalAction = oc.FinalAction
		rec.Consulted = oc.Consulted
		rec.OutcomeClass = oc.OutcomeClass
		rec.Escalated = oc.Escalated
		rec.ServedProvider = oc.ServedProvider
		rec.FallbackReason = oc.FallbackReason
		switch {
		case !oc.Consulted:
			rec.ProviderUsed = "none"
		case oc.OutcomeClass == "unavailable":
			rec.ProviderUsed = "unavailable"
		case oc.ServedProvider != "":
			rec.ProviderUsed = oc.ServedProvider
		default:
			rec.ProviderUsed = *adapterProvider
		}
		adv = append(adv, rec)
	}

	paired := deceleval.BuildPaired(c, baseline, adv)
	data, err := deceleval.MarshalPaired(paired)
	if err != nil {
		log.Fatal(err)
	}
	sha := deceleval.Hash(data)

	m := paired.Metrics
	fmt.Printf("corpus_sha256=%s\n", corpusSHA)
	fmt.Printf("paired_sha256=%s\n", sha)
	fmt.Printf("paired=%d/%d\n", m.Paired, m.Total)
	fmt.Printf("det_correct=%d adv_correct=%d\n", m.DeterministicCorrect, m.AdvisoryCorrect)
	fmt.Printf("unsafe_missed_det=%d unsafe_missed_adv=%d\n", m.UnsafeMissedDeterministic, m.UnsafeMissedAdvisory)
	fmt.Printf("useful_escalations=%d false_escalations=%d final_actions_changed=%d\n", m.UsefulEscalations, m.FalseEscalations, m.FinalActionsChanged)
	fmt.Printf("added_latency_median_ms=%.0f added_latency_p95_ms=%.0f\n", m.AddedLatencyMedianMS, m.AddedLatencyP95MS)
	fmt.Printf("provider_usage=%v\n", m.ProviderUsage)
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		log.Fatal(err)
	}
	_ = os.WriteFile(*out+".sha256", []byte(sha+"  paired-A-B.results.json\n"), 0o644)
	fmt.Printf("wrote %s\n", *out)
}
