package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/failure"
	"github.com/imhttran/agentic-sop/internal/model"
	"github.com/imhttran/agentic-sop/internal/perf"
	"github.com/imhttran/agentic-sop/internal/review"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
)

// runReport prints a concise, CI-friendly summary of a completed run from its
// persisted report.json. With no argument it reports the most recent run. This
// is the informational PR output: build/test results, review findings by
// severity, and the final gate.
func runReport(args []string, stdout, stderr io.Writer, getwd func() (string, error)) int {
	if len(args) > 1 {
		fmt.Fprintln(stderr, "usage: sop report [run-id]")
		return exitUsage
	}

	dir, ok := projectDir(getwd, stderr)
	if !ok {
		return exitError
	}

	runsRoot := filepath.Join(dir, stateDirName, "runs")
	id := ""
	if len(args) == 1 {
		id = args[0]
	} else {
		latest, err := latestRun(runsRoot)
		if err != nil {
			fmt.Fprintf(stderr, "report: %v\n", err)
			return exitError
		}
		id = latest
	}
	if id == "" {
		fmt.Fprintln(stderr, "report: no runs found")
		return exitError
	}

	data, err := os.ReadFile(filepath.Join(runsRoot, id, "report.json"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(stderr, "report: no report for run %s\n", id)
			return exitError
		}
		fmt.Fprintf(stderr, "report: %v\n", err)
		return exitError
	}

	var doc runReportDoc
	if err := json.Unmarshal(data, &doc); err != nil {
		fmt.Fprintf(stderr, "report: parse %s/report.json: %v\n", id, err)
		return exitError
	}

	writeReport(stdout, doc)
	// Early JEV checkpoint evidence (TRIAGE / PRE_EXECUTION) is rendered from the
	// run's persisted artifact. It renders nothing when no early checkpoint ran, so
	// a disabled JEV adds no noise. It is human-readable only.
	writeEarlyReport(stdout, filepath.Join(runsRoot, id))
	writeApprovalReport(stdout, dir, id)
	writePerformance(stdout, dir, doc)
	return exitOK
}

// writeApprovalReport renders the task's approval boundary (SOP's explicit
// request and any decision) by reading the same run artifact the approval
// application boundary owns, so `sop report` shows the human gate and its
// outcome. It renders nothing when SOP recorded no request, and never derives a
// gate from the report's own status.
func writeApprovalReport(w io.Writer, dir, taskID string) {
	req, ok := runpkg.At(runpkg.Dir(dir, taskID)).Approval()
	if !ok {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Approval:")
	fmt.Fprintf(w, "  %-12s %s\n", "Status:", req.Status)
	fmt.Fprintf(w, "  %-12s %s\n", "Kind:", req.Kind)
	if req.Reason != "" {
		fmt.Fprintf(w, "  %-12s %s\n", "Reason:", req.Reason)
	}
	if d := req.Decision; d != nil {
		fmt.Fprintf(w, "  %-12s %s\n", "Decided:", d.DecidedAt.Format(time.RFC3339))
		if d.DecidedBy != "" {
			fmt.Fprintf(w, "  %-12s %s\n", "Decided by:", d.DecidedBy)
		}
		if d.LifecycleAction != "" {
			fmt.Fprintf(w, "  %-12s %s\n", "Lifecycle:", d.LifecycleAction)
		}
	}
}

// writePerformance renders where time was spent: the plan-level run summary when
// one is available, otherwise the reported task's own record, otherwise a clear
// "unavailable" line rather than a fabricated value.
func writePerformance(w io.Writer, dir string, doc runReportDoc) {
	fmt.Fprintln(w)
	if run, ok := loadRunMetrics(dir, activePlanID(dir)); ok {
		perf.WriteRun(w, run)
		return
	}
	if doc.Performance.Measured() {
		fmt.Fprintf(w, "Performance (task %s)\n\n", doc.ID)
		perf.WriteTask(w, doc.Performance)
		return
	}
	fmt.Fprintln(w, "Performance")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "No timing was recorded for this run.")
}

// activePlanID reads the plan identity SOP is currently executing, or "" when
// none is recorded.
func activePlanID(dir string) string {
	data, err := os.ReadFile(filepath.Join(dir, stateDirName, "plan.meta.json"))
	if err != nil {
		return ""
	}
	var meta struct {
		PlanID string `json:"plan_id"`
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return ""
	}
	return strings.TrimSpace(meta.PlanID)
}

// loadRunMetrics reads the plan-level run aggregate, if any.
func loadRunMetrics(dir, planID string) (perf.Run, bool) {
	if planID == "" {
		return perf.Run{}, false
	}
	data, err := os.ReadFile(filepath.Join(dir, stateDirName, "runs", planID, "metrics.json"))
	if err != nil {
		return perf.Run{}, false
	}
	var run perf.Run
	if err := json.Unmarshal(data, &run); err != nil {
		return perf.Run{}, false
	}
	return run, true
}

