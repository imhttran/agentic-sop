package agent

import (
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/writing"
)

// TestRenderPromptCarriesWritingRuleOnce proves the generated technical prompt
// carries the shared writing rule from its single definition, exactly once, so the
// model's prose follows the same rule as SOP's human-facing responses.
func TestRenderPromptCarriesWritingRuleOnce(t *testing.T) {
	got := renderPrompt(Request{Capability: Implement, Task: "do it"})
	if n := strings.Count(got, writing.Rule); n != 1 {
		t.Fatalf("renderPrompt must carry the shared rule exactly once, got %d:\n%s", n, got)
	}
	if !strings.Contains(got, writing.Heading) {
		t.Errorf("renderPrompt missing the rule heading:\n%s", got)
	}
}
