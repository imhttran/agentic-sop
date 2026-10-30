package cli

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/model"
)

// localityLookup returns an environment lookup backed by a map.
func localityLookup(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

// noProbe is a localityObserver that fails the test if it is called.
func noProbe(t *testing.T) localityObserver {
	t.Helper()
	return func(context.Context, model.LocalityRef) (model.Locality, bool) {
		t.Fatal("locality observation must not run when explicit or inferred locality resolves")
		return "", false
	}
}

func TestResolveModelLocalityExplicitDoesNotProbe(t *testing.T) {
	route := model.Route{Small: model.ClassConfig{
		Provider: "ollama", Name: "qwen3:4b", Locality: model.LocalityLocal,
	}}
	res, err := resolveModelLocality(context.Background(), model.Inputs{
		Config:   route,
		Lookup:   localityLookup(nil),
		CLIClass: "small",
	}, noProbe(t))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if res.Selection.Locality != model.LocalityLocal || res.Selection.LocalitySource != model.LocalitySourceExplicit {
		t.Fatalf("selection = %+v, want local/explicit", res.Selection)
	}
}

func TestResolveModelLocalityCloudSuffixDoesNotProbe(t *testing.T) {
	route := model.Route{Medium: model.ClassConfig{Provider: "ollama", Name: "deepseek-v4.1-flash:cloud"}}
	res, err := resolveModelLocality(context.Background(), model.Inputs{
		Config:   route,
		Lookup:   localityLookup(nil),
		CLIClass: "medium",
	}, noProbe(t))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if res.Selection.Locality != model.LocalityCloud || res.Selection.LocalitySource != model.LocalitySourceInferred {
		t.Fatalf("selection = %+v, want cloud/inferred", res.Selection)
	}
}

func TestResolveModelLocalityUsesRuntimeEvidence(t *testing.T) {
	route := model.Route{Small: model.ClassConfig{Provider: "ollama", Name: "qwen3:4b"}}
	var refs []model.LocalityRef
	observe := func(_ context.Context, ref model.LocalityRef) (model.Locality, bool) {
		refs = append(refs, ref)
		return model.LocalityLocal, true
	}
	res, err := resolveModelLocality(context.Background(), model.Inputs{
		Config:   route,
		Lookup:   localityLookup(nil),
		CLIClass: "small",
	}, observe)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if res.Selection.Locality != model.LocalityLocal || res.Selection.LocalitySource != model.LocalitySourceRuntime {
		t.Fatalf("selection = %+v, want local/runtime", res.Selection)
	}
	if len(refs) != 1 || refs[0].Class != model.ClassSmall || refs[0].Role != model.LocalityRolePrimary || refs[0].Model != "qwen3:4b" {
		t.Fatalf("observed refs = %+v, want one SMALL primary reference", refs)
	}
}

func TestResolveModelLocalityUnknownWithoutEvidenceIsActionable(t *testing.T) {
	route := model.Route{Small: model.ClassConfig{Provider: "ollama", Name: "qwen3:4b"}}
	probes := 0
	_, err := resolveModelLocality(context.Background(), model.Inputs{
		Config:   route,
		Lookup:   localityLookup(nil),
		CLIClass: "small",
	}, func(context.Context, model.LocalityRef) (model.Locality, bool) {
		probes++
		return "", false
	})
	var unknown *model.LocalityUnknownError
	if !errors.As(err, &unknown) {
		t.Fatalf("error = %v, want *LocalityUnknownError", err)
	}
	if probes != 1 {
		t.Fatalf("probes = %d, want exactly one bounded observation", probes)
	}
	for _, want := range []string{"small", "qwen3:4b", "models.small.locality", "SOP_MODEL_SMALL_LOCALITY"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q must name %q", err, want)
		}
	}
}

