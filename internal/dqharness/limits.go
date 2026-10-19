package dqharness

import (
	"errors"
	"fmt"
	"time"
)

// Limits are the explicit evaluation limits.
//
// Two different quantities are counted:
//   - Decisions are scheduled evaluation decisions (the frozen design has 180).
//   - Requests are actual provider calls. Every retry is a request, so retries count
//     toward the request, token, and elapsed limits as well as the retry limit.
//
// Limits are enforced over the whole evaluation, including work resumed from a prior
// process: persisted usage is loaded before the first request.
type Limits struct {
	MaxDecisions int
	MaxRequests  int
	MaxRetries   int
	MaxElapsed   time.Duration
	MaxTokens    int
}

// FrozenLimits returns the limits for the frozen RDM-004 design.
func FrozenLimits() Limits {
	return Limits{MaxDecisions: 180, MaxRequests: 360, MaxRetries: 180, MaxElapsed: 90 * time.Minute, MaxTokens: 400000}
}

// ErrLimitReached reports a clean stop caused by an exhausted limit.
var ErrLimitReached = errors.New("dqharness: evaluation limit reached")

// Limiter enforces Limits over persisted plus current-process usage.
type Limiter struct {
	limits    Limits
	base      UsageState
	started   time.Time
	decisions int
	requests  int
	retries   int
	tokens    int
}

// NewLimiter starts a limiter with no prior usage.
func NewLimiter(l Limits) *Limiter { return NewLimiterWithUsage(l, UsageState{}) }

// NewLimiterWithUsage starts a limiter that continues from persisted usage.
func NewLimiterWithUsage(l Limits, base UsageState) *Limiter {
	f := FrozenLimits()
	if l.MaxDecisions <= 0 {
		l.MaxDecisions = f.MaxDecisions
	}
	if l.MaxRequests <= 0 {
		l.MaxRequests = f.MaxRequests
	}
	if l.MaxRetries < 0 {
		l.MaxRetries = 0
	}
	if l.MaxElapsed <= 0 {
		l.MaxElapsed = f.MaxElapsed
	}
	if l.MaxTokens <= 0 {
		l.MaxTokens = f.MaxTokens
	}
	return &Limiter{limits: l, base: base, started: time.Now()}
}

func (l *Limiter) usedDecisions() int { return l.base.Decisions + l.decisions }
func (l *Limiter) usedRequests() int  { return l.base.Requests + l.requests }
func (l *Limiter) usedRetries() int   { return l.base.Retries + l.retries }
func (l *Limiter) usedTokens() int    { return l.base.Tokens + l.tokens }
func (l *Limiter) usedElapsed() time.Duration {
	return time.Duration(l.base.ActiveElapsedMS)*time.Millisecond + time.Since(l.started)
}

// CanRequest reports whether another provider request may start.
func (l *Limiter) CanRequest() error {
	if l.usedRequests() >= l.limits.MaxRequests {
		return fmt.Errorf("%w: requests %d/%d", ErrLimitReached, l.usedRequests(), l.limits.MaxRequests)
	}
	if l.usedTokens() >= l.limits.MaxTokens {
		return fmt.Errorf("%w: tokens %d/%d", ErrLimitReached, l.usedTokens(), l.limits.MaxTokens)
	}
	if l.usedElapsed() >= l.limits.MaxElapsed {
		return fmt.Errorf("%w: elapsed %s/%s", ErrLimitReached, l.usedElapsed().Round(time.Second), l.limits.MaxElapsed)
	}
	return nil
}

// CanDecision reports whether another scheduled decision may start.
func (l *Limiter) CanDecision() error {
	if l.usedDecisions() >= l.limits.MaxDecisions {
		return fmt.Errorf("%w: decisions %d/%d", ErrLimitReached, l.usedDecisions(), l.limits.MaxDecisions)
	}
	return l.CanRequest()
}

// CanRetry reports whether a retry is allowed. The retry also needs a request slot.
func (l *Limiter) CanRetry() error {
	if l.usedRetries() >= l.limits.MaxRetries {
		return fmt.Errorf("%w: retries %d/%d", ErrLimitReached, l.usedRetries(), l.limits.MaxRetries)
	}
	return l.CanRequest()
}

// BeginRequest accounts a request before it starts, so an in-flight request survives a
// crash and is never lost from the totals.
func (l *Limiter) BeginRequest() { l.requests++ }

// EndRequest records the tokens returned by a completed request.
func (l *Limiter) EndRequest(tokens int) { l.tokens += tokens }

// RecordDecision accounts one completed scheduled decision.
func (l *Limiter) RecordDecision() { l.decisions++ }

// RecordRetry accounts one retry.
func (l *Limiter) RecordRetry() { l.retries++ }

// Snapshot renders the totals for durable persistence. Elapsed is active time across
// processes: persisted elapsed plus this process's running time.
func (l *Limiter) Snapshot(inFlight bool) UsageState {
	return UsageState{
		Binding:           l.base.Binding,
		Decisions:         l.usedDecisions(),
		Requests:          l.usedRequests(),
		Retries:           l.usedRetries(),
		Tokens:            l.usedTokens(),
		ActiveElapsedMS:   l.usedElapsed().Milliseconds(),
		InFlight:          inFlight,
		UncertainRequests: l.base.UncertainRequests,
	}
}
