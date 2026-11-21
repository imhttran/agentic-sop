// Command deceleval-ab compares two served models in SOP's structured-decision
// role on one frozen corpus.
//
// It sends the production structured-decision request (the same chat prompt,
// JSON-object schema, and response parser the decision adapter uses) to one
// model at a time, exactly once per case, with no retry and no fallback. It
// records the first attempt, scores schema validity and the expected governed
// action, and reports per-model/per-concurrency quality and latency.
//
// The existing frozen DME corpus (evals/decision-model/corpus.json) and its
// results are never modified. This command reads a separate versioned corpus and
// its own freeze file.
//
// Quick mode runs the 12-case smoke split; full mode runs the 36-case held-out
// split only after a completed quick run is present. Artifacts default to a new
// temporary directory outside the repository.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/imhttran/agentic-sop/internal/deceleval"
	"github.com/imhttran/agentic-sop/internal/decisionadapter/providers/clef"
)

func main() {
	corpusPath := flag.String("corpus", "evals/decision-model/disposition-ab-v1.json", "versioned A/B corpus")
	freezePath := flag.String("freeze", "evals/decision-model/DISPOSITION-AB.sha256", "corpus freeze (<sha256>  <path>); empty disables the check")
	promptFreezePath := flag.String("prompt-freeze", "evals/decision-model/DISPOSITION-AB-PROMPT.sha256", "prompt freeze (<sha256>  prompt); empty disables the check")
	endpoint := flag.String("endpoint", "http://127.0.0.1:8000", "OpenAI-compatible oMLX endpoint")
	modelsFlag := flag.String("models", "Qwen3-4B-4bit,Inkling-2-Mini-4bit", "exact served model ids, comma-separated")
	mode := flag.String("mode", "quick", "quick | full | all")
	concurrencyFlag := flag.String("concurrency", "1,4", "comma-separated concurrency levels")
	timeout := flag.Duration("timeout", 60*time.Second, "per-attempt timeout")
	outDir := flag.String("out", "", "artifact directory (default: a new temp directory outside the repository)")
	discoverOnly := flag.Bool("discover", false, "print the served model ids and endpoint metadata, then exit")
	printPromptHash := flag.Bool("print-prompt-hash", false, "print the frozen prompt sha256, then exit")
	noWarmup := flag.Bool("no-warmup", false, "skip warm-up requests (not recommended)")
	flag.Parse()

	if *printPromptHash {
		fmt.Println(deceleval.ABPromptSHA256())
		return
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if strings.TrimSpace(*freezePath) != "" {
		if err := deceleval.VerifyFreeze(*corpusPath, *freezePath); err != nil {
			fail(err)
		}
	}
	if strings.TrimSpace(*promptFreezePath) != "" {
		frozen, err := os.ReadFile(*promptFreezePath)
		if err != nil {
			fail(err)
		}
		fields := strings.Fields(string(frozen))
		if len(fields) == 0 || fields[0] != deceleval.ABPromptSHA256() {
			fail(fmt.Errorf("prompt freeze mismatch: prompt is %s, frozen %v", deceleval.ABPromptSHA256(), fields))
		}
	}
	corpusSHA, err := deceleval.FreezeOf(*corpusPath)
	if err != nil {
		fail(err)
	}
	corpus, err := deceleval.LoadAB(*corpusPath)
	if err != nil {
		fail(err)
	}
	if err := corpus.Validate(); err != nil {
		fail(err)
	}

	info := deceleval.DiscoverABEndpoint(ctx, *endpoint, 10*time.Second)
	printEndpoint(info)
	if *discoverOnly {
		if info.Error != "" {
			os.Exit(1)
		}
		return
	}
	if info.Error != "" {
		fail(fmt.Errorf("endpoint unavailable: %s", info.Error))
	}

	requested := splitList(*modelsFlag)
	if len(requested) == 0 {
		fail(errors.New("-models is required (comma-separated exact served model ids)"))
	}
	var missing []string
	for _, m := range requested {
		if !info.HasModel(m) {
			missing = append(missing, m)
		}
	}
	concurrency, err := parseConcurrency(*concurrencyFlag)
	if err != nil {
		fail(err)
	}

	dir := *outDir
	if strings.TrimSpace(dir) == "" {
		dir, err = os.MkdirTemp("", "decision-ab-")
		if err != nil {
			fail(err)
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fail(err)
	}

	modes, err := expandModes(*mode)
	if err != nil {
		fail(err)
	}
	for _, m := range modes {
		if m.mode == "full" {
			if err := requireSmokeSummary(dir, requested); err != nil {
				fail(err)
			}
		}
		if err := runMode(ctx, m.mode, m.split, dir, corpus, corpusSHA, info, requested, missing, concurrency, *timeout, !*noWarmup); err != nil {
			fail(err)
		}
		if ctx.Err() != nil {
			fail(ctx.Err())
		}
	}

	fmt.Printf("\nartifacts: %s\n", dir)
	if len(missing) > 0 {
		fmt.Printf("NOT MEASURED (model id not advertised): %s\n", strings.Join(missing, ", "))
	}
}

type runModeSpec struct {
	mode  string
	split string
}

func expandModes(mode string) ([]runModeSpec, error) {
	switch strings.TrimSpace(mode) {
	case "quick":
		return []runModeSpec{{"quick", deceleval.ABSplitSmoke}}, nil
	case "full":
		return []runModeSpec{{"full", deceleval.ABSplitHeldout}}, nil
	case "all":
		return []runModeSpec{
			{"quick", deceleval.ABSplitSmoke},
			{"full", deceleval.ABSplitHeldout},
		}, nil
	default:
		return nil, fmt.Errorf("unknown -mode %q (want quick, full, or all)", mode)
	}
}

func runMode(ctx context.Context, mode, split, dir string, corpus *deceleval.ABCorpus, corpusSHA string, info deceleval.ABEndpointInfo, requested, missing []string, concurrency []int, timeout time.Duration, warmup bool) error {
	cases := corpus.Split(split)
	if len(cases) == 0 {
		return fmt.Errorf("split %q has no cases", split)
	}
	started := time.Now().UTC()

	attemptsPath := filepath.Join(dir, "attempts-"+mode+".jsonl")
	attemptsFile, err := os.Create(attemptsPath)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(attemptsFile)
	var (
		writeMu  sync.Mutex
		writeErr error
	)
	writeAttempt := func(a deceleval.ABAttempt) {
		writeMu.Lock()
		defer writeMu.Unlock()
		if writeErr != nil {
			return
		}
		if err := enc.Encode(a); err != nil {
			writeErr = err
		}
	}

	meta := deceleval.ABRunMeta{
		Mode:             mode,
		Split:            split,
		Endpoint:         info,
		CorpusSHA256:     corpusSHA,
		PromptSHA256:     deceleval.ABPromptSHA256(),
		Prompt:           deceleval.ABPromptSpec(),
		ModelsRequested:  requested,
		Concurrency:      concurrency,
		TimeoutMS:        timeout.Milliseconds(),
		MaxTokens:        nil,
		CaseOrder:        caseIDs(cases),
		OMLXVersion:      nil,
		ModelRevisions:   map[string]*string{},
		ReasoningSetting: "the production SystemOne transport exposes no reasoning/thinking parameter; the endpoint's decision engine applies its own setting",
		SamplingSetting:  "the production SystemOne transport exposes no sampling parameters; the endpoint's decision engine applies its own settings",
		CachePolicy:      "no cache directive is sent; the endpoint default applies",
		Notes: []string{
			"One attempt per case; no retry and no fallback. The production clef provider is called directly, so the adapter CLI's fallback chain cannot mask a first-pass failure.",
			"Models are run one at a time; every model uses the same case order, request text, output cap, timeout, and cache policy.",
			"oMLX exposes no version or model revision through /v1/models or /health; those fields are null. The SystemOne response reports input/output tokens only, so TTFT, decode speed, and model-load duration are null.",
			"The production SystemOne transport sends no max-tokens field, so no output cap is applied; both models use the endpoint default identically.",
			"This corpus exercises the six-action governed disposition contract. The production SEAM-004 change-risk checkpoint offers only LOW/MEDIUM/HIGH/HUMAN and can only add NEEDS_HUMAN.",
		},
	}
	for _, m := range requested {
		meta.ModelRevisions[m] = nil
	}

	var (
		summaries   []deceleval.ABBatchSummary
		allAttempts []deceleval.ABAttempt
		statuses    []deceleval.ABModelStatus
		interrupted bool
	)
	for _, model := range requested {
		if containsString(missing, model) {
			statuses = append(statuses, deceleval.ABModelStatus{
				ID:        model,
				Available: false,
				Error:     "model id is not advertised by the endpoint",
			})
			continue
		}
		decide := newDecider(info.Endpoint, model, timeout)
		status := deceleval.ABModelStatus{ID: model, Available: true}
		if warmup {
			cold, warm, werr := warmupModel(ctx, decide, timeout)
			if werr != nil {
				status.Available = false
				status.Error = werr.Error()
				statuses = append(statuses, status)
				fmt.Fprintf(os.Stderr, "model %s unavailable: %v\n", model, werr)
				continue
			}
			status.ColdLoadMS = cold
			status.WarmupMS = warm
		}
		for _, c := range concurrency {
			attempts, wall := deceleval.RunABBatch(ctx, model, split, cases, c, timeout, decide, writeAttempt)
			sum := deceleval.SummarizeAB(attempts, len(cases), wall)
			sum.Split = split
			sum.Model = model
			sum.Concurrency = c
			sum.ColdLoadMS = status.ColdLoadMS
			sum.WarmupMS = status.WarmupMS
			summaries = append(summaries, sum)
			allAttempts = append(allAttempts, attempts...)
			if ctx.Err() != nil {
				interrupted = true
				break
			}
		}
		statuses = append(statuses, status)
		if interrupted {
			break
		}
	}

	// Never publish a "complete" summary whose per-case JSONL evidence was not
	// persisted. A write or close failure is an error, not a silent success.
	if writeErr != nil {
		_ = attemptsFile.Close()
		return fmt.Errorf("write attempts %s: %w", attemptsPath, writeErr)
	}
	if err := attemptsFile.Close(); err != nil {
		return fmt.Errorf("close attempts %s: %w", attemptsPath, err)
	}

	meta.ModelStatus = statuses
	meta.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	meta.StartedAt = started.Format(time.RFC3339)
	if err := writeJSON(filepath.Join(dir, "run-"+mode+".json"), meta); err != nil {
		return err
	}

	summary := deceleval.ABRunSummary{
		Mode:              mode,
		Split:             split,
		CorpusSHA256:      corpusSHA,
		PromptSHA256:      deceleval.ABPromptSHA256(),
		ModelsRequested:   requested,
		ModelsUnavailable: unavailableModels(statuses),
		Batches:           summaries,
		Disagreements:     deceleval.ConsequentialDisagreements(allAttempts),
		OMLXVersion:       meta.OMLXVersion,
		ModelRevisions:    meta.ModelRevisions,
		ReasoningSetting:  meta.ReasoningSetting,
		SamplingSetting:   meta.SamplingSetting,
		CachePolicy:       meta.CachePolicy,
	}
	for _, st := range statuses {
		if !st.Available {
			summary.Notes = append(summary.Notes, fmt.Sprintf("`%s` was not measured: %s (full endpoint error in run-%s.json)", st.ID, clip(st.Error, 600), mode))
		}
	}
	if interrupted || ctx.Err() != nil {
		summary.Notes = append(summary.Notes, "the run was interrupted; batches are incomplete")
	}
	if err := writeJSON(filepath.Join(dir, "summary-"+mode+".json"), summary); err != nil {
		return err
	}
	report := deceleval.RenderABReport(summary)
	if err := os.WriteFile(filepath.Join(dir, "REPORT-"+mode+".md"), []byte(report), 0o644); err != nil {
		return err
	}
	fmt.Print("\n" + report)
	return nil
}

func newDecider(endpoint, model string, timeout time.Duration) deceleval.ABDecideFunc {
	return func(ctx context.Context, s deceleval.Scenario) deceleval.ABObservation {
		req := deceleval.BuildABRequest(s)
		out, _ := clef.DecideOnce(ctx, endpoint, model, timeout, req)
		obs := deceleval.ABObservation{
			SchemaValid:   out.SchemaValid,
			ErrorCategory: out.ErrorCategory,
			ErrorDetail:   out.ErrorDetail,
		}
		if out.SchemaValid {
			in := out.Result.Usage.InputTokens
			outT := out.Result.Usage.OutputTokens
			obs.InputTokens = &in
			obs.OutputTokens = &outT
		}
		if out.SchemaValid {
			obs.ActualAction = out.Result.Answers[deceleval.ABQuestionID].Choice
		}
		// TimeToFirstAnswerContentMS stays nil: the production request is
		// non-streaming, so content-level TTFT is not observable without changing
		// the path under evaluation.
		return obs
	}
}

type warmupResult struct {
	elapsed time.Duration
	err     error
}

func warmupModel(ctx context.Context, decide deceleval.ABDecideFunc, timeout time.Duration) (*int64, *int64, error) {
	warm := deceleval.Scenario{
		ID:         "warmup",
		Category:   "benign_impl",
		Source:     "synthetic",
		Observable: "Warm-up request: classify a synthetic example. The answer is not scored.",
		Split:      "warmup",
		Expected:   deceleval.Expected{Action: deceleval.AutoFix, Rationale: "warm-up only"},
	}
	cold := measureWarmup(ctx, decide, warm, timeout)
	if cold.err != nil {
		confirm := measureWarmup(ctx, decide, warm, timeout)
		if confirm.err != nil {
			return nil, nil, fmt.Errorf("warm-up failed: %v", confirm.err)
		}
		ms := confirm.elapsed.Milliseconds()
		return &ms, &ms, nil
	}
	warm2 := measureWarmup(ctx, decide, warm, timeout)
	if warm2.err != nil {
		return nil, nil, fmt.Errorf("second warm-up failed: %v", warm2.err)
	}
	coldMS := cold.elapsed.Milliseconds()
	warmMS := warm2.elapsed.Milliseconds()
	return &coldMS, &warmMS, nil
}

func measureWarmup(ctx context.Context, decide deceleval.ABDecideFunc, s deceleval.Scenario, timeout time.Duration) warmupResult {
	callCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	start := time.Now()
	obs := decide(callCtx, s)
	elapsed := time.Since(start)
	if err := warmupFailure(obs); err != nil {
		return warmupResult{elapsed: elapsed, err: err}
	}
	return warmupResult{elapsed: elapsed}
}

// warmupFailure distinguishes a reachability failure from a schema failure. A
// malformed answer means the model served the request; only a transport, HTTP, or
// timeout failure means it could not be measured.
func warmupFailure(obs deceleval.ABObservation) error {
	switch obs.ErrorCategory {
	case "timeout", "http_error", "unavailable", "provider_error":
		if obs.ErrorDetail != "" {
			return errors.New(obs.ErrorDetail)
		}
		return errors.New(obs.ErrorCategory)
	default:
		return nil
	}
}

func requireSmokeSummary(dir string, requested []string) error {
	path := filepath.Join(dir, "summary-quick.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("full mode requires a completed quick run in %s (run -mode quick first, or use -mode all): %w", dir, err)
	}
	var s deceleval.ABRunSummary
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	complete := map[string]bool{}
	for _, b := range s.Batches {
		if b.Split == deceleval.ABSplitSmoke && b.Attempted > 0 && b.Complete {
			complete[b.Model] = true
		}
	}
	unavailable := map[string]bool{}
	for _, m := range s.ModelsUnavailable {
		unavailable[m] = true
	}
	for _, m := range requested {
		if complete[m] || unavailable[m] {
			continue
		}
		return fmt.Errorf("full mode requires a completed smoke batch for %q; run -mode quick first", m)
	}
	return nil
}

func parseConcurrency(raw string) ([]int, error) {
	var out []int
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 {
			return nil, fmt.Errorf("invalid concurrency %q (want a positive integer)", p)
		}
		out = append(out, n)
	}
	if len(out) == 0 {
		return nil, errors.New("-concurrency must contain at least one positive integer")
	}
	return out, nil
}

func splitList(raw string) []string {
	var out []string
	seen := map[string]bool{}
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

func caseIDs(cases []deceleval.Scenario) []string {
	ids := make([]string, 0, len(cases))
	for _, s := range cases {
		ids = append(ids, s.ID)
	}
	return ids
}

func unavailableModels(statuses []deceleval.ABModelStatus) []string {
	var out []string
	for _, s := range statuses {
		if !s.Available {
			out = append(out, s.ID)
		}
	}
	return out
}

func containsString(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func printEndpoint(info deceleval.ABEndpointInfo) {
	fmt.Printf("endpoint: %s\n", info.Endpoint)
	if info.Server != "" {
		fmt.Printf("server:   %s\n", info.Server)
	}
	if info.Error != "" {
		fmt.Printf("error:    %s\n", info.Error)
		return
	}
	fmt.Printf("models:   %s\n", strings.Join(info.ModelIDs(), ", "))
	for _, m := range info.Models {
		fmt.Printf("  - %s (type=%s, max_model_len=%d, owned_by=%s)\n", m.ID, m.ModelType, m.MaxModelLen, m.OwnedBy)
	}
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "deceleval-ab: %v\n", err)
	os.Exit(1)
}
