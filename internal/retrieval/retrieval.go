// Package retrieval is a deterministic lexical ranker (BM25) over the Structural
// Repository Index's candidate corpus. It is the CTX-003 retrieval layer: it answers
// "which repository evidence is lexically relevant to this execution?" and never touches
// the model, the lifecycle, or the repository.
//
// Determinism is a contract, not a best effort: the corpus is scored by aggregate term
// statistics, ranking breaks ties by stable candidate identity, and no filesystem or map
// iteration order can leak into the result. Identical inputs and corpus yield identical
// ranked results.
package retrieval

import (
	"math"
	"sort"
	"strings"

	"github.com/imhttran/agentic-sop/internal/repoindex"
)

// BM25 parameters. They are fixed so ranking is reproducible; a caller never tunes them.
const (
	// K1 is the BM25 term-frequency saturation.
	K1 = 1.2
	// B is the BM25 length-normalization weight.
	B = 0.75
)

// Source is the corpus category a candidate comes from.
type Source string

const (
	// SourceSymbol is a Go declaration (type/struct/interface/function/method).
	SourceSymbol Source = "symbol"
	// SourceFile is an indexed repository file.
	SourceFile Source = "file"
	// SourceDocument is an indexed repository document.
	SourceDocument Source = "document"
)

// Candidate is one retrievable corpus item.
type Candidate struct {
	// ID is a stable identity (a symbol id, "file:<path>", or "doc:<path>").
	ID string
	// Source is the corpus category.
	Source Source
	// Path is the repository-relative path.
	Path string
	// Text is the searchable text: a symbol's name/kind/package/receiver, or a file or
	// document path and class. It is a compact descriptor, never a source body.
	Text string
}

// Query is one retrieval request: the terms assembled from the execution's evidence
// (task, plan, acceptance criteria, and failure evidence). Terms are matched
// case-insensitively.
type Query struct {
	Terms []string
}

// Result is one ranked retrieval result. It always exposes its source, score, a
// deterministic reason, and its 1-based rank.
type Result struct {
	ID     string  `json:"id"`
	Source Source  `json:"source"`
	Path   string  `json:"path"`
	Score  float64 `json:"score"`
	Reason string  `json:"reason"`
	Rank   int     `json:"rank"`
}

// Index is a BM25 index over a candidate corpus.
type Index struct {
	docs  []docEntry
	df    map[string]int
	n     int
	avgdl float64
}

// docEntry is one indexed candidate with its term frequencies and length.
type docEntry struct {
	cand Candidate
	tf   map[string]int
	dl   int
}

// New builds a BM25 index over corpus. The corpus order does not affect any score or
// ranking: statistics are aggregates, and ranking breaks ties by candidate ID.
func New(corpus []Candidate) *Index {
	ix := &Index{df: map[string]int{}}
	total := 0
	for _, c := range corpus {
		toks := Tokenize(c.Text)
		tf := make(map[string]int, len(toks))
		for _, t := range toks {
			tf[t]++
		}
		ix.docs = append(ix.docs, docEntry{cand: c, tf: tf, dl: len(toks)})
		for term := range tf {
			ix.df[term]++
		}
		total += len(toks)
	}
	ix.n = len(ix.docs)
	if ix.n > 0 {
		ix.avgdl = float64(total) / float64(ix.n)
	}
	return ix
}

// Search ranks the corpus against q. It returns at most limit results (limit <= 0 means
// no limit), ordered by descending score and then by ascending candidate ID, with 1-based
// ranks. Only candidates that match at least one term are returned.
func (ix *Index) Search(q Query, limit int) []Result {
	terms := normalizeQuery(q.Terms)

	type scored struct {
		cand    Candidate
		score   float64
		matched []string
	}
	var hits []scored
	for _, d := range ix.docs {
		var score float64
		var matched []string
		for _, term := range terms {
			tf := d.tf[term]
			if tf == 0 {
				continue
			}
			df := ix.df[term]
			idf := math.Log(1 + (float64(ix.n-df)+0.5)/(float64(df)+0.5))
			denom := float64(tf) + K1*(1-B+B*float64(d.dl)/ix.avgdl)
			score += idf * (float64(tf) * (K1 + 1)) / denom
			matched = append(matched, term)
		}
		if score > 0 {
			sort.Strings(matched)
			hits = append(hits, scored{cand: d.cand, score: score, matched: matched})
		}
	}

	sort.Slice(hits, func(i, j int) bool {
		if hits[i].score != hits[j].score {
			return hits[i].score > hits[j].score
		}
		return hits[i].cand.ID < hits[j].cand.ID
	})
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}

	out := make([]Result, 0, len(hits))
	for i, h := range hits {
		out = append(out, Result{
			ID:     h.cand.ID,
			Source: h.cand.Source,
			Path:   h.cand.Path,
			Score:  h.score,
			Reason: "lexical match: " + strings.Join(h.matched, ", "),
			Rank:   i + 1,
		})
	}
	return out
}

// normalizeQuery lowercases and deduplicates query terms, sorted so a reason is stable.
// Duplicate terms do not change a score, so the set is what matters.
func normalizeQuery(terms []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, term := range terms {
		for _, tok := range Tokenize(term) {
			if !seen[tok] {
				seen[tok] = true
				out = append(out, tok)
			}
		}
	}
	sort.Strings(out)
	return out
}

// Tokenize splits text into lowercase alphanumeric tokens ([a-z0-9_]), deterministically.
func Tokenize(s string) []string {
	var out []string
	var b strings.Builder
	flush := func() {
		if b.Len() > 0 {
			out = append(out, b.String())
			b.Reset()
		}
	}
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return out
}

// CandidatesFromIndex builds the lexical corpus from a Structural Repository Index: Go
// symbols, indexed files, and documents. It is deterministic and independent of the
// index's slice order (the result is sorted by candidate ID).
func CandidatesFromIndex(idx repoindex.Index) []Candidate {
	var out []Candidate
	for _, s := range idx.Symbols {
		text := s.Name + " " + string(s.Kind) + " " + s.Package
		if s.Receiver != "" {
			text += " " + s.Receiver
		}
		out = append(out, Candidate{ID: s.ID, Source: SourceSymbol, Path: s.File, Text: text})
	}
	for _, f := range idx.Files {
		out = append(out, Candidate{ID: "file:" + f.Path, Source: SourceFile, Path: f.Path, Text: f.Path})
	}
	for _, d := range idx.Documents {
		out = append(out, Candidate{ID: "doc:" + d.Path, Source: SourceDocument, Path: d.Path, Text: d.Path + " " + string(d.Class)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// QueryFromEvidence assembles retrieval terms from the execution's evidence: the task id,
// the acceptance criteria, and any failure evidence. It is the deterministic input the plan
// specifies (task, plan, acceptance criteria, failure evidence).
func QueryFromEvidence(taskID string, acceptance []string, failure string) Query {
	terms := []string{taskID}
	terms = append(terms, acceptance...)
	if strings.TrimSpace(failure) != "" {
		terms = append(terms, failure)
	}
	return Query{Terms: terms}
}
