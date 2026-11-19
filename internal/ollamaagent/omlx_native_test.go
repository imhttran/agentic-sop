package ollamaagent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/budget"
	"github.com/imhttran/agentic-sop/internal/redact"
	"github.com/imhttran/agentic-sop/internal/toolharness"
)

type nativeScriptedOMLX struct {
	mu        sync.Mutex
	responses []string
	requests  []openAIChatRequest
}

func newNativeScriptedOMLX(t *testing.T, responses ...string) (*nativeScriptedOMLX, *httptest.Server) {
	t.Helper()
	f := &nativeScriptedOMLX{responses: responses}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var req openAIChatRequest
		_ = json.Unmarshal(body, &req)
		f.mu.Lock()
		f.requests = append(f.requests, req)
		resp := ""
		if len(f.responses) > 0 {
			resp = f.responses[0]
			f.responses = f.responses[1:]
		}
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, resp)
	}))
	t.Cleanup(srv.Close)
	return f, srv
}

func nativeToolResponse(id, name string, args map[string]any) string {
	argsJSON, _ := json.Marshal(args)
	body := map[string]any{
		"choices": []any{map[string]any{
			"message": map[string]any{
				"role":    "assistant",
				"content": "",
				"tool_calls": []any{map[string]any{
					"id":   id,
					"type": "function",
					"function": map[string]any{
						"name":      name,
						"arguments": string(argsJSON),
					},
				}},
			},
			"finish_reason": "tool_calls",
		}},
	}
	data, _ := json.Marshal(body)
	return string(data)
}

func nativeFinalResponse(status, summary string, changesExpected bool) string {
	content, _ := json.Marshal(map[string]any{
		"status":           status,
		"summary":          summary,
		"changes_expected": changesExpected,
	})
	body := map[string]any{
		"choices": []any{map[string]any{
			"message":       map[string]any{"role": "assistant", "content": string(content)},
			"finish_reason": "stop",
		}},
	}
	data, _ := json.Marshal(body)
	return string(data)
}

