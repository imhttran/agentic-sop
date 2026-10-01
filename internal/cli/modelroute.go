package cli

import (
	"os"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/dotenv"
	"github.com/imhttran/agentic-sop/internal/model"
	"github.com/imhttran/agentic-sop/internal/recovery"
)

// loadDotEnv applies the optional project .env to the process environment.
//
// It is best-effort: a missing .env is not an error, and a value already present
// in the process environment is never overridden (an explicitly exported
// variable wins over the file). A malformed .env fails clearly rather than
// silently ignoring a line.
func loadDotEnv(dir string) error {
	vars, err := dotenv.LoadDir(dir)
	if err != nil {
		return err
	}
	dotenv.Apply(vars)
	return nil
}

// applyModelRouting resolves the optional model-routing layer and, when it is
// active, updates the configuration's agent provider and model from the selected
// class. It is a no-op when no models: block, SOP_MODEL_* environment, or CLI
// override is present, so an existing configuration keeps its agent unchanged.
//
// The returned result is for observability (the startup summary); policy never
// reads it back.
func applyModelRouting(cfg *config.Config, cliClass string) (model.Result, error) {
	res, err := model.Resolve(model.Inputs{
		Config:   cfg.Models,
		Lookup:   os.Getenv,
		CLIClass: cliClass,
	})
	if err != nil {
		return model.Result{}, err
	}
	if res.Active {
		cfg.Agent.Provider = res.Selection.Provider
		cfg.Agent.Model = res.Selection.Model
	}
	return res, nil
}

// applyRoutingEnabled resolves the automatic model-class router feature flag and
// records it on d. It is off unless SOP_MODEL_ROUTING_ENABLED (or the config's
// models.routing_enabled) turns it on, so an existing installation's behavior is
// unchanged.
func applyRoutingEnabled(d *deps, cfg config.Config) error {
	on, err := model.RoutingEnabled(cfg.Models, os.Getenv)
	if err != nil {
		return err
	}
	d.routingEnabled = on
	return nil
}

// applyEscalationEnabled resolves the bounded execution-recovery escalation policy
// (Phase 5) and records it on d. It is OFF unless SOP_MODEL_ESCALATION_ENABLED (or
// the config's models.escalation_enabled) turns it on, so an existing
// installation's execution behavior is unchanged. An unparseable flag or bound is
// an actionable error.
func applyEscalationEnabled(d *deps, cfg config.Config) error {
	on, err := model.EscalationEnabled(cfg.Models, os.Getenv)
	if err != nil {
		return err
	}
	max, err := model.MaxEscalations(cfg.Models, os.Getenv)
	if err != nil {
		return err
	}
	d.escalation = recovery.Policy{Enabled: on, MaxEscalations: max}
	return nil
}
