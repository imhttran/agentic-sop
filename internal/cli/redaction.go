package cli

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/imhttran/agentic-sop/internal/handoff"
	"github.com/imhttran/agentic-sop/internal/redact"
)

// redactDiagnosticText removes known environment secret values and
// credential-shaped text from a diagnostic before it is sent to a model or
// persisted as evidence. It reuses the shared detector so a shape one surface
// learns is removed here too; the marker stays the established [redacted] text.
//
// It is deliberately scoped to diagnostics and command output. It does not
// rewrite the repository diff or implementation report: those are evidence, and
// altering them could hide what actually happened.
func redactDiagnosticText(s string) string {
	if s == "" {
		return s
	}
	s = handoff.Redact(s, os.Environ())
	return redact.ShapesValues(s, redact.Fixed("[redacted]"))
}

// redactAcceptanceOutput redacts captured acceptance-check output without
// corrupting structured lines. A valid JSON line is decoded, every string value
// is redacted, and the line is re-encoded; a non-JSON line is redacted as plain
// text.
//
// Decoding first matters for `go test -json`: the event's Output field is a JSON
// string, so a quoted credential appears escaped (password=\"value\"). Redacting
// the escaped text would replace only the backslash before the quote and leave
// the value exposed, and would also produce invalid JSON. The caller keeps the
// original captured output separately for structured acceptance decisions.
func redactAcceptanceOutput(raw string) string {
	if raw == "" {
		return raw
	}
	lines := strings.Split(raw, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if redacted, ok := redactJSONLine(line); ok {
			lines[i] = redacted
			continue
		}
		lines[i] = redactDiagnosticText(line)
	}
	return strings.Join(lines, "\n")
}

// redactJSONLine decodes one JSON line, redacts every string value, and
// re-encodes it. A non-JSON line is reported as not handled so the caller can
// apply plain-text redaction.
func redactJSONLine(line string) (string, bool) {
	var value any
	if err := json.Unmarshal([]byte(line), &value); err != nil {
		return "", false
	}
	data, err := json.Marshal(redactJSONValue(value))
	if err != nil {
		return "", false
	}
	return string(data), true
}

// redactJSONValue walks a decoded JSON value and redacts every string, including
// the Output field of a `go test -json` event, while preserving the structure.
func redactJSONValue(value any) any {
	switch v := value.(type) {
	case string:
		return redactDiagnosticText(v)
	case []any:
		for i := range v {
			v[i] = redactJSONValue(v[i])
		}
		return v
	case map[string]any:
		for key, child := range v {
			v[key] = redactJSONValue(child)
		}
		return v
	default:
		return v
	}
}
