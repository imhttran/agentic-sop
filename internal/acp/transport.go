package acp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
)

// Request is one decoded JSON-RPC 2.0 frame. Extra envelope fields are ignored
// (JSON-RPC forbids client-side extensions here only by convention; strictness
// is reserved for method params, where the pinned shape lives).
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// IsNotification reports a frame with no id (or a null id), which under this
// contract consumes no dispatch and receives no response: no inbound ACP
// notification method is implemented in this build.
func (r *Request) IsNotification() bool {
	return len(r.ID) == 0 || string(r.ID) == "null"
}

// Error is a protocol-level failure reported to the client in the frame's
// error member. Code is one of the contract error codes; Message is
// human-readable and carries a remediation hint.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Error implements error so in-package callers can wrap it.
func (e *Error) Error() string {
	return fmt.Sprintf("acp error %d: %s", e.Code, e.Message)
}

// Server is the ACP stdio server for one client connection.
type Server struct {
	version string
	out     *protocolWriter
	diag    *diagnostics
	events  *eventBuffer

	mu          sync.Mutex
	initialized bool
	sessions    map[string]*Session

	// prompts is the governed prompt submission bridge injected by the host
	// process; nil means the server serves no session/prompt method.
	prompts *PromptSubmission
	// progress is the host-supplied persisted-evidence reader for outbound
	// session/update frames (progress.go); nil means the server emits no
	// progress — a missing seam never fabricates a frame.
	progress *ProgressSource
	// done is closed once Serve stops (stopStreams, follow.go), so a background
	// progress stream ends with the connection instead of writing to a protocol
	// stream nobody reads. doneOnce makes that close idempotent.
	done     chan struct{}
	doneOnce sync.Once
	// submissions records one definitive result per session and submission
	// key, so a resent submission is replayed instead of starting duplicate
	// work. It is keyed sessionID\x00submissionKey; submissionOrder bounds it.
	submissions     map[string]SubmissionResult
	submissionOrder []string

	// qa is the host-supplied planner-question and approval-blocker read
	// seam (approval.go, ACP-005); nil means no question or blocker display
	// and no approval gating: a missing seam never fabricates state.
	qa *QuestionApprovalSource
	// reportedBlockers remembers, per session, the joined identity of the
	// approval blockers already reported there, so an unchanged report stops
	// instead of retrying (no attempt or retry budget is consumed);
	// reportedQuestions remembers the displayed pending question set so a
	// repeated prompt does not spam an unchanged display.
	reportedBlockers  map[string]string
	reportedQuestions map[string]string
}

// ServerOption configures one optional server capability.
type ServerOption func(*Server)

// WithPromptSubmission wires the governed prompt submission bridge. Without
// it, session/prompt is rejected as not configured in this server instance.
func WithPromptSubmission(p *PromptSubmission) ServerOption {
	return func(s *Server) { s.prompts = p }
}

