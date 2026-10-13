// Command deceleval-recovery runs the DME-005 failure-recovery advisory
// experiment: it asks the local advisory adapter a failure-RECOVERY question
// (AUTO_FIX / CONTINUE / HUMAN) instead of the production change-RISK question,
// and measures the answer against the frozen labels with the SAME add-attention
// semantics.
//
// Two execution paths produce the SAME RecoveryObservation/RecoveryResult artifact:
//
//	-path real   (default for DME-005) routes through the REAL SEAM-004 advisory
//	             machinery via cli.EvaluateAdvisoryRecovery: the attention gate,
//	             the commandprofile connector, SOP's validator, the adverse rule,
//	             and autonomy.Decide. Only the question kind/choice map are
//	             evaluation-only.
//	-path direct calls the adapter subprocess directly (the earlier follow-up run)
//	             and consults every scenario unconditionally.
//
// Run tuning first to select the question, then freeze the prompt, then eval:
//
//	deceleval-recovery -path real -split tuning -out evals/decision-model/recovery-tuning.results.json
//	deceleval-recovery -path real -split eval -prompt-freeze evals/decision-model/RECOVERY-PROMPT.sha256 -out evals/decision-model/recovery-eval.results.json
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
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
	adapter := flag.String("adapter", "/tmp/sop-decision-adapter", "decision adapter executable")
	provider := flag.String("provider", "clef", "adapter profile")
	path := flag.String("path", "real", "execution path: real | direct")
	split := flag.String("split", "", "split to evaluate: eval | tuning | all")
	timeout := flag.Duration("timeout", 60*time.Second, "per-decision adapter timeout (direct path)")
	promptFreeze := flag.String("prompt-freeze", "", "path to the frozen prompt hash to verify before running")
	out := flag.String("out", "", "path to write the recovery results JSON (default: stdout summary only)")
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

	// 2. Verify the frozen prompt before touching the held-out split.
	promptSHA := deceleval.RecoveryPromptSHA256()
	if *promptFreeze != "" {
		frozen, err := os.ReadFile(*promptFreeze)
		if err != nil {
			log.Fatalf("read prompt freeze: %v", err)
		}
		if fields := strings.Fields(string(frozen)); len(fields) > 0 && fields[0] != promptSHA {
			log.Fatalf("recovery prompt freeze mismatch: prompt is %s, frozen %s", promptSHA, fields[0])
		}
	}

	// Baseline A (deterministic only), reused as the pairing anchor.
	baseline := deceleval.RunBaseline(c, corpusSHA)

	// 3. Ask the recovery question for each scenario in the split.
	var obs []deceleval.RecoveryObservation
	switch *path {
	case "real":
		obs = runReal(c, *split, *adapter, *provider)
	case "direct":
		obs = runDirect(c, *split, *adapter, *provider, *timeout)
	default:
		log.Fatalf("unknown -path %q (want real or direct)", *path)
	}

	res := deceleval.BuildRecovery(c, baseline, obs)
	data, err := deceleval.MarshalRecovery(res)
	if err != nil {
		log.Fatal(err)
	}
	sha := deceleval.Hash(data)

	report(res, corpusSHA, promptSHA, sha, *split, *path)

	if *out != "" {
		if err := os.WriteFile(*out, data, 0o644); err != nil {
			log.Fatalf("write %s: %v", *out, err)
		}
		if err := os.WriteFile(*out+".sha256", []byte(sha+"  recovery.results.json\n"), 0o644); err != nil {
			log.Fatalf("write results hash: %v", err)
		}
		fmt.Printf("wrote %s\n", *out)
	}
}

// runReal routes the failure-recovery question through the real SEAM-004 advisory
// machinery (attention gate + connector + validator + adverse rule + autonomy).
func runReal(c *deceleval.Corpus, split, adapter, provider string) []deceleval.RecoveryObservation {
	cfg := config.Default()
	enabled := true
	cfg.Decision.Enabled = &enabled
	cfg.Decision.Provider = "adapter"
	cfg.Decision.AdapterCommand = []string{adapter}
	cfg.Decision.AdapterProvider = provider

	ctx := context.Background()
	obs := make([]deceleval.RecoveryObservation, 0, len(c.Scenarios))
	for _, s := range c.Scenarios {
		if split != "" && split != "all" && s.Split != split {
			continue
		}
		rec := deceleval.RecoveryObservation{ScenarioID: s.ID, Category: s.Category, Split: s.Split, Expected: string(s.Expected.Action)}
		ev, err := deceleval.EvidenceFor(s)
		if err != nil {
			rec.Error = err.Error()
			rec.OutcomeClass = "runner_error"
			obs = append(obs, rec)
			continue
		}
		cls := failure.Classify(ev)
		oc := cli.EvaluateAdvisoryRecovery(ctx, cfg, cls, &taskfile.Spec{Title: s.Observable})
		rec.OutcomeClass = oc.OutcomeClass
		if rec.OutcomeClass == "" {
			rec.OutcomeClass = "not_eligible"
		}
		rec.LatencyMS = oc.LatencyMS
		if native := deceleval.CanonicalToNativeChoice(oc.Choice); native != "" {
			rec.Consulted = true
			rec.Choice = native
			rec.Recommendation = deceleval.RecoveryRecommendationAction(native)
			conf := oc.Confidence
			rec.Confidence = &conf
			rec.ServedProvider = oc.ServedProvider
			if strings.TrimSpace(rec.ServedProvider) == "" {
				rec.ServedProvider = strings.TrimSpace(provider)
			}
			rec.FallbackReason = oc.FallbackReason
		}
		obs = append(obs, rec)
	}
	return obs
}

