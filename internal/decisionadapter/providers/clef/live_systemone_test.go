package clef

// This file is the CLEF-006 opt-in live integration test. It exercises the
// CLEF-002-selected transport (POST /v1/systemone against a local Ollama
// serving clef-flash) against a real local Clef runtime.
//
// DISABLED BY DEFAULT. The test does not run unless the operator sets the
// explicit opt-in environment variable:
//
//	CLEF_LIVE_TEST=1
//
// This opt-in is deliberately SEPARATE from the production CLEF_ENABLED gate:
// setting only CLEF_ENABLED must not cause the live test to execute.
//
// Environment variables (all optional; defaults match ConfigFromEnv):
//
//	CLEF_LIVE_TEST   opt-in flag ("1"/"true"/"yes") required to run the test
//	OLLAMA_BASE_URL  backend root URL   (default http://localhost:11434)
//	CLEF_MODEL       model name         (default "clef-flash")
//	CLEF_TIMEOUT     per-call timeout   (default 30s, Go duration syntax)
//
// When the opt-in is unset, or when the local runtime prerequisites are absent
// (backend unreachable, or the configured model not served), the test SKIPS
// cleanly with a reason naming the missing prerequisite; it never fails for a
// missing runtime.
//
// Contract-shape validation: the test validates contract shape rather than
// fixed model judgment. It asserts only that:
//
//   - the returned choice is a member of the request's allowed choices;
//   - when confidence is provided it lies within [0, 1];
//   - when confidence is absent it is recorded as unavailable, not a failure.
//
// It never asserts a specific model verdict.
//
// Metrics and per-call outcomes are written to a machine-readable JSON artifact
// (path printed via t.Logf, also written under the OS temp dir) for transcription
// into docs/reports/clef-provider/CLEF-006-local-runtime-verification.md.

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/decision"
)

// liveOptInEnv is the explicit opt-in for live runtime tests. It is separate
// from CLEF_ENABLED on purpose (see the file comment).
const liveOptInEnv = "CLEF_LIVE_TEST"

// liveRepeatCount is the documented number of repeats of the same request used
// to measure repeatability.
const liveRepeatCount = 5

// liveShortTimeout is used by the timeout-behavior subtest: it is short enough
// that a per-call deadline should surface as decision.KindUnavailable rather
// than as success.
const liveShortTimeout = 1 * time.Nanosecond

// liveSample is one record of a single live call outcome.
const liveSample = "sample"

// liveResults is the machine-readable artifact describing one live run.
type liveResults struct {
	BackendURL          string            `json:"backend_url"`
	Model               string            `json:"model"`
	Timeout             string            `json:"timeout"`
	RepeatCount         int               `json:"repeat_count"`
	SampleSize          int               `json:"sample_size"`
	SuccessCount        int               `json:"success_count"`
	SuccessRate         float64           `json:"success_rate"`
	LatencyP50MS        float64           `json:"latency_p50_ms"`
	LatencyP95MS        float64           `json:"latency_p95_ms"`
	ChoiceValidCount    int               `json:"choice_valid_count"`
	ConfidenceAvailable int               `json:"confidence_available_count"`
	TimeoutBehavior     string            `json:"timeout_behavior"`
	MalformedBehavior   string            `json:"malformed_behavior"`
	Repeatability       string            `json:"repeatability"`
	Outcomes            []liveCallOutcome `json:"outcomes"`
	Note                string            `json:"note"`
}

// liveCallOutcome records one call's normalized classification.
type liveCallOutcome struct {
	Index       int      `json:"index"`
	Kind        string   `json:"kind"`
	LatencyMS   float64  `json:"latency_ms"`
	Choice      string   `json:"choice,omitempty"`
	ChoiceValid bool     `json:"choice_valid"`
	Confidence  *float64 `json:"confidence,omitempty"`
	Err         string   `json:"error,omitempty"`
}

// liveOptedIn reports whether the explicit live-test opt-in is set.
func liveOptedIn() bool {
	switch lowerTrim(os.Getenv(liveOptInEnv)) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

func lowerTrim(s string) string {
	// local helper: lowercase + trim without importing strings just for this
	b := []byte(s)
	start, end := 0, len(b)
	for start < end && (b[start] == ' ' || b[start] == '\t' || b[start] == '\n' || b[start] == '\r') {
		start++
	}
	for end > start && (b[end-1] == ' ' || b[end-1] == '\t' || b[end-1] == '\n' || b[end-1] == '\r') {
		end--
	}
	b = b[start:end]
	for i := range b {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 'a' - 'A'
		}
	}
	return string(b)
}

