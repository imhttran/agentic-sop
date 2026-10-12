package cli

import (
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/router"
	"github.com/imhttran/agentic-sop/internal/taskfile"
	"github.com/imhttran/agentic-sop/internal/validate"
	"github.com/imhttran/agentic-sop/internal/verify"
)

// verificationTier derives the deterministic verification tier for a task from typed
// signals and its declared execution mode. It is model-free and escalate-only (see
// internal/verify). With risk-based verification disabled it returns MEDIUM (the safe
// default), which selects the unchanged behavior below.
func verificationTier(cfg config.Config, spec *taskfile.Spec) verify.Tier {
	if !cfg.Verification.Enabled() {
		return verify.Medium
	}
	risk := router.SignalsFrom(nil, router.TaskSignals{
		AcceptanceCriteria: len(spec.AcceptanceCriteria),
		Dependencies:       len(spec.Dependencies),
	})
	tier, _ := verify.Derive(risk, spec.ExecutionMode.VerifyFirst())
	return tier
}

// selectValidation returns the configured validation command set the gate requires
// for the task's tier (RBV-003). With risk-based verification disabled it returns the
// full configured set, so existing behavior is unchanged. When enabled: LOW runs the
// configured targeted set (falling back to the full set when none is configured, so a
// LOW tier can never reduce verification to nothing), MEDIUM runs the build+test set,
// and HIGH runs the full configured set. The deterministic gate verdict is unchanged:
// it still decides from the observed results, never from the tier.
func selectValidation(cfg config.Config, spec *taskfile.Spec) config.Validation {
	if !cfg.Verification.Enabled() {
		return cfg.Validation
	}
	switch verificationTier(cfg, spec) {
	case verify.Low:
		if low := cfg.Verification.LowValidation(); validate.Enabled(low) {
			return low
		}
		return cfg.Validation
	case verify.Medium:
		return config.Validation{Build: cfg.Validation.Build, Test: cfg.Validation.Test}
	default: // verify.High
		return cfg.Validation
	}
}
