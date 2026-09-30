package provider

// HealthStatus is the reachability of a provider runtime.
type HealthStatus string

const (
	// HealthHealthy: the runtime answered and appears usable.
	HealthHealthy HealthStatus = "healthy"
	// HealthUnavailable: the runtime could not be reached or refused the probe.
	HealthUnavailable HealthStatus = "unavailable"
	// HealthDegraded: the runtime answered but reported a degraded state.
	HealthDegraded HealthStatus = "degraded"
	// HealthUnknown: reachability could not be determined (for example the
	// command provider, which exposes no endpoint to probe).
	HealthUnknown HealthStatus = "unknown"
)

// Valid reports whether the status is one of the defined values.
func (s HealthStatus) Valid() bool {
	switch s {
	case HealthHealthy, HealthUnavailable, HealthDegraded, HealthUnknown:
		return true
	default:
		return false
	}
}

// String renders the status, defaulting an unset value to "unknown".
func (s HealthStatus) String() string {
	if s == "" {
		return string(HealthUnknown)
	}
	return string(s)
}

// HealthResult is a provider's reachability report. Message is a short,
// human-readable, NON-SECRET explanation; it MUST NOT contain credentials,
// prompts, or file contents.
type HealthResult struct {
	Status  HealthStatus
	Message string
}
