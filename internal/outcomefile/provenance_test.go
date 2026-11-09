package outcomefile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/retrievalboundary"
)

type provenanceCriterion struct {
	ID       string `json:"ID"`
	Required bool   `json:"Required"`
	Boundary bool   `json:"Boundary,omitempty"`
	Met      bool   `json:"Met"`
}

type provenanceValidation struct {
	Command  string `json:"Command"`
	ExitCode int    `json:"ExitCode"`
	Status   string `json:"Status"`
}

// provenanceReport is the controllable run report used by the TestIngestProvenance
// fixtures. It makes the declared id, the required criteria, and the validation results
// explicit, so a fixture can tamper with exactly one of them.
type provenanceReport struct {
	ID         string
	Outcome    string
	Decision   string
	Reason     string
	Criteria   []provenanceCriterion
	Validation []provenanceValidation
	// GatePassed models the nested quality-gate verdict. Nil means the report predates
	// the field, so it is not treated as a contradiction.
	GatePassed *bool
}

// boolPtr returns a pointer to v for an optional fixture field.
func boolPtr(v bool) *bool { return &v }

// passingReport returns a report that corroborates a claimed success.
func passingReport(id, reason string) provenanceReport {
	return provenanceReport{
		ID:         id,
		Outcome:    "VERIFIED",
		Decision:   "PASS",
		Reason:     reason,
		Criteria:   []provenanceCriterion{{ID: "criterion-1", Required: true, Met: true}},
		Validation: []provenanceValidation{{Command: "go test ./...", ExitCode: 0, Status: "PASS"}},
	}
}

