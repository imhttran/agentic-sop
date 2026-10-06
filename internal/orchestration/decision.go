package orchestration

import (
	"sort"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// Mode is the orchestration execution-mode decision: how many workers run for a
// set of tasks. It is orthogonal to domain.ExecutionMode, which describes how a
// single task runs. Mode is a plan/task metadata value and is never inferred
// from a task's title or prose.
type Mode string

const (
	// ModeSingle runs one agent: no split is justified. It is the deterministic
	// default whenever the inputs do not positively justify more.
	ModeSingle Mode = "SINGLE"
	// ModeSequential runs workers one after another because the units are ordered
	// by dependencies.
	ModeSequential Mode = "SEQUENTIAL"
	// ModeParallel runs workers concurrently because the units are mutually
	// independent, capability-satisfiable, and non-overlapping.
	ModeParallel Mode = "PARALLEL"
)

// Request is the pure, deterministic input to Decide. It carries only
// domain-level, provider-neutral data: task identifiers, the dependency edges
// among them, the capability each task requires, and the set of capabilities
// available.
type Request struct {
	// Units lists the work the orchestrator is considering, one entry per task.
	Units []Unit
	// Dependencies lists directed edges keyed by task ID: Dependencies[id] is the
	// set of task IDs that task id depends on. A dependency on an ID not present in
	// Units is treated as unsatisfiable-in-this-set.
	Dependencies map[string][]string
	// Available is the set of capabilities that can be satisfied. A unit whose
	// capability is absent makes the input unsatisfiable, which defaults to SINGLE.
	Available agent.Capabilities
}

// Unit is a single task considered by the decision, expressed as domain/agent
// values only.
type Unit struct {
	ID         string
	Capability agent.Capability
}

// Decide returns the deterministic orchestration mode for the request.
//
// The rule set, evaluated in order, is:
//
//  1. SINGLE when fewer than two units are present (the lone-task and empty
//     cases). SINGLE remains the efficient default.
//  2. SINGLE when any unit is not a complete capability description (blank ID or
//     blank capability): the input is ambiguous and defaults to SINGLE.
//  3. SINGLE when any unit requires a capability that is not available: an
//     unsatisfiable request cannot be split into workers.
//  4. SEQUENTIAL when any dependency edge exists among the units (there is
//     ordering), or a dependency reference cannot be resolved within the set.
//  5. PARALLEL when two or more distinct units are mutually independent and
//     capability-satisfiable and no two units share the same task ID
//     (non-overlapping).
//
// Decide performs no I/O, consults no provider or model, and is independent of
// map iteration order, so identical inputs always yield identical modes.
func Decide(req Request) Mode {
	if len(req.Units) < 2 {
		return ModeSingle
	}

	// Deterministic ordering: sort a copy of the units by ID so results never
	// depend on caller slice or map order.
	units := make([]Unit, len(req.Units))
	copy(units, req.Units)
	sort.Slice(units, func(i, j int) bool { return units[i].ID < units[j].ID })

	seen := make(map[string]bool, len(units))
	for _, u := range units {
		if u.ID == "" || u.Capability == "" {
			return ModeSingle
		}
		if seen[u.ID] {
			// Overlapping units share a task; not independent.
			return ModeSequential
		}
		seen[u.ID] = true
	}

	// Capability satisfiability: every requested capability must be available.
	// A nil set means "unknown" and is treated as satisfiable so the decision
	// stays purely structural; callers that know the set should supply it.
	if req.Available != nil {
		for _, u := range units {
			if !req.Available.Supports(u.Capability) {
				return ModeSingle
			}
		}
	}

	// Any dependency edge among the units implies ordering.
	for _, u := range units {
		deps := req.Dependencies[u.ID]
		if len(deps) == 0 {
			continue
		}
		for _, dep := range deps {
			if _, ok := seen[dep]; ok {
				return ModeSequential
			}
			// A dangling dependency reference is unsatisfiable in this set.
			return ModeSequential
		}
	}

	return ModeParallel
}
