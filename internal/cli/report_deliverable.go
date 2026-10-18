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
	stage, ok := planTaskStage(dir, task)
	if !ok {
		return nil
	}
	return stage.Deliverables
}

// planTaskAcceptanceChecks recovers the plan-declared acceptance checks for a
// scheduled task's reconciled stage, using the same identity match as
// planTaskDeliverables. A missing/invalid plan, or a stage whose ID, Objective, or
// AcceptanceCriteria no longer matches the persisted task, yields nil — the task
// then behaves exactly as one that declared no checks. The recovered checks are
// projected verbatim into the lifecycle's taskfile.Spec.AcceptanceChecks and are
// evaluated by the existing deriveOutcome/authorizeAcceptanceCommand/
// runAcceptanceArgv seam; no new execution or authorization path is introduced.
func planTaskAcceptanceChecks(dir string, task *domain.Task) []string {
	stage, ok := planTaskStage(dir, task)
	if !ok {
		return nil
	}
	return stage.AcceptanceChecks
}

// planTaskAcceptanceMapping recovers the plan-declared acceptance mapping (SOP-003)
// through the same stage reconciliation, rendered as task-file mapping lines. A
// missing/mismatched plan yields nil: a legacy task with no mapping.
func planTaskAcceptanceMapping(dir string, task *domain.Task) []string {
	stage, ok := planTaskStage(dir, task)
	if !ok {
		return nil
	}
	var out []string
	for _, m := range stage.AcceptanceMapping {
		out = append(out, m.String())
	}
	return out
}

// planTaskStage reads the persisted plan and returns the stage that reconciles
// with the task by identity (ID + Objective + AcceptanceCriteria). It reports
// ok=false when the plan is absent, invalid, or no stage matches — the single
// reconciliation rule shared by the deliverable and acceptance-check
// projections so they can never disagree about which stage owns a task.
func planTaskStage(dir string, task *domain.Task) (planner.Stage, bool) {
	data, err := os.ReadFile(filepath.Join(dir, stateDirName, "plan.json"))
	if err != nil {
		return planner.Stage{}, false
	}
	var plan planner.Plan
	if json.Unmarshal(data, &plan) != nil || plan.Validate() != nil {
		return planner.Stage{}, false
	}
	for _, stage := range plan.Stages {
		if stage.ID == task.ID && stage.Objective == task.Objective &&
			strings.Join(stage.AcceptanceCriteria, "\n") == task.AcceptanceCriteria {
			return stage, true
		}
	}
	return planner.Stage{}, false
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
		if path, ok := reportDeliverablePath(item); ok {
			paths = append(paths, path)
		}
	}
	return paths
}

// reportDeliverablePath returns the declared report deliverable an item names,
// and whether it names one. It is the SINGLE definition of the report-deliverable
// rule, shared by the verified gate (taskReportDeliverables) and the task-text
// display classification, so the two can never disagree.
func reportDeliverablePath(item string) (string, bool) {
	{
		fields := strings.Fields(item)
		if len(fields) == 0 {
			return "", false
		}
		path := strings.Trim(fields[0], "`")
		// An exact Markdown path under docs/reports/ that the task explicitly
		// declares is a task deliverable, whether in a subdirectory or at the top
		// level (for example docs/reports/PERFORMANCE-BASELINE.md). SOP's own
		// generated docs/reports/<plan-id>.md is only task-owned if a task declares it.
		if !safeRepoPath(path) || filepath.ToSlash(filepath.Clean(path)) != path ||
			!strings.HasPrefix(path, planflow.ReportsDir+"/") ||
			filepath.Ext(path) != ".md" {
			return "", false
		}
		return path, true
	}
}

// deliverableGroups classifies the declared deliverables of a task for the
// generated task text, reusing the existing runtime classification:
// reportDeliverablePath for the verified report files, and deliverablePaths for
// path-like tokens. A descriptive deliverable is never presented as an artifact
// the agent must create.
func deliverableGroups(spec *taskfile.Spec) []taskfile.DeliverableGroup {
	var files, paths, outcomes []string
	for _, item := range spec.Deliverables {
		if _, ok := reportDeliverablePath(item); ok {
			files = append(files, item)
			continue
		}
		if len(deliverablePaths(item)) > 0 {
			paths = append(paths, item)
			continue
		}
		outcomes = append(outcomes, item)
	}
	var groups []taskfile.DeliverableGroup
	if len(files) > 0 {
		groups = append(groups, taskfile.DeliverableGroup{
			Heading: "Required deliverable files",
			Note:    "Create each of these repository files before returning your outcome.",
			Items:   files,
		})
	}
	if len(paths) > 0 {
		groups = append(groups, taskfile.DeliverableGroup{
			Heading: "Declared repository paths",
			Note:    "These name repository paths. They are not report deliverables, but SOP does verify through the outcome criteria that each path exists at completion.",
			Items:   paths,
		})
	}
	if len(outcomes) > 0 {
		groups = append(groups, taskfile.DeliverableGroup{
			Heading: "Expected outcomes (descriptive)",
			Note:    "Report these in your summary. They do not require a new file unless an acceptance criterion or a required deliverable file names one.",
			Items:   outcomes,
		})
	}
	return groups
}
