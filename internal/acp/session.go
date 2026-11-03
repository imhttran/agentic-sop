package acp

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// sessionNewRequest is the pinned session/new params shape.
type sessionNewRequest struct {
	// Cwd is the working directory the session operates in.
	Cwd string `json:"cwd"`
	// MCPServers lists the MCP servers the client starts for the session.
	// Absent means no servers.
	MCPServers []mcpServerSpec `json:"mcpServers"`
}

// mcpServerSpec describes one MCP server the client starts.
type mcpServerSpec struct {
	Name    string            `json:"name"`
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// handleSessionNew validates the pinned params shape, binds the session to a
// validated canonical project directory, and returns its descriptor. It never
// persists state and never starts workflow execution: session ownership beyond
// creation (for example governed prompt submission) belongs to the later ACP
// stages that consume sessions.
func (s *Server) handleSessionNew(req *Request) (any, *Error) {
	var params sessionNewRequest
	if len(req.Params) > 0 {
		dec := json.NewDecoder(bytes.NewReader(req.Params))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&params); err != nil {
			return nil, &Error{
				Code: CodeInvalidParams,
				Message: fmt.Sprintf("invalid session/new params: %v; expected fields are cwd and mcpServers",
					shorten(err.Error(), 128)),
			}
		}
		if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			return nil, &Error{
				Code:    CodeInvalidParams,
				Message: "invalid session/new params: unexpected trailing data; send one JSON object",
			}
		}
	}
	if params.Cwd == "" {
		return nil, &Error{
			Code:    CodeInvalidParams,
			Message: "cwd is required: pass the working directory the session operates in",
		}
	}
	canonical, err := canonicalProjectDir(params.Cwd)
	if err != nil {
		return nil, &Error{Code: CodeInvalidParams, Message: "invalid session/new cwd: " + err.Error()}
	}
	sess := s.registerSession(params, canonical)
	return map[string]any{"sessionId": sess.ID}, nil
}

// Session is one in-memory ACP session record. It is never persisted.
type Session struct {
	ID string
	// Cwd is the directory as the client spelled it; ProjectDir is its
	// canonical (absolute, symlink-resolved) form, and ProjectID is the
	// stable identity derived from the canonical directory. Two sessions on
	// differently spelled paths that resolve to the same directory therefore
	// share a project identity, while sessions on different directories stay
	// isolated.
	Cwd        string
	ProjectDir string
	ProjectID  string
	Servers    []mcpServerSpec
	Created    time.Time
	// ActiveJobID is the adapter-owned started job this session last submitted or
	// reconciled (ACP-006). session/cancel stops exactly this job; empty means the
	// session has no foreground governed work to cancel.
	ActiveJobID string
	// streaming records the jobs this session has a live follow stream for
	// (follow.go), so a repeated prompt for running work re-uses that answer
	// instead of starting a second stream. It is in-memory session state under
	// the server mutex, bounded by the jobs the session submitted, and dies with
	// the session; it is never persisted and never restored across a restart.
	streaming map[string]bool
}

// setSessionJob records the started job a session's foreground work belongs to.
func (s *Server) setSessionJob(sessionID, jobID string) {
	if strings.TrimSpace(jobID) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess := s.sessions[sessionID]; sess != nil {
		sess.ActiveJobID = jobID
	}
}

// sessionJob returns the session's recorded foreground job identity, or "".
func (s *Server) sessionJob(sessionID string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sess := s.sessions[sessionID]; sess != nil {
		return sess.ActiveJobID
	}
	return ""
}

// registerSession records the session bound to the canonical project
// directory and returns it. The registry mutex also sequences identifier
// assignment, so concurrent session/new requests always receive distinct
// identifiers.
func (s *Server) registerSession(params sessionNewRequest, canonical string) *Session {
	sess := &Session{
		ID:         newSessionID(),
		Cwd:        params.Cwd,
		ProjectDir: canonical,
		ProjectID:  projectIDFor(canonical),
		Servers:    params.MCPServers,
		Created:    time.Now().UTC(),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[sess.ID] = sess
	return sess
}

// getSession returns the live session record, or nil for an unknown id.
func (s *Server) getSession(id string) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessions[id]
}

// newSessionID returns a stable, unique identifier: a random 64-bit hex value
// under the sess_ prefix. If the random source fails (practically impossible),
// a nanosecond timestamp keeps the request alive rather than failing it.
func newSessionID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err == nil {
		return "sess_" + hex.EncodeToString(b[:])
	}
	return fmt.Sprintf("sess_%x", time.Now().UnixNano())
}

// sessionCount reports the live in-memory session count. Read-only.
func (s *Server) sessionCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.sessions)
}

// activeJobSessions snapshots the live sessions that own a foreground job
// (ActiveJobID set), so disconnect recovery (reconcileDisconnect in cancel.go)
// can act on each snapshot without holding the server mutex while it calls
// the host cancel seam. The snapshot is a shallow copy: recovery reads only
// its identity fields.
func (s *Server) activeJobSessions() []Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Session, 0, len(s.sessions))
	for _, sess := range s.sessions {
		if strings.TrimSpace(sess.ActiveJobID) != "" {
			out = append(out, *sess)
		}
	}
	return out
}
