package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/model"
	"github.com/imhttran/agentic-sop/internal/provider"
	"github.com/imhttran/agentic-sop/internal/provider/command"
	"github.com/imhttran/agentic-sop/internal/provider/llamacpp"
	"github.com/imhttran/agentic-sop/internal/provider/mlx"
	"github.com/imhttran/agentic-sop/internal/provider/ollama"
)

// providerProbeTimeout bounds a single provider health or discovery probe. It is
// short so `sop providers` and opt-in validation never hang on a dead endpoint.
const providerProbeTimeout = 5 * time.Second

// newProviderRegistry builds the provider registry from configuration and the
// environment. It is the composition root for the provider layer, mirroring how
// the agent layer is assembled: endpoints come from config, the environment
// overrides them, and built-in defaults fill the rest. It is read-only with
// respect to SOP state - building a registry starts no execution and selects no
// model class.
func newProviderRegistry(cfg config.Config) (*provider.Registry, error) {
	reg := provider.NewRegistry()
	entries := []provider.Provider{
		ollama.New(endpointOr(cfg.Providers.Ollama.Endpoint, ollama.EnvBaseURL, ollama.DefaultBaseURL), providerProbeTimeout),
		llamacpp.New(
			endpointOr(cfg.Providers.LlamaCpp.Endpoint, llamacpp.EnvBaseURL, llamacpp.DefaultBaseURL),
			llamacppConfiguredModel(cfg),
			providerProbeTimeout,
		),
		mlx.New(endpointOr(cfg.Providers.MLX.Endpoint, mlx.EnvBaseURL, mlx.DefaultBaseURL), providerProbeTimeout),
		command.New(),
	}
	for _, p := range entries {
		if err := reg.Register(p); err != nil {
			return nil, err
		}
	}
	return reg, nil
}

// endpointOr resolves a provider endpoint with the documented precedence:
// environment (which a .env file may supply) over configuration over the
// built-in default.
func endpointOr(configured, envKey, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(envKey)); v != "" {
		return v
	}
	if v := strings.TrimSpace(configured); v != "" {
		return v
	}
	return fallback
}

// llamacppConfiguredModel resolves the model configured for the single-model
// llama.cpp endpoint: the agent path's SOP_LLAMACPP_MODEL wins, then agent.model
// when llama.cpp is the configured provider. It is used only as the provider's
// identity when llama-server cannot enumerate models, never as a discovery
// result, and never to select a class.
func llamacppConfiguredModel(cfg config.Config) string {
	if v := strings.TrimSpace(os.Getenv(agent.EnvLlamaCppModel)); v != "" {
		return v
	}
	if cfg.Agent.Provider == string(provider.LlamaCPP) {
		return strings.TrimSpace(cfg.Agent.Model)
	}
	return ""
}

// runProviders implements `sop providers [--models]`: a read-only inspection of
// the configured provider runtimes. It never mutates SOP state, never executes a
// task, and never prints credentials or endpoints.
func runProviders(args []string, stdout, stderr io.Writer, d deps) int {
	showModels := false
	for _, a := range args {
		switch a {
		case "--models":
			showModels = true
		default:
			fmt.Fprintf(stderr, "unknown flag %s\nusage: sop providers [--models]\n", a)
			return exitUsage
		}
	}

	dir, ok := projectDir(d.getwd, stderr)
	if !ok {
		return exitError
	}
	cfg, err := loadConfigOrDefault(dir)
	if err != nil {
		fmt.Fprintf(stderr, "providers: %v\n", err)
		return exitError
	}
	reg, err := newProviderRegistry(cfg)
	if err != nil {
		fmt.Fprintf(stderr, "providers: %v\n", err)
		return exitError
	}

	ctx := context.Background()
	providers := reg.List()

	fmt.Fprintf(stdout, "%-10s %-12s %s\n", "PROVIDER", "STATUS", "MODELS")
	for _, p := range providers {
		status := p.Health(ctx).Status
		models := "-"
		if infos, err := p.Models(ctx); err == nil {
			models = fmt.Sprintf("%d", len(infos))
		}
		fmt.Fprintf(stdout, "%-10s %-12s %s\n", p.ID(), status, models)
	}

	if showModels {
		for _, p := range providers {
			infos, err := p.Models(ctx)
			if err != nil || len(infos) == 0 {
				continue
			}
			fmt.Fprintf(stdout, "\n%s\n", p.ID())
			for _, m := range infos {
				fmt.Fprintf(stdout, "  %s\n", m.Name)
				if m.Locality != "" {
					fmt.Fprintf(stdout, "    locality: %s\n", m.Locality)
				}
				if meta := m.Metadata(); meta != "" {
					fmt.Fprintf(stdout, "    %s\n", meta)
				}
				if caps, err := p.Capabilities(ctx, m.Name); err == nil {
					fmt.Fprintf(stdout, "    capabilities: %s\n", caps)
				}
			}
		}
	}
	return exitOK
}

// validateSelectedModel checks the resolved model selection before a run starts
// a task, when the operator opted in (providers.validate). It is a strict no-op
// when the layer is off - which is the default - so an existing installation's
// behavior is unchanged.
//
// It validates the run-level/default selection. When the automatic per-task router
// is enabled, the run-level default is not what executes: each task's final
// selection is validated by validateSelection inside applyTaskRouting instead, and
// the caller skips this call (see the `!d.routingEnabled` guard at the call sites)
// so a default class that the router never selects cannot fail the run.
//
// It never substitutes a provider or model: on failure it returns the error and
// the caller stops. The command provider has no model to validate and is skipped.
func validateSelectedModel(ctx context.Context, cfg config.Config, routing model.Result) error {
	if !cfg.Providers.ValidateSelections() {
		return nil
	}
	return validateSelection(ctx, cfg, selectionForValidation(cfg, routing))
}