// NewServer builds a server that writes protocol frames to stdout and
// diagnostics to stderr. stdout is protocol-only: every frame goes through the
// serialized protocolWriter, and nothing else is written there. The variadic
// options wire optional capabilities such as prompt submission and progress.
func NewServer(version string, stdout, stderr io.Writer, opts ...ServerOption) *Server {
	s := &Server{
		version:     version,
		out:         newProtocolWriter(stdout),
		diag:        newDiagnostics(stderr),
		events:      newEventBuffer(EventBufferBound),
		sessions:    map[string]*Session{},
		done:        make(chan struct{}),
		submissions: map[string]SubmissionResult{},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	return s
}

// Serve reads newline-delimited JSON-RPC frames from in and writes one
// response per request until EOF. Each frame is decoded and dispatched exactly
// once. Buffered outbound event frames (session/update) drain to the client
// after each dispatched frame, so progress delivery rides the live
// request/response path. A frame that is not valid JSON, is not a request
// object, or exceeds MaxFrameSize produces an actionable error response and
// the loop resynchronizes on the next newline; the process neither crashes nor
// wedges. A clean EOF (client disconnect) first runs the documented
// disconnect recovery — reconcileDisconnect in cancel.go: safe cancellation of
// every live session's foreground job through the host cancel seam within
// CancelStopBound, then a persisted-state re-read, with no restart, no
// fabricated outcome, and no second lifecycle event — and then returns nil,
// which the CLI maps to exit code 0.
func (s *Server) Serve(ctx context.Context, in io.Reader) error {
	br := bufio.NewReader(in)
	// A live progress stream outlives individual requests, so shutdown ends it
	// before the connection does.
	defer s.stopStreams()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		line, oversize, err := readFrame(br)
		if err != nil {
			if errors.Is(err, io.EOF) {
				s.reconcileDisconnect()
				return nil
			}
			return fmt.Errorf("acp: read frame: %w", err)
		}
		if oversize {
			s.respond(nil, nil, &Error{
				Code: CodeInvalidRequest,
				Message: fmt.Sprintf("frame exceeds the %d-byte input bound; send one JSON-RPC 2.0 request per line of at most %d bytes and resend it smaller",
					MaxFrameSize, MaxFrameSize),
			})
			continue
		}
		if len(line) == 0 {
			continue // a blank line is not a frame
		}
		s.handleFrame(line)
	}
}

// readFrame returns the next newline-delimited frame without its terminator.
// oversize reports a frame that exceeded MaxFrameSize; its bytes are discarded
// so memory stays bounded by the bound, not by the frame.
func readFrame(br *bufio.Reader) (line []byte, oversize bool, err error) {
	var acc []byte
	limit := false
	for {
		chunk, err := br.ReadSlice('\n')
		if err == nil {
			if limit {
				return nil, true, nil
			}
			data := trimEOL(append(acc, chunk...))
			if len(data) > MaxFrameSize {
				return nil, true, nil
			}
			return data, false, nil
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			if !limit && len(acc)+len(chunk) > MaxFrameSize {
				limit = true
				acc = nil
			}
			if !limit {
				acc = append(acc, chunk...)
			}
			continue
		}
		if errors.Is(err, io.EOF) {
			// A frame interrupted by EOF is completed leniently (clients that
			// close stdin right after the last frame keep their final request);
			// the next call sees EOF again and ends the loop.
			if limit {
				return nil, true, nil
			}
			data := trimEOL(append(acc, chunk...))
			if len(data) > MaxFrameSize {
				return nil, true, nil
			}
			if len(data) == 0 {
				return nil, false, io.EOF
			}
			return data, false, nil
		}
		return nil, false, err
	}
}

// trimEOL drops one trailing newline and an optional carriage return.
func trimEOL(data []byte) []byte {
	data = bytes.TrimSuffix(data, []byte("\n"))
	data = bytes.TrimSuffix(data, []byte("\r"))
	return data
}

// handleFrame validates one decoded frame's envelope and dispatches it.
func (s *Server) handleFrame(line []byte) {
	if !json.Valid(line) {
		s.respond(nil, nil, &Error{
			Code:    CodeParseError,
			Message: "the frame is not valid JSON; send one JSON-RPC 2.0 request per line",
		})
		return
	}
	var req Request
	if err := json.Unmarshal(line, &req); err != nil {
		s.respond(nil, nil, &Error{
			Code:    CodeInvalidRequest,
			Message: "the frame is valid JSON but not a JSON-RPC 2.0 request object: " + shorten(err.Error(), 128),
		})
		return
	}
	// An id-less frame is a notification: no response. session/cancel is the one
	// inbound notification this build implements (ACP v1 defines cancellation as
	// a notification); every other notification is consumed as before.
	if req.IsNotification() {
		s.handleNotification(&req)
		return
	}
	if req.JSONRPC != "" && req.JSONRPC != "2.0" {
		s.respond(req.ID, nil, &Error{
			Code:    CodeInvalidRequest,
			Message: fmt.Sprintf("jsonrpc %q is not supported; the pinned protocol is JSON-RPC 2.0", req.JSONRPC),
		})
		return
	}
	if req.Method == "" {
		s.respond(req.ID, nil, &Error{
			Code:    CodeInvalidRequest,
			Message: "the frame has no method; send one JSON-RPC 2.0 request per line",
		})
		return
	}
	result, rerr := s.dispatch(&req)
	s.respond(req.ID, result, rerr)
	// Dispatch may have queued outbound session/update frames; drain them
	// after the response so the client receives the response first and the
	// whole frames follow in order.
	s.PumpEvents()
}

