package vectoreval

import (
	"testing"

	"github.com/imhttran/agentic-sop/internal/retrieval"
	"github.com/imhttran/agentic-sop/internal/retrievalgate"
)

func TestEvaluateIsDeterministic(t *testing.T) {
	a := Evaluate(retrievalgate.Corpus(), retrievalgate.Candidates(), 5)
	b := Evaluate(retrievalgate.Corpus(), retrievalgate.Candidates(), 5)
	if a.String() != b.String() || a.Decision != b.Decision {
		t.Fatalf("not deterministic:\n a=%s\n b=%s", a.String(), b.String())
	}
}

func TestEvaluateProducesADecision(t *testing.T) {
	rep := Evaluate(retrievalgate.Corpus(), retrievalgate.Candidates(), 5)
	t.Logf("\n%s", rep.String())
	if rep.Decision != Adopt && rep.Decision != Reject {
		t.Fatalf("decision = %q", rep.Decision)
	}
	if rep.Reason == "" || len(rep.NotMeasured) == 0 {
		t.Errorf("report missing reason/not-measured: %+v", rep)
	}
	if rep.Decision == Reject {
		for name, c := range map[string]retrievalgate.Counts{"vector": rep.Vector, "hybrid": rep.Hybrid} {
			if c.Precision >= rep.BM25.Precision && c.Recall >= rep.BM25.Recall && c.MRR > rep.BM25.MRR {
				t.Errorf("%s beat BM25 but the gate rejected", name)
			}
		}
	}
}

func TestEvaluateEmptyCorpusRejects(t *testing.T) {
	if rep := Evaluate(nil, retrievalgate.Candidates(), 5); rep.Decision != Reject {
		t.Errorf("empty corpus = %q, want REJECT", rep.Decision)
	}
}

func TestVectorIndexRanksExactMatchFirst(t *testing.T) {
	cands := []retrieval.Candidate{
		{ID: "b", Text: "beta gamma"},
		{ID: "a", Text: "alpha beta"},
	}
	got := newVectorIndex(cands).search([]string{"alpha"})
	if len(got) != 1 || got[0] != "a" {
		t.Errorf("search(alpha) = %v, want [a]", got)
	}
}

func TestFuseIsRrfAndDeterministic(t *testing.T) {
	got := fuse([]string{"a", "b"}, []string{"b", "a"})
	if len(got) != 2 {
		t.Fatalf("fuse = %v", got)
	}
	if got[0] != "a" || got[1] != "b" {
		t.Errorf("fuse tie-break = %v, want [a b]", got)
	}
	again := fuse([]string{"a", "b"}, []string{"b", "a"})
	if again[0] != got[0] || again[1] != got[1] {
		t.Errorf("fuse not deterministic")
	}
}
