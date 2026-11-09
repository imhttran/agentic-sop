package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// TestReadFrameBoundary covers just below, at, and above the frame bound.
func TestReadFrameBoundary(t *testing.T) {
	const max = 8
	cases := []struct {
		name  string
		input string
		ok    bool
	}{
		{"below", "1234567\n", true},
		{"at", "12345678\n", true},
		{"above", "123456789\n", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			frame, err := readFrame(bufio.NewReader(strings.NewReader(tc.input)), max)
			if tc.ok {
				if err != nil {
					t.Fatalf("readFrame: %v", err)
				}
				if string(frame) != strings.TrimSpace(tc.input) {
					t.Errorf("frame = %q", frame)
				}
				return
			}
			if err == nil {
				t.Fatalf("readFrame accepted %q above the bound", tc.input)
			}
		})
	}
}

// TestServeRejectsOversizedFrameAndContinues proves an oversized frame is
// answered once with a parse error and a later valid request still runs.
func TestServeRejectsOversizedFrameAndContinues(t *testing.T) {
	input := strings.Repeat("x", MaxFrameSize+1) + "\n" +
		`{"jsonrpc":"2.0","id":1,"method":"initialize"}` + "\n"

	var out bytes.Buffer
	if err := testServer().Serve(context.Background(), strings.NewReader(input), &out); err != nil {
		t.Fatalf("Serve: %v", err)
	}

	dec := json.NewDecoder(&out)
	var first, second map[string]any
	if err := dec.Decode(&first); err != nil {
		t.Fatalf("decode first response: %v", err)
	}
	if err := dec.Decode(&second); err != nil {
		t.Fatalf("decode second response: %v", err)
	}
	errObj, _ := first["error"].(map[string]any)
	if errObj == nil || errObj["code"] != float64(codeParseError) {
		t.Fatalf("first response = %+v, want parse error", first)
	}
	if second["result"] == nil {
		t.Fatalf("second response = %+v, want the initialize result", second)
	}
}
