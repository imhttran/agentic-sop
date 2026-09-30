package cli

import (
	"os"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/dotenv"
	"github.com/imhttran/agentic-sop/internal/model"
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
