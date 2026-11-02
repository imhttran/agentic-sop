package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/rageval"
)

func TestRunRequiresProducer(t *testing.T) {
	t.Setenv("RAG_EVAL_RAG_EXECUTABLE", "")
	t.Setenv("RAG_EVAL_PRODUCER_COMMIT", "abc123")
	t.Setenv("RAG_EVAL_EMBED_MODEL", "test-embed-model")
	out := t.TempDir()
	code := run([]string{"-out", out})
	if code == 0 {
		t.Fatalf("run with empty RAG_EVAL_RAG_EXECUTABLE returned 0, want non-zero")
	}
	entries, err := os.ReadDir(out)
	if err != nil {
		t.Fatalf("read output dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("output dir %s is not empty: %d entries", out, len(entries))
	}
}

func TestHeaderRecordsProducer(t *testing.T) {
	dir := t.TempDir()
	header := rageval.Header{
		Commit:       "deadbeef",
		ManifestHash: "abc",
		Config:       rageval.Config{TopK: 5, Corpus: "sop-engineering-kb"},
		Status:       rageval.StatusPass,
		Producer: rageval.Producer{
			Executable:      "/usr/local/bin/rag-producer",
			Commit:          "producercommit123",
			EmbedModel:      "nomic-embed-text",
			ExcludedSources: []string{"docs/a.md", "docs/b.md"},
		},
	}
	res := rageval.RunResult{Status: rageval.StatusPass}
	jsonPath, _, err := rageval.WriteReports(dir, header, res)
	if err != nil {
		t.Fatalf("WriteReports: %v", err)
	}
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("read results.json: %v", err)
	}
	body := string(data)
	for _, want := range []string{"producercommit123", "nomic-embed-text", "docs/a.md", "docs/b.md"} {
		if !strings.Contains(body, want) {
			t.Errorf("results.json does not contain %q", want)
		}
	}
	var parsed struct {
		Header struct {
			Producer struct {
				Commit          string   `json:"commit"`
				EmbedModel      string   `json:"embed_model"`
				ExcludedSources []string `json:"excluded_sources"`
			} `json:"producer"`
		} `json:"header"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal results.json: %v", err)
	}
	if parsed.Header.Producer.Commit != "producercommit123" {
		t.Errorf("producer commit = %q", parsed.Header.Producer.Commit)
	}
	if parsed.Header.Producer.EmbedModel != "nomic-embed-text" {
		t.Errorf("producer embed model = %q", parsed.Header.Producer.EmbedModel)
	}
	if len(parsed.Header.Producer.ExcludedSources) != 2 {
		t.Errorf("excluded sources = %v", parsed.Header.Producer.ExcludedSources)
	}
}
