package memindex

import (
	"context"
	"errors"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/imhttran/agentic-sop/internal/outcomefile"
	"github.com/imhttran/agentic-sop/internal/retrievalboundary"
)

func openTestIndex(t *testing.T) *Index {
	t.Helper()
	ix, err := Open(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { ix.Close() })
	return ix
}

func testRecord(id, text string) Record {
	return Record{
		RecordID:    id,
		ContentHash: "hash-" + id,
		Corpus:      retrievalboundary.CorpusOutcome,
		Repository:  "agentic-sop",
		TaskID:      "RDM-002",
		Lifecycle:   "LOCAL_DONE",
		Eligible:    true,
		Citation:    "run/" + id,
		Text:        text,
	}
}

func TestLocalIngestAndRetrieveWithoutPostgres(t *testing.T) {
	ctx := context.Background()
	ix := openTestIndex(t)
	indexed, _, _, err := ix.Rebuild(ctx, []Record{
		testRecord("outcome-a", "failure signature: ollama returned an empty response"),
		testRecord("outcome-b", "verified correction: retried the transient provider failure"),
	})
	if err != nil {
		t.Fatalf("Rebuild: %v", err)
	}
	if indexed != 2 {
		t.Fatalf("indexed = %d, want 2", indexed)
	}
	res, err := ix.Search(ctx, retrievalboundary.ScopedQuery{
		Scope: retrievalboundary.Scope{Corpus: retrievalboundary.CorpusOutcome},
		Text:  "empty response",
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(res.Evidence) != 1 || res.Evidence[0].RecordID != "outcome-a" {
		t.Fatalf("evidence = %+v, want outcome-a", res.Evidence)
	}
}

// TestLocalIndexHasNoPostgresDependency is the deterministic check that the local path
// never initializes a PostgreSQL client: no non-test file in this package imports a
// PostgreSQL driver or client package.
func TestLocalIndexHasNoPostgresDependency(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	checked := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		checked++
		for _, imp := range f.Imports {
			p := strings.Trim(imp.Path.Value, `"`)
			if strings.Contains(p, "pgx") || strings.Contains(p, "lib/pq") || strings.Contains(strings.ToLower(p), "postgres") {
				t.Errorf("%s imports %q: the local index must not depend on PostgreSQL", name, p)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no package files were inspected")
	}
}

func TestSearchHandlesExactErrorsTaskIDsAndPunctuation(t *testing.T) {
	ctx := context.Background()
	ix := openTestIndex(t)
	if _, _, _, err := ix.Rebuild(ctx, []Record{
		testRecord("outcome-a", "RDM-002B failed with: syntax error at or near \"SELECT\" (SQLSTATE 42601)"),
	}); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{
		"RDM-002B",
		"syntax error at or near",
		"\"SELECT\"",
		"SQLSTATE 42601",
		"NEAR AND OR NOT *",
		"no-such-token-anywhere",
	} {
		if _, err := ix.Search(ctx, retrievalboundary.ScopedQuery{
			Scope: retrievalboundary.Scope{Corpus: retrievalboundary.CorpusOutcome},
			Text:  text,
		}); err != nil {
			t.Errorf("Search(%q) returned %v; user text must never break FTS syntax", text, err)
		}
	}
	if _, err := ix.Search(ctx, retrievalboundary.ScopedQuery{
		Scope: retrievalboundary.Scope{Corpus: retrievalboundary.CorpusOutcome},
		Text:  "   ",
	}); err == nil || !strings.Contains(err.Error(), "empty query") {
		t.Errorf("empty query error = %v, want a typed empty-query error", err)
	}
}

func TestDuplicateIngestionIsIdempotent(t *testing.T) {
	ctx := context.Background()
	ix := openTestIndex(t)
	rec := testRecord("outcome-a", "same body")
	for i := 0; i < 3; i++ {
		if err := ix.Upsert(ctx, rec); err != nil {
			t.Fatalf("Upsert %d: %v", i, err)
		}
	}
	st, err := ix.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.Records != 1 || st.Eligible != 1 {
		t.Fatalf("status = %+v, want one eligible record", st)
	}
}

func TestInterruptedIngestionPreservesRecords(t *testing.T) {
	ctx := context.Background()
	src := t.TempDir()
	writeOutcome(t, src, outcomefile.OutcomeRecord{
		RecordID: "complete", Status: outcomefile.StatusPartial, Complete: true, Authorized: true,
		Repository: "agentic-sop", TaskID: "RDM-002", Citation: "run/complete", Text: "failure signature alpha",
	})
	// A writer interrupted mid-write leaves only a non-document temporary file.
	if err := os.WriteFile(filepath.Join(src, ".outcome-interrupted.tmp"), []byte(`{"body":{"record_id":"interrupted"`), 0o644); err != nil {
		t.Fatal(err)
	}

	ix := openTestIndex(t)
	for i := 0; i < 2; i++ {
		rep, err := ix.RebuildFromDir(ctx, src)
		if err != nil {
			t.Fatalf("rebuild %d: %v", i, err)
		}
		if rep.Indexed != 1 || len(rep.Rejected) != 0 {
			t.Fatalf("rebuild %d = %+v, want one indexed record and no rejected documents", i, rep)
		}
	}
	st, err := ix.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if st.Records != 1 {
		t.Fatalf("records = %d, want 1 after repeated and interrupted ingestion", st.Records)
	}
}

func TestRebuildIsDeterministic(t *testing.T) {
	ctx := context.Background()
	ix := openTestIndex(t)
	records := []Record{
		testRecord("b", "shared failure signature alpha"),
		testRecord("a", "shared failure signature alpha"),
		testRecord("c", "shared failure signature alpha"),
	}
	var first []string
	for attempt := 0; attempt < 2; attempt++ {
		if _, _, _, err := ix.Rebuild(ctx, records); err != nil {
			t.Fatal(err)
		}
		res, err := ix.Search(ctx, retrievalboundary.ScopedQuery{
			Scope:  retrievalboundary.Scope{Corpus: retrievalboundary.CorpusOutcome},
			Text:   "shared failure signature",
			Limits: retrievalboundary.Limits{MaxResults: 10},
		})
		if err != nil {
			t.Fatal(err)
		}
		var ids []string
		for _, e := range res.Evidence {
			ids = append(ids, e.RecordID)
		}
		if attempt == 0 {
			first = ids
			continue
		}
		if strings.Join(first, ",") != strings.Join(ids, ",") {
			t.Fatalf("rebuild changed ordering: %v vs %v", first, ids)
		}
	}
}

func writeOutcome(t *testing.T, dir string, rec outcomefile.OutcomeRecord) {
	t.Helper()
	rec.Corpus = retrievalboundary.CorpusOutcome
	rec.ContentHash = ""
	if _, _, err := outcomefile.Write(dir, rec); err != nil {
		t.Fatal(err)
	}
}

func TestStaleEvidenceIsNotReturned(t *testing.T) {
	ctx := context.Background()
	src := t.TempDir()
	writeOutcome(t, src, outcomefile.OutcomeRecord{
		RecordID: "live", Status: outcomefile.StatusPartial, Complete: true, Authorized: true,
		Repository: "agentic-sop", TaskID: "RDM-002", Citation: "run/live", Text: "failure signature alpha",
	})
	writeOutcome(t, src, outcomefile.OutcomeRecord{
		RecordID: "superseded", Status: outcomefile.StatusPartial, Complete: true, Authorized: true, Superseded: true,
		Repository: "agentic-sop", TaskID: "RDM-002", Citation: "run/superseded", Text: "failure signature alpha",
	})
	writeOutcome(t, src, outcomefile.OutcomeRecord{
		RecordID: "removed", Status: outcomefile.StatusPartial, Complete: true, Authorized: true,
		Repository: "agentic-sop", TaskID: "RDM-002", Citation: "run/removed", Text: "failure signature alpha",
	})

	ix := openTestIndex(t)
	rep, err := ix.RebuildFromDir(ctx, src)
	if err != nil {
		t.Fatalf("RebuildFromDir: %v", err)
	}
	if rep.Indexed != 2 || rep.Excluded != 1 {
		t.Fatalf("report = %+v, want 2 indexed and 1 excluded", rep)
	}

	// Delete one source record: a stale index row must not be returned.
	current, err := Snapshot(src)
	if err != nil {
		t.Fatal(err)
	}
	delete(current, "removed")

	lr := &LocalRetriever{Index: ix, Current: func(context.Context) (map[string]outcomefile.OutcomeRecord, error) {
		return current, nil
	}}
	res, err := lr.Retrieve(ctx, retrievalboundary.ScopedQuery{
		Scope:  retrievalboundary.Scope{Corpus: retrievalboundary.CorpusOutcome},
		Text:   "failure signature",
		Limits: retrievalboundary.Limits{MaxResults: 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range res.Evidence {
		if e.RecordID == "removed" {
			t.Fatal("a deleted source record was returned from the stale index")
		}
	}

	// Supersession and access loss invalidate affected rows.
	current["live"] = outcomefile.OutcomeRecord{RecordID: "live", Status: outcomefile.StatusPartial, Complete: true, Authorized: false}
	res, err = lr.Retrieve(ctx, retrievalboundary.ScopedQuery{
		Scope:  retrievalboundary.Scope{Corpus: retrievalboundary.CorpusOutcome},
		Text:   "failure signature",
		Limits: retrievalboundary.Limits{MaxResults: 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Evidence) != 0 {
		t.Fatalf("unauthorized evidence was returned: %+v", res.Evidence)
	}
}

func TestConcurrentReadersAndWriter(t *testing.T) {
	ctx := context.Background()
	ix := openTestIndex(t)
	if _, _, _, err := ix.Rebuild(ctx, []Record{testRecord("base", "failure signature alpha")}); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 32)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				if _, err := ix.Search(ctx, retrievalboundary.ScopedQuery{
					Scope: retrievalboundary.Scope{Corpus: retrievalboundary.CorpusOutcome},
					Text:  "failure signature",
				}); err != nil {
					errs <- err
					return
				}
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 5; j++ {
			if err := ix.Upsert(ctx, testRecord("writer-"+string(rune('a'+j)), "failure signature alpha")); err != nil {
				errs <- err
				return
			}
		}
	}()
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent access failed: %v", err)
	}
}

func TestIndexNeverCreatesExecutionStateTables(t *testing.T) {
	ctx := context.Background()
	ix := openTestIndex(t)
	if _, _, _, err := ix.Rebuild(ctx, []Record{testRecord("a", "body")}); err != nil {
		t.Fatal(err)
	}
	rows, err := ix.db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type='table'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		lower := strings.ToLower(name)
		for _, forbidden := range []string{"task", "plan", "approval", "run", "attempt", "state"} {
			if strings.Contains(lower, forbidden) && !strings.HasPrefix(lower, "records") {
				t.Errorf("memory index created a %q table; it must stay separate from execution state", name)
			}
		}
	}
}

func TestLocalRetrieverRequiresCurrent(t *testing.T) {
	ix := openTestIndex(t)
	lr := &LocalRetriever{Index: ix}
	_, err := lr.Retrieve(context.Background(), retrievalboundary.ScopedQuery{
		Scope: retrievalboundary.Scope{Corpus: retrievalboundary.CorpusOutcome},
		Text:  "failure",
	})
	if !errors.Is(err, retrievalboundary.ErrInvalidQuery) {
		t.Fatalf("err = %v, want a fail-closed InvalidQueryError when Current is nil", err)
	}
}

func TestDriftReasonForClassification(t *testing.T) {
	healthy := outcomefile.OutcomeRecord{
		RecordID: "a", Corpus: retrievalboundary.CorpusOutcome, Complete: true,
		Status: outcomefile.StatusPartial, Authorized: true, ContentHash: "h1",
	}
	cases := []struct {
		name    string
		src     outcomefile.OutcomeRecord
		has     bool
		hash    string
		want    DriftReason
		drifted bool
	}{
		{"healthy", healthy, true, "h1", "", false},
		{"deleted", healthy, false, "h1", DriftDeleted, true},
		{"modified", healthy, true, "h2", DriftModified, true},
		{"superseded", func() outcomefile.OutcomeRecord { r := healthy; r.Superseded = true; return r }(), true, "h1", DriftSuperseded, true},
		{"unauthorized", func() outcomefile.OutcomeRecord { r := healthy; r.Authorized = false; return r }(), true, "h1", DriftUnauthorized, true},
		{"incomplete", func() outcomefile.OutcomeRecord { r := healthy; r.Complete = false; return r }(), true, "h1", DriftIncomplete, true},
		{"success without evidence", func() outcomefile.OutcomeRecord { r := healthy; r.Status = outcomefile.StatusSuccess; return r }(), true, "h1", DriftIncomplete, true},
	}
	for _, tc := range cases {
		reason, drifted := DriftReasonFor(tc.src, tc.has, tc.hash)
		if drifted != tc.drifted || reason != tc.want {
			t.Errorf("%s: (%q,%v), want (%q,%v)", tc.name, reason, drifted, tc.want, tc.drifted)
		}
	}
}

func TestCorruptIndexIsTyped(t *testing.T) {
	path := filepath.Join(t.TempDir(), "broken.db")
	if err := os.WriteFile(path, []byte("this is not a sqlite database"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Open(path)
	if !errors.Is(err, retrievalboundary.ErrIndexCorrupt) {
		t.Fatalf("err = %v, want ErrIndexCorrupt for a corrupt database", err)
	}
}

func TestSearchRejectsUnrelatedEvidence(t *testing.T) {
	ctx := context.Background()
	ix := openTestIndex(t)
	if _, _, _, err := ix.Rebuild(ctx, []Record{
		testRecord("relevant", "provider timeout during retry of a transient failure"),
		testRecord("unrelated", "migration schema index rebuild"),
	}); err != nil {
		t.Fatal(err)
	}
	res, err := ix.Search(ctx, retrievalboundary.ScopedQuery{
		Scope:  retrievalboundary.Scope{Corpus: retrievalboundary.CorpusOutcome},
		Text:   "provider retry failure",
		Limits: retrievalboundary.Limits{MaxResults: 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Evidence) != 1 || res.Evidence[0].RecordID != "relevant" {
		t.Fatalf("evidence = %+v, want only the relevant record", res.Evidence)
	}
}

func TestSearchMatchesWhenOnlySomeTermsMatch(t *testing.T) {
	ctx := context.Background()
	ix := openTestIndex(t)
	if _, _, _, err := ix.Rebuild(ctx, []Record{
		testRecord("a", "the build failed with a missing module sum entry"),
	}); err != nil {
		t.Fatal(err)
	}
	// A natural-language task description: only some terms occur in the record.
	res, err := ix.Search(ctx, retrievalboundary.ScopedQuery{
		Scope:  retrievalboundary.Scope{Corpus: retrievalboundary.CorpusOutcome},
		Text:   "a build failed with a missing module sum",
		Limits: retrievalboundary.Limits{MaxResults: 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Evidence) != 1 {
		t.Fatalf("evidence = %+v, want a keyword match under OR semantics", res.Evidence)
	}
}

func TestBuildFTSQueryQuotesUserText(t *testing.T) {
	got, err := BuildFTSQuery(`SELECT * FROM x WHERE a="b" AND c NEAR d`)
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"*", ":", "^", "-", `"`} {
		if strings.Contains(got, raw) && raw != `"` {
			t.Errorf("FTS query %q contains unquoted operator %q", got, raw)
		}
	}
	if !strings.Contains(got, `"select"`) {
		t.Errorf("FTS query = %q, want quoted lowercased tokens", got)
	}
	if !strings.Contains(got, " OR ") {
		t.Errorf("FTS query = %q, want OR-joined terms so a natural-language query can match", got)
	}
}
