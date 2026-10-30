package provider_test

import (
	"context"
	"testing"

	"github.com/imhttran/agentic-sop/internal/model"
	"github.com/imhttran/agentic-sop/internal/provider"
)

// locStubProvider is a registered provider without the LocalityReporter
// contract: its discovery cannot place a model, so the lookup must report
// not-determined rather than guessing.
type locStubProvider struct{}

func (locStubProvider) ID() provider.ID { return provider.OpenaiCompatible }
func (locStubProvider) Health(_ context.Context) provider.HealthResult {
	return provider.HealthResult{Status: provider.HealthHealthy, Message: "ok"}
}
func (locStubProvider) Models(_ context.Context) ([]provider.ModelInfo, error) {
	return []provider.ModelInfo{{Name: "stub-model", Provider: provider.OpenaiCompatible}}, nil
}
func (locStubProvider) Capabilities(_ context.Context, _ string) (provider.Capabilities, error) {
	return provider.Capabilities{}, nil
}

// locReportingProvider implements LocalityReporter and reports locality only
// for the model it actually serves. Every other name is an absent model. With
// fail set, the evidence request fails.
type locReportingProvider struct {
	served string
	fail   bool
}

func (p locReportingProvider) ID() provider.ID { return provider.Ollama }
func (p locReportingProvider) Health(_ context.Context) provider.HealthResult {
	return provider.HealthResult{Status: provider.HealthHealthy, Message: "ok"}
}
func (p locReportingProvider) Models(_ context.Context) ([]provider.ModelInfo, error) {
	return []provider.ModelInfo{{Name: p.served, Provider: provider.Ollama}}, nil
}
func (p locReportingProvider) Capabilities(_ context.Context, _ string) (provider.Capabilities, error) {
	return provider.Capabilities{}, nil
}
func (p locReportingProvider) Locality(_ context.Context, name string) model.Locality {
	if p.fail || name != p.served {
		return ""
	}
	return model.LocalityCloud
}

func newLocRegistry(t *testing.T, ps ...provider.Provider) *provider.Registry {
	t.Helper()
	reg := provider.NewRegistry()
	for _, p := range ps {
		if err := reg.Register(p); err != nil {
			t.Fatalf("register %s: %v", p.ID(), err)
		}
	}
	return reg
}

func TestModelLocalityReturnsOnlyProviderReportedValue(t *testing.T) {
	reg := newLocRegistry(t, locReportingProvider{served: "gpt-oss:120b"})
	got := provider.ModelLocality(context.Background(), reg, provider.Ollama, "gpt-oss:120b")
	if got != model.LocalityCloud {
		t.Errorf("ModelLocality(ollama, gpt-oss:120b) = %q, want %q", got, model.LocalityCloud)
	}
}

func TestModelLocalityNotDetermined(t *testing.T) {
	reg := newLocRegistry(t, locReportingProvider{served: "gpt-oss:120b"}, locStubProvider{})
	failReg := newLocRegistry(t, locReportingProvider{served: "gpt-oss:120b", fail: true})
	ctx := context.Background()
	tests := []struct {
		name string
		reg  *provider.Registry
		id   provider.ID
		mdl  string
	}{
		{"unknown provider", reg, provider.ID("not-a-provider"), "gpt-oss:120b"},
		{"unregistered provider", reg, provider.LlamaCPP, "gpt-oss:120b"},
		{"unsupported discovery", reg, provider.OpenaiCompatible, "gpt-oss:120b"},
		{"failed request", failReg, provider.Ollama, "gpt-oss:120b"},
		{"absent model", reg, provider.Ollama, "qwen3:8b"},
		{"empty model name", reg, provider.Ollama, "   "},
	}
	for _, tc := range tests {
		if got := provider.ModelLocality(ctx, tc.reg, tc.id, tc.mdl); got != "" {
			t.Errorf("%s: ModelLocality(%s, %q) = %q, want undetermined", tc.name, tc.id, tc.mdl, got)
		}
	}
	if got := provider.ModelLocality(ctx, nil, provider.Ollama, "gpt-oss:120b"); got != "" {
		t.Errorf("nil registry: ModelLocality = %q, want undetermined", got)
	}
}

// TestModelLocalityNeverSubstitutesGuesses guards the evidence boundary: no
// endpoint host, no missing ":cloud" suffix, and no model name such as Claude
// may ever fill a missing provider report.
func TestModelLocalityNeverSubstitutesGuesses(t *testing.T) {
	reg := newLocRegistry(t, locReportingProvider{served: "gpt-oss:120b"}, locStubProvider{})
	ctx := context.Background()
	tests := []struct {
		id  provider.ID
		mdl string
	}{
		{provider.OpenaiCompatible, "claude-3-5-sonnet:cloud"},
		{provider.OpenaiCompatible, "claude-3-5-sonnet"},
		{provider.Ollama, "claude-3-5-sonnet:cloud"},
		{provider.Ollama, "qwen3:8b:cloud"},
	}
	for _, tc := range tests {
		if got := provider.ModelLocality(ctx, reg, tc.id, tc.mdl); got != "" {
			t.Errorf("ModelLocality(%s, %q) = %q, want undetermined: a name is never evidence", tc.id, tc.mdl, got)
		}
	}
}
