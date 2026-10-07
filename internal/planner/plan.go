// Package planner turns a project PRD into a structured Plan by asking an
// Agent and validating the result. It performs no filesystem or workflow work;
// rendering and file IO live elsewhere.
package planner

import (
	"errors"
	"fmt"
	"strings"

	"github.com/imhttran/agentic-sop/internal/domain"
)

// Plan is the machine representation of an implementation plan. PLAN.md is a
// human rendering of this structure, never the other way around.
type Plan struct {
	Project string  `json:"project"`
	Summary string  `json:"summary"`
	Stages  []Stage `json:"stages"`
	// Capabilities is the discovery inventory: the existing capabilities the work
	// depends on, each classified EXISTS/PARTIAL/MISSING/UNKNOWN with the evidence
	// behind the finding. It lets synthesis plan against the real system instead
	// of an assumed one. Optional: a plan with no upstream dependencies omits it.
	Capabilities []Capability `json:"capabilities,omitempty"`
	// Assumptions records the important inferences PLAN made, with their evidence
	// and consequence, so a planning assumption is reviewable rather than silently
	// baked into a task.
	Assumptions []Assumption `json:"assumptions,omitempty"`
}

// Stage kinds. An environment stage is a bootstrap task (development
// environment) that feature stages implicitly depend on.
const (
	KindFeature     = "feature"
	KindEnvironment = "environment"
)

