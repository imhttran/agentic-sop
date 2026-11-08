package retrievalgate

// Code-task retrieval measurement (PLAN-Code-Retrieval-Context-Seam step 4).
//
// Test-only and model-free, like EV-003, but the queries are phrased the way a real task
// title or acceptance criterion is (natural language, not hand-picked identifiers), and a
// hit counts when any top-k result points into a labelled file: the file itself or a
// symbol declared in it. Two arms are logged: "all" (every candidate, as EV-003 ranks)
// and "code" (retrieval.CodeCandidatesFromIndex), which is what the run-context seam sends.
//
// Run:  go test ./internal/retrievalgate/ -run TestCodeTaskRetrieval -v

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/repoindex"
	"github.com/imhttran/agentic-sop/internal/retrieval"
)

type codeTaskCase struct {
	ID    string
	Query string
	Files []string // repository-relative .go files a correct pointer lands in
}

func codeTaskCorpus() []codeTaskCase {
	return []codeTaskCase{
		{"webhook-backoff", "Webhook notifications should retry with exponential backoff", []string{"internal/notify/dispatch.go", "internal/notify/webhook.go"}},
		{"promptcache-eligible", "Never cache IMPLEMENT or FIX responses in the prompt cache", []string{"internal/promptcache/promptcache.go"}},
		{"memindex-rebuild", "Rebuild the advisory memory index from outcome files", []string{"internal/memindex/rebuild.go", "internal/memindex/index.go"}},
		{"question-lock", "Acquire an exclusive lock on the question store before writing", []string{"internal/question/lockapi.go", "internal/question/lock_unix.go"}},
		{"model-class", "Reject unknown model class names when parsing small, medium, large", []string{"internal/model/model.go"}},
		{"run-changed-files", "Record the files a run changed", []string{"internal/run/changes.go"}},
		{"approval-resolution", "Resolve a pending human approval for a run", []string{"internal/run/approval_resolution.go"}},
		{"redact-secrets", "Redact secret values from captured output", []string{"internal/redact/redact.go"}},
		{"rag-knowledge-items", "Convert RAG evidence into low priority untrusted context items", []string{"internal/context/rag.go"}},
		{"bm25-ties", "Make BM25 ranking break ties deterministically", []string{"internal/retrieval/retrieval.go"}},
		{"handoff-selection", "Select which dependency handoff capsules a task receives", []string{"internal/handoff/selection.go"}},
	}
}

// codeTaskFirstHit returns the 1-based rank of the first result that points into one of
// the case's files, or 0 when none does within the results.
func codeTaskFirstHit(results []retrieval.Result, files []string) int {
	for _, r := range results {
		if slices.Contains(files, r.Path) {
			return r.Rank
		}
	}
	return 0
}

// codeTaskReport scores every case against ix and renders per-case and aggregate rows.
func codeTaskReport(arm string, ix *retrieval.Index, corpus []codeTaskCase) string {
	ks := []int{3, 5, 10}
	hits := make([]int, len(ks))
	var mrr float64
	var b strings.Builder
	for _, c := range corpus {
		results := ix.Search(retrieval.Query{Terms: []string{c.Query}}, 10)
		first := codeTaskFirstHit(results, c.Files)
		for i, k := range ks {
			if first > 0 && first <= k {
				hits[i]++
			}
		}
		if first > 0 {
			mrr += 1 / float64(first)
		}
		var top []string
		for _, r := range results[:min(3, len(results))] {
			top = append(top, r.ID)
		}
		fmt.Fprintf(&b, "CODETASK arm=%s case=%s first_hit=%d top3=%v\n", arm, c.ID, first, top)
	}
	n := float64(len(corpus))
	fmt.Fprintf(&b, "CODETASK arm=%s aggregate hit@3=%.3f hit@5=%.3f hit@10=%.3f mrr@10=%.3f cases=%d",
		arm, float64(hits[0])/n, float64(hits[1])/n, float64(hits[2])/n, mrr/n, len(corpus))
	return b.String()
}

func TestCodeTaskRetrieval(t *testing.T) {
	root := ev003RepoRoot(t)
	idx, err := repoindex.Build(repoindex.Options{Root: root})
	if err != nil {
		t.Fatalf("repoindex.Build: %v", err)
	}
	corpus := codeTaskCorpus()
	for _, c := range corpus {
		for _, f := range c.Files {
			if !slices.ContainsFunc(idx.Files, func(e repoindex.File) bool { return e.Path == f }) {
				t.Fatalf("case %s labels %s, which is not in the index", c.ID, f)
			}
		}
	}
	// "all" is the EV-003 corpus; "code" is exactly what the run-context seam ranks.
	arms := map[string]*retrieval.Index{
		"all":  retrieval.New(retrieval.CandidatesFromIndex(idx, root)),
		"code": retrieval.New(retrieval.CodeCandidatesFromIndex(idx)),
	}
	for _, arm := range []string{"all", "code"} {
		out := codeTaskReport(arm, arms[arm], corpus)
		if again := codeTaskReport(arm, arms[arm], corpus); again != out {
			t.Fatalf("code-task retrieval must be deterministic:\n%s\n---\n%s", out, again)
		}
		t.Log("\n" + out)
	}
}
