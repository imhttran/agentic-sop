package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/model"
	"github.com/imhttran/agentic-sop/internal/provider/llamacpp"
	"github.com/imhttran/agentic-sop/internal/provider/mlx"
	"github.com/imhttran/agentic-sop/internal/provider/ollama"
)

// StackSource records where an effective execution-stack value came from. It is
// the same vocabulary the provider already uses (environment, configuration,
// default) so the startup summary names the origin of every resolved value.
type StackSource = agent.ProviderSource

// executionStack is the resolved runtime execution stack shown at startup: the
// harness, the provider and where its name came from, the model when the
// provider serves one, the endpoint the provider runtime is reached at, and the
// command when the command harness is selected. It is a single resolved source
// of truth for the startup summary, derived from the same configuration and
// environment resolution the run itself uses.
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
	// Endpoint is the effective endpoint the provider runtime is reached at:
	// environment over configuration over built-in default, the same resolution
	// newProviderRegistry applies. It is a RUNTIME-SELECTION input only: it
	// chooses which runtime a locality observation asks (see locality.go). It is
	// never locality evidence, never printed, and never persisted in routing
	// evidence. It is empty for the command provider, which has no endpoint.
	Endpoint string
	// Command is set only for the command harness: the external command the
	// harness runs.
	Command string
}

// LegacyEnvAgentCommand is the historical command variable name, accepted so a
// project that only set the legacy name is reported as it is resolved.
const LegacyEnvAgentCommand = "AGENT_SDLC_AGENT_COMMAND"

// resolveExecutionStack derives the execution stack from the effective
// configuration and environment, reusing agent.EffectiveProvider and
// agent.EffectiveHarness so the summary reports exactly what the run will use.
// Harness, model, and endpoint sources are resolved with the documented
// precedence: environment -> project configuration -> built-in defaults.
func resolveExecutionStack(cfg config.Config) executionStack {
	provider, source := agent.EffectiveProvider(cfg.Agent.Provider)
	harness, harnessSource := agent.EffectiveHarness(cfg.Agent.Harness)

	s := executionStack{
		Harness:       harness,
		HarnessSource: harnessSource,
		Provider:      provider,
		Source:        source,
	}
	s.Model, s.ModelSource = effectiveModel(provider, cfg.Agent.Model)
	s.Endpoint = effectiveEndpoint(provider, cfg)
	if harness == config.LegacyHarness {
		s.Command = effectiveCommand()
	}
	return s
}

// effectiveModel resolves the model the provider will serve and where it came
// from, with precedence: SOP_AGENT_MODEL (unified override) > provider-specific
// environment variable (SOP_OLLAMA_MODEL / SOP_LLAMACPP_MODEL / SOP_MLX_MODEL) >
// configured value. The command provider has no model, so it reports none and has
// no source.
func effectiveModel(provider, configured string) (string, StackSource) {
	// Unified model environment variable has highest precedence for any provider.
	if unifiedModel := strings.TrimSpace(os.Getenv(agent.EnvAgentModel)); unifiedModel != "" {
		return unifiedModel, agent.SourceEnvironment
	}

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
	case agent.ProviderMLX:
		if env := strings.TrimSpace(os.Getenv(agent.EnvMLXModel)); env != "" {
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

// effectiveEndpoint resolves the endpoint the effective provider's runtime is
// reached at, with the documented precedence: environment over configuration
// over built-in default. It applies the same resolution newProviderRegistry
// applies (endpointOr), so a locality observation composed from the run's
// configuration asks the runtime the run itself would use. The command provider
// has no endpoint. The endpoint is a runtime-selection input only: it chooses
// which runtime to ask, and it is never read as locality evidence, never
// printed, and never persisted in routing evidence.
func effectiveEndpoint(provider string, cfg config.Config) string {
	switch provider {
	case agent.ProviderOllama:
		return endpointOr(cfg.Providers.Ollama.Endpoint, ollama.EnvBaseURL, ollama.DefaultBaseURL)
	case agent.ProviderLlamaCpp:
		return endpointOr(cfg.Providers.LlamaCpp.Endpoint, llamacpp.EnvBaseURL, llamacpp.DefaultBaseURL)
	case agent.ProviderMLX:
		return endpointOr(cfg.Providers.MLX.Endpoint, mlx.EnvBaseURL, mlx.DefaultBaseURL)
	case agent.ProviderOpenAICompatible:
		return cfg.ResolveOpenAICompatibleEndpoint()
	default:
		return ""
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

// printExecutionStack formats and prints the execution stack to stdout: the
// harness, provider, model when applicable, and configuration sources. When the
// optional model-routing layer is active, it also prints the selected class,
// locality, the source that resolved the locality when known, and the layer so
// the model choice is auditable. Endpoints and credentials are never printed.
func printExecutionStack(w io.Writer, dir string, cfg config.Config, stack executionStack, routing model.Result) {
	fmt.Fprintf(w, "Project: %s\n", projectName(dir, cfg))
	fmt.Fprintf(w, "Harness: %s\n", stack.Harness)
	fmt.Fprintf(w, "Provider: %s\n", stack.Provider)
	if stack.Model != "" {
		fmt.Fprintf(w, "Model: %s\n", stack.Model)
	}
	if routing.Active {
		sel := routing.Selection
		fmt.Fprintf(w, "Model class: %s (%s, source: %s; %s)\n", sel.Class, sel.Locality, sel.Source, sel.Reason)
		if sel.LocalitySource != "" {
			fmt.Fprintf(w, "Model locality source: %s\n", sel.LocalitySource)
		}
	}
	fmt.Fprintf(w, "Provider source: %s\n", stack.Source)
	if stack.Command != "" {
		fmt.Fprintf(w, "Command: %s\n", stack.Command)
	}
}
