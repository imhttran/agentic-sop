package acp

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"
)

// This file covers the live-progress stage (follow.go): the frame a prompt turn
// gets when it started no streamable work, the frame a terminal record answers
// with, the output excerpt that reaches the client, and the follow stream that
// carries a running job to its terminal state.
//
// The step tests drive stepStream directly, so the stream's decisions are
// proven without a clock. One bounded integration test exercises the goroutine
// itself against a short poll interval, because the loop is what a real editor
// observes.

// liveProgress is a ProgressSource whose snapshot can change while a stream is
// reading it, with a fixed output excerpt. It is safe for the concurrent access
// a follow stream makes after the request that started it returned.
type liveProgress struct {
	mu     sync.Mutex
	snaps  map[string]ProgressSnapshot
	output string
	reads  int
}

func (l *liveProgress) source() *ProgressSource {
	return &ProgressSource{
		Progress: func(projectDir, jobID string) (ProgressSnapshot, bool, error) {
			l.mu.Lock()
			defer l.mu.Unlock()
			l.reads++
			snap, ok := l.snaps[jobID]
			return snap, ok, nil
		},
		Output: func(projectDir, jobID string) (string, error) {
			l.mu.Lock()
			defer l.mu.Unlock()
			return l.output, nil
		},
	}
}

func (l *liveProgress) set(snap ProgressSnapshot) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.snaps[snap.JobID] = snap
}

// updateText returns the text body of one session/update frame.
func updateText(t *testing.T, frame map[string]any) string {
	t.Helper()
	params, ok := frame["params"].(map[string]any)
	if !ok {
		t.Fatalf("frame has no params object: %v", frame)
	}
	update, ok := params["update"].(map[string]any)
	if !ok {
		t.Fatalf("frame params carry no update object: %v", params)
	}
	content, ok := update["content"].(map[string]any)
	if !ok {
		t.Fatalf("update carries no content object: %v", update)
	}
	text, _ := content["text"].(string)
	return text
}

// decodeChunk decodes every protocol frame in one raw output chunk.
func decodeChunk(t *testing.T, chunk string) []map[string]any {
	t.Helper()
	chunk = strings.TrimSuffix(chunk, "\n")
	if chunk == "" {
		return nil
	}
	parts := strings.Split(chunk, "\n")
	frames := make([]map[string]any, 0, len(parts))
	for _, p := range parts {
		var m map[string]any
		if err := json.Unmarshal([]byte(p), &m); err != nil {
			t.Fatalf("decode frame %q: %v", p, err)
		}
		frames = append(frames, m)
	}
	return frames
}

// TestStepStreamFollowsOneJobToItsTerminalState drives one stream step at a
// time and proves the three stream decisions: an unchanged state emits nothing,
// a state change emits exactly one frame, and the terminal state emits its own
// frame plus the output excerpt behind it before the stream ends.
func TestStepStreamFollowsOneJobToItsTerminalState(t *testing.T) {
	src := &liveProgress{
		snaps:  map[string]ProgressSnapshot{"work-1": {JobID: "work-1", State: ProgressStateRunning}},
		output: "PLAN: do the thing",
	}
	fix := newProgressFixture(t, nil, src.source())
	sessID := fix.openSession(t.TempDir())
	srv := fix.server
	st := &jobStream{sess: srv.getSession(sessID), jobID: "work-1", last: ProgressStateRunning}

	before := fix.out.Len()
	if done := srv.stepStream(st); done {
		t.Fatal("stream ended while the job's persisted state was still running")
	}
	if got := fix.out.Len(); got != before {
		t.Fatalf("wrote %d bytes for a state the client already had", got-before)
	}

	src.set(ProgressSnapshot{JobID: "work-1", State: ProgressStateCompleted})
	before = fix.out.Len()
	if done := srv.stepStream(st); !done {
		t.Fatal("stream did not end at the job's terminal state")
	}
	frames := decodeChunk(t, fix.out.String()[before:])
	if len(frames) != 2 {
		t.Fatalf("terminal step wrote %d frames, want the terminal frame and the output frame", len(frames))
	}
	if got, _ := decodeSOP(t, frames[0])["state"].(string); got != ProgressStateCompleted {
		t.Errorf("terminal frame state = %q, want %q", got, ProgressStateCompleted)
	}
	if got := decodeSOP(t, frames[1])["notice"]; got != NoticeJobOutput {
		t.Errorf("output frame notice = %v, want %q", got, NoticeJobOutput)
	}
	if text := updateText(t, frames[1]); !strings.Contains(text, "PLAN: do the thing") {
		t.Errorf("output frame text = %q, want the persisted output excerpt", text)
	}
}

