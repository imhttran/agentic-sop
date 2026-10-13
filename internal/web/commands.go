package web

import (
	"context"
	"errors"
	"sync"
	"time"

	"sop-controller/internal/sopclient"
)

// CommandState is the latest result of a SOP command triggered from the UI.
type CommandState struct {
	Verb    string
	State   string // running | done | error
	Output  string
	Error   string
	Started time.Time
	// Conflict is set only for a typed DecisionRejection or ReconcileRejection:
	// SOP refused, so the fragment shows an actionable conflict, not an error.
	Conflict bool
	// TaskID/Stage/Attempt come only from a live sopclient read.
	TaskID  string
	Stage   string
	Attempt int
}

// Running reports whether the command is still in flight.
func (c CommandState) Running() bool { return c.State == "running" }

// CommandRunner runs SOP commands in the background and remembers the latest
// status per project/verb, so the UI never blocks on a long SOP lifecycle.
type CommandRunner struct {
	mu      sync.Mutex
	states  map[string]*CommandState
	timeout time.Duration
}

// NewCommandRunner returns an empty runner with the specified timeout.
func NewCommandRunner(timeout time.Duration) *CommandRunner {
	if timeout <= 0 {
		timeout = 15 * time.Minute
	}
	return &CommandRunner{states: map[string]*CommandState{}, timeout: timeout}
}

func (cr *CommandRunner) key(project, verb string) string { return project + "/" + verb }

// Start launches fn in the background and records its status.
// Returns (started=true, err) if command started, or (false, err) if already running.
func (cr *CommandRunner) Start(project, verb string, fn func(ctx context.Context) (string, error)) (bool, error) {
	cr.mu.Lock()
	k := cr.key(project, verb)
	if st, ok := cr.states[k]; ok && st.State == "running" {
		cr.mu.Unlock()
		return false, nil
	}
	st := &CommandState{Verb: verb, State: "running", Started: time.Now()}
	cr.states[k] = st
	cr.mu.Unlock()

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), cr.timeout)
		defer cancel()
		// Live output (DP-010): the command tees its output through this sink as it
		// runs, so the status fragment shows progress before it finishes. The sink
		// shares cr.mu with Status, so a reader never sees a torn string.
		ctx = sopclient.WithProgress(ctx, &liveWriter{cr: cr, st: st})
		out, err := fn(ctx)

		cr.mu.Lock()
		defer cr.mu.Unlock()
		st.Output = boundOutput(out, maxOutput)
		if err != nil {
			// A non-zero exit that SOP reports as an open human gate is a boundary, not
			// a failure: surface it as "blocked". This reads SOP state; it invents no
			// transition.
			var be *blockedError
			if errors.As(err, &be) {
				st.State = "blocked"
				st.Error = be.Error()
				return
			}
			st.State = "error"
			st.Error = err.Error()
			// SOP's own refusal is a conflict; the controller's pre-SOP precondition errors
			// stay plain errors.
			var drej *sopclient.DecisionRejection
			var rrej *sopclient.ReconcileRejection
			st.Conflict = errors.As(err, &drej) || errors.As(err, &rrej)
			return
		}
		st.State = "done"
	}()
	return true, nil
}

// Status returns the last known status for a project/verb, if any.
func (cr *CommandRunner) Status(project, verb string) (CommandState, bool) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	st, ok := cr.states[cr.key(project, verb)]
	if !ok {
		return CommandState{}, false
	}
	return *st, true
}

// maxOutput bounds retained output so a runaway command can't bloat memory or the UI.
const maxOutput = 8000

// boundOutput caps retained output, keeping the most recent bytes (the tail) so the
// live progress shown before a command finishes is what a reader sees at the end.
func boundOutput(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "… (earlier output truncated)\n" + s[len(s)-n:]
}

// liveWriter appends a command's output to its CommandState as it runs, under the
// runner mutex, keeping the retained output bounded.
type liveWriter struct {
	cr *CommandRunner
	st *CommandState
}

func (w *liveWriter) Write(p []byte) (int, error) {
	w.cr.mu.Lock()
	defer w.cr.mu.Unlock()
	w.st.Output = boundOutput(w.st.Output+string(p), maxOutput)
	return len(p), nil
}

// blockedError marks a delegated start/run that SOP reports parked at a human
// approval gate. The runner maps it to the "blocked" state rather than a failure.
type blockedError struct{ msg string }

func (e *blockedError) Error() string { return e.msg }
