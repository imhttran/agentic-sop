package taskbuilder

import (
	"errors"
	"fmt"

	"github.com/imhttran/agentic-sdlc/internal/domain"
)

// ValidateDAG checks that the task dependency graph is well-formed: every
// dependency references a known task, no task depends on itself, and there are
// no cycles. Cycles are detected with Kahn's algorithm.
func ValidateDAG(tasks []*domain.Task) error {
	ids := make(map[string]bool, len(tasks))
	for _, task := range tasks {
		if ids[task.ID] {
			return fmt.Errorf("duplicate task id %q", task.ID)
		}
		ids[task.ID] = true
	}

	indegree := make(map[string]int, len(tasks))
	successors := make(map[string][]string, len(tasks))
	for _, task := range tasks {
		indegree[task.ID] = 0
	}

	for _, task := range tasks {
		seen := make(map[string]bool, len(task.DependencyIDs))
		for _, dep := range task.DependencyIDs {
			if dep == task.ID {
				return fmt.Errorf("task %s depends on itself", task.ID)
			}
			if !ids[dep] {
				return fmt.Errorf("task %s depends on unknown task %q", task.ID, dep)
			}
			if seen[dep] {
				continue
			}
			seen[dep] = true
			successors[dep] = append(successors[dep], task.ID)
			indegree[task.ID]++
		}
	}

	queue := make([]string, 0, len(tasks))
	for _, task := range tasks {
		if indegree[task.ID] == 0 {
			queue = append(queue, task.ID)
		}
	}

	processed := 0
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		processed++
		for _, next := range successors[id] {
			indegree[next]--
			if indegree[next] == 0 {
				queue = append(queue, next)
			}
		}
	}

	if processed != len(tasks) {
		return errors.New("plan has a dependency cycle")
	}
	return nil
}
