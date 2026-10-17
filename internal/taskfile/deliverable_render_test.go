package taskfile

import (
	"strings"
	"testing"
)

// TestRenderDeliverableGroupsAndLegacyFallback: grouped rendering is used when the
// caller supplies the classification, and a task file that supplies none keeps the
// exact legacy rendering.
func TestRenderDeliverableGroupsAndLegacyFallback(t *testing.T) {
	legacy := &Spec{Title: "t", Deliverables: []string{"a recorded note", "another"}}
	text := legacy.Render()
	if !strings.Contains(text, "## Deliverables") || !strings.Contains(text, "- a recorded note") {
		t.Fatalf("legacy rendering changed:\n%s", text)
	}

	grouped := &Spec{Title: "t", Deliverables: []string{"a recorded note", "another"}}
	grouped.DeliverableGroups = []DeliverableGroup{{
		Heading: "Expected outcomes (descriptive)",
		Note:    "Report these in your summary.",
		Items:   []string{"a recorded note", "another"},
	}}
	g := grouped.Render()
	if strings.Contains(g, "## Deliverables") {
		t.Errorf("grouped rendering still uses the artifact heading:\n%s", g)
	}
	if !strings.Contains(g, "## Expected outcomes (descriptive)") || !strings.Contains(g, "Report these in your summary.") {
		t.Fatalf("grouped rendering missing heading or note:\n%s", g)
	}
	if !strings.Contains(g, "- a recorded note") {
		t.Fatalf("items not rendered:\n%s", g)
	}
}
