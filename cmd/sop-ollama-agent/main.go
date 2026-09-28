// Command sop-ollama-agent is the bootstrap Ollama coding-agent harness. SOP's
// command provider runs it with a JSON request on stdin and reads the response
// content from stdout, exactly like any other command agent.
//
// It talks to a local Ollama server (default http://127.0.0.1:11434) and the
// model in SOP_OLLAMA_MODEL (default deepseek-v4.1-flash:cloud), and gives the
// model a small set of controlled repository tools. It is an implementation
// adapter only: SOP remains the workflow authority.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/imhttran/agentic-sop/internal/ollamaagent"
)

func main() {
	if err := ollamaagent.Run(context.Background(), os.Stdin, os.Stdout, os.Stderr, os.Getwd); err != nil {
		fmt.Fprintf(os.Stderr, "sop-ollama-agent: %v\n", err)
		os.Exit(1)
	}
}