// runDirect calls the adapter subprocess directly (the earlier follow-up run),
// consulting every scenario unconditionally. Adapter transport failures are
// outcomes (unavailable); only a runner/fixture problem sets Error.
func runDirect(c *deceleval.Corpus, split, adapter, provider string, timeout time.Duration) []deceleval.RecoveryObservation {
	tmp, err := os.CreateTemp("", "deceleval-recovery-*.json")
	if err != nil {
		log.Fatal(err)
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)

	obs := make([]deceleval.RecoveryObservation, 0, len(c.Scenarios))
	for _, s := range c.Scenarios {
		if split != "" && split != "all" && s.Split != split {
			continue
		}
		obs = append(obs, runOneDirect(adapter, provider, timeout, tmpPath, s))
	}
	return obs
}

func runOneDirect(adapter, provider string, timeout time.Duration, reqPath string, s deceleval.Scenario) deceleval.RecoveryObservation {
	rec := deceleval.RecoveryObservation{
		ScenarioID: s.ID,
		Category:   s.Category,
		Split:      s.Split,
		Expected:   string(s.Expected.Action),
	}
	reqBytes, err := deceleval.MarshalRecoveryRequest(s)
	if err != nil {
		rec.Error = err.Error()
		rec.OutcomeClass = "runner_error"
		return rec
	}
	if err := os.WriteFile(reqPath, reqBytes, 0o644); err != nil {
		rec.Error = err.Error()
		rec.OutcomeClass = "runner_error"
		return rec
	}

	cmd := exec.Command(adapter, "decide", "-provider", provider, "-file", reqPath, "-json", "-timeout", timeout.String())
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	start := time.Now()
	runErr := cmd.Run()
	rec.LatencyMS = int(time.Since(start).Milliseconds())

	if runErr != nil {
		rec.Consulted = false
		rec.OutcomeClass = "unavailable"
		rec.Detail = firstLine(stderr.String())
		return rec
	}
	ans, err := deceleval.ParseRecoveryResult(stdout.Bytes())
	if err != nil {
		rec.Consulted = false
		rec.OutcomeClass = "malformed"
		rec.Detail = err.Error()
		return rec
	}
	rec.Consulted = true
	rec.OutcomeClass = "decision"
	rec.Choice = ans.Choice
	rec.Confidence = ans.Confidence
	rec.Recommendation = deceleval.RecoveryRecommendationAction(ans.Choice)
	rec.ServedProvider = ans.ServedProvider
	rec.Model = ans.Model
	rec.FallbackReason = ans.FallbackReason
	return rec
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

func report(res deceleval.RecoveryResult, corpusSHA, promptSHA, sha, split, path string) {
	if split == "" {
		split = "all"
	}
	fmt.Printf("corpus_sha256=%s\n", corpusSHA)
	fmt.Printf("prompt_sha256=%s\n", promptSHA)
	fmt.Printf("result_sha256=%s\n", sha)
	fmt.Printf("path=%s split=%s\n", path, split)
	for _, name := range []string{"tuning", "eval"} {
		m, ok := res.BySplit[name]
		if !ok {
			continue
		}
		fmt.Printf("[%s] n=%d consulted=%d baseline_correct=%d recommend_correct=%d final_correct=%d operational_agree=%d\n",
			name, m.N, m.Consulted, m.BaselineCorrect, m.RecommendCorrect, m.FinalCorrect, m.OperationalAgree)
		fmt.Printf("[%s] useful_escalations=%d false_escalations=%d unsafe_missed=%d no_recommendation=%d not_eligible=%d\n",
			name, m.UsefulEscalations, m.FalseEscalations, m.UnsafeMissed, m.NoRecommendation, m.NotEligible)
		fmt.Printf("[%s] by_choice=%v provider_usage=%v\n", name, m.ByChoice, m.ProviderUsage)
		fmt.Printf("[%s] latency_median_ms=%.0f latency_p95_ms=%.0f\n", name, m.LatencyMedianMS, m.LatencyP95MS)
	}
}