// writeProvenanceRun writes a run tree from rep under runsDir/<dirTaskID>. The declared id
// inside the report can differ from dirTaskID, which models substituted content.
func writeProvenanceRun(t *testing.T, runsDir, dirTaskID string, rep provenanceReport) string {
	t.Helper()
	dir := filepath.Join(runsDir, dirTaskID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	outcome := map[string]any{
		"status":        rep.Outcome,
		"criteria":      rep.Criteria,
		"reasons":       []string{rep.Reason},
		"lifecycle":     "LOCAL_DONE",
		"compatibility": "declared acceptance mapping",
	}
	if rep.GatePassed != nil {
		outcome["validation"] = map[string]any{"passed": *rep.GatePassed}
	}
	body := map[string]any{
		"id":         rep.ID,
		"stage":      "PASSED",
		"decision":   rep.Decision,
		"reasons":    []string{rep.Reason},
		"validation": rep.Validation,
		"model_selection": map[string]any{
			"class": "large", "provider": "ollama", "model": "m",
			"locality": "cloud", "source": "env", "fallback": false,
		},
		"outcome":      outcome,
		"generated_at": "2026-10-19T17:10:35.909017Z",
	}
	data, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "report.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{"id":"`+dirTaskID+`"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	counts := `{"counts":{"attempts":1,"invocations":1},"binary":{"revision":"abc123","modified":"true"}}`
	if err := os.WriteFile(filepath.Join(dir, "counts.json"), []byte(counts), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// TestIngestProvenanceTamperedSuccessIsNotVerified proves a forged success claim does not
// become a verified success merely by ingestion.
func TestIngestProvenanceTamperedSuccessIsNotVerified(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*provenanceReport)
	}{
		{"required criterion unmet", func(r *provenanceReport) { r.Criteria[0].Met = false }},
		{"failing validation exit", func(r *provenanceReport) { r.Validation[0].ExitCode = 1 }},
		{"failing validation status", func(r *provenanceReport) { r.Validation[0].Status = "FAIL" }},
		{"no validation recorded", func(r *provenanceReport) { r.Validation = nil }},
		{"no required criterion", func(r *provenanceReport) { r.Criteria = nil }},
		{"optional criteria only", func(r *provenanceReport) {
			r.Criteria = []provenanceCriterion{{ID: "criterion-1", Required: false, Met: true}}
		}},
		{"unmet boundary criterion", func(r *provenanceReport) {
			r.Criteria = []provenanceCriterion{
				{ID: "criterion-1", Required: true, Met: true},
				{ID: "boundary-1", Required: false, Met: false, Boundary: true},
			}
		}},
		{"failed quality gate", func(r *provenanceReport) { r.GatePassed = boolPtr(false) }},
		{"failed quality decision", func(r *provenanceReport) { r.Decision = "FAIL" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runsDir := t.TempDir()
			outDir := t.TempDir()
			rep := passingReport("TAMPER-1", "forged success")
			tc.mutate(&rep)
			writeProvenanceRun(t, runsDir, "TAMPER-1", rep)

			rec, err := IngestRunFor(runsDir, "TAMPER-1", "agentic-sop")
			if err != nil {
				t.Fatalf("IngestRunFor: %v", err)
			}
			if rec.Status == StatusSuccess {
				t.Fatalf("status = %q: a forged success claim became a verified success", rec.Status)
			}
			if rec.Status != StatusPartial {
				t.Fatalf("status = %q, want partial so the history stays advisory", rec.Status)
			}
			if _, _, err := Write(outDir, rec); err != nil {
				t.Fatal(err)
			}
			records, _, err := ReadAll(outDir)
			if err != nil || len(records) != 1 {
				t.Fatalf("ReadAll = %d records, %v", len(records), err)
			}
			if ok, reason := records[0].Eligible(); !ok {
				t.Fatalf("tampered record is not usable as advisory data: %q", reason)
			}
			if records[0].Status == StatusSuccess {
				t.Fatal("the persisted record claims success")
			}
		})
	}
}

// TestIngestProvenanceValidSuccessControl proves a corroborated success is still ingested
// as a verified success and passes the gate.
func TestIngestProvenanceValidSuccessControl(t *testing.T) {
	runsDir := t.TempDir()
	outDir := t.TempDir()
	writeProvenanceRun(t, runsDir, "GOOD-1", passingReport("GOOD-1", "every required check passed"))

	rec, err := IngestRunFor(runsDir, "GOOD-1", "agentic-sop")
	if err != nil {
		t.Fatalf("IngestRunFor: %v", err)
	}
	if rec.Status != StatusSuccess {
		t.Fatalf("status = %q, want success for a corroborated claim", rec.Status)
	}
	if _, _, err := Write(outDir, rec); err != nil {
		t.Fatal(err)
	}
	records, _, err := ReadAll(outDir)
	if err != nil || len(records) != 1 {
		t.Fatalf("ReadAll = %d records, %v", len(records), err)
	}
	if ex := GateWithSourcesAndDigests(records, FileSourceExists, FileSourceDigest); len(ex) != 0 {
		t.Fatalf("valid success control was excluded: %+v", ex)
	}
}

// TestIngestProvenanceSubstitutedContentRejected proves a report whose declared id differs
// from its run directory is rejected, and the previous document is unchanged.
func TestIngestProvenanceSubstitutedContentRejected(t *testing.T) {
	runsDir := t.TempDir()
	outDir := t.TempDir()
	writeProvenanceRun(t, runsDir, "OWNER-1", passingReport("OWNER-1", "the owning task"))
	before, err := IngestRunFor(runsDir, "OWNER-1", "agentic-sop")
	if err != nil {
		t.Fatal(err)
	}
	beforePath, beforeHash, err := Write(outDir, before)
	if err != nil {
		t.Fatal(err)
	}

	stolen := passingReport("OTHER-2", "stolen evidence")
	writeProvenanceRun(t, runsDir, "OWNER-1-copy", stolen)
	if _, err := IngestRunFor(runsDir, "OWNER-1-copy", "agentic-sop"); err == nil {
		t.Fatal("IngestRunFor accepted a report whose declared id does not match its run directory")
	} else if !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("err = %v, want an explicit id-mismatch error", err)
	}

	records, rejected, err := ReadAll(outDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(rejected) != 0 || len(records) != 1 {
		t.Fatalf("records = %d rejected = %v, want the previous document intact", len(records), rejected)
	}
	if records[0].ContentHash != beforeHash || records[0].RecordID != before.RecordID {
		t.Fatal("the previous document changed after a rejected ingest")
	}
	if PathFor(outDir, records[0].RecordID) != beforePath {
		t.Fatal("the previous document path changed")
	}
}

// TestIngestProvenanceMalformedCountsRejected proves malformed counts metadata is rejected
// and leaves the previous state intact.
func TestIngestProvenanceMalformedCountsRejected(t *testing.T) {
	runsDir := t.TempDir()
	outDir := t.TempDir()
	dir := writeProvenanceRun(t, runsDir, "COUNTS-1", passingReport("COUNTS-1", "valid run"))
	rec, err := IngestRunFor(runsDir, "COUNTS-1", "agentic-sop")
	if err != nil {
		t.Fatal(err)
	}
	_, beforeHash, err := Write(outDir, rec)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "counts.json"), []byte(`{"counts":`), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := IngestRunFor(runsDir, "COUNTS-1", "agentic-sop")
	if err == nil {
		t.Fatalf("IngestRunFor accepted malformed counts.json and returned %+v", got)
	}
	if !strings.Contains(err.Error(), "counts.json") {
		t.Fatalf("err = %v, want an error naming counts.json", err)
	}

	records, rejected, err := ReadAll(outDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(rejected) != 0 || len(records) != 1 || records[0].ContentHash != beforeHash {
		t.Fatalf("the previous document did not stay intact: %d records, %v", len(records), rejected)
	}
}

// TestIngestProvenanceModifiedSourceInvalidated proves a changed source artifact loses its
// verified identity instead of keeping the recorded digest.
func TestIngestProvenanceModifiedSourceInvalidated(t *testing.T) {
	runsDir := t.TempDir()
	outDir := t.TempDir()
	dir := writeProvenanceRun(t, runsDir, "MOD-1", passingReport("MOD-1", "valid run"))
	rec, err := IngestRunFor(runsDir, "MOD-1", "agentic-sop")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Status != StatusSuccess {
		t.Fatalf("setup status = %q, want a verified success before the change", rec.Status)
	}
	if _, _, err := Write(outDir, rec); err != nil {
		t.Fatal(err)
	}
	records, _, err := ReadAll(outDir)
	if err != nil || len(records) != 1 {
		t.Fatalf("ReadAll = %d records, %v", len(records), err)
	}
	if ex := GateWithSourcesAndDigests(records, FileSourceExists, FileSourceDigest); len(ex) != 0 {
		t.Fatalf("an unmodified source was excluded: %+v", ex)
	}

	reportPath := filepath.Join(dir, "report.json")
	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(reportPath, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}

	ex := GateWithSourcesAndDigests(records, FileSourceExists, FileSourceDigest)
	if len(ex) != 1 || ex[0].Reason != ReasonSourceModified {
		t.Fatalf("exclusions = %+v, want one %q", ex, ReasonSourceModified)
	}
	if ex[0].Reason == ReasonSourceDeleted {
		t.Fatal("a modified source was reported as deleted")
	}
}

// TestIngestProvenanceRejectedInputPreservesPreviousState proves a rejected ingest never
// overwrites an existing valid record.
func TestIngestProvenanceRejectedInputPreservesPreviousState(t *testing.T) {
	runsDir := t.TempDir()
	outDir := t.TempDir()
	writeProvenanceRun(t, runsDir, "KEEP-1", passingReport("KEEP-1", "valid run"))
	good, err := IngestRunFor(runsDir, "KEEP-1", "agentic-sop")
	if err != nil {
		t.Fatal(err)
	}
	path, hash, err := Write(outDir, good)
	if err != nil {
		t.Fatal(err)
	}
	rawBefore, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(runsDir, "KEEP-1", "report.json"), []byte(`{"id":"OTHER"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := IngestRunFor(runsDir, "KEEP-1", "agentic-sop"); err == nil {
		t.Fatal("IngestRunFor accepted a report with the wrong declared identity")
	}

	rawAfter, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the previous document is no longer readable: %v", err)
	}
	if string(rawBefore) != string(rawAfter) {
		t.Fatal("a rejected ingest changed the previously written document")
	}
	rec, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if rec.ContentHash != hash || rec.RecordID != good.RecordID {
		t.Fatalf("the previous identity changed: %+v", rec)
	}
}

// TestIngestProvenanceGateRejectsContradictedSuccess proves a success whose recorded
// verification contradicts it is excluded with an explicit reason.
func TestIngestProvenanceGateRejectsContradictedSuccess(t *testing.T) {
	rec := OutcomeRecord{
		RecordID: "contradicted", Corpus: retrievalboundary.CorpusOutcome, Complete: true,
		Status: StatusSuccess, Authorized: true, ContentHash: "h",
		EvidenceRefs: []string{"run/report.json"},
		Verification: []Verification{{Command: "go test ./...", ExitCode: 1, Result: "FAIL"}},
	}
	ok, reason := rec.Eligible()
	if ok {
		t.Fatal("a success contradicted by its recorded verification is eligible")
	}
	if reason != "verification contradicts success" {
		t.Fatalf("reason = %q, want an explicit contradiction reason", reason)
	}
	ex := Gate([]OutcomeRecord{rec})
	if len(ex) != 1 || ex[0].Reason != reason {
		t.Fatalf("Gate = %+v, want one explicit exclusion", ex)
	}

	rec.Status = StatusPartial
	if ok, reason := rec.Eligible(); !ok {
		t.Fatalf("partial history was excluded: %q", reason)
	}
}

// TestIngestProvenanceMutableLifecycleStateDoesNotInvalidate proves an ordinary
// lifecycle rewrite of state.json (parking a run at the human boundary after capture)
// does not invalidate a record whose evidence is unchanged. A changed report still does.
func TestIngestProvenanceMutableLifecycleStateDoesNotInvalidate(t *testing.T) {
	runsDir := t.TempDir()
	outDir := t.TempDir()
	dir := writeProvenanceRun(t, runsDir, "PARK-1", passingReport("PARK-1", "valid run"))
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{"id":"PARK-1","stage":"LOCAL_DONE"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	rec, err := IngestRunFor(runsDir, "PARK-1", "agentic-sop")
	if err != nil {
		t.Fatal(err)
	}
	for artifact := range rec.SourceDigests {
		if filepath.Base(filepath.FromSlash(artifact)) == "state.json" {
			t.Fatalf("mutable lifecycle state was digested: %s", artifact)
		}
	}
	if _, _, err := Write(outDir, rec); err != nil {
		t.Fatal(err)
	}
	records, _, err := ReadAll(outDir)
	if err != nil || len(records) != 1 {
		t.Fatalf("ReadAll = %d records, %v", len(records), err)
	}
	if ex := GateWithSourcesAndDigests(records, FileSourceExists, FileSourceDigest); len(ex) != 0 {
		t.Fatalf("an unmodified record was excluded: %+v", ex)
	}

	// The lifecycle parks the run after capture, rewriting state.json.
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(`{"id":"PARK-1","stage":"WAITING_FOR_HUMAN"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if ex := GateWithSourcesAndDigests(records, FileSourceExists, FileSourceDigest); len(ex) != 0 {
		t.Fatalf("a lifecycle state rewrite invalidated the record: %+v", ex)
	}

	// A changed evidence artifact still invalidates the same record.
	reportPath := filepath.Join(dir, "report.json")
	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(reportPath, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	ex := GateWithSourcesAndDigests(records, FileSourceExists, FileSourceDigest)
	if len(ex) != 1 || ex[0].Reason != ReasonSourceModified {
		t.Fatalf("exclusions = %+v, want one %q", ex, ReasonSourceModified)
	}
}

// TestIngestProvenanceRelativeSourcePathsAreAbsolute proves a relative runs directory is
// persisted as absolute source paths, so a later rebuild from another working directory
// does not wrongly exclude an unchanged source as deleted.
func TestIngestProvenanceRelativeSourcePathsAreAbsolute(t *testing.T) {
	absRuns := t.TempDir()
	writeProvenanceRun(t, absRuns, "REL-1", passingReport("REL-1", "valid run"))
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relRuns, err := filepath.Rel(cwd, absRuns)
	if err != nil {
		t.Fatal(err)
	}
	rec, err := IngestRunFor(relRuns, "REL-1", "agentic-sop")
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.SourceArtifacts) == 0 {
		t.Fatal("no source artifacts were recorded")
	}
	for _, artifact := range rec.SourceArtifacts {
		if !filepath.IsAbs(filepath.FromSlash(artifact)) {
			t.Fatalf("source artifact %q is not absolute", artifact)
		}
	}
	if ex := GateWithSourcesAndDigests([]OutcomeRecord{rec}, FileSourceExists, FileSourceDigest); len(ex) != 0 {
		t.Fatalf("an unchanged relative-ingest record was excluded: %+v", ex)
	}
}
