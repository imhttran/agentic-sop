// Command sop-controller is a thin wrapper over the shared controller service.
// The unified `sop controller serve` command calls the same implementation, so the
// two stay behaviorally identical.
package main

import (
	"fmt"
	"os"

	"github.com/imhttran/agentic-sop/internal/controller"
)

func main() {
	if err := controller.Serve(os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "controller: %v\n", err)
		os.Exit(1)
	}
}
