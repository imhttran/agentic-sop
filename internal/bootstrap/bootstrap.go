// Package bootstrap wires an environment (bootstrap) stage into the task graph.
// When a plan marks a stage as the development environment, every other task
// gets an implicit dependency on it, so feature work stays blocked until the
// environment is done. It is deterministic and never calls an agent.
package bootstrap

import (
	"errors"
	"fmt"

	"github.com/imhttran/agentic-sdlc/internal/domain"
	"github.com/imhttran/agentic-sdlc/internal/planner"
)

// Apply adds an implicit dependency on the environment (bootstrap) task to every
// other task. A plan without an environment stage is left unchanged.
func Apply(plan *planner.Plan, tasks []*domain.Task) error {
	if plan == nil {
		return errors.New("plan is nil")
	}

	envID, err := environmentStageID(plan)
	if err != nil {
		return err
	}
	if envID == "" {
		return nil
	}

	if !hasTask(tasks, envID) {
		return fmt.Errorf("bootstrap: environment task %s is not present", envID)
	}

	for _, task := range tasks {
		if task.ID == envID || hasDependency(task, envID) {
			continue
		}
		task.DependencyIDs = append(task.DependencyIDs, envID)
	}
	return nil
}

// environmentStageID returns the id of the single environment stage, or "" when
// the plan has none. More than one environment stage is an error.
func environmentStageID(plan *planner.Plan) (string, error) {
	id := ""
	for _, stage := range plan.Stages {
		if stage.Kind != planner.KindEnvironment {
			continue
		}
		if id != "" {
			return "", fmt.Errorf("bootstrap: plan has multiple environment stages (%s, %s)", id, stage.ID)
		}
		id = stage.ID
	}
	return id, nil
}

func hasTask(tasks []*domain.Task, id string) bool {
	for _, task := range tasks {
		if task.ID == id {
			return true
		}
	}
	return false
}

func hasDependency(task *domain.Task, id string) bool {
	for _, dep := range task.DependencyIDs {
		if dep == id {
			return true
		}
	}
	return false
}