// handleNotification consumes one id-less frame without a response.
// session/cancel is the only inbound notification this build implements (ACP v1
// defines cancellation as a notification); every other notification is ignored,
// so an unknown notification can never desynchronize the stream or fabricate a
// result. A failed notification is recorded as a diagnostic, never as a response.
func (s *Server) handleNotification(req *Request) {
	if req.Method != MethodSessionCancel {
		return
	}
	if _, rerr := s.handleSessionCancel(req); rerr != nil {
		s.diag.Printf("session/cancel notification: %s", rerr.Message)
	}
	s.PumpEvents()
}

// dispatch routes one validated request. It never panics on client input and
// never leaves the stream desynchronized: every failure is a response.
func (s *Server) dispatch(req *Request) (any, *Error) {
	switch req.Method {
	case MethodInitialize:
		return s.handleInitialize(req)
	case MethodSessionNew:
		if err := s.requireInitialized(req.Method); err != nil {
			return nil, err
		}
		return s.handleSessionNew(req)
	case MethodSessionPrompt:
		if err := s.requireInitialized(req.Method); err != nil {
			return nil, err
		}
		result, rerr := s.handleSessionPrompt(req)
		if rerr == nil {
			// The turn ends at the transport boundary (a protocol-defined stop
			// reason); the SOP outcome of the started work travels only in the
			// session/update frames queued below, never in this response.
			s.stampPromptStopReason(result)
			s.streamPromptProgress(req, result)
		}
		return result, rerr
	case MethodSessionCancel:
		if err := s.requireInitialized(req.Method); err != nil {
			return nil, err
		}
		return s.handleSessionCancel(req)
	case MethodSessionLoad:
		// A valid ACP method whose capability is not advertised: reject with
		// the exact capability reason rather than a generic not-found.
		return nil, &Error{
			Code: CodeCapabilityUnsupported,
			Message: fmt.Sprintf("capability not advertised: %s: %s is not implemented in this build; supported methods are %s",
				CapabilityLoadSession, MethodSessionLoad, strings.Join(SupportedMethods, ", ")),
		}
	default:
		return nil, &Error{
			Code: CodeMethodNotFound,
			Message: fmt.Sprintf("unknown method %q: supported methods are %s",
				shorten(req.Method, 64), strings.Join(SupportedMethods, ", ")),
		}
	}
}

// requireInitialized gates method ordering after initialize.
func (s *Server) requireInitialized(method string) *Error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.initialized {
		return &Error{
			Code:    CodeNotInitialized,
			Message: fmt.Sprintf("send initialize before %s", method),
		}
	}
	return nil
}

// setInitialized records a completed initialize handshake.
func (s *Server) setInitialized() {
	s.mu.Lock()
	s.initialized = true
	s.mu.Unlock()
}

// respond writes one response frame through the single serialization point.
// result is ignored when rerr is set; the error member carries the failure.
func (s *Server) respond(id json.RawMessage, result any, rerr *Error) {
	frame := map[string]any{"jsonrpc": "2.0"}
	if len(id) == 0 {
		// JSON-RPC: an error response to a request whose id cannot be detected
		// still carries an id member, and it is null.
		frame["id"] = nil
	} else {
		frame["id"] = id
	}
	if rerr != nil {
		frame["error"] = rerr
	} else {
		frame["result"] = result
	}
	data, err := json.Marshal(frame)
	if err != nil {
		// Response values are plain JSON; a marshal failure is a server bug,
		// not a client condition. Keep the stream and record the failure.
		s.diag.Printf("marshal response: %v", err)
		return
	}
	if err := s.out.WriteFrame(data); err != nil {
		s.diag.Printf("write response: %v", err)
	}
}
