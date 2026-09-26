package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func testServer() *Server {
	echo := Tool{
		Name:        "echo",
		Description: "returns hello",
		Handler: func(context.Context, json.RawMessage) (string, error) {
			return "hello", nil
		},
	}
	boom := Tool{
		Name:        "boom",
		Description: "always fails",
		Handler: func(context.Context, json.RawMessage) (string, error) {
			return "", errors.New("tool exploded")
		},
	}
	return NewServer("test", echo, boom)
}

// run feeds input to the server and returns the decoded responses.
func run(t *testing.T, s *Server, input string) []map[string]any {
	t.Helper()
	var out bytes.Buffer
	if err := s.Serve(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("Serve failed: %v", err)
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

func TestInitializeAndLists(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize"}
{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":2,"method":"tools/list"}
`
	resps := run(t, testServer(), input)
	if len(resps) != 2 {
		t.Fatalf("got %d responses, want 2 (notification produces none)", len(resps))
	}

	init := resps[0]["result"].(map[string]any)
	info := init["serverInfo"].(map[string]any)
	if info["name"] != ServerName {
		t.Errorf("serverInfo.name = %v", info["name"])
	}
	if init["protocolVersion"] != ProtocolVersion {
		t.Errorf("protocolVersion = %v", init["protocolVersion"])
	}

	result := resps[1]["result"].(map[string]any)
	tools := result["tools"].([]any)
	if len(tools) != 2 {
		t.Fatalf("tools = %v, want 2", tools)
	}
	first := tools[0].(map[string]any)
	if first["name"] != "echo" || first["description"] != "returns hello" {
		t.Errorf("tool[0] = %v", first)
	}
}

func TestCallToolSuccessAndError(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"echo","arguments":{}}}
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"boom","arguments":{}}}
`
	resps := run(t, testServer(), input)
	if len(resps) != 2 {
		t.Fatalf("got %d responses, want 2", len(resps))
	}

	okResult := resps[0]["result"].(map[string]any)
	if okResult["isError"] != false {
		t.Errorf("isError = %v, want false", okResult["isError"])
	}
	content := okResult["content"].([]any)[0].(map[string]any)
	if content["text"] != "hello" {
		t.Errorf("text = %v, want hello", content["text"])
	}

	errResult := resps[1]["result"].(map[string]any)
	if errResult["isError"] != true {
		t.Errorf("isError = %v, want true", errResult["isError"])
	}
	errText := errResult["content"].([]any)[0].(map[string]any)["text"]
	if !strings.Contains(errText.(string), "tool exploded") {
		t.Errorf("error text = %v", errText)
	}
}

func TestUnknownMethodAndTool(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"does/not/exist"}
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"nope","arguments":{}}}
`
	resps := run(t, testServer(), input)
	if len(resps) != 2 {
		t.Fatalf("got %d responses, want 2", len(resps))
	}

	mErr := resps[0]["error"].(map[string]any)
	if mErr["code"].(float64) != codeMethodNotFound {
		t.Errorf("method error code = %v", mErr["code"])
	}
	tErr := resps[1]["error"].(map[string]any)
	if tErr["code"].(float64) != codeInvalidParams {
		t.Errorf("tool error code = %v", tErr["code"])
	}
}

func TestTextToolIgnoresArguments(t *testing.T) {
	s := NewServer("test", TextTool("status", "list tasks", func(context.Context) (string, error) {
		return "T001 DONE", nil
	}))
	resps := run(t, s, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"status","arguments":{"x":1}}}`+"\n")
	if len(resps) != 1 {
		t.Fatalf("got %d responses, want 1", len(resps))
	}
	text := resps[0]["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"]
	if text != "T001 DONE" {
		t.Errorf("text = %v", text)
	}
}
