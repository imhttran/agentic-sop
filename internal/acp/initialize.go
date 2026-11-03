package acp

import (
	"encoding/json"
	"fmt"
)

// initializeRequest is the pinned initialize params shape.
type initializeRequest struct {
	// ProtocolVersion is the client's declared version. Unset or null defers
	// to the server; the string "v1" and the number 1 are the accepted
	// spellings of the pinned version. Unknown extra fields are ignored here
	// by design: initialize is the negotiation point, so a later ACP draft
	// must not turn a handshake into a hard failure.
	ProtocolVersion    json.RawMessage `json:"protocolVersion"`
	ClientCapabilities json.RawMessage `json:"clientCapabilities"`
}

// handleInitialize negotiates the protocol version and returns the capability
// advertisement. An incompatible version is rejected with
// CodeVersionUnsupported, a message naming the supported version, and no
// capability set; the session stays uninitialized, so a later method still
// receives the ordering error rather than a session built on the wrong version.
func (s *Server) handleInitialize(req *Request) (any, *Error) {
	var params initializeRequest
	if len(req.Params) > 0 {
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, &Error{
				Code:    CodeInvalidParams,
				Message: "invalid initialize params: " + shorten(err.Error(), 128),
			}
		}
	}
	if rerr := negotiateVersion(params.ProtocolVersion); rerr != nil {
		return nil, rerr
	}
	s.setInitialized()
	// The response carries the numeric revision (ProtocolVersionNumber):
	// ACP clients validate this field against a number, and the string alias
	// "v1" is accepted only on input.
	return map[string]any{
		"protocolVersion":   ProtocolVersionNumber,
		"agentCapabilities": AgentCapabilities(),
		"agentInfo":         map[string]any{"name": ServerName, "version": s.version},
	}, nil
}

// negotiateVersion accepts the client's declared version when it matches the
// pinned contract: unset or null defers to the server, the pinned string is
// accepted exactly, and the numeric alias 1 is accepted for clients that
// declare a number. Anything else is a CodeVersionUnsupported error that names
// the pinned version.
func negotiateVersion(raw json.RawMessage) *Error {
	if len(raw) == 0 || string(raw) == "null" {
		return nil // the client defers; the pinned version applies
	}
	var declared string
	if err := json.Unmarshal(raw, &declared); err == nil {
		if declared == ProtocolVersion {
			return nil
		}
		return versionUnsupported(declared)
	}
	var numeric json.Number
	if err := json.Unmarshal(raw, &numeric); err == nil {
		if numeric.String() == "1" {
			return nil
		}
		return versionUnsupported(numeric.String())
	}
	return versionUnsupported(string(raw))
}

// versionUnsupported builds the actionable rejection: it names the value the
// client declared (redacted and bounded) and the pinned version to declare
// instead.
func versionUnsupported(declared string) *Error {
	return &Error{
		Code: CodeVersionUnsupported,
		Message: fmt.Sprintf("protocol version %s is not supported: this build is pinned to ACP %s; declare protocolVersion %q (or omit it) to connect",
			shorten(redactLine(declared), 64), ProtocolVersion, ProtocolVersion),
	}
}
