package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/imhttran/agentic-sop/internal/model"
)

// AvailabilitySource is the deterministic, typed provenance of an availability
// resolution: WHERE the executing model came from. It is a small closed set,
// never model-generated prose, and it mirrors model.ExecutionSource so the
// provider layer and the evidence layer agree on the vocabulary.
type AvailabilitySource string

const (
	// AvailabilityPrimary: the class's primary model can be served; it executes.
	AvailabilityPrimary AvailabilitySource = "primary"
	// AvailabilityFallback: the class's configured cloud fallback executes because
	// the local runtime is HEALTHY but cannot serve the configured local model.
	AvailabilityFallback AvailabilitySource = "availability-fallback"
)

// Availability is the outcome of resolving whether a LOCAL class's primary model
// can be served by its runtime. It distinguishes the two failure modes the spec
// requires to stay separate:
//
//   - RuntimeUnreachable: the local runtime itself could not be reached (for
//     example Ollama at http://127.0.0.1:11434 is down). A cloud-hosted Ollama
//     model still uses the same Ollama execution path, so renaming the model
//     cannot repair an unreachable endpoint. This MUST NOT select the cloud
//     fallback; it follows the existing provider/runtime-unavailable error path.
//   - ModelUnavailable: the runtime is reachable but the configured local model
//     is absent, or reports it cannot chat. This is the availability-fallback
//     trigger.
//
// It is a CANDIDATE verdict: it never substitutes a provider or model itself and
// never changes SOP state. The caller decides, deterministically, what to do.
type Availability struct {
	// Source records what should execute: the primary or the fallback.
	Source AvailabilitySource
	// Reason is a short, deterministic, NON-SECRET explanation. It is one of a
	// fixed set of phrases, never model-generated prose.
	Reason string
	// RuntimeUnreachable reports that the local runtime itself could not be
	// reached. When true the caller MUST NOT apply the cloud fallback and MUST use
	// the existing provider-unavailable error path.
	RuntimeUnreachable bool
}

// Availability reasons (deterministic, non-secret).
const (
	// ReasonPrimaryServed: the local model is present and can chat.
	ReasonPrimaryServed = "local model available"
	// ReasonModelUnavailable: the runtime is healthy but the configured local
	// model is absent, or reports it cannot chat.
	ReasonModelUnavailable = "configured local model unavailable"
	// ReasonRuntimeUnreachable: the local runtime itself could not be reached.
	ReasonRuntimeUnreachable = "local runtime unreachable"
	// ReasonNoObservation: availability could not be determined, so the local
	// model is kept (an absent observation is never read as an outage).
	ReasonNoObservation = "local availability not observed"
)

// ResolveAvailability observes whether a LOCAL selection's runtime can serve its
// configured model, using the existing provider abstractions (reachability,
// authoritative model list, definitely-known chat capability). It issues no
// generation request and makes no direct /api/* call: it drives the Provider
// interface only.
//
// It is deliberately conservative: a provider that cannot report reachability or
// enumerate models is NOT read as an outage, so the fallback triggers only on a
// positive observation. An empty/unsupported observation keeps the local model.
//
// The distinction it enforces, and that LocalUsable did not, is: a HEALTHY
// runtime that cannot serve the configured local model is a fallback trigger,
// while an UNREACHABLE runtime is not - it is a provider-unavailable error the
// caller must surface unchanged.
func ResolveAvailability(ctx context.Context, reg *Registry, sel model.Selection) Availability {
	if reg == nil || strings.TrimSpace(sel.Provider) == "" {
		return Availability{Source: AvailabilityPrimary, Reason: ReasonNoObservation}
	}
	id, err := ParseID(sel.Provider)
	if err != nil {
		return Availability{Source: AvailabilityPrimary, Reason: ReasonNoObservation}
	}
	p, err := reg.Get(id)
	if err != nil {
		return Availability{Source: AvailabilityPrimary, Reason: ReasonNoObservation}
	}
	if strings.TrimSpace(sel.Model) == "" {
		return Availability{Source: AvailabilityPrimary, Reason: ReasonNoObservation}
	}

	// Reachability first: an unreachable runtime is NOT a fallback trigger. It is
	// a provider/runtime-unavailable condition the caller surfaces through the
	// existing error path, never a cloud swap.
	if h := p.Health(ctx); h.Status == HealthUnavailable {
		return Availability{
			Source:             AvailabilityPrimary,
			Reason:             ReasonRuntimeUnreachable,
			RuntimeUnreachable: true,
		}
	}

	// Model presence, only when discovery is authoritative. A provider that cannot
	// enumerate models is not read as "absent" - it falls through to the capability
	// check, which may still establish a definite inability to chat.
	observed := true
	infos, err := p.Models(ctx)
	switch {
	case errors.Is(err, ErrDiscoveryUnsupported):
		observed = false
	case err != nil:
		observed = false
	default:
		if !containsModel(infos, sel.Model) {
			return Availability{Source: AvailabilityFallback, Reason: ReasonModelUnavailable}
		}
	}

	// A definitely-known missing chat capability is a fallback trigger; an unknown
	// capability is not.
	caps, err := p.Capabilities(ctx, sel.Model)
	if err == nil && caps.Chat.Known() && !caps.Chat.Present() {
		return Availability{Source: AvailabilityFallback, Reason: ReasonModelUnavailable}
	}
	if !observed {
		return Availability{Source: AvailabilityPrimary, Reason: ReasonNoObservation}
	}
	return Availability{Source: AvailabilityPrimary, Reason: ReasonPrimaryServed}
}

// AvailabilityError renders the runtime-unreachable verdict as the existing
// provider/runtime-unavailable error, so a down runtime surfaces the same error
// path as any other provider-unavailable condition and never a cloud swap.
func AvailabilityError(sel model.Selection) error {
	return fmt.Errorf("provider %s: %w: %s (a cloud-hosted model still uses the same execution path)",
		strings.TrimSpace(sel.Provider), ErrProviderUnavailable, ReasonRuntimeUnreachable)
}
