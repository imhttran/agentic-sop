// Command sop-ollama-agent is the bootstrap Ollama coding-agent harness. SOP's
// command provider runs it with a JSON request on stdin and reads the response
// content from stdout, exactly like any other command agent.
//
// It talks to a local Ollama server (default http://127.0.0.1:11434) and the
// model in SOP_OLLAMA_MODEL (default deepseek-v4.1-flash:cloud), and gives the
// model a small set of controlled repository tools. It is an implementation
// adapter only: SOP remains the workflow authority.
//
// It is normally installed once as an immutable known-good binary outside the
// working tree under edit (see scripts/install-sop-ollama-agent.sh) and invoked
// from there, so a compile error in candidate source cannot take away the agent
// needed to repair it. Run `sop-ollama-agent -version` to report which binary
// ran and which source revision it was built from.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/imhttran/agentic-sop/internal/ollamaagent"
)

// Version identifiers. They are set at build time by the install workflow with
// -ldflags "-X main.version=... -X main.sourceRevision=...", so an installed
// known-good binary can always be told apart from a working-tree build (which
// keeps the defaults below).
var (
	version        = "devel"
	sourceRevision = "unknown"
)

func main() {
	showVersion := flag.Bool("version", false, "print the binary version and source revision, then exit")
	flag.Parse()
	if *showVersion {
		fmt.Print(ollamaagent.VersionReport(version, sourceRevision))
		return
	}
	if err := ollamaagent.Run(context.Background(), os.Stdin, os.Stdout, os.Stderr, os.Getwd); err != nil {
		fmt.Fprintf(os.Stderr, "sop-ollama-agent: %v\n", err)
		os.Exit(1)
	}
}
