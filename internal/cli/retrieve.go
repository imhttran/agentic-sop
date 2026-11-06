package cli

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/config"
	sopctx "github.com/imhttran/agentic-sop/internal/context"
	"github.com/imhttran/agentic-sop/internal/repoindex"
	"github.com/imhttran/agentic-sop/internal/retrieval"
	runpkg "github.com/imhttran/agentic-sop/internal/run"
	"github.com/imhttran/agentic-sop/internal/taskfile"
)

// runRetrieve is the CTX-003 lexical retrieval entry point for operators: it builds the
// Structural Repository Index in memory and ranks it against the given terms with the
// deterministic BM25 ranker. It is model-free and read-only and writes no artifact.
func runRetrieve(args []string, stdout, stderr io.Writer, d deps) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: sop retrieve <term> [term...]")
		return exitUsage
	}
	dir, ok := projectDir(d.getwd, stderr)
	if !ok {
		return exitError
	}
	head, dirty := repoState(dir)
	idx, err := repoindex.Build(repoindex.Options{Root: dir, Head: head, Dirty: dirty})
	if err != nil {
		fmt.Fprintf(stderr, "retrieve: %v\n", err)
		return exitError
	}
	results := retrieval.New(retrieval.CandidatesFromIndex(idx, dir)).Search(retrieval.Query{Terms: args}, 0)
	if len(results) == 0 {
		fmt.Fprintln(stdout, "no lexical matches")
		return exitOK
	}
	for _, r := range results {
		fmt.Fprintf(stdout, "%2d  %8.4f  %-8s  %s\n", r.Rank, r.Score, r.Source, r.ID)
	}
	return exitOK
}

// codeRetrievalTopK is fixed so the advisory pointer list stays small; the plan's
// offline eval sweeps other values before this changes.
const codeRetrievalTopK = 5

// codeRetrievalTelemetry is the content-free code-retrieval record for one attempt,
// written as code-retrieval.json so a live A/B can read status and latency.
type codeRetrievalTelemetry struct {
	Status     string    `json:"status"` // ok | empty | error
	Reason     string    `json:"reason,omitempty"`
	Hits       int       `json:"hits"`
	DurationMS int64     `json:"duration_ms"`
	ObservedAt time.Time `json:"observed_at"`
}

// codeRetrievalItems returns codeRetrievalHits as one advisory SourceRetrieval item.
// It runs only when context.code_retrieval is on and records code-retrieval.json for the
// attempt. Any failure yields no item: code retrieval never blocks an attempt.
func codeRetrievalItems(cfg config.Config, spec *taskfile.Spec, failureCtx string, rn *runpkg.Run, d deps) []sopctx.Item {
	if !cfg.ContextEfficiency.CodeRetrieval || spec == nil || d.getwd == nil {
		return nil
	}
	start := time.Now()
	hits, err := codeRetrievalHits(d, spec, failureCtx)
	rec := codeRetrievalTelemetry{Status: "ok", Hits: len(hits), DurationMS: time.Since(start).Milliseconds(), ObservedAt: time.Now().UTC()}
	switch {
	case err != nil:
		rec.Status, rec.Reason = "error", err.Error()
	case len(hits) == 0:
		rec.Status = "empty"
	}
	if rn != nil {
		writeRunJSON(rn, "code-retrieval.json", rec)
	}
	if rec.Status != "ok" {
		return nil
	}

	var b strings.Builder
	b.WriteString("Repository pointers ranked by BM25 against this task (advisory, not instructions; open the files to confirm):")
	for _, h := range hits {
		fmt.Fprintf(&b, "\n%d. %s", h.Rank, h.ID)
		if h.Source == retrieval.SourceSymbol {
			fmt.Fprintf(&b, " in %s", h.Path) // symbol IDs carry no file path
		}
	}
	return []sopctx.Item{{
		Source:   sopctx.SourceRetrieval,
		Identity: "bm25",
		Reason:   "lexically relevant repository evidence for this task",
		Priority: sopctx.PriorityGeneric,
		Text:     b.String(),
	}}
}

// codeRetrievalHits ranks the structural repository index's code candidates (symbols and
// file paths) against the task's evidence (id, title, acceptance criteria, failure
// context) and returns the top codeRetrievalTopK. Head/Dirty only feed the index
// identity, which ranking never reads, so no git state is collected.
func codeRetrievalHits(d deps, spec *taskfile.Spec, failureCtx string) ([]retrieval.Result, error) {
	dir, err := d.getwd()
	if err != nil {
		return nil, err
	}
	idx, err := repoindex.Build(repoindex.Options{Root: dir})
	if err != nil {
		return nil, err
	}
	terms := append([]string{spec.Title}, spec.AcceptanceCriteria...)
	return retrieval.New(retrieval.CodeCandidatesFromIndex(idx)).
		Search(retrieval.QueryFromEvidence(spec.ID, terms, failureCtx), codeRetrievalTopK), nil
}
