package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/imhttran/agentic-sop/internal/hipcompare"
)

// runCompare implements the opt-in `sop compare` subcommand. It is only reachable
// by an explicit invocation: it is never called from `sop run` or any default
// execution path, and it adds no orchestrator, service, database, or external
// dependency. It delegates to internal/hipcompare for the whole comparison.
func runCompare(args []string, stdout, stderr io.Writer) int {
	if helpRequested(args) {
		writeCompareHelp(stdout)
		return exitOK
	}
	code := hipcompare.Command(context.Background(), args, stdout)
	if code != 0 {
		// Command already wrote a diagnostic to stdout; keep stderr empty so a
		// caller sees one stream of diagnostics.
		_ = stderr
	}
	return code
}

func writeCompareHelp(w io.Writer) {
	fmt.Fprint(w, `sop compare — the opt-in HIP-003 comparison harness

Usage:
  sop compare --manifest MANIFEST.json --baseline BASE.json --candidate CAND.json
              [--out DIR] [--commit SHA] [--executor NAME] [--model NAME]
              [--executor-command CMD] [--setting K=V] [--allow-tool NAME]...

Fixed limits (not configurable): 3 pairs, 1 held-out pair, 8 executions maximum,
per-run time and token limits, no automatic retries. Both arms run from the same
starting commit in separate disposable worktrees and share the executor, model,
settings, tool permissions, and verification commands. The candidate may edit
only the manifest's allowed_candidate_paths; any other edit aborts the comparison.
A completed comparison resumes only when every fingerprint component matches.
This command is off the default SOP execution path.
`)
}
