package acp

import (
	"fmt"
	"io"
	"regexp"
	"sync"

	"github.com/imhttran/agentic-sop/internal/redact"
)

// diagnostics is the single stderr path for the ACP transport. Every write is
// prefixed "acp: ", redacts credential-shaped values, and serializes so two
// diagnostics cannot interleave. It never writes to the protocol stream: the
// protocolWriter owns stdout, so diagnostics cannot corrupt frames.
type diagnostics struct {
	mu  sync.Mutex
	out io.Writer
}

// newDiagnostics wraps out as the diagnostics stream.
func newDiagnostics(out io.Writer) *diagnostics {
	return &diagnostics{out: out}
}

// redactedMarker replaces every redacted credential shape.
const redactedMarker = "[REDACTED]"

// Diagnostic text is untrusted: a provider or transport error can echo a
// credential.
//
// The credential shapes are owned by internal/redact, so the ACP and the
// controller agree on what a credential looks like. One guard stays here: a long
// opaque blob is worth redacting from a transport diagnostic, but it is not a
// credential shape -- teaching it to every surface would redact the commit SHAs
// and identifiers that other diagnostics depend on.
var opaqueBlobRE = regexp.MustCompile(`[A-Za-z0-9_-]{32,}`)

// redactLine replaces credential-shaped values in one line of diagnostic text,
// keeping the key for context.
func redactLine(text string) string {
	text = redact.ShapesValues(text, redact.Fixed(redactedMarker))
	return opaqueBlobRE.ReplaceAllString(text, redactedMarker)
}

// shorten bounds untrusted text used inside an error message so a client
// cannot inflate a response or diagnostic with a huge value.
func shorten(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + "..."
}

// Printf writes one diagnostic line: redacted text under the "acp: " prefix.
func (d *diagnostics) Printf(format string, args ...any) {
	d.mu.Lock()
	defer d.mu.Unlock()
	fmt.Fprintf(d.out, "acp: %s\n", redactLine(fmt.Sprintf(format, args...)))
}
