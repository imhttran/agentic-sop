package context

import (
	"strings"
	"testing"
)

func baseInputs() Inputs {
	return Inputs{TaskID: "T1", Task: "do the thing", Plan: "plan", ChangedFiles: []string{"a.go"}}
}

func TestInputsNilKnowledgeIsByteIdentical(t *testing.T) {
	without := FromInputs(baseInputs(), DefaultLimits())
	in := baseInputs()
	in.Knowledge = nil
	withNil := FromInputs(in, DefaultLimits())
	if withNil.Bytes() != without.Bytes() {
		t.Fatalf("nil Knowledge changed the context: %d vs %d bytes", withNil.Bytes(), without.Bytes())
	}
}

func TestInputsKnowledgeAppearsAndSortsLast(t *testing.T) {
	in := baseInputs()
	in.Knowledge = []Item{{Source: SourceKnowledgeRAG, Identity: "kb/a.md#S", Reason: "untrusted", Priority: PriorityKnowledgeRAG, Text: "evidence"}}
	c := FromInputs(in, DefaultLimits())
	if len(c.items) == 0 {
		t.Fatal("empty context")
	}
	last := c.items[len(c.items)-1]
	if last.Source != SourceKnowledgeRAG || last.Priority != PriorityKnowledgeRAG || last.Text != "evidence" {
		t.Fatalf("knowledge item not last/preserved: %+v", last)
	}
	for i := 0; i < len(c.items)-1; i++ {
		if c.items[i].Priority >= PriorityKnowledgeRAG {
			t.Fatalf("non-knowledge item at priority %d sorts at/after knowledge", c.items[i].Priority)
		}
	}
}

func TestInputsSharedBudgetPreservesHigherPriority(t *testing.T) {
	in := baseInputs()
	in.Knowledge = []Item{{Source: SourceKnowledgeRAG, Identity: "kb/x", Reason: "u", Priority: PriorityKnowledgeRAG, Text: strings.Repeat("x", 4096)}}
	c := FromInputs(in, Limits{MaxItems: 64, MaxFiles: 32, MaxBytes: 40})
	if c.Bytes() > 40 {
		t.Fatalf("shared byte budget exceeded: %d", c.Bytes())
	}
	var hasLifecycle, hasRepo bool
	for _, it := range c.items {
		if it.Priority == PriorityLifecycle {
			hasLifecycle = true
		}
		if it.Source == SourceRepository {
			hasRepo = true
		}
	}
	if !hasLifecycle || !hasRepo {
		t.Fatalf("higher-priority context lost under a tight budget: lifecycle=%v repo=%v", hasLifecycle, hasRepo)
	}
	if last := c.items[len(c.items)-1]; last.Source == SourceKnowledgeRAG && len(last.Text) >= 4096 {
		t.Fatal("knowledge item was not bounded by the shared budget")
	}
}
