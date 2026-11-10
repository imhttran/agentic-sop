package memindex

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/imhttran/agentic-sop/internal/outcomefile"
	"github.com/imhttran/agentic-sop/internal/retrievalboundary"
)

// llm09Sources writes one source artifact and returns its absolute path and digest so a
// record can be ingested with a valid SourceDigests entry. The caller rewrites or deletes
// it to model a stale source.
func llm09Sources(t *testing.T, dir, name, body string) (string, string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir sources: %v", err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write source %s: %v", name, err)
	}
	digest, err := outcomefile.FileSourceDigest(path)
	if err != nil {
		t.Fatalf("digest source %s: %v", name, err)
	}
	return path, digest
}

// llm09Index writes one outcome record with a source artifact and digest, rebuilds the
// index, and returns the open index, the outcomes directory and the source directory for
// read-path checks.
func llm09Index(t *testing.T, root string, rec outcomefile.OutcomeRecord, artifactName, artifactBody string) (*Index, string, string) {
	t.Helper()
	outcomes := filepath.Join(root, "outcomes")
	srcDir := filepath.Join(root, "sources")
	path, digest := llm09Sources(t, srcDir, artifactName, artifactBody)
	rec.SourceArtifacts = []string{filepath.ToSlash(path)}
	rec.SourceDigests = map[string]string{filepath.ToSlash(path): digest}
	if _, _, err := outcomefile.Write(outcomes, rec); err != nil {
		t.Fatalf("write outcome: %v", err)
	}
	ix, err := Open(filepath.Join(root, "index.db"))
	if err != nil {
		t.Fatalf("open index: %v", err)
	}
	if _, err := ix.RebuildFromDir(context.Background(), outcomes); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	return ix, outcomes, srcDir
}

// llm09Retrieve runs one scoped query through the real local retriever with the gated
// read-path snapshot.
func llm09Retrieve(t *testing.T, ix *Index, outcomes, repository, text string) []retrievalboundary.Evidence {
	t.Helper()
	lr := &LocalRetriever{
		Index: ix,
		Current: func(ctx context.Context) (map[string]outcomefile.OutcomeRecord, error) {
			return SnapshotGatedContext(ctx, outcomes)
		},
	}
	q := retrievalboundary.ScopedQuery{
		Scope: retrievalboundary.Scope{Corpus: retrievalboundary.CorpusOutcome, Repository: repository},
		Text:  text,
		Limits: retrievalboundary.Limits{
			MaxResults:  10,
			TokenBudget: 4000,
		},
	}
	res, err := lr.Retrieve(context.Background(), q)
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	return res.Evidence
}

func llm09Record(id, repository, text string) outcomefile.OutcomeRecord {
	return outcomefile.OutcomeRecord{
		RecordID:   id,
		Corpus:     retrievalboundary.CorpusOutcome,
		Repository: repository,
		TaskID:     "TASK-" + id,
		Status:     outcomefile.StatusSuccess,
		// A success claim needs recorded evidence to be eligible.
		Verification: []outcomefile.Verification{{Command: "go test ./...", ExitCode: 0, Result: "PASS"}},
		Complete:     true,
		Authorized:   true,
		Citation:     "run/" + id,
		Text:         text,
	}
}

func llm09IDs(ev []retrievalboundary.Evidence) []string {
	out := make([]string, 0, len(ev))
	for _, e := range ev {
		out = append(out, e.RecordID)
	}
	return out
}

func llm09Contains(ev []retrievalboundary.Evidence, id string) bool {
	for _, e := range ev {
		if e.RecordID == id {
			return true
		}
	}
	return false
}

