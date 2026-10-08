package retrievalgate

// EV-003 test-only, model-free evaluation harness (E1 offline arm).
//
// This file adds NO production behavior: it is a Go test file, so it is never
// compiled into any binary. It exercises the existing, unmodified facilities
// (repoindex.Build CTX-002, retrieval.New/Search CTX-003, retrievalgate.Measure /
// baselineOrder CTX-004) over the live repository's structural index, reusing the
// shared CTX-004 metric verbatim rather than a weaker second implementation.
//
// It is deterministic and model-free: no provider, no network, no token counting.
// Identical inputs yield identical per-case metric rows, and the ranking is invariant
// to candidate-corpus iteration order (the order-independence assertion required by
// EV-003 acceptance).
//
// Run:  go test ./internal/retrievalgate/ -run TestEV003LiveEvaluation -v

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/repoindex"
	"github.com/imhttran/agentic-sop/internal/retrieval"
)

// ev003K is the fixed evaluation depth. It is constant so runs are reproducible.
const ev003K = 5

// ev003Case is one live-repository evaluation case: a query and its labelled relevant
// IDs in the retrieval.Candidate vocabulary (symbol IDs, "file:<path>", "doc:<path>").
// The labels follow the EV-002 §3.3 labelling procedure: they are drawn from the
// repository's own structural index at the pinned ref, never guessed.
type ev003Case struct {
	ID       string
	Category string
	Query    []string
	Relevant []string
}

// ev003Metrics is one arm's measured metric row for one case.
type ev003Metrics struct {
	Precision float64
	Recall    float64
	MRR       float64
	Bytes     int
}

// ev003Row is one case's measured baseline-vs-retrieval comparison plus the top
// retrieval result's CTX-003 fields (source, score, reason, rank).
type ev003Row struct {
	Case      string
	Category  string
	Baseline  ev003Metrics
	Retrieval ev003Metrics
	TopID     string
	TopSource string
	TopScore  float64
	TopReason string
	TopRank   int
}

// ev003Corpus is the live-repository corpus: one case per Phase 8 §14 category,
// labelled against the agentic-sop structural index. The relevant IDs are the
// EV-002 §3.3 targets for each category over this repository.
func ev003Corpus() []ev003Case {
	return []ev003Case{
		{
			ID: "single-file", Category: "single_file",
			Query:    []string{"repoindex", "index", "build"},
			Relevant: []string{"file:internal/repoindex/index.go"},
		},
		{
			ID: "multi-file", Category: "multi_file",
			Query:    []string{"retrieval", "rank", "search", "index"},
			Relevant: []string{"file:internal/retrieval/retrieval.go", "file:internal/repoindex/index.go", "doc:docs/reports/ev-context-run/EV-002-corpus-and-baseline-protocol.md"},
		},
		{
			ID: "test-discovery", Category: "existing_test_discovery",
			Query:    []string{"retrievalgate", "gate", "test", "evaluate"},
			Relevant: []string{"file:internal/retrievalgate/gate_test.go"},
		},
		{
			ID: "interface", Category: "interface_implementation",
			Query:    []string{"retrieval", "index", "search"},
			Relevant: []string{"file:internal/retrieval/retrieval.go"},
		},
		{
			ID: "config-change", Category: "configuration_change",
			Query:    []string{"retrievalgate", "measure", "corpus", "evaluate"},
			Relevant: []string{"file:internal/retrievalgate/gate.go"},
		},
		{
			ID: "docs-code", Category: "documentation_and_code",
			Query:    []string{"context", "engine", "retrieval", "evaluation"},
			Relevant: []string{"doc:docs/reports/ev-context-run/EV-002-corpus-and-baseline-protocol.md", "doc:docs/reports/ev-context-run/EV-003-retrieval-evaluation.md"},
		},
		{
			ID: "bug-fix", Category: "bug_fix",
			Query:    []string{"retrievalgate", "baseline", "order", "stable"},
			Relevant: []string{"file:internal/retrievalgate/gate.go"},
		},
		{
			ID: "cross-package", Category: "cross_package_dependency",
			Query:    []string{"retrieval", "candidates", "index"},
			Relevant: []string{"file:internal/retrieval/retrieval.go", "file:internal/repoindex/index.go"},
		},
		{
			ID: "replan", Category: "replan_after_failure",
			Query:    []string{"retrievalgate", "measure", "precision", "recall"},
			Relevant: []string{"file:internal/retrievalgate/gate.go"},
		},
	}
}

