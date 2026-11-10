package ollamaagent

import (
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/redact"
)

// The approved seven-format enhancement is C1-gated. With C1 enabled, the
// prompt boundary redacts the token; with C1 disabled, behavior is unchanged.
func TestSevenFormatPromptRedacted(t *testing.T) {
	const token = "ya29.a0AfH6SMBexampleexampleexampleexample"
	enabled := redact.Policy{Enabled: true}

	got := userPromptWithPolicy(agent.Request{
		Task:  "fix " + token,
		Input: "context " + token,
	}, enabled)
	if strings.Contains(got, token) {
		t.Fatalf("C1 left the seven-format value in the prompt: %q", got)
	}
	if !strings.Contains(got, "[redacted]") {
		t.Fatalf("C1 did not mark the prompt redaction: %q", got)
	}

	off := userPromptWithPolicy(agent.Request{
		Task:  "fix " + token,
		Input: "context " + token,
	}, redact.Policy{})
	if !strings.Contains(off, token) {
		t.Fatalf("C1-disabled prompt behavior changed: %q", off)
	}
}
