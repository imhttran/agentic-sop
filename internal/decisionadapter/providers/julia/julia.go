// Package julia adapts the Julia decision model, executed as ONNX inference, to
// the provider-neutral decision contract.
//
// The request path is:
//
//	decision.Provider -> internal/providers/julia -> Runner -> ONNX inference -> Julia
//
// The concrete ONNX execution is isolated behind the Runner seam (see
// runner.go), so the adapter can be tested without any ONNX runtime and an
// in-process ONNX binding can replace the external command runner later without
// touching the adapter, the translation layer, the CLI, or the public decision
// contract.
//
// The adapter only translates and normalizes. It never defines or enforces SOP
// policy; approval gates, execution thresholds, and safety enforcement belong
// to agentic-sop.
package julia

import (
	"context"
	"strings"

	"github.com/imhttran/agentic-sop/decision"
)

// Provider implements decision.Provider for Julia.
type Provider struct {
	cfg    Config
	runner Runner
}

// New builds a Julia provider. A nil runner defaults to a CommandRunner built
// from cfg: the repository-owned helper (tools/julia/infer.py, run with
// JULIA_PYTHON) unless cfg.InferenceCmd overrides it. Tests pass a stub runner.
func New(cfg Config, runner Runner) *Provider {
	cfg = cfg.WithDefaults()
	if runner == nil {
		command := cfg.InferenceCmd
		helperPath := ""
		if len(command) == 0 {
			helperPath = resolveHelperPath()
			command = []string{cfg.Python, helperPath}
		}
		runner = &CommandRunner{
			Command:    command,
			ModelPath:  cfg.ModelPath,
			HelperPath: helperPath,
			Timeout:    cfg.Timeout,
		}
	}
	return &Provider{cfg: cfg, runner: runner}
}

// NewFromEnv builds a Julia provider from environment configuration.
func NewFromEnv() *Provider {
	return New(ConfigFromEnv(), nil)
}

// Name implements decision.Provider.
func (p *Provider) Name() string { return ProviderName }

// Available implements decision.Provider.
//
// It delegates to the runner, which cheaply reports whether inference can
// currently be served (for example whether the helper, interpreter, and model
// exist). It never performs an inference request.
func (p *Provider) Available(ctx context.Context) bool {
	return p.runner.Available(ctx)
}

// Decide implements decision.Provider.
//
// It validates the request, translates it into the Julia runtime request, executes
// the runner, and normalizes the raw Outputs back into a provider-neutral
// decision.DecisionResult.
//
// The runner-reported model takes precedence. When the runner reports none, the
// configured model (JULIA_MODEL) is reported instead.
func (p *Provider) Decide(ctx context.Context, req decision.DecisionRequest) (decision.DecisionResult, error) {
	if err := req.Validate(); err != nil {
		return decision.DecisionResult{}, providerErr(decision.KindInvalidRequest, err)
	}

	in, err := BuildInputs(req)
	if err != nil {
		return decision.DecisionResult{}, providerErr(decision.KindInvalidRequest, err)
	}

	out, err := p.runner.Run(ctx, in)
	if err != nil {
		return decision.DecisionResult{}, providerErr(classifyRunnerError(err), err)
	}

	result, err := NormalizeOutputs(out, req)
	if err != nil {
		return decision.DecisionResult{}, providerErr(decision.KindMalformedResponse, err)
	}

	// The runner-reported model wins; fall back to the configured model only
	// when the runner reported none. This runs after NormalizeOutputs (and
	// therefore after result.Validate()), so it never alters validation
	// semantics.
	if strings.TrimSpace(result.Model) == "" {
		result.Model = p.cfg.Model
	}

	return result, nil
}

// providerErr wraps cause in a normalized decision.ProviderError attributed to
// this adapter.
func providerErr(kind decision.ErrorKind, cause error) error {
	return &decision.ProviderError{Provider: ProviderName, Kind: kind, Err: cause}
}

// classifyRunnerError maps a runner-level failure to a normalized
// decision.ErrorKind. Unavailable runners (unconfigured or unreachable) become
// KindUnavailable; undecodable output becomes KindMalformedResponse; anything
// else is a generic KindProviderFailure.
func classifyRunnerError(err error) decision.ErrorKind {
	if err == nil {
		return decision.KindProviderFailure
	}
	if isRunnerMalformed(err) {
		return decision.KindMalformedResponse
	}
	if isRunnerUnavailable(err) {
		return decision.KindUnavailable
	}
	return decision.KindProviderFailure
}