// ev003RunCase scores one case through both arms with the shared CTX-004 metric and
// captures the top retrieval result's CTX-003 fields.
func ev003RunCase(c ev003Case, bm25 *retrieval.Index, candidates []retrieval.Candidate) ev003Row {
	row := ev003Row{Case: c.ID, Category: c.Category}

	base := Measure(baselineOrder(Case{ID: c.ID, Category: c.Category, Query: c.Query, Relevant: c.Relevant}, candidates), c.Relevant, ev003K, candidates)
	cand := Measure(bm25IDs(bm25, c.Query), c.Relevant, ev003K, candidates)
	row.Baseline = ev003Metrics{Precision: base.Precision, Recall: base.Recall, MRR: base.MRR, Bytes: base.Bytes}
	row.Retrieval = ev003Metrics{Precision: cand.Precision, Recall: cand.Recall, MRR: cand.MRR, Bytes: cand.Bytes}

	results := bm25.Search(retrieval.Query{Terms: c.Query}, ev003K)
	if len(results) > 0 {
		top := results[0]
		row.TopID = top.ID
		row.TopSource = string(top.Source)
		row.TopScore = top.Score
		row.TopReason = top.Reason
		row.TopRank = top.Rank
	}
	return row
}

// ev003Rows runs the whole corpus and returns the per-case rows.
func ev003Rows(corpus []ev003Case, bm25 *retrieval.Index, candidates []retrieval.Candidate) []ev003Row {
	rows := make([]ev003Row, 0, len(corpus))
	for _, c := range corpus {
		rows = append(rows, ev003RunCase(c, bm25, candidates))
	}
	return rows
}

// ev003Key renders a row as a comparable string, used for re-run and order checks.
func ev003Key(r ev003Row) string {
	return fmt.Sprintf("%s|b=%.9f,%.9f,%.9f,%d|c=%.9f,%.9f,%.9f,%d|t=%s,%s,%.9f,%d,%s",
		r.Case,
		r.Baseline.Precision, r.Baseline.Recall, r.Baseline.MRR, r.Baseline.Bytes,
		r.Retrieval.Precision, r.Retrieval.Recall, r.Retrieval.MRR, r.Retrieval.Bytes,
		r.TopID, r.TopSource, r.TopScore, r.TopRank, r.TopReason)
}

