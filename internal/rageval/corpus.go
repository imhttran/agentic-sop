package rageval

// Corpus loader for the four comparison arms.
//
// LoadCorpus walks the manifest-pinned corpus files under <root>/corpus and
// returns one CorpusDoc per file, in manifest order. It reads only files the
// manifest lists, so every arm scores exactly the verified corpus. The identity
// of each document is its corpus-relative slash path, which matches the
// identities the ragclient evidence and the labelled expectations use.

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/imhttran/agentic-sop/internal/repoindex"
	"github.com/imhttran/agentic-sop/internal/retrieval"
)

// CorpusDoc is one manifest-pinned corpus document: its corpus-relative slash
// path and its full file text.
type CorpusDoc struct {
	Path string
	Text string
}

// LoadCorpus reads every manifest entry of ds and returns its documents in
// manifest order.
func LoadCorpus(ds *Dataset) ([]CorpusDoc, error) {
	if ds == nil {
		return nil, fmt.Errorf("rageval: LoadCorpus: nil dataset")
	}
	if len(ds.Manifest) == 0 {
		return nil, fmt.Errorf("rageval: LoadCorpus: empty manifest")
	}
	root := CorpusRoot(ds)
	docs := make([]CorpusDoc, 0, len(ds.Manifest))
	for _, e := range ds.Manifest {
		full := filepath.Join(root, filepath.FromSlash(e.Path))
		raw, err := os.ReadFile(full)
		if err != nil {
			return nil, fmt.Errorf("rageval: LoadCorpus: %s: %w", e.Path, err)
		}
		docs = append(docs, CorpusDoc{Path: e.Path, Text: string(raw)})
	}
	return docs, nil
}

// CorpusRoot returns the absolute-or-relative directory that holds the corpus
// files of ds.
func CorpusRoot(ds *Dataset) string {
	return filepath.Join(ds.Root, "corpus")
}

// BuildArmAIndex builds the arm A index exactly as SOP does: repoindex.Build
// over each corpus repository (<owner>/<repo> under the corpus directory), its
// document candidates through retrieval.CandidatesFromIndex, and one
// retrieval.New over all of them. Building per repository makes
// repository-relative rules (docs/history/ authority) apply as they do when SOP
// runs in that repository. Non-document candidates are dropped and every
// identity is re-prefixed with its repository, so it is a corpus path.
func BuildArmAIndex(ds *Dataset, stateDir string) (*retrieval.Index, error) {
	root := CorpusRoot(ds)
	repos, err := filepath.Glob(filepath.Join(root, "*", "*"))
	if err != nil {
		return nil, fmt.Errorf("rageval: build arm A index: %w", err)
	}
	var docs []retrieval.Candidate
	for _, repo := range repos {
		if fi, err := os.Stat(repo); err != nil || !fi.IsDir() {
			continue
		}
		idx, err := repoindex.Build(repoindex.Options{Root: repo, StateDir: stateDir})
		if err != nil {
			return nil, fmt.Errorf("rageval: build arm A index: %w", err)
		}
		rel, err := filepath.Rel(root, repo)
		if err != nil {
			return nil, fmt.Errorf("rageval: build arm A index: %w", err)
		}
		prefix := filepath.ToSlash(rel) + "/"
		for _, c := range retrieval.CandidatesFromIndex(idx, repo) {
			if c.Source == retrieval.SourceDocument {
				c.ID, c.Path = "doc:"+prefix+c.Path, prefix+c.Path
				docs = append(docs, c)
			}
		}
	}
	if len(docs) == 0 {
		return nil, fmt.Errorf("rageval: build arm A index: no documents under %s/<owner>/<repo>", root)
	}
	return retrieval.New(docs), nil
}

// BuildArmDIndex builds the diagnostic arm D index over the full file text of
// every corpus document.
func BuildArmDIndex(docs []CorpusDoc) *retrieval.Index {
	cands := make([]retrieval.Candidate, 0, len(docs))
	for _, d := range docs {
		cands = append(cands, retrieval.Candidate{
			ID:     "doc:" + d.Path,
			Source: retrieval.SourceDocument,
			Path:   d.Path,
			Text:   d.Text,
		})
	}
	return retrieval.New(cands)
}

// searchResults turns retrieval results into ranked Retrieved items. The
// evidence identity is Result.Path, relative to the corpus root, and the corpus
// is the question's permitted corpus.
func searchResults(results []retrieval.Result, corpus string) []Retrieved {
	out := make([]Retrieved, 0, len(results))
	for _, r := range results {
		out = append(out, Retrieved{ID: r.Path, Rank: r.Rank, Corpus: corpus})
	}
	return out
}
