package main

import (
	"os"

	"github.com/imhttran/agentic-sop/internal/cli"
)

func main() {
	code := cli.Run(os.Args[1:], os.Stdout, os.Stderr)
	// A detached background job records its exit and enqueues its terminal event
	// from this process, so it notifies even when the MCP server that started it is
	// already gone. Foreground invocations are unaffected.
	cli.FinishBackgroundJob(code)
	os.Exit(code)
}
