package ollamaagent

import (
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/redact"
)

// The seven-format enhancement is not authorized. This records the current
// prompt-boundary gap: a recognized-format value still passes through
// userPromptWithPolicy. After authorization and implementation, flip this test
// to assert redaction.
func TestSevenFormatPromptPending(t *testing.T) {
	const token = "ya29.a0AfH6SMBexampleexampleexampleexample"
	got := userPromptWithPolicy(agent.Request{
		Task:  "fix " + token,
		Input: "context " + token,
	}, redact.Policy{Enabled: true})
	if !strings.Contains(got, token) {
		t.Fatalf("the seven-format value is already redacted; update the approval status and this test: %q", got)
	}
}
