package acp

import (
	"fmt"
	"io"
	"regexp"
	"sync"
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

// The patterns below redact credential shapes from untrusted diagnostic text.
// security.RedactValues removes exact known secret values from evaluation
// text, but a diagnostic error string never enumerates the credential values it
// may embed, so exact-value redaction would leak anything not on a known list.
// Shape-based redaction here is the second line of defense: it cannot prove
// absence, it only bounds what diagnostics can leak.
var (
	// credentialBearerRE matches a Bearer authorization value.
	credentialBearerRE = regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._-]+`)
	// credentialTokenREs match common credential token shapes: OpenAI-style
	// sk-/pk- keys, GitHub app tokens, Slack tokens, AWS access keys, JWTs,
	// and any long opaque base64url or hex blob.
	credentialTokenREs = []*regexp.Regexp{
		regexp.MustCompile(`(?:sk|pk)-[A-Za-z0-9_-]{8,}`),
		regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{20,}`),
		regexp.MustCompile(`xox[abprs]-[A-Za-z0-9-]{10,}`),
		regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
		regexp.MustCompile(`eyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+`),
		regexp.MustCompile(`eyJ[A-Za-z0-9_-]{20,}`),
		regexp.MustCompile(`[A-Za-z0-9_-]{32,}`),
	}
	// credentialKeyRE matches key=value or key: value assignments whose key
	// names a credential; the value is replaced, the key is kept for context.
	credentialKeyRE = regexp.MustCompile(`(?i)\b(api[_-]?key|secret|token|password|credential|authorization)[a-z0-9 _-]*["']?\s*[:=]\s*["']?[A-Za-z0-9_./+=-]+`)
)

// redactLine replaces credential-shaped values in one line of diagnostic text.
// The key=value pass runs first (it keeps the key for context), then the token
// shapes replace their whole match.
func redactLine(text string) string {
	text = credentialKeyRE.ReplaceAllString(text, "${1}="+redactedMarker)
	text = credentialBearerRE.ReplaceAllString(text, "Bearer "+redactedMarker)
	for _, re := range credentialTokenREs {
		text = re.ReplaceAllString(text, redactedMarker)
	}
	return text
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