// TestLLM09CrossProjectScope proves that a query favoring the other project's canary
// returns nothing from the other project when the trusted repository is this project.
func TestLLM09CrossProjectScope(t *testing.T) {
	root := t.TempDir()
	projectA := llm09Record("A-1", "project-alpha", "alpha canary CANARY-ALPHA retry fix")
	ixA, outcomesA, _ := llm09Index(t, root, projectA, "alpha.txt", "alpha artifact")
	defer ixA.Close()

	projectB := llm09Record("B-1", "project-beta", "beta canary CANARY-BETA retry fix")
	ixB, outcomesB, _ := llm09Index(t, filepath.Join(root, "beta"), projectB, "beta.txt", "beta artifact")
	defer ixB.Close()

	// A query that favors the other project's canary returns only project A's own
	// records (A-1 shares the ordinary "retry fix" terms); B-1 never appears.
	got := llm09Retrieve(t, ixA, outcomesA, "project-alpha", "CANARY-BETA retry fix")
	if llm09Contains(got, "B-1") {
		t.Fatalf("project A returned cross-project evidence B-1: %v", llm09IDs(got))
	}
	gotA := llm09Retrieve(t, ixA, outcomesA, "project-alpha", "CANARY-ALPHA retry fix")
	if !llm09Contains(gotA, "A-1") {
		t.Fatalf("valid in-scope control missing: %v", llm09IDs(gotA))
	}
	gotB := llm09Retrieve(t, ixB, outcomesB, "project-beta", "CANARY-ALPHA retry fix")
	if llm09Contains(gotB, "A-1") {
		t.Fatalf("project beta returned cross-project evidence A-1")
	}
}

// TestLLM09SharedDocumentNames proves two projects with identical document names return
// only their own canary.
func TestLLM09SharedDocumentNames(t *testing.T) {
	root := t.TempDir()
	text := "shared document name doc-common retry fix"
	recA := llm09Record("DOC-A", "project-alpha", "CANARY-ALPHA "+text)
	ixA, outcomesA, _ := llm09Index(t, root, recA, "doc.txt", "alpha doc")
	defer ixA.Close()
	recB := llm09Record("DOC-B", "project-beta", "CANARY-BETA "+text)
	ixB, outcomesB, _ := llm09Index(t, filepath.Join(root, "beta"), recB, "doc.txt", "beta doc")
	defer ixB.Close()

	got := llm09Retrieve(t, ixA, outcomesA, "project-alpha", "doc-common retry fix")
	if !llm09Contains(got, "DOC-A") {
		t.Fatalf("shared-name in-scope record missing: %v", llm09IDs(got))
	}
	if llm09Contains(got, "DOC-B") {
		t.Fatalf("shared-name cross-project record leaked: %v", llm09IDs(got))
	}
	gotB := llm09Retrieve(t, ixB, outcomesB, "project-beta", "doc-common retry fix")
	if !llm09Contains(gotB, "DOC-B") || llm09Contains(gotB, "DOC-A") {
		t.Fatalf("project beta scope wrong: %v", llm09IDs(gotB))
	}
}

// TestLLM09ModifiedSourceExcluded proves a modified source artifact excludes the record
// from retrieval before a manual rebuild via the gated read path.
func TestLLM09ModifiedSourceExcluded(t *testing.T) {
	root := t.TempDir()
	rec := llm09Record("M-1", "project-alpha", "modified source CANARY-ALPHA retry fix")
	ix, outcomes, srcDir := llm09Index(t, root, rec, "mod.txt", "original content")
	defer ix.Close()

	got := llm09Retrieve(t, ix, outcomes, "project-alpha", "CANARY-ALPHA retry fix")
	if !llm09Contains(got, "M-1") {
		t.Fatalf("fresh record not retrievable: %v", llm09IDs(got))
	}
	if err := os.WriteFile(filepath.Join(srcDir, "mod.txt"), []byte("tampered content"), 0o644); err != nil {
		t.Fatalf("rewrite source: %v", err)
	}
	got2 := llm09Retrieve(t, ix, outcomes, "project-alpha", "CANARY-ALPHA retry fix")
	if llm09Contains(got2, "M-1") {
		t.Fatalf("modified source still retrievable before rebuild: %v", llm09IDs(got2))
	}
}

