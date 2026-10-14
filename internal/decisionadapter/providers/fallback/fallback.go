// Package fallback composes two decision providers: the backup serves only when
// the primary fails. It is the smallest failover mechanism over the existing
// decision.Provider interface, and it defines no policy.
//
// The composite reports the PRIMARY provider's name so the provider-neutral
// decision contract's profile correlation check still matches (the adapter
// output's "provider" must equal the configured profile). The serving backend
// (primary or backup) is therefore NOT carried by the decision contract; callers
// that need it can read ServedBackup after Decide.
package fallback

import (
	"context"

	"github.com/imhttran/agentic-sop/decision"
)

// Provider tries primary first and backup only when primary fails.
type Provider struct {
	primary      decision.Provider
	backup       decision.Provider
	servedBackup bool
}

// New builds a primary-with-backup provider. Neither argument may be nil.
func New(primary, backup decision.Provider) *Provider {
	return &Provider{primary: primary, backup: backup}
}

// Name reports the primary provider's name, so the adapter output matches the
// configured profile.
func (p *Provider) Name() string { return p.primary.Name() }

// Available reports whether either provider can currently serve.
func (p *Provider) Available(ctx context.Context) bool {
	return p.primary.Available(ctx) || p.backup.Available(ctx)
}

// ServedBackup reports whether the most recent Decide was served by the backup.
func (p *Provider) ServedBackup() bool { return p.servedBackup }

// Decide evaluates the request with the primary, falling back to the backup only
// when the primary errors. If the context is already done after a primary
// failure, the backup is not attempted and the primary error is returned. When
// the backup serves, the result's provider is reported as the primary name so
// the profile correlation check still matches.
func (p *Provider) Decide(ctx context.Context, req decision.DecisionRequest) (decision.DecisionResult, error) {
	p.servedBackup = false
	res, err := p.primary.Decide(ctx, req)
	if err == nil {
		return res, nil
	}
	if ctx.Err() != nil {
		return decision.DecisionResult{}, err
	}
	res, err = p.backup.Decide(ctx, req)
	if err != nil {
		return decision.DecisionResult{}, err
	}
	p.servedBackup = true
	res.Provider = p.primary.Name()
	// Record which backend actually served, as advisory diagnostics only. It never
	// influences policy and is absent when the primary serves.
	if res.Diagnostics == nil {
		res.Diagnostics = map[string]string{}
	}
	res.Diagnostics["served_provider"] = p.backup.Name()
	res.Diagnostics["fallback"] = "primary unavailable"
	return res, nil
}