// TestStepStreamEndsWhenTheRecordDisappears proves a vanished record ends the
// stream with a diagnostic and no frame: the adapter never turns a missing
// record into an outcome.
func TestStepStreamEndsWhenTheRecordDisappears(t *testing.T) {
	src := &liveProgress{snaps: map[string]ProgressSnapshot{}}
	fix := newProgressFixture(t, nil, src.source())
	sessID := fix.openSession(t.TempDir())
	srv := fix.server
	st := &jobStream{sess: srv.getSession(sessID), jobID: "work-gone", last: ProgressStateRunning}

	before := fix.out.Len()
	if done := srv.stepStream(st); !done {
		t.Fatal("stream continued after the job record disappeared")
	}
	if got := fix.out.Len(); got != before {
		t.Fatalf("wrote %d bytes for a missing record", got-before)
	}
	if !strings.Contains(fix.errOut.String(), "no longer has a persisted record") {
		t.Errorf("diagnostics = %q, want the missing-record notice", fix.errOut.String())
	}
}

// TestTerminalPromptAnswersWithItsRecordAndOutput proves a prompt whose job is
// already terminal — the reconciled resend that used to be silent — answers with
// the terminal frame and the output excerpt, and starts no stream.
func TestTerminalPromptAnswersWithItsRecordAndOutput(t *testing.T) {
	src := &liveProgress{
		snaps:  map[string]ProgressSnapshot{"work-1": {JobID: "work-1", State: ProgressStateCompleted}},
		output: "PLAN: do the thing",
	}
	fb := &fakeBridge{start: SubmissionResult{Outcome: WorkOutcomeReconciled, JobID: "work-1", State: persistedStateCompleted}}
	fix := newProgressFixture(t, fb, src.source())
	sessID := fix.openSession(t.TempDir())

	frames := fix.prompt(sessID, "explain", nil)
	if len(frames) != 3 {
		t.Fatalf("wrote %d frames, want the response, the terminal frame, and the output frame", len(frames))
	}
	if got, _ := frames[0]["result"].(map[string]any)["outcome"].(string); got != WorkOutcomeReconciled {
		t.Fatalf("outcome = %q, want %q", got, WorkOutcomeReconciled)
	}
	if got, _ := decodeSOP(t, frames[1])["state"].(string); got != ProgressStateCompleted {
		t.Errorf("terminal frame state = %q, want %q", got, ProgressStateCompleted)
	}
	if got := decodeSOP(t, frames[2])["notice"]; got != NoticeJobOutput {
		t.Errorf("output frame notice = %v, want %q", got, NoticeJobOutput)
	}
	if text := updateText(t, frames[2]); !strings.Contains(text, "PLAN: do the thing") {
		t.Errorf("output frame text = %q, want the persisted output excerpt", text)
	}
	if streams := fix.streamClaims(sessID); len(streams) != 0 {
		t.Errorf("a terminal record claimed a stream: %v", streams)
	}
}

// TestFollowStreamRunsFromPromptToTerminal exercises the goroutine an editor
// actually observes: a running snapshot starts a stream, a later terminal
// snapshot reaches the client with its output, and the stream releases its claim
// so a later prompt about the same job is answered again.
func TestFollowStreamRunsFromPromptToTerminal(t *testing.T) {
	prev := followPollInterval
	followPollInterval = 2 * time.Millisecond
	t.Cleanup(func() { followPollInterval = prev })

	src := &liveProgress{
		snaps:  map[string]ProgressSnapshot{"work-1": {JobID: "work-1", State: ProgressStateRunning}},
		output: "PLAN: do the thing",
	}
	fb := &fakeBridge{start: SubmissionResult{Outcome: WorkOutcomeStarted, JobID: "work-1", State: persistedStateRunning}}
	out := &syncBuffer{}
	errOut := &syncBuffer{}
	srv := NewServer("test-version", out, errOut,
		WithPromptSubmission((&recordedCalls{bridges: fb}).bridge()),
		WithProgress(src.source()))
	t.Cleanup(srv.stopStreams)

	sessID := sendSessionNew(t, srv, out, t.TempDir())
	before := out.Len()
	sendPrompt(t, srv, out, sessID, "explain")
	if frames := decodeChunk(t, out.String()[before:]); len(frames) != 2 {
		t.Fatalf("prompt wrote %d frames, want the response and the running frame", len(frames))
	}

	src.set(ProgressSnapshot{JobID: "work-1", State: ProgressStateVerified})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		text := out.String()
		if strings.Contains(text, "verification passed") && strings.Contains(text, "PLAN: do the thing") {
			break
		}
		time.Sleep(time.Millisecond)
	}
	text := out.String()
	if !strings.Contains(text, "verification passed") {
		t.Fatalf("stream never delivered the terminal frame; output = %q", text)
	}
	if !strings.Contains(text, "PLAN: do the thing") {
		t.Fatalf("stream never delivered the output frame; output = %q", text)
	}

	// The stream releases its claim, so the same session can answer for the job
	// again instead of being treated as permanently in progress.
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && len(streamClaims(t, srv, sessID)) != 0 {
		time.Sleep(time.Millisecond)
	}
	if claims := streamClaims(t, srv, sessID); len(claims) != 0 {
		t.Fatalf("stream claim was never released: %v", claims)
	}
}

