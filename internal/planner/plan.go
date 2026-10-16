// Package planner turns a project PRD into a structured Plan by asking an
// Agent and validating the result. It performs no filesystem or workflow work;
// rendering and file IO live elsewhere.
package planner

import (
	"errors"
	"fmt"
	"strings"

	"github.com/imhttran/agentic-sop/internal/budget"
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
	// AcceptanceChecks are operator-declared deterministic acceptance checks for
	// the stage: each is a shell command whose exit status 0 means the criterion
	// is met. They are the verifiable half of acceptance (unlike the free-text
	// AcceptanceCriteria) and are carried in plan.json, in rendered PLAN.md, and
	// in task files. Optional: a stage with no checks omits the key so a plan
	// without checks stays byte-identical to the previous format.
	AcceptanceChecks []string `json:"acceptance_checks,omitempty"`
	// Kind is optional: "" and "feature" are ordinary stages; "environment"
	// marks a bootstrap stage feature work depends on.
	Kind string `json:"kind,omitempty"`
	// ExecutionMode is optional: "" and "implement" run the implementation agent
	// first; "verify-first" runs deterministic validation before any agent and only
	// invokes one when that validation fails.
	ExecutionMode domain.ExecutionMode `json:"execution_mode,omitempty"`
	// DiscoveryIterations is optional: the per-task discovery allowance (IMPLEMENT/FIX
	// model turns of credited repository discovery before non-mutating turns become
	// stale). It overrides the operator environment allowance and the built-in default
	// for THIS task only; a zero or absent value keeps the environment/default
	// resolution. It is bounded by the same limits as the operator allowance and can
	// never exceed the capability's hard iteration ceiling.
	DiscoveryIterations int `json:"discovery_iterations,omitempty"`
	// Requires names the capabilities this stage depends on. Each must be declared
	// in the plan's capability inventory, and a stage may not depend on one whose
	// status is UNKNOWN: synthesis must not build work on a capability it could
	// not verify.
	Requires []string `json:"requires,omitempty"`
	// RequiredEvidence names external retrieval evidence this stage requires
	// before it may run (RAG-SOP-003). Each item is "corpus:<id>" or
	// "source:<prefix>". An empty list (the default) declares no required evidence
	// and preserves the existing behavior exactly. Syntax is validated in
	// Validate; authorization is enforced at execution time against the operator
	// allowlist.
	RequiredEvidence []string `json:"required_evidence,omitempty"`
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
		if stage.DiscoveryIterations < 0 {
			return fmt.Errorf("plan: stage %s has negative discovery_iterations %d", stage.ID, stage.DiscoveryIterations)
		}
		if stage.DiscoveryIterations > budget.MaxDiscoveryIterations {
			return fmt.Errorf("plan: stage %s discovery_iterations %d exceeds the maximum %d", stage.ID, stage.DiscoveryIterations, budget.MaxDiscoveryIterations)
		}
	}

	// Required evidence (RAG-SOP-003): validate each declaration's syntax and
	// normalize duplicates deterministically. A stage with no declarations keeps
	// a nil slice, so an unaffected plan is byte-identical to before.
	for i := range p.Stages {
		normalized, err := NormalizeRequiredEvidence(p.Stages[i].RequiredEvidence)
		if err != nil {
			return fmt.Errorf("plan: stage %s: %w", p.Stages[i].ID, err)
		}
		p.Stages[i].RequiredEvidence = normalized
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
// NormalizeRequiredEvidence validates and normalizes a stage's required-evidence
// declarations (RAG-SOP-003). Each item must be "corpus:<id>" or "source:<prefix>".
// An empty, malformed, or unknown-kind item is an error; the identity must be
// non-empty and free of whitespace. Duplicates are dropped, preserving first-seen
// order, so the result is deterministic. Multiple items are ANDed by the caller:
// every declared requirement must be satisfiable for the evidence to be present.
// An empty input returns a nil slice, preserving the pre-existing behavior.
func NormalizeRequiredEvidence(items []string) ([]string, error) {
	if len(items) == 0 {
		return nil, nil
	}

	out := make([]string, 0, len(items))
	seen := make(map[string]bool, len(items))

	for _, raw := range items {
		item := strings.TrimSpace(raw)
		kind, id, ok := strings.Cut(item, ":")
		if !ok {
			return nil, fmt.Errorf("required evidence %q must be \"corpus:<id>\" or \"source:<prefix>\"", raw)
		}

		kind = strings.ToLower(strings.TrimSpace(kind))
		id = strings.TrimSpace(id)

		if kind != "corpus" && kind != "source" {
			return nil, fmt.Errorf("required evidence %q has an unknown kind %q (want corpus or source)", raw, kind)
		}
		if id == "" {
			return nil, fmt.Errorf("required evidence %q has an empty identity", raw)
		}
		if strings.ContainsAny(id, " \t\n") {
			return nil, fmt.Errorf("required evidence %q must not contain whitespace in its identity", raw)
		}

		norm := kind + ":" + id
		if seen[norm] {
			continue
		}
		seen[norm] = true
		out = append(out, norm)
	}

	return out, nil
}

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