func TestOMLXNativeToolsReadResultWrite(t *testing.T) {
	dir := t.TempDir()
	gitInit(t, dir)
	const marker = "fixture-marker\n"
	if err := os.WriteFile(filepath.Join(dir, "marker-input.txt"), []byte(marker), 0o600); err != nil {
		t.Fatal(err)
	}

	f, srv := newNativeScriptedOMLX(t,
		nativeToolResponse("call-read", "read_file", map[string]any{"path": "marker-input.txt"}),
		nativeToolResponse("call-write", "write_file", map[string]any{"path": "marker-output.txt", "content": marker}),
		nativeFinalResponse("completed", "copied", true),
	)
	cfg := omLXTestConfig(srv.URL)
	cfg.Model = "gpt-oss-20b-MXFP4-Q8"
	cfg.NativeTools = true
	cfg.MaxToolCalls = 50
	h := NewWithClient(cfg, dir, newOpenAIChatClient(cfg))
	ev := &mutationEvidence{}

	content, _, err := h.CompleteWithEvidence(context.Background(), implementRequest(), ev)
	if err != nil {
		t.Fatalf("CompleteWithEvidence failed: %v", err)
	}
	if !strings.Contains(content, `"status":"completed"`) {
		t.Fatalf("content = %q, want completed outcome", content)
	}
	got, err := os.ReadFile(filepath.Join(dir, "marker-output.txt"))
	if err != nil {
		t.Fatalf("marker-output.txt was not created: %v", err)
	}
	if string(got) != marker {
		t.Errorf("marker-output.txt = %q, want %q", string(got), marker)
	}
	if paths := ev.mutationPaths(); len(paths) != 1 || paths[0] != "marker-output.txt" {
		t.Errorf("mutation evidence = %v, want [marker-output.txt]", paths)
	}

	f.mu.Lock()
	requests := append([]openAIChatRequest(nil), f.requests...)
	f.mu.Unlock()
	if len(requests) != 3 {
		t.Fatalf("requests = %d, want 3 (read, write, final)", len(requests))
	}

	first := requests[0]
	if len(first.Tools) == 0 || first.ToolChoice != "auto" || first.ResponseFormat != nil {
		t.Errorf("first request tools=%d tool_choice=%q response_format=%v; want native tools, auto, nil", len(first.Tools), first.ToolChoice, first.ResponseFormat)
	}
	if len(first.Messages) == 0 || !strings.Contains(first.Messages[0].Content, "## Native tools") {
		t.Errorf("first request did not use the native system prompt")
	}
	if len(first.Messages) > 0 && strings.Contains(first.Messages[0].Content, `{"tool": "<name>"`) {
		t.Errorf("native request still carries the JSON-in-content tool instructions")
	}
	foundRead, foundWrite := false, false
	for _, tool := range first.Tools {
		if tool.Function.Name == toolharness.ToolReadFile {
			foundRead = true
			props, _ := tool.Function.Parameters["properties"].(map[string]any)
			path, _ := props["path"].(map[string]any)
			if path["type"] != "string" {
				t.Errorf("read_file path schema = %v, want string", path)
			}
		}
		if tool.Function.Name == toolharness.ToolWriteFile {
			foundWrite = true
		}
	}
	if !foundRead || !foundWrite {
		t.Errorf("native tools missing read_file/write_file: read=%v write=%v", foundRead, foundWrite)
	}

	if !hasToolResult(requests[1], "call-read", marker) {
		t.Errorf("second request did not return the read result with tool_call_id=call-read")
	}
	if got := toolMessageContent(requests[1], "call-read"); !strings.HasPrefix(got, `Tool "read_file" result:`) {
		t.Errorf("read tool result format = %q, want the harness tool-result prefix", got)
	}
	if !hasToolResult(requests[2], "call-write", "marker-output.txt") {
		t.Errorf("third request did not return the write result with tool_call_id=call-write")
	}
	if got := toolMessageContent(requests[2], "call-write"); !strings.HasPrefix(got, `Tool "write_file" result:`) {
		t.Errorf("write tool result format = %q, want the harness tool-result prefix", got)
	}

	audit := h.AuditRecords()
	readExecuted, writeExecuted := false, false
	for _, rec := range audit {
		if rec.Tool == toolharness.ToolReadFile && rec.Action == toolharness.ActionAllow && rec.Outcome == toolharness.OutcomeOK {
			readExecuted = true
		}
		if rec.Tool == toolharness.ToolWriteFile && rec.Action == toolharness.ActionAllow && rec.Outcome == toolharness.OutcomeOK {
			writeExecuted = true
		}
	}
	if !readExecuted || !writeExecuted {
		t.Errorf("audit did not record executed read_file and write_file: read=%v write=%v", readExecuted, writeExecuted)
	}
}

func hasToolResult(req openAIChatRequest, id, contains string) bool {
	for _, msg := range req.Messages {
		if msg.Role == "tool" && msg.ToolCallID == id && strings.Contains(msg.Content, contains) {
			return true
		}
	}
	return false
}

func toolMessageContent(req openAIChatRequest, id string) string {
	for _, msg := range req.Messages {
		if msg.Role == "tool" && msg.ToolCallID == id {
			return msg.Content
		}
	}
	return ""
}

func nativeCall(id, name string, args map[string]any) map[string]any {
	argsJSON, _ := json.Marshal(args)
	return map[string]any{
		"id":   id,
		"type": "function",
		"function": map[string]any{
			"name":      name,
			"arguments": string(argsJSON),
		},
	}
}

func nativeMultiToolResponse(calls ...map[string]any) string {
	toolCalls := make([]any, 0, len(calls))
	for _, call := range calls {
		toolCalls = append(toolCalls, call)
	}
	body := map[string]any{
		"choices": []any{map[string]any{
			"message": map[string]any{
				"role":       "assistant",
				"content":    "",
				"tool_calls": toolCalls,
			},
			"finish_reason": "tool_calls",
		}},
	}
	data, _ := json.Marshal(body)
	return string(data)
}

