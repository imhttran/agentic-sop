package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/jev"
	"github.com/imhttran/agentic-sop/internal/model"
	"github.com/imhttran/agentic-sop/internal/ollamaagent"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
)

// applyStageModels resolves every pinned supporting stage to a concrete
// selection and records it on d: a stage pinned straight to a provider and model
// (SOP_MODEL_STAGE_<STAGE>_PROVIDER and _NAME) uses that, and a class pin
// (models.stages or SOP_MODEL_STAGE_<STAGE>_CLASS) resolves through the same class
// table the implementing class uses. An unpinned stage is
// left out, so its existing selection is unchanged. It runs once at startup, so a
// bad setting fails before any model call.
//
// Early JEV runs only on the Ollama path, so a JEV class on another provider is
// rejected rather than silently analyzed with a different model.
func applyStageModels(ctx context.Context, d *deps, cfg config.Config) error {
	d.stageModels = nil
	for _, st := range model.Stages {
		sel, direct, err := model.StageDirect(st, os.Getenv)
		if err != nil {
			return err
		}
		if !direct {
			class, err := model.StageClass(cfg.Models, st, os.Getenv)
			if err != nil {
				return err
			}
			if class == "" {
				continue
			}
			res, err := resolveModelLocality(ctx, model.Inputs{
				Config: cfg.Models, Lookup: os.Getenv, CLIClass: string(class),
			}, runtimeLocalityObserver(cfg))
			if err != nil {
				return fmt.Errorf("model routing: stages.%s: %w", st, err)
			}
			sel = res.Selection
		}
		if st == model.StageJEV && sel.Provider != agent.ProviderOllama {
			return fmt.Errorf("model routing: stages.jev: provider %q; JEV runs only on provider %q", sel.Provider, agent.ProviderOllama)
		}
		// The agent factory lets a model environment variable override the name it
		// is given. JEV sets its model explicitly; plan and review would silently run
		// the overriding model while the artifact records the pinned one.
		if key, v := modelEnvOverride(sel.Provider); st != model.StageJEV && v != "" && v != sel.Model {
			return fmt.Errorf("model routing: stages.%s is pinned to %s, but %s=%s overrides it; unset %s or pin the stage to %s", st, sel.Model, key, v, key, v)
		}
		if d.stageModels == nil {
			d.stageModels = map[model.Stage]model.Selection{}
		}
		d.stageModels[st] = sel
	}
	if sel, ok := d.stageModels[model.StageJEV]; ok && d.newJEVAnalyzerForModel != nil {
		build := d.newJEVAnalyzerForModel
		d.newJEVAnalyzer = func(config.Config) (jev.Analyzer, error) { return build(sel.Model) }
	}
	return nil
}

// stageAgent returns the agent a pinned stage runs on, or fallback when the
// stage is not pinned. The pinned agent is capability-guarded like any other.
func stageAgent(cfg config.Config, d deps, st model.Stage, capability agent.Capability, fallback agent.Agent) (agent.Agent, error) {
	sel, ok := d.stageModels[st]
	if !ok {
		return fallback, nil
	}
	a, err := d.newAgent(cfg.Agent.Harness, sel.Provider, sel.Model, string(sel.Class))
	if err != nil {
		return nil, fmt.Errorf("stages.%s: build agent for %s %s: %w", st, sel.Provider, sel.Model, err)
	}
	if err := guardCapability(a, capability); err != nil {
		return nil, fmt.Errorf("stages.%s: %w", st, err)
	}
	return a, nil
}

// stageModelDocs records which model each supporting stage ran on for this run:
// the pinned class when one is set, else the run-level agent (plan, review) or
// the Ollama environment/default model (JEV, only when a JEV gate is enabled).
// It is diagnostic evidence only and never feeds a decision.
func stageModelDocs(cfg config.Config, d deps) map[string]runpkg.StageModel {
	docs := map[string]runpkg.StageModel{}
	runLevel := runpkg.StageModel{Class: cfg.Agent.Class, Provider: cfg.Agent.Provider, Model: cfg.Agent.Model, Source: "run"}
	for _, st := range model.Stages {
		if sel, ok := d.stageModels[st]; ok {
			docs[string(st)] = runpkg.StageModel{Class: string(sel.Class), Provider: sel.Provider, Model: sel.Model, Source: "stage"}
			continue
		}
		switch st {
		case model.StagePlan:
			docs[string(st)] = runLevel
		case model.StageReview:
			if cfg.Review.Engine == "open-code-review" {
				docs[string(st)] = runpkg.StageModel{Provider: "open-code-review", Source: "review-engine"}
			} else {
				docs[string(st)] = runLevel
			}
		case model.StageJEV:
			if cfg.Quality.JEVEnabled() || cfg.EarlyJEVTaskTriageEnabled() || cfg.EarlyJEVPreExecutionEnabled() {
				if oc, err := ollamaagent.ConfigFromEnv(); err == nil {
					docs[string(st)] = runpkg.StageModel{Provider: agent.ProviderOllama, Model: oc.Model, Source: "ollama-env"}
				}
			}
		}
	}
	return docs
}

// modelEnvOverride returns the environment variable that overrides the model a
// provider's agent is built with, and its value, or "" when none is set. It
// mirrors the agent constructors' precedence: SOP_AGENT_MODEL, then the
// provider's own model variable.
func modelEnvOverride(provider string) (string, string) {
	keys := []string{agent.EnvAgentModel}
	switch provider {
	case agent.ProviderOllama:
		keys = append(keys, agent.EnvOllamaModel)
	case agent.ProviderLlamaCpp:
		keys = append(keys, agent.EnvLlamaCppModel)
	case agent.ProviderMLX:
		keys = append(keys, agent.EnvMLXModel)
	case agent.ProviderOpenAICompatible:
		keys = append(keys, agent.EnvOpenAICompatibleModel)
	}
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return k, v
		}
	}
	return "", ""
}
