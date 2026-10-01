package model_test

import (
	"testing"

	"github.com/imhttran/agentic-sop/internal/model"
	"github.com/imhttran/agentic-sop/internal/provider"
)

// TestSmallRouteOpenAICompatibleParsesUnchanged locks the exact Phase 1 SMALL
// routing contract: SOP_MODEL_SMALL_PROVIDER=openai_compatible with the given
// name and locality parses and resolves verbatim. Phase 1 does not infer
// locality, does not reroute, and does not execute the provider.
func TestSmallRouteOpenAICompatibleParsesUnchanged(t *testing.T) {
	env := map[string]string{
		model.ClassEnvKey(model.ClassSmall, "PROVIDER"): string(provider.OpenaiCompatible),
		model.ClassEnvKey(model.ClassSmall, "NAME"):     "mlx-community/Qwen3-4B-4bit",
		model.ClassEnvKey(model.ClassSmall, "LOCALITY"): "local",
	}
	// The env keys must be the documented, literal ones: no drift.
	if model.ClassEnvKey(model.ClassSmall, "PROVIDER") != "SOP_MODEL_SMALL_PROVIDER" ||
		model.ClassEnvKey(model.ClassSmall, "NAME") != "SOP_MODEL_SMALL_NAME" ||
		model.ClassEnvKey(model.ClassSmall, "LOCALITY") != "SOP_MODEL_SMALL_LOCALITY" {
		t.Fatalf("unexpected SMALL env keys: %q %q %q",
			model.ClassEnvKey(model.ClassSmall, "PROVIDER"),
			model.ClassEnvKey(model.ClassSmall, "NAME"),
			model.ClassEnvKey(model.ClassSmall, "LOCALITY"))
	}

	lookup := func(k string) string { return env[k] }
	res, err := model.Resolve(model.Inputs{Lookup: lookup, CLIClass: string(model.ClassSmall)})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if !res.Active {
		t.Fatal("routing must be active")
	}
	if res.Selection.Class != model.ClassSmall {
		t.Fatalf("class = %q, want small", res.Selection.Class)
	}
	if res.Selection.Provider != string(provider.OpenaiCompatible) {
		t.Fatalf("provider = %q, want %q", res.Selection.Provider, provider.OpenaiCompatible)
	}
	if res.Selection.Model != "mlx-community/Qwen3-4B-4bit" {
		t.Fatalf("model = %q", res.Selection.Model)
	}
	if res.Selection.Locality != model.LocalityLocal {
		t.Fatalf("locality = %q, want local (no inference)", res.Selection.Locality)
	}
}

// TestSmallRouteProviderIsCanonical proves the SMALL routing env accepts the
// canonical openai_compatible identity (no raw string duplication) and rejects
// an unknown one.
func TestSmallRouteProviderIsCanonical(t *testing.T) {
	base := map[string]string{
		model.ClassEnvKey(model.ClassSmall, "NAME"):     "some-model",
		model.ClassEnvKey(model.ClassSmall, "LOCALITY"): "local",
	}

	with := func(providerName string) func(string) string {
		env := make(map[string]string, len(base)+1)
		for k, v := range base {
			env[k] = v
		}
		env[model.ClassEnvKey(model.ClassSmall, "PROVIDER")] = providerName
		return func(k string) string { return env[k] }
	}

	if _, err := model.Resolve(model.Inputs{Lookup: with(string(provider.OpenaiCompatible)), CLIClass: string(model.ClassSmall)}); err != nil {
		t.Fatalf("canonical provider must resolve: %v", err)
	}
	if _, err := model.Resolve(model.Inputs{Lookup: with("not_a_provider"), CLIClass: string(model.ClassSmall)}); err == nil {
		t.Fatal("unknown routed provider must fail")
	}
}
