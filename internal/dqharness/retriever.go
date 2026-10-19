package dqharness

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/imhttran/agentic-sop/internal/decisionquality"
	"github.com/imhttran/agentic-sop/internal/memindex"
	"github.com/imhttran/agentic-sop/internal/retrievalboundary"
)

// DefaultEvidenceTokens is the fallback evidence-token budget when the run
// configuration does not set one.
const DefaultEvidenceTokens = 2000

// KeywordRetriever is arm B: the local SQLite FTS5 keyword index over the case's
// pre-decision evidence. It is the same bounded boundary used by the advisory
// implementation, so the evaluation measures the shipped retrieval path rather than
// a parallel re-implementation.
//
// Every call indexes only the case's pre-decision evidence, so a case can never see
// its own eventual solution, a near-duplicate solution, or evidence produced after
// the cutoff. The index is a disposable temporary database.
type KeywordRetriever struct{}

// Mode names the retrieval implementation in the run record.
func (KeywordRetriever) Mode() string { return "sqlite-fts5-keyword" }

// Retrieve indexes the case's pre-decision evidence and returns the bounded matches.
func (KeywordRetriever) Retrieve(c decisionquality.Case, budget int) ([]decisionquality.EvidenceItem, error) {
	if budget <= 0 {
		budget = DefaultEvidenceTokens
	}
	dir, err := os.MkdirTemp("", "rdm004-index-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	ix, err := memindex.Open(filepath.Join(dir, "index.db"))
	if err != nil {
		return nil, err
	}
	defer ix.Close()

	ctx := context.Background()
	for _, ev := range c.PreDecisionEvidence {
		rec := memindex.Record{
			RecordID:    ev.RecordID,
			ContentHash: ev.ContentHash,
			Corpus:      retrievalboundary.CorpusOutcome,
			Repository:  "rdm004",
			TaskID:      c.ID,
			Status:      "partial",
			Eligible:    true,
			Citation:    ev.RecordID,
			Text:        ev.Text,
		}
		if rec.ContentHash == "" {
			rec.ContentHash = "sha256:" + ev.RecordID
		}
		if err := ix.Upsert(ctx, rec); err != nil {
			return nil, fmt.Errorf("dqharness: index evidence %s: %w", ev.RecordID, err)
		}
	}

	res, err := ix.Search(ctx, retrievalboundary.ScopedQuery{
		Scope: retrievalboundary.Scope{Corpus: retrievalboundary.CorpusOutcome},
		Text:  c.TaskContext,
		Limits: retrievalboundary.Limits{
			MaxResults:  len(c.PreDecisionEvidence),
			TokenBudget: budget,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("dqharness: retrieve %s: %w", c.ID, err)
	}

	byID := make(map[string]decisionquality.EvidenceItem, len(c.PreDecisionEvidence))
	for _, ev := range c.PreDecisionEvidence {
		byID[ev.RecordID] = ev
	}
	out := make([]decisionquality.EvidenceItem, 0, len(res.Evidence))
	for _, e := range res.Evidence {
		ev, ok := byID[e.RecordID]
		if !ok {
			return nil, fmt.Errorf("dqharness: retriever returned unknown evidence %q", e.RecordID)
		}
		out = append(out, ev)
	}
	return out, nil
}
