// Package normalize is the CTX-006 Response Normalizer: the deterministic boundary
// between provider output and harness interpretation.
//
// It normalizes only what is safe and deterministic — surrounding whitespace, a
// single known code-fence wrapper, and the empty response — and it never repairs a
// semantic mistake with a hidden heuristic. Content that is intended as structured
// JSON but is malformed or ambiguous fails safely: the caller sees an error and the
// raw response is preserved for diagnostics. It is pure and model-free, so identical
// input yields identical output.
//
// Normalization is structural only. The normalizer does not decide the meaning of a
// response and never turns a failure into a success; it is not a second decision
// engine.
package normalize

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Kind classifies a normalized response.
type Kind string

const (
	// Empty is a response with no non-whitespace content.
	KindEmpty Kind = "empty"
	// JSON is a response whose content is a single valid JSON document (after any
	// known wrapper is removed).
	KindJSON Kind = "json"
	// Text is a response that is neither empty nor intended as JSON.
	KindText Kind = "text"
	// Malformed is a response whose content is intended as JSON (it begins with an
	// object or array) but does not parse. It is a fail-safe classification.
	KindMalformed Kind = "malformed"
)

// Result is the outcome of normalizing one raw response.
type Result struct {
	// Kind is the classification.
	Kind Kind
	// Content is the normalized content: whitespace trimmed, and a single known code
	// fence removed when it wrapped the whole response.
	Content string
	// Raw is the original response, preserved verbatim for diagnostics.
	Raw string
	// Wrappers names the deterministic wrappers removed, in order.
	Wrappers []string
	// Err is set only for a Malformed response.
	Err error
}

const wrapperCodeFence = "code-fence"

// Response normalizes raw provider output. It is pure and total: every input
// produces a Result, and malformed structured output yields a Malformed result with
// a non-nil Err rather than a repaired value.
func Response(raw string) Result {
	res := Result{Raw: raw}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		res.Kind = KindEmpty
		return res
	}

	content := trimmed
	if inner, ok := unwrapCodeFence(trimmed); ok {
		content = inner
		res.Wrappers = append(res.Wrappers, wrapperCodeFence)
	}
	res.Content = content
	if content == "" {
		res.Kind = KindEmpty
		return res
	}

	switch content[0] {
	case '{', '[':
		if json.Valid([]byte(content)) {
			res.Kind = KindJSON
			return res
		}
		res.Kind = KindMalformed
		res.Err = fmt.Errorf("normalize: response is not valid JSON")
		return res
	default:
		res.Kind = KindText
		return res
	}
}

// JSON extracts a single JSON document from a response. It removes one known code
// fence and trims whitespace, then requires the content to parse as JSON. It never
// repairs malformed content, so an ambiguous or invalid response fails closed.
func JSON(raw string) ([]byte, error) {
	res := Response(raw)
	switch res.Kind {
	case KindJSON:
		return []byte(res.Content), nil
	case KindEmpty:
		return nil, fmt.Errorf("normalize: empty response")
	case KindMalformed:
		return nil, res.Err
	default:
		return nil, fmt.Errorf("normalize: response is not JSON")
	}
}

// unwrapCodeFence removes a single fenced code block when it wraps the entire
// trimmed response. A fence is a line of exactly three backticks or tildes, an
// optional single language token, the body, and a matching closing fence. Content
// with prose before or after the fence is left unchanged: it is ambiguous, and
// guessing at it would be a heuristic.
func unwrapCodeFence(s string) (string, bool) {
	nl := strings.IndexByte(s, '\n')
	if nl < 0 {
		return "", false
	}
	open := strings.TrimRight(s[:nl], " \t\r")
	if !fenceOpen(open) {
		return "", false
	}
	body := strings.TrimRight(s[nl+1:], " \t\r\n")
	if body == "" {
		return "", false
	}
	lastNL := strings.LastIndexByte(body, '\n')
	var inner, close string
	if lastNL >= 0 {
		inner = body[:lastNL]
		close = body[lastNL+1:]
	} else {
		close = body
	}
	if !fenceClose(close, open[0]) {
		return "", false
	}
	return strings.TrimSpace(inner), true
}

// fenceOpen reports whether line opens a fence: exactly three backticks or tildes,
// optionally followed by a single language token with no embedded whitespace.
func fenceOpen(line string) bool {
	if len(line) < 3 || (line[0] != '`' && line[0] != '~') {
		return false
	}
	if line[:3] != strings.Repeat(string(line[0]), 3) {
		return false
	}
	rest := line[3:]
	return rest == "" || !strings.ContainsAny(rest, " \t")
}

// fenceClose reports whether line is a closing fence matching the opening marker.
func fenceClose(line string, marker byte) bool {
	return line == strings.Repeat(string(marker), 3)
}
