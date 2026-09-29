package jev

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/ollamaagent"
	"github.com/imhttran/agentic-sop/internal/review"
)

// fakeChat is a deterministic in-process ChatProvider. It records the model and
// prompt it received and returns canned output or an error, so adapter tests
// need no live Ollama and no network access.
type fakeChat struct {
	output string
	err    error
	block  bool
	model  string
	prompt string
	schema json.RawMessage
	calls  int
}

func (f *fakeChat) Chat(ctx context.Context, model, prompt string) (string, error) {
	return f.respond(ctx, model, prompt, nil)
}

func (f *fakeChat) ChatStructured(ctx context.Context, model, prompt string, schema json.RawMessage) (string, error) {
	return f.respond(ctx, model, prompt, schema)
}

// respond is the shared body of Chat and ChatStructured: it records the request
// and returns the canned output or error.
func (f *fakeChat) respond(ctx context.Context, model, prompt string, schema json.RawMessage) (string, error) {
	f.calls++
	f.model = model
	f.prompt = prompt
	f.schema = schema
	if f.block {
		<-ctx.Done()
		return "", ctx.Err()
	}
	return f.output, f.err
}

func testConfig() ollamaagent.Config {
	return ollamaagent.Config{
		BaseURL: "http://127.0.0.1:11434",
		Model:   "test-model:latest",
		Timeout: 2 * time.Second,
	}
}

// TestAdapterSatisfiesAnalyzer asserts the adapter conforms to jev.Analyzer and
// records the bounded Request it was given.
func TestAdapterSatisfiesAnalyzer(t *testing.T) {
	var _ Analyzer = &OllamaAnalyzer{}

	chat := &fakeChat{output: `{"status":"PASS","summary":"ok"}`}
	a, err := NewOllamaAnalyzer(testConfig(), chat)
	if err != nil {
		t.Fatalf("NewOllamaAnalyzer: %v", err)
	}
	req := Request{Task: "t", Criteria: "c", ChangedFiles: []string{"a.go"}, RepositoryContext: "diff"}
	res, err := a.Analyze(context.Background(), req)
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if res.Status != StatusPass {
		t.Fatalf("status = %q, want PASS", res.Status)
	}
	if !strings.Contains(chat.prompt, "t") || !strings.Contains(chat.prompt, "a.go") {
		t.Fatalf("prompt missing bounded context: %q", chat.prompt)
	}
}

// TestModelIsConfigurable asserts the configured model is observably passed into
// the reused provider path, and that no model is a required hardcoded value.
func TestModelIsConfigurable(t *testing.T) {
	cfg := testConfig()
	cfg.Model = "configured-model:7b"
	chat := &fakeChat{output: `{"status":"PASS"}`}
	a, err := NewOllamaAnalyzer(cfg, chat)
	if err != nil {
		t.Fatalf("NewOllamaAnalyzer: %v", err)
	}
	if _, err := a.Analyze(context.Background(), Request{Task: "t"}); err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if chat.model != "configured-model:7b" {
		t.Fatalf("model = %q, want configured-model:7b", chat.model)
	}
}

// TestConfigFromEnvSuppliesModel asserts the reused ConfigFromEnv path supplies
// the model from SOP_OLLAMA_MODEL, so no model is hardcoded.
func TestConfigFromEnvSuppliesModel(t *testing.T) {
	t.Setenv("SOP_OLLAMA_MODEL", "env-model:1b")
	t.Setenv("SOP_OLLAMA_BASE_URL", "http://example.invalid:11434")
	cfg, err := ollamaagent.ConfigFromEnv()
	if err != nil {
		t.Fatalf("ConfigFromEnv: %v", err)
	}
	if cfg.Model != "env-model:1b" {
		t.Fatalf("model = %q, want env-model:1b", cfg.Model)
	}
	chat := &fakeChat{output: `{"status":"PASS"}`}
	a, err := NewOllamaAnalyzer(cfg, chat)
	if err != nil {
		t.Fatalf("NewOllamaAnalyzer: %v", err)
	}
	if _, err := a.Analyze(context.Background(), Request{Task: "t"}); err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if chat.model != "env-model:1b" {
		t.Fatalf("model = %q, want env-model:1b", chat.model)
	}
}

// TestMissingModelIsFocusedConfigError asserts a missing model fails as a
// focused configuration error, never as a pass.
func TestMissingModelIsFocusedConfigError(t *testing.T) {
	cfg := testConfig()
	cfg.Model = ""
	_, err := NewOllamaAnalyzer(cfg, &fakeChat{})
	assertKind(t, err, ErrConfigInvalid)
}

