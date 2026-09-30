// Package command adapts the external command provider to the provider
// interface. The command provider runs a subprocess that is already a full
// agent; it exposes no endpoint to probe, so its health is unknown, it cannot
// enumerate models, and its capabilities are undetermined.
//
// Reporting those limits honestly — rather than inventing metadata — is the
// point: a caller must not conclude a model is absent merely because this
// provider cannot list one.
package command

import (
	"context"

	"github.com/imhttran/agentic-sop/internal/provider"
)

// Provider is the command-provider adapter. It is stateless.
type Provider struct{}

// New returns a command provider.
func New() *Provider { return &Provider{} }

// ID returns the provider identity.
func (p *Provider) ID() provider.ID { return provider.Command }

// Health reports HealthUnknown: the command provider has no endpoint to probe.
func (p *Provider) Health(context.Context) provider.HealthResult {
	return provider.HealthResult{
		Status:  provider.HealthUnknown,
		Message: "command provider exposes no endpoint to probe",
	}
}

// Models reports ErrDiscoveryUnsupported: the command provider cannot enumerate
// models.
func (p *Provider) Models(context.Context) ([]provider.ModelInfo, error) {
	return nil, provider.ErrDiscoveryUnsupported
}

// Capabilities reports an undetermined set: the command provider declares no
// model capabilities, so nothing is asserted.
func (p *Provider) Capabilities(context.Context, string) (provider.Capabilities, error) {
	return provider.Capabilities{}, nil
}