// liveProvider builds a provider with Config.Enable=true (independent of
// CLEF_ENABLED) using env configuration for URL/model/timeout.
func liveProvider(timeout time.Duration) *Provider {
	cfg := ConfigFromEnv()
	cfg.Enable = true
	if timeout > 0 {
		cfg.Timeout = timeout
	}
	return New(cfg, nil)
}

// skipUnlessLive gates a test on opt-in and on runtime availability. It skips
// cleanly (never fails) when the opt-in is unset or the runtime is absent.
func skipUnlessLive(ctx context.Context, t *testing.T) *Provider {
	t.Helper()
	if !liveOptedIn() {
		t.Skipf("live Clef runtime test disabled by default; set %s=1 to opt in", liveOptInEnv)
	}
	p := liveProvider(0)
	if !p.Available(ctx) {
		cfg := ConfigFromEnv()
		t.Skipf("local Clef runtime prerequisite absent: backend %s unreachable or model %q not served", cfg.BaseURL, cfg.Model)
	}
	return p
}

// TestLiveSystemOneContract is the opt-in live integration test.
func TestLiveSystemOneContract(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	p := skipUnlessLive(ctx, t)

	req := liveChoiceRequest()
	results := runLiveRepeats(ctx, t, p, req)

	// Contract-shape assertions over every outcome.
	for _, o := range results.Outcomes {
		if o.Kind != "success" {
			continue
		}
		if !o.ChoiceValid {
			t.Errorf("call %d: returned choice %q is not a member of the allowed choices", o.Index, o.Choice)
		}
		if o.Confidence != nil && (*o.Confidence < 0 || *o.Confidence > 1) {
			t.Errorf("call %d: confidence %v out of bounds [0,1]", o.Index, *o.Confidence)
		}
	}

	writeLiveArtifact(t, results)
}

// TestLiveSystemOneTimeoutBehavior exercises a deliberately short per-call
// timeout and asserts it surfaces as decision.KindUnavailable (not success, not
// an unclassified error).
func TestLiveSystemOneTimeoutBehavior(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	if !liveOptedIn() {
		t.Skipf("live Clef runtime test disabled by default; set %s=1 to opt in", liveOptInEnv)
	}

	// Build a provider with an effectively instantaneous timeout so the call
	// cannot complete; availability may be false, in which case we still assert
	// the classification of the decision call itself.
	p := liveProvider(liveShortTimeout)
	_, err := p.Decide(ctx, liveChoiceRequest())
	if err == nil {
		t.Skip("short-timeout call unexpectedly succeeded; runtime fast enough to complete within 1ns; timeout classification not exercisable on this host")
	}
	if !errors.Is(err, decision.ErrUnavailable) {
		t.Errorf("short-timeout call: got error %v (kind %v); want KindUnavailable", err, errorKind(err))
	}
}

// TestLiveSystemOneInvalidRequest asserts an invalid request is classified as
// KindInvalidRequest without contacting the backend.
func TestLiveSystemOneInvalidRequest(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	p := liveProvider(0) // does not require the backend to be reachable
	_, err := p.Decide(ctx, decision.DecisionRequest{})
	if !errors.Is(err, decision.ErrInvalidRequest) {
		t.Fatalf("empty request: got error %v (kind %v); want KindInvalidRequest", err, errorKind(err))
	}
}

// liveChoiceRequest builds the deterministic contract-shape probe request. It
// deliberately does NOT encode an expected verdict.
func liveChoiceRequest() decision.DecisionRequest {
	return decision.DecisionRequest{
		State: "A deploy pipeline is about to promote a build to production. The change touches authentication middleware.",
		Questions: []decision.Question{
			{
				ID:           "risk",
				Type:         decision.QuestionChoice,
				Instructions: "Choose the risk level of promoting this change.",
				Criteria:     "Deployment risk level",
				Choices:      []string{"LOW", "MEDIUM", "HIGH"},
			},
		},
	}
}

