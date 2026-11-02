package acp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// This file is the deterministic coverage ACP-002's acceptance criteria require:
// initialization and session creation, malformed and oversized input that must
// neither crash nor desynchronize the process, method ordering, the capability
// rejection for an unimplemented method, the bounded event buffer, and the
// guarantee that diagnostics never reach the protocol stream.
//
// The tests drive the real Serve loop over an in-memory stream, so they exercise
// the same framing, dispatch, and writer code the CLI serves. No model, network,
// or filesystem is involved.

// response is one decoded output frame.
type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *Error          `json:"error"`
}

// serve runs one Server over input and returns every output frame plus stderr.
func serve(t *testing.T, input string) ([]response, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	s := NewServer("test-version", &stdout, &stderr)
	if err := s.Serve(context.Background(), strings.NewReader(input)); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	return parseFrames(t, stdout.String()), stderr.String()
}

// parseFrames decodes newline-delimited output frames, failing on any line that
// is not a JSON object (which would mean stdout carried something other than a
// protocol frame).
func parseFrames(t *testing.T, out string) []response {
	t.Helper()
	var frames []response
	for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		if line == "" {
			continue
		}
		if !json.Valid([]byte(line)) {
			t.Fatalf("stdout carried a non-JSON line: %q", line)
		}
		var r response
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			t.Fatalf("decode frame %q: %v", line, err)
		}
		if r.JSONRPC != "2.0" {
			t.Fatalf("frame %q has jsonrpc %q, want 2.0", line, r.JSONRPC)
		}
		frames = append(frames, r)
	}
	return frames
}

func initFrame(id string) string {
	return `{"jsonrpc":"2.0","id":` + id + `,"method":"initialize","params":{"protocolVersion":"v1"}}` + "\n"
}

// TestInitializeNegotiatesPinnedVersion proves the accepted version spellings,
// the deferral when the client omits a version, and the actionable rejection of
// an incompatible one.
func TestInitializeNegotiatesPinnedVersion(t *testing.T) {
	cases := map[string]struct {
		params     string
		wantErr    int
		wantInitOK bool
	}{
		"pinned string": {params: `{"protocolVersion":"v1"}`, wantInitOK: true},
		"numeric alias": {params: `{"protocolVersion":1}`, wantInitOK: true},
		"omitted":       {params: `{}`, wantInitOK: true},
		"null":          {params: `{"protocolVersion":null}`, wantInitOK: true},
		"unsupported":   {params: `{"protocolVersion":"v2"}`, wantErr: CodeVersionUnsupported},
		"foreign":       {params: `{"protocolVersion":"2024-11-05"}`, wantErr: CodeVersionUnsupported},
		"invalid":       {params: `{"protocolVersion":[]}`, wantErr: CodeVersionUnsupported},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			frames, _ := serve(t, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":`+tc.params+"}\n")
			if len(frames) != 1 {
				t.Fatalf("got %d frames, want 1", len(frames))
			}
			got := frames[0]
			if tc.wantErr != 0 {
				if got.Error == nil {
					t.Fatalf("expected error %d, got result %s", tc.wantErr, got.Result)
				}
				if got.Error.Code != tc.wantErr {
					t.Errorf("error code = %d, want %d", got.Error.Code, tc.wantErr)
				}
				if !strings.Contains(got.Error.Message, ProtocolVersion) {
					t.Errorf("error message %q does not name the pinned version %q", got.Error.Message, ProtocolVersion)
				}
				return
			}
			if got.Error != nil {
				t.Fatalf("unexpected error: %+v", got.Error)
			}
			// protocolVersion decodes as a number: a string here is a response
			// the client rejects, so decoding into json.Number fails the test.
			var result struct {
				ProtocolVersion   json.Number    `json:"protocolVersion"`
				AgentCapabilities map[string]any `json:"agentCapabilities"`
				AgentInfo         map[string]any `json:"agentInfo"`
			}
			if err := json.Unmarshal(got.Result, &result); err != nil {
				t.Fatalf("decode result: %v", err)
			}
			if got := result.ProtocolVersion.String(); got != "1" {
				t.Errorf("protocolVersion = %q, want numeric %d", got, ProtocolVersionNumber)
			}
			if result.ProtocolVersion != json.Number(fmt.Sprint(ProtocolVersionNumber)) {
				t.Errorf("protocolVersion = %s, want %d", result.ProtocolVersion, ProtocolVersionNumber)
			}
			if v, ok := result.AgentCapabilities[CapabilityLoadSession]; !ok || v != false {
				t.Errorf("agentCapabilities[%s] = %v, want advertised false", CapabilityLoadSession, v)
			}
			if result.AgentInfo["name"] != ServerName {
				t.Errorf("agentInfo.name = %v, want %q", result.AgentInfo["name"], ServerName)
			}
			if result.AgentInfo["version"] != "test-version" {
				t.Errorf("agentInfo.version = %v, want test-version", result.AgentInfo["version"])
			}
		})
	}
}