// Stage is a single implementation stage.
type Stage struct {
	ID                 string   `json:"id"`
	Title              string   `json:"title"`
	Objective          string   `json:"objective"`
	Dependencies       []string `json:"dependencies"`
	Deliverables       []string `json:"deliverables"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
	// Kind is optional: "" and "feature" are ordinary stages; "environment"
	// marks a bootstrap stage feature work depends on.
	Kind string `json:"kind,omitempty"`
	// ExecutionMode is optional: "" and "implement" run the implementation agent
	// first; "verify-first" runs deterministic validation before any agent and only
	// invokes one when that validation fails.
	ExecutionMode domain.ExecutionMode `json:"execution_mode,omitempty"`
	// Requires names the capabilities this stage depends on. Each must be declared
	// in the plan's capability inventory, and a stage may not depend on one whose
	// status is UNKNOWN: synthesis must not build work on a capability it could
	// not verify.
	Requires []string `json:"requires,omitempty"`
}

// Validate performs deterministic structural validation of a Plan. It is the
// only judge of plan validity; no model is consulted.
func (p *Plan) Validate() error {
	if strings.TrimSpace(p.Project) == "" {
		return errors.New("plan: project is empty")
	}
	if strings.TrimSpace(p.Summary) == "" {
		return errors.New("plan: summary is empty")
	}
	if len(p.Stages) == 0 {
		return errors.New("plan: no stages")
	}

	ids := make(map[string]bool, len(p.Stages))
	for i, stage := range p.Stages {
		if strings.TrimSpace(stage.ID) == "" {
			return fmt.Errorf("plan: stage %d has empty id", i)
		}
		if stage.ID != strings.TrimSpace(stage.ID) {
			return fmt.Errorf("plan: stage %d id %q must not have surrounding whitespace", i, stage.ID)
		}
		if ids[stage.ID] {
			return fmt.Errorf("plan: duplicate stage id %q", stage.ID)
		}
		ids[stage.ID] = true

		if strings.TrimSpace(stage.Title) == "" {
			return fmt.Errorf("plan: stage %s has empty title", stage.ID)
		}
		if strings.TrimSpace(stage.Objective) == "" {
			return fmt.Errorf("plan: stage %s has empty objective", stage.ID)
		}
		if len(stage.AcceptanceCriteria) == 0 {
			return fmt.Errorf("plan: stage %s has no acceptance criteria", stage.ID)
		}
		switch stage.Kind {
		case "", KindFeature, KindEnvironment:
		default:
			return fmt.Errorf("plan: stage %s has unknown kind %q", stage.ID, stage.Kind)
		}
		if !domain.KnownExecutionMode(stage.ExecutionMode) {
			return fmt.Errorf("plan: stage %s has unknown execution_mode %q", stage.ID, stage.ExecutionMode)
		}
	}

	for _, stage := range p.Stages {
		for _, dep := range stage.Dependencies {
			if dep == stage.ID {
				return fmt.Errorf("plan: stage %s depends on itself", stage.ID)
			}
			if !ids[dep] {
				return fmt.Errorf("plan: stage %s has unknown dependency %q", stage.ID, dep)
			}
		}
	}

	if err := detectCycle(p.Stages); err != nil {
		return err
	}

	return p.validateCapabilities()
}

// validateCapabilities checks the capability inventory and the stages that
// depend on it. Capability names are unique and non-empty, statuses are known,
// and every stage requirement names a declared capability. A stage may not
// depend on a capability whose status is UNKNOWN, so synthesis cannot silently
// treat an unverified capability as if it exists.
//
// normalizeCapabilityName case-folds and collapses repeated whitespace, preserving
// word boundaries, so a stage requirement matches a declared capability by a
// cosmetic name variant (for example "External Tool" and "external   tool"). This
// is the one normalization used for both validation and the missing-capability
// ownership gate in CapabilityGaps, so the two can never disagree about which
// requirement names which declared capability.
func normalizeCapabilityName(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

func (p *Plan) validateCapabilities() error {
	status := make(map[string]CapabilityStatus, len(p.Capabilities))
	for i, c := range p.Capabilities {
		name := strings.TrimSpace(c.Name)
		if name == "" {
			return fmt.Errorf("plan: capability %d has empty name", i)
		}
		if c.Name != name {
			return fmt.Errorf("plan: capability %q must not have surrounding whitespace", c.Name)
		}
		if _, dup := status[c.Name]; dup {
			return fmt.Errorf("plan: duplicate capability %q", c.Name)
		}
		if !KnownCapabilityStatus(c.Status) {
			return fmt.Errorf("plan: capability %s has unknown status %q", c.Name, c.Status)
		}
		status[c.Name] = normalizeCapabilityStatus(c.Status)
	}

	for i, a := range p.Assumptions {
		if strings.TrimSpace(a.Assumption) == "" {
			return fmt.Errorf("plan: assumption %d is empty", i)
		}
	}

	// Iterate the declared slice, not the status map, so the duplicate case/
	// whitespace diagnostic is deterministic for identical input regardless of map
	// iteration order.
	normalized := make(map[string]string, len(p.Capabilities))
	for _, c := range p.Capabilities {
		key := normalizeCapabilityName(c.Name)
		if prev, ok := normalized[key]; ok {
			return fmt.Errorf("plan: capabilities %q and %q differ only by case or whitespace", prev, c.Name)
		}
		normalized[key] = c.Name
	}

	for _, stage := range p.Stages {
		for _, name := range stage.Requires {
			st, ok := status[name]
			if !ok {
				if exact, ok2 := normalized[normalizeCapabilityName(name)]; ok2 {
					st, ok = status[exact], true
				}
			}
			if !ok {
				return fmt.Errorf("plan: stage %s requires undeclared capability %q", stage.ID, name)
			}
			if st == CapabilityUnknown {
				return fmt.Errorf("plan: stage %s requires capability %q whose status is UNKNOWN; verify it before depending on it", stage.ID, name)
			}
		}
	}

	return nil
}

// detectCycle reports the first dependency cycle, naming the path so the error
// is actionable. Dependencies were already checked to reference known stages.
func detectCycle(stages []Stage) error {
	const (
		white = 0
		grey  = 1
		black = 2
	)

	state := make(map[string]int, len(stages))
	deps := make(map[string][]string, len(stages))
	for _, stage := range stages {
		deps[stage.ID] = stage.Dependencies
	}

	var visit func(id string, path []string) error
	visit = func(id string, path []string) error {
		switch state[id] {
		case grey:
			return fmt.Errorf("plan: dependency cycle: %s", strings.Join(append(path, id), " -> "))
		case black:
			return nil
		}
		state[id] = grey
		for _, dep := range deps[id] {
			if err := visit(dep, append(path, id)); err != nil {
				return err
			}
		}
		state[id] = black
		return nil
	}

	for _, stage := range stages {
		if err := visit(stage.ID, nil); err != nil {
			return err
		}
	}
	return nil
}
