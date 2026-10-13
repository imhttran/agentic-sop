// Package budget is the canonical, explicit model of the deterministic execution
// limits an agent harness invocation runs under.
//
// Core principle: models may consume budgets; the deterministic harness owns,
// enforces, records, and evaluates them. A model never sets, extends, or reads a
// budget to decide its own behavior, and a non-positive operator value never
// disables a safety limit — it keeps the built-in default.
//
// A limit belongs here only if it is deterministically measurable, has exactly
// one enforcement owner, and has defined exhaustion semantics. Two candidate
// limits are deliberately excluded:
//
//   - the productive-discovery allowance is a soft steering threshold with no
//     exhaustion of its own — the stale guard is the hard bound — so it is a
//     bounded, operator-configured allowance (DiscoveryIterations) rather than a
//     hard ceiling;
//   - duration has no enforced owner today, so a duration budget is not invented.
//
// The budget is metadata about how much execution is allowed. It is deliberately
// separate from progress signals ("did useful evidence appear?"), termination
// ("why did execution stop?"), and evaluation ("did the behavior satisfy the
// contract?").
package budget

import (
	"fmt"
	"strconv"
	"strings"
)

// Budget is the canonical set of limits. Every field has a deterministic default;
// a zero or absent operator value means "use the default", never "unlimited".
type Budget struct {
	// ImplementIterations is the IMPLEMENT hard iteration ceiling (model turns).
	ImplementIterations int `json:"implement_iterations"`
	// FixIterations is the FIX hard iteration ceiling (model turns).
	FixIterations int `json:"fix_iterations"`
	// StaleIterations is the consecutive-stale-turn bound before the first
	// mutation: the no-progress guard.
	StaleIterations int `json:"stale_iterations"`
	// ToolCalls bounds executed tool calls across one invocation.
	ToolCalls int `json:"tool_calls"`
	// DiscoveryIterations is the bounded, task-scoped discovery allowance: how many
	// model turns an unmutated IMPLEMENT/FIX run may spend on novel successful
	// repository discovery before the pre-mutation convergence enforcement opens and
	// every non-mutating turn counts as stale. It is deliberately an operator-set
	// allowance, never a per-model or per-provider hardcoding. A zero or absent value
	// keeps the built-in default (the historical fixed window), so an unconfigured
	// project is byte-for-byte unchanged. It is resolved and clamped to
	// [MinDiscoveryIterations, MaxDiscoveryIterations], so an operator can widen
	// legitimate exploration without unbounding it.
	DiscoveryIterations int `json:"discovery_iterations"`
}

// Built-in defaults. These are the values the harness shipped with, so an
// unconfigured project is byte-for-byte unchanged.
const (
	DefaultImplementIterations = 32
	DefaultFixIterations       = 24
	DefaultStaleIterations     = 5
	DefaultToolCalls           = 80
	// DefaultDiscoveryIterations is the historical fixed productive-discovery
	// window (implementNowAfter in policy). An unconfigured project keeps it, so the
	// allowance is byte-for-byte unchanged.
	DefaultDiscoveryIterations = 12

	// MinDiscoveryIterations and MaxDiscoveryIterations bound the operator-configured
	// discovery allowance. The minimum keeps the window positive so the convergence
	// enforcement and stale guard always have a defined opening; the maximum bounds
	// it so an excessive value cannot push the allowance past a useful range. The
	// effective window is additionally capped below the capability's hard iteration
	// ceiling where the allowance is applied, so a maximum never disables the ceiling.
	MinDiscoveryIterations = 1
	MaxDiscoveryIterations = 64
)

// Environment variable names, following the existing SOP_OLLAMA_* override
// convention. An unset, blank, non-numeric, or non-positive value keeps the
// default, so a malformed value can never disable a limit.
const (
	EnvImplementIterations = "SOP_OLLAMA_IMPLEMENT_ITERATIONS"
	EnvFixIterations       = "SOP_OLLAMA_FIX_ITERATIONS"
	EnvStaleIterations     = "SOP_OLLAMA_STALE_ITERATIONS"
	EnvToolCalls           = "SOP_OLLAMA_TOOL_CALLS"
	// EnvDiscoveryIterations configures the task-scoped discovery allowance. An
	// unset, blank, non-numeric, or non-positive value keeps the base value, and a
	// value outside [MinDiscoveryIterations, MaxDiscoveryIterations] is clamped, so a
	// malformed or excessive override can never disable or unbounded the allowance.
	EnvDiscoveryIterations = "SOP_OLLAMA_DISCOVERY_ITERATIONS"
)

// Defaults returns the built-in budget.
func Defaults() Budget {
	return Budget{
		ImplementIterations: DefaultImplementIterations,
		FixIterations:       DefaultFixIterations,
		StaleIterations:     DefaultStaleIterations,
		ToolCalls:           DefaultToolCalls,
		DiscoveryIterations: DefaultDiscoveryIterations,
	}
}

// Resolve returns base with the operator environment overrides applied. base lets
// a caller start from a non-default value (for example a Config that already
// carries a tool-call limit), so integration does not silently reset it. A value
// that is absent, blank, non-numeric, or non-positive keeps base.
func Resolve(getenv func(string) string, base Budget) Budget {
	if getenv == nil {
		return base
	}
	b := base
	b.ImplementIterations = positiveInt(getenv(EnvImplementIterations), b.ImplementIterations)
	b.FixIterations = positiveInt(getenv(EnvFixIterations), b.FixIterations)
	b.StaleIterations = positiveInt(getenv(EnvStaleIterations), b.StaleIterations)
	b.ToolCalls = positiveInt(getenv(EnvToolCalls), b.ToolCalls)
	b.DiscoveryIterations = boundedPositiveInt(getenv(EnvDiscoveryIterations), b.DiscoveryIterations, MinDiscoveryIterations, MaxDiscoveryIterations)
	return b
}

// Validate rejects a negative limit. A negative value can only come from an
// explicit caller (never from Resolve, which keeps the default), so an explicit
// invalid budget fails loudly rather than silently becoming unlimited.
func (b Budget) Validate() error {
	fields := []struct {
		name  string
		value int
	}{
		{"implement_iterations", b.ImplementIterations},
		{"fix_iterations", b.FixIterations},
		{"stale_iterations", b.StaleIterations},
		{"tool_calls", b.ToolCalls},
		{"discovery_iterations", b.DiscoveryIterations},
	}
	for _, f := range fields {
		if f.value < 0 {
			return fmt.Errorf("budget: %s must not be negative (%d)", f.name, f.value)
		}
	}
	// The discovery allowance has a defined maximum as well as a floor: an explicit
	// caller value above it fails loudly rather than silently widening the window
	// (the environment path clamps instead, so a malformed override never fails a
	// run). A zero value stays valid: it means "unset", resolved to the default.
	if b.DiscoveryIterations > MaxDiscoveryIterations {
		return fmt.Errorf("budget: discovery_iterations must not exceed %d (%d)", MaxDiscoveryIterations, b.DiscoveryIterations)
	}
	return nil
}

// positiveInt parses s as a positive integer, or returns fallback.
func positiveInt(s string, fallback int) int {
	v := strings.TrimSpace(s)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

// boundedPositiveInt is positiveInt with the parsed value clamped to [min, max].
// An absent, blank, non-numeric, or non-positive value keeps fallback verbatim, so
// a caller's base is never silently rewritten. A present, positive value is clamped
// rather than rejected, so a malformed or excessive operator override keeps the run
// bounded instead of failing it.
func boundedPositiveInt(s string, fallback, min, max int) int {
	n := positiveInt(s, 0)
	if n == 0 {
		return fallback
	}
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}
