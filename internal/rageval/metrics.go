package rageval

import "strings"

// Retrieval metrics for the three-arm Rag evaluation runner.
//
// Every function here is pure and deterministic: it reads its arguments and
// writes no state. That property is what lets the runner produce identical rows
// for two runs over the same dataset, corpus, and frozen RAG responses.

// K is the cutoff used by recall@5 and precision@5.
const K = 5

// Retrieved is one retrieved item, ranked by its arm.
type Retrieved struct {
	// ID is the stable identity of the item (corpus-relative source path plus
	// section, as the arms report it).
	ID string
	// Rank is 1-based. Rank 1 is the best hit.
	Rank int
	// Corpus is the corpus the item belongs to.
	Corpus string
}

// documentID returns the document part of an identity: the part before the
// first '#', or the whole string when there is no '#'. Retrieval scoring
// compares documents, not sections, so "a.md" and "a.md#Some Section" are
// the same document.
func documentID(id string) string {
	if i := strings.Index(id, "#"); i >= 0 {
		return id[:i]
	}
	return id
}

// RankedIDs returns the document identities of the first k items, ordered by
// rank ascending. Each item's ID is mapped through documentID and only the
// first occurrence of each document is kept, so the top five are five
// distinct documents. Ties keep their input order so the result is
// deterministic.
func RankedIDs(items []Retrieved, k int) []string {
	ordered := stableByRank(items)
	if k > len(ordered) {
		k = len(ordered)
	}
	if k < 0 {
		k = 0
	}
	ids := make([]string, 0, k)
	seen := make(map[string]bool, k)
	for _, it := range ordered[:k] {
		doc := documentID(it.ID)
		if seen[doc] {
			continue
		}
		seen[doc] = true
		ids = append(ids, doc)
	}
	return ids
}

// RecallAt5 is the fraction of expected identities found in the first K
// retrieved identities. With no expected identities there is nothing to
// recall, so it returns 0 and ok is false.
func RecallAt5(expected []string, items []Retrieved) (value float64, ok bool) {
	if len(expected) == 0 {
		return 0, false
	}
	hit := map[string]bool{}
	for _, id := range RankedIDs(items, K) {
		hit[id] = true
	}
	found := 0
	for _, want := range uniqueSorted(expected) {
		if hit[want] {
			found++
		}
	}
	return float64(found) / float64(len(uniqueSorted(expected))), true
}

// PrecisionAt5 is the fraction of the first K retrieved identities that are
// expected. With no retrieved identities there is nothing to score, so it
// returns 0 and ok is false.
func PrecisionAt5(expected []string, items []Retrieved) (value float64, ok bool) {
	top := RankedIDs(items, K)
	if len(top) == 0 {
		return 0, false
	}
	want := map[string]bool{}
	for _, id := range expected {
		want[id] = true
	}
	hits := 0
	for _, id := range top {
		if want[id] {
			hits++
		}
	}
	return float64(hits) / float64(len(top)), true
}

// CitationCorrect reports whether every cited identity is one of the expected
// identities, and whether at least one citation exists.
func CitationCorrect(expected []string, items []Retrieved) (correct bool, ok bool) {
	cited := RankedIDs(items, K)
	if len(cited) == 0 {
		return false, false
	}
	want := map[string]bool{}
	for _, id := range expected {
		want[id] = true
	}
	for _, id := range cited {
		if !want[id] {
			return false, true
		}
	}
	return true, true
}

// ProvenanceCorrect reports whether every cited identity carries a non-empty
// corpus that lies inside the permitted corpus. A cited identity with no corpus
// is a provenance failure.
func ProvenanceCorrect(permitted string, items []Retrieved) (correct bool, ok bool) {
	cited := stableByRank(items)
	if len(cited) > K {
		cited = cited[:K]
	}
	if len(cited) == 0 {
		return false, false
	}
	for _, it := range cited {
		if it.Corpus == "" || it.Corpus != permitted {
			return false, true
		}
	}
	return true, true
}

// CrossCorpus returns the identities of items whose corpus differs from the
// permitted corpus, ordered by rank. A non-empty result is a hard-blocker
// failure for the run.
func CrossCorpus(permitted string, items []Retrieved) []string {
	var out []string
	for _, it := range stableByRank(items) {
		if it.Corpus != permitted {
			out = append(out, it.ID)
		}
	}
	return out
}

// stableByRank sorts items by Rank ascending with a stable sort so equal ranks
// keep input order.
func stableByRank(items []Retrieved) []Retrieved {
	out := make([]Retrieved, len(items))
	copy(out, items)
	// Insertion sort keeps this dependency-free and stable for small slices.
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].Rank < out[j-1].Rank; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// uniqueSorted returns s without duplicates, sorted ascending.
func uniqueSorted(s []string) []string {
	seen := make(map[string]bool, len(s))
	out := make([]string, 0, len(s))
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}