// TestBuildPromptIncludesAllFields asserts every bounded Request field is
// represented in the mapped input.
func TestBuildPromptIncludesAllFields(t *testing.T) {
	a := &OllamaAnalyzer{cfg: testConfig()}
	req := Request{
		Task:              "the task",
		Criteria:          "the criteria",
		ChangedFiles:      []string{"x.go", "y.go"},
		RepositoryContext: "the repo context",
		ValidationResult:  "the validation",
		ReviewResult:      "the review",
	}
	p := a.buildPrompt(req)
	for _, want := range []string{"the task", "the criteria", "x.go", "y.go", "the repo context", "the validation", "the review"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing %q", want)
		}
	}
}

// TestBuildPromptIsBounded asserts oversized input is truncated, not forwarded
// unbounded.
func TestBuildPromptIsBounded(t *testing.T) {
	a := &OllamaAnalyzer{cfg: testConfig()}
	huge := strings.Repeat("x", maxPromptRunes*2)
	p := a.buildPrompt(Request{Task: huge})
	if len([]rune(p)) > maxPromptRunes+64 {
		t.Fatalf("prompt not bounded: %d runes", len([]rune(p)))
	}
	if !strings.Contains(p, "[truncated]") {
		t.Fatal("expected explicit truncation marker")
	}
}

