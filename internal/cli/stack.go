package cli

import (
	"os"
	"strings"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/config"
)

// StackSource records where an effective execution-stack value came from. It is
// the same vocabulary the provider already uses (environment, configuration,
// default) so the startup summary names the origin of every resolved value.
type StackSource = agent.ProviderSource

// executionStack is the resolved runtime execution stack shown at startup: the
// harness, the provider and where its name came from, the model when the
// provider serves one, and the command when the command harness is selected.
// It is a single resolved source of truth for the startup summary, derived from
// the same configuration and environment resolution the run itself uses.
type executionStack struct {
	Harness string
	// HarnessSource records where the effective harness came from.
	HarnessSource StackSource
	Provider      string
	Source        StackSource
	// Model is set only when the provider serves a model (ollama, llamacpp);
	// it is empty for the command provider, which has no model.
	Model string
	// ModelSource records where the effective model came from. It is empty when
	// no model applies (the command provider).
	ModelSource StackSource
	// Command is set only for the command harness: the external command the
	// harness runs.
	Command string
}

// EnvAgentHarness lets the environment override the configured harness, matching
// the precedence the provider already uses. When unset the configured harness
// (or the default) is used.
const EnvAgentHarness = "SOP_AGENT_HARNESS"

// LegacyEnvAgentCommand is the historical command variable name, accepted so a
// project that only set the legacy name is reported as it is resolved.
const LegacyEnvAgentCommand = "AGENT_SDLC_AGENT_COMMAND"

// resolveExecutionStack derives the execution stack from the effective
// configuration and environment, reusing agent.EffectiveProvider so the summary
// reports exactly what the run will use. Harness and model sources are resolved
// with the documented precedence environment -> project configuration ->
// built-in defaults.
func resolveExecutionStack(cfg config.Config) executionStack {
	provider, source := agent.EffectiveProvider(cfg.Agent.Provider)
	harness, harnessSource := effectiveHarness(cfg.Agent.Harness)

	s := executionStack{
		Harness:       harness,
		HarnessSource: harnessSource,
		Provider:      provider,
		Source:        source,
	}
	s.Model, s.ModelSource = effectiveModel(provider, cfg.Agent.Model)
	if harness == config.LegacyHarness {
		s.Command = effectiveCommand()
	}
	return s
}

// effectiveHarness resolves the harness the run will use and where it came from:
// SOP_AGENT_HARNESS (environment) wins over the configured harness, which wins
// over the default tool harness.
func effectiveHarness(configured string) (string, StackSource) {
	if env := strings.TrimSpace(os.Getenv(EnvAgentHarness)); env != "" {
		return env, agent.SourceEnvironment
	}
	if cfg := strings.TrimSpace(configured); cfg != "" {
		return cfg, agent.SourceConfiguration
	}
	return config.DefaultHarness, agent.SourceDefault
}

// effectiveModel resolves the model the provider will serve and where it came
// from, mirroring the provider constructors: the provider's environment variable
// (SOP_OLLAMA_MODEL / SOP_LLAMACPP_MODEL) overrides the configured value. The
// command provider has no model, so it reports none and has no source.
func effectiveModel(provider, configured string) (string, StackSource) {
	switch provider {
	case agent.ProviderOllama:
		if env := strings.TrimSpace(os.Getenv(agent.EnvOllamaModel)); env != "" {
			return env, agent.SourceEnvironment
		}
		if model := strings.TrimSpace(configured); model != "" {
			return model, agent.SourceConfiguration
		}
		return "", ""
	case agent.ProviderLlamaCpp:
		if env := strings.TrimSpace(os.Getenv(agent.EnvLlamaCppModel)); env != "" {
			return env, agent.SourceEnvironment
		}
		if model := strings.TrimSpace(configured); model != "" {
			return model, agent.SourceConfiguration
		}
		return "", ""
	default:
		return "", ""
	}
}

// effectiveCommand resolves the external command the command harness runs,
// accepting the legacy variable name so existing configurations are reported
// as they are actually resolved.
func effectiveCommand() string {
	if command := strings.TrimSpace(os.Getenv(agent.EnvAgentCommand)); command != "" {
		return command
	}
	return strings.TrimSpace(os.Getenv(LegacyEnvAgentCommand))
}