// latestRun returns the run id whose report.json was written most recently.
func latestRun(runsRoot string) (string, error) {
	entries, err := os.ReadDir(runsRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}

	type candidate struct {
		id  string
		mod int64
	}
	var found []candidate
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		info, err := os.Stat(filepath.Join(runsRoot, entry.Name(), "report.json"))
		if err != nil {
			continue
		}
		found = append(found, candidate{id: entry.Name(), mod: info.ModTime().UnixNano()})
	}
	if len(found) == 0 {
		return "", nil
	}
	sort.Slice(found, func(i, j int) bool {
		if found[i].mod != found[j].mod {
			return found[i].mod > found[j].mod
		}
		return found[i].id < found[j].id
	})
	return found[0].id, nil
}

// writeReport renders the concise summary.
func writeReport(w io.Writer, doc runReportDoc) {
	fmt.Fprintf(w, "Run: %s\n", doc.ID)
	fmt.Fprintf(w, "Stage: %s\n", doc.Stage)
	fmt.Fprintf(w, "Provider: %s\n", doc.Provider)
	fmt.Fprintf(w, "Review: %s\n", doc.Engine)
	fmt.Fprintf(w, "Fix cycles: %d\n", doc.FixCycles)
	fmt.Fprintf(w, "Gate: %s\n\n", doc.Decision)
	writeModelSelectionSummary(w, doc.ModelSelection)
	writeRoutingSummary(w, doc.Routing)

	fmt.Fprintln(w, "Validation:")
	if len(doc.Validation) == 0 {
		fmt.Fprintln(w, "  (none)")
	}
	for _, r := range doc.Validation {
		fmt.Fprintf(w, "  %-6s %-12s %s\n", r.Status, r.Category, r.Command)
	}

	counts := findingsBySeverity(doc.Findings)
	fmt.Fprintf(w, "\nReview findings: %d\n", len(doc.Findings))
	for _, severity := range []review.Severity{review.Critical, review.High, review.Medium, review.Low, review.Info} {
		if counts[severity] > 0 {
			fmt.Fprintf(w, "  %-8s %d\n", severity, counts[severity])
		}
	}

	writeJEVSummary(w, doc.JEV)
	writeClassificationSummary(w, doc.Classification)
}

// writeModelSelectionSummary renders the run's resolved model-routing evidence
// (class, provider, model, locality, layer, reason) in `sop report`, so the model
// choice is auditable. It renders nothing when model routing was inactive.
func writeModelSelectionSummary(w io.Writer, sel *model.Selection) {
	if sel == nil {
		return
	}
	fmt.Fprintln(w, "Model selection:")
	fmt.Fprintf(w, "  %-9s %s\n", "Class:", sel.Class)
	fmt.Fprintf(w, "  %-9s %s\n", "Provider:", sel.Provider)
	fmt.Fprintf(w, "  %-9s %s\n", "Model:", sel.Model)
	fmt.Fprintf(w, "  %-9s %s\n", "Locality:", sel.Locality)
	fmt.Fprintf(w, "  %-9s %s\n", "Source:", sel.Source)
	if sel.Reason != "" {
		fmt.Fprintf(w, "  %-9s %s\n", "Reason:", sel.Reason)
	}
	if sel.Fallback {
		fmt.Fprintf(w, "  %-9s %s\n", "Fallback:", "true")
	}
	fmt.Fprintln(w)
}

// writeClassificationSummary renders the failure classification of a run that did
// not pass, so `sop report` shows the kind, the disposition SOP applied, and the
// reason. It renders nothing for a passing run (a nil classification).
func writeClassificationSummary(w io.Writer, cls *failure.Classification) {
	if cls == nil {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Classification:")
	fmt.Fprintf(w, "  %-12s %s\n", "Kind:", cls.Kind)
	fmt.Fprintf(w, "  %-12s %s\n", "Disposition:", cls.Disposition)
	fmt.Fprintf(w, "  %-12s %s\n", "Confidence:", cls.Confidence)
	if reason := strings.TrimSpace(cls.Reason); reason != "" {
		fmt.Fprintf(w, "  %-12s %s\n", "Reason:", reason)
	}
}

// writeJEVSummary renders the JEV section of `sop report`, reusing the single
// JEV section renderer (renderJEVSection) so the report and the run's report.md
// can never drift. It renders nothing when JEV did not run. The JEV evidence is
// diagnostic and read-only; it never restates validation or review findings as
// JEV and never feeds a decision.
func writeJEVSummary(w io.Writer, jev *jevReportDoc) {
	if jev == nil {
		return
	}
	section := renderJEVSection(&jevRunDoc{
		TaskID:     jev.TaskID,
		RunID:      jev.RunID,
		Status:     jev.Status,
		FailClosed: jev.FailClosed,
		Reason:     jev.Reason,
		Findings:   jev.Findings,
		Metrics:    jev.Metrics,
	}, jev.ArtifactPath)
	if section == "" {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprint(w, section)
}

// findingsBySeverity counts findings per severity.
func findingsBySeverity(findings []review.Finding) map[review.Severity]int {
	counts := make(map[review.Severity]int, len(findings))
	for _, f := range findings {
		counts[f.Severity]++
	}
	return counts
}
