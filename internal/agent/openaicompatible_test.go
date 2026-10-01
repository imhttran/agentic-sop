package agent_test

import (
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/model"
	"github.com/imhttran/agentic-sop/internal/provider"
)

// clearOpenAICompatibleEnv unsets every variable the generic provider reads, so a
// developer's shell cannot leak into a resolution assertion.
func clearOpenAICompatibleEnv(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		agent.EnvAgentProvider,
		agent.EnvAgentModel,
		agent.EnvOpenAICompatibleBaseURL,
		agent.EnvOpenAICompatibleModel,
		agent.EnvOpenAICompatibleTimeout,
		agent.EnvOpenAICompatibleAPIKey,
	} {
		t.Setenv(k, "")
	}
}

// TestOpenAICompatibleIdentitySynchronization proves one canonical identity
// ("openai_compatible") flows through the provider, agent, model-routing, and
// config layers without any layer inventing its own string: ParseID accepts it,
// the agent constant equals it, the model router's allow-list contains it, and
// the config layer validates it as a selectable agent.provider.
func TestOpenAICompatibleIdentitySynchronization(t *testing.T) {
	id := string(provider.OpenaiCompatible)

	got, err := provider.ParseID(id)
	if err != nil {
		t.Fatalf("ParseID(%q): %v", id, err)
	}
	if got != provider.OpenaiCompatible {
		t.Fatalf("ParseID(%q) = %q, want %q", id, got, provider.OpenaiCompatible)
	}
	if agent.ProviderOpenAICompatible != id {
		t.Fatalf("agent provider %q != canonical %q", agent.ProviderOpenAICompatible, id)
	}
	if !containsString(model.KnownProviders(), id) {
		t.Fatalf("model router providers %v must contain %q", model.KnownProviders(), id)
	}

	// The configuration layer's endpoint vocabulary is the same one the agent
	// layer reads: one name per setting.
	if agent.EnvOpenAICompatibleBaseURL != config.EnvOpenAICompatibleBaseURL {
		t.Fatalf("agent env %q != config env %q", agent.EnvOpenAICompatibleBaseURL, config.EnvOpenAICompatibleBaseURL)
	}

	// Unknown identities still fail rather than resolving to a known provider.
	if _, err := provider.ParseID("not_a_provider"); err == nil {
		t.Fatal("ParseID(unknown) must fail")
	}
}

