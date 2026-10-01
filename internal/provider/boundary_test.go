package provider_test

import (
	"context"
	"reflect"
	"sort"
	"strings"
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

// authorityVerbs are method-name fragments that would indicate the provider layer
// had grown lifecycle, approval, routing, or repository authority. It is a
// heuristic guardrail, not a proof: it catches an obvious boundary violation early.
var authorityVerbs = []string{
	"Approve", "Reject", "Transition", "Advance", "Execute", "Generate",
	"Route", "SelectClass", "SelectModel", "Fallback", "Commit", "Push",
	"Merge", "Write", "Mutate", "SetState", "SetStatus",
}

// assertNoAuthorityMethods fails if t exposes a method whose name suggests a
// capability the provider layer must never have.
func assertNoAuthorityMethods(t *testing.T, name string, typ reflect.Type) {
	t.Helper()
	for i := 0; i < typ.NumMethod(); i++ {
		method := typ.Method(i).Name
		for _, verb := range authorityVerbs {
			if strings.Contains(method, verb) {
				t.Errorf("%s.%s looks like lifecycle/authority: providers are read-only observers", name, method)
			}
		}
	}
}

// TestProviderTypesCarryNoAuthorityMethods proves the exported provider types
// expose no method that could approve, reject, transition state, route, or mutate
// the repository: capability discovery and model listing are evidence only and
// cannot bypass approval or drive the lifecycle.
func TestProviderTypesCarryNoAuthorityMethods(t *testing.T) {
	providerType := reflect.TypeOf((*provider.Provider)(nil)).Elem()
	assertNoAuthorityMethods(t, "Provider", providerType)
	assertNoAuthorityMethods(t, "Registry", reflect.TypeOf((*provider.Registry)(nil)))
	assertNoAuthorityMethods(t, "Capabilities", reflect.TypeOf(provider.Capabilities{}))
	assertNoAuthorityMethods(t, "ModelInfo", reflect.TypeOf(provider.ModelInfo{}))
	assertNoAuthorityMethods(t, "HealthResult", reflect.TypeOf(provider.HealthResult{}))
}

// TestRegistryHasNoLifecycleAuthority freezes the Registry method set. A registry
// is constructed and inspected, never driven: if a future change adds an
// execute/transition/approve method, this test fails.
func TestRegistryHasNoLifecycleAuthority(t *testing.T) {
	typ := reflect.TypeOf((*provider.Registry)(nil))
	var names []string
	for i := 0; i < typ.NumMethod(); i++ {
		names = append(names, typ.Method(i).Name)
	}
	sort.Strings(names)
	want := []string{"Get", "IDs", "Len", "List", "Register"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("Registry methods = %v, want %v", names, want)
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
