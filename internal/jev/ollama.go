package jev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/imhttran/agentic-sop/internal/ollamaagent"
	"github.com/imhttran/agentic-sop/internal/review"
)

// Compile-time assertion that the Ollama adapter satisfies the JEV boundary.
var _ Analyzer = (*OllamaAnalyzer)(nil)

// maxPromptRunes bounds the prompt built from a Request. The boundary receives
// read-only snapshots that can be arbitrarily large (a full diff), so the
// adapter truncates rather than forwarding unbounded input to the model.
const maxPromptRunes = 48 << 10 // 48 Ki characters

// maxOutputRunes bounds model output parsed into a Result. Anything beyond this
// is truncated before parsing so a runaway model cannot retain unbounded memory.
const maxOutputRunes = 256 << 10 // 256 Ki characters

// ChatProvider is the reused Ollama provider path the adapter drives. It is the
// single-turn chat entry point of the existing Ollama infrastructure; the
// adapter introduces no second provider stack.
type ChatProvider interface {
	Chat(ctx context.Context, model, prompt string) (string, error)
}

// OllamaAnalyzer is a jev.Analyzer backed by the existing Ollama infrastructure.
// It receives only the bounded jev.Request, builds a prompt from it, and returns
// a validated jev.Result. It never transitions task state, mutates SOP
// persistence, or marks validation/review successful.
type OllamaAnalyzer struct {
	cfg      ollamaagent.Config
	provider ChatProvider
}

// ProviderErrorKind names the class of an adapter failure so SOP can react to a
// focused category rather than a stringly-typed message.
type ProviderErrorKind string

const (
	ErrProviderUnreachable ProviderErrorKind = "PROVIDER_UNREACHABLE"
	ErrConfigInvalid       ProviderErrorKind = "CONFIG_INVALID"
	ErrTimeout             ProviderErrorKind = "TIMEOUT"
	ErrMalformedOutput     ProviderErrorKind = "MALFORMED_OUTPUT"
)

// ProviderError is a focused JEV provider failure. It names the failure class in
// JEV terminology and preserves the underlying cause. It never implies a
// lifecycle transition or a validation/review success.
type ProviderError struct {
	Kind ProviderErrorKind
	Err  error
}

func (e *ProviderError) Error() string {
	if e.Err == nil {
		return "jev: provider failure (" + string(e.Kind) + ")"
	}
	return fmt.Sprintf("jev: provider failure (%s): %v", e.Kind, e.Err)
}

func (e *ProviderError) Unwrap() error { return e.Err }

func providerError(kind ProviderErrorKind, err error) error {
	return &ProviderError{Kind: kind, Err: err}
}

// NewOllamaAnalyzer builds an adapter from the reused Ollama configuration. The
// model, base URL, and timeout come from ollamaagent.ConfigFromEnv, so no model
// is a required hardcoded value: the adapter works whenever configuration
// supplies one.
func NewOllamaAnalyzer(cfg ollamaagent.Config, provider ChatProvider) (*OllamaAnalyzer, error) {
	if provider == nil {
		return nil, providerError(ErrConfigInvalid, errors.New("ollama chat provider is required"))
	}
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, providerError(ErrConfigInvalid, errors.New("ollama base URL is required"))
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, providerError(ErrConfigInvalid, errors.New("ollama model is required (set SOP_OLLAMA_MODEL)"))
	}
	if cfg.Timeout <= 0 {
		return nil, providerError(ErrConfigInvalid, errors.New("ollama timeout must be positive"))
	}
	return &OllamaAnalyzer{cfg: cfg, provider: provider}, nil
}

// NewOllamaAnalyzerFromEnv builds an adapter from the environment using the
// reused ollamaagent.ConfigFromEnv path, wiring the reused Ollama chat client.
func NewOllamaAnalyzerFromEnv() (*OllamaAnalyzer, error) {
	cfg, err := ollamaagent.ConfigFromEnv()
	if err != nil {
		return nil, providerError(ErrConfigInvalid, err)
	}
	provider, err := ollamaagent.NewChatProvider(cfg)
	if err != nil {
		return nil, providerError(ErrConfigInvalid, err)
	}
	return NewOllamaAnalyzer(cfg, provider)
}

// JEVProvider names the provider behind the adapter for JEV invocation metrics.
// It is diagnostic metadata only and never feeds a decision.
func (a *OllamaAnalyzer) JEVProvider() string { return "ollama" }

