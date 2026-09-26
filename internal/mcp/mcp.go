// Package mcp implements a minimal Model Context Protocol server over stdio.
//
// It speaks newline-delimited JSON-RPC 2.0 (initialize, tools/list, tools/call)
// and exposes tools whose handlers call the same application services as the
// CLI — the workflow is never reimplemented here. Security is by construction:
// only registered tools exist, each is scoped to one project, and there is no
// arbitrary filesystem or shell access beyond what a tool is explicitly given.
package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ProtocolVersion is the MCP protocol revision this server implements.
const ProtocolVersion = "2024-11-05"

// ServerName identifies this server in the initialize handshake.
const ServerName = "sop"

// Tool is a callable MCP tool. Handler receives the raw arguments object and
// returns text content; an error is reported as tool content with isError set.
type Tool struct {
	Name        string
	Description string
	InputSchema map[string]any
	Handler     func(ctx context.Context, args json.RawMessage) (string, error)
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
	codeMethodNotFound = -32601
	codeInvalidParams  = -32602
)

// Serve reads requests from in and writes responses to out until in reaches EOF
// or the context is cancelled. Notifications produce no response.
func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	dec := json.NewDecoder(in)
	enc := json.NewEncoder(out)

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		var req rpcRequest
		if err := dec.Decode(&req); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("mcp: decode request: %w", err)
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
		out = append(out, map[string]any{
			"name":        t.Name,
			"description": t.Description,
			"inputSchema": schema,
		})
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
