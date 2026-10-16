package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/planflow"
	"github.com/imhttran/agentic-sop/internal/store"
)

// notInitializedMsg is shown by read commands when project state is missing.
const notInitializedMsg = "project is not initialized; run `sop init`"

// runStatus lists persisted tasks. It never creates state: the database must
// already exist. The concise shape is the default; `--detailed` (or a configured
// output.responses: detailed) adds each task's dependencies and attempt count.
func runStatus(args []string, stdout, stderr io.Writer, getwd func() (string, error)) int {
	detailed := false
	var rest []string
	for _, a := range args {
		if a == "--detailed" {
			detailed = true
			continue
		}
		rest = append(rest, a)
	}
	if len(rest) != 0 {
		fmt.Fprintln(stderr, "usage: sop status [--detailed]")
		return exitUsage
	}

	dir, ok := projectDir(getwd, stderr)
	if !ok {
		return exitError
	}

	path := statePath(dir)
	if !requireState(path, stderr) {
		return exitError
	}

	// The response shape: the explicit flag wins; otherwise the configured default
	// (concise unless output.responses: detailed). A missing or unreadable
	// configuration leaves the concise default, so `sop status` stays a
	// read-only-informational command.
	if !detailed {
		if cfg, err := loadConfigOrDefault(dir); err == nil {
			detailed = cfg.ResponsesDetailed()
		}
	}

	st, err := store.Open(path)
	if err != nil {
		fmt.Fprintf(stderr, "status: %v\n", err)
		return exitError
	}
	defer st.Close()

	tasks, err := st.List()
	if err != nil {
		fmt.Fprintf(stderr, "status: %v\n", err)
		return exitError
	}

	// The canonical active-plan reference, so the run state is unambiguous: at most
	// one plan is ACTIVE, and its identity comes from recorded provenance rather than
	// a scan of Markdown files.
	if id, source, ok := planflow.ActivePlanID(dir); ok {
		if id == "" {
			id = "(unnamed)"
		}
		fmt.Fprintf(stdout, "Plan: %s\n", id)
		if source != "" {
			fmt.Fprintf(stdout, "Source: %s\n", source)
		}
		fmt.Fprintln(stdout, "State: ACTIVE")
		fmt.Fprintln(stdout)
	}

	for _, task := range tasks {
		line := fmt.Sprintf("%s %s %s", task.ID, task.Status, task.Title)
		// Surface a recorded designated-approver waiver (SOP-HARD-04) beside the task,
		// so a waived required criterion is visible from `sop status`.
		if recs, err := readWaiverRecords(dir, task.ID); err == nil && len(recs) > 0 {
			ids := make([]string, 0, len(recs))
			for _, r := range recs {
				ids = append(ids, r.CriterionID)
			}
			line += fmt.Sprintf("  [waived: %s]", strings.Join(ids, ","))
		}
		fmt.Fprintln(stdout, line)
		if detailed {
			writeStatusTaskDetail(stdout, task)
		}
	}
	return exitOK
}

// writeStatusTaskDetail renders the per-task dependency and attempt detail that the
// concise status line omits. It prints nothing when a task has neither, so the
// detailed view adds no empty lines.
func writeStatusTaskDetail(w io.Writer, task *domain.Task) {
	if len(task.DependencyIDs) > 0 {
		fmt.Fprintf(w, "  depends on: %s\n", strings.Join(task.DependencyIDs, ", "))
	}
	if n := len(task.Attempts); n > 0 {
		fmt.Fprintf(w, "  attempts: %d\n", n)
	}
}

// requireState reports whether the state database exists, printing a helpful
// message when it does not. It distinguishes a missing database (project not
// initialized) from an unexpected filesystem error, and returns false when the
// caller should exit with a runtime failure.
func requireState(path string, stderr io.Writer) bool {
	found, err := exists(path)
	switch {
	case err != nil:
		fmt.Fprintf(stderr, "cannot access state database: %v\n", err)
		return false
	case !found:
		fmt.Fprintln(stderr, notInitializedMsg)
		return false
	default:
		return true
	}
}
