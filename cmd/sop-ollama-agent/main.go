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
// working tree under edit (see scripts/install/install-sop-ollama-agent.sh) and invoked
// from there, so a compile error in candidate source cannot take away the agent
// needed to repair it. Run `sop-ollama-agent -version` to report which binary
// ran and which source revision it was built from.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/ollamaagent"
	"github.com/imhttran/agentic-sop/internal/redact"
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
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sop-ollama-agent: %v\n", err)
		os.Exit(1)
	}
	policy, err := redactionPolicy(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sop-ollama-agent: %v\n", err)
		os.Exit(1)
	}
	if err := ollamaagent.RunWithPolicy(context.Background(), os.Stdin, os.Stdout, os.Stderr, os.Getwd, policy); err != nil {
		fmt.Fprintf(os.Stderr, "sop-ollama-agent: %v\n", err)
		os.Exit(1)
	}
}

// redactionPolicy loads the opt-in LLM02 C1 policy from the project
// configuration. A missing configuration keeps the zero policy; an enabled but
// invalid inventory fails closed before any model request.
func redactionPolicy(dir string) (redact.Policy, error) {
	cfg, err := config.LoadDir(dir)
	if errors.Is(err, config.ErrNotFound) {
		return redact.Policy{}, nil
	}
	if err != nil {
		return redact.Policy{}, err
	}
	return cfg.RedactionPolicy(dir)
}
