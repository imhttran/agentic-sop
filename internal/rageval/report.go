package rageval

// Report writers for the four-arm run.
//
// Both writers place their output under docs/reports/rag-eval/<timestamp>/ and
// both carry a header with the git commit, the dataset manifest hash, and the
// effective configuration. Repeated runs over the same inputs produce identical
// bodies apart from the timestamp directory name.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Producer records the real RAG producer that the run measured.
type Producer struct {
	Executable      string   `json:"executable"`
	Commit          string   `json:"commit"`
	EmbedModel      string   `json:"embed_model"`
	ExcludedSources []string `json:"excluded_sources,omitempty"`
}

// Header is the recorded run header.
type Header struct {
	Commit       string   `json:"git_commit"`
	ManifestHash string   `json:"dataset_manifest_hash"`
	Config       Config   `json:"config"`
	Status       Status   `json:"status"`
	Producer     Producer `json:"producer"`
}

// JSONResults is the JSON results file shape.
type JSONResults struct {
	Header Header `json:"header"`
	Rows   []Row  `json:"rows"`
}

// WriteReports writes results.json and results.md under dir. It creates dir,
// including parents. It returns the two file paths.
func WriteReports(dir string, header Header, res RunResult) (jsonPath, mdPath string, err error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", fmt.Errorf("rageval: create report dir: %w", err)
	}
	jsonPath = filepath.Join(dir, "results.json")
	mdPath = filepath.Join(dir, "results.md")
	body, err := json.MarshalIndent(JSONResults{Header: header, Rows: res.Rows}, "", "  ")
	if err != nil {
		return "", "", fmt.Errorf("rageval: marshal results: %w", err)
	}
	if err := os.WriteFile(jsonPath, append(body, '\n'), 0o644); err != nil {
		return "", "", fmt.Errorf("rageval: write %s: %w", jsonPath, err)
	}
	if err := os.WriteFile(mdPath, []byte(RenderMarkdown(header, res.Rows)), 0o644); err != nil {
		return "", "", fmt.Errorf("rageval: write %s: %w", mdPath, err)
	}
	return jsonPath, mdPath, nil
}

// RenderMarkdown renders the Markdown table plus header. The body is byte-stable
// for the same header and rows.
func RenderMarkdown(header Header, rows []Row) string {
	var b strings.Builder
	b.WriteString("# RAG evaluation — four-arm comparison\n\n")
	fmt.Fprintf(&b, "- git commit: %s\n", header.Commit)
	fmt.Fprintf(&b, "- dataset manifest hash: %s\n", header.ManifestHash)
	fmt.Fprintf(&b, "- corpus: %s\n", header.Config.Corpus)
	fmt.Fprintf(&b, "- top_k: %d\n", header.Config.TopK)
	fmt.Fprintf(&b, "- status: %s\n", header.Status)
	fmt.Fprintf(&b, "- producer executable: %s\n", header.Producer.Executable)
	fmt.Fprintf(&b, "- producer commit: %s\n", header.Producer.Commit)
	fmt.Fprintf(&b, "- producer embed model: %s\n", header.Producer.EmbedModel)
	fmt.Fprintf(&b, "- excluded sources: %s\n\n", joinStrings(header.Producer.ExcludedSources))
	b.WriteString("| question | category | arm | retrieved | recall@5 | precision@5 | citation | provenance | context bytes | truncated | latency ms | fallback | cross-corpus | returned | injection reached | injection below task |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s | %s | %t | %s | %s | %s | %s | %s | %s |\n",
			r.QuestionID, r.Category, r.Arm,
			joinOr(r.Retrieved), fmtFloat(r.RecallAt5), fmtFloat(r.PrecisionAt5),
			fmtBool(r.CitationCorrect), fmtBool(r.ProvenanceCorrect),
			r.ContextBytes, r.Truncated, r.LatencyMS, r.FallbackReason,
			joinStrings(r.CrossCorpus), joinStrings(r.Returned),
			fmtBool(r.InjectionReached), fmtBool(r.InjectionBelowTask),
		)
	}
	return b.String()
}

func joinOr(items []Retrieved) string {
	ids := make([]string, 0, len(items))
	for _, it := range items {
		ids = append(ids, fmt.Sprintf("%s(rank %d)", it.ID, it.Rank))
	}
	if len(ids) == 0 {
		return "—"
	}
	return strings.Join(ids, ", ")
}

func joinStrings(items []string) string {
	if len(items) == 0 {
		return "—"
	}
	return strings.Join(items, ", ")
}

func fmtFloat(v *float64) string {
	if v == nil {
		return "—"
	}
	return fmt.Sprintf("%.2f", *v)
}

func fmtBool(v *bool) string {
	if v == nil {
		return "—"
	}
	if *v {
		return "yes"
	}
	return "no"
}
