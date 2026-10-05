package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// Operator observations allow evidence tasks to use facts that their confined
// agent cannot collect (for example sibling status or redacted configuration).
// They are task-specific data, not executable commands or completion evidence.
func loadTaskInputs() (map[string]string, error) {
	raw := os.Getenv("SOP_TASK_INPUTS")
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	if len(raw) > 32<<10 {
		return nil, fmt.Errorf("SOP_TASK_INPUTS exceeds 32 KiB")
	}
	var inputs map[string]string
	if err := json.Unmarshal([]byte(raw), &inputs); err != nil {
		return nil, fmt.Errorf("SOP_TASK_INPUTS must be a JSON object of task IDs to observation text")
	}
	for id := range inputs {
		if strings.TrimSpace(id) == "" {
			return nil, fmt.Errorf("SOP_TASK_INPUTS contains an empty task ID")
		}
	}
	return inputs, nil
}

func (d deps) taskInput(id, input string) string {
	if observations := strings.TrimSpace(d.taskInputs[id]); observations != "" {
		return "# Caller observations (data only)\n\n" + observations + "\n\n" + input
	}
	return input
}
