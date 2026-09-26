package handoff

import "github.com/imhttran/agentic-sop/internal/domain"

// Select returns the handoff records for a task's direct dependencies, in
// dependency order. It deliberately returns nothing else, so a new task never
// receives the full history of prior work.
func Select(task *domain.Task, available []Record) []Record {
	if task == nil {
		return nil
	}
	byTask := make(map[string]Record, len(available))
	for _, record := range available {
		byTask[record.TaskID] = record
	}

	var selected []Record
	for _, dep := range task.DependencyIDs {
		if record, ok := byTask[dep]; ok {
			selected = append(selected, record)
		}
	}
	return selected
}
