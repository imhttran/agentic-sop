package perf

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// EnvMaxTotalTokens is the operator's per-run token budget: the input plus
// output tokens every model call under one task or prompt may use together.
// Unset means no budget.
const EnvMaxTotalTokens = "SOP_AGENT_MAX_TOTAL_TOKENS"

// ErrTokenBudgetExceeded is returned before dispatch when the run has used its
// token budget, or when the budget cannot be enforced.
var ErrTokenBudgetExceeded = errors.New("token budget exceeded")

// CheckTokenBudget is called before every model request. With a budget set it
// refuses the request when the run's reported tokens have reached the budget,
// and fails closed when usage cannot be measured: no usage collector on ctx,
// or a provider that did not report usage for an earlier call.
// ponytail: checked before each request, so the request that crosses the
// budget still completes; the overshoot is bounded by one request's input plus
// SOP_AGENT_MAX_OUTPUT_TOKENS. It is a token cap, not a dollar cap: SOP has no
// pricing data.
func CheckTokenBudget(ctx context.Context) error {
	raw := strings.TrimSpace(os.Getenv(EnvMaxTotalTokens))
	if raw == "" {
		return nil
	}
	budget, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || budget <= 0 {
		return fmt.Errorf("%w: %s: invalid value %q: want a positive integer", ErrTokenBudgetExceeded, EnvMaxTotalTokens, raw)
	}
	u, _ := ctx.Value(usageKey{}).(*Usage)
	if u == nil {
		return fmt.Errorf("%w: %s is set but this run has no usage accounting", ErrTokenBudgetExceeded, EnvMaxTotalTokens)
	}
	sum := TotalTokens(UsageFrom(ctx))
	if sum.UnreportedCalls > 0 {
		return fmt.Errorf("%w: %d earlier call(s) did not report usage, so %s cannot be enforced", ErrTokenBudgetExceeded, sum.UnreportedCalls, EnvMaxTotalTokens)
	}
	// Negative reports are never counted, so a negative total or sum means the
	// int64 counters overflowed: fail closed instead of reading it as small.
	if used := sum.Input + sum.Output; sum.Input < 0 || sum.Output < 0 || used < 0 || used >= budget {
		return fmt.Errorf("%w: %d of %d tokens used (%s)", ErrTokenBudgetExceeded, used, budget, EnvMaxTotalTokens)
	}
	return nil
}
