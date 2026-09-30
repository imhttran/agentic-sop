package provider_test

import (
	"context"
	"testing"

	"github.com/imhttran/agentic-sop/internal/provider"
)

func TestCapTriState(t *testing.T) {
	if provider.CapUnknown.Known() || provider.CapUnknown.Present() {
		t.Fatal("unknown must be neither known nor present")
	}
	if !provider.CapNo.Known() || provider.CapNo.Present() {
		t.Fatal("no must be known and not present")
	}
	if !provider.CapYes.Known() || !provider.CapYes.Present() {
		t.Fatal("yes must be known and present")
	}
}

func TestCapStringDefaultsUnknown(t *testing.T) {
	var zero provider.Cap
	if zero.String() != "unknown" {
		t.Fatalf("zero Cap = %q, want unknown", zero.String())
	}
}

func TestCapabilitiesStringOmitsUnknown(t *testing.T) {
	caps := provider.Capabilities{Chat: provider.CapYes, Tools: provider.CapUnknown, Streaming: provider.CapNo}
	got := caps.String()
	if got != "chat=yes streaming=no" {
		t.Fatalf("Capabilities.String() = %q", got)
	}
}

func TestCapabilitiesStringNoneDetermined(t *testing.T) {
	var caps provider.Capabilities
	if got := caps.String(); got != "none determined" {
		t.Fatalf("empty Capabilities.String() = %q", got)
	}
}

// stubProvider is a configurable in-memory Provider for the boundary tests.
type stubProvider struct {
	id        provider.ID
	health    provider.HealthResult
	models    []provider.ModelInfo
	modelsErr error
	caps      provider.Capabilities
	capsErr   error
}

func (s *stubProvider) ID() provider.ID { return s.id }

func (s *stubProvider) Health(context.Context) provider.HealthResult { return s.health }

func (s *stubProvider) Models(context.Context) ([]provider.ModelInfo, error) {
	return s.models, s.modelsErr
}

func (s *stubProvider) Capabilities(context.Context, string) (provider.Capabilities, error) {
	return s.caps, s.capsErr
}
