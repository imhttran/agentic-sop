// Package taskbuilder turns a validated planner.Plan into executable
// domain.Tasks, validates the resulting dependency graph, and coordinates
// atomic persistence. It is deterministic and never calls an agent.
package taskbuilder

import (
	"errors"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/bootstrap"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/planner"
)

// SerializeAcceptanceCriteria renders stage criteria into the single string the
// domain Task stores. It is the one owner of that representation: criteria are
// newline-delimited.
func SerializeAcceptanceCriteria(criteria []string) string {
	return strings.Join(criteria, "\n")
}

// Build converts each Plan stage into a domain.Task, preserving IDs, title,
// objective, acceptance criteria, and dependency IDs. New tasks start in
// PLANNED with retry defaults applied. Task order follows Plan stage order.
func Build(plan *planner.Plan) ([]*domain.Task, error) {
	if plan == nil {
		return nil, errors.New("plan is nil")
	}
	if err := plan.Validate(); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	tasks := make([]*domain.Task, 0, len(plan.Stages))
	for _, stage := range plan.Stages {
		tasks = append(tasks, &domain.Task{
			ID:                 stage.ID,
			Title:              stage.Title,
			Objective:          stage.Objective,
			AcceptanceCriteria: SerializeAcceptanceCriteria(stage.AcceptanceCriteria),
			ExecutionMode:      stage.ExecutionMode,
			Status:             domain.PLANNED,
			BlockedReason:      domain.NO_REASON,
			Attempt:            0,
			MaxAttempts:        domain.DefaultRetryPolicy().MaxAttempts,
			DependencyIDs:      append([]string{}, stage.Dependencies...),
			Attempts:           []domain.Attempt{},
			CreatedAt:          now,
			UpdatedAt:          now,
		})
	}
	return tasks, nil
}

// CreateTasksFromPlan is the application operation: build tasks from a plan,
// wire in any environment bootstrap dependency, validate the dependency graph,
// then persist them atomically. It coordinates components and owns no graph
// algorithms itself.
func CreateTasksFromPlan(plan *planner.Plan, save func([]*domain.Task) error) ([]*domain.Task, error) {
	tasks, err := Build(plan)
	if err != nil {
		return nil, err
	}
	if err := bootstrap.Apply(plan, tasks); err != nil {
		return nil, err
	}
	if err := ValidateDAG(tasks); err != nil {
		return nil, err
	}
	if err := save(tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}
