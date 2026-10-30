package cli

// Locality resolution seam (LOC-003).
//
// Every model.Resolve call in the CLI routes through resolveModelLocality: the
// run-level/default selection, per-task routing, prompt routing, and escalation
// all resolve locality here. No call site resolves a model on its own.
//
// The helper resolves in two steps:
//
//  1. It calls model.Resolve with the caller's inputs and no runtime evidence.
//     Explicit configuration and reliable naming inference resolve here, so an
//     explicit locality and an ollama ":cloud" model never trigger a probe.
//
//  2. When resolution reports a *model.LocalityUnknownError, it performs a
//     bounded runtime observation: it asks the runtime once for every reference
//     the error reports, and re-resolves with the evidence keyed by provider and
//     model. Each reference is observed at most once. When an observation adds no
//     new evidence, the helper returns the original actionable unknown-locality
//     error instead of looping or guessing.
//
// The observation asks the runtime the run itself would use: the provider
// registry is composed from the same configuration and environment precedence
// the run composes (environment over configuration over built-in default), so
// the effective endpoint only chooses which runtime is asked. The endpoint is
// never locality evidence, never part of an evidence key, and never persisted.

import (
	"context"
	"errors"
	"strings"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/model"
	"github.com/imhttran/agentic-sop/internal/provider"
	"github.com/imhttran/agentic-sop/internal/provider/command"
	"github.com/imhttran/agentic-sop/internal/provider/llamacpp"
	"github.com/imhttran/agentic-sop/internal/provider/mlx"
	"github.com/imhttran/agentic-sop/internal/provider/ollama"
	"github.com/imhttran/agentic-sop/internal/provider/openai"
)

// localityObserver reports the locality one provider runtime authoritatively
// reports for a provider/model reference. ok is false when the runtime supplies
// no evidence (unreachable runtime, absent model, unsupported discovery): the
// helper then keeps the original resolution error and never guesses. Production
// wiring and tests inject observers, so the helper itself performs no I/O setup.
type localityObserver func(ctx context.Context, ref model.LocalityRef) (model.Locality, bool)

// runtimeLocalityObserver returns the production observer. It asks the provider
// runtime the run itself would use, resolved through the same effective endpoint
// precedence (environment over configuration over built-in default) that the
// execution stack exposes. It reports the locality through the provider's
// authoritative LocalityReporter contract: the endpoint chooses which runtime is
// asked and is never read as evidence. Each probe is bounded by the shared
// provider timeout.
func runtimeLocalityObserver(cfg config.Config) localityObserver {
	stack := resolveExecutionStack(cfg)
	return func(ctx context.Context, ref model.LocalityRef) (model.Locality, bool) {
		// Prefer the execution stack's resolved endpoint for the run-level provider;
		// a routed class on another provider gets that provider's effective endpoint.
		endpoint := effectiveEndpoint(ref.Provider, cfg)
		if strings.EqualFold(strings.TrimSpace(ref.Provider), stack.Provider) {
			endpoint = stack.Endpoint
		}
		p, ok := localityProvider(ref.Provider, endpoint, ref.Model)
		if !ok {
			return "", false
		}
		reporter, ok := p.(provider.LocalityReporter)
		if !ok {
			return "", false
		}
		probeCtx, cancel := context.WithTimeout(ctx, providerProbeTimeout)
		defer cancel()
		loc := reporter.Locality(probeCtx, strings.TrimSpace(ref.Model))
		if loc != model.LocalityLocal && loc != model.LocalityCloud {
			return "", false
		}
		return loc, true
	}
}

// localityProvider builds the runtime adapter for one provider at its effective
// endpoint. A provider without an observable runtime (or an unknown identity)
// reports no observer, so resolution keeps its actionable unknown-locality error
// rather than guessing.
func localityProvider(name, endpoint, modelName string) (provider.Provider, bool) {
	switch provider.ID(strings.TrimSpace(name)) {
	case provider.Ollama:
		return ollama.New(endpoint, providerProbeTimeout), true
	case provider.LlamaCPP:
		return llamacpp.New(endpoint, strings.TrimSpace(modelName), providerProbeTimeout), true
	case provider.MLX:
		return mlx.New(endpoint, providerProbeTimeout), true
	case provider.OpenaiCompatible:
		return openai.New(provider.OpenaiCompatible, endpoint, providerProbeTimeout), true
	case provider.Command:
		return command.New(), true
	default:
		return nil, false
	}
}

// resolveModelLocality resolves a model selection for one CLI seam, adding
// bounded runtime evidence only when resolution reports an unknown locality.
// Evidence the caller already supplied in in.LocalityEvidence is kept.
//
// The loop is bounded: every distinct provider/model reference the resolve
// errors report is observed at most once, so the loop runs at most once per
// unresolved reference. When observation adds no new evidence, the helper
// returns the *model.LocalityUnknownError it tried to fix — the actionable error
// naming class, role, provider, model, and the exact settings to set — and never
// guesses a locality. Every other error, including a *model.LocalityConflictError,
// returns unchanged and before any probe or agent work.
func resolveModelLocality(ctx context.Context, in model.Inputs, observe localityObserver) (model.Result, error) {
	if observe == nil {
		// No observation seam: resolve exactly as before and fail closed on an
		// unknown locality.
		return model.Resolve(in)
	}

	evidence := make(map[model.LocalityEvidence]model.Locality, len(in.LocalityEvidence))
	for k, v := range in.LocalityEvidence {
		if strings.TrimSpace(k.Provider) == "" || strings.TrimSpace(k.Model) == "" || !v.Valid() {
			continue
		}
		evidence[k] = v
	}
	// observed tracks every reference already asked, so each reported reference
	// is observed at most once and the loop stays finite.
	observed := make(map[model.LocalityEvidence]bool)

	for {
		inputs := in
		inputs.LocalityEvidence = evidence
		res, err := model.Resolve(inputs)
		if err == nil {
			return res, nil
		}
		var unknown *model.LocalityUnknownError
		if !errors.As(err, &unknown) {
			// A conflict or any other resolution failure fails closed here,
			// before any probe and before any agent work.
			return model.Result{}, err
		}

		// Observe only the references the error reports, each at most once.
		added := 0
		for _, ref := range unknown.Refs {
			key := model.LocalityEvidence{
				Provider: strings.TrimSpace(ref.Provider),
				Model:    strings.TrimSpace(ref.Model),
			}
			if key.Provider == "" || key.Model == "" || observed[key] {
				continue
			}
			observed[key] = true
			loc, ok := observe(ctx, ref)
			if !ok || !loc.Valid() {
				continue
			}
			evidence[key] = loc
			added++
		}
		if added == 0 {
			// No new evidence: return the original actionable unknown-locality
			// error instead of looping or guessing.
			return model.Result{}, err
		}
	}
}
