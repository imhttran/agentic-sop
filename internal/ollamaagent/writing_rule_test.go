package ollamaagent

import (
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/writing"
)

// TestSystemPromptCarriesWritingRuleOnce proves the tool harness's generated
// technical prompt carries the shared writing rule from its single definition,
// exactly once, so the model's prose follows the same rule as SOP's human-facing
// responses.
func TestSystemPromptCarriesWritingRuleOnce(t *testing.T) {
	req := agent.Request{Capability: agent.Implement, Task: "do it"}
	got := systemPrompt(req, PolicyFor(agent.Implement), "/repo", nil)
	if n := strings.Count(got, writing.Rule); n != 1 {
		t.Fatalf("systemPrompt must carry the shared rule exactly once, got %d:\n%s", n, got)
	}
	if !strings.Contains(got, writing.Heading) {
		t.Errorf("systemPrompt missing the rule heading:\n%s", got)
	}
}
