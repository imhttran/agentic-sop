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
//   - the productive-discovery window (implementNowAfter) is a soft steering
//     threshold with no exhaustion of its own — the stale guard is the hard bound;
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
}

// Built-in defaults. These are the values the harness shipped with, so an
// unconfigured project is byte-for-byte unchanged.
const (
	DefaultImplementIterations = 32
	DefaultFixIterations       = 24
	DefaultStaleIterations     = 5
	DefaultToolCalls           = 80
)

// Environment variable names, following the existing SOP_OLLAMA_* override
// convention. An unset, blank, non-numeric, or non-positive value keeps the
// default, so a malformed value can never disable a limit.
const (
	EnvImplementIterations = "SOP_OLLAMA_IMPLEMENT_ITERATIONS"
	EnvFixIterations       = "SOP_OLLAMA_FIX_ITERATIONS"
	EnvStaleIterations     = "SOP_OLLAMA_STALE_ITERATIONS"
	EnvToolCalls           = "SOP_OLLAMA_TOOL_CALLS"
)

// Defaults returns the built-in budget.
func Defaults() Budget {
	return Budget{
		ImplementIterations: DefaultImplementIterations,
		FixIterations:       DefaultFixIterations,
		StaleIterations:     DefaultStaleIterations,
		ToolCalls:           DefaultToolCalls,
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
	}
	for _, f := range fields {
		if f.value < 0 {
			return fmt.Errorf("budget: %s must not be negative (%d)", f.name, f.value)
		}
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