// TestLLM09DeletedSourceExcluded proves a deleted source artifact excludes the record
// from retrieval before a manual rebuild.
func TestLLM09DeletedSourceExcluded(t *testing.T) {
	root := t.TempDir()
	rec := llm09Record("D-1", "project-alpha", "deleted source CANARY-ALPHA retry fix")
	ix, outcomes, srcDir := llm09Index(t, root, rec, "del.txt", "content")
	defer ix.Close()

	if err := os.Remove(filepath.Join(srcDir, "del.txt")); err != nil {
		t.Fatalf("remove source: %v", err)
	}
	got := llm09Retrieve(t, ix, outcomes, "project-alpha", "CANARY-ALPHA retry fix")
	if llm09Contains(got, "D-1") {
		t.Fatalf("deleted source still retrievable before rebuild: %v", llm09IDs(got))
	}
}

// TestLLM09RebuildScopeParity proves a scoped query preserves the scope: after a
// rebuild the other project's records remain invisible and in-scope records are still
// returned. It does not prove import isolation; see
// TestLLM09RebuildImportVersusQueryIsolation for the direct row-count check.
func TestLLM09RebuildScopeParity(t *testing.T) {
	root := t.TempDir()
	recA := llm09Record("R-A", "project-alpha", "rebuild CANARY-ALPHA retry fix")
	ix, outcomes, srcDir := llm09Index(t, root, recA, "a.txt", "alpha")
	defer ix.Close()
	recB := llm09Record("R-B", "project-beta", "rebuild CANARY-BETA retry fix")
	pathB, digestB := llm09Sources(t, srcDir, "b.txt", "beta")
	recB.SourceArtifacts = []string{filepath.ToSlash(pathB)}
	recB.SourceDigests = map[string]string{filepath.ToSlash(pathB): digestB}
	if _, _, err := outcomefile.Write(outcomes, recB); err != nil {
		t.Fatalf("write beta outcome: %v", err)
	}
	if _, err := ix.RebuildFromDir(context.Background(), outcomes); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	got := llm09Retrieve(t, ix, outcomes, "project-alpha", "CANARY-ALPHA CANARY-BETA retry rebuild")
	if !llm09Contains(got, "R-A") {
		t.Fatalf("in-scope record missing after rebuild: %v", llm09IDs(got))
	}
	if llm09Contains(got, "R-B") {
		t.Fatalf("cross-repository record leaked after rebuild: %v", llm09IDs(got))
	}
}

