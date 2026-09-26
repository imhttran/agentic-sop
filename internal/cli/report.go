package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/imhttran/agentic-sop/internal/review"
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
	return exitOK
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
}

// findingsBySeverity counts findings per severity.
func findingsBySeverity(findings []review.Finding) map[review.Severity]int {
	counts := make(map[review.Severity]int, len(findings))
	for _, f := range findings {
		counts[f.Severity]++
	}
	return counts
}
