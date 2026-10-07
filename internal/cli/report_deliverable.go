package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/imhttran/agentic-sop/internal/config"
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

// validatePromptDeliverables validates operator-declared deliverables for
// `sop prompt --capability implement` and returns them in declaration order. A
// declaration MUST name an explicit repository FILE inside the project: an empty
// value, an absolute or escaping path, a non-clean path (traversal or a directory
// reference), a directory-wide declaration, or a path inside SOP's own state tree
// is refused. This keeps the declaration authority with the operator while
// refusing a whole-directory exemption or SOP state, so a declared report is the
// only thing a declaration can make task-owned. The returned paths are projected
// verbatim into the lifecycle's taskfile.Spec.Deliverables.
func validatePromptDeliverables(paths []string) ([]string, error) {
	out := make([]string, 0, len(paths))
	for _, raw := range paths {
		path := strings.TrimSpace(raw)
		if path == "" {
			return nil, fmt.Errorf("deliverable path is empty")
		}
		// safeRepoPath rejects absolute and escaping paths; the clean comparison
		// rejects an uncleaned path (a `..` segment or a trailing slash that would
		// otherwise name a directory).
		if !safeRepoPath(path) || filepath.ToSlash(filepath.Clean(path)) != path {
			return nil, fmt.Errorf("deliverable %q must be a clean, repository-relative path inside the project", raw)
		}
		if path == config.DirName || strings.HasPrefix(path, config.DirName+"/") {
			return nil, fmt.Errorf("deliverable %q is SOP state; declare a repository file, not SOP's own state", raw)
		}
		// A directory-wide declaration (for example docs/reports) names no file and
		// would exempt a whole tree; require an explicit file.
		if filepath.Ext(path) == "" {
			return nil, fmt.Errorf("deliverable %q is a directory-wide declaration; declare an explicit file", raw)
		}
		out = append(out, path)
	}
	return out, nil
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
