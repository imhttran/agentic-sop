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

// TestLocalUsable pins the availability probe behind the local-first cloud
// fallback. It is exactly ValidateSelection's read-only verdict, and it is
// conservative: a runtime whose reachability cannot be determined, or that cannot
// enumerate models, is NOT reported unusable, so the fallback triggers only on a
// positive observation.
func TestLocalUsable(t *testing.T) {
	healthy := &stubProvider{
		id:     provider.Ollama,
		health: provider.HealthResult{Status: provider.HealthHealthy},
		models: []provider.ModelInfo{{Name: "qwen3:4b", Provider: provider.Ollama}},
		caps:   provider.Capabilities{Chat: provider.CapYes},
	}
	if ok, detail := provider.LocalUsable(context.Background(), registryWith(healthy), ollamaSelection("qwen3:4b")); !ok {
		t.Fatalf("a reachable runtime with the model must be usable: %s", detail)
	}

	t.Run("reachable but model absent", func(t *testing.T) {
		ok, detail := provider.LocalUsable(context.Background(), registryWith(healthy), ollamaSelection("missing:1b"))
		if ok || detail == "" {
			t.Fatalf("an absent local model must be unusable with a detail, got (%v, %q)", ok, detail)
		}
	})

	t.Run("unreachable", func(t *testing.T) {
		down := &stubProvider{id: provider.Ollama, health: provider.HealthResult{Status: provider.HealthUnavailable, Message: "refused"}}
		ok, detail := provider.LocalUsable(context.Background(), registryWith(down), ollamaSelection("qwen3:4b"))
		if ok || detail == "" {
			t.Fatalf("an unreachable runtime must be unusable with a detail, got (%v, %q)", ok, detail)
		}
	})

	t.Run("undetermined reachability is usable", func(t *testing.T) {
		p := &stubProvider{
			id:        provider.Ollama,
			health:    provider.HealthResult{Status: provider.HealthUnknown},
			modelsErr: provider.ErrDiscoveryUnsupported,
			caps:      provider.Capabilities{Chat: provider.CapUnknown},
		}
		if ok, detail := provider.LocalUsable(context.Background(), registryWith(p), ollamaSelection("anything")); !ok {
			t.Fatalf("undetermined availability must never be read as unusable: %s", detail)
		}
	})

	t.Run("known-incapable model is unusable", func(t *testing.T) {
		p := &stubProvider{
			id:        provider.Ollama,
			health:    provider.HealthResult{Status: provider.HealthHealthy},
			modelsErr: provider.ErrDiscoveryUnsupported,
			caps:      provider.Capabilities{Chat: provider.CapNo},
		}
		if ok, _ := provider.LocalUsable(context.Background(), registryWith(p), ollamaSelection("m")); ok {
			t.Fatal("a model that definitely cannot chat must be unusable")
		}
	})
}
