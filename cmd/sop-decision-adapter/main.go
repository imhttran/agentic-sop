// Command sop-decision-adapter is a thin wrapper over the shared decision-adapter
// CLI. The unified `sop decision` command calls the same implementation, so flags,
// exit codes and JSON-only stdout stay identical.
package main

import (
	"os"

	adaptercli "github.com/imhttran/agentic-sop/internal/decisionadapter/cli"
)

func main() {
	os.Exit(adaptercli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
