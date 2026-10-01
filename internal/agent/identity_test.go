package agent_test

import (
	"slices"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/provider"
	"github.com/imhttran/agentic-sop/internal/provider/llamacpp"
	"github.com/imhttran/agentic-sop/internal/provider/mlx"
	"github.com/imhttran/agentic-sop/internal/provider/ollama"
)

// TestProviderIdentityIsCanonical proves the agent layer's provider name
// constants are derived from — and stay equal to — the canonical provider
// identifiers in internal/provider, so the execution, configuration, and runtime
// layers cannot drift into separate vocabularies.
func TestProviderIdentityIsCanonical(t *testing.T) {
	cases := []struct {
		name string
		want provider.ID
	}{
		{agent.ProviderCommand, provider.Command},
		{agent.ProviderOllama, provider.Ollama},
		{agent.ProviderLlamaCpp, provider.LlamaCPP},
		{agent.ProviderMLX, provider.MLX},
	}
	for _, tc := range cases {
		if tc.name != string(tc.want) {
			t.Errorf("agent provider name %q, want %q", tc.name, tc.want)
		}
		if _, err := provider.ParseID(tc.name); err != nil {
			t.Errorf("agent provider name %q must be a canonical provider id: %v", tc.name, err)
		}
	}

	// Every canonical id must have an agent-level name; otherwise the agent layer
	// would reject a provider the rest of SOP accepts.
	names := []string{agent.ProviderCommand, agent.ProviderOllama, agent.ProviderLlamaCpp, agent.ProviderMLX}
	for _, id := range provider.KnownIDs() {
		if !slices.Contains(names, id.String()) {
			t.Errorf("canonical provider %q has no agent provider constant", id)
		}
	}
}

// TestProviderEnvNamesDoNotDrift proves the base-URL environment variable the
// agent path reads is the same setting the provider/runtime layer reads. One name
// per setting; a rename on either side fails here.
func TestProviderEnvNamesDoNotDrift(t *testing.T) {
	cases := []struct {
		what     string
		agentEnv string
		provEnv  string
	}{
		{"ollama base URL", agent.EnvOllamaBaseURL, ollama.EnvBaseURL},
		{"llamacpp base URL", agent.EnvLlamaCppBaseURL, llamacpp.EnvBaseURL},
		{"mlx base URL", agent.EnvMLXBaseURL, mlx.EnvBaseURL},
	}
	for _, tc := range cases {
		if tc.agentEnv != tc.provEnv {
			t.Errorf("%s: agent env %q != provider env %q", tc.what, tc.agentEnv, tc.provEnv)
		}
	}
}