// TestLLM09RebuildImportVersusQueryIsolation separates two properties that the
// earlier parity test conflated: (1) RebuildFromDir imports every eligible record in
// the outcomes directory it is given, without applying a repository filter, and
// (2) Index.Search enforces the trusted repository at query time. A shared outcomes
// directory therefore imports cross-repository rows that a scoped query cannot return;
// a workspace-local outcomes directory imports only that workspace's rows.
func TestLLM09RebuildImportVersusQueryIsolation(t *testing.T) {
	root := t.TempDir()
	outcomes := filepath.Join(root, "outcomes")
	srcDir := filepath.Join(root, "sources")

	writeRecord := func(rec outcomefile.OutcomeRecord, name, body string) {
		t.Helper()
		path, digest := llm09Sources(t, srcDir, name, body)
		rec.SourceArtifacts = []string{filepath.ToSlash(path)}
		rec.SourceDigests = map[string]string{filepath.ToSlash(path): digest}
		if _, _, err := outcomefile.Write(outcomes, rec); err != nil {
			t.Fatalf("write outcome %s: %v", rec.RecordID, err)
		}
	}

	recA := llm09Record("IMP-A", "project-alpha", "import isolation shared-name CANARY-ALPHA retry fix")
	recB := llm09Record("IMP-B", "project-beta", "import isolation shared-name CANARY-BETA retry fix")
	writeRecord(recA, "alpha-shared.txt", "alpha")
	writeRecord(recB, "beta-shared.txt", "beta")

	ix, err := Open(filepath.Join(root, "shared-index.db"))
	if err != nil {
		t.Fatalf("open shared index: %v", err)
	}
	defer ix.Close()

	rep, err := ix.RebuildFromDir(context.Background(), outcomes)
	if err != nil {
		t.Fatalf("rebuild shared outcomes: %v", err)
	}
	if rep.Indexed != 2 {
		t.Fatalf("rebuild imported %d rows, want 2: import is directory-scoped, not repository-scoped", rep.Indexed)
	}
	st, err := ix.Status(context.Background())
	if err != nil {
		t.Fatalf("shared status: %v", err)
	}
	if st.Records != 2 {
		t.Fatalf("shared index holds %d rows, want 2", st.Records)
	}
	// Measure the imported rows directly from the records table, independent of
	// Index.Search: the shared index physically holds both repositories' rows.
	sharedRows, err := ix.HashRows(context.Background())
	if err != nil {
		t.Fatalf("shared hash rows: %v", err)
	}
	if _, ok := sharedRows["IMP-A"]; !ok {
		t.Fatalf("shared index is missing in-scope row IMP-A: %v", sharedRows)
	}
	if _, ok := sharedRows["IMP-B"]; !ok {
		t.Fatalf("shared index did not import cross-repository row IMP-B: %v", sharedRows)
	}

	// The cross-repository row is physically present, but a scoped query must not
	// return it. This is query isolation, not import isolation.
	got := llm09Retrieve(t, ix, outcomes, "project-alpha", "import isolation CANARY-ALPHA CANARY-BETA retry fix")
	if !llm09Contains(got, "IMP-A") {
		t.Fatalf("in-scope record missing from shared index query: %v", llm09IDs(got))
	}
	if llm09Contains(got, "IMP-B") {
		t.Fatalf("cross-repository record returned from shared index: %v", llm09IDs(got))
	}

	// A per-workspace outcomes directory gives the import-isolation property: only
	// alpha's row is imported into alpha's index.
	alphaOutcomes := filepath.Join(root, "alpha-outcomes")
	if _, _, err := outcomefile.Write(alphaOutcomes, recA); err != nil {
		t.Fatalf("write alpha outcome: %v", err)
	}
	ixA, err := Open(filepath.Join(root, "alpha-index.db"))
	if err != nil {
		t.Fatalf("open alpha index: %v", err)
	}
	defer ixA.Close()
	repA, err := ixA.RebuildFromDir(context.Background(), alphaOutcomes)
	if err != nil {
		t.Fatalf("rebuild alpha outcomes: %v", err)
	}
	if repA.Indexed != 1 {
		t.Fatalf("alpha-only rebuild imported %d rows, want 1", repA.Indexed)
	}
	stA, err := ixA.Status(context.Background())
	if err != nil {
		t.Fatalf("alpha status: %v", err)
	}
	if stA.Records != 1 {
		t.Fatalf("alpha index holds %d rows, want 1", stA.Records)
	}
	alphaRows, err := ixA.HashRows(context.Background())
	if err != nil {
		t.Fatalf("alpha hash rows: %v", err)
	}
	if len(alphaRows) != 1 {
		t.Fatalf("alpha index imported %d rows, want 1: %v", len(alphaRows), alphaRows)
	}
	if _, ok := alphaRows["IMP-A"]; !ok {
		t.Fatalf("alpha index is missing its own row IMP-A: %v", alphaRows)
	}
	if _, ok := alphaRows["IMP-B"]; ok {
		t.Fatalf("alpha-only directory imported cross-repository row IMP-B: %v", alphaRows)
	}
	gotA := llm09Retrieve(t, ixA, alphaOutcomes, "project-alpha", "import isolation CANARY-ALPHA CANARY-BETA retry fix")
	if !llm09Contains(gotA, "IMP-A") || llm09Contains(gotA, "IMP-B") {
		t.Fatalf("alpha-only index query = %v, want only IMP-A", llm09IDs(gotA))
	}
}

