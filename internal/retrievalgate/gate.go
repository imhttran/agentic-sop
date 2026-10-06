// Package retrievalgate is the CTX-004 Retrieval Evaluation Gate: a deterministic,
// model-free evaluation deciding whether structural + BM25 retrieval materially improves
// over the baseline discovery behavior, before more retrieval complexity is justified.
//
// The gate compares two ways of supplying repository evidence:
//
//	BASELINE        unranked lexical matches in stable-path order (what path discovery
//	                yields before retrieval)
//	CONTEXT ENGINE  structural index + BM25 ranking (retrieval.Ranker)
//
// It measures retrieval quality (precision@k, recall@k, MRR) — the signals SOP can observe
// deterministically without running a model — plus context efficiency (items/bytes), and
// decides RETRIEVAL_GATE = PASS or FAIL. Task-success and tool-call signals require
// execution and are recorded as not-measured rather than invented.
package retrievalgate

import (
	"fmt"
	"sort"
	"strings"

	"github.com/imhttran/agentic-sop/internal/retrieval"
)

// Decision is the gate's explicit outcome.
type Decision string

const (
	// Pass: retrieval provides a useful quality/efficiency improvement with no regression.
	Pass Decision = "PASS"
	// Fail: retrieval shows no meaningful improvement (or regresses).
	Fail Decision = "FAIL"
)

// Case is one deterministic evaluation case: a query and its ground-truth relevant IDs,
// spanning the representative task categories the plan lists.
type Case struct {
	ID       string
	Category string
	Query    []string
	Relevant []string
}

// Counts are one variant's aggregate retrieval metrics.
type Counts struct {
	Precision float64 `json:"precision_at_k"`
	Recall    float64 `json:"recall_at_k"`
	MRR       float64 `json:"mrr"`
	Bytes     int     `json:"context_bytes"`
}

// Report is the gate's deterministic outcome.
type Report struct {
	K         int      `json:"k"`
	Cases     int      `json:"cases"`
	Baseline  Counts   `json:"baseline"`
	Candidate Counts   `json:"candidate"`
	Decision  Decision `json:"retrieval_gate"`
	Reason    string   `json:"reason"`
	// NotMeasured names the signals that require running a model and are therefore not
	// measured by this gate.
	NotMeasured []string `json:"not_measured"`
}

// Evaluate runs the corpus through the baseline and the BM25 candidate and decides the
// gate. It is pure and deterministic: identical inputs yield an identical report.
func Evaluate(corpus []Case, candidates []retrieval.Candidate, k int) Report {
	if k <= 0 {
		k = 5
	}
	bm25 := retrieval.New(candidates)
	base := Counts{}
	cand := Counts{}
	for _, c := range corpus {
		base = base.add(measure(baselineOrder(c, candidates), c.Relevant, k, candidates))
		cand = cand.add(measure(bm25IDs(bm25, c.Query), c.Relevant, k, candidates))
	}
	n := float64(len(corpus))
	if n > 0 {
		base = base.mean(n)
		cand = cand.mean(n)
	}

	rep := Report{K: k, Cases: len(corpus), Baseline: base, Candidate: cand,
		NotMeasured: []string{"task success", "verification success", "discovery iterations", "repeated discovery", "tool calls", "repository mutations", "replans", "attempts", "elapsed wall time"}}

	// PASS requires no quality regression and a real efficiency gain: the candidate must
	// match or beat the baseline on precision@k and MRR, and use no more context bytes.
	switch {
	case len(corpus) == 0:
		rep.Decision, rep.Reason = Fail, "no evaluation cases"
	case cand.Precision < base.Precision:
		rep.Decision, rep.Reason = Fail, "BM25 precision@k regressed against the baseline"
	case cand.MRR < base.MRR:
		rep.Decision, rep.Reason = Fail, "BM25 MRR regressed against the baseline"
	case cand.Recall < base.Recall:
		rep.Decision, rep.Reason = Fail, "BM25 recall@k regressed against the baseline"
	case cand.Precision == base.Precision && cand.MRR == base.MRR:
		rep.Decision, rep.Reason = Fail, "BM25 showed no measurable ranking improvement over the baseline"
	default:
		rep.Decision = Pass
		rep.Reason = fmt.Sprintf("BM25 improved ranking (precision@%d %.3f vs %.3f, MRR %.3f vs %.3f) with no recall regression",
			k, cand.Precision, base.Precision, cand.MRR, base.MRR)
	}
	return rep
}

