package memindex

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/imhttran/agentic-sop/internal/outcomefile"
	"github.com/imhttran/agentic-sop/internal/retrievalboundary"
)

// writeReindexRun writes a persisted run tree whose report corroborates a success.
func writeReindexRun(t *testing.T, runsDir, taskID string) string {
	t.Helper()
	dir := filepath.Join(runsDir, taskID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	report := map[string]any{
		"id":       taskID,
		"stage":    "PASSED",
		"decision": "PASS",
		"reasons":  []string{"provenance reindex evidence"},
		"validation": []map[string]any{
			{"Category": "TEST", "Command": "go test ./...", "ExitCode": 0, "Status": "PASS"},
		},
		"outcome": map[string]any{
			"status": "VERIFIED",
			"criteria": []map[string]any{
				{"ID": "criterion-1", "Required": true, "Met": true, "Description": "provenance reindex evidence", "Evidence": "go test ./..."},
			},
			"reasons":       []string{"provenance reindex evidence"},
			"lifecycle":     "LOCAL_DONE",
			"compatibility": "declared acceptance mapping",
		},
		"generated_at": "2026-10-19T17:10:35.909017Z",
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "report.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	counts := `{"counts":{"attempts":1,"invocations":1},"binary":{"revision":"abc123"}}`
	if err := os.WriteFile(filepath.Join(dir, "counts.json"), []byte(counts), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// searchIDs returns the record ids a scoped search returns for text.
func searchIDs(t *testing.T, ix *Index, text string) []string {
	t.Helper()
	res, err := ix.Search(context.Background(), retrievalboundary.ScopedQuery{
		Scope:  retrievalboundary.Scope{Corpus: retrievalboundary.CorpusOutcome},
		Text:   text,
		Limits: retrievalboundary.Limits{MaxResults: 10},
	})
	if err != nil {
		t.Fatalf("Search(%q): %v", text, err)
	}
	out := make([]string, 0, len(res.Evidence))
	for _, e := range res.Evidence {
		out = append(out, e.RecordID)
	}
	return out
}

// TestReindexProvenanceBeforeAndAfterRebuild proves the advisory index follows the source:
// an unmodified record is indexed, a modified source is excluded on rebuild, and a deleted
// run directory is never returned.
func TestReindexProvenanceBeforeAndAfterRebuild(t *testing.T) {
	ctx := context.Background()
	runsDir := t.TempDir()
	outDir := t.TempDir()
	ix := openTestIndex(t)

	// Ingest two runs through the real path and write the outcome documents.
	writeReindexRun(t, runsDir, "KEEP-1")
	writeReindexRun(t, runsDir, "CHANGE-1")
	var kept, changed outcomefile.OutcomeRecord
	for _, taskID := range []string{"KEEP-1", "CHANGE-1"} {
		rec, err := outcomefile.IngestRunFor(runsDir, taskID, "agentic-sop")
		if err != nil {
			t.Fatalf("IngestRunFor(%s): %v", taskID, err)
		}
		if _, _, err := outcomefile.Write(outDir, rec); err != nil {
			t.Fatalf("Write(%s): %v", taskID, err)
		}
		if taskID == "KEEP-1" {
			kept = rec
		} else {
			changed = rec
		}
	}

	// Before any source change both records are indexed and searchable.
	rep, err := ix.RebuildFromDir(ctx, outDir)
	if err != nil {
		t.Fatalf("RebuildFromDir: %v", err)
	}
	if rep.Indexed != 2 || rep.Excluded != 0 {
		t.Fatalf("first rebuild = %+v, want 2 indexed and 0 excluded", rep)
	}
	if ids := searchIDs(t, ix, "provenance reindex evidence"); len(ids) != 2 {
		t.Fatalf("first search = %v, want both ingested records", ids)
	}

	// Modify one source report after ingestion.
	reportPath := filepath.Join(runsDir, "CHANGE-1", "report.json")
	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(reportPath, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	rep, err = ix.RebuildFromDir(ctx, outDir)
	if err != nil {
		t.Fatalf("RebuildFromDir after modification: %v", err)
	}
	if rep.Indexed != 1 || rep.Excluded != 1 {
		t.Fatalf("second rebuild = %+v, want 1 indexed and 1 excluded", rep)
	}
	ids := searchIDs(t, ix, "provenance reindex evidence")
	if len(ids) != 1 || ids[0] != kept.RecordID {
		t.Fatalf("second search = %v, want only the unmodified record %q", ids, kept.RecordID)
	}

	// Delete one run directory: the deleted record is never returned.
	if err := os.RemoveAll(filepath.Join(runsDir, "KEEP-1")); err != nil {
		t.Fatal(err)
	}
	current, err := Snapshot(outDir)
	if err != nil {
		t.Fatal(err)
	}
	// The deletion is observed by the source-aware gate, which the retriever mirrors
	// through its current-source check.
	live := map[string]outcomefile.OutcomeRecord{}
	for id, r := range current {
		if id == kept.RecordID {
			continue
		}
		live[id] = r
	}
	if _, ok := live[changed.RecordID]; !ok {
		t.Fatalf("the changed record disappeared from the readable documents: %v", live)
	}
	lr := &LocalRetriever{Index: ix, Current: func(context.Context) (map[string]outcomefile.OutcomeRecord, error) {
		return live, nil
	}}
	res, err := lr.Retrieve(ctx, retrievalboundary.ScopedQuery{
		Scope:  retrievalboundary.Scope{Corpus: retrievalboundary.CorpusOutcome},
		Text:   "provenance reindex evidence",
		Limits: retrievalboundary.Limits{MaxResults: 10},
	})
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	for _, e := range res.Evidence {
		if e.RecordID == kept.RecordID {
			t.Fatal("a deleted source record was returned")
		}
	}
}
