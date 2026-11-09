// Package mcp implements a minimal Model Context Protocol server over stdio.
//
// It speaks newline-delimited JSON-RPC 2.0 (initialize, tools/list, tools/call)
// and exposes tools whose handlers call the same application services as the
// CLI — the workflow is never reimplemented here. Security is by construction:
// only registered tools exist, each is scoped to one project, and there is no
// arbitrary filesystem or shell access beyond what a tool is explicitly given.
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ProtocolVersion is the MCP protocol revision this server implements.
const ProtocolVersion = "2024-11-05"

// MaxFrameSize bounds one newline-delimited JSON-RPC frame, so a client cannot
// make the server allocate an unbounded request. It mirrors the ACP transport's
// frame bound; a normal request is far smaller.
const MaxFrameSize = 1 << 20

// ServerName identifies this server in the initialize handshake.
const ServerName = "sop"

// Tool is a callable MCP tool. Handler receives the raw arguments object and
// returns text content; an error is reported as tool content with isError set.
type Tool struct {
	Name        string
	Description string
	InputSchema map[string]any
	// ReadOnly, when true, advertises the MCP annotation readOnlyHint: true in
	// tools/list so a client (for example ChatGPT) does not gate the call behind a
	// write-confirmation prompt. It is descriptive only: access is enforced by
	// which tools the server registers, never by this flag.
	ReadOnly bool
	Handler  func(ctx context.Context, args json.RawMessage) (string, error)
}

// Server is an MCP server exposing a fixed set of tools.
type Server struct {
	version string
	tools   []Tool
}

// NewServer builds a server exposing tools.
func NewServer(version string, tools ...Tool) *Server {
	return &Server{version: version, tools: tools}
}

// rpcRequest is a JSON-RPC request or notification.
type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// rpcResponse is a JSON-RPC response. Error is nil on success.
type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// JSON-RPC error codes.
const (
	codeParseError     = -32700
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
)

// Serve reads newline-delimited requests from in and writes responses to out
// until in reaches EOF or the context is cancelled. A frame larger than
// MaxFrameSize is rejected once with a parse error and the rest of that line is
// discarded, so one oversized request cannot unboundedly allocate or poison the
// stream. Notifications produce no response.
func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	reader := bufio.NewReaderSize(in, 64<<10)
	enc := json.NewEncoder(out)

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		frame, err := readFrame(reader, MaxFrameSize)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			if errors.Is(err, errFrameTooLarge) {
				if eerr := enc.Encode(s.fail(json.RawMessage("null"), codeParseError,
					fmt.Sprintf("request exceeds the %d-byte frame bound", MaxFrameSize))); eerr != nil {
					return fmt.Errorf("mcp: encode response: %w", eerr)
				}
				continue
			}
			return fmt.Errorf("mcp: read frame: %w", err)
		}
		if len(frame) == 0 {
			continue
		}

		var req rpcRequest
		if err := json.Unmarshal(frame, &req); err != nil {
			if eerr := enc.Encode(s.fail(json.RawMessage("null"), codeParseError, "invalid JSON request")); eerr != nil {
				return fmt.Errorf("mcp: encode response: %w", eerr)
			}
			continue
		}

		resp := s.handle(ctx, req)
		if resp == nil {
			continue // notification
		}
		if err := enc.Encode(resp); err != nil {
			return fmt.Errorf("mcp: encode response: %w", err)
		}
	}
}

// errFrameTooLarge reports a frame that exceeded the configured bound. The rest
// of that line is discarded before the error is returned.
var errFrameTooLarge = errors.New("mcp: frame exceeds bound")

