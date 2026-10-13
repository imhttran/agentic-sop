// Package nimble adapts the Nimble decision model, served through an
// Ollama-compatible backend, to the provider-neutral decision contract.
//
// The request path is:
//
//	decision.Provider -> internal/providers/nimble -> POST /v1/systemone -> Ollama -> Nimble
//
// The adapter only translates and normalizes. It never defines or enforces SOP
// policy; approval gates, execution thresholds, and safety enforcement belong
// to agentic-sop.
//
// SystemOne-specific concepts (the "noul" boolean representation, Ollama
// request/response shapes) are implementation details confined to this package
// and never appear in the public decision package.
package nimble

import (
	"context"
	"fmt"
	"strings"

	"github.com/imhttran/sop-decision-adapters/decision"
)

// Provider implements decision.Provider for Nimble.
type Provider struct {
	cfg       Config
	transport Transport
	// scorer is set only for the standalone /score transport; the SystemOne
	// transport is unchanged and leaves it nil.
	scorer Scorer
}

// New builds a Nimble provider. A nil transport defaults to an HTTPTransport
// pointed at cfg.BaseURL, which is convenient in production and stubbed in
// tests.
func New(cfg Config, transport Transport) *Provider {
	cfg = cfg.WithDefaults()
	if transport == nil {
		transport = NewHTTPTransport(cfg.BaseURL, cfg.Timeout)
	}
	p := &Provider{cfg: cfg, transport: transport}
	if cfg.Transport == TransportScore {
		p.scorer = NewScoreTransport(cfg.ScoreURL, cfg.Timeout)
	}
	return p
}

// NewFromEnv builds a Nimble provider from environment configuration.
func NewFromEnv() *Provider {
	return New(ConfigFromEnv(), nil)
}

// Name implements decision.Provider.
func (p *Provider) Name() string { return ProviderName }

// Available implements decision.Provider.
//
// It distinguishes three states without issuing an inference request:
//
//   - Ollama unreachable (or the request times out)   -> false
//   - Ollama reachable but the configured model absent -> false
//   - Ollama reachable and the configured model present -> true
//
// Model presence is verified through the backend's model listing endpoint.
func (p *Provider) Available(ctx context.Context) bool {
	if p.cfg.Transport == TransportScore {
		return p.scorer != nil && p.scorer.Health(ctx) == nil
	}
	models, err := p.transport.ListModels(ctx)
	if err != nil {
		return false
	}
	return p.hasModel(models)
}

// hasModel reports whether the configured model is present in the reported
// model names. Ollama may report names with a tag suffix ("nimble:latest"), so
// an exact match or a tag-prefixed match both count.
func (p *Provider) hasModel(models []string) bool {
	want := strings.TrimSpace(p.cfg.Model)
	if want == "" {
		return false
	}
	for _, m := range models {
		m = strings.TrimSpace(m)
		if m == want || strings.HasPrefix(m, want+":") {
			return true
		}
	}
	return false
}

// Decide implements decision.Provider.
//
// It validates the request, translates it into a SystemOne request, performs
// the native POST /v1/systemone call, and normalizes the response back into a
// provider-neutral decision.DecisionResult.
func (p *Provider) Decide(ctx context.Context, req decision.DecisionRequest) (decision.DecisionResult, error) {
	if err := req.Validate(); err != nil {
		return decision.DecisionResult{}, providerErr(decision.KindInvalidRequest, err)
	}

	if p.cfg.Transport == TransportScore {
		return p.decideScore(ctx, req)
	}

	sysReq, err := BuildSystemOneRequest(p.cfg.Model, req)
	if err != nil {
		return decision.DecisionResult{}, providerErr(decision.KindInvalidRequest, err)
	}

	sysResp, err := p.transport.SystemOne(ctx, sysReq)
	if err != nil {
		return decision.DecisionResult{}, providerErr(classifyTransportError(err), err)
	}

	result, err := NormalizeSystemOneResponse(sysResp, req)
	if err != nil {
		return decision.DecisionResult{}, providerErr(decision.KindMalformedResponse, err)
	}

	return result, nil
}

// decideScore runs the standalone /score transport: translate to the scoring
// schema, POST /score, and normalize the response into the decision contract.
func (p *Provider) decideScore(ctx context.Context, req decision.DecisionRequest) (decision.DecisionResult, error) {
	if p.scorer == nil {
		return decision.DecisionResult{}, providerErr(decision.KindProviderFailure, fmt.Errorf("nimble: score transport is not configured"))
	}
	scoreReq, err := BuildScoreRequest(req)
	if err != nil {
		return decision.DecisionResult{}, providerErr(decision.KindInvalidRequest, err)
	}
	resp, err := p.scorer.Score(ctx, scoreReq)
	if err != nil {
		return decision.DecisionResult{}, providerErr(classifyTransportError(err), err)
	}
	result, err := NormalizeScoreResponse(resp, req)
	if err != nil {
		return decision.DecisionResult{}, providerErr(decision.KindMalformedResponse, err)
	}
	return result, nil
}

// providerErr wraps cause in a normalized decision.ProviderError attributed to
// this adapter.
func providerErr(kind decision.ErrorKind, cause error) error {
	return &decision.ProviderError{Provider: ProviderName, Kind: kind, Err: cause}
}

// classifyTransportError maps a transport-level failure to a normalized
// decision.ErrorKind. Connectivity failures become KindUnavailable; responses
// the transport could not decode become KindMalformedResponse; anything else is
// a generic KindProviderFailure.
func classifyTransportError(err error) decision.ErrorKind {
	if err == nil {
		return decision.KindProviderFailure
	}
	if isMalformedResponse(err) {
		return decision.KindMalformedResponse
	}
	if isUnavailable(err) {
		return decision.KindUnavailable
	}
	return decision.KindProviderFailure
}