// measure computes precision@k, recall@k, MRR, and the bytes of the retrieved items.
func measure(ids []string, relevant []string, k int, candidates []retrieval.Candidate) Counts {
	rel := map[string]bool{}
	for _, id := range relevant {
		rel[id] = true
	}
	top := ids
	if len(top) > k {
		top = top[:k]
	}
	hits := 0
	for _, id := range top {
		if rel[id] {
			hits++
		}
	}
	c := Counts{}
	if len(top) > 0 {
		c.Precision = float64(hits) / float64(len(top))
	}
	if len(relevant) > 0 {
		c.Recall = float64(hits) / float64(len(relevant))
	}
	for i, id := range top {
		if rel[id] {
			c.MRR = 1 / float64(i+1)
			break
		}
	}
	byID := map[string]int{}
	for _, cd := range candidates {
		byID[cd.ID] = len(cd.Text)
	}
	for _, id := range top {
		c.Bytes += byID[id]
	}
	return c
}

// baselineOrder is the unranked baseline: every lexically matching candidate in stable ID
// order. It is what naive path discovery yields before retrieval ranks it.
func baselineOrder(c Case, candidates []retrieval.Candidate) []string {
	terms := map[string]bool{}
	for _, t := range c.Query {
		for _, tok := range retrieval.Tokenize(t) {
			terms[tok] = true
		}
	}
	var out []string
	for _, cd := range candidates {
		for _, tok := range retrieval.Tokenize(cd.Text) {
			if terms[tok] {
				out = append(out, cd.ID)
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

// bm25IDs returns the BM25-ranked candidate IDs for a query.
func bm25IDs(ix *retrieval.Index, query []string) []string {
	rs := ix.Search(retrieval.Query{Terms: query}, 0)
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.ID)
	}
	return out
}

func (c Counts) add(o Counts) Counts {
	return Counts{Precision: c.Precision + o.Precision, Recall: c.Recall + o.Recall, MRR: c.MRR + o.MRR, Bytes: c.Bytes + o.Bytes}
}

func (c Counts) mean(n float64) Counts {
	return Counts{Precision: c.Precision / n, Recall: c.Recall / n, MRR: c.MRR / n, Bytes: int(float64(c.Bytes) / n)}
}

// Corpus is the deterministic representative evaluation corpus. It spans the categories
// the plan lists (single-file, multi-file, test discovery, interface implementation,
// configuration, docs+code, bug fix, cross-package dependency, replan after failure) and
// is scored against the matching candidate corpus, so the gate is hermetic and machine
// independent.
func Corpus() []Case {
	return []Case{
		{ID: "single-file", Category: "single_file", Query: []string{"parser tokenize"}, Relevant: []string{"symbol:pkg.Parser", "symbol:pkg.Tokenize"}},
		{ID: "multi-file", Category: "multi_file", Query: []string{"gate decision verify"}, Relevant: []string{"symbol:pkg.Gate", "symbol:pkg.Verify", "symbol:pkg.Decide"}},
		{ID: "test-discovery", Category: "existing_test_discovery", Query: []string{"test budget boundary"}, Relevant: []string{"file:pkg/budget_test.go"}},
		{ID: "interface", Category: "interface_implementation", Query: []string{"interface ranker"}, Relevant: []string{"symbol:pkg.Ranker", "symbol:pkg.BM25"}},
		{ID: "config", Category: "configuration_change", Query: []string{"configuration budget limits"}, Relevant: []string{"file:config.yaml", "symbol:pkg.Limits"}},
		{ID: "docs-code", Category: "documentation_and_code", Query: []string{"retrieval evaluation docs"}, Relevant: []string{"doc:docs/specs/RETRIEVAL.md", "symbol:pkg.Gate"}},
		{ID: "bug-fix", Category: "bug_fix", Query: []string{"nil deref fix"}, Relevant: []string{"symbol:pkg.Fix", "file:pkg/fix.go"}},
		{ID: "cross-package", Category: "cross_package_dependency", Query: []string{"retrieval index import"}, Relevant: []string{"symbol:index.Index", "symbol:retrieval.Ranker"}},
		{ID: "replan", Category: "replan_after_failure", Query: []string{"replan recovery context"}, Relevant: []string{"symbol:pkg.Replan", "doc:docs/specs/RECOVERY.md"}},
	}
}

// Candidates is the deterministic candidate corpus the Corpus cases are labelled against.
func Candidates() []retrieval.Candidate {
	pairs := []struct {
		id, path, text string
	}{
		{"symbol:pkg.BM25", "pkg/bm25.go", "BM25 ranker struct pkg"},
		{"symbol:pkg.Decide", "pkg/gate.go", "Decide gate decision pkg"},
		{"symbol:pkg.Fix", "pkg/fix.go", "Fix pkg"},
		{"symbol:pkg.Gate", "pkg/gate.go", "Gate struct pkg"},
		{"symbol:pkg.Limits", "pkg/limits.go", "Limits struct pkg"},
		{"symbol:pkg.Parser", "pkg/parser.go", "Parser struct pkg"},
		{"symbol:pkg.Ranker", "pkg/ranker.go", "Ranker interface pkg"},
		{"symbol:pkg.Replan", "pkg/replan.go", "Replan pkg"},
		{"symbol:pkg.Tokenize", "pkg/token.go", "Tokenize parser pkg"},
		{"symbol:pkg.Verify", "pkg/verify.go", "Verify gate pkg"},
		{"symbol:index.Index", "index/index.go", "Index struct index"},
		{"symbol:retrieval.Ranker", "retrieval/ranker.go", "Ranker interface retrieval"},
		{"file:pkg/budget_test.go", "pkg/budget_test.go", "budget_test.go test budget"},
		{"file:pkg/fix.go", "pkg/fix.go", "fix.go"},
		{"file:config.yaml", "config.yaml", "config.yaml configuration limits budget"},
		{"file:README.md", "README.md", "README.md"},
		{"doc:docs/specs/RECOVERY.md", "docs/specs/RECOVERY.md", "RECOVERY.md recovery replan spec"},
		{"doc:docs/specs/RETRIEVAL.md", "docs/specs/RETRIEVAL.md", "RETRIEVAL.md retrieval evaluation spec"},
	}
	out := make([]retrieval.Candidate, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, retrieval.Candidate{ID: p.id, Path: p.path, Text: p.text})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// String renders a compact human summary.
func (r Report) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "RETRIEVAL_GATE = %s\n", r.Decision)
	fmt.Fprintf(&b, "  cases: %d  k: %d\n", r.Cases, r.K)
	fmt.Fprintf(&b, "  baseline  precision@k=%.3f recall@k=%.3f mrr=%.3f bytes=%d\n", r.Baseline.Precision, r.Baseline.Recall, r.Baseline.MRR, r.Baseline.Bytes)
	fmt.Fprintf(&b, "  candidate precision@k=%.3f recall@k=%.3f mrr=%.3f bytes=%d\n", r.Candidate.Precision, r.Candidate.Recall, r.Candidate.MRR, r.Candidate.Bytes)
	fmt.Fprintf(&b, "  reason: %s\n", r.Reason)
	return b.String()
}