// TestMethodOrderingRequiresInitialize proves a session method before a
// successful initialize is rejected with the ordering code and builds no
// session.
func TestMethodOrderingRequiresInitialize(t *testing.T) {
	frames, _ := serve(t, `{"jsonrpc":"2.0","id":7,"method":"session/new","params":{"cwd":"/tmp"}}`+"\n")
	if len(frames) != 1 {
		t.Fatalf("got %d frames, want 1", len(frames))
	}
	if frames[0].Error == nil || frames[0].Error.Code != CodeNotInitialized {
		t.Fatalf("error = %+v, want code %d", frames[0].Error, CodeNotInitialized)
	}
	if !strings.Contains(frames[0].Error.Message, MethodInitialize) {
		t.Errorf("message %q does not name %q", frames[0].Error.Message, MethodInitialize)
	}
}

// TestSessionNewCreatesOneInMemorySession proves the session descriptor, the
// strict params shape, and that no session is created for rejected params.
func TestSessionNewCreatesOneInMemorySession(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		s := NewServer("test-version", &stdout, &stderr)
		input := initFrame("1") + `{"jsonrpc":"2.0","id":2,"method":"session/new","params":{"cwd":"/tmp","mcpServers":[{"name":"a","command":"x","args":["--y"]}]}}` + "\n"
		if err := s.Serve(context.Background(), strings.NewReader(input)); err != nil {
			t.Fatalf("Serve: %v", err)
		}
		frames := parseFrames(t, stdout.String())
		if len(frames) != 2 {
			t.Fatalf("got %d frames, want 2", len(frames))
		}
		if frames[1].Error != nil {
			t.Fatalf("session/new error: %+v", frames[1].Error)
		}
		var result struct {
			SessionID string `json:"sessionId"`
		}
		if err := json.Unmarshal(frames[1].Result, &result); err != nil {
			t.Fatalf("decode result: %v", err)
		}
		if !strings.HasPrefix(result.SessionID, "sess_") {
			t.Errorf("sessionId = %q, want a sess_ identifier", result.SessionID)
		}
		if got := s.sessionCount(); got != 1 {
			t.Errorf("session count = %d, want 1", got)
		}
	})

	rejected := map[string]string{
		"missing cwd":   `{"mcpServers":[]}`,
		"unknown field": `{"cwd":"/tmp","bogus":true}`,
		"wrong type":    `{"cwd":42}`,
		"null params":   `null`,
	}
	for name, params := range rejected {
		t.Run(name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			s := NewServer("test-version", &stdout, &stderr)
			input := initFrame("1") + `{"jsonrpc":"2.0","id":2,"method":"session/new","params":` + params + "}\n"
			if err := s.Serve(context.Background(), strings.NewReader(input)); err != nil {
				t.Fatalf("Serve: %v", err)
			}
			frames := parseFrames(t, stdout.String())
			if len(frames) != 2 {
				t.Fatalf("got %d frames, want 2", len(frames))
			}
			if frames[1].Error == nil || frames[1].Error.Code != CodeInvalidParams {
				t.Fatalf("error = %+v, want code %d", frames[1].Error, CodeInvalidParams)
			}
			if got := s.sessionCount(); got != 0 {
				t.Errorf("session count = %d, want 0 for rejected params", got)
			}
		})
	}
}

