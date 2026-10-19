package retrievalboundary

import (
	"context"

	"github.com/imhttran/agentic-sop/internal/retrieval"
)

// RankerAdapter adapts the existing deterministic BM25 ranker (internal/retrieval.Index)
// to the bounded Retriever interface without changing its ranking behavior. It is the
// S1 compatibility adapter: the same corpus and terms produce the same IDs and order as
// retrieval.Index.Search. Bounds are enforced after ranking.
type RankerAdapter struct {
	Index  *retrieval.Index
	Corpus Corpus
}

// Retrieve ranks the wrapped index against q.Text, then applies the Scope corpus check
// and the Limits bounds. The ranker's own tie-break (ascending candidate ID) is kept
// unchanged and satisfies the boundary's deterministic tie-break requirement.
func (a *RankerAdapter) Retrieve(ctx context.Context, q ScopedQuery) (Result, error) {
	lim, err := Validate(q)
	if err != nil {
		return Result{}, err
	}
	if a.Index == nil {
		return Result{}, &InvalidQueryError{Reason: "nil ranker index"}
	}
	if a.Corpus != "" && q.Scope.Corpus != a.Corpus {
		return Result{}, &UnsupportedCorpusError{Corpus: q.Scope.Corpus}
	}
	// The wrapped ranker carries no repository or task metadata, so it cannot honor a
	// narrower scope. Failing closed prevents evidence from another repository or task
	// being returned as if it matched the requested scope.
	if q.Scope.Repository != "" || q.Scope.TaskID != "" {
		return Result{}, &InvalidQueryError{Reason: "ranker adapter cannot scope by repository or task id"}
	}
	if err := ctx.Err(); err != nil {
		return Result{}, timeoutErr(err)
	}

	// Rank all matches (limit 0 = no limit) so the token budget, not the ranker, decides
	// the cut. Determinism is identical to Search with no limit.
	ranked := a.Index.Search(retrieval.Query{Terms: retrieval.Tokenize(q.Text)}, 0)
	ev := make([]Evidence, 0, len(ranked))
	for _, r := range ranked {
		ev = append(ev, Evidence{
			RecordID:   r.ID,
			Corpus:     q.Scope.Corpus,
			Repository: q.Scope.Repository,
			TaskID:     q.Scope.TaskID,
			Citation:   r.Path,
			Score:      r.Score,
			Tokens:     EstimateTokens(r.Path + " " + r.Reason),
		})
		if len(ev) >= lim.MaxResults {
			break
		}
	}
	res := ApplyBounds(ev, lim)
	res.Compatibility = "ranker adapter: score descending, record id ascending"
	return res, nil
}
