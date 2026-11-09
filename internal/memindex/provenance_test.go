package memindex

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
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
	var kept outcomefile.OutcomeRecord
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

	// Delete one run directory and rebuild: the deleted record is excluded by the
	// source-aware gate and is never returned, so the index no longer carries it.
	if err := os.RemoveAll(filepath.Join(runsDir, "KEEP-1")); err != nil {
		t.Fatal(err)
	}
	rep, err = ix.RebuildFromDir(ctx, outDir)
	if err != nil {
		t.Fatalf("RebuildFromDir after deletion: %v", err)
	}
	if rep.Indexed != 0 || rep.Excluded != 2 {
		t.Fatalf("third rebuild = %+v, want 0 indexed and 2 excluded", rep)
	}
	if ids := searchIDs(t, ix, "provenance reindex evidence"); len(ids) != 0 {
		t.Fatalf("third search = %v, want no record after deletion", ids)
	}
}

// TestReindexProvenanceIgnoresMutableLifecycleState proves a run parked at the human
// boundary after capture stays indexed: the lifecycle rewrite of state.json is not
// evidence, so it does not exclude an otherwise unchanged record.
func TestReindexProvenanceIgnoresMutableLifecycleState(t *testing.T) {
	ctx := context.Background()
	runsDir := t.TempDir()
	outDir := t.TempDir()
	ix := openTestIndex(t)

	dir := writeReindexRun(t, runsDir, "PARK-1")
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{"id":"PARK-1","stage":"LOCAL_DONE"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	rec, err := outcomefile.IngestRunFor(runsDir, "PARK-1", "agentic-sop")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := outcomefile.Write(outDir, rec); err != nil {
		t.Fatal(err)
	}
	// The lifecycle parks the run after capture, rewriting state.json.
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{"id":"PARK-1","stage":"WAITING_FOR_HUMAN"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	rep, err := ix.RebuildFromDir(ctx, outDir)
	if err != nil {
		t.Fatalf("RebuildFromDir: %v", err)
	}
	if rep.Indexed != 1 || rep.Excluded != 0 {
		t.Fatalf("rebuild = %+v, want 1 indexed and 0 excluded after a lifecycle update", rep)
	}
	if ids := searchIDs(t, ix, "provenance reindex evidence"); len(ids) != 1 {
		t.Fatalf("search = %v, want the parked record", ids)
	}
}

// TestReindexProvenanceLegacyDigestlessSuccessIsDowngraded proves a success record
// written before provenance digests existed cannot re-enter the advisory index as a
// verified success. It stays usable as partial advisory history.
func TestReindexProvenanceLegacyDigestlessSuccessIsDowngraded(t *testing.T) {
	ctx := context.Background()
	runsDir := t.TempDir()
	outDir := t.TempDir()
	ix := openTestIndex(t)

	runDir := filepath.Join(runsDir, "LEGACY-1")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	reportPath := filepath.Join(runDir, "report.json")
	if err := os.WriteFile(reportPath, []byte(`{"id":"LEGACY-1"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Pre-PR-268 shape: a success record with no SourceDigests at all.
	rec := outcomefile.OutcomeRecord{
		SchemaVersion:   outcomefile.SchemaVersion,
		RecordID:        "LEGACY-1",
		RunID:           "LEGACY-1",
		TaskID:          "LEGACY-1",
		Corpus:          retrievalboundary.CorpusOutcome,
		Repository:      "agentic-sop",
		Status:          outcomefile.StatusSuccess,
		Complete:        true,
		Authorized:      true,
		EvidenceRefs:    []string{filepath.ToSlash(reportPath)},
		Verification:    []outcomefile.Verification{{Command: "go test ./...", ExitCode: 0, Result: "PASS"}},
		SourceArtifacts: []string{filepath.ToSlash(reportPath)},
		SourceHash:      "legacy-source-hash",
		RecordedAt:      "2026-10-19T17:10:35.909017Z",
		AccessScope:     "local",
		Lifecycle:       "LOCAL_DONE",
		Citation:        filepath.ToSlash(reportPath),
		Text:            "legacy digestless success evidence",
	}
	if _, _, err := outcomefile.Write(outDir, rec); err != nil {
		t.Fatal(err)
	}

	rep, err := ix.RebuildFromDir(ctx, outDir)
	if err != nil {
		t.Fatalf("RebuildFromDir: %v", err)
	}
	if rep.Indexed != 1 || rep.Excluded != 0 {
		t.Fatalf("rebuild = %+v, want the digest-less record kept as advisory history", rep)
	}
	res, err := ix.Search(ctx, retrievalboundary.ScopedQuery{
		Scope:  retrievalboundary.Scope{Corpus: retrievalboundary.CorpusOutcome},
		Text:   "legacy digestless success evidence",
		Limits: retrievalboundary.Limits{MaxResults: 10},
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Evidence) != 1 {
		t.Fatalf("search returned %d records, want the legacy record", len(res.Evidence))
	}
	if res.Evidence[0].Status != string(outcomefile.StatusPartial) {
		t.Fatalf("indexed status = %q, want partial for a digest-less success", res.Evidence[0].Status)
	}
}

// TestReindexProvenanceUnreadableArtifactIsExcluded proves RebuildFromDir excludes a
// record whose immutable artifact could not be digested at ingest, so a later change to
// that artifact cannot escape the content check.
func TestReindexProvenanceUnreadableArtifactIsExcluded(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("file permissions do not make an artifact unreadable in this environment")
	}
	ctx := context.Background()
	runsDir := t.TempDir()
	outDir := t.TempDir()
	ix := openTestIndex(t)

	dir := writeReindexRun(t, runsDir, "UNREAD-1")
	artifact := filepath.Join(dir, "report.md")
	if err := os.WriteFile(artifact, []byte("original evidence"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(artifact, 0o000); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(artifact, 0o644) }()

	rec, err := outcomefile.IngestRunFor(runsDir, "UNREAD-1", "agentic-sop")
	if err != nil {
		t.Fatalf("IngestRunFor: %v", err)
	}
	if _, _, err := outcomefile.Write(outDir, rec); err != nil {
		t.Fatal(err)
	}

	// The artifact becomes readable and changes after capture.
	if err := os.Chmod(artifact, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artifact, []byte("changed evidence"), 0o644); err != nil {
		t.Fatal(err)
	}

	rep, err := ix.RebuildFromDir(ctx, outDir)
	if err != nil {
		t.Fatalf("RebuildFromDir: %v", err)
	}
	if rep.Indexed != 0 || rep.Excluded != 1 {
		t.Fatalf("rebuild = %+v, want the unverified record excluded", rep)
	}
	if ids := searchIDs(t, ix, "provenance reindex evidence"); len(ids) != 0 {
		t.Fatalf("search = %v, want no unverified record", ids)
	}
}
