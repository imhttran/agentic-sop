package model

// Execution-source values: where a run's executing model came from, recorded
// beside the routing selection so persisted evidence distinguishes the routing
// DECISION (the class) from the actual EXECUTION target.
//
// These are a small, closed, typed set — never model-generated prose — so the
// distinction stays deterministic and auditable. They are non-secret: an
// ExecutionTarget records the provider, model, locality, and source, never a
// credential.
type ExecutionSource string

const (
	// ExecutionSourcePrimary: the class's primary model executed. This is the
	// normal case for every class.
	ExecutionSourcePrimary ExecutionSource = "primary"
	// ExecutionSourceAvailabilityFallback: a LOCAL class's configured cloud
	// fallback executed instead of the primary, because a read-only availability
	// observation reported the local runtime could not serve the primary. The
	// routing class is unchanged; only the concrete provider/model differ. It is
	// never produced by a generation, validation, review, or quality-gate failure.
	ExecutionSourceAvailabilityFallback ExecutionSource = "availability-fallback"
)

// Valid reports whether s is one of the defined execution sources.
func (s ExecutionSource) Valid() bool {
	switch s {
	case ExecutionSourcePrimary, ExecutionSourceAvailabilityFallback:
		return true
	default:
		return false
	}
}

// String renders the source, defaulting an unset value to "primary" so a target
// built without an explicit source reads as the primary execution.
func (s ExecutionSource) String() string {
	if s == "" {
		return string(ExecutionSourcePrimary)
	}
	return string(s)
}

// ExecutionTarget is the resolved execution target for a run, recorded separately
// from the routing Selection so persisted evidence keeps the routing CLASS
// (for example SMALL) while making the ACTUAL execution model and its source
// explicit. It is a typed, non-secret record: provider, model, locality, and an
// execution source — never a credential and never free-form decision text.
type ExecutionTarget struct {
	// Class is the routing class carried over from the routing Selection,
	// UNCHANGED. Availability fallback never rewrites it (SMALL stays SMALL).
	Class Class `json:"class"`
	// Provider is the executing provider (for example ollama).
	Provider string `json:"provider"`
	// Model is the executing model name, which may differ from the routing
	// selection's model when a fallback applied.
	Model string `json:"model"`
	// Locality is the executing model's configured locality (authoritative
	// configuration, never inferred from a model name).
	Locality Locality `json:"locality"`
	// Source records where the executing model came from (primary or
	// availability-fallback). It is a typed value, not prose.
	Source ExecutionSource `json:"execution_source"`
}

// ExecutionTargetForPrimary returns the execution target for a routing selection
// that runs its primary model: the selection's provider/model/locality, with the
// execution source set to primary and the routing class carried over unchanged.
func ExecutionTargetForPrimary(sel Selection) ExecutionTarget {
	return ExecutionTarget{
		Class:    sel.Class,
		Provider: sel.Provider,
		Model:    sel.Model,
		Locality: sel.Locality,
		Source:   ExecutionSourcePrimary,
	}
}

// ExecutionTargetForFallback returns the execution target for a routing selection
// whose class's configured availability fallback executes. It carries the routing
// class over UNCHANGED (the selected class must remain, for example, SMALL) and
// takes the provider/model/locality from the fallback selection, with the source
// set to availability-fallback.
func ExecutionTargetForFallback(routing Selection, fallback Selection) ExecutionTarget {
	return ExecutionTarget{
		Class:    routing.Class,
		Provider: fallback.Provider,
		Model:    fallback.Model,
		Locality: fallback.Locality,
		Source:   ExecutionSourceAvailabilityFallback,
	}
}