// TestOutputTextIsBoundedAndRuneSafe proves the frame body never exceeds the
// bound and never starts mid-rune after truncation.
func TestOutputTextIsBoundedAndRuneSafe(t *testing.T) {
	// A multi-byte rune straddling the cut: the bound cuts the last four bytes,
	// which lands inside this three-byte rune, so the helper must advance.
	excerpt := strings.Repeat("a", MaxOutputChunkBytes-1) + "€tail"
	text := outputText("work-1", excerpt)
	if len(text) > MaxOutputChunkBytes+len("…")+len("SOP job work-1 output:\n\n") {
		t.Fatalf("frame body = %d bytes, want it bounded", len(text))
	}
	if strings.Contains(text, "\ufffd") {
		t.Error("frame body contains a replacement rune: the cut split a rune")
	}
	body := strings.TrimPrefix(text, "SOP job work-1 output:\n\n")
	if !strings.HasPrefix(body, "…") {
		t.Errorf("truncated body does not start with the truncation marker: %q", body[:8])
	}
	if !strings.HasSuffix(body, "tail") {
		t.Errorf("truncated body lost the end of the excerpt, which carries the result")
	}
}

// TestPromptOutcomeText proves the notice body repeats the result rather than
// inventing one, and that an empty result still produces a sentence.
func TestPromptOutcomeText(t *testing.T) {
	for _, tc := range []struct {
		name string
		res  map[string]any
		want string
	}{
		{"outcome and message", map[string]any{"outcome": "busy", "message": "nothing was started"}, "SOP outcome busy: nothing was started"},
		{"message only", map[string]any{"message": "approval is pending"}, "SOP: approval is pending"},
		{"outcome only", map[string]any{"outcome": "cancelled"}, "SOP outcome cancelled."},
		{"neither", map[string]any{}, "SOP answered without an outcome or message; read the project's persisted state with sop status."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := promptOutcomeText(tc.res); got != tc.want {
				t.Errorf("promptOutcomeText(%v) = %q, want %q", tc.res, got, tc.want)
			}
		})
	}
}

// syncBuffer is a bytes.Buffer safe for the concurrent writes a follow stream
// makes after the request that started it returned.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Len()
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// streamClaims returns the jobs the session holds a live stream for.
func (f *progressFixture) streamClaims(sessionID string) []string {
	return streamClaims(f.t, f.server, sessionID)
}

// streamClaims reads the session's stream claims under the server mutex, so the
// test observes the same state the server does.
func streamClaims(t *testing.T, srv *Server, sessionID string) []string {
	t.Helper()
	srv.mu.Lock()
	defer srv.mu.Unlock()
	sess := srv.sessions[sessionID]
	if sess == nil {
		return nil
	}
	out := make([]string, 0, len(sess.streaming))
	for jobID := range sess.streaming {
		out = append(out, jobID)
	}
	return out
}

// sendSessionNew initializes one server and opens one session on dir, returning
// the session id.
func sendSessionNew(t *testing.T, srv *Server, out *syncBuffer, dir string) string {
	t.Helper()
	sendFrame(t, srv, map[string]any{"jsonrpc": "2.0", "id": 1, "method": MethodInitialize, "params": map[string]any{"protocolVersion": ProtocolVersion}})
	before := out.Len()
	sendFrame(t, srv, map[string]any{"jsonrpc": "2.0", "id": 2, "method": MethodSessionNew, "params": map[string]any{"cwd": dir}})
	frames := decodeChunk(t, out.String()[before:])
	if len(frames) != 1 {
		t.Fatalf("session/new wrote %d frames, want one response", len(frames))
	}
	result, _ := frames[0]["result"].(map[string]any)
	sessID, _ := result["sessionId"].(string)
	if sessID == "" {
		t.Fatalf("session/new returned no sessionId: %v", frames[0])
	}
	return sessID
}

// sendPrompt submits one text prompt.
func sendPrompt(t *testing.T, srv *Server, out *syncBuffer, sessionID, text string) {
	t.Helper()
	sendFrame(t, srv, map[string]any{
		"jsonrpc": "2.0", "id": 3, "method": MethodSessionPrompt,
		"params": map[string]any{"sessionId": sessionID, "prompt": []map[string]any{{"type": "text", "text": text}}},
	})
}

// sendFrame marshals and dispatches one request frame.
func sendFrame(t *testing.T, srv *Server, frame map[string]any) {
	t.Helper()
	data, err := json.Marshal(frame)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	srv.handleFrame(data)
}
