package hipcompare

import (
	"fmt"
	"sort"
	"strings"
)

// Fixed budget defaults for the HIP-003 pilot. They are fixed in code: a caller
// cannot raise them, and a configuration that exceeds any of them is rejected
// before any adapter invocation.
const (
	// MaxPairs is the maximum number of baseline/candidate pairs.
	MaxPairs = 3
	// MaxHeldOutPairs is the maximum number of additional held-out pairs.
	MaxHeldOutPairs = 1
	// MaxExecutions is the absolute execution ceiling across every arm.
	MaxExecutions = 8
	// DefaultPerRunTimeLimitSeconds is the fixed per-run wall-clock limit.
	DefaultPerRunTimeLimitSeconds = 900
	// DefaultPerRunTokenLimit is the fixed per-run token limit.
	DefaultPerRunTokenLimit = 200000
)

// Budget is the comparison budget. Every field must be present and within the
// fixed ceiling; Validate rejects an absent or excessive limit. The zero value is
// never a valid budget: a caller must opt into explicit values.
//
// Defaults come from the existing SOP budget controls when they are configured
// (see ResolveBudget); otherwise the fixed pilot defaults apply. A caller may
// lower the counts below the ceiling, for example to run one pair, but may never
// exceed any ceiling.
type Budget struct {
	Pairs             int `json:"pairs"`
	HeldOutPairs      int `json:"held_out_pairs"`
	MaxExecutions     int `json:"max_executions"`
	PerRunTimeSeconds int `json:"per_run_time_limit_seconds"`
	PerRunTokenLimit  int `json:"per_run_token_limit"`
	// AutomaticRetries is the number of automatic retries. It must be zero: a failed
	// or timed-out arm is recorded, never retried.
	AutomaticRetries int `json:"automatic_retries"`
}

// DefaultBudget returns the fixed comparison budget.
func DefaultBudget() Budget {
	return Budget{
		Pairs:             MaxPairs,
		HeldOutPairs:      MaxHeldOutPairs,
		MaxExecutions:     MaxExecutions,
		PerRunTimeSeconds: DefaultPerRunTimeLimitSeconds,
		PerRunTokenLimit:  DefaultPerRunTokenLimit,
		AutomaticRetries:  0,
	}
}

// ResolveBudget takes a caller-supplied budget and fills only the unset fields
// from the existing default. A caller can therefore lower Pairs to run a single
// pair, but can never raise a ceiling. The result is validated, so an excessive
// or incomplete budget is rejected before execution.
func ResolveBudget(caller Budget) (Budget, error) {
	def := DefaultBudget()
	if caller.Pairs <= 0 {
		caller.Pairs = def.Pairs
	}
	if caller.HeldOutPairs < 0 {
		return Budget{}, fmt.Errorf("budget: held_out_pairs must not be negative (got %d)", caller.HeldOutPairs)
	}
	if caller.MaxExecutions <= 0 {
		caller.MaxExecutions = def.MaxExecutions
	}
	if caller.PerRunTimeSeconds <= 0 {
		caller.PerRunTimeSeconds = def.PerRunTimeSeconds
	}
	if caller.PerRunTokenLimit <= 0 {
		caller.PerRunTokenLimit = def.PerRunTokenLimit
	}
	if err := caller.Validate(); err != nil {
		return Budget{}, err
	}
	return caller, nil
}

// Validate rejects a budget that exceeds the fixed counts or omits a per-run
// time or token limit.
func (b Budget) Validate() error {
	if b.Pairs <= 0 {
		return fmt.Errorf("budget: pairs must be positive (got %d)", b.Pairs)
	}
	if b.Pairs > MaxPairs {
		return fmt.Errorf("budget: pairs %d exceeds the fixed maximum %d", b.Pairs, MaxPairs)
	}
	if b.HeldOutPairs < 0 {
		return fmt.Errorf("budget: held_out_pairs must not be negative (got %d)", b.HeldOutPairs)
	}
	if b.HeldOutPairs > MaxHeldOutPairs {
		return fmt.Errorf("budget: held_out_pairs %d exceeds the fixed maximum %d", b.HeldOutPairs, MaxHeldOutPairs)
	}
	if b.MaxExecutions <= 0 {
		return fmt.Errorf("budget: max_executions must be positive (got %d)", b.MaxExecutions)
	}
	if b.MaxExecutions > MaxExecutions {
		return fmt.Errorf("budget: max_executions %d exceeds the fixed maximum %d", b.MaxExecutions, MaxExecutions)
	}
	required := 2 * (b.Pairs + b.HeldOutPairs)
	if required > b.MaxExecutions {
		return fmt.Errorf("budget: %d pairs plus %d held-out pair(s) need %d executions, exceeding max_executions %d", b.Pairs, b.HeldOutPairs, required, b.MaxExecutions)
	}
	if b.PerRunTimeSeconds <= 0 {
		return fmt.Errorf("budget: per_run_time_limit_seconds is required and must be positive (got %d)", b.PerRunTimeSeconds)
	}
	if b.PerRunTimeSeconds > DefaultPerRunTimeLimitSeconds {
		return fmt.Errorf("budget: per_run_time_limit_seconds %d exceeds the fixed maximum %d", b.PerRunTimeSeconds, DefaultPerRunTimeLimitSeconds)
	}
	if b.PerRunTokenLimit <= 0 {
		return fmt.Errorf("budget: per_run_token_limit is required and must be positive (got %d)", b.PerRunTokenLimit)
	}
	if b.PerRunTokenLimit > DefaultPerRunTokenLimit {
		return fmt.Errorf("budget: per_run_token_limit %d exceeds the fixed maximum %d", b.PerRunTokenLimit, DefaultPerRunTokenLimit)
	}
	if b.AutomaticRetries != 0 {
		return fmt.Errorf("budget: automatic_retries must be 0 (got %d): the comparison never retries automatically", b.AutomaticRetries)
	}
	return nil
}

// ExecutionCeiling is the maximum number of arm executions this budget permits.
func (b Budget) ExecutionCeiling() int {
	ceiling := 2 * (b.Pairs + b.HeldOutPairs)
	if ceiling > MaxExecutions {
		return MaxExecutions
	}
	return ceiling
}

// CanonicalSettings renders the shared executor/model/settings/tool-permission
// identity the fingerprint hashes. Both arms receive exactly this string.
func CanonicalSettings(executor, model, settings string, toolPermissions []string) string {
	perms := append([]string{}, toolPermissions...)
	sort.Strings(perms)
	return fmt.Sprintf("executor=%s\nmodel=%s\nsettings=%s\ntools=%s",
		executor, model, settings, strings.Join(perms, ","))
}
