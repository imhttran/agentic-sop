package ollamaagent

import (
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/redact"
)

// The user prompt uses the same C1 composition. An opaque value with no
// credential-like key, known format, or inventory entry remains visible; an
// exact inventory value is redacted. This records the residual; it does not
// change production behavior.
func TestUnknownFormatPromptResidual(t *testing.T) {
	const opaque = "boundary-residual-value-99"
	enabled := redact.Policy{Enabled: true}

	got := userPromptWithPolicy(agent.Request{
		Task:  "fix " + opaque,
		Input: "context " + opaque,
	}, enabled)
	if !strings.Contains(got, opaque) {
		t.Fatalf("the context-free prompt residual is no longer observed; update the assessment: %q", got)
	}

	withInventory := redact.Policy{Enabled: true, Values: []string{opaque}}
	got = userPromptWithPolicy(agent.Request{
		Task:  "fix " + opaque,
		Input: "context " + opaque,
	}, withInventory)
	if strings.Contains(got, opaque) {
		t.Fatalf("the inventory value was not redacted from the prompt: %q", got)
	}
}