// TestLLM09RebuildModifiedAndDeletedSourceWithHistoricalControl proves the rebuild
// lifecycle on synthetic fixtures: after a source is modified and another is deleted,
// the gated read path and a subsequent rebuild exclude both while a valid historical
// record remains retrievable as the positive control.
func TestLLM09RebuildModifiedAndDeletedSourceWithHistoricalControl(t *testing.T) {
	root := t.TempDir()
	outcomes := filepath.Join(root, "outcomes")
	srcDir := filepath.Join(root, "sources")

	writeRecord := func(rec outcomefile.OutcomeRecord, name, body string) {
		t.Helper()
		path, digest := llm09Sources(t, srcDir, name, body)
		rec.SourceArtifacts = []string{filepath.ToSlash(path)}
		rec.SourceDigests = map[string]string{filepath.ToSlash(path): digest}
		if _, _, err := outcomefile.Write(outcomes, rec); err != nil {
			t.Fatalf("write outcome %s: %v", rec.RecordID, err)
		}
	}

	writeRecord(llm09Record("HIST-KEEP", "project-alpha", "rebuild lifecycle CANARY-KEEP retry fix"), "keep.txt", "keep content")
	writeRecord(llm09Record("HIST-MOD", "project-alpha", "rebuild lifecycle CANARY-MOD retry fix"), "mod.txt", "original content")
	writeRecord(llm09Record("HIST-DEL", "project-alpha", "rebuild lifecycle CANARY-DEL retry fix"), "del.txt", "deleted content")

	ix, err := Open(filepath.Join(root, "index.db"))
	if err != nil {
		t.Fatalf("open index: %v", err)
	}
	defer ix.Close()

	rep, err := ix.RebuildFromDir(context.Background(), outcomes)
	if err != nil {
		t.Fatalf("initial rebuild: %v", err)
	}
	if rep.Indexed != 3 {
		t.Fatalf("initial rebuild indexed %d, want 3", rep.Indexed)
	}
	if got := llm09Retrieve(t, ix, outcomes, "project-alpha", "rebuild lifecycle retry fix"); len(got) != 3 {
		t.Fatalf("initial query = %v, want all three synthetic records", llm09IDs(got))
	}

	if err := os.WriteFile(filepath.Join(srcDir, "mod.txt"), []byte("tampered content"), 0o644); err != nil {
		t.Fatalf("modify source: %v", err)
	}
	if err := os.Remove(filepath.Join(srcDir, "del.txt")); err != nil {
		t.Fatalf("delete source: %v", err)
	}

	// Before rebuild: the gated read path excludes the modified and deleted sources
	// while the untouched historical record remains the positive control.
	got := llm09Retrieve(t, ix, outcomes, "project-alpha", "rebuild lifecycle retry fix")
	if !llm09Contains(got, "HIST-KEEP") {
		t.Fatalf("valid historical control missing before rebuild: %v", llm09IDs(got))
	}
	if llm09Contains(got, "HIST-MOD") || llm09Contains(got, "HIST-DEL") {
		t.Fatalf("stale source still returned before rebuild: %v", llm09IDs(got))
	}

	// After rebuild: the source-aware gate keeps only the valid historical record.
	rep, err = ix.RebuildFromDir(context.Background(), outcomes)
	if err != nil {
		t.Fatalf("rebuild after source changes: %v", err)
	}
	if rep.Indexed != 1 || rep.Excluded != 2 {
		t.Fatalf("rebuild = %+v, want 1 indexed and 2 excluded", rep)
	}
	st, err := ix.Status(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if st.Records != 1 {
		t.Fatalf("rebuilt index holds %d rows, want 1", st.Records)
	}
	got = llm09Retrieve(t, ix, outcomes, "project-alpha", "rebuild lifecycle retry fix")
	if !llm09Contains(got, "HIST-KEEP") {
		t.Fatalf("valid historical control missing after rebuild: %v", llm09IDs(got))
	}
	if llm09Contains(got, "HIST-MOD") || llm09Contains(got, "HIST-DEL") {
		t.Fatalf("stale source still returned after rebuild: %v", llm09IDs(got))
	}
}
