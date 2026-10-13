// Command deceleval-baseline runs the deterministic-only (Baseline A) evaluation
// over the frozen decision-model corpus and writes a reproducible results
// artifact for paired comparison with the advisory run (DME-004).
//
// It calls no model and does not touch real project state.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/imhttran/agentic-sop/internal/deceleval"
)

func main() {
	corpusPath := flag.String("corpus", "evals/decision-model/corpus.json", "path to the frozen corpus")
	freezePath := flag.String("freeze", "evals/decision-model/CORPUS.sha256", "path to the freeze file")
	out := flag.String("out", "", "path to write the baseline results JSON (default: stdout summary only)")
	flag.Parse()

	// 1. Verify the frozen corpus hash before execution.
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

	// 2. Run Baseline A.
	res := deceleval.RunBaseline(c, corpusSHA)
	data, err := deceleval.MarshalBaseline(res)
	if err != nil {
		log.Fatal(err)
	}
	resultSHA := deceleval.Hash(data)

	// 3. Report.
	m := res.Metrics
	sl := res.SecondLabel
	fmt.Printf("corpus_sha256=%s\n", corpusSHA)
	fmt.Printf("result_sha256=%s\n", resultSHA)
	fmt.Printf("total=%d classified=%d fixture_failures=%d\n", m.Total, m.Classified, m.FixtureFailures)
	fmt.Printf("correct=%d incorrect=%d unsafe_missed_escalation=%d false_escalation=%d\n",
		m.Correct, m.Incorrect, m.UnsafeMissedEscalation, m.FalseEscalation)
	fmt.Printf("latency_median_ms=%.0f latency_p95_ms=%.0f\n", m.LatencyMedianMS, m.LatencyP95MS)
	fmt.Printf("second_label_sample=%d agreement=%.1f%%\n", sl.SampleSize, sl.AgreementPct)

	if *out != "" {
		if err := os.WriteFile(*out, data, 0o644); err != nil {
			log.Fatalf("write %s: %v", *out, err)
		}
		if err := os.WriteFile(*out+".sha256", []byte(resultSHA+"  baseline-A.results.json\n"), 0o644); err != nil {
			log.Fatalf("write results hash: %v", err)
		}
		fmt.Printf("wrote %s\n", *out)
	}
}
