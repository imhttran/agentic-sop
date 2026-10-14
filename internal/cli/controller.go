package cli

import (
	"fmt"
	"io"

	"github.com/imhttran/agentic-sop/internal/controller"
)

// runController implements `sop controller <subcommand>`. Bare `sop controller`
// prints help. It never initializes SOP lifecycle state, so a help request or a
// service run cannot create or mutate .agent-sdlc.
func runController(args []string, stdout, stderr io.Writer, _ deps) int {
	if len(args) == 0 {
		writeControllerHelp(stdout)
		return exitOK
	}
	switch args[0] {
	case "serve":
		// `sop controller serve --help`/`-h` must print usage and exit without side
		// effects: check for help BEFORE Serve, so it never loads runtime
		// configuration, discovers projects or opens a listener.
		if helpRequested(args[1:]) {
			writeControllerHelp(stdout)
			return exitOK
		}
		if err := controller.Serve(stdout, stderr); err != nil {
			fmt.Fprintf(stderr, "controller: %v\n", err)
			return exitError
		}
		return exitOK
	case "help", "-h", "--help":
		writeControllerHelp(stdout)
		return exitOK
	default:
		fmt.Fprintf(stderr, "unknown controller command: %s\n", args[0])
		fmt.Fprintln(stderr, "run `sop controller --help` for usage")
		return exitUsage
	}
}

// helpRequested reports whether args carry a help flag. A command checks it before
// loading runtime configuration, discovering resources or opening a listener, so a
// help request is side-effect free.
func helpRequested(args []string) bool {
	for _, a := range args {
		if a == "-h" || a == "--help" {
			return true
		}
	}
	return false
}

func writeControllerHelp(w io.Writer) {
	fmt.Fprint(w, `sop controller — the SOP Controller dashboard

Usage:
  sop controller serve    serve the local-first, read-only dashboard

The controller reads SOP's state read-only and delegates commands to the installed
sop executable (SOP_BIN overrides the default executable). It never owns lifecycle
state and never initializes .agent-sdlc.
`)
}
