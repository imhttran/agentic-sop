package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/mcp"
)

// serveMCP builds a server over tools and feeds input, returning decoded
// responses. It exercises the real stdio server path (initialize, tools/list,
// tools/call) without spawning a process.
func serveMCP(t *testing.T, tools []mcp.Tool, input string) []map[string]any {
	t.Helper()
	var out bytes.Buffer
	srv := mcp.NewServer(Version, tools...)
	if err := srv.Serve(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	var resps []map[string]any
	dec := json.NewDecoder(&out)
	for {
		var m map[string]any
		if err := dec.Decode(&m); err != nil {
			break
		}
		resps = append(resps, m)
	}
	return resps
}

func toolResultText(t *testing.T, resp map[string]any) string {
	t.Helper()
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("response has no result: %v", resp)
	}
	content, ok := result["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatalf("result has no content: %v", result)
	}
	return content[0].(map[string]any)["text"].(string)
}

// TestMCPStatusOnlyRegistration proves status-only mode registers exactly one
// tool and that the full mode is unchanged.
func TestMCPStatusOnlyRegistration(t *testing.T) {
	dir := t.TempDir()

	statusTools := mcpToolsWithMode(dir, deps{}, true)
	if len(statusTools) != 1 || statusTools[0].Name != "sop_status" {
		names := make([]string, 0, len(statusTools))
		for _, tl := range statusTools {
			names = append(names, tl.Name)
		}
		t.Fatalf("status-only tools = %v, want [sop_status]", names)
	}
	if !statusTools[0].ReadOnly {
		t.Errorf("sop_status is not marked read-only in status-only mode")
	}

	full := mcpTools(dir, deps{})
	got := map[string]bool{}
	for _, tl := range full {
		got[tl.Name] = true
	}
	want := []string{"sop_status", "sop_validate", "sop_review", "sop_approval", "sop_approve", "sop_decline"}
	if len(full) != len(want) {
		t.Errorf("full mode has %d tools, want %d", len(full), len(want))
	}
	for _, name := range want {
		if !got[name] {
			t.Errorf("full mode missing tool %s (existing behavior must be preserved)", name)
		}
	}
	// Only sop_status is read-only; the approval actions never are.
	for _, tl := range full {
		if tl.Name == "sop_status" && !tl.ReadOnly {
			t.Errorf("sop_status should be read-only in full mode too")
		}
		if (tl.Name == "sop_approve" || tl.Name == "sop_decline") && tl.ReadOnly {
			t.Errorf("%s must not be marked read-only", tl.Name)
		}
	}
}

// TestMCPStatusOnlyServerSurface exercises initialize, tools/list, status
// retrieval, and rejection of excluded tools against disposable state.
func TestMCPStatusOnlyServerSurface(t *testing.T) {
	dir := t.TempDir()
	initProject(t, dir)
	seedTask(t, dir, &domain.Task{ID: "S001", Title: "Baseline inventory", Status: domain.PLANNED, MaxAttempts: 3})

	tools := mcpToolsWithMode(dir, deps{}, true)
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize"}
{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":2,"method":"tools/list"}
{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"sop_status","arguments":{}}}
{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"sop_approve","arguments":{"taskId":"S001"}}}
{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"sop_decline","arguments":{"taskId":"S001"}}}
`
	resps := serveMCP(t, tools, input)
	if len(resps) != 5 {
		t.Fatalf("got %d responses, want 5 (the notification produces none)", len(resps))
	}

	// 1. initialize.
	init := resps[0]["result"].(map[string]any)
	if init["protocolVersion"] != mcp.ProtocolVersion {
		t.Errorf("protocolVersion = %v, want %v", init["protocolVersion"], mcp.ProtocolVersion)
	}
	if info := init["serverInfo"].(map[string]any); info["name"] != mcp.ServerName {
		t.Errorf("serverInfo.name = %v, want %v", info["name"], mcp.ServerName)
	}

	// 2. tools/list: only sop_status, and it carries readOnlyHint: true.
	list := resps[1]["result"].(map[string]any)["tools"].([]any)
	if len(list) != 1 {
		t.Fatalf("tools/list returned %d tools, want 1", len(list))
	}
	only := list[0].(map[string]any)
	if only["name"] != "sop_status" {
		t.Errorf("only tool = %v, want sop_status", only["name"])
	}
	ann, ok := only["annotations"].(map[string]any)
	if !ok || ann["readOnlyHint"] != true {
		t.Errorf("sop_status annotations = %v, want readOnlyHint:true", only["annotations"])
	}

	// 3. status retrieval from disposable state.
	statusResult := resps[2]["result"].(map[string]any)
	if statusResult["isError"] != false {
		t.Errorf("sop_status isError = %v, want false", statusResult["isError"])
	}
	if text := toolResultText(t, resps[2]); !strings.Contains(text, "S001") {
		t.Errorf("sop_status text = %q, want it to include the seeded task", text)
	}

	// 4./5. excluded approval tools are rejected by tools/call (absent server-side).
	for i, name := range []string{"sop_approve", "sop_decline"} {
		resp := resps[3+i]
		if _, isResult := resp["result"]; isResult {
			t.Fatalf("tools/call %s returned a result, want a JSON-RPC error: %v", name, resp)
		}
		rpcErr, ok := resp["error"].(map[string]any)
		if !ok {
			t.Fatalf("tools/call %s has no error: %v", name, resp)
		}
		if rpcErr["code"].(float64) != -32602 {
			t.Errorf("tools/call %s error code = %v, want -32602", name, rpcErr["code"])
		}
		if msg := rpcErr["message"].(string); !strings.Contains(msg, "unknown tool") || !strings.Contains(msg, name) {
			t.Errorf("tools/call %s error = %q, want unknown tool", name, msg)
		}
	}
}

// TestMCPStatusOnlyWithoutState proves status retrieval is read-only: it reports
// an uninitialized project instead of creating state.
func TestMCPStatusOnlyWithoutState(t *testing.T) {
	dir := t.TempDir() // disposable, no SOP state

	resps := serveMCP(t, mcpToolsWithMode(dir, deps{}, true),
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"sop_status","arguments":{}}}`+"\n")
	if len(resps) != 1 {
		t.Fatalf("got %d responses, want 1", len(resps))
	}
	if text := toolResultText(t, resps[0]); text != notInitializedMsg {
		t.Errorf("sop_status text = %q, want %q", text, notInitializedMsg)
	}
}

// TestMCPUsageRejectsUnknownFlag pins the flag surface of the command.
func TestMCPUsageRejectsUnknownFlag(t *testing.T) {
	var out, errOut bytes.Buffer
	if code := runMCP([]string{"--nope"}, &out, &errOut, deps{}); code != exitUsage {
		t.Errorf("code = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(errOut.String(), "usage: sop mcp") || !strings.Contains(errOut.String(), "--status-only") {
		t.Errorf("stderr = %q, want the usage line with --status-only", errOut.String())
	}
}
