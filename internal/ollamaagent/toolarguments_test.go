package ollamaagent

import (
	"encoding/json"
	"testing"
)

// TestToolArgumentsDecodesEveryWrapperShape pins the argument-decoding contract.
// A wrapper (the prompt's {"tool":...,"args":...} shape) may carry the real
// argument object either as an object or as a JSON string, and both must decode to
// the same arguments. Left wrapped, the call loses every argument and fails with
// "missing required argument", which reads to the model as a correct call the
// harness refuses — the CLOSE-004 failure mode, where a successful file write was
// never attempted because its path never reached the tool.
func TestToolArgumentsDecodesEveryWrapperShape(t *testing.T) {
	for _, tc := range []struct{ name, raw string }{
		{"flat object", `{"path":"a.go","content":"c"}`},
		{"top-level json string", `"{\"path\":\"a.go\",\"content\":\"c\"}"`},
		{"args object", `{"args":{"path":"a.go","content":"c"}}`},
		{"args json string", `{"args":"{\"path\":\"a.go\",\"content\":\"c\"}"}`},
		{"tool plus args object", `{"tool":"create_file","args":{"path":"a.go","content":"c"}}`},
		{"tool plus args json string", `{"tool":"create_file","args":"{\"path\":\"a.go\",\"content\":\"c\"}"}`},
		{"args json string with raw newlines", `{"args":"{\n\"path\":\"a.go\",\n\"content\":\"line1\nline2\"\n}"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := toolArguments(json.RawMessage(tc.raw))
			if args["path"] != "a.go" {
				t.Fatalf("toolArguments(%s) = %v, want the path decoded", tc.raw, args)
			}
			if _, ok := args["content"]; !ok {
				t.Fatalf("toolArguments(%s) = %v, want the content decoded", tc.raw, args)
			}
		})
	}
}

// TestToolArgumentsKeepsUndecodableArgs is the conservative counterpart: a value
// that is not a JSON object is preserved as-is rather than guessed at, so an
// unrecognized shape can never be silently turned into fabricated arguments.
func TestToolArgumentsKeepsUndecodableArgs(t *testing.T) {
	args := toolArguments(json.RawMessage(`{"args":"not json"}`))
	if args["args"] != "not json" {
		t.Fatalf("toolArguments = %v, want the undecodable value preserved", args)
	}
}