// JEVModel names the model the adapter analyzes with, for JEV invocation
// metrics. It is diagnostic metadata only and never feeds a decision.
func (a *OllamaAnalyzer) JEVModel() string { return a.cfg.Model }

// JEVToolCalls reports the number of provider calls one analysis makes. The
// adapter is a single bounded chat round-trip, so it is one. It is diagnostic
// metadata only and never feeds a decision.
func (a *OllamaAnalyzer) JEVToolCalls() int { return 1 }

// Analyze maps the bounded Request to a model prompt, runs the reused provider
// under a bounded context, and converts the output into a validated Result. It
// fails closed: a non-nil error is always accompanied by a zero Result.
func (a *OllamaAnalyzer) Analyze(ctx context.Context, req Request) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	// Bound the whole call by the configured timeout in addition to any caller
	// deadline, so execution terminates even if the caller passes none.
	callCtx, cancel := context.WithTimeout(ctx, a.cfg.Timeout)
	defer cancel()

	prompt := a.buildPrompt(req)

	out, err := a.provider.Chat(callCtx, a.cfg.Model, prompt)
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded), errors.Is(callCtx.Err(), context.DeadlineExceeded):
			return Result{}, providerError(ErrTimeout, err)
		case errors.Is(err, context.Canceled), errors.Is(callCtx.Err(), context.Canceled):
			return Result{}, providerError(ErrTimeout, err)
		default:
			return Result{}, providerError(ErrProviderUnreachable, err)
		}
	}
	if err := callCtx.Err(); err != nil {
		return Result{}, providerError(ErrTimeout, err)
	}

	result, err := parseModelOutput(out)
	if err != nil {
		return Result{}, providerError(ErrMalformedOutput, err)
	}
	return result, nil
}

// buildPrompt renders the Request using the boundary's own terminology and
// bounds the result. It uses only the read-only Request snapshot.
func (a *OllamaAnalyzer) buildPrompt(req Request) string {
	var b strings.Builder
	writeField := func(label, value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		b.WriteString(label)
		b.WriteString(":\n")
		b.WriteString(value)
		b.WriteString("\n\n")
	}
	writeField("Task", req.Task)
	writeField("Criteria", req.Criteria)
	if len(req.ChangedFiles) > 0 {
		b.WriteString("ChangedFiles:\n")
		b.WriteString(strings.Join(req.ChangedFiles, "\n"))
		b.WriteString("\n\n")
	}
	writeField("RepositoryContext", req.RepositoryContext)
	writeField("ValidationResult", req.ValidationResult)
	writeField("ReviewResult", req.ReviewResult)
	return truncateRunes(b.String(), maxPromptRunes)
}

// modelOutput is the JSON shape the adapter expects from the model.
type modelOutput struct {
	Status   string         `json:"status"`
	Summary  string         `json:"summary"`
	Findings []modelFinding `json:"findings"`
}

type modelFinding struct {
	ID       string `json:"id"`
	Severity string `json:"severity"`
	Category string `json:"category"`
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Message  string `json:"message"`
	Evidence string `json:"evidence"`
}

// parseModelOutput converts bounded model output into a validated Result. It
// fails closed: malformed output never yields PASS.
func parseModelOutput(raw string) (Result, error) {
	trimmed := strings.TrimSpace(truncateRunes(raw, maxOutputRunes))
	if trimmed == "" {
		return Result{}, errors.New("empty model output")
	}
	var out modelOutput
	if err := json.Unmarshal([]byte(trimmed), &out); err != nil {
		return Result{}, fmt.Errorf("model output is not valid JSON: %w", err)
	}

	status := Status(strings.ToUpper(strings.TrimSpace(out.Status)))
	if !status.Valid() {
		return Result{}, errInvalidStatus(out.Status)
	}

	findings := make([]Finding, 0, len(out.Findings))
	for _, f := range out.Findings {
		sev := review.Severity(strings.ToUpper(strings.TrimSpace(f.Severity)))
		if !sev.Valid() {
			return Result{}, errInvalidSeverity(f.Severity)
		}
		findings = append(findings, Finding{
			ID:       f.ID,
			Severity: sev,
			Category: f.Category,
			Path:     f.Path,
			Line:     f.Line,
			Message:  f.Message,
			Evidence: f.Evidence,
		})
	}

	result := Result{Status: status, Findings: findings, Summary: out.Summary}
	if err := result.Validate(); err != nil {
		return Result{}, err
	}
	return result, nil
}

// truncateRunes bounds s to at most max runes, appending an explicit marker so a
// truncated prompt/output is observable rather than silently dropped.
func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "\n...[truncated]"
}
