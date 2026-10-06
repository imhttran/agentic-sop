package orchestration

import (
	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/domain"
)

// PlanMode derives the orchestration mode for a plan of domain tasks in a
// read-only fashion. It is the package's public decision API.
//
// PLACEHOLDER / UNSUPPORTED CONSUMER BOUNDARY
//
// The internal/scheduler and internal/run runnable-task selection contracts are
// UNCONFIRMED. This function therefore does NOT integrate with the scheduler and
// makes no assumption about how the scheduler selects work: it only translates
// already-loaded domain tasks into the pure decision input and returns the
// mode. Real scheduler wiring is deferred to a future task once that contract is
// confirmed. Callers that do integrate must treat the returned mode as
// advisory metadata and must not let it mutate lifecycle state.
//
// PlanMode never mutates a domain.Task. It reads metadata (ID, DependencyIDs)
// and the caller-supplied capability map only.
func PlanMode(tasks []*domain.Task, required map[string]agent.Capability, available agent.Capabilities) Mode {
	units := make([]Unit, 0, len(tasks))
	deps := make(map[string][]string, len(tasks))
	for _, task := range tasks {
		if task == nil {
			continue
		}
		capability, ok := required[task.ID]
		if !ok {
			// Without a requested capability the unit is ambiguous; the decision
			// function treats an empty capability as SINGLE.
			capability = ""
		}
		units = append(units, Unit{ID: task.ID, Capability: capability})
		if len(task.DependencyIDs) > 0 {
			depCopy := make([]string, len(task.DependencyIDs))
			copy(depCopy, task.DependencyIDs)
			deps[task.ID] = depCopy
		}
	}
	return Decide(Request{Units: units, Dependencies: deps, Available: available})
}
