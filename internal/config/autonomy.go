package config

import (
	"fmt"
	"os"
	"strconv"
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
	// MaxContinuations bounds automatic continuation of productive incomplete work.
	// It is separate from the per-task retry budget (max_attempts).
	MaxContinuations *int `yaml:"max_continuations"`
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
	if c.Autonomy.MaxContinuations != nil && *c.Autonomy.MaxContinuations > 0 {
		p.MaxContinuations = *c.Autonomy.MaxContinuations
	}
	if n, ok, _ := envCount(EnvMaxContinuations, 1); ok {
		p.MaxContinuations = n
	}
	return p
}

// Environment overrides for the continuation budgets. They are validated before
// any task runs (ValidateAutonomyEnv), so a bad value fails early.
const (
	// EnvMaxContinuations overrides autonomy.max_continuations: the total
	// continuations a task may use across runs (at least 1).
	EnvMaxContinuations = "SOP_AUTONOMY_MAX_CONTINUATIONS"
	// EnvRunMaxContinues bounds how many of them a task may use within one sop run
	// before the run stops on it (0 stops at the first, as before).
	EnvRunMaxContinues = "SOP_RUN_MAX_CONTINUES"
)

// DefaultRunMaxContinues lets a task continue once within a run: a single
// continuation clears a one-off stop, while a second in a row usually means the
// model is stuck and the operator should look.
const DefaultRunMaxContinues = 1

// RunMaxContinues is how many continuations a task may use within one sop run.
func (c Config) RunMaxContinues() int {
	if n, ok, _ := envCount(EnvRunMaxContinues, 0); ok {
		return n
	}
	return DefaultRunMaxContinues
}

// envCount parses name as an integer of at least min. ok is false when it is
// unset; err reports a set but invalid value.
func envCount(name string, min int) (int, bool, error) {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return 0, false, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < min {
		return 0, false, fmt.Errorf("config: %s=%q: want a whole number of at least %d", name, v, min)
	}
	return n, true, nil
}

// AutonomyLevel returns the resolved autonomy level string.
func (c Config) AutonomyLevel() autonomy.Level {
	return autonomy.Level(normalizeLevel(c.Autonomy.Level))
}

// validateAutonomy rejects an unknown level at load time, so a bad policy is
// never deferred to first use.
func (c *Config) validateAutonomy() error {
	if level := strings.TrimSpace(c.Autonomy.Level); level != "" && !autonomy.Level(strings.ToLower(level)).Valid() {
		return fmt.Errorf("config: unknown autonomy.level %q (want low, balanced, high)", c.Autonomy.Level)
	}
	return ValidateAutonomyEnv()
}

// ValidateAutonomyEnv rejects a set but invalid continuation override. It runs at
// config load and again once the project .env is applied, which happens later.
func ValidateAutonomyEnv() error {
	if _, _, err := envCount(EnvMaxContinuations, 1); err != nil {
		return err
	}
	_, _, err := envCount(EnvRunMaxContinues, 0)
	return err
}

// normalizeLevel lowercases and trims an autonomy level for resolution.
func normalizeLevel(level string) string {
	return strings.ToLower(strings.TrimSpace(level))
}
