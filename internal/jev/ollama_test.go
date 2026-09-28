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
	calls  int
}

func (f *fakeChat) Chat(ctx context.Context, model, prompt string) (string, error) {
	f.calls++
	f.model = model
	f.prompt = prompt
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
