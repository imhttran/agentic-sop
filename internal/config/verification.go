package config

import (
	"fmt"
	"slices"
	"strings"
)

// Verification configures the optional risk-based verification tier.
//
// It is OFF by default: an omitted block leaves verification exactly as it is
// today, and the block alone (without risk_based: true) changes nothing. When
// enabled, SOP derives a deterministic verification tier (LOW/MEDIUM/HIGH) from
// typed evidence and selects the configured command set for the gate; the
// deterministic quality gate and every approval boundary are unchanged.
//
// The low sub-block is the targeted command set used only for the LOW tier. When
// it is empty, LOW falls back to the standard configured set, so enabling the
// feature can never reduce verification to nothing.
type Verification struct {
	// RiskBased enables tier-selected validation. Default false.
	RiskBased *bool `yaml:"risk_based"`
	// Low is the targeted command set used when the derived tier is LOW.
	Low VerificationLow `yaml:"low"`
	// Scoped maps changed paths to the test commands that cover them. A plan
	// drive runs only the covering tests per task and the full set once before
	// completion. Empty (the default) changes nothing.
	Scoped []ScopedRule `yaml:"scoped"`
}

// ScopedRule maps path globs (`**` spans segments) to the test commands that
// cover a change under them.
type ScopedRule struct {
	Paths []string `yaml:"paths"`
	Test  []string `yaml:"test"`
}

// VerificationLow is the LOW-tier targeted command set. It reuses the validation
// shape (build, test) so the existing validate/testrunner runner executes it
// unchanged.
type VerificationLow struct {
	Build []string `yaml:"build"`
	Test  []string `yaml:"test"`
}

// Enabled reports whether risk-based verification is turned on. An omitted flag and
// an explicit false both resolve to disabled, so the default behavior is unchanged.
func (v Verification) Enabled() bool {
	return v.RiskBased != nil && *v.RiskBased
}

// LowValidation returns the LOW-tier targeted command set as a Validation, so the
// existing validate/testrunner runner and the verification cache execute it without
// a second execution path.
func (v Verification) LowValidation() Validation {
	return Validation{Build: v.Low.Build, Test: v.Low.Test}
}

// validateScoped rejects a scoped rule that could never select or never run a
// test, so a typo cannot silently narrow verification.
func (v Verification) validateScoped() error {
	for i, r := range v.Scoped {
		if len(r.Paths) == 0 || len(r.Test) == 0 || slices.ContainsFunc(r.Paths, blank) || slices.ContainsFunc(r.Test, blank) {
			return fmt.Errorf("config: verification.scoped[%d] requires non-empty paths and test", i)
		}
	}
	return nil
}

func blank(s string) bool { return strings.TrimSpace(s) == "" }
