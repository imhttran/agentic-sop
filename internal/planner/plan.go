// Package planner turns a project PRD into a structured Plan by asking an
// Agent and validating the result. It performs no filesystem or workflow work;
// rendering and file IO live elsewhere.
package planner

import (
	"errors"
	"fmt"
	"strings"
)

// Plan is the machine representation of an implementation plan. PLAN.md is a
// human rendering of this structure, never the other way around.
type Plan struct {
	Project string  `json:"project"`
	Summary string  `json:"summary"`
	Stages  []Stage `json:"stages"`
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

	return nil
}