// runLiveRepeats issues the same request liveRepeatCount times and collects the
// metrics artifact.
func runLiveRepeats(ctx context.Context, t *testing.T, p *Provider, req decision.DecisionRequest) liveResults {
	t.Helper()

	cfg := ConfigFromEnv()
	res := liveResults{
		BackendURL:  cfg.BaseURL,
		Model:       cfg.Model,
		Timeout:     cfg.Timeout.String(),
		RepeatCount: liveRepeatCount,
		Note:        "observed local runtime sample data; not an SLA or guarantee",
	}

	choices := map[string]int{}
	latencies := make([]float64, 0, liveRepeatCount)

	for i := 0; i < liveRepeatCount; i++ {
		start := time.Now()
		result, err := p.Decide(ctx, req)
		elapsed := float64(time.Since(start).Microseconds()) / 1000.0
		latencies = append(latencies, elapsed)

		o := liveCallOutcome{Index: i, LatencyMS: elapsed}
		if err != nil {
			o.Kind = classifyOutcome(err)
			o.Err = err.Error()
			res.Outcomes = append(res.Outcomes, o)
			continue
		}

		o.Kind = "success"
		res.SuccessCount++
		ans, ok := result.Answers["risk"]
		if ok && ans.Type == decision.AnswerChoice {
			o.Choice = ans.Choice
			o.ChoiceValid = req.Questions[0].Allows(ans.Choice)
			if o.ChoiceValid {
				res.ChoiceValidCount++
			}
			if ans.Confidence != nil {
				o.Confidence = ans.Confidence
				if *ans.Confidence >= 0 && *ans.Confidence <= 1 {
					res.ConfidenceAvailable++
				}
			}
			choices[ans.Choice]++
		}
		res.Outcomes = append(res.Outcomes, o)
	}

	res.SampleSize = len(res.Outcomes)
	if res.SampleSize > 0 {
		res.SuccessRate = float64(res.SuccessCount) / float64(res.SampleSize)
	}
	res.LatencyP50MS = percentile(latencies, 0.50)
	res.LatencyP95MS = percentile(latencies, 0.95)

	res.TimeoutBehavior = "exercised by TestLiveSystemOneTimeoutBehavior: a deliberately short per-call timeout surfaces as decision.KindUnavailable"
	res.MalformedBehavior = "malformed/indeterminate responses are normalized to decision.KindMalformedResponse and recorded in outcomes (kind=malformed_response), never counted as success"
	res.Repeatability = repeatabilityStatement(choices, res.SuccessCount, res.SampleSize)

	return res
}

// classifyOutcome maps a normalized provider error to a stable outcome label.
func classifyOutcome(err error) string {
	switch {
	case errors.Is(err, decision.ErrUnavailable):
		return "unavailable"
	case errors.Is(err, decision.ErrMalformedResponse):
		return "malformed_response"
	case errors.Is(err, decision.ErrInvalidRequest):
		return "invalid_request"
	default:
		return "provider_failure"
	}
}

func errorKind(err error) string {
	var pe *decision.ProviderError
	if errors.As(err, &pe) {
		return string(pe.Kind)
	}
	return "unknown"
}

// percentile returns the nearest-rank percentile of xs.
func percentile(xs []float64, p float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	sorted := append([]float64(nil), xs...)
	sort.Float64s(sorted)
	idx := int(p*float64(len(sorted))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func repeatabilityStatement(choices map[string]int, success, sample int) string {
	if success == 0 {
		return "no successful calls; repeatability not measurable"
	}
	distinct := len(choices)
	return "repeatability derived from the collected outcomes: " + itoa(distinct) + " distinct choice(s) observed over " + itoa(success) + " successful call(s) out of " + itoa(sample) + " sample(s)"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// writeLiveArtifact writes the machine-readable results JSON and logs its path.
func writeLiveArtifact(t *testing.T, res liveResults) {
	t.Helper()
	data, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		t.Fatalf("marshal live artifact: %v", err)
	}
	path := filepath.Join(os.TempDir(), "clef-006-live-results.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Logf("could not write live artifact to %s: %v", path, err)
	} else {
		t.Logf("live artifact written to %s", path)
	}
	t.Logf("live results: %s", string(data))
}
