package sopclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
)

// Live activity delivery (CTRL007). Store.activity is per task; this adds a
// stable per-event Cursor, a project-wide order (per-task append order, tasks
// by id), and bounded windows after a cursor. An unknown cursor returns the
// recent window with Reset set, instead of an error. Read-only; Detail is
// already sanitized by Store.activity.

// ActivityView is an event plus its Cursor and project-wide Seq.
type ActivityView struct {
	ActivityEvent
	// Cursor is also the "after" value for the next request.
	Cursor string `json:"cursor"`
	Seq    int    `json:"seq"`
}

// DefaultActivityWindow bounds a recovery window when no limit is supplied.
const DefaultActivityWindow = 100

// MaxActivityWindow bounds any window request, regardless of what a caller asks.
const MaxActivityWindow = 500

// activityCursor is deterministic over the event's persisted fields.
func activityCursor(projectID, taskID string, index int, e ActivityEvent) string {
	h := sha256.New()
	for _, part := range []string{
		projectID, taskID, strconv.Itoa(index),
		e.Timestamp.UTC().Format("2006-01-02T15:04:05.000000000Z07:00"),
		e.Stage, e.Action, e.Detail,
	} {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:32]
}

// ActivityWindow is a bounded, ordered window used by live delivery and recovery.
type ActivityWindow struct {
	Events []ActivityView `json:"events"`
	// Cursor is the last delivered event, or the supplied cursor if none.
	Cursor string `json:"cursor"`
	// More: the window was truncated by the limit.
	More bool `json:"more"`
	// Reset: a non-empty "after" was not found, so Events is the recent window and
	// the caller's timeline may have gaps.
	Reset bool `json:"reset"`
}

// ActivityWindow returns events strictly after `after`, or the most recent limit
// events when after is empty or unknown (Reset). limit <= 0 uses
// DefaultActivityWindow; it is clamped to MaxActivityWindow.
func (c *Client) ActivityWindow(ctx context.Context, projectID, after string, limit int) (ActivityWindow, error) {
	st, ok := c.stores[projectID]
	if !ok {
		return ActivityWindow{}, ErrProjectNotFound
	}
	if limit <= 0 {
		limit = DefaultActivityWindow
	}
	if limit > MaxActivityWindow {
		limit = MaxActivityWindow
	}

	all, err := st.activityViews(ctx, projectID)
	if err != nil {
		return ActivityWindow{}, err
	}

	win := ActivityWindow{Cursor: after, Events: []ActivityView{}}
	start := 0
	if after != "" {
		// Resume after the last-seen event; an unknown cursor falls through to recovery.
		if idx := indexOfCursor(all, after); idx >= 0 {
			start = idx + 1
		} else {
			win.Reset = true
		}
	}
	remaining := all[start:]
	if start == 0 {
		if len(remaining) > limit {
			remaining = remaining[len(remaining)-limit:]
		}
	} else if len(remaining) > limit {
		remaining = remaining[:limit]
	}
	win.More = len(all)-start > len(remaining)
	win.Events = append([]ActivityView{}, remaining...)
	if n := len(win.Events); n > 0 {
		win.Cursor = win.Events[n-1].Cursor
	}
	return win, nil
}

// TaskActivityView returns one task's views from its own activity, so a task's
// older events still render. Seq is offset by preceding tasks' events to match
// the project-wide order.
func (c *Client) TaskActivityView(ctx context.Context, projectID, taskID string) ([]ActivityView, error) {
	st, ok := c.stores[projectID]
	if !ok {
		return nil, ErrProjectNotFound
	}
	ids, err := st.taskIDs(ctx)
	if err != nil {
		return nil, err
	}
	seq := 0
	for _, id := range ids {
		events := st.activity(id)
		if id == taskID {
			out := make([]ActivityView, 0, len(events))
			for i, e := range events {
				out = append(out, ActivityView{
					ActivityEvent: e,
					Cursor:        activityCursor(projectID, id, i, e),
					Seq:           seq + i,
				})
			}
			return out, nil
		}
		seq += len(events)
	}
	return nil, ErrTaskNotFound
}

// activityViews returns every project event in total order with cursor and Seq.
func (s *Store) activityViews(ctx context.Context, projectID string) ([]ActivityView, error) {
	ids, err := s.taskIDs(ctx)
	if err != nil {
		return nil, err
	}
	var out []ActivityView
	seq := 0
	for _, id := range ids {
		events := s.activity(id)
		for i, e := range events {
			out = append(out, ActivityView{
				ActivityEvent: e,
				Cursor:        activityCursor(projectID, id, i, e),
				Seq:           seq,
			})
			seq++
		}
	}
	return out, nil
}

// indexOfCursor returns the position of the event with the given cursor, or -1.
func indexOfCursor(views []ActivityView, cursor string) int {
	for i := range views {
		if views[i].Cursor == cursor {
			return i
		}
	}
	return -1
}
