package provider_test

import (
	"context"
	"errors"
	"testing"

	"github.com/imhttran/agentic-sop/internal/model"
	"github.com/imhttran/agentic-sop/internal/provider"
)

func ollamaSelection(modelName string) model.Selection {
	return model.Selection{Provider: string(provider.Ollama), Model: modelName}
}

func registryWith(p *stubProvider) *provider.Registry {
	reg := provider.NewRegistry()
	if err := reg.Register(p); err != nil {
		panic(err)
	}
	return reg
}

func TestValidateSelectionPasses(t *testing.T) {
	p := &stubProvider{
		id:     provider.Ollama,
		health: provider.HealthResult{Status: provider.HealthHealthy},
		models: []provider.ModelInfo{{Name: "qwen3:4b", Provider: provider.Ollama}},
		caps:   provider.Capabilities{Chat: provider.CapYes},
	}
	if err := provider.ValidateSelection(context.Background(), registryWith(p), ollamaSelection("qwen3:4b")); err != nil {
		t.Fatalf("ValidateSelection: %v", err)
	}
}

func TestValidateSelectionUnknownProvider(t *testing.T) {
	reg := provider.NewRegistry()
	err := provider.ValidateSelection(context.Background(), reg, model.Selection{Provider: "vllm", Model: "x"})
	if !errors.Is(err, provider.ErrUnknownProvider) {
		t.Fatalf("err = %v, want ErrUnknownProvider", err)
	}
}

func TestValidateSelectionNotRegistered(t *testing.T) {
	err := provider.ValidateSelection(context.Background(), provider.NewRegistry(), ollamaSelection("qwen3:4b"))
	if !errors.Is(err, provider.ErrNotRegistered) {
		t.Fatalf("err = %v, want ErrNotRegistered", err)
	}
}

func TestValidateSelectionNoProviderNamed(t *testing.T) {
	reg := provider.NewRegistry()
	err := provider.ValidateSelection(context.Background(), reg, model.Selection{Model: "x"})
	if err == nil {
		t.Fatal("a selection with no provider must fail")
	}
}

func TestValidateSelectionUnavailable(t *testing.T) {
	p := &stubProvider{id: provider.Ollama, health: provider.HealthResult{Status: provider.HealthUnavailable, Message: "refused"}}
	err := provider.ValidateSelection(context.Background(), registryWith(p), ollamaSelection("qwen3:4b"))
	if !errors.Is(err, provider.ErrProviderUnavailable) {
		t.Fatalf("err = %v, want ErrProviderUnavailable", err)
	}
}

func TestValidateSelectionModelAbsentFails(t *testing.T) {
	p := &stubProvider{
		id:     provider.Ollama,
		health: provider.HealthResult{Status: provider.HealthHealthy},
		models: []provider.ModelInfo{{Name: "qwen3:4b", Provider: provider.Ollama}},
		caps:   provider.Capabilities{Chat: provider.CapYes},
	}
	err := provider.ValidateSelection(context.Background(), registryWith(p), ollamaSelection("missing:1b"))
	if !errors.Is(err, provider.ErrModelNotFound) {
		t.Fatalf("err = %v, want ErrModelNotFound", err)
	}
}

func TestValidateSelectionDiscoveryUnsupportedDoesNotClaimAbsence(t *testing.T) {
	p := &stubProvider{
		id:        provider.Ollama,
		health:    provider.HealthResult{Status: provider.HealthHealthy},
		modelsErr: provider.ErrDiscoveryUnsupported,
		caps:      provider.Capabilities{Chat: provider.CapUnknown},
	}
	if err := provider.ValidateSelection(context.Background(), registryWith(p), ollamaSelection("anything")); err != nil {
		t.Fatalf("unsupported discovery must not report absence: %v", err)
	}
}

func TestValidateSelectionUnknownCapabilityIsNotMissing(t *testing.T) {
	p := &stubProvider{
		id:     provider.Ollama,
		health: provider.HealthResult{Status: provider.HealthUnknown},
		models: []provider.ModelInfo{{Name: "m", Provider: provider.Ollama}},
		caps:   provider.Capabilities{Chat: provider.CapUnknown},
	}
	if err := provider.ValidateSelection(context.Background(), registryWith(p), ollamaSelection("m")); err != nil {
		t.Fatalf("an unknown capability must not fail validation: %v", err)
	}
}

func TestValidateSelectionCapabilityAbsentFails(t *testing.T) {
	p := &stubProvider{
		id:        provider.Ollama,
		health:    provider.HealthResult{Status: provider.HealthHealthy},
		modelsErr: provider.ErrDiscoveryUnsupported,
		caps:      provider.Capabilities{Chat: provider.CapNo},
	}
	err := provider.ValidateSelection(context.Background(), registryWith(p), ollamaSelection("m"))
	if err == nil || errors.Is(err, provider.ErrModelNotFound) {
		t.Fatalf("err = %v, want a definite chat-capability failure", err)
	}
}

func TestValidateSelectionEmptyModelFails(t *testing.T) {
	p := &stubProvider{id: provider.Ollama, health: provider.HealthResult{Status: provider.HealthHealthy}}
	err := provider.ValidateSelection(context.Background(), registryWith(p), ollamaSelection(""))
	if !errors.Is(err, provider.ErrModelRequired) {
		t.Fatalf("err = %v, want ErrModelRequired", err)
	}
}

func TestValidateSelectionNilRegistry(t *testing.T) {
	if err := provider.ValidateSelection(context.Background(), nil, ollamaSelection("m")); err == nil {
		t.Fatal("a nil registry must fail")
	}
}

func TestValidateSelectionDiscoveryErrorFails(t *testing.T) {
	p := &stubProvider{
		id:        provider.Ollama,
		health:    provider.HealthResult{Status: provider.HealthHealthy},
		modelsErr: errors.New("boom"),
	}
	err := provider.ValidateSelection(context.Background(), registryWith(p), ollamaSelection("m"))
	if err == nil || errors.Is(err, provider.ErrModelNotFound) {
		t.Fatalf("a real discovery error must fail without claiming absence: %v", err)
	}
}

// TestValidateSelectionDoesNotMutate proves validation is a pure observation: it
// returns a verdict and changes nothing about the selection it was given.
func TestValidateSelectionDoesNotMutate(t *testing.T) {
	p := &stubProvider{
		id:     provider.Ollama,
		health: provider.HealthResult{Status: provider.HealthHealthy},
		models: []provider.ModelInfo{{Name: "m", Provider: provider.Ollama}},
		caps:   provider.Capabilities{Chat: provider.CapYes},
	}
	sel := ollamaSelection("m")
	before := sel
	_ = provider.ValidateSelection(context.Background(), registryWith(p), sel)
	if sel != before {
		t.Fatalf("selection mutated: %+v -> %+v", before, sel)
	}
}
