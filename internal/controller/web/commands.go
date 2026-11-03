package web

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/imhttran/agentic-sop/internal/controller/sopclient"
)

// CommandState is the latest result of a SOP command triggered from the UI.
type CommandState struct {
	Verb   string
	State  string // running | done | error
	Output string
	Error  string
	// partial is the in-flight line (no newline yet). It is kept raw and redacted
	// only when rendered, so a credential split across two writes is still
	// recognisable; Status folds it into the Output a reader sees.
	partial string
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
		st.Output = boundOutput(sopclient.SanitizeDiagnostic(out), maxOutput)
		// The command has finished: its whole output is available, so it is redacted
		// as a unit (before bounding) and no in-flight line remains.
		st.partial = ""
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

// Status returns the last known status for a project/verb, if any. The in-flight
// line is redacted here rather than as it arrives, so a credential that spans two
// writes is still whole when the redactor sees it.
func (cr *CommandRunner) Status(project, verb string) (CommandState, bool) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	st, ok := cr.states[cr.key(project, verb)]
	if !ok {
		return CommandState{}, false
	}
	out := *st
	if st.partial != "" {
		out.Output = boundOutput(st.Output+sopclient.SanitizeDiagnostic(st.partial), maxOutput)
	}
	return out, true
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
	// skipRest drops the remainder of an over-long line once its head has been
	// retained, so a stream with no line breaks cannot grow the state without bound.
	skipRest bool
}

func (w *liveWriter) Write(p []byte) (int, error) {
	w.cr.mu.Lock()
	defer w.cr.mu.Unlock()
	// Redact complete lines into the retained output and hold the unfinished one.
	// Redacting a partial line would replace the key with "[redacted]" before its
	// value arrived, leaving the rest of the value unrecognisable to the next write.
	data := w.st.partial + string(p)
	if w.skipRest {
		if i := strings.IndexByte(data, '\n'); i >= 0 {
			data, w.skipRest = data[i+1:], false
		} else {
			data = ""
		}
	}
	if i := strings.LastIndexByte(data, '\n'); i >= 0 {
		w.st.Output = boundOutput(w.st.Output+sopclient.SanitizeDiagnostic(data[:i+1]), maxOutput)
		data = data[i+1:]
	}
	if len(data) > maxOutput {
		// A line this long never ends in practice. Keep its head, where a credential's
		// key sits, and drop the rest of the line rather than retaining it unbounded.
		data = data[:maxOutput]
		w.skipRest = true
	}
	w.st.partial = data
	return len(p), nil
}

// blockedError marks a delegated start/run that SOP reports parked at a human
// approval gate. The runner maps it to the "blocked" state rather than a failure.
type blockedError struct{ msg string }

func (e *blockedError) Error() string { return e.msg }