// TestContextCancellationIsBounded asserts a cancelled context terminates the
// call without hanging.
func TestContextCancellationIsBounded(t *testing.T) {
	chat := &fakeChat{block: true}
	a, err := NewOllamaAnalyzer(testConfig(), chat)
	if err != nil {
		t.Fatalf("NewOllamaAnalyzer: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.Analyze(ctx, Request{Task: "t"}); err == nil {
		t.Fatal("expected error on cancelled context")
	}
}

// TestProviderTimeoutIsFocusedError asserts an unresponsive provider yields a
// focused timeout error rather than hanging.
func TestProviderTimeoutIsFocusedError(t *testing.T) {
	cfg := testConfig()
	cfg.Timeout = 50 * time.Millisecond
	chat := &fakeChat{block: true}
	a, err := NewOllamaAnalyzer(cfg, chat)
	if err != nil {
		t.Fatalf("NewOllamaAnalyzer: %v", err)
	}
	_, err = a.Analyze(context.Background(), Request{Task: "t"})
	assertKind(t, err, ErrTimeout)
}

// TestProviderFailureIsFocusedError asserts a provider failure surfaces as a
// focused provider error and never a PASS result.
func TestProviderFailureIsFocusedError(t *testing.T) {
	chat := &fakeChat{err: errors.New("connection refused")}
	a, err := NewOllamaAnalyzer(testConfig(), chat)
	if err != nil {
		t.Fatalf("NewOllamaAnalyzer: %v", err)
	}
	res, err := a.Analyze(context.Background(), Request{Task: "t"})
	assertKind(t, err, ErrProviderUnreachable)
	if res.Status == StatusPass {
		t.Fatal("provider failure must not yield PASS")
	}
}

// TestMalformedOutputIsFocusedError asserts malformed output fails closed and
// never yields PASS.
func TestMalformedOutputIsFocusedError(t *testing.T) {
	for _, out := range []string{"not json", "", `{"status":"NOPE"}`, `{"status":"PASS","findings":[{"severity":"HIGH"}]}`} {
		chat := &fakeChat{output: out}
		a, err := NewOllamaAnalyzer(testConfig(), chat)
		if err != nil {
			t.Fatalf("NewOllamaAnalyzer: %v", err)
		}
		res, err := a.Analyze(context.Background(), Request{Task: "t"})
		if err == nil {
			t.Fatalf("output %q: expected error", out)
		}
		if res.Status == StatusPass {
			t.Fatalf("output %q: must not yield PASS", out)
		}
	}
}

// TestFindingsResult asserts a FINDINGS result carries valid severities.
func TestFindingsResult(t *testing.T) {
	out, _ := json.Marshal(modelOutput{
		Status:  "FINDINGS",
		Summary: "issues",
		Findings: []modelFinding{{
			ID: "F1", Severity: "HIGH", Category: "quality", Message: "issue",
		}},
	})
	chat := &fakeChat{output: string(out)}
	a, err := NewOllamaAnalyzer(testConfig(), chat)
	if err != nil {
		t.Fatalf("NewOllamaAnalyzer: %v", err)
	}
	res, err := a.Analyze(context.Background(), Request{Task: "t"})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if res.Status != StatusFindings || len(res.Findings) != 1 {
		t.Fatalf("unexpected result: %+v", res)
	}
	if !res.Findings[0].Severity.Valid() || res.Findings[0].Severity != review.High {
		t.Fatalf("invalid severity: %v", res.Findings[0].Severity)
	}
}

// TestOversizedOutputIsBounded asserts model output beyond the bound is bounded,
// not retained unbounded.
func TestOversizedOutputIsBounded(t *testing.T) {
	// A structurally valid PASS document padded far beyond the output bound: the
	// truncation marker makes it malformed, so it fails closed rather than being
	// retained unbounded.
	pad := strings.Repeat("x", maxOutputRunes*2)
	chat := &fakeChat{output: `{"status":"PASS","summary":"` + pad + `"}`}
	a, err := NewOllamaAnalyzer(testConfig(), chat)
	if err != nil {
		t.Fatalf("NewOllamaAnalyzer: %v", err)
	}
	res, err := a.Analyze(context.Background(), Request{Task: "t"})
	if err == nil && len(res.Summary) > maxOutputRunes {
		t.Fatal("output retained unbounded")
	}
}

// TestAnalyzerThroughInterface drives the adapter through jev.Analyzer exactly
// as SOP invokes it.
func TestAnalyzerThroughInterface(t *testing.T) {
	chat := &fakeChat{output: `{"status":"INCOMPLETE"}`}
	a, err := NewOllamaAnalyzer(testConfig(), chat)
	if err != nil {
		t.Fatalf("NewOllamaAnalyzer: %v", err)
	}
	var analyzer Analyzer = a
	res, err := analyzer.Analyze(context.Background(), Request{Task: "t", Criteria: "c"})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if res.Status != StatusIncomplete {
		t.Fatalf("status = %q, want INCOMPLETE", res.Status)
	}
	if chat.calls != 1 {
		t.Fatalf("calls = %d, want 1 (bounded execution)", chat.calls)
	}
}

// scriptedChat is a deterministic in-process ChatProvider that returns a fixed
// sequence of outputs, one per call (repeating the last), and records each prompt
// it received. It drives the adapter's bounded corrective retry with no live
// Ollama and no network access.
type scriptedChat struct {
	outputs []string
	prompts []string
	schemas []json.RawMessage
}

func (s *scriptedChat) Chat(_ context.Context, _ string, prompt string) (string, error) {
	return s.record(prompt, nil)
}

func (s *scriptedChat) ChatStructured(_ context.Context, _ string, prompt string, schema json.RawMessage) (string, error) {
	return s.record(prompt, schema)
}

// record appends the request and returns the next scripted output.
func (s *scriptedChat) record(prompt string, schema json.RawMessage) (string, error) {
	s.prompts = append(s.prompts, prompt)
	s.schemas = append(s.schemas, schema)
	i := len(s.prompts) - 1
	if i >= len(s.outputs) {
		i = len(s.outputs) - 1
	}
	return s.outputs[i], nil
}

// TestValidJSONFirstAttemptIsNotRetried asserts a well-formed first response is
// used as-is, with no corrective retry.
func TestValidJSONFirstAttemptIsNotRetried(t *testing.T) {
	chat := &scriptedChat{outputs: []string{`{"status":"PASS"}`}}
	a, err := NewOllamaAnalyzer(testConfig(), chat)
	if err != nil {
		t.Fatalf("NewOllamaAnalyzer: %v", err)
	}
	res, err := a.Analyze(context.Background(), Request{Task: "t"})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if res.Status != StatusPass {
		t.Fatalf("status = %q, want PASS", res.Status)
	}
	if len(chat.prompts) != 1 {
		t.Fatalf("attempts = %d, want 1", len(chat.prompts))
	}
}

// TestMarkdownBeforeJSONIsCorrectivelyRetried asserts markdown preceding
// otherwise valid JSON is corrected by a bounded retry, and that the retry
// prompt explicitly states the schema violation. SOP never interprets the prose
// itself.
func TestMarkdownBeforeJSONIsCorrectivelyRetried(t *testing.T) {
	chat := &scriptedChat{outputs: []string{
		"# Review\n\n" + `{"status":"PASS"}`,
		`{"status":"PASS"}`,
	}}
	a, err := NewOllamaAnalyzer(testConfig(), chat)
	if err != nil {
		t.Fatalf("NewOllamaAnalyzer: %v", err)
	}
	res, err := a.Analyze(context.Background(), Request{Task: "t"})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if res.Status != StatusPass {
		t.Fatalf("status = %q, want PASS", res.Status)
	}
	if len(chat.prompts) != 2 {
		t.Fatalf("attempts = %d, want 2 (initial + one corrective retry)", len(chat.prompts))
	}
	if !strings.Contains(chat.prompts[1], "violated the required JSON schema") {
		t.Errorf("corrective prompt missing the schema-violation instruction:\n%s", chat.prompts[1])
	}
}

// TestFencedJSONIsCorrectivelyRetried asserts a ```json code fence is treated as
// malformed output and corrected by a bounded retry.
func TestFencedJSONIsCorrectivelyRetried(t *testing.T) {
	chat := &scriptedChat{outputs: []string{
		"```json\n{\"status\":\"PASS\"}\n```",
		`{"status":"FINDINGS","findings":[{"severity":"HIGH","message":"x"}]}`,
	}}
	a, err := NewOllamaAnalyzer(testConfig(), chat)
	if err != nil {
		t.Fatalf("NewOllamaAnalyzer: %v", err)
	}
	res, err := a.Analyze(context.Background(), Request{Task: "t"})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if res.Status != StatusFindings {
		t.Fatalf("status = %q, want FINDINGS", res.Status)
	}
	if len(chat.prompts) != 2 {
		t.Fatalf("attempts = %d, want 2", len(chat.prompts))
	}
}

// TestMalformedEveryAttemptExhaustsCorrectiveRetries asserts the corrective retry
// is bounded: malformed output on every attempt fails closed with a
// MALFORMED_OUTPUT error that records the exhausted attempt count.
func TestMalformedEveryAttemptExhaustsCorrectiveRetries(t *testing.T) {
	chat := &scriptedChat{outputs: []string{"# nope", "# nope", "# nope", "# nope"}}
	a, err := NewOllamaAnalyzer(testConfig(), chat)
	if err != nil {
		t.Fatalf("NewOllamaAnalyzer: %v", err)
	}
	res, err := a.Analyze(context.Background(), Request{Task: "t"})
	assertKind(t, err, ErrMalformedOutput)
	if res.Status == StatusPass {
		t.Fatal("exhausted malformed output must not yield PASS")
	}
	if len(chat.prompts) != maxAnalysisAttempts {
		t.Fatalf("attempts = %d, want %d", len(chat.prompts), maxAnalysisAttempts)
	}
	if !strings.Contains(err.Error(), "after 3 attempts") {
		t.Errorf("error must record the exhausted attempt count: %v", err)
	}
}

// TestProviderErrorIsNotCorrectivelyRetried asserts a provider failure is handled
// by the provider-call boundary, not by the schema-corrective retry: it is
// returned immediately as a focused provider error.
func TestProviderErrorIsNotCorrectivelyRetried(t *testing.T) {
	chat := &fakeChat{err: errors.New("connection refused")}
	a, err := NewOllamaAnalyzer(testConfig(), chat)
	if err != nil {
		t.Fatalf("NewOllamaAnalyzer: %v", err)
	}
	_, err = a.Analyze(context.Background(), Request{Task: "t"})
	assertKind(t, err, ErrProviderUnreachable)
	if chat.calls != 1 {
		t.Fatalf("provider attempts = %d, want 1 (a provider failure is not corrective-retried)", chat.calls)
	}
}

// TestCanceledContextIsNotRetried asserts a canceled context is terminal: the
// adapter does not start another attempt.
func TestCanceledContextIsNotRetried(t *testing.T) {
	chat := &fakeChat{block: true}
	a, err := NewOllamaAnalyzer(testConfig(), chat)
	if err != nil {
		t.Fatalf("NewOllamaAnalyzer: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.Analyze(ctx, Request{Task: "t"}); err == nil {
		t.Fatal("expected error on cancelled context")
	}
	if chat.calls != 0 {
		t.Fatalf("attempts = %d, want 0 (a canceled context is terminal)", chat.calls)
	}
}

// TestPromptStatesJSONSchemaContract asserts the prompt explicitly states the
// required JSON shape and forbids markdown/prose, so the model is told the
// contract instead of being expected to infer it.
func TestPromptStatesJSONSchemaContract(t *testing.T) {
	a := &OllamaAnalyzer{cfg: testConfig()}
	p := a.buildPrompt(Request{Task: "t"})
	for _, want := range []string{
		"Return ONE JSON object",
		"no markdown",
		"\"status\":\"PASS|FINDINGS|INCOMPLETE|ERROR\"",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt missing %q:\n%s", want, p)
		}
	}
}

// TestStructuredRequestCarriesSchema asserts the adapter constrains every provider
// call to the JEV output schema, so Ollama enforces the output shape instead of
// the model inferring it from prose.
func TestStructuredRequestCarriesSchema(t *testing.T) {
	chat := &fakeChat{output: `{"status":"PASS"}`}
	a, err := NewOllamaAnalyzer(testConfig(), chat)
	if err != nil {
		t.Fatalf("NewOllamaAnalyzer: %v", err)
	}
	if _, err := a.Analyze(context.Background(), Request{Task: "t"}); err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if chat.schema == nil {
		t.Fatal("provider was not called with a schema")
	}
	if string(chat.schema) != string(jevOutputJSONSchema) {
		t.Errorf("schema = %s, want the JEV output schema", chat.schema)
	}
}

// TestJEVOutputSchemaIsValidJSONForTheParserContract asserts the schema shipped to
// the provider is valid JSON and constrains the same status/severity vocabulary
// the strict parser validates.
func TestJEVOutputSchemaIsValidJSONForTheParserContract(t *testing.T) {
	if !json.Valid(jevOutputJSONSchema) {
		t.Fatalf("schema is not valid JSON: %s", jevOutputJSONSchema)
	}
	for _, want := range []string{
		`"status"`, `"FINDINGS"`, `"severity"`, `"CRITICAL"`, `"findings"`,
	} {
		if !strings.Contains(string(jevOutputJSONSchema), want) {
			t.Errorf("schema missing %s: %s", want, jevOutputJSONSchema)
		}
	}
}

// TestMetricsReportFirstAttemptCall asserts a first-attempt success reports one
// JEV-level provider call.
func TestMetricsReportFirstAttemptCall(t *testing.T) {
	chat := &scriptedChat{outputs: []string{`{"status":"PASS"}`}}
	a, err := NewOllamaAnalyzer(testConfig(), chat)
	if err != nil {
		t.Fatalf("NewOllamaAnalyzer: %v", err)
	}
	if _, err := a.Analyze(context.Background(), Request{Task: "t"}); err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if got := a.JEVToolCalls(); got != 1 {
		t.Errorf("JEVToolCalls = %d, want 1", got)
	}
}

// TestMetricsReportCorrectiveRetryCalls asserts the metric reflects the real
// JEV-level provider cost when a corrective retry occurs.
func TestMetricsReportCorrectiveRetryCalls(t *testing.T) {
	chat := &scriptedChat{outputs: []string{
		"# Review\n\n" + `{"status":"PASS"}`,
		`{"status":"PASS"}`,
	}}
	a, err := NewOllamaAnalyzer(testConfig(), chat)
	if err != nil {
		t.Fatalf("NewOllamaAnalyzer: %v", err)
	}
	if _, err := a.Analyze(context.Background(), Request{Task: "t"}); err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if got := a.JEVToolCalls(); got != 2 {
		t.Errorf("JEVToolCalls = %d, want 2 (initial + one corrective retry)", got)
	}
}

// TestMetricsReportExhaustedCorrectiveCalls asserts an exhausted analysis reports
// the full provider-call budget it actually spent.
func TestMetricsReportExhaustedCorrectiveCalls(t *testing.T) {
	chat := &scriptedChat{outputs: []string{"# nope"}}
	a, err := NewOllamaAnalyzer(testConfig(), chat)
	if err != nil {
		t.Fatalf("NewOllamaAnalyzer: %v", err)
	}
	_, err = a.Analyze(context.Background(), Request{Task: "t"})
	assertKind(t, err, ErrMalformedOutput)
	if got := a.JEVToolCalls(); got != maxAnalysisAttempts {
		t.Errorf("JEVToolCalls = %d, want %d", got, maxAnalysisAttempts)
	}
}

// assertKind asserts err is a focused ProviderError of the given kind.
func assertKind(t *testing.T, err error, kind ProviderErrorKind) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error of kind %s, got nil", kind)
	}
	var pe *ProviderError
	if !errors.As(err, &pe) {
		t.Fatalf("error %v is not a *ProviderError", err)
	}
	if pe.Kind != kind {
		t.Fatalf("kind = %s, want %s", pe.Kind, kind)
	}
}
