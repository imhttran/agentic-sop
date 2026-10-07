package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/imhttran/agentic-sop/internal/planner"
)

// loadPlanContext reads the active machine plan (.agent-sdlc/plan.json) and returns
// the authoritative capability context for taskID: the compiled capability
// inventory, plus the capability names the task's own compiled stage requires. It is
// read-only and deterministic; a missing or unreadable plan (for example an ad-hoc
// `sop run --task`) yields an empty context, so the task-level planner behaves
// exactly as before.
//
// The compiled inventory is authoritative (SOP-PLANNER-CAP-001): it is what lets a
// task-level plan record "this capability already exists" instead of re-deriving it
// from task prose, so a repository discovery target named in the task cannot be
// promoted to an external prerequisite capability.
func loadPlanContext(dir, taskID string) planner.PlanContext {
	data, err := os.ReadFile(filepath.Join(dir, stateDirName, "plan.json"))
	if err != nil {
		return planner.PlanContext{}
	}
	var plan planner.Plan
	if err := json.Unmarshal(data, &plan); err != nil {
		return planner.PlanContext{}
	}
	ctx := planner.PlanContext{Capabilities: plan.Capabilities}
	id := strings.TrimSpace(taskID)
	for _, stage := range plan.Stages {
		if stage.ID == id {
			ctx.Requires = append([]string{}, stage.Requires...)
			break
		}
	}
	return ctx
}
