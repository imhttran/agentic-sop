package ollamaagent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// The one-object-per-turn wire protocol: each model turn is either a tool call
// ({"tool": ..., "args": {...}}), expressed in content or as a native Ollama tool
// call, or the capability's final JSON object. Turn interpretation, progress
// detection, and JSON extraction live here, shared by every capability's runner.

// errNarrate marks a model turn that carried no tool call and no final object
// (prose only); the loop nudges the model rather than failing.
var errNarrate = errors.New("model turn was narration, not a tool call or a final object")

// turnToolCall interprets one model turn. It returns a tool call when the turn
// is one, or the final JSON object (a document or an outcome) when it is not.
// Content is preferred over native tool calls so a final answer is never lost;
// a turn with neither returns errNarrate.
func turnToolCall(raw string, calls []toolCall) (name string, args map[string]any, isTool bool, final map[string]any, err error) {
	obj, objErr := firstJSONObject(raw)
	switch {
	case objErr == nil:
		n, a, t, aerr := asToolCall(obj)
		if aerr != nil {
			return "", nil, false, nil, aerr
		}
		if !t {
			return "", nil, false, obj, nil
		}
		return n, a, true, nil, nil
	case len(calls) > 0:
		return calls[0].Name, calls[0].Args, true, nil, nil
	default:
		return "", nil, false, nil, errNarrate
	}
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

// firstJSONObject extracts the JSON object at the first "{" in s. With format
// "json" the model emits a bare object, but leading prose is tolerated rather
// than trusted: the object begins at the first brace. If that brace does not open
// a decodable object, the turn has no usable JSON.
func firstJSONObject(s string) (map[string]any, error) {
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return nil, errors.New("no JSON object in response")
	}
	if obj, ok := objectAt(s, start); ok {
		return obj, nil
	}
	return nil, errors.New("no JSON object in response")
}

// objectAt parses the balanced JSON object beginning at the "{" at index start,
// reporting whether one was found and decoded.
func objectAt(s string, start int) (map[string]any, bool) {
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
				if err := json.Unmarshal([]byte(escapeControlChars(s[start:i+1])), &obj); err != nil {
					return nil, false
				}
				return obj, true
			}
		}
	}
	return nil, false
}

// escapeControlChars escapes raw control characters that appear inside JSON
// strings. A model routinely writes a multi-line string value (a file body) with
// literal newlines and tabs, which strict JSON rejects; escaping them keeps the
// model's intent without a lenient parser.
func escapeControlChars(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 16)
	inString := false
	escaped := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case escaped:
			escaped = false
			b.WriteByte(c)
		case inString && c == '\\':
			b.WriteByte(c)
		case c == '"':
			inString = !inString
			b.WriteByte(c)
		case inString && c < 0x20:
			switch c {
			case '\n':
				b.WriteString(`\n`)
			case '\r':
				b.WriteString(`\r`)
			case '\t':
				b.WriteString(`\t`)
			case '\b':
				b.WriteString(`\b`)
			case '\f':
				b.WriteString(`\f`)
			default:
				fmt.Fprintf(&b, `\u%04x`, c)
			}
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// assistantEcho is the assistant message echoed back before a tool result. It is
// the model's own content when it expressed the call there, or a synthesized
// tool-call object when the call came from Ollama's native tool_calls, so the
// conversation stays in the JSON protocol the model was prompted with.
func assistantEcho(name string, args map[string]any, raw string) string {
	if strings.TrimSpace(raw) != "" {
		return raw
	}
	b, err := json.Marshal(map[string]any{"tool": name, "args": args})
	if err != nil {
		return fmt.Sprintf(`{"tool": %q}`, name)
	}
	return string(b)
}

// noProgressThreshold is how many genuinely identical consecutive turns are
// tolerated before the model is told to stop repeating itself; one more
// repetition after that instruction terminates the run early.
const noProgressThreshold = 3

// Termination reasons, reported in both the error and the trace.
const (
	terminationIteration    = "iteration_limit"
	terminationNoProgress   = "no_progress"
	terminationSynthesis    = "synthesis_limit"
	terminationFinalization = "finalization_limit"
)

// turnProgress tracks consecutive identical turns so the loop can tell a
// productive exploration from a model stuck repeating itself. State is scoped to
// a single invocation.
type turnProgress struct {
	prev     string
	repeated int
	recovery bool
}

// observe folds one turn's fingerprint into the run-length state. A fingerprint
// that differs from the previous turn is progress and resets the run length. It
// returns whether the recovery instruction should be sent this turn, and whether
// the model kept repeating itself after that instruction and must be stopped.
func (p *turnProgress) observe(fp string) (sendRecovery, terminate bool) {
	if fp != "" && fp == p.prev {
		p.repeated++
	} else {
		p.repeated = 1
	}
	p.prev = fp
	if p.repeated < noProgressThreshold {
		return false, false
	}
	if p.recovery {
		return false, true
	}
	p.recovery = true
	return true, false
}

// label reports the progress word for a trace entry.
func (p *turnProgress) label() string {
	if p.repeated > 1 {
		return progressRepeat
	}
	return progressOK
}

// actionFingerprint is a deterministic identity for one executed tool action. It
// includes the result, so a repeated action that now returns different content (a
// file changed, a test started passing) is treated as progress rather than a
// stuck loop. It hashes arguments and result, so the trace never carries content.
func actionFingerprint(name string, args map[string]any, result string, err error) string {
	sum := sha256.New()
	io.WriteString(sum, name)
	sum.Write([]byte{0})
	// encoding/json sorts map keys, so the argument encoding is deterministic.
	if b, merr := json.Marshal(args); merr == nil {
		sum.Write(b)
	}
	sum.Write([]byte{0})
	if err != nil {
		io.WriteString(sum, "error:"+err.Error())
	} else {
		io.WriteString(sum, "ok:"+result)
	}
	return hex.EncodeToString(sum.Sum(nil))
}

// narrationFingerprint identifies a narration turn (no tool and no final object)
// for progress detection.
func narrationFingerprint(c agent.Capability) string { return "narrate:" + string(c) }

// turnReminder is sent when a model turn is neither a tool call nor a final
// object, so the model continues in the one-object-per-turn protocol instead of
// failing the task.
const turnReminder = `Your reply contained no JSON object. Reply now with exactly one JSON object and no prose: either a tool call {"tool": "<name>", "args": {...}} or the required final object.`

// progressReminder is injected once when the model is detected repeating
// non-progressing turns: it tells the model to conclude with what it already has
// instead of continuing optional exploration.
const progressReminder = `You are repeating actions without making progress.

Use the information already gathered.
Do not perform additional optional exploration.
Complete the requested capability now and return the required final response.

SOP will independently validate the repository after you finish.`

// nudgeText is the user message sent after a narration turn: normally a reminder
// to reply in the one-object protocol; when recovery is warranted, the stronger
// instruction to stop repeating and finish.
func nudgeText(recovery bool) string {
	if recovery {
		return progressReminder
	}
	return turnReminder
}

// recoverySuffix appends the recovery instruction to a tool result, so it stays in
// the same user turn rather than producing two consecutive user messages.
func recoverySuffix(recovery bool) string {
	if !recovery {
		return ""
	}
	return "\n\n" + progressReminder
}

// actionSuffix renders the last action for a diagnostic, or "" when none ran.
func actionSuffix(tool, request string) string {
	if tool == "" {
		return ""
	}
	return fmt.Sprintf(", last_action=%q", strings.TrimSpace(tool+" "+request))
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
