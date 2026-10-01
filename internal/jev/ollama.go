package jev

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/imhttran/agentic-sop/internal/ollamaagent"
	"github.com/imhttran/agentic-sop/internal/review"
)

// Compile-time assertion that the Ollama adapter satisfies the JEV boundary.
var _ Analyzer = (*OllamaAnalyzer)(nil)

// maxPromptRunes bounds the prompt built from a Request. The boundary receives
// read-only snapshots that can be arbitrarily large (a full diff), so the
// adapter truncates rather than forwarding unbounded input to the model.
const maxPromptRunes = 48 << 10 // 48 Ki characters

// maxPromptRunesEnv optionally overrides maxPromptRunes. Raising it lets the
// analyzer review a change set larger than the built-in bound, at the cost of a
// larger prompt. An unset, non-numeric, or non-positive value keeps the default,
// so behavior is unchanged when it is not set.
const maxPromptRunesEnv = "SOP_JEV_MAX_PROMPT_RUNES"

// promptRunesBound returns the effective prompt bound: the environment override
// when it is a positive integer, otherwise the built-in default.
func promptRunesBound() int { return positiveEnvInt(maxPromptRunesEnv, maxPromptRunes) }

// positiveEnvInt returns the named environment variable when it parses as a
// positive integer, and fallback otherwise (unset, blank, non-numeric, or <= 0).
func positiveEnvInt(name string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

// maxOutputRunes bounds model output parsed into a Result. Anything beyond this
// is truncated before parsing so a runaway model cannot retain unbounded memory.
const maxOutputRunes = 256 << 10 // 256 Ki characters

// maxAnalysisAttempts bounds one JEV analysis: the initial attempt plus at most
// two corrective retries when the model returns output that does not satisfy the
// required JSON schema. It is small and fixed so a malformed response can neither
// become an unbounded loop nor a provider-retry storm. Provider failures are not
// retried here; they are absorbed at the provider-call boundary.
const maxAnalysisAttempts = 3

// jevOutputSchema is the output contract for a JEV analysis. It states the exact
// JSON shape the adapter validates and forbids the markdown/prose forms that make
// the output unparseable, so the model is told the contract rather than guessed
// at. It reuses the schema the adapter already validates (modelOutput) and is
// complemented by the provider's schema-constrained mode (jevOutputJSONSchema);
// it introduces no second protocol.
const jevOutputSchema = `Return ONE JSON object and nothing else (no prose, no markdown, no code fences):
{"status":"PASS|FINDINGS|INCOMPLETE|ERROR","summary":string,"findings":[{"id":string,"severity":"INFO|LOW|MEDIUM|HIGH|CRITICAL","category":string,"path":string,"line":number,"message":string,"evidence":string}]}
Use "PASS" with an empty findings list when there is nothing to report, and "FINDINGS" with at least one finding otherwise. Report only real findings with concrete evidence.`

// jevCorrectiveInstruction is appended to the prompt when the model's previous
// response did not satisfy the required JSON schema. It asks the model to correct
// its own response; SOP never interprets arbitrary prose as findings or strips
// arbitrary output until it happens to parse.
const jevCorrectiveInstruction = `Your previous response violated the required JSON schema. Return ONLY valid JSON matching the schema. Do not include markdown, prose, code fences, headings, or commentary.`

// ChatProvider is the reused Ollama provider path the adapter drives. It is the
// single-turn chat entry point of the existing Ollama infrastructure; the adapter
// introduces no second provider stack. ChatStructured is the same provider entry
// point constrained to a JSON schema, so the adapter can ask Ollama to enforce the
// output shape instead of relying on the model to infer it from prose.
type ChatProvider interface {
	Chat(ctx context.Context, model, prompt string) (string, error)
	ChatStructured(ctx context.Context, model, prompt string, schema json.RawMessage) (string, error)
}

// jevOutputJSONSchema is the JSON schema the adapter constrains the model's
// response to (Ollama's "format" field). It mirrors the authoritative modelOutput
// contract the strict parser validates, so the provider enforces the same shape
// the parser already requires rather than the model inferring it. It is the
// schema-constraint layer of defense in depth; the prompt contract and the strict
// parser + bounded corrective retry remain in place.
var jevOutputJSONSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "status": {"type": "string", "enum": ["PASS", "FINDINGS", "INCOMPLETE", "ERROR"]},
    "summary": {"type": "string"},
    "findings": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "id": {"type": "string"},
          "severity": {"type": "string", "enum": ["INFO", "LOW", "MEDIUM", "HIGH", "CRITICAL"]},
          "category": {"type": "string"},
          "path": {"type": "string"},
          "line": {"type": "integer"},
          "message": {"type": "string"},
          "evidence": {"type": "string"}
        },
        "required": ["severity"]
      }
    }
  },
  "required": ["status"]
}`)

// OllamaAnalyzer is a jev.Analyzer backed by the existing Ollama infrastructure.
// It receives only the bounded jev.Request, builds a prompt from it, and returns
// a validated jev.Result. It never transitions task state, mutates SOP
// persistence, or marks validation/review successful.
type OllamaAnalyzer struct {
	cfg      ollamaagent.Config
	provider ChatProvider
	// providerCalls records the number of JEV-level provider calls the most recent
	// Analyze made: the initial attempt plus any bounded corrective retries. It is
	// diagnostic metadata only (see JEVToolCalls) and never feeds a decision. It is
	// atomic so a shared analyzer cannot race if Analyze is ever called
	// concurrently.
	providerCalls atomic.Int64
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
	// Attempts is how many model attempts were made before the failure, when
	// known (zero when not applicable). It is diagnostic metadata only, recorded
	// so a report can distinguish a first-attempt failure from an exhausted
	// corrective retry.
	Attempts int
}

func (e *ProviderError) Error() string {
	base := "jev: provider failure (" + string(e.Kind) + ")"
	if e.Attempts > 1 {
		base += fmt.Sprintf(" after %d attempts", e.Attempts)
	}
	if e.Err == nil {
		return base
	}
	return base + ": " + e.Err.Error()
}

func (e *ProviderError) Unwrap() error { return e.Err }

func providerError(kind ProviderErrorKind, err error) error {
	return &ProviderError{Kind: kind, Err: err}
}

// providerErrorAfter is providerError carrying the number of attempts made before
// the failure, so a malformed-output failure records how many corrective retries
// were exhausted instead of reading like a first-attempt failure.
func providerErrorAfter(kind ProviderErrorKind, err error, attempts int) error {
	return &ProviderError{Kind: kind, Err: err, Attempts: attempts}
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

// JEVToolCalls reports the number of JEV-level provider calls the most recent
// analysis made: the initial attempt plus any bounded corrective retries. It is
// the analysis's real provider cost, not a fixed one; it does not count
// lower-level transport retries the shared Ollama client performs internally, so
// it can under-report a call the transport itself retried. It is diagnostic
// metadata only and never feeds a decision.
func (a *OllamaAnalyzer) JEVToolCalls() int { return int(a.providerCalls.Load()) }

// Analyze maps the bounded Request to a model prompt, runs the reused provider
// under a bounded context, and converts the output into a validated Result. It
// fails closed: a non-nil error is always accompanied by a zero Result.
//
// A response that does not satisfy the required JSON schema is retried a small,
// fixed number of times with a corrective instruction, so the model corrects its
// own malformed output (for example markdown preceding the JSON) rather than SOP
// interpreting arbitrary prose as findings. Provider failures are not retried
// here: they are absorbed at the provider-call boundary. The corrective retry is
// bounded by maxAnalysisAttempts and never becomes an unbounded loop.
func (a *OllamaAnalyzer) Analyze(ctx context.Context, req Request) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	// Bound the whole call by the configured timeout in addition to any caller
	// deadline, so execution terminates even if the caller passes none.
	callCtx, cancel := context.WithTimeout(ctx, a.cfg.Timeout)
	defer cancel()

	prompt := a.buildPrompt(req)

	// Record the analysis's real provider cost (initial attempt plus corrective
	// retries) for the metrics, whatever path it exits by.
	providerCalls := 0
	defer func() { a.providerCalls.Store(int64(providerCalls)) }()

	var lastErr error
	for attempt := 1; attempt <= maxAnalysisAttempts; attempt++ {
		// A cancelled/expired context is terminal: never retry it.
		if err := callCtx.Err(); err != nil {
			return Result{}, providerError(ErrTimeout, err)
		}

		// The first attempt carries the base contract; a corrective retry appends
		// the schema-violation instruction so the model corrects its own output.
		p := prompt
		if attempt > 1 {
			p = prompt + "\n\n" + jevCorrectiveInstruction
		}

		// The provider constrains the response to the JEV output schema; the strict
		// parser below and the corrective retry are the remaining layers of defense
		// in depth.
		providerCalls++
		out, err := a.provider.ChatStructured(callCtx, a.cfg.Model, p, jevOutputJSONSchema)
		if err != nil {
			// A provider failure is handled by the provider-call boundary (which
			// retries transient failures); it is never corrective-retried here and
			// never retried once the context is gone.
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
		if err == nil {
			return result, nil
		}
		lastErr = err
	}

	// Every attempt produced output that violated the schema: fail closed with a
	// malformed-output error that records how many attempts were exhausted.
	return Result{}, providerErrorAfter(ErrMalformedOutput, lastErr, maxAnalysisAttempts)
}

// buildPrompt renders the Request using the boundary's own terminology and
// bounds the result. It uses only the read-only Request snapshot.
func (a *OllamaAnalyzer) buildPrompt(req Request) string {
	var b strings.Builder
	// State the output contract first, so it survives prompt truncation and the
	// model is told the required JSON shape rather than being expected to infer it.
	b.WriteString(jevOutputSchema)
	b.WriteString("\n\n")
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
	return truncateRunes(b.String(), promptRunesBound())
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
