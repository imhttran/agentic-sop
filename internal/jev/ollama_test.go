package jev

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/ollamaagent"
	"github.com/imhttran/agentic-sop/internal/provider"
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

// TestBuildPromptStatesEarlyPurpose pins that an early checkpoint tells the model
// nothing is implemented yet, and a post-implement review carries no such brief.
func TestBuildPromptStatesEarlyPurpose(t *testing.T) {
	a := &OllamaAnalyzer{cfg: testConfig()}
	for _, p := range []Purpose{PurposeTaskTriage, PurposePreExecution} {
		if got := a.buildPrompt(Request{Purpose: p, Task: "t"}); !strings.Contains(got, "Nothing has been implemented yet") {
			t.Errorf("%s prompt does not say nothing is implemented yet:\n%s", p, got)
		}
	}
	if got := a.buildPrompt(Request{Purpose: PurposeQuality, Task: "t"}); strings.Contains(got, "Purpose:") {
		t.Errorf("a quality review must not carry an early brief:\n%s", got)
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
			ID: "F1", Severity: "HIGH", Category: "scope", Message: "issue",
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

// TestOllamaAnalyzerEmitsStructuredEvidence asserts the production analyzer
// populates Result.StructuredEvidence from a well-formed model response, and that
// the evidence satisfies the versioned contract validator. It uses only the
// in-process fake chat provider: no live provider and no network.
func TestOllamaAnalyzerEmitsStructuredEvidence(t *testing.T) {
	out, err := json.Marshal(modelOutput{
		Status:  "FINDINGS",
		Summary: "found issues",
		Findings: []modelFinding{{
			ID:       "F1",
			Severity: "HIGH",
			Category: "scope",
			Path:     "internal/x.go",
			Line:     3,
			Message:  "change exceeds task scope",
			Evidence: "touches files outside the task scope",
		}},
	})
	if err != nil {
		t.Fatalf("marshal model output: %v", err)
	}
	chat := &fakeChat{output: string(out)}
	a, err := NewOllamaAnalyzer(testConfig(), chat)
	if err != nil {
		t.Fatalf("NewOllamaAnalyzer: %v", err)
	}
	res, err := a.Analyze(context.Background(), Request{Task: "t", Purpose: PurposeTaskTriage})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if res.StructuredEvidence == nil {
		t.Fatal("StructuredEvidence = nil, want non-nil evidence from the production analyzer")
	}
	if err := res.StructuredEvidence.Validate(); err != nil {
		t.Fatalf("StructuredEvidence failed validation: %v", err)
	}
	if res.StructuredEvidence.Version != EvidenceVersion {
		t.Errorf("evidence version = %d, want %d", res.StructuredEvidence.Version, EvidenceVersion)
	}
	if res.StructuredEvidence.Purpose != PurposeTaskTriage {
		t.Errorf("evidence purpose = %q, want TASK_TRIAGE from the request context", res.StructuredEvidence.Purpose)
	}
	if res.StructuredEvidence.Status != EvidenceFindings {
		t.Errorf("evidence status = %q, want FINDINGS", res.StructuredEvidence.Status)
	}
	if len(res.StructuredEvidence.Items) != 1 {
		t.Fatalf("evidence items = %d, want 1", len(res.StructuredEvidence.Items))
	}
	if got := res.StructuredEvidence.Items[0].Category; got != CategoryScope {
		t.Errorf("item category = %q, want %q", got, CategoryScope)
	}
	if res.Status != StatusFindings || len(res.Findings) != 1 {
		t.Errorf("existing Status/Findings behavior changed: %+v", res)
	}
}

// TestStructuredEvidenceInvalidFailsClosed asserts that evidence failing the
// fail-closed contract is never attached: Analyze returns a non-nil error and a
// zero Result, so invalid evidence can never authorize continuation. The request
// declares an unknown purpose, so the evidence derived from it is invalid. It uses
// only the in-process fake chat provider: no live provider and no network.
func TestStructuredEvidenceInvalidFailsClosed(t *testing.T) {
	out, err := json.Marshal(modelOutput{Status: "PASS", Summary: "ok"})
	if err != nil {
		t.Fatalf("marshal model output: %v", err)
	}
	chat := &fakeChat{output: string(out)}
	a, err := NewOllamaAnalyzer(testConfig(), chat)
	if err != nil {
		t.Fatalf("NewOllamaAnalyzer: %v", err)
	}
	res, err := a.Analyze(context.Background(), Request{Task: "t", Purpose: Purpose("NOT_A_KNOWN_PURPOSE")})
	if err == nil {
		t.Fatal("expected a fail-closed error for invalid structured evidence")
	}
	if res.Status != "" || len(res.Findings) != 0 || res.Summary != "" || res.StructuredEvidence != nil {
		t.Fatalf("result = %+v, want a zero Result on invalid evidence", res)
	}
}

// blockingChat is a ChatProvider that blocks until its context is done, signalling
// once it has been entered so a test can cancel deterministically rather than
// racing a sleep. It returns the context's error, exactly as a real provider does
// when its call is aborted.
type blockingChat struct {
	entered chan struct{}
}

func (c *blockingChat) Chat(ctx context.Context, _, _ string) (string, error) {
	return c.block(ctx)
}

func (c *blockingChat) ChatStructured(ctx context.Context, _, _ string, _ json.RawMessage) (string, error) {
	return c.block(ctx)
}

func (c *blockingChat) block(ctx context.Context) (string, error) {
	select {
	case c.entered <- struct{}{}:
	default:
	}
	<-ctx.Done()
	return "", ctx.Err()
}

// TestCanceledContextIsNotTimeout asserts a caller cancellation is classified as
// CANCELED, never as a provider TIMEOUT. The failure kind is distinct, the
// wrapping error still satisfies errors.Is(err, context.Canceled), the result is
// the zero Result, and cancellation is terminal (it is never retried).
func TestCanceledContextIsNotTimeout(t *testing.T) {
	assertCanceled := func(t *testing.T, err error) {
		t.Helper()
		if err == nil {
			t.Fatal("expected an error on a cancelled context")
		}
		var pe *ProviderError
		if !errors.As(err, &pe) {
			t.Fatalf("error %v is not a *ProviderError", err)
		}
		if pe.Kind == ErrTimeout {
			t.Fatalf("cancellation was mislabeled as %s", ErrTimeout)
		}
		if pe.Kind != ErrCanceled {
			t.Fatalf("kind = %s, want %s", pe.Kind, ErrCanceled)
		}
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error %v does not wrap context.Canceled", err)
		}
	}

	t.Run("canceled-before-call", func(t *testing.T) {
		chat := &blockingChat{entered: make(chan struct{}, 1)}
		a, err := NewOllamaAnalyzer(testConfig(), chat)
		if err != nil {
			t.Fatalf("NewOllamaAnalyzer: %v", err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		res, err := a.Analyze(ctx, Request{Task: "t"})
		assertCanceled(t, err)
		if res.Status != "" || len(res.Findings) != 0 || res.Summary != "" {
			t.Fatalf("cancellation must yield the zero Result, got %+v", res)
		}
		if got := a.JEVToolCalls(); got != 0 {
			t.Fatalf("JEVToolCalls = %d, want 0 (a canceled context is terminal, never retried)", got)
		}
	})

	t.Run("canceled-during-call", func(t *testing.T) {
		chat := &blockingChat{entered: make(chan struct{}, 1)}
		a, err := NewOllamaAnalyzer(testConfig(), chat)
		if err != nil {
			t.Fatalf("NewOllamaAnalyzer: %v", err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		done := make(chan struct{})
		go func() {
			<-chat.entered
			cancel()
			close(done)
		}()
		res, err := a.Analyze(ctx, Request{Task: "t"})
		<-done
		assertCanceled(t, err)
		if res.Status != "" || len(res.Findings) != 0 || res.Summary != "" {
			t.Fatalf("cancellation must yield the zero Result, got %+v", res)
		}
		if got := a.JEVToolCalls(); got != 1 {
			t.Fatalf("JEVToolCalls = %d, want 1 (cancellation is terminal, never retried)", got)
		}
	})
}

// TestJEVProviderMetadataNoSecrets asserts the provider/model metadata JEV
// records is canonical, independent of the model string, and secret-free.
//
// It covers the JEV-204 acceptance criteria: the provider and model can be
// identified unambiguously; the reported provider is a canonical known provider
// id and does not depend on the model; nothing shaped like a secret reaches the
// recorded metadata; and the analysis runs against a fake chat provider, so
// Claude and a live Ollama are never required.
func TestJEVProviderMetadataNoSecrets(t *testing.T) {
	// A synthetic sentinel stands in for a credential. It is not a real secret
	// and is never persisted: it only proves non-leakage. SOP reads no Ollama
	// credential, so this secret-shaped variable must have no effect at all.
	const sentinel = "jev204-sentinel-credential-not-a-real-secret"
	t.Setenv("SOP_OLLAMA_API_KEY", sentinel)
	t.Setenv("SOP_OLLAMA_BASE_URL", "http://127.0.0.1:11434")
	t.Setenv("SOP_OLLAMA_MODEL", "deepseek-v4.1-flash:cloud")
	t.Setenv("SOP_OLLAMA_TIMEOUT", "2m")

	// The reused shared configuration path supplies the model, base URL, and
	// timeout. The analyzer is wired to a fake chat provider, so the test needs
	// neither a live Ollama endpoint nor Claude.
	cfg, err := ollamaagent.ConfigFromEnv()
	if err != nil {
		t.Fatalf("ConfigFromEnv: %v", err)
	}
	chat := &fakeChat{output: `{"status":"PASS"}`}
	a, err := NewOllamaAnalyzer(cfg, chat)
	if err != nil {
		t.Fatalf("NewOllamaAnalyzer: %v", err)
	}

	// (a) The reported provider and model are exactly the configured values.
	if got := a.JEVProvider(); got != "ollama" {
		t.Fatalf("JEVProvider = %q, want %q", got, "ollama")
	}
	if got := a.JEVModel(); got != cfg.Model {
		t.Fatalf("JEVModel = %q, want %q", got, cfg.Model)
	}

	// (c) Nothing shaped like a secret appears in the recorded provider/model
	// metadata: not the sentinel, and not a secret-shaped field name.
	metadata := a.JEVProvider() + " / " + a.JEVModel()
	if strings.Contains(metadata, sentinel) {
		t.Fatalf("recorded provider/model metadata leaked the sentinel: %q", metadata)
	}
	for _, shaped := range []string{"api_key", "apikey", "authorization", "bearer", "secret", "token", "password", "credential"} {
		if strings.Contains(strings.ToLower(metadata), shaped) {
			t.Fatalf("recorded provider/model metadata contains secret-shaped field %q: %q", shaped, metadata)
		}
	}

	// (b) The reported provider is a canonical known provider id, validated
	// through the provider package rather than compared as a raw string, and it
	// is invariant to the model string.
	id, err := provider.ParseID(a.JEVProvider())
	if err != nil {
		t.Fatalf("JEVProvider %q is not a canonical provider id: %v", a.JEVProvider(), err)
	}
	if id != provider.Ollama {
		t.Fatalf("provider id = %q, want %q", id, provider.Ollama)
	}
	for _, model := range []string{"deepseek-v4.1-flash:cloud", "qwen3:4b", "nemotron-3-nano:30b-cloud"} {
		alt := testConfig()
		alt.Model = model
		am, err := NewOllamaAnalyzer(alt, &fakeChat{output: `{"status":"PASS"}`})
		if err != nil {
			t.Fatalf("NewOllamaAnalyzer(%q): %v", model, err)
		}
		if am.JEVProvider() != a.JEVProvider() {
			t.Fatalf("JEVProvider changed with model %q: %q, want %q", model, am.JEVProvider(), a.JEVProvider())
		}
		if am.JEVModel() != model {
			t.Fatalf("JEVModel = %q, want %q", am.JEVModel(), model)
		}
		if _, err := provider.ParseID(am.JEVProvider()); err != nil {
			t.Fatalf("JEVProvider %q is not canonical for model %q: %v", am.JEVProvider(), model, err)
		}
	}

	// (d) The analysis runs against the fake chat provider only: it completes
	// with no Claude, no live Ollama, and no network.
	if _, err := a.Analyze(context.Background(), Request{Task: "t"}); err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if chat.calls != 1 {
		t.Fatalf("fake provider calls = %d, want 1", chat.calls)
	}
	if chat.model != cfg.Model {
		t.Fatalf("provider model = %q, want %q", chat.model, cfg.Model)
	}
}

// TestUnknownCategoryFailsClosed asserts the analyzer fails closed when the model
// reports a non-empty category outside the closed JEV category vocabulary: Analyze
// returns a non-nil error and the zero Result. An unrecognized category is never
// silently coerced to the unspecified value, because that would drop the model's
// classification and could let a mislabeled security-family finding bypass the
// router's conservative HIGH floor. It uses only the in-process fake chat provider:
// no live provider and no network.
func TestUnknownCategoryFailsClosed(t *testing.T) {
	out, err := json.Marshal(modelOutput{
		Status:  "FINDINGS",
		Summary: "issue",
		Findings: []modelFinding{{
			ID: "F1", Severity: "HIGH", Category: "totally_unknown_category", Message: "issue",
		}},
	})
	if err != nil {
		t.Fatalf("marshal model output: %v", err)
	}
	chat := &fakeChat{output: string(out)}
	a, err := NewOllamaAnalyzer(testConfig(), chat)
	if err != nil {
		t.Fatalf("NewOllamaAnalyzer: %v", err)
	}
	res, err := a.Analyze(context.Background(), Request{Task: "t"})
	if err == nil {
		t.Fatal("expected a fail-closed error for an unknown non-empty category")
	}
	if res.Status != "" || len(res.Findings) != 0 || res.Summary != "" || res.StructuredEvidence != nil {
		t.Fatalf("result = %+v, want a zero Result on an unknown non-empty category", res)
	}
}

// TestValidSecurityCategoryRemainsValid asserts a valid security-family category is
// preserved through the analyzer: the evidence stays valid and the item keeps its
// typed category, so the router's conservative HIGH floor still applies. It uses
// only the in-process fake chat provider: no live provider and no network.
func TestValidSecurityCategoryRemainsValid(t *testing.T) {
	out, err := json.Marshal(modelOutput{
		Status:  "FINDINGS",
		Summary: "issue",
		Findings: []modelFinding{{
			ID: "F1", Severity: "MEDIUM", Category: "security", Message: "hardcoded secret",
			Evidence: "literal in source",
		}},
	})
	if err != nil {
		t.Fatalf("marshal model output: %v", err)
	}
	chat := &fakeChat{output: string(out)}
	a, err := NewOllamaAnalyzer(testConfig(), chat)
	if err != nil {
		t.Fatalf("NewOllamaAnalyzer: %v", err)
	}
	res, err := a.Analyze(context.Background(), Request{Task: "t"})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if res.StructuredEvidence == nil {
		t.Fatal("StructuredEvidence = nil, want non-nil evidence")
	}
	if err := res.StructuredEvidence.Validate(); err != nil {
		t.Fatalf("StructuredEvidence failed validation: %v", err)
	}
	if len(res.StructuredEvidence.Items) != 1 {
		t.Fatalf("evidence items = %d, want 1", len(res.StructuredEvidence.Items))
	}
	if got := res.StructuredEvidence.Items[0].Category; got != CategorySecurity {
		t.Errorf("item category = %q, want %q", got, CategorySecurity)
	}
}

// categoryEnumFromSchema extracts the findings[].items.properties.category enum
// from the shipped schema, so the vocabulary test reads the real constraint
// rather than a copy of it.
func categoryEnumFromSchema(t *testing.T, schema json.RawMessage) []string {
	t.Helper()
	var doc struct {
		Properties struct {
			Findings struct {
				Items struct {
					Properties struct {
						Category struct {
							Enum []string `json:"enum"`
						} `json:"category"`
					} `json:"properties"`
				} `json:"items"`
			} `json:"findings"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(schema, &doc); err != nil {
		t.Fatalf("schema is not valid JSON: %v", err)
	}
	return doc.Properties.Findings.Items.Properties.Category.Enum
}

// TestPromptAndSchemaAdvertiseEveryCategory asserts the rendered prompt and the
// shipped JSON schema category enum advertise every value in the authoritative
// vocabulary (Categories, derived from evidence.go) and nothing else, so a drift
// between the advertised and the validated sets fails here.
func TestPromptAndSchemaAdvertiseEveryCategory(t *testing.T) {
	cats := Categories()
	if len(cats) == 0 {
		t.Fatal("authoritative category vocabulary is empty")
	}

	a := &OllamaAnalyzer{cfg: testConfig()}
	prompt := a.buildPrompt(Request{Task: "t"})
	for _, c := range cats {
		if !strings.Contains(prompt, string(c)) {
			t.Errorf("prompt does not advertise category %q", c)
		}
	}

	enum := categoryEnumFromSchema(t, jevOutputJSONSchema)
	if len(enum) != len(cats) {
		t.Fatalf("schema category enum has %d values, authoritative set has %d: %v", len(enum), len(cats), enum)
	}
	enumSet := make(map[string]bool, len(enum))
	for _, v := range enum {
		if enumSet[v] {
			t.Errorf("schema category enum repeats %q", v)
		}
		enumSet[v] = true
	}
	for _, c := range cats {
		if !enumSet[string(c)] {
			t.Errorf("schema category enum missing %q", c)
		}
	}
}

// TestEveryAuthoritativeCategoryYieldsValidEvidence asserts every valid category
// is preserved through the analyzer and yields valid evidence, so the advertised
// vocabulary is actually accepted.
func TestEveryAuthoritativeCategoryYieldsValidEvidence(t *testing.T) {
	for _, c := range Categories() {
		c := c
		t.Run(string(c), func(t *testing.T) {
			out, err := json.Marshal(modelOutput{
				Status:  "FINDINGS",
				Summary: "issue",
				Findings: []modelFinding{{
					ID: "F1", Severity: "HIGH", Category: string(c), Message: "issue",
				}},
			})
			if err != nil {
				t.Fatalf("marshal model output: %v", err)
			}
			chat := &fakeChat{output: string(out)}
			a, err := NewOllamaAnalyzer(testConfig(), chat)
			if err != nil {
				t.Fatalf("NewOllamaAnalyzer: %v", err)
			}
			res, err := a.Analyze(context.Background(), Request{Task: "t"})
			if err != nil {
				t.Fatalf("Analyze: %v", err)
			}
			if res.StructuredEvidence == nil {
				t.Fatal("StructuredEvidence = nil, want non-nil evidence")
			}
			if err := res.StructuredEvidence.Validate(); err != nil {
				t.Fatalf("evidence failed validation: %v", err)
			}
			if len(res.StructuredEvidence.Items) != 1 {
				t.Fatalf("evidence items = %d, want 1", len(res.StructuredEvidence.Items))
			}
			if got := res.StructuredEvidence.Items[0].Category; got != c {
				t.Errorf("item category = %q, want %q", got, c)
			}
		})
	}
}
