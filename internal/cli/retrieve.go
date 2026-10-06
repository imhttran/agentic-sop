package cli

import (
	"fmt"
	"io"

	"github.com/imhttran/agentic-sop/internal/repoindex"
	"github.com/imhttran/agentic-sop/internal/retrieval"
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
	results := retrieval.New(retrieval.CandidatesFromIndex(idx)).Search(retrieval.Query{Terms: args}, 0)
	if len(results) == 0 {
		fmt.Fprintln(stdout, "no lexical matches")
		return exitOK
	}
	for _, r := range results {
		fmt.Fprintf(stdout, "%2d  %8.4f  %-8s  %s\n", r.Rank, r.Score, r.Source, r.ID)
	}
	return exitOK
}