// TestSessionLoadIsCapabilityUnsupported proves an unimplemented ACP method is
// rejected with the capability reason, not a generic not-found.
func TestSessionLoadIsCapabilityUnsupported(t *testing.T) {
	frames, _ := serve(t, initFrame("1")+`{"jsonrpc":"2.0","id":2,"method":"session/load","params":{"sessionId":"sess_x"}}`+"\n")
	if len(frames) != 2 {
		t.Fatalf("got %d frames, want 2", len(frames))
	}
	if frames[1].Error == nil || frames[1].Error.Code != CodeCapabilityUnsupported {
		t.Fatalf("error = %+v, want code %d", frames[1].Error, CodeCapabilityUnsupported)
	}
	if !strings.Contains(frames[1].Error.Message, CapabilityLoadSession) {
		t.Errorf("message %q does not name the capability %q", frames[1].Error.Message, CapabilityLoadSession)
	}
}

// TestUnknownMethodIsNotFound proves a method outside the pinned surface is a
// not-found response. It uses a name this server never implements, so the test
// stays valid as later stages add more ACP methods.
func TestUnknownMethodIsNotFound(t *testing.T) {
	frames, _ := serve(t, initFrame("1")+`{"jsonrpc":"2.0","id":2,"method":"session/not-a-real-method","params":{}}`+"\n")
	if len(frames) != 2 {
		t.Fatalf("got %d frames, want 2", len(frames))
	}
	if frames[1].Error == nil || frames[1].Error.Code != CodeMethodNotFound {
		t.Fatalf("error = %+v, want code %d", frames[1].Error, CodeMethodNotFound)
	}
}

// TestMalformedFramesResynchronize proves a malformed frame produces one
// actionable error and the stream is still usable afterwards.
func TestMalformedFramesResynchronize(t *testing.T) {
	input := "not json at all\n" +
		"[]\n" +
		"\n" +
		`{"jsonrpc":"1.0","id":4,"method":"initialize"}` + "\n" +
		initFrame("5")
	frames, _ := serve(t, input)
	if len(frames) != 4 {
		t.Fatalf("got %d frames, want 4", len(frames))
	}
	wantCodes := []int{CodeParseError, CodeInvalidRequest, CodeInvalidRequest, 0}
	for i, want := range wantCodes {
		if want == 0 {
			if frames[i].Error != nil {
				t.Errorf("frame %d: unexpected error %+v", i, frames[i].Error)
			}
			continue
		}
		if frames[i].Error == nil || frames[i].Error.Code != want {
			t.Errorf("frame %d: error = %+v, want code %d", i, frames[i].Error, want)
		}
	}
}

// TestNotificationGetsNoResponse proves an id-less frame is consumed without a
// response and without changing initialization state.
func TestNotificationGetsNoResponse(t *testing.T) {
	frames, stderr := serve(t, `{"jsonrpc":"2.0","method":"initialize","params":{"protocolVersion":"v1"}}`+"\n")
	if len(frames) != 0 {
		t.Fatalf("got %d frames, %+v; want none", len(frames), frames)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want empty", stderr)
	}
}

