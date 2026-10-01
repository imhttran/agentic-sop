package agent_test

import (
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/model"
	"github.com/imhttran/agentic-sop/internal/provider"
)

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

// TestOpenAICompatibleIsNotExecutable locks the Phase 1 contract: the provider
// is recognized and configurable, but attempting to build it returns the
// existing unsupported/not-configured error with no fallback and no silent
// substitution to another provider.
func TestOpenAICompatibleIsNotExecutable(t *testing.T) {
	for _, env := range []string{
		agent.EnvAgentProvider,
		agent.EnvAgentModel,
		agent.EnvOpenAICompatibleModel,
		agent.EnvOpenAICompatibleBaseURL,
		agent.EnvOpenAICompatibleAPIKey,
		agent.EnvOpenAICompatibleTimeout,
	} {
		t.Setenv(env, "")
	}

	t.Run("FromConfig returns unsupported error", func(t *testing.T) {
		a, err := agent.FromConfig(agent.ProviderOpenAICompatible, "mlx-community/Qwen3-4B-4bit")
		if err == nil {
			t.Fatalf("FromConfig must fail for %s, got agent %T", agent.ProviderOpenAICompatible, a)
		}
		if a != nil {
			t.Fatalf("no agent must be constructed, got %T", a)
		}
		if !strings.Contains(err.Error(), agent.ProviderOpenAICompatible) {
			t.Fatalf("error %q must name the provider", err)
		}
		if !strings.Contains(err.Error(), "no ") {
			t.Fatalf("error %q must describe the not-configured state", err)
		}
	})

	t.Run("HarnessFromConfig returns unsupported error", func(t *testing.T) {
		h, err := agent.HarnessFromConfig(agent.HarnessTool, agent.ProviderOpenAICompatible, "some-model")
		if err == nil {
			t.Fatalf("HarnessFromConfig must fail, got harness %T", h)
		}
		if h != nil {
			t.Fatalf("no harness must be constructed (no substitution), got %T", h)
		}
	})

	t.Run("no silent substitution to another provider", func(t *testing.T) {
		// Even with SOP_AGENT_MODEL set (a value every other provider would use),
		// selecting the generic provider must not silently build a different one.
		t.Setenv(agent.EnvAgentModel, "m")
		a, err := agent.FromConfig(agent.ProviderOpenAICompatible, "m")
		if err == nil {
			t.Fatalf("must not substitute a provider, got %T", a)
		}
	})
}

func containsString(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
