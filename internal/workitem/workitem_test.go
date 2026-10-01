package workitem

import (
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/taskfile"
)

func TestFromTaskPreservesIdentityAndCapability(t *testing.T) {
	spec := &taskfile.Spec{
		ID:                 "P5-001",
		Title:              "Add caching",
		Description:        "Cache provider discovery.",
		AcceptanceCriteria: []string{"cache hit is served"},
		Dependencies:       []string{"P5-000"},
	}
	w := FromTask(spec)
	if w.Kind != KindTask {
		t.Fatalf("kind = %q, want task", w.Kind)
	}
	if w.ID != "P5-001" {
		t.Errorf("id = %q", w.ID)
	}
	if w.Title != "Add caching" {
		t.Errorf("title = %q", w.Title)
	}
	if w.Capability != agent.Implement {
		t.Errorf("capability = %q, want IMPLEMENT", w.Capability)
	}
	// The adapter projects the spec; it does not drop the criteria/dependencies,
	// which remain available to the task lifecycle.
	if !strings.Contains(w.Content, "cache hit is served") {
		t.Errorf("rendered content should carry the acceptance criteria:\n%s", w.Content)
	}
	if err := w.Validate(); err != nil {
		t.Errorf("Validate: %v", err)
	}
}

func TestFromTaskNilSpec(t *testing.T) {
	if w := FromTask(nil); w != (WorkItem{}) {
		t.Fatalf("nil spec should yield the zero item, got %+v", w)
	}
}

func TestFromPromptPreservesContent(t *testing.T) {
	prompt := "  Review internal/provider for layering violations.\nSecond line.  "
	w, err := FromPrompt("prompt-1", agent.Review, prompt)
	if err != nil {
		t.Fatalf("FromPrompt: %v", err)
	}
	if w.Kind != KindPrompt {
		t.Errorf("kind = %q, want prompt", w.Kind)
	}
	if w.Capability != agent.Review {
		t.Errorf("capability = %q, want REVIEW", w.Capability)
	}
	if w.Title != "Review internal/provider for layering violations." {
		t.Errorf("title = %q", w.Title)
	}
	// Surrounding whitespace is normalized, the operator's content is preserved.
	if w.Content != strings.TrimSpace(prompt) {
		t.Errorf("content = %q", w.Content)
	}
}

func TestFromPromptRejectsEmpty(t *testing.T) {
	for _, in := range []string{"", "   ", "\n\t\n"} {
		if _, err := FromPrompt("prompt-1", agent.Plan, in); err == nil {
			t.Errorf("empty prompt %q should be rejected", in)
		}
	}
}

func TestFromPromptRejectsEmptyID(t *testing.T) {
	if _, err := FromPrompt("  ", agent.Plan, "do a thing"); err == nil {
		t.Error("empty id should be rejected")
	}
}

func TestValidateRejectsMalformed(t *testing.T) {
	cases := map[string]WorkItem{
		"no id":         {Kind: KindPrompt, Capability: agent.Plan, Content: "x"},
		"unknown kind":  {ID: "i", Kind: "bogus", Capability: agent.Plan, Content: "x"},
		"no capability": {ID: "i", Kind: KindPrompt, Content: "x"},
		"empty content": {ID: "i", Kind: KindPrompt, Capability: agent.Plan, Content: "  "},
	}
	for name, w := range cases {
		if err := w.Validate(); err == nil {
			t.Errorf("%s: expected a validation error", name)
		}
	}
}

func TestKindValid(t *testing.T) {
	if !KindTask.Valid() || !KindPrompt.Valid() {
		t.Error("task and prompt must be valid kinds")
	}
	if Kind("other").Valid() {
		t.Error("an unknown kind must be invalid")
	}
}
