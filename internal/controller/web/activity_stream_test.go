package web

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// CTRL007 verification (S1-S5). These tests are cursor/sequence driven: they
// never sleep and never wait on a ticker. They exercise the read-only activity
// transport directly against the same fixture the rest of the web suite uses.

type windowPayload struct {
	Cursor string            `json:"cursor"`
	More   bool              `json:"more"`
	Reset  bool              `json:"reset"`
	Events []activityPayload `json:"events"`
}

func getWindow(t *testing.T, srv *httptest.Server, id, after string, limit int) windowPayload {
	t.Helper()
	url := srv.URL + "/projects/" + id + "/activity/window"
	q := "?"
	if limit != 0 {
		q += "limit=" + strconv.Itoa(limit) + "&"
	}
	if after != "" {
		q += "after=" + after
	}
	resp, err := http.Get(url + q)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("window status %d", resp.StatusCode)
	}
	var w windowPayload
	if err := json.NewDecoder(resp.Body).Decode(&w); err != nil {
		t.Fatalf("decode window: %v", err)
	}
	return w
}

// readFrame reads one SSE frame (terminated by a blank line) from r. It blocks
// only on actual bytes from the connection and is not sensitive to how the
// transport coalesces or splits writes, so it replaces a single Read() that could
// observe a partial frame.
func readFrame(t *testing.T, r *bufio.Reader) string {
	t.Helper()
	var sb strings.Builder
	for {
		line, err := r.ReadString('\n')
		sb.WriteString(line)
		if err != nil {
			t.Fatalf("read SSE frame: %v (got %q)", err, sb.String())
		}
		if line == "\n" {
			return sb.String()
		}
	}
}

// S1/S2: cursor-less recovery returns the recent window, oldest first, without
// error; cursor determinism holds across repeated reads.
func TestActivityWindowRecoveryAndDeterminism(t *testing.T) {
	srv, id := newTestServer(t)

	first := getWindow(t, srv, id, "", 0)
	if first.Reset {
		t.Fatalf("fresh load must not set reset")
	}
	if len(first.Events) == 0 {
		t.Fatalf("expected recent events for fixture")
	}
	if first.Cursor != first.Events[len(first.Events)-1].Cursor {
		t.Fatalf("window cursor should be last event cursor")
	}

	// Determinism: the same persisted events yield identical cursors on repeat.
	second := getWindow(t, srv, id, "", 0)
	if len(second.Events) != len(first.Events) {
		t.Fatalf("non-deterministic window length: %d vs %d", len(first.Events), len(second.Events))
	}
	for i := range first.Events {
		if first.Events[i].Cursor != second.Events[i].Cursor {
			t.Fatalf("cursor diverged at %d", i)
		}
		if first.Events[i].Seq != second.Events[i].Seq {
			t.Fatalf("seq diverged at %d", i)
		}
	}
}

// S2: unknown cursor reports Reset=true and returns the bounded recent window.
func TestActivityWindowUnknownCursorResets(t *testing.T) {
	srv, id := newTestServer(t)
	win := getWindow(t, srv, id, "deadbeefdeadbeefdeadbeefdeadbeef", 0)
	if !win.Reset {
		t.Fatalf("unknown cursor must set reset")
	}
	if len(win.Events) == 0 {
		t.Fatalf("reset must still return the bounded recent window")
	}
}

// S2: limit bounds the delivered window.
func TestActivityWindowLimitBounds(t *testing.T) {
	srv, id := newTestServer(t)
	win := getWindow(t, srv, id, "", 1)
	if len(win.Events) > 1 {
		t.Fatalf("limit=1 should deliver at most one event, got %d", len(win.Events))
	}
	clamped := getWindow(t, srv, id, "", 100000)
	if clamped.Reset {
		t.Fatalf("clamped limit must not reset")
	}
}

