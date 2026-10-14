package cli

import (
	"io"

	adaptercli "github.com/imhttran/agentic-sop/internal/decisionadapter/cli"
)

// runDecision implements `sop decision <subcommand>`. Bare `sop decision` prints
// the decision command help; it never initializes SOP lifecycle state, so a help
// request or a standalone decision cannot create or mutate .agent-sdlc. The
// subcommand implementation is shared with the legacy sop-decision-adapter
// executable, so flags, exit codes and JSON-only stdout stay identical.
func runDecision(args []string, stdout, stderr io.Writer, _ deps) int {
	if len(args) == 0 {
		adaptercli.Run([]string{"help"}, stdout, stderr)
		return exitOK
	}
	return adaptercli.Run(args, stdout, stderr)
}
