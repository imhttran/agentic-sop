package retrieval

import (
	"reflect"
	"testing"

	"github.com/imhttran/agentic-sop/internal/repoindex"
)

func corpus() []Candidate {
	return []Candidate{
		{ID: "symbol:example.com/m.Config", Source: SourceSymbol, Path: "config.go", Text: "Config struct example.com/m"},
		{ID: "symbol:example.com/m.Router", Source: SourceSymbol, Path: "router.go", Text: "Router struct example.com/m"},
		{ID: "doc:docs/specs/ROUTING.md", Source: SourceDocument, Path: "docs/specs/ROUTING.md", Text: "docs/specs/ROUTING.md spec"},
		{ID: "file:notes.txt", Source: SourceFile, Path: "notes.txt", Text: "notes.txt"},
	}
}

func ids(rs []Result) []string {
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.ID)
	}
	return out
}

func TestSearchRanksByRelevance(t *testing.T) {
	ix := New(corpus())
	rs := ix.Search(Query{Terms: []string{"router"}}, 0)
	if len(rs) != 1 || rs[0].ID != "symbol:example.com/m.Router" {
		t.Fatalf("results = %v, want the Router symbol", ids(rs))
	}
	if rs[0].Rank != 1 || rs[0].Score <= 0 || rs[0].Reason == "" {
		t.Errorf("result = %+v, want rank 1, positive score, and a reason", rs[0])
	}
}

func TestSearchIsDeterministicAcrossCorpusOrder(t *testing.T) {
	a := New(corpus())
	rev := corpus()
	for i, j := 0, len(rev)-1; i < j; i, j = i+1, j-1 {
		rev[i], rev[j] = rev[j], rev[i]
	}
	b := New(rev)
	q := Query{Terms: []string{"struct", "config", "router"}}
	ra, rb := a.Search(q, 0), b.Search(q, 0)
	if !reflect.DeepEqual(ra, rb) {
		t.Errorf("corpus order changed the ranking:\n a=%+v\n b=%+v", ra, rb)
	}
}

func TestSearchTieBreaksByID(t *testing.T) {
	// Two documents with the same single-term content rank by ID ascending.
	ix := New([]Candidate{
		{ID: "b", Text: "alpha"},
		{ID: "a", Text: "alpha"},
	})
	rs := ix.Search(Query{Terms: []string{"alpha"}}, 0)
	if got := ids(rs); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Errorf("tie order = %v, want [a b]", got)
	}
}

func TestSearchNoMatchIsEmpty(t *testing.T) {
	if rs := New(corpus()).Search(Query{Terms: []string{"nothinghere"}}, 0); len(rs) != 0 {
		t.Errorf("results = %v, want none", ids(rs))
	}
}

func TestSearchLimitBoundary(t *testing.T) {
	ix := New([]Candidate{
		{ID: "a", Text: "alpha"},
		{ID: "b", Text: "alpha"},
		{ID: "c", Text: "alpha"},
	})
	q := Query{Terms: []string{"alpha"}}
	if rs := ix.Search(q, 2); len(rs) != 2 || rs[0].Rank != 1 || rs[1].Rank != 2 {
		t.Errorf("limit 2 = %v, want 2 ranked results", ids(rs))
	}
	if rs := ix.Search(q, 3); len(rs) != 3 {
		t.Errorf("limit 3 = %v, want 3", ids(rs))
	}
	if rs := ix.Search(q, 5); len(rs) != 3 {
		t.Errorf("limit above the corpus = %v, want all 3", ids(rs))
	}
}

func TestSearchRepeatedIsIdentical(t *testing.T) {
	ix := New(corpus())
	q := Query{Terms: []string{"config", "router", "docs"}}
	if !reflect.DeepEqual(ix.Search(q, 0), ix.Search(q, 0)) {
		t.Error("repeated search must return identical results")
	}
}

func TestTokenize(t *testing.T) {
	got := Tokenize("Config_Name v2/foo-bar")
	want := []string{"config_name", "v2", "foo", "bar"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Tokenize = %v, want %v", got, want)
	}
}

func TestCandidatesFromIndex(t *testing.T) {
	idx := repoindex.Index{
		Symbols:   []repoindex.Symbol{{ID: "symbol:example.com/m.Config", Kind: repoindex.SymStruct, Package: "example.com/m", File: "config.go", Name: "Config"}},
		Files:     []repoindex.File{{Path: "config.go"}},
		Documents: []repoindex.Document{{Path: "docs/README.md", Class: repoindex.ClassDocumentation}},
	}
	cs := CandidatesFromIndex(idx)
	if len(cs) != 3 {
		t.Fatalf("candidates = %d, want 3", len(cs))
	}
	if cs[0].ID != "doc:docs/README.md" || cs[1].ID != "file:config.go" || cs[2].ID != "symbol:example.com/m.Config" {
		t.Errorf("candidates not sorted by id: %+v", cs)
	}
	if !contains(cs[2].Text, "struct") {
		t.Errorf("symbol text = %q, want the kind", cs[2].Text)
	}
}

func TestQueryFromEvidence(t *testing.T) {
	q := QueryFromEvidence("T001", []string{"starts"}, "compiler error")
	got := normalizeQuery(q.Terms)
	want := []string{"compiler", "error", "starts", "t001"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("query terms = %v, want %v", got, want)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
