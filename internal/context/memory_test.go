package context

import (
	"strings"
	"testing"
)

// TestFromInputsIncludesMemory proves memory items are represented, rendered, and given
// the lowest priority (they never outrank the task or current evidence).
func TestFromInputsIncludesMemory(t *testing.T) {
	c := FromInputs(Inputs{
		TaskID: "S001",
		Task:   "implement the widget",
		Memory: []Item{{
			Source:   SourceMemory,
			Identity: "d1",
			Reason:   "durable decision applicable to this repository",
			Priority: PriorityGeneric,
			Text:     "Postgres is canonical storage - single source of truth",
		}},
	}, DefaultLimits())

	if c.Empty() {
		t.Fatal("context is empty")
	}
	found := false
	for _, s := range c.Sources() {
		if s.Source == SourceMemory {
			found = true
		}
	}
	if !found {
		t.Errorf("the memory source is not represented: %v", c.Sources())
	}

	render := c.Render()
	if !strings.Contains(render, "Postgres is canonical storage") {
		t.Errorf("memory text not rendered:\n%s", render)
	}
	if strings.Index(render, "implement the widget") > strings.Index(render, "Postgres is canonical storage") {
		t.Errorf("memory must sort after the task (lowest priority):\n%s", render)
	}
}
