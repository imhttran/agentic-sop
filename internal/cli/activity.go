package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/imhttran/agentic-sop/internal/activity"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/runtrace"
)

// activityArtifactName is the run artifact holding the task's activity stream. It
// lives beside the run's other artifacts so a controller/API can read the same
// events the CLI rendered, without parsing CLI text. The name is owned by the run
// package, so the writer and the legacy-evidence reader agree.
const activityArtifactName = runpkg.ActivityArtifactName

// envActivity overrides activity-stream enablement. Its values are on/off words;
// any other value (or unset) falls back to interactive-terminal detection, so the
// default stays "stream on a terminal, silent when piped".
const envActivity = "SOP_ACTIVITY"

// activityStream is the CLI's activity Sink: it renders each structured event as
// one concise line, prefixed with elapsed time since the task started. It never
// prints prompts, model output, file contents, or raw tool arguments — producers
// emit only short summaries — and it redacts/truncates defensively as a backstop.
type activityStream struct {
	w     io.Writer
	start time.Time

	mu sync.Mutex
}

// newActivityStream returns a stream that times events relative to start.
func newActivityStream(w io.Writer, start time.Time) *activityStream {
	return &activityStream{w: w, start: start}
}

// Emit renders one event. Writing is serialized so concurrent producers cannot
// interleave a line.
func (s *activityStream) Emit(e activity.Event) {
	line := renderActivityLine(e, s.start)
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = io.WriteString(s.w, line+"\n")
}

// renderActivityLine formats one activity event: "[mm:ss] STAGE   message". The
// stage is padded so the messages align; the message is the action and detail
// joined, and is always a single line.
func renderActivityLine(e activity.Event, start time.Time) string {
	elapsed := e.Timestamp.Sub(start)
	if elapsed < 0 {
		elapsed = 0
	}
	message := activityMessage(e)
	if message == "" {
		message = e.Stage
	}
	return fmt.Sprintf("[%s] %-10s %s", elapsedClock(elapsed), e.Stage, message)
}

// activityMessage joins an event's action and detail into a single concise line.
func activityMessage(e activity.Event) string {
	action := oneLine(e.Action)
	detail := oneLine(e.Detail)
	switch {
	case action == "":
		return detail
	case detail == "":
		return action
	default:
		return action + " " + detail
	}
}

// oneLine collapses an event field to a single trimmed line, so a stray newline
// in a command cannot break the one-line-per-event contract.
func oneLine(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	return s
}

// elapsedClock renders a duration as mm:ss (minutes are not capped, so a very
// long task reads "72:05" rather than wrapping).
func elapsedClock(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	total := int64(d / time.Second)
	return fmt.Sprintf("%02d:%02d", total/60, total%60)
}

// taskActivityContext returns ctx carrying the task's activity recorder, or ctx
// unchanged when no consumer is available. When reporting is enabled it feeds two
// consumers from one event stream: the CLI renderer (when stdout is interactive)
// and the persisted run artifact (so a controller/API can read it later).
func taskActivityContext(ctx context.Context, runDir string, stdout io.Writer, taskID, title string) context.Context {
	// Always attach the trace collector, so the structured run trace observes the
	// trajectory independently of whether the human-facing stream is enabled. The
	// CLI renderer and the persisted activity artifact stay gated exactly as before
	// (redirection and non-interactive runs are unchanged); only the in-memory
	// collector - written to trace.json later - is always present. It observes the
	// same events and feeds no decision.
	collector := runtrace.NewCollector(time.Now)
	sinks := activity.Multi{collector}
	if activityEnabled(stdout) {
		sinks = append(sinks, newActivityStream(stdout, time.Now()))
		if runDir != "" {
			sinks = append(sinks, newActivityArtifact(filepath.Join(runDir, activityArtifactName)))
		}
	}
	rec := activity.New(taskID, sinks)
	rec.Emit(activity.StageStart, title, "")
	return runtrace.WithCollector(activity.WithRecorder(ctx, rec), collector)
}

// activityArtifact persists activity events as JSON lines. It is best-effort:
// any failure to encode, open, or write drops the event and never affects the
// run, so persistence can never change a task's outcome. Each write opens the
// file in append mode, so no file handle is retained across events.
type activityArtifact struct {
	path string

	mu sync.Mutex
}

// newActivityArtifact returns a sink that appends JSON-line events to path.
func newActivityArtifact(path string) *activityArtifact { return &activityArtifact{path: path} }

// Emit appends one JSON object for e. Writing is serialized so concurrent
// producers cannot interleave a line.
func (a *activityArtifact) Emit(e activity.Event) {
	data, err := json.Marshal(e)
	if err != nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	f, err := os.OpenFile(a.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	_, _ = f.Write(append(data, '\n'))
	_ = f.Close()
}

// activityEnabled reports whether activity should be streamed to w. An explicit
// SOP_ACTIVITY value wins; otherwise activity is streamed only to an interactive
// terminal, so non-interactive runs (pipes, files, tests) are unchanged.
func activityEnabled(w io.Writer) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(envActivity))) {
	case "1", "true", "on", "yes", "always":
		return true
	case "0", "false", "off", "no", "never":
		return false
	}
	return isTerminal(w)
}

// isTerminal reports whether w is a character device (an interactive terminal).
// A bytes.Buffer, a pipe, or a regular file is not, so redirecting stdout keeps
// the previous, quieter behavior.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