// S3: tiling single-event windows forward from a known cursor yields every event
// exactly once. A cursor-less recovery returns the *most recent* window (that is
// its job), so forward tiling starts from the first event of the recovered window
// and uses strict-after resume: the concatenation must have no repeated Cursor,
// and the window past the end must deliver nothing.
func TestActivityWindowReplayNoDuplicates(t *testing.T) {
	srv, id := newTestServer(t)
	full := getWindow(t, srv, id, "", 0)
	if len(full.Events) < 2 {
		t.Fatalf("fixture needs at least 2 events, got %d", len(full.Events))
	}

	seen := map[string]bool{}
	seen[full.Events[0].Cursor] = true
	after := full.Events[0].Cursor

	// Replaying from the recovered window's first event must reproduce exactly
	// the recovered window, one event per request, in order.
	for i := 1; i < len(full.Events); i++ {
		win := getWindow(t, srv, id, after, 1)
		if win.Reset {
			t.Fatalf("resume %d: known cursor must not reset", i)
		}
		if len(win.Events) != 1 {
			t.Fatalf("resume %d: expected exactly one event, got %d", i, len(win.Events))
		}
		if win.Events[0].Cursor != full.Events[i].Cursor {
			t.Fatalf("resume %d: cursor %s, want %s", i, win.Events[0].Cursor, full.Events[i].Cursor)
		}
		if seen[win.Events[0].Cursor] {
			t.Fatalf("duplicate cursor during replay: %s", win.Events[0].Cursor)
		}
		seen[win.Events[0].Cursor] = true
		after = win.Cursor
	}

	// Exhausted: the next resume must deliver nothing (no re-delivery).
	done := getWindow(t, srv, id, after, 1)
	if len(done.Events) != 0 {
		t.Fatalf("replay past the end delivered %d events", len(done.Events))
	}
}

// S3: Seq is strictly increasing across the delivered window.
func TestActivityWindowStableOrdering(t *testing.T) {
	srv, id := newTestServer(t)
	win := getWindow(t, srv, id, "", 0)
	last := -1
	for _, ev := range win.Events {
		if ev.Seq <= last {
			t.Fatalf("seq not strictly increasing: %d after %d", ev.Seq, last)
		}
		last = ev.Seq
	}
}

// S3: simulated refresh (no cursor) then resume with last cursor yields only
// subsequent events (none, since the fixture is static).
func TestActivityWindowRefreshThenResume(t *testing.T) {
	srv, id := newTestServer(t)
	full := getWindow(t, srv, id, "", 0)
	if len(full.Events) == 0 {
		t.Skip("no fixture events")
	}
	resume := getWindow(t, srv, id, full.Cursor, 0)
	if resume.Reset {
		t.Fatalf("known cursor must not reset")
	}
	if len(resume.Events) != 0 {
		t.Fatalf("resume should deliver no already-seen events, got %d", len(resume.Events))
	}
}

// S2: the SSE endpoint emits the SSE content type and an initial ': ready' frame
// when the window for the supplied cursor is empty. The frame is read with a
// line-oriented reader so the assertion does not depend on how the transport
// coalesces writes (no bare single Read, no sleep).
func TestActivityStreamFrameShape(t *testing.T) {
	srv, id := newTestServer(t)

	full := getWindow(t, srv, id, "", 0)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET",
		srv.URL+"/projects/"+id+"/activity/stream?after="+full.Cursor, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type = %q", ct)
	}
	frame := readFrame(t, bufio.NewReader(resp.Body))
	if !strings.Contains(frame, ": ready") {
		t.Fatalf("expected ': ready' frame, got %q", frame)
	}
	cancel()
}

