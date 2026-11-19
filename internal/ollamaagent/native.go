package ollamaagent

import "context"

// nativeToolSpec is the provider-neutral description of one tool declared to a
// native-tool provider. Only the tools the current capability policy permits are
// included; FINALIZE declares none.
type nativeToolSpec struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// nativeChatClient is implemented by the oMLX OpenAI-compatible transport. The
// governed loop keeps its existing content path when this is absent.
type nativeChatClient interface {
	chatWithTools(ctx context.Context, messages []chatMessage, tools []nativeToolSpec) (string, []toolCall, error)
}

// appendToolOutcome appends one tool outcome using the native transcript when a
// native tool_call_id is available, and the content protocol otherwise. Denial
// paths use it too, so a denied native call still reaches the model as a tool
// result with its original id.
func (h *Harness) appendToolOutcome(messages []chatMessage, raw, name string, args map[string]any, nativeID, content string) []chatMessage {
	if h.cfg.NativeTools && nativeID != "" {
		return append(messages,
			chatMessage{Role: "assistant", Content: raw, NativeToolCalls: []toolCall{{ID: nativeID, Name: name, Args: args}}},
			chatMessage{Role: "tool", ToolCallID: nativeID, Content: content},
		)
	}
	return append(messages,
		chatMessage{Role: "assistant", Content: assistantEcho(name, args, raw)},
		chatMessage{Role: "user", Content: content},
	)
}

func nativeToolSpecs(policy CapabilityPolicy, phase implementPhase) []nativeToolSpec {
	if phase == implFinalize {
		return nil
	}
	out := make([]nativeToolSpec, 0, len(policy.AllowedTools))
	for _, name := range policy.ToolList() {
		params, ok := nativeToolSchemas[name]
		if !ok {
			continue
		}
		out = append(out, nativeToolSpec{Name: name, Description: nativeToolDescriptions[name], Parameters: params})
	}
	return out
}

var nativeToolDescriptions = map[string]string{
	"read_file":    "Read a file from the repository.",
	"write_file":   "Write or overwrite a file; creates missing parent directories.",
	"create_file":  "Create a new regular file; fails if it exists.",
	"delete_file":  "Delete a file.",
	"restore_file": "Restore a file from version control.",
	"copy_file":    "Copy a file byte-for-byte into the repository.",
	"rewrite_file": "Replace an exact string in one file.",
	"list_files":   "List a directory.",
	"search_files": "Search for a substring.",
	"run_command":  "Run an allow-listed command.",
	"git_status":   "Show git status.",
	"git_diff":     "Show the git diff.",
}

var nativeToolSchemas = map[string]map[string]any{
	"read_file": {
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string"},
			"root": map[string]any{"type": "string"},
		},
		"required":             []string{"path"},
		"additionalProperties": false,
	},
	"write_file": {
		"type": "object",
		"properties": map[string]any{
			"path":    map[string]any{"type": "string"},
			"content": map[string]any{"type": "string"},
			"root":    map[string]any{"type": "string"},
		},
		"required":             []string{"path", "content"},
		"additionalProperties": false,
	},
	"create_file": {
		"type": "object",
		"properties": map[string]any{
			"path":    map[string]any{"type": "string"},
			"content": map[string]any{"type": "string"},
			"root":    map[string]any{"type": "string"},
		},
		"required":             []string{"path", "content"},
		"additionalProperties": false,
	},
	"delete_file": {
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string"},
			"root": map[string]any{"type": "string"},
		},
		"required":             []string{"path"},
		"additionalProperties": false,
	},
	"restore_file": {
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string"},
			"root": map[string]any{"type": "string"},
		},
		"required":             []string{"path"},
		"additionalProperties": false,
	},
	"copy_file": {
		"type": "object",
		"properties": map[string]any{
			"source":      map[string]any{"type": "string"},
			"source_root": map[string]any{"type": "string"},
			"dest":        map[string]any{"type": "string"},
		},
		"required":             []string{"source", "dest"},
		"additionalProperties": false,
	},
	"rewrite_file": {
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string"},
			"from": map[string]any{"type": "string"},
			"to":   map[string]any{"type": "string"},
			"root": map[string]any{"type": "string"},
		},
		"required":             []string{"path", "from", "to"},
		"additionalProperties": false,
	},
	"list_files": {
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{"type": "string"},
			"root": map[string]any{"type": "string"},
		},
		"additionalProperties": false,
	},
	"search_files": {
		"type": "object",
		"properties": map[string]any{
			"pattern": map[string]any{"type": "string"},
			"path":    map[string]any{"type": "string"},
			"root":    map[string]any{"type": "string"},
		},
		"required":             []string{"pattern"},
		"additionalProperties": false,
	},
	"run_command": {
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{"type": "string"},
			"cwd":     map[string]any{"type": "string"},
		},
		"required":             []string{"command"},
		"additionalProperties": false,
	},
	"git_status": {"type": "object", "properties": map[string]any{}, "additionalProperties": false},
	"git_diff":   {"type": "object", "properties": map[string]any{}, "additionalProperties": false},
}
