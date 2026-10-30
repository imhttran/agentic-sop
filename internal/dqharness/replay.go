package dqharness

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"runtime"

	"github.com/imhttran/agentic-sop/internal/decisionquality"
)

// IsolationBackend identifies the OS-level isolation mechanism that assembles replay
// command arguments.
type IsolationBackend string

const (
	// BackendNone means no isolation backend is selected; replay must refuse.
	BackendNone IsolationBackend = ""
	// BackendSeatbelt is the macOS sandbox-exec backend, driven by a Seatbelt profile.
	BackendSeatbelt IsolationBackend = "seatbelt"
	// BackendBubblewrap is the Linux bubblewrap backend, driven by bwrap arguments.
	BackendBubblewrap IsolationBackend = "bubblewrap"
)

// Isolation describes the available replay isolation mechanism.
type Isolation struct {
	Tool      string
	Backend   IsolationBackend
	Available bool
	Reason    string
}

// DetectIsolation reports whether this host can run a replay under OS-level isolation.
// It never runs anything; it only reports. It is the single selector of the backend.
func DetectIsolation() Isolation {
	return detectIsolationFor(runtime.GOOS)
}

// detectIsolationFor is the pure backend selector. It maps one GOOS value to an
// Isolation so tests can exercise darwin and unsupported hosts on any machine.
func detectIsolationFor(goos string) Isolation {
	switch goos {
	case "darwin":
		if _, err := exec.LookPath("sandbox-exec"); err == nil {
			return Isolation{Tool: "sandbox-exec", Backend: BackendSeatbelt, Available: true}
		}
		return Isolation{Tool: "sandbox-exec", Backend: BackendSeatbelt, Reason: "sandbox-exec not found"}
	case "linux":
		if _, err := exec.LookPath("bwrap"); err == nil {
			return Isolation{Tool: "bwrap", Backend: BackendBubblewrap, Available: true}
		}
		return Isolation{Tool: "bwrap", Backend: BackendBubblewrap, Reason: "bwrap not found"}
	default:
		return Isolation{Tool: "", Backend: BackendNone, Reason: "no supported OS-level isolation on " + goos}
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
