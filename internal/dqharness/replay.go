package dqharness

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"

	"github.com/imhttran/agentic-sop/internal/decisionquality"
)

// Isolation describes the available replay isolation mechanism.
type Isolation struct {
	Tool      string
	Available bool
	Reason    string
}

// DetectIsolation reports whether this host can run a replay under OS-level isolation.
// It never runs anything; it only reports.
func DetectIsolation() Isolation {
	switch runtime.GOOS {
	case "darwin":
		if _, err := exec.LookPath("sandbox-exec"); err == nil {
			return Isolation{Tool: "sandbox-exec", Available: true}
		}
		return Isolation{Tool: "sandbox-exec", Reason: "sandbox-exec not found"}
	case "linux":
		if _, err := exec.LookPath("bwrap"); err == nil {
			return Isolation{Tool: "bwrap", Available: true}
		}
		return Isolation{Tool: "bwrap", Reason: "bwrap not found"}
	default:
		return Isolation{Tool: "", Reason: "no supported OS-level isolation on " + runtime.GOOS}
	}
}

// BlockedSandbox is the explicit "replay unavailable" sandbox. It never reports success,
// so an executable case is recorded as not evaluable instead of silently scoring.
type BlockedSandbox struct{ Reason string }

// Replay always fails with the recorded reason.
func (b BlockedSandbox) Replay(_ context.Context, _ decisionquality.Case, _ string) (bool, error) {
	return false, errors.New(b.Reason)
}

// ReplayBlocker explains why the frozen corpus cannot be replayed. The 12 executable
// cases declare `executable: true` but carry no repository fixture, task file, or
// verification command, and the corpus is frozen: inventing fixtures here would change
// the experiment after the fact.
func ReplayBlocker(c decisionquality.Corpus) string {
	executable := 0
	for _, cs := range c.Cases {
		if cs.Executable {
			executable++
		}
	}
	if executable == 0 {
		return ""
	}
	iso := DetectIsolation()
	if !iso.Available {
		return fmt.Sprintf("%d executable cases require replay, but no isolation is available: %s", executable, iso.Reason)
	}
	return fmt.Sprintf("%d executable cases require replay, but the frozen corpus declares no repository fixture or verification command for them; replay cannot be implemented without changing the frozen experiment", executable)
}
