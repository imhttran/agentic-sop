package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/planflow"
	"github.com/imhttran/agentic-sop/internal/planner"
	"github.com/imhttran/agentic-sop/internal/taskfile"
)

// The persisted task omits deliverables. Recover them from its reconciled plan,
// without rewriting the task or accepting a model-generated implementation plan
// as authority. A missing/mismatched plan grants no report exception.
func planTaskDeliverables(dir string, task *domain.Task) []string {
	data, err := os.ReadFile(filepath.Join(dir, stateDirName, "plan.json"))
	if err != nil {
		return nil
	}
	var plan planner.Plan
	if json.Unmarshal(data, &plan) != nil || plan.Validate() != nil {
		return nil
	}
	for _, stage := range plan.Stages {
		if stage.ID == task.ID && stage.Objective == task.Objective &&
			strings.Join(stage.AcceptanceCriteria, "\n") == task.AcceptanceCriteria {
			return stage.Deliverables
		}
	}
	return nil
}

// Explicit report files in a subdirectory are repository deliverables, unlike
// SOP-generated docs/reports/<plan-id>.md. Never grant an exception to state,
// directories, traversal, or model-chosen paths.
func taskReportDeliverables(spec *taskfile.Spec) []string {
	var paths []string
	for _, item := range spec.Deliverables {
		fields := strings.Fields(item)
		if len(fields) == 0 {
			continue
		}
		path := strings.Trim(fields[0], "`")
		// An exact Markdown path under docs/reports/ that the task explicitly
		// declares is a task deliverable, whether in a subdirectory or at the top
		// level (for example docs/reports/PERFORMANCE-BASELINE.md). SOP's own
		// generated docs/reports/<plan-id>.md is only task-owned if a task declares it.
		if !safeRepoPath(path) || filepath.ToSlash(filepath.Clean(path)) != path ||
			!strings.HasPrefix(path, planflow.ReportsDir+"/") ||
			filepath.Ext(path) != ".md" {
			continue
		}
		paths = append(paths, path)
	}
	return paths
}
