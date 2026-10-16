package context

import (
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/ragclient"
)

func ragEvidence(corpus, source, section, hash, text string) ragclient.Evidence {
	return ragclient.Evidence{ID: "1", Corpus: corpus, Source: source, Section: section, DocumentHash: hash, Text: text, RetrievalMethod: "hybrid", Rank: 1}
}

func TestKnowledgeRAGPriorityIsLowest(t *testing.T) {
	if PriorityKnowledgeRAG != 6 {
		t.Fatalf("PriorityKnowledgeRAG = %d, want 6", PriorityKnowledgeRAG)
	}
	for name, p := range map[string]int{
		"explicit": PriorityExplicit, "lifecycle": PriorityLifecycle, "repository": PriorityRepository,
		"execution": PriorityExecution, "recovery": PriorityRecovery, "generic": PriorityGeneric,
	} {
		if p >= PriorityKnowledgeRAG {
			t.Fatalf("existing priority %s = %d is not below RAG (6)", name, p)
		}
	}
	if !SourceKnowledgeRAG.Valid() {
		t.Fatal("SourceKnowledgeRAG is not a valid source")
	}
}

func TestKnowledgeItemsConversion(t *testing.T) {
	items := KnowledgeItems([]ragclient.Evidence{ragEvidence("kb", "docs/a.md", "S", "h1", "evidence text")}, 0, 0)
	if len(items) != 1 {
		t.Fatalf("items = %d, want 1", len(items))
	}
	it := items[0]
	if it.Source != SourceKnowledgeRAG || it.Priority != PriorityKnowledgeRAG {
		t.Fatalf("source/priority = %s/%d", it.Source, it.Priority)
	}
	if it.Text != "evidence text" {
		t.Fatalf("text not preserved: %q", it.Text)
	}
	if it.Identity != "kb/docs/a.md#S" {
		t.Fatalf("citation = %q", it.Identity)
	}
	if !strings.Contains(it.Reason, "untrusted") || !strings.Contains(it.Reason, "h1") {
		t.Fatalf("reason lacks untrusted/hash: %q", it.Reason)
	}
}

func TestKnowledgeItemsRejectsUnattributed(t *testing.T) {
	items := KnowledgeItems([]ragclient.Evidence{
		ragEvidence("", "s", "sec", "h", "t"),
		ragEvidence("kb", "", "sec", "h", "t"),
		ragEvidence("kb", "s", "sec", "h", "   "),
	}, 0, 0)
	if len(items) != 0 {
		t.Fatalf("unattributed evidence accepted: %+v", items)
	}
}

func TestKnowledgeDedupAndMissingHash(t *testing.T) {
	// Exact duplicates collapse.
	dupes := KnowledgeItems([]ragclient.Evidence{
		ragEvidence("kb", "a.md", "S", "h", "t1"),
		ragEvidence("kb", "a.md", "S", "h", "t1"),
	}, 0, 0)
	if len(dupes) != 1 {
		t.Fatalf("duplicates not collapsed: %d", len(dupes))
	}
	// A missing hash must not collapse unrelated documents.
	distinct := KnowledgeItems([]ragclient.Evidence{
		ragEvidence("kb", "a.md", "S", "", "t1"),
		ragEvidence("kb", "b.md", "S", "", "t2"),
	}, 0, 0)
	if len(distinct) != 2 {
		t.Fatalf("missing hash collapsed unrelated documents: %d", len(distinct))
	}
}

func TestKnowledgeBudgetAndOrdering(t *testing.T) {
	ev := []ragclient.Evidence{
		ragEvidence("kb", "b.md", "S", "h2", "bbbbbb"),
		ragEvidence("kb", "a.md", "S", "h1", "aaaaaa"),
	}
	ordered := KnowledgeItems(ev, 0, 0)
	if ordered[0].Identity != "kb/a.md#S" || ordered[1].Identity != "kb/b.md#S" {
		t.Fatalf("ordering not deterministic: %q, %q", ordered[0].Identity, ordered[1].Identity)
	}
	if got := KnowledgeItems(ev, 1, 0); len(got) != 1 {
		t.Fatalf("count limit not enforced: %d", len(got))
	}
	budgeted := KnowledgeItems(ev, 0, 8)
	if len(budgeted) != 2 {
		t.Fatalf("byte budget items = %d, want 2 (second truncated)", len(budgeted))
	}
	if n := len(budgeted[0].Text) + len(budgeted[1].Text); n > 8 {
		t.Fatalf("byte budget exceeded: %d", n)
	}
}

func TestKnowledgeItemsAreAdditiveToBaseBuild(t *testing.T) {
	base := Build([]Item{{Source: SourceTask, Identity: "t", Priority: PriorityExplicit, Text: "task"}}, DefaultLimits())
	rag := KnowledgeItems([]ragclient.Evidence{ragEvidence("kb", "a.md", "S", "h", "x")}, 0, 0)
	combined := Build(append([]Item{{Source: SourceTask, Identity: "t", Priority: PriorityExplicit, Text: "task"}}, rag...), DefaultLimits())

	if len(base.items) != 1 {
		t.Fatalf("base context changed: %d", len(base.items))
	}
	if len(combined.items) != 2 || combined.items[1].Source != SourceKnowledgeRAG {
		t.Fatalf("RAG item not ordered last: %+v", combined.items)
	}
}

// TestKnowledgeDistinctUnhashedChunksSurvive pins the corrected Phase-4B dedup:
// distinct un-hashed chunks of the SAME corpus/source/section both survive, while
// exact duplicates still collapse.
func TestKnowledgeDistinctUnhashedChunksSurvive(t *testing.T) {
	distinct := KnowledgeItems([]ragclient.Evidence{
		ragEvidence("kb", "a.md", "S", "", "chunk one"),
		ragEvidence("kb", "a.md", "S", "", "chunk two"),
	}, 0, 0)
	if len(distinct) != 2 {
		t.Fatalf("distinct un-hashed chunks collapsed: %d, want 2", len(distinct))
	}
	dupes := KnowledgeItems([]ragclient.Evidence{
		ragEvidence("kb", "a.md", "S", "", "identical"),
		ragEvidence("kb", "a.md", "S", "", "identical"),
	}, 0, 0)
	if len(dupes) != 1 {
		t.Fatalf("exact duplicates not collapsed: %d, want 1", len(dupes))
	}
}

// TestKnowledgeSameHashDistinctChunksSurvive pins Phase-4B-1 Step 1: two distinct
// chunks that share a document hash both survive (the hash is provenance, not a
// content identity); exact duplicates still collapse.
func TestKnowledgeSameHashDistinctChunksSurvive(t *testing.T) {
	items := KnowledgeItems([]ragclient.Evidence{
		ragEvidence("kb", "a.md", "S", "samehash", "chunk one"),
		ragEvidence("kb", "a.md", "S", "samehash", "chunk two"),
	}, 0, 0)
	if len(items) != 2 {
		t.Fatalf("distinct chunks sharing a hash collapsed: %d, want 2", len(items))
	}
	dupes := KnowledgeItems([]ragclient.Evidence{
		ragEvidence("kb", "a.md", "S", "samehash", "identical"),
		ragEvidence("kb", "a.md", "S", "samehash", "identical"),
	}, 0, 0)
	if len(dupes) != 1 {
		t.Fatalf("exact duplicates not collapsed: %d, want 1", len(dupes))
	}
}
