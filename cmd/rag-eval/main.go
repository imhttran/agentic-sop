// Command rag-eval validates the frozen RAG evaluation dataset and runs the
// four-arm comparison.
//
// Usage:
//
//	rag-eval validate [--dir evals/rag]
//	rag-eval run [--dir evals/rag] [--corpus sop-engineering-kb] [--top-k 5] [--excluded FILE] [--out docs/reports/rag-eval]
//
// validate loads the dataset through internal/rageval, then prints one line per
// known category with its question count. It exits 0 on success. It prints the
// loader error to stderr and exits 1 on any validation failure.
//
// run loads the dataset and corpus, evaluates arms A (BM25-only through
// internal/retrieval), B (BM25 plus RAG), C (RAG-only) and D (full-text
// retrieval, diagnostic) for every question, and writes results.json plus
// results.md under <out>/<timestamp>/. The header records the git commit, the
// dataset manifest hash, the measured producer, and the effective
// configuration. run requires RAG_EVAL_RAG_EXECUTABLE,
// RAG_EVAL_PRODUCER_COMMIT, and RAG_EVAL_EMBED_MODEL; it refuses stub
// providers and exits non-zero without writing a report when any is missing.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/imhttran/agentic-sop/internal/ragclient"
	"github.com/imhttran/agentic-sop/internal/rageval"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "validate":
		validate(os.Args[2:])
	case "run":
		os.Exit(run(os.Args[2:]))
	case "summarize":
		os.Exit(summarize(os.Args[2:]))
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "usage:\n  rag-eval validate [--dir evals/rag]\n  rag-eval run [--dir evals/rag] [--corpus sop-engineering-kb] [--top-k 5] [--excluded FILE] [--out docs/reports/rag-eval]\n  rag-eval summarize [--dir evals/rag] --runs dir,dir,dir --control dir --out dir\n")
	os.Exit(2)
}

// summarize executes the summarize subcommand and returns the process exit
// code. It only reads files: it never runs a producer, a model, or a database.
func summarize(args []string) int {
	fs := flag.NewFlagSet("summarize", flag.ExitOnError)
	dir := fs.String("dir", "evals/rag", "dataset root holding questions.json and corpus/MANIFEST.sha256")
	runs := fs.String("runs", "", "comma-separated recorded run directories, each holding results.json")
	control := fs.String("control", "", "control run directory recorded with -top-k 10, holding results.json")
	out := fs.String("out", "", "output directory for summary.json and summary.md")
	fs.Parse(args)
	if *runs == "" || *control == "" || *out == "" {
		fmt.Fprintf(os.Stderr, "rag-eval: summarize requires -runs, -control and -out\n")
		return 2
	}
	if _, err := rageval.Summarize(rageval.SummaryConfig{Dir: *dir, Runs: strings.Split(*runs, ","), Control: *control, Out: *out}); err != nil {
		fmt.Fprintf(os.Stderr, "rag-eval: %v\n", err)
		return 1
	}
	return 0
}

