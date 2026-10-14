package decision

import "context"

// Provider is the provider-neutral interface every decision adapter
// implements. Callers such as agentic-sop depend only on this contract, never
// on provider-specific types.
type Provider interface {
	// Name reports the stable adapter identifier (for example "nimble").
	Name() string

	// Available reports whether the provider can currently serve decisions. It
	// must not perform an inference request.
	Available(ctx context.Context) bool

	// Decide evaluates every question in req against req.State and returns a
	// normalized, provider-neutral result.
	Decide(ctx context.Context, req DecisionRequest) (DecisionResult, error)
}