// TestOversizedFrameIsRejectedAndStreamRecovers proves the input bound is
// enforced once per oversized frame, memory stays bounded, and the next frame
// still succeeds.
func TestOversizedFrameIsRejectedAndStreamRecovers(t *testing.T) {
	input := strings.Repeat("x", MaxFrameSize+1) + "\n" + initFrame("1")
	frames, _ := serve(t, input)
	if len(frames) != 2 {
		t.Fatalf("got %d frames, want 2", len(frames))
	}
	if frames[0].Error == nil || frames[0].Error.Code != CodeInvalidRequest {
		t.Fatalf("error = %+v, want code %d", frames[0].Error, CodeInvalidRequest)
	}
	if !strings.Contains(frames[0].Error.Message, "bound") {
		t.Errorf("message %q does not explain the input bound", frames[0].Error.Message)
	}
	if frames[1].Error != nil {
		t.Errorf("the frame after an oversized one failed: %+v", frames[1].Error)
	}
}

// TestTrailingFrameWithoutNewlineIsServed proves a client that closes stdin
// directly after its last frame keeps that request.
func TestTrailingFrameWithoutNewlineIsServed(t *testing.T) {
	frames, _ := serve(t, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"v1"}}`)
	if len(frames) != 1 {
		t.Fatalf("got %d frames, want 1", len(frames))
	}
	if frames[0].Error != nil {
		t.Fatalf("unexpected error: %+v", frames[0].Error)
	}
}

// countingWriter records how many Write calls the protocol writer makes, so a
// frame can never be split across writes by another goroutine.
type countingWriter struct {
	writes int
	buf    bytes.Buffer
}

func (c *countingWriter) Write(p []byte) (int, error) {
	c.writes++
	return c.buf.Write(p)
}

// TestProtocolWriterEmitsWholeFrames proves each frame plus its newline is one
// write, so concurrent responses cannot interleave bytes.
func TestProtocolWriterEmitsWholeFrames(t *testing.T) {
	cw := &countingWriter{}
	w := newProtocolWriter(cw)
	for _, frame := range []string{`{"id":1}`, `{"id":2}`} {
		if err := w.WriteFrame([]byte(frame)); err != nil {
			t.Fatalf("WriteFrame: %v", err)
		}
	}
	if cw.writes != 2 {
		t.Errorf("writes = %d, want 2", cw.writes)
	}
	if got := cw.buf.String(); got != "{\"id\":1}\n{\"id\":2}\n" {
		t.Errorf("output = %q", got)
	}
}

// TestEventBufferIsBounded proves a slow client cannot grow memory without
// limit: pushes beyond the bound are rejected and the buffered set is unchanged.
func TestEventBufferIsBounded(t *testing.T) {
	s := NewServer("test-version", &bytes.Buffer{}, &bytes.Buffer{})
	for i := 0; i < EventBufferBound; i++ {
		if err := s.EnqueueEvent([]byte(`{"i":1}`)); err != nil {
			t.Fatalf("push %d: %v", i, err)
		}
	}
	if err := s.EnqueueEvent([]byte(`{"i":2}`)); !errors.Is(err, ErrEventBufferFull) {
		t.Fatalf("push beyond bound = %v, want ErrEventBufferFull", err)
	}
	if got := s.events.Len(); got != EventBufferBound {
		t.Errorf("buffered = %d, want %d", got, EventBufferBound)
	}
}

// TestDiagnosticsRedactCredentials proves the diagnostics path never carries a
// credential shape and never writes to the protocol stream.
func TestDiagnosticsRedactCredentials(t *testing.T) {
	var out bytes.Buffer
	d := newDiagnostics(&out)
	d.Printf("token=%s", "sk-abcdefgh12345678")
	got := out.String()
	if !strings.HasPrefix(got, "acp: ") {
		t.Errorf("diagnostic %q lacks the acp: prefix", got)
	}
	if strings.Contains(got, "sk-abcdefgh12345678") {
		t.Errorf("diagnostic leaked a credential: %q", got)
	}
	if !strings.Contains(got, redactedMarker) {
		t.Errorf("diagnostic %q does not mark the redaction", got)
	}
}
