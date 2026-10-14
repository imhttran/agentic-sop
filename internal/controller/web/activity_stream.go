package web

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/imhttran/agentic-sop/internal/controller/config"
	"github.com/imhttran/agentic-sop/internal/controller/sopclient"
)

// Live activity delivery (CTRL007): SSE (/activity/stream) and a bounded-poll
// fallback (/activity/window), both over sopclient.ActivityWindow. Both resume
// from the last-seen cursor, so a reconnect never duplicates events. The same
// ticker re-reads SOP's gates at attentionInterval (C2-007); inactivity never
// implies a gate.

// activityPayload is an ActivityView plus a "since" label, identical on both paths.
type activityPayload struct {
	Cursor    string `json:"cursor"`
	Seq       int    `json:"seq"`
	TaskID    string `json:"taskId"`
	Stage     string `json:"stage"`
	Action    string `json:"action"`
	Detail    string `json:"detail"`
	Timestamp string `json:"timestamp"`
	Since     string `json:"since"`
	StageCSS  string `json:"stageClass"`
}

func newActivityPayload(v sopclient.ActivityView) activityPayload {
	ts := ""
	if !v.Timestamp.IsZero() {
		ts = v.Timestamp.UTC().Format(time.RFC3339Nano)
	}
	return activityPayload{
		Cursor:    v.Cursor,
		Seq:       v.Seq,
		TaskID:    v.TaskID,
		Stage:     v.Stage,
		Action:    v.Action,
		Detail:    v.Detail,
		Timestamp: ts,
		Since:     since(v.Timestamp),
		StageCSS:  stageClass(v.Stage),
	}
}

// attentionPayload is one SOP-reported decision for the live consumer (C2-007).
type attentionPayload struct {
	TaskID string `json:"taskId"`
	Title  string `json:"title,omitempty"`
	Kind   string `json:"kind,omitempty"`
}

const activityStreamWindow = sopclient.DefaultActivityWindow

// attentionInterval resolves the attention cadence: zero means
// config.AttentionIntervalDefault, and it never goes below
// config.AttentionIntervalFloor.
func (h *Handlers) attentionInterval() time.Duration {
	if h.attention <= 0 {
		return config.AttentionIntervalDefault
	}
	if h.attention < config.AttentionIntervalFloor {
		return config.AttentionIntervalFloor
	}
	return h.attention
}

// resumeCursor prefers ?after= and falls back to Last-Event-ID, which a browser
// sends on EventSource reconnect; without it a reconnect would replay the window.
func resumeCursor(r *http.Request) string {
	if after := r.URL.Query().Get("after"); after != "" {
		return after
	}
	return r.Header.Get("Last-Event-ID")
}

// activityLimit reads ?limit=, falling back to the default window.
func activityLimit(r *http.Request) int {
	n, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || n <= 0 {
		return sopclient.DefaultActivityWindow
	}
	return n
}

// activityWindow is the poll endpoint: same window shape and cursor contract as
// the stream, plus the attention projection and cadence (C2-007).
func (h *Handlers) activityWindow(w http.ResponseWriter, r *http.Request) {
	project := r.PathValue("project")
	win, err := h.sop.ActivityWindow(r.Context(), project, resumeCursor(r), activityLimit(r))
	if err != nil {
		h.renderError(w, r, err)
		return
	}
	out := struct {
		Cursor            string             `json:"cursor"`
		More              bool               `json:"more"`
		Reset             bool               `json:"reset"`
		Events            []activityPayload  `json:"events"`
		Attention         []attentionPayload `json:"attention"`
		AttentionInterval string             `json:"attentionInterval"`
	}{
		Cursor:            win.Cursor,
		More:              win.More,
		Reset:             win.Reset,
		Events:            make([]activityPayload, 0, len(win.Events)),
		Attention:         h.projectAttention(r.Context(), project),
		AttentionInterval: h.attentionInterval().String(),
	}
	for _, v := range win.Events {
		out.Events = append(out.Events, newActivityPayload(v))
	}
	writeJSON(w, http.StatusOK, out)
}