// S3: an SSE stream that already knows the cursor delivers only events after it,
// as cursor-keyed frames (id: <cursor>), and emits nothing for an exhausted
// cursor. This exercises the Last-Event-ID-equivalent resume path without timing.
func TestActivityStreamDeliversAfterCursor(t *testing.T) {
	srv, id := newTestServer(t)
	full := getWindow(t, srv, id, "", 0)
	if len(full.Events) < 2 {
		t.Fatalf("fixture needs at least 2 events, got %d", len(full.Events))
	}
	// Resume from the first event: the stream must emit the remaining events in
	// order, keyed by cursor, and never re-emit the first.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET",
		srv.URL+"/projects/"+id+"/activity/stream?after="+full.Events[0].Cursor, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	br := bufio.NewReader(resp.Body)
	seen := map[string]bool{}
	var order []string
	for i := 1; i < len(full.Events); i++ {
		// Read the frame's id/data lines until the blank-line terminator.
		var id string
		for {
			line, err := br.ReadString('\n')
			if err != nil {
				t.Fatalf("read SSE frame %d: %v", i, err)
			}
			line = strings.TrimRight(line, "\n")
			if strings.HasPrefix(line, "id: ") {
				id = strings.TrimPrefix(line, "id: ")
			}
			if line == "" {
				break
			}
		}
		if id == "" {
			t.Fatalf("frame %d had no id", i)
		}
		if id == full.Events[0].Cursor {
			t.Fatalf("stream re-emitted the resume cursor %s", id)
		}
		if seen[id] {
			t.Fatalf("stream duplicated cursor %s", id)
		}
		seen[id] = true
		order = append(order, id)
	}
	for i, got := range order {
		if got != full.Events[i+1].Cursor {
			t.Fatalf("stream order %d: got %s want %s", i, got, full.Events[i+1].Cursor)
		}
	}
	cancel()
}

// S2: a request carrying Last-Event-ID (as a reconnecting EventSource does) but
// no ?after= resumes from that cursor, so a reconnect does not re-emit the whole
// recent window.
func TestActivityWindowLastEventIDResume(t *testing.T) {
	srv, id := newTestServer(t)

	full := getWindow(t, srv, id, "", 0)
	if len(full.Events) == 0 {
		t.Fatalf("expected recent events for fixture")
	}
	url := srv.URL + "/projects/" + id + "/activity/window"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Last-Event-ID", full.Cursor)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var win windowPayload
	if err := json.NewDecoder(resp.Body).Decode(&win); err != nil {
		t.Fatalf("decode window: %v", err)
	}
	if win.Reset {
		t.Fatalf("Last-Event-ID resume of a known cursor must not reset")
	}
	if len(win.Events) != 0 {
		t.Fatalf("Last-Event-ID resume re-delivered %d events", len(win.Events))
	}
}

// nonFlusherWriter implements http.ResponseWriter but deliberately NOT
// http.Flusher, so the SSE handler's transport-capability check can be exercised.
type nonFlusherWriter struct {
	http.ResponseWriter
}

// S1/S5: the SSE handler requires an http.Flusher and rejects a writer that is
// not one, before touching any SOP state (note h.sop is nil here).
func TestActivityStreamRequiresFlusher(t *testing.T) {
	h := &Handlers{}
	rr := httptest.NewRecorder()
	w := nonFlusherWriter{ResponseWriter: rr}
	req := httptest.NewRequest("GET", "/projects/demo/activity/stream", nil)
	req.SetPathValue("project", "demo")
	h.activityStream(w, req)
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("non-flusher writer: status %d, want 500", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "streaming unsupported") {
		t.Fatalf("expected 'streaming unsupported', got %q", rr.Body.String())
	}
}

// flushRecorder is a minimal http.ResponseWriter + http.Flusher that records
// writes without a live connection, so streamActivity's failure path can be
// exercised directly.
type flushRecorder struct {
	header http.Header
	body   strings.Builder
	status int
}

func (f *flushRecorder) Header() http.Header { return f.header }
func (f *flushRecorder) Write(p []byte) (int, error) {
	return f.body.Write(p)
}
func (f *flushRecorder) WriteHeader(status int) { f.status = status }
func (f *flushRecorder) Flush()                 {}

// S5: a transport read failure ends the stream rather than leaving it hung, and
// nothing is written for the failed attempt. It drives streamActivity with an
// already-cancelled context (the same failure surface a missing root or a
// cancelled request produces) and asserts the stream terminates without a
// fabricated frame and without touching SOP state.
func TestActivityStreamEndsOnReadFailure(t *testing.T) {
	// h.sop is nil here on purpose: streamActivity must return before any SOP
	// access when the context is already cancelled, so no nil dereference occurs.
	h := &Handlers{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	rec := &flushRecorder{header: http.Header{}}
	var cursor string
	if h.streamActivity(ctx, rec, rec, "demo", &cursor, true) {
		t.Fatalf("cancelled context must end the stream, not report success")
	}
	if rec.body.Len() != 0 {
		t.Fatalf("failed read must emit no frame, got %q", rec.body.String())
	}
}