// readFrame reads one newline-delimited frame, retaining at most max bytes (plus
// the delimiter). A larger frame is drained through its newline and reported as
// errFrameTooLarge so the caller can answer and continue.
func readFrame(r *bufio.Reader, max int) ([]byte, error) {
	var buf []byte
	tooLarge := false
	for {
		chunk, err := r.ReadSlice('\n')
		if !tooLarge {
			if len(buf)+len(chunk) > max+1 {
				tooLarge = true
				buf = nil
			} else {
				buf = append(buf, chunk...)
			}
		}
		switch {
		case err == nil:
			if tooLarge {
				return nil, errFrameTooLarge
			}
			return bytes.TrimSpace(buf), nil
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		case errors.Is(err, io.EOF):
			if tooLarge {
				return nil, errFrameTooLarge
			}
			if len(buf) > 0 {
				return bytes.TrimSpace(buf), nil
			}
			return nil, io.EOF
		default:
			return nil, err
		}
	}
}

// handle dispatches one request. It returns nil for a notification.
func (s *Server) handle(ctx context.Context, req rpcRequest) *rpcResponse {
	notification := len(req.ID) == 0

	switch req.Method {
	case "initialize":
		return s.respond(req.ID, map[string]any{
			"protocolVersion": ProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": ServerName, "version": s.version},
		})
	case "notifications/initialized":
		return nil
	case "tools/list":
		return s.respond(req.ID, map[string]any{"tools": s.toolDescriptors()})
	case "tools/call":
		return s.callTool(ctx, req)
	default:
		if notification {
			return nil
		}
		return s.fail(req.ID, codeMethodNotFound, "method not found: "+req.Method)
	}
}

// callTool runs a tool and wraps its output as MCP tool content.
func (s *Server) callTool(ctx context.Context, req rpcRequest) *rpcResponse {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(req.Params, &params); err != nil {
		return s.fail(req.ID, codeInvalidParams, "invalid tools/call params")
	}

	tool, ok := s.find(params.Name)
	if !ok {
		return s.fail(req.ID, codeInvalidParams, "unknown tool: "+params.Name)
	}

	text, err := tool.Handler(ctx, params.Arguments)
	if err != nil {
		return s.respond(req.ID, toolResult(err.Error(), true))
	}
	return s.respond(req.ID, toolResult(text, false))
}

// toolResult builds the MCP tool result envelope.
func toolResult(text string, isError bool) map[string]any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
		"isError": isError,
	}
}

// toolDescriptors renders the tools/list payload.
func (s *Server) toolDescriptors() []map[string]any {
	out := make([]map[string]any, 0, len(s.tools))
	for _, t := range s.tools {
		schema := t.InputSchema
		if schema == nil {
			schema = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		desc := map[string]any{
			"name":        t.Name,
			"description": t.Description,
			"inputSchema": schema,
		}
		if t.ReadOnly {
			desc["annotations"] = map[string]any{"readOnlyHint": true}
		}
		out = append(out, desc)
	}
	return out
}

func (s *Server) find(name string) (Tool, bool) {
	for _, t := range s.tools {
		if t.Name == name {
			return t, true
		}
	}
	return Tool{}, false
}

func (s *Server) respond(id json.RawMessage, result any) *rpcResponse {
	return &rpcResponse{JSONRPC: "2.0", ID: id, Result: result}
}

func (s *Server) fail(id json.RawMessage, code int, message string) *rpcResponse {
	return &rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message}}
}

// WithReadOnly returns a copy of the tool marked read-only, so tools/list
// advertises annotations.readOnlyHint: true for it.
func (t Tool) WithReadOnly() Tool {
	t.ReadOnly = true
	return t
}

// TextTool builds a tool whose handler ignores arguments and returns text.
func TextTool(name, description string, handler func(ctx context.Context) (string, error)) Tool {
	return Tool{
		Name:        name,
		Description: description,
		InputSchema: map[string]any{"type": "object", "properties": map[string]any{}},
		Handler: func(ctx context.Context, _ json.RawMessage) (string, error) {
			return handler(ctx)
		},
	}
}

// JoinLines joins non-empty lines, trimming trailing whitespace.
func JoinLines(lines []string) string {
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}
