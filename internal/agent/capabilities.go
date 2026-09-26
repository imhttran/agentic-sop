package agent

import (
	"context"
	"fmt"
	"strings"
)

// capabilityOrder is the canonical, stable order used when listing capabilities.
var capabilityOrder = []Capability{Plan, DesignTests, Implement, DiagnoseFailure, Fix, Review}

// Capabilities is the set of capabilities an agent declares it can serve. It is
// the routing vocabulary: the workflow asks for a capability, and a provider
// that cannot serve it is rejected clearly instead of being sent the request.
type Capabilities map[Capability]bool

// AllCapabilities returns the full set the transport defines.
func AllCapabilities() Capabilities {
	c := make(Capabilities, len(capabilityOrder))
	for _, capability := range capabilityOrder {
		c[capability] = true
	}
	return c
}

// NewCapabilities returns a set containing exactly caps.
func NewCapabilities(caps ...Capability) Capabilities {
	c := make(Capabilities, len(caps))
	for _, capability := range caps {
		c[capability] = true
	}
	return c
}

// Supports reports whether the set contains capability.
func (c Capabilities) Supports(capability Capability) bool { return c[capability] }

// List returns the supported capabilities in a stable order.
func (c Capabilities) List() []Capability {
	out := make([]Capability, 0, len(c))
	for _, capability := range capabilityOrder {
		if c[capability] {
			out = append(out, capability)
		}
	}
	return out
}

// Declarer is implemented by agents that serve a restricted set of capabilities.
// An agent that does not implement it is assumed to serve every capability.
type Declarer interface {
	Capabilities() Capabilities
}

// CapabilitiesOf returns a's declared capabilities, or all capabilities when a
// does not declare any (or declares none).
func CapabilitiesOf(a Agent) Capabilities {
	if d, ok := a.(Declarer); ok {
		if c := d.Capabilities(); c != nil {
			return c
		}
	}
	return AllCapabilities()
}

// Checked wraps an Agent and rejects a request whose capability the underlying
// agent does not declare. It is the routing guard: an unsupported role/provider
// combination fails with a message naming the supported set, rather than being
// sent to a provider that cannot serve it.
type Checked struct {
	inner Agent
	caps  Capabilities
}

// NewChecked wraps a, using a's declared capabilities.
func NewChecked(a Agent) *Checked {
	return &Checked{inner: a, caps: CapabilitiesOf(a)}
}

// Capabilities reports the wrapped agent's capabilities.
func (c *Checked) Capabilities() Capabilities { return c.caps }

// Generate rejects an unsupported capability and otherwise delegates. The
// request is validated first, so an invalid request never reaches the provider.
func (c *Checked) Generate(ctx context.Context, request Request) (Response, error) {
	if err := request.Validate(); err != nil {
		return Response{}, err
	}
	if !c.caps.Supports(request.Capability) {
		return Response{}, fmt.Errorf("agent does not support capability %s (supported: %s)",
			request.Capability, joinCapabilities(c.caps.List()))
	}
	return c.inner.Generate(ctx, request)
}

// joinCapabilities renders a capability list for error messages.
func joinCapabilities(caps []Capability) string {
	names := make([]string, len(caps))
	for i, capability := range caps {
		names[i] = string(capability)
	}
	return strings.Join(names, ", ")
}
