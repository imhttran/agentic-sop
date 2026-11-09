package cli

import (
	"io"
	"strings"
	"unicode"
	"unicode/utf8"
)

// terminalSink is the operator's console: the CLI's stdout and stderr when a
// model, a tool, or a provider supplies the text. It filters control characters
// so untrusted output cannot drive the terminal.
//
// This is a display-sink filter only. It never touches persisted artifacts,
// run reports, state, or machine-readable output: JSON encoders already escape
// control characters, so a JSON document that passes through is unchanged, while
// a raw ESC sequence written to the console is removed.
type terminalSink struct {
	w       io.Writer
	pending []byte
}

// newTerminalSink wraps w with the terminal filter. An already-wrapped writer is
// returned unchanged, so the filter is applied at most once.
func newTerminalSink(w io.Writer) io.Writer {
	if _, ok := w.(*terminalSink); ok {
		return w
	}
	return &terminalSink{w: w}
}

// Write sanitizes p and forwards it. It reports len(p) so the caller knows the
// whole input was consumed; the sanitized output may be shorter. A trailing
// incomplete UTF-8 sequence is held until the next write, so a multi-byte
// character is never split into a replacement character.
func (t *terminalSink) Write(p []byte) (int, error) {
	data := p
	if len(t.pending) > 0 {
		data = append(t.pending, p...)
		t.pending = nil
	}

	cut := len(data) - incompleteRuneTail(data)
	if cut > 0 {
		out := sanitizeTerminal(string(data[:cut]))
		n, err := io.WriteString(t.w, out)
		if err != nil {
			return 0, err
		}
		if n != len(out) {
			return 0, io.ErrShortWrite
		}
	}
	if cut < len(data) {
		t.pending = append(t.pending, data[cut:]...)
	}
	return len(p), nil
}

// sanitizeTerminal removes C0 and C1 control characters from s, keeping tab and
// newline for layout. ESC (which introduces ANSI/OSC sequences) and CR (which can
// overwrite an earlier line) are control characters, so they are removed. All
// other runes, including valid UTF-8 text, are preserved.
func sanitizeTerminal(s string) string {
	if s == "" {
		return s
	}
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

// incompleteRuneTail reports how many trailing bytes of b form the start of a
// multi-byte UTF-8 sequence that is not yet complete. It returns 0 for valid
// ASCII, for a complete final rune, and for invalid bytes, which the sanitizer
// replaces deterministically.
func incompleteRuneTail(b []byte) int {
	for i := 1; i <= utf8.UTFMax-1 && i <= len(b); i++ {
		start := len(b) - i
		c := b[start]
		if c < utf8.RuneSelf {
			return 0
		}
		if c&0xC0 == 0x80 {
			continue // continuation byte: keep looking for the lead byte
		}
		if !utf8.FullRune(b[start:]) {
			return i
		}
		return 0
	}
	return 0
}

// terminalWriter unwraps the terminal sink to the writer beneath it, so a
// terminal check (isTerminal) still observes the real file.
func terminalWriter(w io.Writer) io.Writer {
	if t, ok := w.(*terminalSink); ok {
		return t.w
	}
	return w
}
