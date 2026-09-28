package ollamaagent

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
)

// envTraceLog optionally names a file to append the per-turn diagnostic trace to.
// Like SOP_TOOL_AUDIT_LOG it is chosen by the operator and must never be SOP's
// state database; inspecting the trace never touches .agent-sdlc/state.db.
const envTraceLog = "SOP_AGENT_TRACE_LOG"

// TraceRecord is one safe, per-turn diagnostic of the agent loop. It exists so a
// failed or slow run can be understood turn by turn without exposing secrets: it
// carries only the capability, iteration, tool, a content-free request summary,
// progress, and termination — never model prompts, file contents, credentials, or
// API keys.
type TraceRecord struct {
	Capability  string `json:"capability"`
	Iteration   int    `json:"iteration"`
	Tool        string `json:"tool,omitempty"`
	Request     string `json:"request,omitempty"`
	Progress    string `json:"progress,omitempty"`
	Recovery    bool   `json:"recovery,omitempty"`
	Termination string `json:"termination,omitempty"`
}

// Progress labels recorded in a TraceRecord.
const (
	progressOK     = "ok"
	progressRepeat = "repeat"
)

// TraceLog is a bounded, inspectable per-turn diagnostic trail with an optional
// durable sink. It is informational only and never affects execution.
type TraceLog struct {
	mu      sync.Mutex
	max     int
	path    string
	records []TraceRecord
}

// newTraceLog returns a trace retaining at most max records (a non-positive max
// keeps all) and, when path is non-empty, appending each record as a JSON line.
func newTraceLog(max int, path string) *TraceLog {
	return &TraceLog{max: max, path: path}
}

// Record appends a record, dropping the oldest when the in-memory bound is
// reached, and best-effort appends it to the durable sink.
func (l *TraceLog) Record(r TraceRecord) {
	l.mu.Lock()
	l.records = append(l.records, r)
	if l.max > 0 && len(l.records) > l.max {
		l.records = l.records[len(l.records)-l.max:]
	}
	path := l.path
	l.mu.Unlock()

	if path == "" {
		return
	}
	line, err := json.Marshal(r)
	if err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(line, '\n'))
}

// Records returns a copy of the recorded events in order.
func (l *TraceLog) Records() []TraceRecord {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]TraceRecord, len(l.records))
	copy(out, l.records)
	return out
}

// Flush writes a compact human-readable trace to w so an operator can see the
// turn-by-turn history of a failed run without reading the durable sink. A nil or
// empty trail writes nothing.
func (l *TraceLog) Flush(w io.Writer) {
	records := l.Records()
	if len(records) == 0 {
		return
	}
	fmt.Fprintf(w, "sop-ollama-agent: trace: %d turns\n", len(records))
	for _, r := range records {
		line := fmt.Sprintf("  %s #%d", r.Capability, r.Iteration)
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
