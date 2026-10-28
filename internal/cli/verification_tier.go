package cli

import (
	"slices"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/quality"
	"github.com/imhttran/agentic-sop/internal/router"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
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

// shouldRunJEV reports whether the optional JEV analysis runs for this task (RBV-004).
// JEV must be enabled, and with risk-based verification enabled it runs only for the
// HIGH tier (LOW/MEDIUM keep it optional), reusing the tier's RequiresJEV rule. With
// the feature off, an enabled JEV runs exactly as before.
func shouldRunJEV(cfg config.Config, tier verify.Tier) bool {
	if !cfg.JEVActive() {
		return false
	}
	if !cfg.Verification.Enabled() {
		return true
	}
	return tier.RequiresJEV()
}

// scopeValidation narrows v.Test to the verification.scoped commands covering the
// task's changed paths. It returns v unchanged when no rule is configured, the
// change set is empty or may be truncated, the tier is not MEDIUM (LOW already
// runs its own targeted set, HIGH runs everything), or any changed path matches
// no rule, so an unmapped change always runs the full set. Build and lint are
// never narrowed. The plan's final gate re-runs the full set before completion.
func scopeValidation(cfg config.Config, v config.Validation, tier verify.Tier, changed []string) config.Validation {
	rules := cfg.Verification.Scoped
	if len(rules) == 0 || len(changed) == 0 || len(changed) >= runpkg.MaxTaskChangedFiles || tier != verify.Medium {
		return v
	}
	matched := make([]bool, len(rules))
	for _, p := range changed {
		covered := false
		for i, r := range rules {
			if slices.ContainsFunc(r.Paths, func(g string) bool { return quality.MatchPath(g, p) }) {
				matched[i], covered = true, true
			}
		}
		if !covered {
			return v
		}
	}
	var test []string
	for i, r := range rules {
		if !matched[i] {
			continue
		}
		for _, c := range r.Test {
			if !slices.Contains(test, c) {
				test = append(test, c)
			}
		}
	}
	v.Test = test
	return v
}
