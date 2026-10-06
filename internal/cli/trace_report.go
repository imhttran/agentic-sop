package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/imhttran/agentic-sop/internal/runtrace"
)

// writeTraceReport renders a concise summary of a run structured trace, read
// from the run trace.json. It renders nothing when the artifact is absent (an
// older run, or a step that does not trace), so `sop report` stays compatible.
// It reports only metrics that are actually recorded, never invented values.
func writeTraceReport(w io.Writer, runDir string) {
	data, err := os.ReadFile(filepath.Join(runDir, runtrace.FileName))
	if err != nil {
		return
	}
	var t runtrace.Trace
	if err := json.Unmarshal(data, &t); err != nil {
		return
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Trace:")
	fmt.Fprintf(w, "  %-22s %d\n", "Schema:", t.SchemaVersion)

	if ex := t.Execution; ex.ModelClass != "" || ex.Provider != "" || ex.Model != "" {
		if ex.ModelClass != "" {
			fmt.Fprintf(w, "  %-22s %s\n", "Model class:", ex.ModelClass)
		}
		if ex.Provider != "" || ex.Model != "" {
			target := ex.Provider
			if ex.Model != "" {
				target += "/" + ex.Model
			}
			locality := ex.Locality
			if locality == "" {
				locality = "unknown"
			}
			source := ex.ExecutionSource
			if source == "" {
				source = "primary"
			}
			fmt.Fprintf(w, "  %-22s %s (%s, %s)\n", "Executed:", target, locality, source)
		}
		if ex.RoutingSource != "" {
			fmt.Fprintf(w, "  %-22s %s\n", "Routing source:", ex.RoutingSource)
		}
	}

	fmt.Fprintf(w, "  %-22s %d\n", "Iterations:", len(t.Iterations))
	if n := toolActions(t.Iterations); n > 0 {
		fmt.Fprintf(w, "  %-22s %d\n", "Tool actions:", n)
	}
	fmt.Fprintf(w, "  %-22s %d\n", "Repository mutations:", t.RepositoryMutations)
	fmt.Fprintf(w, "  %-22s %d\n", "Changed files:", len(t.ChangedFiles))
	fmt.Fprintf(w, "  %-22s %s\n", "Verification:", verificationSummary(t.Verification))
	fmt.Fprintf(w, "  %-22s %s\n", "Termination:", terminationSummary(t.Termination))
}

// toolActions counts the iterations whose phase is a tool phase (discovery,
// change, or validation). It is a derived summary of recorded iterations, never
// a stored value, so it cannot overstate what happened.
func toolActions(its []runtrace.Iteration) int {
	n := 0
	for _, it := range its {
		switch it.Phase {
		case "DISCOVER", "CHANGE", "VALIDATE":
			n++
		}
	}
	return n
}

// verificationSummary reports PASS only when every recorded check passed, FAIL
// when any failed, and "none" when no check was recorded.
func verificationSummary(vs []runtrace.Verification) string {
	if len(vs) == 0 {
		return "none"
	}
	for _, v := range vs {
		if !strings.EqualFold(v.Status, "PASS") {
			return "FAIL"
		}
	}
	return "PASS"
}

// terminationSummary renders the run stage plus the failure taxonomy when one
// applies, and whether a human was required.
func terminationSummary(t runtrace.Termination) string {
	s := t.Stage
	if s == "" {
		s = "unknown"
	}
	if t.Disposition != "" {
		s += " " + t.Disposition
		if t.Kind != "" {
			s += " (" + t.Kind + ")"
		}
	}
	if t.HumanRequired {
		s += " - human required"
	}
	return s
}