// TestOpenAICompatibleConfigSelectable proves the configuration layer accepts
// the canonical identity as agent.provider (recognized and configurable), with
// no raw-string comparison in the test: the value comes from the provider
// vocabulary itself.
func TestOpenAICompatibleConfigSelectable(t *testing.T) {
	cfg, err := config.Parse([]byte("" +
		"version: 1\n" +
		"project:\n  name: demo\n" +
		"agent:\n  harness: tool\n  provider: " + string(provider.OpenaiCompatible) + "\n  model: m\n"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.Agent.Provider != agent.ProviderOpenAICompatible {
		t.Fatalf("provider = %q, want %q", cfg.Agent.Provider, agent.ProviderOpenAICompatible)
	}
}

// TestOpenAICompatibleIsExecutable locks the Phase 2 contract: the generic
// provider is no longer rejected as "not configured"; selecting it builds the
// shared OpenAI-compatible execution transport, and it never silently
// substitutes another provider.
func TestOpenAICompatibleIsExecutable(t *testing.T) {
	clearOpenAICompatibleEnv(t)
	t.Setenv(agent.EnvOpenAICompatibleModel, "mlx-community/Qwen3-4B-4bit")

	a, err := agent.FromConfig(agent.ProviderOpenAICompatible, "")
	if err != nil {
		t.Fatalf("FromConfig must build the generic provider, got error %v", err)
	}
	if _, ok := a.(*agent.OpenAICompatible); !ok {
		t.Fatalf("got %T, want *agent.OpenAICompatible (the shared transport)", a)
	}

	h, err := agent.HarnessFromConfig(agent.HarnessTool, agent.ProviderOpenAICompatible, "some-model")
	if err != nil {
		t.Fatalf("HarnessFromConfig must wrap the generic provider, got error %v", err)
	}
	if h == nil {
		t.Fatal("HarnessFromConfig must build a harness, got nil")
	}

	t.Run("no silent substitution to another provider", func(t *testing.T) {
		// Even with SOP_AGENT_MODEL set (a value every other provider would use),
		// selecting the generic provider must build it, not a different one.
		t.Setenv(agent.EnvAgentModel, "m")
		a, err := agent.FromConfig(agent.ProviderOpenAICompatible, "m")
		if err != nil {
			t.Fatalf("FromConfig: %v", err)
		}
		if _, ok := a.(*agent.OpenAICompatible); !ok {
			t.Fatalf("got %T, want the generic provider, not a substitute", a)
		}
	})

	t.Run("a missing model fails clearly", func(t *testing.T) {
		t.Setenv(agent.EnvAgentModel, "")
		t.Setenv(agent.EnvOpenAICompatibleModel, "")
		if _, err := agent.FromConfig(agent.ProviderOpenAICompatible, ""); err == nil {
			t.Fatal("a missing model must fail clearly")
		}
	})
}

// TestOpenAICompatibleEndpointResolution proves the endpoint the execution path
// uses follows the documented precedence for the environment and default tiers,
// and that the default matches the config layer's resolver so the two cannot
// drift.
func TestOpenAICompatibleEndpointResolution(t *testing.T) {
	t.Run("environment wins", func(t *testing.T) {
		t.Setenv(agent.EnvOpenAICompatibleBaseURL, "http://127.0.0.1:9999")
		if got, want := agent.OpenAICompatibleEndpointFromEnv(), "http://127.0.0.1:9999"; got != want {
			t.Fatalf("endpoint = %q, want %q", got, want)
		}
	})

	t.Run("default when unset", func(t *testing.T) {
		t.Setenv(agent.EnvOpenAICompatibleBaseURL, "")
		if got, want := agent.OpenAICompatibleEndpointFromEnv(), config.DefaultOpenAICompatibleBaseURL; got != want {
			t.Fatalf("endpoint = %q, want default %q", got, want)
		}
	})

	t.Run("blank environment falls through to the default", func(t *testing.T) {
		t.Setenv(agent.EnvOpenAICompatibleBaseURL, "   ")
		if got, want := agent.OpenAICompatibleEndpointFromEnv(), config.DefaultOpenAICompatibleBaseURL; got != want {
			t.Fatalf("endpoint = %q, want default %q", got, want)
		}
	})

	t.Run("default endpoint matches the config resolver", func(t *testing.T) {
		clearOpenAICompatibleEnv(t)
		if got, want := agent.OpenAICompatibleEndpointFromEnv(), (config.Config{}).ResolveOpenAICompatibleEndpoint(); got != want {
			t.Fatalf("agent default %q != config default %q", got, want)
		}
	})
}

// TestOpenAICompatibleSMALLRouting proves a routed SMALL class may name the
// generic provider and keeps its class, model, and locality unchanged — routing
// resolves the identity, it never rewrites it.
func TestOpenAICompatibleSMALLRouting(t *testing.T) {
	clearOpenAICompatibleEnv(t)
	env := map[string]string{
		model.EnvDefaultClass:                           "small",
		model.ClassEnvKey(model.ClassSmall, "PROVIDER"): "openai_compatible",
		model.ClassEnvKey(model.ClassSmall, "NAME"):     "mlx-community/Qwen3-4B-4bit",
		model.ClassEnvKey(model.ClassSmall, "LOCALITY"): "local",
	}
	res, err := model.Resolve(model.Inputs{Lookup: func(k string) string { return env[k] }})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	sel := res.Selection
	if sel.Provider != agent.ProviderOpenAICompatible {
		t.Fatalf("provider = %q, want %q", sel.Provider, agent.ProviderOpenAICompatible)
	}
	if sel.Model != "mlx-community/Qwen3-4B-4bit" {
		t.Fatalf("model = %q, want it unchanged", sel.Model)
	}
	if sel.Locality != model.LocalityLocal {
		t.Fatalf("locality = %q, want %q (never inferred)", sel.Locality, model.LocalityLocal)
	}

	// The routed selection reaches the execution provider unchanged.
	a, err := agent.FromConfig(sel.Provider, sel.Model)
	if err != nil {
		t.Fatalf("FromConfig for the routed selection: %v", err)
	}
	if _, ok := a.(*agent.OpenAICompatible); !ok {
		t.Fatalf("routed selection did not reach the generic provider: got %T", a)
	}
}

// TestOpenAICompatibleCloudLocalityIsLegal proves the generic provider does not
// decide locality: a cloud endpoint is as legal as a local one.
func TestOpenAICompatibleCloudLocalityIsLegal(t *testing.T) {
	clearOpenAICompatibleEnv(t)
	env := map[string]string{
		model.EnvDefaultClass:                           "small",
		model.ClassEnvKey(model.ClassSmall, "PROVIDER"): "openai_compatible",
		model.ClassEnvKey(model.ClassSmall, "NAME"):     "gpt-oss-120b",
		model.ClassEnvKey(model.ClassSmall, "LOCALITY"): "cloud",
	}
	res, err := model.Resolve(model.Inputs{Lookup: func(k string) string { return env[k] }})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Selection.Locality != model.LocalityCloud {
		t.Fatalf("locality = %q, want %q", res.Selection.Locality, model.LocalityCloud)
	}
}

func containsString(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
