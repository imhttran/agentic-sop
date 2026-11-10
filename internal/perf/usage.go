package perf

import (
	"context"
	"sort"
	"sync"
)

// Tokens is one model's token usage as its server reported it.
type Tokens struct {
	Calls  int64 `json:"calls"`
	Input  int64 `json:"input"`
	Output int64 `json:"output"`
	// UnreportedCalls counts calls whose server did not report usage. It is kept
	// separate from Calls so an absent report is never rendered as zero tokens: the
	// totals above cover only the calls that actually reported usage.
	UnreportedCalls int64 `json:"unreported_calls,omitempty"`
}

// add combines two token records, including the unreported-call count.
func (t Tokens) add(o Tokens) Tokens {
	t.Calls += o.Calls
	t.Input += o.Input
	t.Output += o.Output
	t.UnreportedCalls += o.UnreportedCalls
	return t
}

// Usage collects the token usage of every model call made under one context,
// whichever stage or client made it (implement, fix, plan, review, JEV). It is
// safe for concurrent use. It is diagnostic only: counts come from the server's
// own report, and a server that reports none adds a call with zero tokens.
type Usage struct {
	mu      sync.Mutex
	byModel map[string]Tokens
}

type usageKey struct{}

// WithUsage returns ctx carrying u, so model clients below it can report into it.
func WithUsage(ctx context.Context, u *Usage) context.Context {
	return context.WithValue(ctx, usageKey{}, u)
}

// AddUsage records one model call on the Usage carried by ctx, if any. It treats
// the call as having reported usage; use AddUsageWithReport when the server may
// omit it.
func AddUsage(ctx context.Context, model string, input, output int) {
	AddUsageWithReport(ctx, model, input, output, true)
}

// AddUsageWithReport records one model call and whether its server reported
// token usage. An unreported call increments UnreportedCalls instead of silently
// recording zero tokens, so a missing report stays distinguishable from a
// measured zero.
func AddUsageWithReport(ctx context.Context, model string, input, output int, reported bool) {
	u, _ := ctx.Value(usageKey{}).(*Usage)
	if u == nil {
		return
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.byModel == nil {
		u.byModel = map[string]Tokens{}
	}
	t := u.byModel[model]
	t.Calls++
	// A negative count is not a usable report: counting it would lower the
	// total and let a run slip under its token budget.
	if reported && input >= 0 && output >= 0 {
		t.Input += int64(input)
		t.Output += int64(output)
	} else {
		t.UnreportedCalls++
	}
	u.byModel[model] = t
}

// UsageFrom returns a copy of the usage carried by ctx, or nil when there is none
// or no call was recorded.
func UsageFrom(ctx context.Context) map[string]Tokens {
	u, _ := ctx.Value(usageKey{}).(*Usage)
	if u == nil {
		return nil
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if len(u.byModel) == 0 {
		return nil
	}
	out := make(map[string]Tokens, len(u.byModel))
	for m, t := range u.byModel {
		out[m] = t
	}
	return out
}

// TotalTokens sums a usage map across models.
func TotalTokens(byModel map[string]Tokens) Tokens {
	var sum Tokens
	for _, t := range byModel {
		sum = sum.add(t)
	}
	return sum
}

// sortedModels lists a usage map's models in a stable order.
func sortedModels(byModel map[string]Tokens) []string {
	names := make([]string, 0, len(byModel))
	for m := range byModel {
		names = append(names, m)
	}
	sort.Strings(names)
	return names
}

func addTokens(a, b map[string]Tokens) map[string]Tokens {
	if len(b) == 0 {
		return a
	}
	out := make(map[string]Tokens, len(a)+len(b))
	for m, t := range a {
		out[m] = t
	}
	for m, t := range b {
		s := out[m]
		s = s.add(t)
		out[m] = s
	}
	return out
}
