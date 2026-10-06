// Package vectoreval is the CTX-009 Vector Retrieval Evaluation: the second evidence
// gate in Phase 8.
//
// It compares the CTX-003 BM25 ranker against a deterministic vector-space candidate
// and a hybrid of the two, over the CTX-004 corpus, using the CTX-004 metric. It is an
// evaluation harness, not production retrieval: no vector retriever is wired into the
// workflow, and nothing here is invoked by a task.
//
// The candidate it can evaluate deterministically is a model-free vector-space model
// (smoothed TF-IDF with cosine similarity). Embedding-based semantic retrieval would
// require a model or embedding provider, which the deterministic harness cannot own
// reproducibly, so it is not evaluated here; a vector retriever that cannot be
// reproduced cannot be adopted on evidence. The gate therefore adopts vector retrieval
// only when the deterministic candidate strictly beats BM25 on the CTX-004 metric with
// no precision or recall regression. A tie is a rejection: do not ship retrieval that
// does not materially improve results.
package vectoreval

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/imhttran/agentic-sop/internal/retrieval"
	"github.com/imhttran/agentic-sop/internal/retrievalgate"
)

// Decision is the gate's explicit outcome.
type Decision string

const (
	// Adopt: a deterministic vector or hybrid candidate strictly improves on BM25.
	Adopt Decision = "ADOPT"
	// Reject: no deterministic candidate materially improves on BM25; retain BM25.
	Reject Decision = "REJECT"
)

// rrfK is the reciprocal-rank-fusion constant used by the hybrid candidate.
const rrfK = 60.0

// Report is the gate's deterministic outcome.
type Report struct {
	K        int                  `json:"k"`
	Cases    int                  `json:"cases"`
	BM25     retrievalgate.Counts `json:"bm25"`
	Vector   retrievalgate.Counts `json:"vector"`
	Hybrid   retrievalgate.Counts `json:"hybrid"`
	Decision Decision             `json:"vector_retrieval"`
	Reason   string               `json:"reason"`
	// NotMeasured names the comparison dimensions this deterministic gate cannot
	// observe without running a model.
	NotMeasured []string `json:"not_measured"`
}

// Evaluate runs the corpus through BM25, the vector-space candidate, and the hybrid,
// and decides the gate. It is pure and deterministic: identical inputs yield an
// identical report.
func Evaluate(corpus []retrievalgate.Case, candidates []retrieval.Candidate, k int) Report {
	if k <= 0 {
		k = 5
	}
	bm25 := retrieval.New(candidates)
	vec := newVectorIndex(candidates)

	var bm, ve, hy retrievalgate.Counts
	for _, c := range corpus {
		bmIDs := bm25IDs(bm25, c.Query)
		vecIDs := vec.search(c.Query)
		hyIDs := fuse(bmIDs, vecIDs)

		bm = addCounts(bm, retrievalgate.Measure(bmIDs, c.Relevant, k, candidates))
		ve = addCounts(ve, retrievalgate.Measure(vecIDs, c.Relevant, k, candidates))
		hy = addCounts(hy, retrievalgate.Measure(hyIDs, c.Relevant, k, candidates))
	}
	n := float64(len(corpus))
	if n > 0 {
		bm, ve, hy = meanCounts(bm, n), meanCounts(ve, n), meanCounts(hy, n)
	}

	rep := Report{K: k, Cases: len(corpus), BM25: bm, Vector: ve, Hybrid: hy,
		NotMeasured: []string{"task success", "verification success", "discovery effort", "reliable elapsed time", "operational complexity", "embedding-based semantic retrieval (not deterministically reproducible)"}}

	switch {
	case len(corpus) == 0:
		rep.Decision, rep.Reason = Reject, "no evaluation cases"
	default:
		if name, c, ok := beatsBM25(bm, map[string]retrievalgate.Counts{"vector": ve, "hybrid": hy}); ok {
			rep.Decision = Adopt
			rep.Reason = fmt.Sprintf("%s strictly improved ranking (precision@%d %.3f vs %.3f, MRR %.3f vs %.3f) with no recall regression",
				name, k, c.Precision, bm.Precision, c.MRR, bm.MRR)
		} else {
			rep.Decision = Reject
			rep.Reason = fmt.Sprintf("no deterministic candidate beat BM25 (vector MRR %.3f, hybrid MRR %.3f, BM25 MRR %.3f); retain BM25",
				ve.MRR, hy.MRR, bm.MRR)
		}
	}
	return rep
}

// beatsBM25 returns the first candidate that exceeds BM25 on MRR without regressing on
// precision@k or recall@k. Candidates are checked in a fixed order for determinism.
func beatsBM25(bm retrievalgate.Counts, cands map[string]retrievalgate.Counts) (string, retrievalgate.Counts, bool) {
	for _, name := range []string{"vector", "hybrid"} {
		c := cands[name]
		if c.Precision >= bm.Precision && c.Recall >= bm.Recall && c.MRR > bm.MRR {
			return name, c, true
		}
	}
	return "", retrievalgate.Counts{}, false
}

// bm25IDs returns the BM25-ranked candidate ids for a query.
func bm25IDs(ix *retrieval.Index, query []string) []string {
	rs := ix.Search(retrieval.Query{Terms: query}, 0)
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.ID)
	}
	return out
}