func validate(args []string) {
	fs := flag.NewFlagSet("validate", flag.ExitOnError)
	dir := fs.String("dir", "evals/rag", "dataset root holding questions.json and corpus/MANIFEST.sha256")
	fs.Parse(args)
	ds, err := rageval.Load(*dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rag-eval: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("dataset: %s\n", ds.Root)
	for _, c := range rageval.Categories {
		fmt.Printf("%s: %d\n", c, ds.Counts[c])
	}
	fmt.Printf("total: %d\n", len(ds.Questions))
}

// run executes the run subcommand and returns the process exit code.
func run(args []string) int {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	dir := fs.String("dir", "evals/rag", "dataset root holding questions.json and corpus/MANIFEST.sha256")
	corpus := fs.String("corpus", "sop-engineering-kb", "corpus allowlist the run is scoped to")
	topK := fs.Int("top-k", rageval.K, "number of results each arm requests")
	excluded := fs.String("excluded", "", "file with one corpus source per line that the producer could not ingest")
	out := fs.String("out", filepath.Join("docs", "reports", "rag-eval"), "output root for the timestamped report directory")
	fs.Parse(args)

	excludedSources, err := loadExcluded(*excluded)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rag-eval: %v\n", err)
		return 1
	}
	producerCommit := strings.TrimSpace(os.Getenv("RAG_EVAL_PRODUCER_COMMIT"))
	if producerCommit == "" {
		fmt.Fprintf(os.Stderr, "rag-eval: RAG_EVAL_PRODUCER_COMMIT is required\n")
		return 1
	}
	embedModel := strings.TrimSpace(os.Getenv("RAG_EVAL_EMBED_MODEL"))
	if embedModel == "" {
		fmt.Fprintf(os.Stderr, "rag-eval: RAG_EVAL_EMBED_MODEL is required\n")
		return 1
	}

	ds, err := rageval.Load(*dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rag-eval: %v\n", err)
		return 1
	}
	docs, err := rageval.LoadCorpus(ds)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rag-eval: %v\n", err)
		return 1
	}
	stateDir, err := os.MkdirTemp("", "rag-eval-repoindex-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "rag-eval: temp state dir: %v\n", err)
		return 1
	}
	defer os.RemoveAll(stateDir)
	indexA, err := rageval.BuildArmAIndex(ds, stateDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rag-eval: %v\n", err)
		return 1
	}

	cfg := rageval.Config{TopK: *topK, Corpus: *corpus}
	provider, exe, err := ragProvider(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rag-eval: %v\n", err)
		return 1
	}

	r := &rageval.Runner{
		Dataset:  ds,
		Docs:     docs,
		IndexA:   indexA,
		IndexD:   rageval.BuildArmDIndex(docs),
		Provider: provider,
		Config:   cfg,
	}
	res, runErr := r.Run(context.Background())
	if runErr != nil {
		fmt.Fprintf(os.Stderr, "rag-eval: run: %v\n", runErr)
		return 1
	}

	ts := time.Now().UTC().Format("20060102T150405Z")
	reportDir := filepath.Join(*out, ts)
	header := rageval.Header{
		Commit:       gitCommit(),
		ManifestHash: rageval.ManifestHash(ds.Manifest),
		Config:       cfg,
		Status:       res.Status,
		Producer: rageval.Producer{
			Executable:      exe,
			Commit:          producerCommit,
			EmbedModel:      embedModel,
			ExcludedSources: excludedSources,
		},
	}
	jsonPath, mdPath, err := rageval.WriteReports(reportDir, header, res)
	if err != nil {
		fmt.Fprintf(os.Stderr, "rag-eval: %v\n", err)
		return 1
	}
	fmt.Printf("status: %s\n", res.Status)
	fmt.Printf("json: %s\n", jsonPath)
	fmt.Printf("markdown: %s\n", mdPath)
	for _, blocker := range res.HardBlockers {
		fmt.Fprintf(os.Stderr, "hard-blocker: %s\n", blocker)
	}
	if res.Status == rageval.StatusHardBlocker {
		return 1
	}
	return 0
}

// loadExcluded reads one corpus source per line from path. An empty path
// yields an empty slice.
func loadExcluded(path string) ([]string, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("rag-eval: read excluded file: %w", err)
	}
	sources := []string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			sources = append(sources, line)
		}
	}
	return sources, nil
}

// ragProvider builds the RAG provider from the executable named by
// RAG_EVAL_RAG_EXECUTABLE. An empty value is an error: the run never falls
// back to a stub provider.
func ragProvider(cfg rageval.Config) (rageval.RAGProvider, string, error) {
	exe := strings.TrimSpace(os.Getenv("RAG_EVAL_RAG_EXECUTABLE"))
	if exe == "" {
		return nil, "", fmt.Errorf("RAG_EVAL_RAG_EXECUTABLE is required: refusing stub provider")
	}
	client, err := ragclient.New(ragclient.Config{
		Executable:     exe,
		AllowedCorpora: []string{cfg.Corpus},
		MaxResults:     cfg.TopK,
	})
	if err != nil {
		return nil, "", err
	}
	return rageval.RagClientProvider{Client: client}, exe, nil
}

// gitCommit returns the current HEAD commit, or an explicit marker when git is
// unavailable.
func gitCommit() string {
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return "unavailable"
	}
	commit := strings.TrimSpace(string(out))
	if commit == "" {
		return "unavailable"
	}
	return commit
}
