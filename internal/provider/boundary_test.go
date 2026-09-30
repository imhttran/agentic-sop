package provider_test

import (
	"context"
	"reflect"
	"sort"
	"testing"

	"github.com/imhttran/agentic-sop/internal/provider"
)

// TestProviderInterfaceHasNoAuthority freezes the Provider contract. The method
// set is exactly the four read-only observations; if a future change adds an
// execution, routing, approval, or state-transition method, this test fails and
// forces the boundary to be reconsidered rather than drifting.
func TestProviderInterfaceHasNoAuthority(t *testing.T) {
	typ := reflect.TypeOf((*provider.Provider)(nil)).Elem()
	var names []string
	for i := 0; i < typ.NumMethod(); i++ {
		names = append(names, typ.Method(i).Name)
	}
	sort.Strings(names)
	want := []string{"Capabilities", "Health", "ID", "Models"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("Provider methods = %v, want %v", names, want)
	}
}

// TestHealthCannotSelectAClass proves a provider's health is inert with respect
// to routing: the same selection validates the same way whatever the health
// status, and a healthy provider never yields a class (the API cannot).
func TestHealthCannotSelectAClass(t *testing.T) {
	for _, status := range []provider.HealthStatus{
		provider.HealthHealthy, provider.HealthUnknown, provider.HealthDegraded,
	} {
		p := &stubProvider{
			id:        provider.Ollama,
			health:    provider.HealthResult{Status: status},
			modelsErr: provider.ErrDiscoveryUnsupported,
			caps:      provider.Capabilities{Chat: provider.CapUnknown},
		}
		if err := provider.ValidateSelection(context.Background(), registryWith(p), ollamaSelection("m")); err != nil {
			t.Fatalf("status %s: %v", status, err)
		}
	}
}

// TestProviderFailureCannotSubstitute proves validation reports failure rather
// than silently switching to another provider: an unavailable provider is an
// error, and the registry is unchanged afterwards.
func TestProviderFailureCannotSubstitute(t *testing.T) {
	reg := provider.NewRegistry()
	bad := &stubProvider{id: provider.Ollama, health: provider.HealthResult{Status: provider.HealthUnavailable}}
	good := &stubProvider{id: provider.LlamaCPP, health: provider.HealthResult{Status: provider.HealthHealthy}}
	if err := reg.Register(bad); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(good); err != nil {
		t.Fatal(err)
	}
	err := provider.ValidateSelection(context.Background(), reg, ollamaSelection("m"))
	if err == nil {
		t.Fatal("an unavailable provider must fail, not fall through to another")
	}
	if ids := reg.IDs(); len(ids) != 2 {
		t.Fatalf("registry mutated by validation: %v", ids)
	}
}
