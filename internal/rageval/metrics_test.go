package rageval

import (
	"math"
	"testing"
)

func approx(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-9 {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestRecallAt5HandComputed(t *testing.T) {
	items := []Retrieved{
		{ID: "a", Rank: 2},
		{ID: "b", Rank: 1},
		{ID: "c", Rank: 3},
	}
	// Expected 3 of which a and c are retrieved => 2/3.
	got, ok := RecallAt5([]string{"a", "c", "z"}, items)
	if !ok {
		t.Fatal("ok = false, want true")
	}
	approx(t, got, 2.0/3.0)
}

func TestRecallAt5NoExpected(t *testing.T) {
	_, ok := RecallAt5(nil, []Retrieved{{ID: "a", Rank: 1}})
	if ok {
		t.Fatal("ok = true, want false for no expected identities")
	}
}

func TestRecallAt5Cutoff(t *testing.T) {
	items := []Retrieved{
		{ID: "a", Rank: 1},
		{ID: "b", Rank: 2},
		{ID: "c", Rank: 3},
		{ID: "d", Rank: 4},
		{ID: "e", Rank: 5},
		{ID: "f", Rank: 6},
	}
	// f is rank 6, outside top K=5, so recall of f is 0.
	got, _ := RecallAt5([]string{"f"}, items)
	approx(t, got, 0)
}

func TestRecallAtK(t *testing.T) {
	items := []Retrieved{
		{ID: "a", Rank: 1},
		{ID: "b", Rank: 2},
		{ID: "c", Rank: 3},
		{ID: "d", Rank: 4},
		{ID: "e", Rank: 5},
		{ID: "f", Rank: 6},
		{ID: "g", Rank: 7},
		{ID: "h", Rank: 8},
		{ID: "i", Rank: 9},
		{ID: "j", Rank: 10},
	}
	expected := []string{"a", "f", "j"}
	// Depth 5 sees only a: 1/3. RecallAt5 must equal RecallAtK at depth 5.
	got5, ok := RecallAtK(expected, items, 5)
	if !ok {
		t.Fatal("depth 5: ok = false, want true")
	}
	approx(t, got5, 1.0/3.0)
	gotDef, _ := RecallAt5(expected, items)
	approx(t, gotDef, got5)
	// Depth 10 sees a, f and j: 3/3.
	got10, ok := RecallAtK(expected, items, 10)
	if !ok {
		t.Fatal("depth 10: ok = false, want true")
	}
	approx(t, got10, 1.0)
	// Two sections of one document count once, at any depth.
	sections := []Retrieved{
		{ID: "a.md#One", Rank: 1},
		{ID: "a.md#Two", Rank: 2},
		{ID: "b.md", Rank: 3},
	}
	gotDoc, _ := RecallAtK([]string{"a.md", "b.md"}, sections, 5)
	approx(t, gotDoc, 1.0)
	// No expected identities: ok is false at any depth.
	if _, ok := RecallAtK(nil, items, 10); ok {
		t.Fatal("depth 10: ok = true, want false for no expected identities")
	}
}

func TestPrecisionAt5HandComputed(t *testing.T) {
	items := []Retrieved{
		{ID: "x", Rank: 1},
		{ID: "y", Rank: 2},
		{ID: "z", Rank: 3},
	}
	// 1 of 3 retrieved is expected.
	got, ok := PrecisionAt5([]string{"y", "w"}, items)
	if !ok {
		t.Fatal("ok = false, want true")
	}
	approx(t, got, 1.0/3.0)
}

func TestPrecisionEmpty(t *testing.T) {
	_, ok := PrecisionAt5([]string{"a"}, nil)
	if ok {
		t.Fatal("ok = true, want false with no retrieved items")
	}
}

func TestCitationCorrect(t *testing.T) {
	items := []Retrieved{{ID: "a", Rank: 1}, {ID: "b", Rank: 2}}
	if c, ok := CitationCorrect([]string{"a", "b"}, items); !ok || !c {
		t.Fatalf("all cited = expected, got correct=%v ok=%v", c, ok)
	}
	if c, ok := CitationCorrect([]string{"a"}, items); !ok || c {
		t.Fatalf("b not expected, want correct=false, got %v ok=%v", c, ok)
	}
	if _, ok := CitationCorrect([]string{"a"}, nil); ok {
		t.Fatal("no citations, want ok=false")
	}
}

func TestProvenanceCorrect(t *testing.T) {
	items := []Retrieved{{ID: "a", Corpus: "rag", Rank: 1}}
	if c, ok := ProvenanceCorrect("rag", items); !ok || !c {
		t.Fatalf("same corpus, want correct=true ok=true, got %v %v", c, ok)
	}
	bad := []Retrieved{{ID: "a", Corpus: "other", Rank: 1}}
	if c, ok := ProvenanceCorrect("rag", bad); !ok || c {
		t.Fatalf("foreign corpus, want correct=false got %v %v", c, ok)
	}
}

func TestCrossCorpus(t *testing.T) {
	items := []Retrieved{
		{ID: "a", Corpus: "rag", Rank: 1},
		{ID: "b", Corpus: "other", Rank: 2},
		{ID: "c", Corpus: "third", Rank: 3},
	}
	got := CrossCorpus("rag", items)
	if len(got) != 2 || got[0] != "b" || got[1] != "c" {
		t.Fatalf("CrossCorpus = %v, want [b c]", got)
	}
}

func TestRecallIsDocumentLevel(t *testing.T) {
	// Both a file-level arm and a section-level arm recall the same document.
	expected := []string{"a.md"}
	fileLevel := []Retrieved{{ID: "a.md", Rank: 1}}
	sectionLevel := []Retrieved{{ID: "a.md#Some Section", Rank: 1}}
	for name, items := range map[string][]Retrieved{"file": fileLevel, "section": sectionLevel} {
		got, ok := RecallAt5(expected, items)
		if !ok {
			t.Fatalf("%s: ok = false, want true", name)
		}
		approx(t, got, 1.0)
	}
	// Two sections of the same file count as one item in the top five.
	items := []Retrieved{
		{ID: "a.md#One", Rank: 1},
		{ID: "a.md#Two", Rank: 2},
		{ID: "b.md", Rank: 3},
	}
	got := RankedIDs(items, 5)
	if len(got) != 2 || got[0] != "a.md" || got[1] != "b.md" {
		t.Fatalf("RankedIDs = %v, want [a.md b.md]: two sections of a.md count as one", got)
	}
}

func TestRankedIDsStableOrder(t *testing.T) {
	items := []Retrieved{
		{ID: "second", Rank: 2},
		{ID: "first", Rank: 1},
	}
	got := RankedIDs(items, 5)
	if len(got) != 2 || got[0] != "first" || got[1] != "second" {
		t.Fatalf("RankedIDs = %v, want [first second]", got)
	}
}
