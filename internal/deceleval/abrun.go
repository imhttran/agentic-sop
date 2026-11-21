package deceleval

// A/B benchmark execution, endpoint discovery, and report rendering. Tooling
// only: it records measurements and never changes production defaults.

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// ABAttemptLog is the benchmark's per-case evidence sink. It writes one JSON
// object per attempt and retains the first write error, so a failed write can
// never be mistaken for a complete artifact.
type ABAttemptLog struct {
	enc *json.Encoder
	err error
}

// NewABAttemptLog wraps w as an attempt log.
func NewABAttemptLog(w io.Writer) *ABAttemptLog {
	return &ABAttemptLog{enc: json.NewEncoder(w)}
}

// Write appends one attempt. After the first write failure, later attempts are
// dropped and the first error is retained.
func (l *ABAttemptLog) Write(a ABAttempt) {
	if l.err != nil {
		return
	}
	if err := l.enc.Encode(a); err != nil {
		l.err = err
	}
}

// Err reports the first write failure, or nil.
func (l *ABAttemptLog) Err() error { return l.err }

// RunABBatch runs the cases in corpus order with a bounded worker pool. The
// measured batch wall time starts before the workers are created and stops after
// the last attempt is collected. Every case produces exactly one attempt record;
// an error or timeout is a recorded attempt, never a missing case.
func RunABBatch(ctx context.Context, model, split string, cases []Scenario, concurrency int, timeout time.Duration, decide ABDecideFunc, onAttempt func(ABAttempt)) ([]ABAttempt, time.Duration) {
	if concurrency < 1 {
		concurrency = 1
	}
	type result struct {
		idx int
		a   ABAttempt
	}
	jobs := make(chan int)
	results := make(chan result, len(cases))
	var wg sync.WaitGroup
	start := time.Now()
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				if ctx.Err() != nil {
					return
				}
				select {
				case <-ctx.Done():
					return
				case i, ok := <-jobs:
					if !ok {
						return
					}
					results <- result{idx: i, a: runOneAB(ctx, model, split, concurrency, cases[i], timeout, decide)}
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for i := range cases {
			// A canceled batch stops dispatching. Sending the remaining cases
			// with a canceled context would record near-zero cancellation
			// failures and could make an interrupted batch look complete.
			if ctx.Err() != nil {
				return
			}
			select {
			case jobs <- i:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()
	var got []result
	for r := range results {
		got = append(got, r)
		if onAttempt != nil {
			onAttempt(r.a)
		}
	}
	sort.Slice(got, func(i, j int) bool { return got[i].idx < got[j].idx })
	attempts := make([]ABAttempt, 0, len(got))
	for _, r := range got {
		attempts = append(attempts, r.a)
	}
	return attempts, time.Since(start)
}

func runOneAB(parent context.Context, model, split string, concurrency int, s Scenario, timeout time.Duration, decide ABDecideFunc) ABAttempt {
	ctx := parent
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(parent, timeout)
		defer cancel()
	}
	start := time.Now()
	obs := decide(ctx, s)
	elapsed := time.Since(start)

	a := ABAttempt{
		CaseID:                     s.ID,
		Split:                      split,
		Category:                   s.Category,
		Model:                      model,
		Concurrency:                concurrency,
		Attempt:                    1,
		ExpectedAction:             string(s.Expected.Action),
		SchemaValid:                obs.SchemaValid,
		ElapsedMS:                  elapsed.Milliseconds(),
		ErrorDetail:                obs.ErrorDetail,
		InputTokens:                obs.InputTokens,
		OutputTokens:               obs.OutputTokens,
		TotalTokens:                obs.TotalTokens,
		ReasoningTokens:            obs.ReasoningTokens,
		CachedTokens:               obs.CachedTokens,
		TimeToFirstTokenMS:         obs.TimeToFirstTokenMS,
		TimeToFirstAnswerContentMS: obs.TimeToFirstAnswerContentMS,
		DecodeTokensPerSecond:      obs.DecodeTokensPerSecond,
		ModelLoadDurationMS:        obs.ModelLoadDurationMS,
	}
	if obs.SchemaValid {
		actual := obs.ActualAction
		a.ActualAction = &actual
		a.Correct = actual == string(s.Expected.Action)
	}
	if obs.ErrorCategory != "" {
		cat := obs.ErrorCategory
		a.ErrorCategory = &cat
	} else if !obs.SchemaValid {
		// The decider is expected to categorize its own failure, but a context
		// deadline is authoritative even if the transport did not label it.
		cat := "provider_error"
		if ctx.Err() == context.DeadlineExceeded {
			cat = "timeout"
		}
		a.ErrorCategory = &cat
	}
	return a
}

// ABModelInfo is one model advertised by the served endpoint.
type ABModelInfo struct {
	ID          string `json:"id"`
	OwnedBy     string `json:"owned_by,omitempty"`
	ModelType   string `json:"model_type,omitempty"`
	MaxModelLen int    `json:"max_model_len,omitempty"`
	Created     int64  `json:"created,omitempty"`
}

// ABEndpointInfo is the observed endpoint state. Health is the raw /health body
// when the endpoint exposes one; OMLX does not expose a version or model revision
// through these routes, so callers report those as null rather than guessing.
type ABEndpointInfo struct {
	Endpoint string          `json:"endpoint"`
	Server   string          `json:"server,omitempty"`
	Models   []ABModelInfo   `json:"models"`
	Health   json.RawMessage `json:"health,omitempty"`
	Error    string          `json:"error,omitempty"`
}

// DiscoverABEndpoint reads the served model ids and the health payload. It never
// performs an inference request.
func DiscoverABEndpoint(ctx context.Context, endpoint string, timeout time.Duration) ABEndpointInfo {
	base := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	info := ABEndpointInfo{Endpoint: base}
	client := &http.Client{Timeout: timeout}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/v1/models", nil)
	if err != nil {
		info.Error = err.Error()
		return info
	}
	resp, err := client.Do(req)
	if err != nil {
		info.Error = err.Error()
		return info
	}
	defer resp.Body.Close()
	info.Server = resp.Header.Get("Server")
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		info.Error = fmt.Sprintf("GET /v1/models: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		return info
	}
	var listing struct {
		Data []ABModelInfo `json:"data"`
	}
	if err := json.Unmarshal(body, &listing); err != nil {
		info.Error = fmt.Sprintf("decode /v1/models: %v", err)
		return info
	}
	info.Models = listing.Data

	if hreq, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/health", nil); err == nil {
		if hresp, err := client.Do(hreq); err == nil {
			hbody, _ := io.ReadAll(io.LimitReader(hresp.Body, 1<<20))
			hresp.Body.Close()
			if hresp.StatusCode == http.StatusOK && json.Valid(hbody) {
				info.Health = json.RawMessage(hbody)
			}
		}
	}
	return info
}

// ModelIDs returns the advertised model ids.
func (e ABEndpointInfo) ModelIDs() []string {
	ids := make([]string, 0, len(e.Models))
	for _, m := range e.Models {
		ids = append(ids, m.ID)
	}
	sort.Strings(ids)
	return ids
}

// HasModel reports whether id is advertised.
func (e ABEndpointInfo) HasModel(id string) bool {
	for _, m := range e.Models {
		if m.ID == id {
			return true
		}
	}
	return false
}

// ABModelStatus records whether a model could be reached for measurement.
type ABModelStatus struct {
	ID         string `json:"id"`
	Available  bool   `json:"available"`
	Error      string `json:"error,omitempty"`
	ColdLoadMS *int64 `json:"cold_load_ms"`
	WarmupMS   *int64 `json:"warmup_ms"`
}

// ABRunMeta is the machine-readable description of one benchmark run.
type ABRunMeta struct {
	Mode     string         `json:"mode"`
	Split    string         `json:"split"`
	Endpoint ABEndpointInfo `json:"endpoint"`

	CorpusSHA256 string   `json:"corpus_sha256"`
	PromptSHA256 string   `json:"prompt_sha256"`
	Prompt       ABPrompt `json:"prompt"`

	ModelsRequested []string        `json:"models_requested"`
	ModelStatus     []ABModelStatus `json:"model_status"`
	Concurrency     []int           `json:"concurrency"`
	TimeoutMS       int64           `json:"timeout_ms"`
	MaxTokens       *int            `json:"max_tokens"`
	CaseOrder       []string        `json:"case_order"`

	StartedAt  string `json:"started_at"`
	FinishedAt string `json:"finished_at"`

	// Observable settings. oMLX exposes no version or model revision through
	// /v1/models or /health, so these stay null.
	OMLXVersion      *string            `json:"omlx_version"`
	ModelRevisions   map[string]*string `json:"model_revisions"`
	ReasoningSetting string             `json:"reasoning_setting"`
	SamplingSetting  string             `json:"sampling_setting"`
	CachePolicy      string             `json:"cache_policy"`
	Notes            []string           `json:"notes,omitempty"`
}

// ABRunSummary is the comparison artifact for one mode/split.
type ABRunSummary struct {
	Mode              string           `json:"mode"`
	Split             string           `json:"split"`
	CorpusSHA256      string           `json:"corpus_sha256"`
	PromptSHA256      string           `json:"prompt_sha256"`
	ModelsRequested   []string         `json:"models_requested"`
	ModelsUnavailable []string         `json:"models_unavailable"`
	Batches           []ABBatchSummary `json:"batches"`
	Disagreements     []ABDisagreement `json:"disagreements"`

	OMLXVersion      *string            `json:"omlx_version"`
	ModelRevisions   map[string]*string `json:"model_revisions"`
	ReasoningSetting string             `json:"reasoning_setting"`
	SamplingSetting  string             `json:"sampling_setting"`
	CachePolicy      string             `json:"cache_policy"`

	Notes []string `json:"notes,omitempty"`
}

// RenderABReport renders the operator-facing Markdown report from a summary.
func RenderABReport(s ABRunSummary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Decision-model A/B — %s (%s)\n\n", s.Split, s.Mode)
	fmt.Fprintf(&b, "- Corpus: `%s`\n", s.CorpusSHA256)
	fmt.Fprintf(&b, "- Prompt: `%s`\n", s.PromptSHA256)
	fmt.Fprintf(&b, "- Models requested: `%s`\n", strings.Join(s.ModelsRequested, "`, `"))
	if len(s.ModelsUnavailable) > 0 {
		fmt.Fprintf(&b, "- **Models unavailable:** `%s`\n", strings.Join(s.ModelsUnavailable, "`, `"))
	}
	b.WriteString("\n")

	b.WriteString("## Observable settings\n\n")
	b.WriteString("- oMLX version: " + observability(s.OMLXVersion) + "\n")
	b.WriteString("- Model revisions: " + revisionText(s.ModelRevisions) + "\n")
	b.WriteString("- Reasoning: " + s.ReasoningSetting + "\n")
	b.WriteString("- Sampling: " + s.SamplingSetting + "\n")
	b.WriteString("- Cache: " + s.CachePolicy + "\n\n")

	b.WriteString("## Cold load and warm-up (outside the timed batches)\n\n")
	b.WriteString("| Model | Cold load ms | Warm-up ms |\n| --- | ---: | ---: |\n")
	seenModel := map[string]bool{}
	for _, x := range s.Batches {
		if seenModel[x.Model] {
			continue
		}
		seenModel[x.Model] = true
		fmt.Fprintf(&b, "| `%s` | %s | %s |\n", x.Model, msText(x.ColdLoadMS), msText(x.WarmupMS))
	}
	b.WriteString("\n")

	b.WriteString("## First-pass correctness and latency\n\n")
	b.WriteString("| Model | Conc | Attempted | Schema-valid | Correct | Invalid | Wrong | Timeout | Other errors | Median ms | Slowest ms (case) | Correct/min | Complete |\n")
	b.WriteString("| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- | ---: | --- |\n")
	for _, x := range s.Batches {
		fmt.Fprintf(&b, "| `%s` | %d | %s | %s | %s | %d | %d | %d | %d | %.0f | %d (`%s`) | %.2f | %v |\n",
			x.Model, x.Concurrency,
			ratio(x.Attempted, x.Attempted), ratio(x.SchemaValid, x.Attempted), ratio(x.Correct, x.Attempted),
			x.InvalidResponses, x.WrongActions, x.Timeouts, x.OtherErrors,
			x.LatencyMedianMS, x.LatencySlowestMS, x.SlowestCase, x.CorrectPerMinute, x.Complete)
	}
	b.WriteString("\n")
	b.WriteString("No fallback was used in any batch (`fallbacks = 0`): the decider calls the production provider directly.\n\n")

	b.WriteString("## Per-action correctness\n\n")
	b.WriteString("| Model | Conc | Action | Attempted | Correct | Schema-valid | Wrong | Invalid | Timeout | Other |\n")
	b.WriteString("| --- | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, x := range s.Batches {
		for _, a := range ABChoices {
			st := x.ByAction[a]
			if st.Attempted == 0 {
				continue
			}
			fmt.Fprintf(&b, "| `%s` | %d | %s | %d | %s | %d | %d | %d | %d | %d |\n",
				x.Model, x.Concurrency, a, st.Attempted, ratio(st.Correct, st.Attempted),
				st.SchemaValid, st.Wrong, st.Invalid, st.Timeout, st.OtherError)
		}
	}
	b.WriteString("\n")

	b.WriteString("## Consequential disagreements (expected BLOCK or NEEDS_HUMAN)\n\n")
	if len(s.Disagreements) == 0 {
		b.WriteString("None: every first attempt on a BLOCK/NEEDS_HUMAN case selected the expected action.\n\n")
	} else {
		b.WriteString("| Case | Model | Conc | Expected | Selected | Schema-valid | Error |\n")
		b.WriteString("| --- | --- | ---: | --- | --- | --- | --- |\n")
		for _, d := range s.Disagreements {
			sel := "`<none>`"
			if d.SelectedAction != nil {
				sel = "`" + *d.SelectedAction + "`"
			}
			errCat := ""
			if d.ErrorCategory != nil {
				errCat = *d.ErrorCategory
			}
			fmt.Fprintf(&b, "| `%s` | `%s` | %d | %s | %s | %v | %s |\n",
				d.CaseID, d.Model, d.Concurrency, d.ExpectedAction, sel, d.SchemaValid, errCat)
		}
		b.WriteString("\n")
	}

	b.WriteString("## Measurement notes\n\n")
	b.WriteString("- One attempt per case. The decider calls the production clef provider directly and never retries or falls back; `fallbacks` is always 0.\n")
	b.WriteString("- `elapsed_ms` is end-to-end around the attempt. `wall_ms` is the batch wall time used for correct/min.\n")
	b.WriteString("- `attempts-*.jsonl` records each attempt as it completes; `summary-*.json` aggregates the corpus case order.\n")
	b.WriteString("- Cold-load and warm-up requests are measured outside the timed batches (see `run-*.json`).\n")
	b.WriteString("- The production SystemOne response reports input/output tokens only; time-to-first-answer-content, decode speed, and model-load duration are null.\n")
	b.WriteString("- A field the endpoint does not report is null, never a measured zero.\n")
	for _, n := range s.Notes {
		fmt.Fprintf(&b, "- %s\n", n)
	}
	return b.String()
}

func observability(v *string) string {
	if v == nil {
		return "not exposed by the endpoint (null)"
	}
	return "`" + *v + "`"
}

func revisionText(m map[string]*string) string {
	if len(m) == 0 {
		return "not exposed by the endpoint (null)"
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		v := m[k]
		if b.Len() > 0 {
			b.WriteString(", ")
		}
		if v == nil {
			fmt.Fprintf(&b, "`%s` = not exposed", k)
		} else {
			fmt.Fprintf(&b, "`%s` = `%s`", k, *v)
		}
	}
	return b.String()
}

func msText(v *int64) string {
	if v == nil {
		return "not measured"
	}
	return fmt.Sprintf("%d", *v)
}

func ratio(n, d int) string {
	if d == 0 {
		return "0/0 (n/a)"
	}
	return fmt.Sprintf("%d/%d (%.1f%%)", n, d, 100*float64(n)/float64(d))
}
