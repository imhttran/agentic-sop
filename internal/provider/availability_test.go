package provider_test

import (
	"context"
	"testing"

	"github.com/imhttran/agentic-sop/internal/model"
	"github.com/imhttran/agentic-sop/internal/provider"
)

// TestResolveAvailabilityPrimary pins the happy path: a healthy runtime serving
// the configured local model resolves to the primary with no fallback.
func TestResolveAvailabilityPrimary(t *testing.T) {
	p := &stubProvider{
		id:     provider.Ollama,
		health: provider.HealthResult{Status: provider.HealthHealthy},
		models: []provider.ModelInfo{{Name: "qwen3:4b", Provider: provider.Ollama}},
		caps:   provider.Capabilities{Chat: provider.CapYes},
	}
	av := provider.ResolveAvailability(context.Background(), registryWith(p), ollamaSelection("qwen3:4b"))
	if av.Source != provider.AvailabilityPrimary {
		t.Fatalf("source = %q, want primary", av.Source)
	}
	if av.RuntimeUnreachable {
		t.Fatal("a healthy runtime must not be flagged unreachable")
	}
}

// TestResolveAvailabilityModelAbsentTriggersFallback pins the one positive
// fallback trigger: a HEALTHY runtime that does not list the configured local
// model.
func TestResolveAvailabilityModelAbsentTriggersFallback(t *testing.T) {
	p := &stubProvider{
		id:     provider.Ollama,
		health: provider.HealthResult{Status: provider.HealthHealthy},
		models: []provider.ModelInfo{{Name: "other:1b", Provider: provider.Ollama}},
		caps:   provider.Capabilities{Chat: provider.CapUnknown},
	}
	av := provider.ResolveAvailability(context.Background(), registryWith(p), ollamaSelection("qwen3:4b"))
	if av.Source != provider.AvailabilityFallback {
		t.Fatalf("source = %q, want availability-fallback", av.Source)
	}
	if av.RuntimeUnreachable {
		t.Fatal("a model-absent verdict must not be flagged unreachable")
	}
}

// TestResolveAvailabilityRuntimeUnreachableIsNotFallback is the load-bearing
// distinction: a down Ollama daemon (http://127.0.0.1:11434 unreachable) MUST NOT
// resolve to the cloud fallback, because a cloud-hosted Ollama model uses the same
// execution path. It resolves to the primary with RuntimeUnreachable set, so the
// caller surfaces the existing provider-unavailable error instead of swapping in
// nemotron-3-nano:30b-cloud.
func TestResolveAvailabilityRuntimeUnreachableIsNotFallback(t *testing.T) {
	down := &stubProvider{id: provider.Ollama, health: provider.HealthResult{Status: provider.HealthUnavailable, Message: "refused"}}
	av := provider.ResolveAvailability(context.Background(), registryWith(down), ollamaSelection("qwen3:4b"))
	if av.Source == provider.AvailabilityFallback {
		t.Fatal("an unreachable runtime must never resolve to the cloud fallback")
	}
	if !av.RuntimeUnreachable {
		t.Fatal("an unreachable runtime must be flagged RuntimeUnreachable")
	}
	if err := provider.AvailabilityError(ollamaSelection("qwen3:4b")); err == nil {
		t.Fatal("the runtime-unreachable verdict must render a provider-unavailable error")
	}
}

// TestResolveAvailabilityConservative pins that an undetermined observation keeps
// the local model: it is never read as an outage, so the fallback cannot fire on
// missing information.
func TestResolveAvailabilityConservative(t *testing.T) {
	p := &stubProvider{
		id:        provider.Ollama,
		health:    provider.HealthResult{Status: provider.HealthUnknown},
		modelsErr: provider.ErrDiscoveryUnsupported,
		caps:      provider.Capabilities{Chat: provider.CapUnknown},
	}
	av := provider.ResolveAvailability(context.Background(), registryWith(p), ollamaSelection("anything"))
	if av.Source == provider.AvailabilityFallback {
		t.Fatal("an undetermined observation must keep the local model")
	}
}

// TestResolveAvailabilityKnownIncapableTriggersFallback pins that a definitely
// known missing chat capability is a fallback trigger.
func TestResolveAvailabilityKnownIncapableTriggersFallback(t *testing.T) {
	p := &stubProvider{
		id:        provider.Ollama,
		health:    provider.HealthResult{Status: provider.HealthHealthy},
		modelsErr: provider.ErrDiscoveryUnsupported,
		caps:      provider.Capabilities{Chat: provider.CapNo},
	}
	av := provider.ResolveAvailability(context.Background(), registryWith(p), ollamaSelection("m"))
	if av.Source != provider.AvailabilityFallback {
		t.Fatalf("source = %q, want availability-fallback", av.Source)
	}
}

// TestResolveAvailabilityNilRegistryIsConservative proves a missing registry is
// never read as an outage.
func TestResolveAvailabilityNilRegistryIsConservative(t *testing.T) {
	av := provider.ResolveAvailability(context.Background(), nil, model.Selection{Provider: "ollama", Model: "m"})
	if av.Source == provider.AvailabilityFallback || av.RuntimeUnreachable {
		t.Fatalf("a nil registry must be conservative, got %+v", av)
	}
}