// validateSelection checks one resolved model selection against the configured
// provider runtimes, when validation is opted in. It is the single validation
// seam: both the run-level check and the final per-task check go through it.
//
// It is a strict no-op when providers.validate is off. It never substitutes a
// provider or model, and the command provider (which has no model to inspect) is
// skipped.
func validateSelection(ctx context.Context, cfg config.Config, sel model.Selection) error {
	if !cfg.Providers.ValidateSelections() {
		return nil
	}
	if sel.Provider == string(provider.Command) {
		return nil
	}
	reg, err := newProviderRegistry(cfg)
	if err != nil {
		return err
	}
	return provider.ValidateSelection(ctx, reg, sel)
}

// selectionForValidation returns the selection to validate: the routing layer's
// selection when routing is active, otherwise the effective execution stack's
// provider and model.
func selectionForValidation(cfg config.Config, routing model.Result) model.Selection {
	if routing.Active {
		return routing.Selection
	}
	stack := resolveExecutionStack(cfg)
	return model.Selection{Provider: stack.Provider, Model: stack.Model}
}

// localRuntimeProbe observes whether a local class's model can be served by its
// configured runtime. It is the production availability probe behind the
// local-first cloud fallback.
//
// It is deliberately MORE precise than a bare "can this run" check: it
// distinguishes an UNREACHABLE runtime (Ollama at http://127.0.0.1:11434 is
// down) from a HEALTHY runtime that cannot serve the configured local model. A
// cloud-hosted Ollama model still uses the same Ollama execution path, so an
// unreachable runtime MUST NOT trigger the availability fallback - that case
// follows the existing provider/runtime-unavailable error path. It reports
// fallback=true ONLY for the healthy-but-model-unavailable case, so
// applyLocalFallback can never silently swap in a cloud model to "fix" a down
// daemon.
//
// It probes read-only (reachability, model list, and chat capability) and never
// substitutes a provider or model. A command-harness selection has no model
// runtime to probe. A probe that cannot be built, or that observes nothing
// definite, reports no fallback so the local model is kept.
func localRuntimeProbe(cfg config.Config, sel model.Selection) (bool, string) {
	if sel.Provider == string(provider.Command) {
		return false, ""
	}
	reg, err := newProviderRegistry(cfg)
	if err != nil {
		return false, ""
	}
	ctx, cancel := context.WithTimeout(context.Background(), providerProbeTimeout)
	defer cancel()
	av := provider.ResolveAvailability(ctx, reg, sel)
	if av.Source == provider.AvailabilityFallback {
		return true, av.Reason
	}
	return false, av.Reason
}

// localRuntimeUnreachable reports whether the local class's runtime is itself
// unreachable (as opposed to healthy-but-model-absent). It is used so the CLI can
// surface the existing provider/runtime-unavailable error rather than pretend a
// cloud model fixes a down endpoint.
func localRuntimeUnreachable(cfg config.Config, sel model.Selection) bool {
	if sel.Provider == string(provider.Command) {
		return false
	}
	reg, err := newProviderRegistry(cfg)
	if err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), providerProbeTimeout)
	defer cancel()
	return provider.ResolveAvailability(ctx, reg, sel).RuntimeUnreachable
}

// localUnavailableProbe returns the closure that reports whether a local
// selection's runtime is HEALTHY but cannot serve it, or nil when no probe is
// wired. A nil closure means "no observation", so the local model is kept - an
// absent probe is never read as an outage. As above, an unreachable runtime is
// NOT reported here: it is not a fallback trigger.
func localUnavailableProbe(cfg config.Config, d deps) func(model.Selection) bool {
	if d.localProbe == nil {
		return nil
	}
	return func(sel model.Selection) bool {
		unavailable, _ := d.localProbe(cfg, sel)
		return unavailable
	}
}

// applyLocalFallback returns res with its selection replaced by the class's
// configured cloud fallback when the local runtime is HEALTHY but cannot serve
// the primary model, along with whether it applied. It is the single place the
// local-first runtime fallback is applied, so the automatic router and a manual
// --model-class override behave identically.
//
// It is a no-op - returning (res, false) - when the class is not local, no
// fallback is configured, no probe is wired, the probe reports the local model is
// usable, or the probe reports the local runtime is UNREACHABLE. An unreachable
// runtime is NOT a fallback trigger: a cloud-hosted Ollama model uses the same
// execution path, so the run keeps the local selection and surfaces the existing
// provider/runtime-unavailable error rather than pretending a cloud model fixes a
// down daemon. It never runs the fallback on a generation, validation, review, or
// gate failure: those keep the existing recovery behavior, because the probe
// observes runtime AVAILABILITY only.
//
// The returned bool is the EXPLICIT decision the caller records as the execution
// source (primary vs availability-fallback); it is not re-derived from an
// unrelated Selection field.
func applyLocalFallback(cfg config.Config, d deps, res model.Result) (model.Result, bool) {
	if !res.Active || res.LocalFallback == nil {
		return res, false
	}
	probe := localUnavailableProbe(cfg, d)
	if probe == nil || !probe(res.Selection) {
		return res, false
	}
	res.Selection = *res.LocalFallback
	return res, true
}