func TestResolveModelLocalityConflictFailsBeforeProbe(t *testing.T) {
	route := model.Route{Small: model.ClassConfig{
		Provider: "ollama", Name: "qwen3:4b", Locality: model.LocalityLocal,
	}}
	_, err := resolveModelLocality(context.Background(), model.Inputs{
		Config:   route,
		Lookup:   localityLookup(nil),
		CLIClass: "small",
		LocalityEvidence: map[model.LocalityEvidence]model.Locality{
			{Provider: "ollama", Model: "qwen3:4b"}: model.LocalityCloud,
		},
	}, noProbe(t))
	var conflict *model.LocalityConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("error = %v, want *LocalityConflictError", err)
	}
}

func TestResolveModelLocalityFallbackUsesSameResolver(t *testing.T) {
	route := model.Route{Small: model.ClassConfig{
		Provider: "ollama", Name: "qwen3:4b", Locality: model.LocalityLocal,
		Fallback: &model.FallbackConfig{Provider: "ollama", Name: "my-cloud-model"},
	}}
	var refs []model.LocalityRef
	observe := func(_ context.Context, ref model.LocalityRef) (model.Locality, bool) {
		refs = append(refs, ref)
		return model.LocalityCloud, true
	}
	res, err := resolveModelLocality(context.Background(), model.Inputs{
		Config:   route,
		Lookup:   localityLookup(nil),
		CLIClass: "small",
	}, observe)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if res.LocalFallback == nil {
		t.Fatal("the fallback candidate must be reported")
	}
	if res.LocalFallback.Locality != model.LocalityCloud || res.LocalFallback.LocalitySource != model.LocalitySourceRuntime {
		t.Fatalf("fallback = %+v, want cloud/runtime", *res.LocalFallback)
	}
	if len(refs) != 1 || refs[0].Role != model.LocalityRoleFallback || refs[0].Model != "my-cloud-model" {
		t.Fatalf("observed refs = %+v, want one SMALL fallback reference", refs)
	}
}

func TestResolveModelLocalityNilObserverFailsClosed(t *testing.T) {
	route := model.Route{Small: model.ClassConfig{Provider: "ollama", Name: "qwen3:4b"}}
	_, err := resolveModelLocality(context.Background(), model.Inputs{
		Config:   route,
		Lookup:   localityLookup(nil),
		CLIClass: "small",
	}, nil)
	var unknown *model.LocalityUnknownError
	if !errors.As(err, &unknown) {
		t.Fatalf("error = %v, want *LocalityUnknownError", err)
	}
}

func TestResolveModelLocalityEnvironmentOverridesYAML(t *testing.T) {
	route := model.Route{Small: model.ClassConfig{
		Provider: "ollama", Name: "qwen3:4b", Locality: model.LocalityLocal,
	}}
	env := map[string]string{
		model.ClassEnvKey(model.ClassSmall, "LOCALITY"): "cloud",
	}
	res, err := resolveModelLocality(context.Background(), model.Inputs{
		Config:   route,
		Lookup:   localityLookup(env),
		CLIClass: "small",
	}, noProbe(t))
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if res.Selection.Locality != model.LocalityCloud || res.Selection.LocalitySource != model.LocalitySourceExplicit {
		t.Fatalf("selection = %+v, want cloud/explicit from the environment override", res.Selection)
	}
}

func TestResolveModelLocalityUsesConfiguredEndpointOnlyToSelectRuntime(t *testing.T) {
	// The observable contract is the provider/model evidence key: the observer is
	// asked with a LocalityRef, not an endpoint, so an endpoint can never become
	// evidence. This test documents that boundary with an explicit locality.
	cfg := config.Config{}
	if endpoint := effectiveEndpoint("ollama", cfg); endpoint == "" {
		t.Fatal("the ollama endpoint must resolve to a runtime-selection value")
	}
	route := model.Route{Small: model.ClassConfig{
		Provider: "ollama", Name: "qwen3:4b", Locality: model.LocalityLocal,
	}}
	if _, err := resolveModelLocality(context.Background(), model.Inputs{
		Config:   route,
		Lookup:   localityLookup(nil),
		CLIClass: "small",
	}, noProbe(t)); err != nil {
		t.Fatalf("resolve: %v", err)
	}
}