func TestOMLXNativeToolsRejectsMultipleCalls(t *testing.T) {
	dir := t.TempDir()
	_, srv := newNativeScriptedOMLX(t, nativeMultiToolResponse(
		nativeCall("call-read", "read_file", map[string]any{"path": "marker-input.txt"}),
		nativeCall("call-write", "write_file", map[string]any{"path": "marker-output.txt", "content": "x"}),
	))
	cfg := omLXTestConfig(srv.URL)
	cfg.NativeTools = true
	h := NewWithClient(cfg, dir, newOpenAIChatClient(cfg))
	_, err := h.Execute(context.Background(), implementRequest())
	if err == nil || !strings.Contains(err.Error(), "one tool call per turn") {
		t.Fatalf("err = %v, want a one-tool-per-turn failure", err)
	}
	if audit := h.AuditRecords(); len(audit) != 0 {
		t.Errorf("multi-call rejection executed tools: %+v", audit)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "marker-output.txt")); !os.IsNotExist(statErr) {
		t.Errorf("a rejected multi-call turn must not write marker-output.txt")
	}
}

type nonNativeClient struct{}

func (nonNativeClient) chat(context.Context, []chatMessage) (string, []toolCall, error) {
	return "", nil, nil
}

func TestNativeChatRequiresNativeTransport(t *testing.T) {
	cfg := omLXTestConfig("http://127.0.0.1:1")
	h := NewWithClient(cfg, t.TempDir(), nonNativeClient{})
	_, _, err := h.chatNative(context.Background(), nil, nil)
	if err == nil || !strings.Contains(err.Error(), "does not support") {
		t.Fatalf("err = %v, want a native-transport-required error", err)
	}
}

func TestOMLXNativeToolsRejectsMissingToolCallID(t *testing.T) {
	dir := t.TempDir()
	_, srv := newNativeScriptedOMLX(t, nativeToolResponse("", "read_file", map[string]any{"path": "marker-input.txt"}))
	cfg := omLXTestConfig(srv.URL)
	cfg.NativeTools = true
	h := NewWithClient(cfg, dir, newOpenAIChatClient(cfg))
	_, err := h.Execute(context.Background(), implementRequest())
	if err == nil || !strings.Contains(err.Error(), "missing a tool_call_id") {
		t.Fatalf("err = %v, want a missing-tool_call_id failure", err)
	}
}

func TestNativeToolSpecsPhaseGating(t *testing.T) {
	policy := policyFor(agent.Implement, budget.Defaults())
	if got := nativeToolSpecs(policy, implFinalize); len(got) != 0 {
		t.Errorf("FINALIZE declared %d tools, want 0", len(got))
	}
	specs := nativeToolSpecs(policy, implDiscover)
	names := map[string]bool{}
	for _, spec := range specs {
		names[spec.Name] = true
		if !policy.Allows(spec.Name) {
			t.Errorf("declared tool %q is not permitted by the policy", spec.Name)
		}
	}
	if !names[toolharness.ToolReadFile] || !names[toolharness.ToolWriteFile] {
		t.Errorf("native specs missing read_file/write_file: %v", names)
	}
}

// TestNativeOMLXEnvToggle proves SOP_MLX_NATIVE_TOOLS controls the opt-in and
// the default remains the JSON-in-content protocol.
func TestNativeOMLXEnvToggle(t *testing.T) {
	t.Setenv(agent.EnvAgentModel, "")
	t.Setenv(agent.EnvOllamaModel, "")
	t.Setenv(agent.EnvMLXModel, "m")
	t.Setenv(agent.EnvMLXNativeTools, "true")
	a, err := NativeOMLXAgentFromEnvWithPolicy("configured", false, redact.Policy{})
	if err != nil {
		t.Fatal(err)
	}
	if !a.cfg.NativeTools {
		t.Error("NativeTools = false, want true when SOP_MLX_NATIVE_TOOLS=true")
	}
	t.Setenv(agent.EnvMLXNativeTools, "false")
	b, err := NativeOMLXAgentFromEnvWithPolicy("configured", false, redact.Policy{})
	if err != nil {
		t.Fatal(err)
	}
	if b.cfg.NativeTools {
		t.Error("NativeTools = true, want false when SOP_MLX_NATIVE_TOOLS=false")
	}
}

func TestNativeToolSchemasCoverHarness(t *testing.T) {
	for _, name := range toolharness.Tools() {
		if _, ok := nativeToolSchemas[name]; !ok {
			t.Errorf("no native schema for permitted tool %q", name)
		}
	}
}