// TestEV003LiveEvaluation is the E1 offline, model-free evaluation. It builds the live
// structural index, scores the corpus through both arms, and asserts determinism,
// order-independence, and the CTX-003 result contract.
func TestEV003LiveEvaluation(t *testing.T) {
	start := time.Now()
	root := ev003RepoRoot(t)
	idx, err := repoindex.Build(repoindex.Options{Root: root})
	if err != nil {
		t.Fatalf("repoindex.Build: %v", err)
	}
	buildMs := time.Since(start).Milliseconds()

	candidates := retrieval.CandidatesFromIndex(idx)
	if len(candidates) == 0 {
		t.Fatal("live candidate corpus is empty; cannot evaluate")
	}
	bm25 := retrieval.New(candidates)
	corpus := ev003Corpus()

	searchStart := time.Now()
	rows := ev003Rows(corpus, bm25, candidates)
	searchMs := time.Since(searchStart).Milliseconds()

	// Per-case output.
	aggBase, aggCand := ev003Metrics{}, ev003Metrics{}
	for _, r := range rows {
		t.Logf("EV003 case=%s category=%s base p@%d=%.3f r@%d=%.3f mrr=%.3f bytes=%d | cand p@%d=%.3f r@%d=%.3f mrr=%.3f bytes=%d | top id=%q source=%q score=%.6f rank=%d reason=%q",
			r.Case, r.Category, ev003K, r.Baseline.Precision, ev003K, r.Baseline.Recall, r.Baseline.MRR, r.Baseline.Bytes,
			ev003K, r.Retrieval.Precision, ev003K, r.Retrieval.Recall, r.Retrieval.MRR, r.Retrieval.Bytes,
			r.TopID, r.TopSource, r.TopScore, r.TopRank, r.TopReason)
		aggBase.Precision += r.Baseline.Precision
		aggBase.Recall += r.Baseline.Recall
		aggBase.MRR += r.Baseline.MRR
		aggBase.Bytes += r.Baseline.Bytes
		aggCand.Precision += r.Retrieval.Precision
		aggCand.Recall += r.Retrieval.Recall
		aggCand.MRR += r.Retrieval.MRR
		aggCand.Bytes += r.Retrieval.Bytes
	}
	n := float64(len(rows))
	aggBase = ev003Metrics{Precision: aggBase.Precision / n, Recall: aggBase.Recall / n, MRR: aggBase.MRR / n, Bytes: int(float64(aggBase.Bytes) / n)}
	aggCand = ev003Metrics{Precision: aggCand.Precision / n, Recall: aggCand.Recall / n, MRR: aggCand.MRR / n, Bytes: int(float64(aggCand.Bytes) / n)}
	t.Logf("EV003 aggregate base p@%d=%.3f r@%d=%.3f mrr=%.3f bytes_mean=%d", ev003K, aggBase.Precision, ev003K, aggBase.Recall, aggBase.MRR, aggBase.Bytes)
	t.Logf("EV003 aggregate cand p@%d=%.3f r@%d=%.3f mrr=%.3f bytes_mean=%d", ev003K, aggCand.Precision, ev003K, aggCand.Recall, aggCand.MRR, aggCand.Bytes)
	t.Logf("EV003 index_build_ms=%d search_ms=%d candidates=%d cases=%d", buildMs, searchMs, len(candidates), len(rows))

	// CTX-003 contract: every emitted result exposes source, score, reason, and rank.
	for _, r := range rows {
		if r.TopID == "" || r.TopSource == "" || r.TopReason == "" || r.TopRank < 1 {
			t.Fatalf("case %s: result missing CTX-003 fields: id=%q source=%q reason=%q rank=%d",
				r.Case, r.TopID, r.TopSource, r.TopReason, r.TopRank)
		}
	}

	// Re-run identity: identical inputs yield identical rows.
	rowsB := ev003Rows(corpus, retrieval.New(candidates), candidates)
	for i := range rows {
		if ev003Key(rows[i]) != ev003Key(rowsB[i]) {
			t.Fatalf("case %s: re-run differs:\n a=%s\n b=%s", rows[i].Case, ev003Key(rows[i]), ev003Key(rowsB[i]))
		}
	}

	// Order independence: reverse the candidate corpus; rows must be identical.
	reversed := make([]retrieval.Candidate, len(candidates))
	for i, c := range candidates {
		reversed[len(candidates)-1-i] = c
	}
	rowsRev := ev003Rows(corpus, retrieval.New(reversed), reversed)
	for i := range rows {
		if ev003Key(rows[i]) != ev003Key(rowsRev[i]) {
			t.Fatalf("case %s: ranking depends on iteration order:\n a=%s\n b=%s", rows[i].Case, ev003Key(rows[i]), ev003Key(rowsRev[i]))
		}
	}
}

// ev003RepoRoot walks up from the package directory to the module root (the directory
// containing go.mod). It is deterministic and filesystem-order independent.
func ev003RepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("resolve package dir: %v", err)
	}
	cur := dir
	for {
		entries, err := filepath.Glob(filepath.Join(cur, "go.mod"))
		if err == nil && len(entries) > 0 {
			return cur
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			t.Fatalf("could not locate module root from %s", dir)
		}
		cur = parent
	}
}
