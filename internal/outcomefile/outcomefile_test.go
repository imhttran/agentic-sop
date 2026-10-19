package outcomefile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/retrievalboundary"
)

// writeRun creates a minimal persisted run artifact set for one task.
func writeRun(t *testing.T, runsDir, taskID, outcomeStatus, decision, reason string) {
	t.Helper()
	dir := filepath.Join(runsDir, taskID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	report := `{
  "id": "` + taskID + `",
  "stage": "PASSED",
  "decision": "` + decision + `",
  "reasons": ["` + reason + `"],
  "validation": [{"Category":"BUILD","Command":"go build ./...","ExitCode":0,"Status":"PASS"}],
  "model_selection": {"class":"large","provider":"ollama","model":"m","locality":"cloud","source":"env","fallback":false},
  "outcome": {
    "status": "` + outcomeStatus + `",
    "criteria": [{"ID":"criterion-1","Required":true,"Met":false,"Description":"a criterion","Evidence":"unverifiable: no deterministic check supplied"}],
    "reasons": ["` + reason + `"],
    "lifecycle": "LOCAL_DONE",
    "compatibility": "legacy plan: no acceptance mapping is declared"
  },
  "generated_at": "2026-10-19T17:10:35.909017Z"
}`
	if err := os.WriteFile(filepath.Join(dir, "report.json"), []byte(report), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{"id":"`+taskID+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	counts := `{"counts":{"attempts":2,"invocations":2},"binary":{"revision":"abc123","modified":"true"}}`
	if err := os.WriteFile(filepath.Join(dir, "counts.json"), []byte(counts), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestIngestionIsIdempotent(t *testing.T) {
	runsDir := t.TempDir()
	outDir := t.TempDir()
	writeRun(t, runsDir, "RDM-002", "PARTIAL", "PASS", "all required checks passed")

	first, err := IngestRun(runsDir, "RDM-002")
	if err != nil {
		t.Fatalf("IngestRun: %v", err)
	}
	second, err := IngestRun(runsDir, "RDM-002")
	if err != nil {
		t.Fatalf("IngestRun again: %v", err)
	}
	if first.RecordID != second.RecordID || first.SourceHash != second.SourceHash {
		t.Fatalf("repeated ingestion is not stable: %+v vs %+v", first, second)
	}

	p1, h1, err := Write(outDir, first)
	if err != nil {
		t.Fatal(err)
	}
	p2, h2, err := Write(outDir, second)
	if err != nil {
		t.Fatal(err)
	}
	if p1 != p2 || h1 != h2 {
		t.Fatalf("repeated write changed the document: %s/%s vs %s/%s", p1, h1, p2, h2)
	}
	records, rejected, err := ReadAll(outDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(rejected) != 0 || len(records) != 1 {
		t.Fatalf("got %d records and %d rejected, want 1 and 0", len(records), len(rejected))
	}
}

func TestStatusesRemainDistinct(t *testing.T) {
	runsDir := t.TempDir()
	for _, tc := range []struct {
		outcome string
		want    Status
	}{
		{"VERIFIED", StatusSuccess},
		{"PARTIAL", StatusPartial},
		{"FAILED", StatusFailed},
		{"BLOCKED", StatusBlocked},
	} {
		writeRun(t, runsDir, "T-"+tc.outcome, tc.outcome, "PASS", "reason")
		rec, err := IngestRun(runsDir, "T-"+tc.outcome)
		if err != nil {
			t.Fatalf("%s: %v", tc.outcome, err)
		}
		if rec.Status != tc.want {
			t.Errorf("%s: status = %q, want %q", tc.outcome, rec.Status, tc.want)
		}
	}
}

func TestGatePassCannotConvertPartialToSuccess(t *testing.T) {
	runsDir := t.TempDir()
	// The run's gate decision is PASS, but the objective outcome is PARTIAL.
	writeRun(t, runsDir, "RDM-002", "PARTIAL", "PASS", "all required checks passed")
	rec, err := IngestRun(runsDir, "RDM-002")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Status != StatusPartial {
		t.Fatalf("status = %q, want partial: a gate pass must not become a verified success", rec.Status)
	}
	if rec.Action != "PASS" {
		t.Fatalf("action = %q, want the observed decision preserved", rec.Action)
	}
}

func TestMissingEvidenceIsExplicit(t *testing.T) {
	cases := []struct {
		name string
		rec  OutcomeRecord
		want string
	}{
		{"incomplete", OutcomeRecord{RecordID: "x", Corpus: retrievalboundary.CorpusOutcome, Status: StatusSuccess, ContentHash: "h", Authorized: true}, "incomplete"},
		{"no status", OutcomeRecord{RecordID: "x", Corpus: retrievalboundary.CorpusOutcome, Complete: true, ContentHash: "h", Authorized: true}, "missing status"},
		{"success without evidence", OutcomeRecord{RecordID: "x", Corpus: retrievalboundary.CorpusOutcome, Complete: true, Status: StatusSuccess, ContentHash: "h", Authorized: true}, "missing evidence for success"},
	}
	for _, tc := range cases {
		ok, reason := tc.rec.Eligible()
		if ok || reason != tc.want {
			t.Errorf("%s: Eligible = %v, %q; want false, %q", tc.name, ok, reason, tc.want)
		}
	}
	// A partial outcome needs no positive evidence to be eligible; it simply never
	// claims success.
	partial := OutcomeRecord{RecordID: "y", Corpus: retrievalboundary.CorpusOutcome, Complete: true, Status: StatusPartial, ContentHash: "h", Authorized: true}
	if ok, reason := partial.Eligible(); !ok {
		t.Errorf("partial record should be eligible, got %q", reason)
	}
}

func TestSourceDeletionSupersessionAndAccessInvalidate(t *testing.T) {
	runsDir := t.TempDir()
	outDir := t.TempDir()
	writeRun(t, runsDir, "RDM-002", "PARTIAL", "PASS", "reason")
	rec, err := IngestRun(runsDir, "RDM-002")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := Write(outDir, rec); err != nil {
		t.Fatal(err)
	}
	records, _, err := ReadAll(outDir)
	if err != nil || len(records) != 1 {
		t.Fatalf("ReadAll = %d records, %v", len(records), err)
	}

	if ex := GateWithSources(records, func(string) bool { return true }); len(ex) != 0 {
		t.Fatalf("live source produced exclusions: %+v", ex)
	}
	ex := GateWithSources(records, func(string) bool { return false })
	if len(ex) != 1 || ex[0].Reason != "source deleted" {
		t.Fatalf("deleted source exclusions = %+v, want one 'source deleted'", ex)
	}

	superseded := records[0]
	superseded.ContentHash = records[0].ContentHash
	superseded.Superseded = true
	if ok, reason := superseded.Eligible(); ok || reason != "superseded" {
		t.Errorf("superseded = %v, %q", ok, reason)
	}

	unauthorized := records[0]
	unauthorized.ContentHash = records[0].ContentHash
	unauthorized.Authorized = false
	if ok, reason := unauthorized.Eligible(); ok || reason != "unauthorized" {
		t.Errorf("unauthorized = %v, %q", ok, reason)
	}

	// Supersession recorded in the document on disk is excluded by Gate.
	rec.Superseded = true
	rec.SupersededBy = "RDM-002-attempt-3"
	if _, _, err := Write(outDir, rec); err != nil {
		t.Fatal(err)
	}
	records, _, err = ReadAll(outDir)
	if err != nil {
		t.Fatal(err)
	}
	if ex := Gate(records); len(ex) != 1 || ex[0].Reason != "superseded" {
		t.Fatalf("Gate after supersession = %+v", ex)
	}
}

func TestRedactionAndProvenance(t *testing.T) {
	in := "failed with bearer abcdefghijklmnop and api_key=supersecretvalue and ghp_abcdefghijklmnopqrstuvwx"
	out := Redact(in)
	for _, leaked := range []string{"abcdefghijklmnop", "supersecretvalue", "ghp_abcdefghijklmnopqrstuvwx"} {
		if strings.Contains(out, leaked) {
			t.Errorf("redaction left %q in %q", leaked, out)
		}
	}
	if !strings.Contains(out, "<redacted>") && !strings.Contains(out, "<redacted-token>") {
		t.Errorf("redaction produced no marker: %q", out)
	}
	if again := Redact(out); again != out {
		t.Errorf("redaction is not idempotent: %q vs %q", out, again)
	}

	runsDir := t.TempDir()
	writeRun(t, runsDir, "RDM-002", "FAILED", "FAIL", "leaked api_key=supersecretvalue")
	rec, err := IngestRun(runsDir, "RDM-002")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rec.Rationale, "supersecretvalue") || strings.Contains(rec.Text, "supersecretvalue") {
		t.Fatalf("ingested evidence was not redacted: %+v", rec)
	}
	if rec.SourceHash == "" || len(rec.SourceArtifacts) == 0 || rec.RecordedAt == "" {
		t.Fatalf("provenance is incomplete: %+v", rec)
	}
	if rec.RationaleSource != RationaleObserved {
		t.Errorf("rationale source = %q, want observed", rec.RationaleSource)
	}
}

func TestIngestRunForCarriesRepository(t *testing.T) {
	runsDir := t.TempDir()
	writeRun(t, runsDir, "RDM-002", "PARTIAL", "PASS", "reason")
	rec, err := IngestRunFor(runsDir, "RDM-002", "agentic-sop")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Repository != "agentic-sop" {
		t.Fatalf("repository = %q, want the scoped identity", rec.Repository)
	}
	if ok, reason := rec.Eligible(); !ok {
		t.Fatalf("record not eligible: %q", reason)
	}
}

func TestIngestedRecordIsImmediatelyEligible(t *testing.T) {
	runsDir := t.TempDir()
	writeRun(t, runsDir, "RDM-002", "PARTIAL", "PASS", "reason")
	rec, err := IngestRun(runsDir, "RDM-002")
	if err != nil {
		t.Fatal(err)
	}
	if rec.ContentHash == "" {
		t.Fatal("ingested record has no content hash; eligibility would be meaningless before Write")
	}
	if ok, reason := rec.Eligible(); !ok {
		t.Fatalf("ingested record is not eligible: %q", reason)
	}
}

func TestDuplicateRecordIDIsRejected(t *testing.T) {
	dir := t.TempDir()
	rec := OutcomeRecord{
		RecordID: "dup", Corpus: retrievalboundary.CorpusOutcome, Complete: true,
		Status: StatusPartial, Authorized: true, Text: "body",
	}
	path, _, err := Write(dir, rec)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "copy.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	records, rejected, err := ReadAll(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || len(rejected) != 1 {
		t.Fatalf("records=%d rejected=%v, want one record and one rejected duplicate", len(records), rejected)
	}
	for _, reason := range rejected {
		if !strings.Contains(reason, "duplicate record id") {
			t.Fatalf("rejection reason = %q, want an explicit duplicate-id reason", reason)
		}
	}
}

func TestInterruptedTemporaryFileIsNeverSelected(t *testing.T) {
	dir := t.TempDir()
	rec := OutcomeRecord{RecordID: "a", Corpus: retrievalboundary.CorpusOutcome, Status: StatusFailed, Complete: true, Authorized: true}
	if _, _, err := Write(dir, rec); err != nil {
		t.Fatal(err)
	}
	tmp := filepath.Join(dir, tempPrefix+"interrupted.tmp")
	if err := os.WriteFile(tmp, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	records, rejected, err := ReadAll(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || len(rejected) != 0 {
		t.Fatalf("records=%d rejected=%v; the temporary file must be ignored", len(records), rejected)
	}
}
