package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/imhttran/agentic-sop/internal/runmetrics"
)

// runReportDecisions renders SOP's decision telemetry for the project: the
// deterministic aggregate internal/runmetrics computes from the recorded run
// artifacts under .agent-sdlc/runs/.
//
// It is read-only and opt-in (`sop report --decisions`). It makes no model call,
// performs no evaluation, retry, approval, or execution, opens no network
// connection, and never mutates run state: it only reads existing artifacts and
// prints them. With asJSON it emits the deterministic aggregate JSON; otherwise a
// deterministic text summary. A project with no runs (or no recorded decisions)
// renders an empty-but-explicit aggregate rather than failing.
//
// This is the single approved consumer of internal/runmetrics outside that package;
// the unwired guard (internal/runmetrics/unwired_test.go) allows only this file, so
// no production execution path can reach the aggregator.
func runReportDecisions(dir string, asJSON bool, stdout, stderr io.Writer) int {
	agg, err := runmetrics.AggregateProject(dir)
	if err != nil {
		fmt.Fprintf(stderr, "report: %v\n", err)
		return exitError
	}
	if asJSON {
		data, err := json.MarshalIndent(agg, "", "  ")
		if err != nil {
			fmt.Fprintf(stderr, "report: encode decisions: %v\n", err)
			return exitError
		}
		fmt.Fprintln(stdout, string(data))
		return exitOK
	}
	fmt.Fprint(stdout, runmetrics.RenderDecisions(agg))
	return exitOK
}
