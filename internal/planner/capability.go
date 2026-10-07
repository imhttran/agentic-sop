package planner

import (
	"errors"
	"fmt"
	"strings"
)

// CapabilityStatus records whether an existing system already provides a
// capability the requested work depends on. PLAN discovery assigns one of these
// to every capability the plan relies on, so synthesis plans against what the
// repository actually provides instead of an assumed architecture.
type CapabilityStatus string

// The four possible findings. They are deliberately coarse: the point is to
// distinguish a verified capability from an assumed one before tasks are
// created, not to describe an arbitrary amount of detail.
const (
	// CapabilityExists: the capability is present and usable in the repository.
	CapabilityExists CapabilityStatus = "EXISTS"
	// CapabilityPartial: part of the capability is present and part is missing
	// (for example a read operation exists while its command counterpart does
	// not), so the plan must reflect the split rather than assume the whole.
	CapabilityPartial CapabilityStatus = "PARTIAL"
	// CapabilityMissing: the repository does not provide the capability.
	CapabilityMissing CapabilityStatus = "MISSING"
	// CapabilityUnknown: discovery could not establish whether the capability
	// exists. It must stay explicit and must never be treated as EXISTS.
	CapabilityUnknown CapabilityStatus = "UNKNOWN"
)

// KnownCapabilityStatus reports whether s names a recognized capability status.
// Comparison ignores surrounding whitespace and case, so a model that writes
// "missing" is understood without a repair round.
func KnownCapabilityStatus(s CapabilityStatus) bool {
	switch normalizeCapabilityStatus(s) {
	case CapabilityExists, CapabilityPartial, CapabilityMissing, CapabilityUnknown:
		return true
	default:
		return false
	}
}

// normalizeCapabilityStatus returns the canonical uppercase form of a status.
func normalizeCapabilityStatus(s CapabilityStatus) CapabilityStatus {
	return CapabilityStatus(strings.ToUpper(strings.TrimSpace(string(s))))
}

// Capability is one capability the requested work depends on, as discovered in
// the existing system during PLAN. It records more than a yes/no: the evidence
// behind the finding and, when the capability is absent or partial, which layer
// owns providing it — the information synthesis needs to decide whether a gap
// can be handled by the plan or needs a human decision.
type Capability struct {
	// Name is the requested capability in the plan's own vocabulary (for
	// example "CancelRun" or "sop reconcile").
	Name string `json:"name"`
	// Status is EXISTS | PARTIAL | MISSING | UNKNOWN.
	Status CapabilityStatus `json:"status"`
	// Evidence is the repository observation behind Status: a path, symbol, or
	// command result. It is what makes the finding reviewable.
	Evidence string `json:"evidence,omitempty"`
	// Location is where an existing capability lives, when one was found.
	Location string `json:"location,omitempty"`
	// Owner names the layer that must provide a missing or partial capability
	// (for example "SOP" or "controller"). A determined owner means the requested
	// architecture already decides who does the work, so the gap does not need a
	// human decision.
	Owner string `json:"owner,omitempty"`
	// Gap explains what is absent for a partial, missing, or unknown capability.
	Gap string `json:"gap,omitempty"`
	// Resolution records how the plan handles the gap without a human, for
	// example "the controller exposes it as unsupported until SOP provides it"
	// or a prerequisite stage that implements it in the owning layer.
	Resolution string `json:"resolution,omitempty"`
}

// Assumption is one important assumption PLAN made, with the evidence behind it
// and its consequence for the plan. Recording an assumption keeps a planning
// inference reviewable instead of silently baked into a task.
type Assumption struct {
	// Assumption states the inferred fact.
	Assumption string `json:"assumption"`
	// Evidence is the repository basis for the assumption.
	Evidence string `json:"evidence,omitempty"`
	// Consequence is what the assumption means for the plan.
	Consequence string `json:"consequence,omitempty"`
}

// gapResolved reports whether the plan records enough to handle a non-EXISTS
// capability without a human: a determined owning layer, or an explicit
// resolution.
func (c Capability) gapResolved() bool {
	return strings.TrimSpace(c.Owner) != "" || strings.TrimSpace(c.Resolution) != ""
}

// CapabilityGaps splits the plan's declared capabilities into the gaps the plan
// handles on its own and the gaps that need a human decision.
//
// A non-EXISTS capability that some stage depends on is handled when the owning
// layer or an explicit resolution is recorded: the requested architecture
// already determines the safe behavior, so the plan records the gap and
// continues. Without either, ownership is a genuine product or architectural
// choice the requirements do not settle, so the gap needs a human. A capability
// no stage depends on is inventory, not a blocker.
//
// Requirement matching is normalized with normalizeCapabilityName on both sides,
// exactly as validateCapabilities matches them, so validateCapabilities accepting a
// case/whitespace variant cannot let an unresolved dependency bypass this ownership
// gate by spelling a stage Require with that variant.
func (p *Plan) CapabilityGaps() (handled, needsHuman []Capability) {
	required := make(map[string]bool)
	for _, stage := range p.Stages {
		for _, name := range stage.Requires {
			required[normalizeCapabilityName(name)] = true
		}
	}
	for _, c := range p.Capabilities {
		if !required[normalizeCapabilityName(c.Name)] || normalizeCapabilityStatus(c.Status) == CapabilityExists {
			continue
		}
		if c.gapResolved() {
			handled = append(handled, c)
		} else {
			needsHuman = append(needsHuman, c)
		}
	}
	return handled, needsHuman
}

// CapabilityGapError reports capabilities a plan depends on whose owning layer
// the requirements do not determine. It is a genuine planning ambiguity rather
// than a structural defect, so it is surfaced to a human instead of being
// returned to the agent for a repair round that could invent an owner.
type CapabilityGapError struct {
	Capabilities []Capability
}

// Error renders the ambiguity and what the human must decide. It leads with
// NEEDS_HUMAN so the human gate is recognizable without special handling.
func (e *CapabilityGapError) Error() string {
	var b strings.Builder
	b.WriteString("NEEDS_HUMAN: the plan depends on capabilities whose owner the requirements do not determine:\n")
	for _, c := range e.Capabilities {
		fmt.Fprintf(&b, "\n  %s (%s)", c.Name, normalizeCapabilityStatus(c.Status))
		if g := strings.TrimSpace(c.Gap); g != "" {
			fmt.Fprintf(&b, ": %s", g)
		}
	}
	b.WriteString("\n\nPLAN will not invent an owning layer or an implementation. Decide who owns\neach capability (or scope it out), then rerun.")
	return b.String()
}

// IsNeedsHuman reports whether err is a planning ambiguity that must reach a
// human rather than being repaired by an agent.
func IsNeedsHuman(err error) bool {
	var gap *CapabilityGapError
	return errors.As(err, &gap)
}

// acceptPlan applies the missing-capability policy to a structurally valid plan.
// A gap whose owner or resolution the plan records is handled by the plan; a gap
// with neither is a genuine ambiguity and is surfaced as a human decision.
func acceptPlan(plan *Plan) (*Plan, error) {
	_, needsHuman := plan.CapabilityGaps()
	if len(needsHuman) > 0 {
		return nil, &CapabilityGapError{Capabilities: needsHuman}
	}
	return plan, nil
}
