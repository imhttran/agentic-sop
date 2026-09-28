package deepseekagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// Run reads one SOP command-agent request from in, executes it against the
// configured Ollama model, and writes the response content to out.
//
// A harness failure is reported as a structured "failed" outcome for a
// capability that expects one (IMPLEMENT, FIX, DESIGN_TESTS, DIAGNOSE_FAILURE).
// For PLAN and REVIEW, which expect their own JSON schema, it returns an error so
// SOP records an agent failure instead of a malformed document.
func Run(ctx context.Context, in io.Reader, out, errOut io.Writer, getwd func() (string, error)) error {
	var req agent.Request
	if err := json.NewDecoder(in).Decode(&req); err != nil {
		return fmt.Errorf("parse request: %w", err)
	}
	if err := req.Validate(); err != nil {
		return err
	}

	cfg, err := ConfigFromEnv()
	if err != nil {
		return err
	}
	root, err := getwd()
	if err != nil {
		return fmt.Errorf("determine repository root: %w", err)
	}

	content, err := New(cfg, root).Execute(ctx, req)
	if err != nil {
		if wantsOutcome(req.Capability) {
			fmt.Fprintf(errOut, "sop-deepseek-agent: %v\n", err)
			return writeJSON(out, outcomeWire{Status: string(agent.OutcomeFailed), Reason: err.Error()})
		}
		return err
	}
	if _, err := io.WriteString(out, content); err != nil {
		return err
	}
	return nil
}

// wantsOutcome reports whether a capability's expected response is a structured
// execution outcome rather than a schema document (PLAN, REVIEW).
func wantsOutcome(c agent.Capability) bool {
	switch c {
	case agent.Plan, agent.Review:
		return false
	default:
		return true
	}
}

// outcomeWire is the command-agent outcome as SOP's command provider parses it.
// The field tags match that wire format exactly.
type outcomeWire struct {
	Status          string `json:"status"`
	Summary         string `json:"summary,omitempty"`
	Reason          string `json:"reason,omitempty"`
	ChangesExpected *bool  `json:"changes_expected,omitempty"`
}

// Harness executes one SOP request against Ollama using controlled tools.
type Harness struct {
	cfg    Config
	client *ollamaClient
	tools  *toolbox
}

// New returns a Harness working inside root.
func New(cfg Config, root string) *Harness {
	return &Harness{
		cfg:    cfg,
		client: newOllamaClient(cfg),
		tools:  newToolbox(root, cfg),
	}
}

// Execute runs the bounded tool loop and returns the model's final JSON object as
// canonical JSON, ready for SOP to parse.
func (h *Harness) Execute(ctx context.Context, req agent.Request) (string, error) {
	messages := []chatMessage{
		{Role: "system", Content: systemPrompt(req)},
		{Role: "user", Content: userPrompt(req)},
	}

	toolCalls := 0
	for iteration := 0; iteration < h.cfg.MaxIterations; iteration++ {
		raw, err := h.chat(ctx, messages)
		if err != nil {
			return "", err
		}

		obj, err := firstJSONObject(raw)
		if err != nil {
			return "", fmt.Errorf("malformed model response: %w", err)
		}

		name, args, isTool, err := asToolCall(obj)
		if err != nil {
			return "", err
		}
		if !isTool {
			encoded, err := json.Marshal(obj)
			if err != nil {
				return "", fmt.Errorf("encode final response: %w", err)
			}
			return string(encoded), nil
		}

		if toolCalls >= h.cfg.MaxToolCalls {
			return "", fmt.Errorf("tool-call limit reached (%d tool calls); the model did not finish", h.cfg.MaxToolCalls)
		}
		toolCalls++

		result, toolErr := h.tools.run(ctx, name, args)
		messages = append(messages,
			chatMessage{Role: "assistant", Content: raw},
			chatMessage{Role: "user", Content: toolResultMessage(name, result, toolErr)},
		)
	}
	return "", fmt.Errorf("iteration limit reached (%d); the model did not finish", h.cfg.MaxIterations)
}

// maxEmptyRetries bounds how many times an empty model turn is re-requested
// before it is a failure.
const maxEmptyRetries = 2

// chat asks the model for the next turn, re-requesting a bounded number of times
// when the model returns no content. Other errors are returned immediately.
func (h *Harness) chat(ctx context.Context, messages []chatMessage) (string, error) {
	var err error
	for attempt := 0; attempt <= maxEmptyRetries; attempt++ {
		var raw string
		raw, err = h.client.chat(ctx, messages)
		if err == nil {
			return raw, nil
		}
		if !errors.Is(err, errEmptyResponse) {
			return "", err
		}
	}
	return "", err
}

// asToolCall reports whether obj is a tool call and validates its shape. A
// present-but-malformed "tool" field is an explicit error, never a silent final
// answer.
func asToolCall(obj map[string]any) (name string, args map[string]any, isTool bool, err error) {
	raw, present := obj["tool"]
	if !present {
		return "", nil, false, nil
	}
	s, ok := raw.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return "", nil, false, errors.New(`malformed tool request: "tool" must be a non-empty string`)
	}
	args = map[string]any{}
	if v, present := obj["args"]; present {
		m, ok := v.(map[string]any)
		if !ok {
			return "", nil, false, errors.New(`malformed tool request: "args" must be an object`)
		}
		args = m
	}
	return strings.TrimSpace(s), args, true, nil
}

// firstJSONObject extracts the first balanced JSON object from s. With format
// "json" the model emits a bare object, but a stray fence or trailing text is
// tolerated rather than trusted.
func firstJSONObject(s string) (map[string]any, error) {
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return nil, errors.New("no JSON object in response")
	}
	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(s); i++ {
		c := s[i]
		switch {
		case escaped:
			escaped = false
		case inString && c == '\\':
			escaped = true
		case c == '"':
			inString = !inString
		case inString:
			// skip string content
		case c == '{':
			depth++
		case c == '}':
			depth--
			if depth == 0 {
				var obj map[string]any
				if err := json.Unmarshal([]byte(s[start:i+1]), &obj); err != nil {
					return nil, err
				}
				return obj, nil
			}
		}
	}
	return nil, errors.New("unterminated JSON object in response")
}

// writeJSON writes v as a single JSON line.
func writeJSON(w io.Writer, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = w.Write(append(data, '\n'))
	return err
}

// truncate bounds s to at most n bytes, marking the cut.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + fmt.Sprintf("\n… [truncated %d bytes]", len(s)-n)
}
