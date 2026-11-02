package rageval

import (
	"reflect"
	"strings"
	"testing"

	sopctx "github.com/imhttran/agentic-sop/internal/context"
	"github.com/imhttran/agentic-sop/internal/ragclient"
	"github.com/imhttran/agentic-sop/internal/retrieval"
)

// armBExpectedEvidence returns frozen evidence that includes a section and a
// page, so the test proves arm B drops both suffixes from the citation.
func armBExpectedEvidence() []ragclient.Evidence {
	return []ragclient.Evidence{
		{Corpus: "rag", Source: "docs/loader.md", Section: "verify", Text: "sectioned evidence text", Rank: 1},
		{Corpus: "rag", Source: "docs/paged.md", Page: 7, Text: "paged evidence text", Rank: 2},
	}
}

// expectedArmBIdentities computes the expected ranked identities from the frozen
// evidence's Corpus and Source fields directly, without the runner's helpers.
func expectedArmBIdentities(ev []ragclient.Evidence) []Retrieved {
	out := make([]Retrieved, 0, len(ev))
	for _, e := range ev {
		out = append(out, Retrieved{ID: e.Source, Corpus: e.Corpus, Rank: len(out) + 1})
	}
	return out
}

// wantRAGIdentities reads the knowledge-rag items straight out of a built
// context and recovers their (source, corpus) identity without the runner's
// splitCitation helper: split at the first '/', then cut any '#…' and ':p…'
// suffix. This keeps the expectation independent of the runner code under test.
func wantRAGIdentities(c sopctx.Context) []Retrieved {
	var out []Retrieved
	for _, it := range c.Items() {
		if it.Source != sopctx.SourceKnowledgeRAG {
			continue
		}
		citation := it.Identity
		corpus := ""
		source := citation
		if slash := strings.Index(citation, "/"); slash >= 0 {
			corpus = citation[:slash]
			source = citation[slash+1:]
		}
		if i := strings.Index(source, "#"); i >= 0 {
			source = source[:i]
		}
		if i := strings.Index(source, ":p"); i >= 0 {
			source = source[:i]
		}
		out = append(out, Retrieved{ID: source, Corpus: corpus, Rank: len(out) + 1})
	}
	return out
}

// TestArmBUsesSOPContextBuild proves arm B's ranked identities, assembled
// context, bytes, and truncation equal what sopctx.KnowledgeItems plus
// sopctx.Build produce for the same arm A hits and frozen evidence, built
// directly here. It also proves internal/rageval holds no mergeRanks.
func TestArmBUsesSOPContextBuild(t *testing.T) {
	docs := corpusDocs()
	idxA := armAIndex(t, docs)
	q := smallDataset().Questions[0] // Q-CODE, a scorable question with hits.

	// Arm A hits come from the same repoindex/retrieval path the runner uses.
	hits := idxA.Search(retrieval.Query{Terms: retrieval.Tokenize(q.Question)}, 5)

	// Frozen evidence carries a section and a page.
	ev := armBExpectedEvidence()
	rag := RAGResult{RawEvidence: ev}

	// Build the expected context directly through the SOP context package.
	items := []sopctx.Item{{
		Source:   sopctx.SourceTask,
		Identity: q.ID,
		Reason:   "evaluation question",
		Priority: sopctx.PriorityLifecycle,
		Text:     q.Question,
	}}
	for _, hit := range hits {
		items = append(items, sopctx.Item{
			Source:   sopctx.SourceRepository,
			Identity: hit.Path,
			Reason:   "bm25 hit",
			Priority: sopctx.PriorityRepository,
			Text:     hit.Path,
		})
	}
	items = append(items, sopctx.KnowledgeItems(ev, 5, 0)...)
	want := sopctx.Build(items, sopctx.DefaultLimits())

	// Build the runner's context through the same call chain.
	r := &Runner{Config: Config{TopK: 5, Corpus: "rag"}}
	got, err := r.buildArmBContext(q, searchResults(hits, q.PermittedCorpus), rag)
	if err != nil {
		t.Fatalf("buildArmBContext: %v", err)
	}

	if got.Bytes() != want.Bytes() {
		t.Fatalf("arm B bytes = %d, want %d", got.Bytes(), want.Bytes())
	}
	if got.Truncated() != want.Truncated() {
		t.Fatalf("arm B truncated = %v, want %v", got.Truncated(), want.Truncated())
	}
	if got.Render() != want.Render() {
		t.Fatalf("arm B render differs from sopctx.Build:\n got: %q\nwant: %q", got.Render(), want.Render())
	}
	gotIDs, err := identitiesFromContext(got, q.PermittedCorpus)
	if err != nil {
		t.Fatalf("identitiesFromContext(got): %v", err)
	}
	wantIDs, err := identitiesFromContext(want, q.PermittedCorpus)
	if err != nil {
		t.Fatalf("identitiesFromContext(want): %v", err)
	}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("arm B identities differ from sopctx.Build order")
	}

	// The RAG identities computed directly from the built knowledge-rag items
	// must equal the evidence Source with section and page suffixes dropped.
	gotRAG := wantRAGIdentities(got)
	wantRAG := expectedArmBIdentities(ev)
	if len(gotRAG) != len(wantRAG) {
		t.Fatalf("arm B RAG identities = %v, want %v", gotRAG, wantRAG)
	}
	for i, w := range wantRAG {
		if gotRAG[i].ID != w.ID || gotRAG[i].Corpus != w.Corpus {
			t.Fatalf("arm B RAG identity %d = %+v, want %+v", i, gotRAG[i], w)
		}
	}

	// The runner's recovered identities must agree too: a knowledge-rag item
	// resolves to its evidence source, not to its corpus path or citation.
	runnerIDs := gotIDs
	for _, w := range wantRAG {
		found := false
		for _, id := range runnerIDs {
			if id.ID == w.ID && id.Corpus == w.Corpus {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("runner identities %v do not contain evidence identity %+v", runnerIDs, w)
		}
	}

	// The package must not hold a hand-written rank merge. The runner merges
	// through sopctx.Build only.
	assertNoMergeRanks(t)
}
