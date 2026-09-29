package config

import (
	"fmt"
	"strings"

	"github.com/imhttran/agentic-sop/internal/autonomy"
)

// Autonomy is the risk-based autonomy policy: how much recovery SOP performs
// without asking a human. It resolves to the deterministic autonomy.Policy the
// lifecycle consults.
//
// The block is optional and additive. An omitted block resolves to the
// conservative default (Balanced), so existing configurations keep working and no
// project is silently switched to maximum autonomy. The boolean fields are
// pointers so an explicit false is distinguishable from an omitted value; an
// omitted value keeps the level's built-in setting.
type Autonomy struct {
	// Level selects the coarse autonomy level: low, balanced, or high. It defaults
	// to balanced.
	Level string `yaml:"level"`
	// AutoRetry permits bounded retries of transient failures.
	AutoRetry *bool `yaml:"auto_retry"`
	// AutoContinue permits bounded continuation of productive incomplete work.
	AutoContinue *bool `yaml:"auto_continue"`
	// AutoFix permits the bounded, deterministic fix loop.
	AutoFix *bool `yaml:"auto_fix"`
	// AutoReconcileSafeChanges permits automatic reconciliation of semantically
	// safe plan changes.
	AutoReconcileSafeChanges *bool `yaml:"auto_reconcile_safe_changes"`
	// RequireHuman lists the risk categories that always wait for a human. When
	// empty it defaults to the standard authority boundaries.
	RequireHuman []string `yaml:"require_human"`
}

// AutonomyPolicy resolves the configured autonomy block into the deterministic
// policy the lifecycle consults. An omitted block resolves to the conservative
// default; explicit flags override the level's built-in settings.
func (c Config) AutonomyPolicy() autonomy.Policy {
	p := autonomy.PolicyFor(autonomy.Level(normalizeLevel(c.Autonomy.Level)))
	if c.Autonomy.AutoRetry != nil {
		p.AutoRetry = *c.Autonomy.AutoRetry
	}
	if c.Autonomy.AutoContinue != nil {
		p.AutoContinue = *c.Autonomy.AutoContinue
	}
	if c.Autonomy.AutoFix != nil {
		p.AutoFix = *c.Autonomy.AutoFix
	}
	if c.Autonomy.AutoReconcileSafeChanges != nil {
		p.AutoReconcileSafeChanges = *c.Autonomy.AutoReconcileSafeChanges
	}
	if len(c.Autonomy.RequireHuman) > 0 {
		p.RequireHuman = normalizeCategories(c.Autonomy.RequireHuman)
	}
	return p
}

// AutonomyLevel returns the resolved autonomy level string.
func (c Config) AutonomyLevel() autonomy.Level {
	return autonomy.Level(normalizeLevel(c.Autonomy.Level))
}

// validateAutonomy rejects an unknown level or risk category at load time, so a
// bad policy is never deferred to first use.
func (c *Config) validateAutonomy() error {
	if level := strings.TrimSpace(c.Autonomy.Level); level != "" && !autonomy.Level(strings.ToLower(level)).Valid() {
		return fmt.Errorf("config: unknown autonomy.level %q (want low, balanced, high)", c.Autonomy.Level)
	}
	for _, cat := range c.Autonomy.RequireHuman {
		if !validCategory(cat) {
			return fmt.Errorf("config: unknown autonomy.require_human category %q (want %s)",
				cat, strings.Join(categoryNames(), ", "))
		}
	}
	return nil
}

// normalizeLevel lowercases and trims an autonomy level for resolution.
func normalizeLevel(level string) string {
	return strings.ToLower(strings.TrimSpace(level))
}

// normalizeCategories maps configured category names to the risk-category values,
// trimming and lowercasing, and de-duplicating while preserving order.
func normalizeCategories(in []string) []autonomy.RiskCategory {
	seen := make(map[autonomy.RiskCategory]bool, len(in))
	out := make([]autonomy.RiskCategory, 0, len(in))
	for _, c := range in {
		cat := autonomy.RiskCategory(strings.ToLower(strings.TrimSpace(c)))
		if cat == "" || seen[cat] {
			continue
		}
		seen[cat] = true
		out = append(out, cat)
	}
	return out
}

// validCategory reports whether name is a known risk category.
func validCategory(name string) bool {
	cat := autonomy.RiskCategory(strings.ToLower(strings.TrimSpace(name)))
	for _, known := range autonomy.Categories() {
		if cat == known {
			return true
		}
	}
	return false
}

// categoryNames returns the known risk categories as strings, for error messages.
func categoryNames() []string {
	cats := autonomy.Categories()
	out := make([]string, 0, len(cats))
	for _, c := range cats {
		out = append(out, string(c))
	}
	return out
}