// projectAttention builds the attention projection from the decisions reads. A
// read failure yields no attention, never a fabricated gate.
func (h *Handlers) projectAttention(ctx context.Context, project string) []attentionPayload {
	detail, err := h.sop.Project(ctx, project)
	if err != nil {
		return nil
	}
	changed, changedErr := h.readChangedTasks(ctx, detail)
	d := decisions(h.baseForProject(project), detail, changed, changedErr)
	return attentionFromDecisions(d)
}

// attentionFromDecisions lists SOP-reported gates and pending accepts.
func attentionFromDecisions(d decisionsData) []attentionPayload {
	out := make([]attentionPayload, 0, len(d.Gates)+len(d.Accepts))
	for _, g := range d.Gates {
		out = append(out, attentionPayload(g))
	}
	for _, a := range d.Accepts {
		out = append(out, attentionPayload{TaskID: a.TaskID, Title: a.Title, Kind: "accept-changed"})
	}
	return out
}

// activityStream is the SSE endpoint. It is read-only; any transport failure
// just ends the stream. Each window is followed by an attention frame when SOP
// reports a gate (C2-007).
func (h *Handlers) activityStream(w http.ResponseWriter, r *http.Request) {
	// Reject non-streaming clients before touching the project.
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	project := r.PathValue("project")
	if _, ok := h.sop.Root(project); !ok {
		h.notFound(w, r, "Project not found")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ctx := r.Context()
	cursor := resumeCursor(r)
	// Tick at the attention cadence so new gates show promptly.
	ticker := time.NewTicker(h.attentionInterval())
	defer ticker.Stop()

	// Send the current window immediately; if that first read fails, end the stream
	// rather than leave it hanging.
	if !h.streamActivity(ctx, w, flusher, project, &cursor, true) {
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !h.streamActivity(ctx, w, flusher, project, &cursor, false) {
				// A later read or write failure ends the stream so the client reconnects or polls.
				return
			}
		}
	}
}

// streamActivity sends one window of events, a ': ready' keepalive for an empty
// initial window, then any attention frame. It returns false when the stream
// should end.
func (h *Handlers) streamActivity(ctx context.Context, w http.ResponseWriter, flusher http.Flusher, project string, cursor *string, initial bool) bool {
	if ctx.Err() != nil {
		return false
	}
	win, err := h.sop.ActivityWindow(ctx, project, *cursor, activityStreamWindow)
	if err != nil {
		return false
	}
	if len(win.Events) == 0 {
		if initial {
			if _, err := w.Write([]byte(": ready\n\n")); err != nil {
				return false
			}
		}
		*cursor = win.Cursor
		if !h.streamAttention(ctx, w, project) {
			return false
		}
		if initial {
			flusher.Flush()
		}
		return true
	}
	for _, v := range win.Events {
		payload := newActivityPayload(v)
		raw, err := json.Marshal(payload)
		if err != nil {
			continue
		}
		frame := "event: activity\nid: " + payload.Cursor + "\ndata: "
		if _, err := w.Write([]byte(frame)); err != nil {
			return false
		}
		if _, err := w.Write(raw); err != nil {
			return false
		}
		if _, err := w.Write([]byte("\n\n")); err != nil {
			return false
		}
	}
	*cursor = win.Cursor

	if !h.streamAttention(ctx, w, project) {
		return false
	}
	flusher.Flush()
	return true
}

// streamAttention sends one `event: attention` frame when a gate exists, nothing
// otherwise. It returns false only on a write failure.
func (h *Handlers) streamAttention(ctx context.Context, w http.ResponseWriter, project string) bool {
	attention := h.projectAttention(ctx, project)
	if len(attention) == 0 {
		return true
	}
	raw, err := json.Marshal(attention)
	if err != nil {
		return true
	}
	if _, err := w.Write([]byte("event: attention\ndata: ")); err != nil {
		return false
	}
	if _, err := w.Write(raw); err != nil {
		return false
	}
	if _, err := w.Write([]byte("\n\n")); err != nil {
		return false
	}
	return true
}
