package cli

import (
	"encoding/json"
	"io"
	"strings"
	"testing"
)

// controlText is model output that carries a terminal clear, a red SGR sequence,
// a carriage return, and an OSC 8 hyperlink. None of it is legitimate display
// text; a raw ESC or CR reaching the terminal is the LLM10 sink violation.
const controlText = "\x1b[2J\x1b[31mred\x1b[0m\rrewritten\x1b]8;;https://example.invalid\x07link\x07"

// TestReviewOutputNeutralizesTerminalControls reproduces the review sink gap: a
// model review response is rendered to the operator's terminal. ANSI/OSC control
// sequences in the summary or a finding title must not reach the terminal, while
// the human-readable text must remain.
func TestReviewOutputNeutralizesTerminalControls(t *testing.T) {
	dir := t.TempDir()
	// The model response is JSON, so the control characters are encoded
	// (\u001b, \r, \u0007) and decode back to raw controls in the parsed report.
	raw, err := json.Marshal(map[string]any{
		"summary": "summary " + controlText,
		"findings": []map[string]any{{
			"severity": "MEDIUM",
			"title":    "title " + controlText,
			"file":     "a.go",
			"line":     3,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	code, stdout, _ := runInjectedCLI(t, dir, "diff --git a/a.go b/a.go\n", &fakeAgent{content: string(raw)}, "review")
	if code != exitOK {
		t.Fatalf("code=%d, want %d (stdout=%q)", code, exitOK, stdout)
	}
	assertNoTerminalControls(t, "review stdout", stdout)
	if !strings.Contains(stdout, "summary") || !strings.Contains(stdout, "title") {
		t.Errorf("legitimate review text was lost: %q", stdout)
	}
}

// TestPromptOutputNeutralizesTerminalControls reproduces the prompt sink gap:
// `sop prompt` prints the model result verbatim. A control sequence in that
// result must not reach the operator's terminal.
func TestPromptOutputNeutralizesTerminalControls(t *testing.T) {
	clearProviderEnv(t)
	dir := t.TempDir()
	content := "before " + controlText + " after"
	code, stdout, stderr := runCLIWithAgent(t, dir, &fakeCapabilityAgent{review: content}, "prompt", "--capability", "review", "explain this")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	assertNoTerminalControls(t, "prompt stdout", stdout)
	if !strings.Contains(stdout, "before") || !strings.Contains(stdout, "after") {
		t.Errorf("legitimate prompt text was lost: %q", stdout)
	}
}

// TestPromptJSONKeepsModelResultIntact proves the terminal filter is a display
// sink only: machine-readable output stays valid JSON and still carries the
// model's exact result. JSON escapes control characters, so the filter must not
// alter the decoded value.
func TestPromptJSONKeepsModelResultIntact(t *testing.T) {
	clearProviderEnv(t)
	dir := t.TempDir()
	content := "before " + controlText + " after"
	code, stdout, stderr := runCLIWithAgent(t, dir, &fakeCapabilityAgent{review: content}, "prompt", "--json", "--capability", "review", "explain this")
	if code != exitOK {
		t.Fatalf("code=%d stderr=%s", code, stderr)
	}
	var doc struct {
		Status string `json:"status"`
		Result string `json:"result"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("machine-readable output is not valid JSON: %v\n%s", err, stdout)
	}
	if doc.Status != "completed" {
		t.Errorf("status = %q, want completed", doc.Status)
	}
	if doc.Result != content {
		t.Errorf("result = %q, want the model text preserved exactly", doc.Result)
	}
}

// assertNoTerminalControls fails when display text still carries a C0/C1 control
// character other than tab or newline.
func assertNoTerminalControls(t *testing.T, label, text string) {
	t.Helper()
	for _, r := range text {
		if r == '\n' || r == '\t' {
			continue
		}
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			t.Errorf("%s carries terminal control U+%04X: %q", label, r, text)
			return
		}
	}
}

// TestSanitizeTerminalPreservesLayout proves the filter removes controls while
// keeping the tab/newline layout and non-ASCII text a report or prompt relies on.
func TestSanitizeTerminalPreservesLayout(t *testing.T) {
	in := "a\tb\n\x1b[31mc\x1b[0m\r\x00d\u009be\u00e9\U0001F600"
	got := sanitizeTerminal(in)

	assertNoTerminalControls(t, "sanitized text", got)
	for _, want := range []string{"a\tb\n", "c", "d", "\u00e9\U0001F600"} {
		if !strings.Contains(got, want) {
			t.Errorf("sanitized = %q, missing %q", got, want)
		}
	}
}

// TestTerminalSinkReassemblesSplitRune proves a multi-byte character written in
// pieces is not corrupted into a replacement character by the streaming filter.
func TestTerminalSinkReassemblesSplitRune(t *testing.T) {
	var buf strings.Builder
	sink := newTerminalSink(&buf)

	euro := "\u20ac" // 3 bytes
	if _, err := io.WriteString(sink, "cost "+euro[:1]); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(sink, euro[1:2]); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(sink, euro[2:]+"\x1b[0m"); err != nil {
		t.Fatal(err)
	}

	if got := buf.String(); !strings.HasPrefix(got, "cost \u20ac") {
		t.Errorf("streamed output = %q, want the intact euro sign", got)
	}
	// Removing ESC neutralizes the trailing SGR sequence; its body stays as inert
	// text, but no control character may remain.
	assertNoTerminalControls(t, "streamed output", buf.String())
}

// TestTerminalWriterUnwrapsSink proves the terminal check still sees the writer
// beneath the sink, so wrapping does not disable interactive activity output.
func TestTerminalWriterUnwrapsSink(t *testing.T) {
	var buf strings.Builder
	if got := terminalWriter(newTerminalSink(&buf)); got != io.Writer(&buf) {
		t.Errorf("terminalWriter did not unwrap the sink: %T", got)
	}
	if got := terminalWriter(&buf); got != io.Writer(&buf) {
		t.Errorf("terminalWriter changed an unwrapped writer: %T", got)
	}
}