// fuse combines two ranked id lists by reciprocal rank fusion, returning a
// deterministically ordered id list (descending fused score, then ascending id).
func fuse(a, b []string) []string {
	score := map[string]float64{}
	for i, id := range a {
		score[id] += 1.0 / (rrfK + float64(i+1))
	}
	for i, id := range b {
		score[id] += 1.0 / (rrfK + float64(i+1))
	}
	ids := make([]string, 0, len(score))
	for id := range score {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		if score[ids[i]] != score[ids[j]] {
			return score[ids[i]] > score[ids[j]]
		}
		return ids[i] < ids[j]
	})
	return ids
}

// vectorIndex is a deterministic, model-free vector-space index: smoothed TF-IDF
// weights with cosine similarity. All floating-point accumulation iterates terms in
// sorted order, so map iteration order cannot leak into a score.
type vectorIndex struct {
	docs []vectorDoc
	idf  map[string]float64
}

type vectorDoc struct {
	id   string
	vec  map[string]float64
	norm float64
}

// newVectorIndex builds the vector-space index over the candidate corpus.
func newVectorIndex(corpus []retrieval.Candidate) *vectorIndex {
	n := len(corpus)
	tfs := make([]map[string]int, n)
	df := map[string]int{}
	for i, c := range corpus {
		tf := map[string]int{}
		for _, tok := range retrieval.Tokenize(c.Text) {
			tf[tok]++
		}
		tfs[i] = tf
		for t := range tf {
			df[t]++
		}
	}
	idf := map[string]float64{}
	terms := sortedKeys(df)
	for _, t := range terms {
		idf[t] = math.Log(float64(n+1)/float64(df[t]+1)) + 1
	}

	ix := &vectorIndex{idf: idf}
	for i, c := range corpus {
		vec := map[string]float64{}
		var sum float64
		for _, t := range sortedKeys(tfs[i]) {
			w := (1 + math.Log(float64(tfs[i][t]))) * idf[t]
			vec[t] = w
			sum += w * w
		}
		ix.docs = append(ix.docs, vectorDoc{id: c.ID, vec: vec, norm: math.Sqrt(sum)})
	}
	return ix
}

// search ranks the corpus against query terms by cosine similarity, returning matching
// ids by descending similarity and then ascending id.
func (ix *vectorIndex) search(query []string) []string {
	qtf := map[string]int{}
	for _, term := range query {
		for _, tok := range retrieval.Tokenize(term) {
			qtf[tok]++
		}
	}
	qvec := map[string]float64{}
	var qsum float64
	for _, t := range sortedKeys(qtf) {
		idf, ok := ix.idf[t]
		if !ok {
			continue
		}
		w := (1 + math.Log(float64(qtf[t]))) * idf
		qvec[t] = w
		qsum += w * w
	}
	if qsum == 0 {
		return nil
	}
	qnorm := math.Sqrt(qsum)
	qterms := sortedKeys(qvec)

	type scored struct {
		id  string
		cos float64
	}
	var hits []scored
	for _, d := range ix.docs {
		if d.norm == 0 {
			continue
		}
		var dot float64
		for _, t := range qterms {
			if w, ok := d.vec[t]; ok {
				dot += qvec[t] * w
			}
		}
		if dot <= 0 {
			continue
		}
		hits = append(hits, scored{id: d.id, cos: dot / (qnorm * d.norm)})
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].cos != hits[j].cos {
			return hits[i].cos > hits[j].cos
		}
		return hits[i].id < hits[j].id
	})
	out := make([]string, 0, len(hits))
	for _, h := range hits {
		out = append(out, h.id)
	}
	return out
}

// sortedKeys returns the keys of a map in ascending order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// String renders a compact human summary.
func (r Report) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "VECTOR_RETRIEVAL = %s\n", r.Decision)
	fmt.Fprintf(&b, "  cases: %d  k: %d\n", r.Cases, r.K)
	fmt.Fprintf(&b, "  bm25   precision@k=%.3f recall@k=%.3f mrr=%.3f\n", r.BM25.Precision, r.BM25.Recall, r.BM25.MRR)
	fmt.Fprintf(&b, "  vector precision@k=%.3f recall@k=%.3f mrr=%.3f\n", r.Vector.Precision, r.Vector.Recall, r.Vector.MRR)
	fmt.Fprintf(&b, "  hybrid precision@k=%.3f recall@k=%.3f mrr=%.3f\n", r.Hybrid.Precision, r.Hybrid.Recall, r.Hybrid.MRR)
	fmt.Fprintf(&b, "  reason: %s\n", r.Reason)
	return b.String()
}

// addCounts accumulates two retrievalgate.Counts values, including the byte total.
func addCounts(a, b retrievalgate.Counts) retrievalgate.Counts {
	return retrievalgate.Counts{Precision: a.Precision + b.Precision, Recall: a.Recall + b.Recall, MRR: a.MRR + b.MRR, Bytes: a.Bytes + b.Bytes}
}

// meanCounts divides aggregate counts by the number of cases.
func meanCounts(c retrievalgate.Counts, n float64) retrievalgate.Counts {
	return retrievalgate.Counts{Precision: c.Precision / n, Recall: c.Recall / n, MRR: c.MRR / n, Bytes: int(float64(c.Bytes) / n)}
}
