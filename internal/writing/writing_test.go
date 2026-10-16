package writing

import (
	"strings"
	"testing"
)

// TestRuleIsSingleSourcedAndComplete proves the shared rule is defined once, that
// PromptBlock carries it verbatim (not a second copy), and that it states the
// required clauses: the style target, the preservation list, and the human-facing
// response order.
func TestRuleIsSingleSourcedAndComplete(t *testing.T) {
	if strings.TrimSpace(Rule) == "" {
		t.Fatal("Rule must not be empty")
	}

	block := PromptBlock()
	if n := strings.Count(block, Rule); n != 1 {
		t.Errorf("PromptBlock repeats the rule %d times, want exactly once", n)
	}
	if !strings.Contains(block, Heading) {
		t.Errorf("PromptBlock must carry the %q heading:\n%s", Heading, block)
	}

	for _, clause := range []string{
		"active voice",
		"ASD-STE100",
		"acceptance criteria",
		"approval conditions",
		"lead with the outcome",
		"next action",
	} {
		if !strings.Contains(Rule, clause) {
			t.Errorf("the shared rule is missing the %q clause:\n%s", clause, Rule)
		}
	}
}
