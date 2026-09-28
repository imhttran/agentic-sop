package ollamaagent

import (
	"fmt"
	"io"
)

// TraceRecord is one safe, per-turn diagnostic of the agent loop. It exists so a
// failed or slow run can be understood turn by turn without exposing secrets: it
// carries only the capability, phase, iteration, tool, a content-free request
// summary, progress, and termination — never model prompts, file contents,
// credentials, or API keys.
type TraceRecord struct {
	Capability string
	// Phase names the capability phase (for example PLAN's DISCOVERY or
	// SYNTHESIS). It is empty for single-phase capabilities.
	Phase string
	// Event is a phase transition marker with no turn of its own, such as
	// PLAN's discovery-to-synthesis transition.
	Event string
	// Detail is an optional short, secret-free reason attached to an Event (for
	// example why IMPLEMENT stayed in CHANGE instead of finalizing).
	Detail      string
	Iteration   int
	Tool        string
	Request     string
	Progress    string
	Recovery    bool
	Termination string
}

// Progress labels recorded in a TraceRecord.
const (
	progressOK     = "ok"
	progressRepeat = "repeat"
	// progressDenied marks a turn whose tool call was refused without executing.
	progressDenied = "denied"
)

// TraceLog is a bounded, inspectable per-turn diagnostic trail. It is
// informational only and never affects execution.
type TraceLog struct {
	max     int
	records []TraceRecord
}

// newTraceLog returns a trace retaining at most max records (a non-positive max
// keeps all).
func newTraceLog(max int) *TraceLog {
	return &TraceLog{max: max}
}

// Record appends a record, dropping the oldest when the bound is reached.
func (l *TraceLog) Record(r TraceRecord) {
	l.records = append(l.records, r)
	if l.max > 0 && len(l.records) > l.max {
		l.records = l.records[len(l.records)-l.max:]
	}
}

// Records returns a copy of the recorded events in order.
func (l *TraceLog) Records() []TraceRecord {
	out := make([]TraceRecord, len(l.records))
	copy(out, l.records)
	return out
}

// Flush writes a compact human-readable trace to w so an operator can see the
// turn-by-turn history of a failed run. A nil or empty trail writes nothing.
func (l *TraceLog) Flush(w io.Writer) {
	records := l.Records()
	if len(records) == 0 {
		return
	}
	fmt.Fprintf(w, "sop-ollama-agent: trace: %d turns\n", len(records))
	for _, r := range records {
		if r.Event != "" {
			line := "  " + r.Capability + " " + r.Event
			if r.Detail != "" {
				line += " (" + r.Detail + ")"
			}
			fmt.Fprintln(w, line)
			continue
		}
		line := "  " + r.Capability
		if r.Phase != "" {
			line += " " + r.Phase
		}
		line += fmt.Sprintf(" #%d", r.Iteration)
		if r.Tool != "" {
			line += " " + r.Tool
			if r.Request != "" {
				line += " " + r.Request
			}
		}
		if r.Progress != "" {
			line += " [" + r.Progress + "]"
		}
		if r.Recovery {
			line += " recovery-sent"
		}
		if r.Termination != "" {
			line += " termination=" + r.Termination
		}
		fmt.Fprintln(w, line)
	}
}
