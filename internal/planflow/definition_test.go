package planflow

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/planner"
	"github.com/imhttran/agentic-sop/internal/projectlock"
)

// definitionDoc is a one-stage plan whose single stage carries every field the
// definition digest covers, so a test can vary one field at a time.
const definitionDoc = "# Implementation Plan\n\n## Project\n\nWidget\n\n## Summary\n\nAdd a widget.\n\n## S001 — Application skeleton\n\nCreate it.\n\n### Deliverables\n\n- cmd/main.go\n\n### Acceptance Criteria\n\n- starts\n\n### Acceptance Checks\n\n- go build ./...\n"

func writeDefinitionPlan(t *testing.T, dir string) {
	t.Helper()
	if err := writePlan(filepath.Join(dir, config.DirName, planFileName), &planner.Plan{
		Project: "Widget",
		Summary: "Add a widget.",
		Stages:  []planner.Stage{{ID: "S001", Title: "t", Objective: "o", AcceptanceCriteria: []string{"starts"}, Deliverables: []string{"cmd/main.go"}, AcceptanceChecks: []string{"go build ./..."}}},
	}); err != nil {
		t.Fatalf("write plan: %v", err)
	}
}

// TestTaskDefinitionIdentityStableAcrossReads proves the digest of an unchanged
// persisted plan is identical on repeated reads.
func TestTaskDefinitionIdentityStableAcrossReads(t *testing.T) {
	dir := t.TempDir()
	writeDefinitionPlan(t, dir)

	first, err := TaskDefinitionIdentity(dir, "S001")
	if err != nil {
		t.Fatalf("first read: %v", err)
	}
	if !first.Present || first.Digest == "" {
		t.Fatalf("identity = %+v, want a present digest", first)
	}
	for i := 0; i < 3; i++ {
		again, err := TaskDefinitionIdentity(dir, "S001")
		if err != nil {
			t.Fatalf("read %d: %v", i, err)
		}
		if again.Digest != first.Digest || again.Present != first.Present {
			t.Fatalf("digest changed across reads: %q vs %q", first.Digest, again.Digest)
		}
	}
}

// TestTaskDefinitionIdentityChangesWithEveryCoveredField proves a change to any
// covered stage field changes the digest.
func TestTaskDefinitionIdentityChangesWithEveryCoveredField(t *testing.T) {
	base := planner.Stage{
		ID:                 "S001",
		Title:              "t",
		Objective:          "o",
		Deliverables:       []string{"a", "b"},
		AcceptanceCriteria: []string{"c1", "c2"},
		AcceptanceChecks:   []string{"go test ./..."},
		AcceptanceMapping:  []planner.AcceptanceMapping{{Criterion: 1, Check: 1}},
		Dependencies:       []string{"S000"},
		Requires:           []string{"Cap"},
	}
	baseDigest := stageDigest(base)

	mutations := map[string]func(s *planner.Stage){
		"deliverables":        func(s *planner.Stage) { s.Deliverables = append(s.Deliverables, "c") },
		"acceptance_criteria": func(s *planner.Stage) { s.AcceptanceCriteria = append(s.AcceptanceCriteria, "c3") },
		"acceptance_checks":   func(s *planner.Stage) { s.AcceptanceChecks = append(s.AcceptanceChecks, "go vet ./...") },
		"acceptance_mapping":  func(s *planner.Stage) { s.AcceptanceMapping = nil },
		"dependencies":        func(s *planner.Stage) { s.Dependencies = nil },
		"requires":            func(s *planner.Stage) { s.Requires = nil },
		"required_evidence":   func(s *planner.Stage) { s.RequiredEvidence = []string{"corpus:x"} },
		"title":               func(s *planner.Stage) { s.Title = "renamed" },
		"objective":           func(s *planner.Stage) { s.Objective = "changed" },
		"kind":                func(s *planner.Stage) { s.Kind = planner.KindEnvironment },
		"execution_mode":      func(s *planner.Stage) { s.ExecutionMode = domain.ExecutionVerifyFirst },
	}
	for name, mutate := range mutations {
		s := base
		s.Deliverables = append([]string(nil), base.Deliverables...)
		s.AcceptanceCriteria = append([]string(nil), base.AcceptanceCriteria...)
		s.AcceptanceChecks = append([]string(nil), base.AcceptanceChecks...)
		s.Dependencies = append([]string(nil), base.Dependencies...)
		s.Requires = append([]string(nil), base.Requires...)
		s.AcceptanceMapping = append([]planner.AcceptanceMapping(nil), base.AcceptanceMapping...)
		mutate(&s)
		if got := stageDigest(s); got == baseDigest {
			t.Errorf("digest unchanged after mutating %s", name)
		}
	}
}

// TestStageDigestOrderIndependent proves list ordering does not affect the digest.
func TestStageDigestOrderIndependent(t *testing.T) {
	a := planner.Stage{ID: "S", AcceptanceCriteria: []string{"x", "y"}, Dependencies: []string{"A", "B"}}
	b := planner.Stage{ID: "S", AcceptanceCriteria: []string{"y", "x"}, Dependencies: []string{"B", "A"}}
	if stageDigest(a) != stageDigest(b) {
		t.Error("digest must be independent of list order")
	}
}

// TestTaskDefinitionIdentityUndefinedAndAbsent proves an undefined task and a
// missing plan both report an empty digest with Present false and no error.
func TestTaskDefinitionIdentityUndefinedAndAbsent(t *testing.T) {
	dir := t.TempDir()
	missing, err := TaskDefinitionIdentity(dir, "S001")
	if err != nil {
		t.Fatalf("missing plan: %v", err)
	}
	if missing.Present || missing.Digest != "" {
		t.Errorf("missing plan = %+v, want absent with empty digest", missing)
	}
	if _, err := os.Stat(filepath.Join(dir, config.DirName)); !os.IsNotExist(err) {
		t.Error("the read must not create .agent-sdlc")
	}

	writeDefinitionPlan(t, dir)
	undefined, err := TaskDefinitionIdentity(dir, "S999")
	if err != nil {
		t.Fatalf("undefined task: %v", err)
	}
	if undefined.Present || undefined.Digest != "" {
		t.Errorf("undefined task = %+v, want absent with empty digest", undefined)
	}
}

// TestTaskDefinitionIdentityPreparedProject proves a prepared project yields a
// digest for each defined stage and absent for an undefined one, and that the
// digest follows a reconciled change.
func TestTaskDefinitionIdentityPreparedProject(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", reconcileDocTwo)

	for _, id := range []string{"S001", "S002"} {
		got, err := TaskDefinitionIdentity(dir, id)
		if err != nil {
			t.Fatalf("%s: %v", id, err)
		}
		if !got.Present || got.Digest == "" {
			t.Errorf("%s identity = %+v, want present", id, got)
		}
	}
	if got, _ := TaskDefinitionIdentity(dir, "S999"); got.Present {
		t.Errorf("S999 identity = %+v, want absent", got)
	}

	before, err := TaskDefinitionIdentity(dir, "S002")
	if err != nil {
		t.Fatalf("before: %v", err)
	}

	// Reconcile a changed definition for S002.
	changed := strings.Replace(reconcileDocTwo, "Ingest it.", "Ingest it thoroughly.", 1)
	write(t, dir, "PLAN.md", changed)
	if _, err := reconcile(t, dir, "PLAN.md", st); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	after, err := TaskDefinitionIdentity(dir, "S002")
	if err != nil {
		t.Fatalf("after: %v", err)
	}
	if after.Digest == before.Digest {
		t.Error("digest must change when the persisted stage changes")
	}
	assertOnlyKnownStateFiles(t, dir)
}

// TestTaskReconciliationProvenanceAbsentAndRecorded proves the provenance read
// reports absent evidence without error, and reports a recorded approval with the
// reconciliation time after --accept-changed.
func TestTaskReconciliationProvenanceAbsentAndRecorded(t *testing.T) {
	dir := t.TempDir()
	absent, err := TaskReconciliationProvenance(dir, "S001")
	if err != nil {
		t.Fatalf("absent: %v", err)
	}
	if absent.Present || absent.Reconciled || !absent.RecordedAt.IsZero() {
		t.Errorf("absent = %+v, want absent", absent)
	}
	if _, err := os.Stat(filepath.Join(dir, config.DirName)); !os.IsNotExist(err) {
		t.Error("the read must not create .agent-sdlc")
	}

	st := prepareForReconcile(t, dir, "PLAN.md", planDoc)
	markExecuted(taskByID(t, st.tasks, "S001"))
	write(t, dir, "PLAN.md", strings.ReplaceAll(planDoc, "Application skeleton", "Renamed"))

	unrecorded, err := TaskReconciliationProvenance(dir, "S001")
	if err != nil {
		t.Fatalf("unrecorded: %v", err)
	}
	if unrecorded.Present || unrecorded.Reconciled {
		t.Errorf("unrecorded = %+v, want absent", unrecorded)
	}

	planPath := filepath.Join(dir, config.DirName, planFileName)
	metaPath := filepath.Join(dir, config.DirName, metaFileName)
	beforePlan := read(t, planPath)
	if _, err := reconcileAccept(t, dir, "PLAN.md", st, "S001"); err != nil {
		t.Fatalf("reconcile --accept-changed: %v", err)
	}

	recorded, err := TaskReconciliationProvenance(dir, "S001")
	if err != nil {
		t.Fatalf("recorded: %v", err)
	}
	if !recorded.Present || !recorded.Reconciled {
		t.Fatalf("recorded = %+v, want reconciled", recorded)
	}
	meta := readMetadata(metaPath)
	if !recorded.RecordedAt.Equal(meta.GeneratedAt) || recorded.RecordedAt.IsZero() {
		t.Errorf("recorded time = %v, want the persisted %v", recorded.RecordedAt, meta.GeneratedAt)
	}

	// The reads never mutate the persisted files.
	if got := read(t, planPath); got == beforePlan {
		t.Error("the reconciliation should have persisted the new plan")
	}
	metaBefore := read(t, metaPath)
	planBefore := read(t, planPath)
	if _, err := TaskReconciliationProvenance(dir, "S001"); err != nil {
		t.Fatalf("reread: %v", err)
	}
	if _, err := TaskDefinitionIdentity(dir, "S001"); err != nil {
		t.Fatalf("reread definition: %v", err)
	}
	if read(t, metaPath) != metaBefore || read(t, planPath) != planBefore {
		t.Error("the reads must not mutate plan.json or plan.meta.json")
	}
	assertOnlyKnownStateFiles(t, dir)
}

// TestReconcileRecordsTaskScopedAcceptance proves reconciled acceptance is bound to
// the task, the concrete definition change, and the acceptance time - and that a
// later plan write cannot move that acceptance time the way GeneratedAt moves.
func TestReconcileRecordsTaskScopedAcceptance(t *testing.T) {
	dir := t.TempDir()
	st := prepareForReconcile(t, dir, "PLAN.md", planDoc)
	markExecuted(taskByID(t, st.tasks, "S001"))

	beforeIdentity, err := TaskDefinitionIdentity(dir, "S001")
	if err != nil || !beforeIdentity.Present {
		t.Fatalf("before identity: %v present=%v", err, beforeIdentity.Present)
	}

	write(t, dir, "PLAN.md", strings.ReplaceAll(planDoc, "Application skeleton", "Renamed"))
	if _, err := reconcileAccept(t, dir, "PLAN.md", st, "S001"); err != nil {
		t.Fatalf("reconcile --accept-changed: %v", err)
	}

	got, err := TaskReconciliationProvenance(dir, "S001")
	if err != nil {
		t.Fatalf("provenance: %v", err)
	}
	if !got.Accepted || got.AcceptedAt.IsZero() {
		t.Fatalf("provenance = %+v, want a task-scoped acceptance with a time", got)
	}
	afterIdentity, err := TaskDefinitionIdentity(dir, "S001")
	if err != nil || !afterIdentity.Present {
		t.Fatalf("after identity: %v present=%v", err, afterIdentity.Present)
	}
	if got.DefinitionDigestBefore != beforeIdentity.Digest {
		t.Errorf("digest before = %q, want the recorded definition %q", got.DefinitionDigestBefore, beforeIdentity.Digest)
	}
	if got.DefinitionDigestAfter != afterIdentity.Digest {
		t.Errorf("digest after = %q, want the current definition %q", got.DefinitionDigestAfter, afterIdentity.Digest)
	}
	if got.DefinitionDigestBefore == got.DefinitionDigestAfter {
		t.Error("the acceptance recorded no definition change")
	}

	// A later plan write refreshes GeneratedAt. It must not touch the acceptance
	// time, which is the only supported proof of when the change was accepted.
	firstAccepted := got.AcceptedAt
	metaPath := filepath.Join(dir, config.DirName, metaFileName)
	firstGenerated := readMetadata(metaPath).GeneratedAt
	if _, err := reconcileAccept(t, dir, "PLAN.md", st); err != nil {
		t.Fatalf("no-op reconcile: %v", err)
	}
	again, err := TaskReconciliationProvenance(dir, "S001")
	if err != nil {
		t.Fatalf("provenance after no-op: %v", err)
	}
	if !again.AcceptedAt.Equal(firstAccepted) {
		t.Errorf("acceptance time moved on a no-op write: %v -> %v", firstAccepted, again.AcceptedAt)
	}
	if !readMetadata(metaPath).GeneratedAt.After(firstGenerated) {
		t.Log("generated_at did not advance; the independence check is inert in this run")
	}
}

// TestUnrelatedAcceptedChangeDoesNotQualifyAnotherTask proves provenance is scoped
// to the task it names.
func TestUnrelatedAcceptedChangeDoesNotQualifyAnotherTask(t *testing.T) {
	dir := t.TempDir()
	writeMetadataFile(t, dir, Metadata{
		Source: "docs/PLAN.md", SourceKind: KindPlan, SourceSHA256: "sha", PlanID: "p",
		AcceptedChanges: []AcceptedChange{{
			TaskID:     "OTHER",
			AcceptedAt: time.Now().UTC(),
		}},
	})
	got, err := TaskReconciliationProvenance(dir, "S001")
	if err != nil {
		t.Fatalf("provenance: %v", err)
	}
	if got.Accepted || got.Present {
		t.Errorf("provenance = %+v, want absent for a task with no record", got)
	}
}

// TestAcceptedChangeWithoutTimeIsUnusable proves a record missing its acceptance
// time is not silently completed from other metadata.
func TestAcceptedChangeWithoutTimeIsUnusable(t *testing.T) {
	dir := t.TempDir()
	writeMetadataFile(t, dir, Metadata{
		Source: "docs/PLAN.md", SourceKind: KindPlan, SourceSHA256: "sha", PlanID: "p",
		GeneratedAt: time.Now().UTC(),
		AcceptedChanges: []AcceptedChange{{
			TaskID:                 "S001",
			DefinitionDigestBefore: "a",
			DefinitionDigestAfter:  "b",
		}},
	})
	got, err := TaskReconciliationProvenance(dir, "S001")
	if err != nil {
		t.Fatalf("provenance: %v", err)
	}
	if got.Accepted || !got.AcceptedAt.IsZero() {
		t.Errorf("provenance = %+v, want unusable evidence with no invented time", got)
	}
}

// writeMetadataFile persists plan metadata for a provenance test.
func writeMetadataFile(t *testing.T, dir string, meta Metadata) {
	t.Helper()
	sdlc := filepath.Join(dir, config.DirName)
	if err := os.MkdirAll(sdlc, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := writeMetadata(filepath.Join(sdlc, metaFileName), meta); err != nil {
		t.Fatalf("write metadata: %v", err)
	}
}

// TestTaskReconciliationProvenanceUnreadableMetadata proves an unreadable
// metadata file reports absent evidence without error or file creation.
func TestTaskReconciliationProvenanceUnreadableMetadata(t *testing.T) {
	dir := t.TempDir()
	sdlc := filepath.Join(dir, config.DirName)
	if err := os.MkdirAll(sdlc, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sdlc, metaFileName), []byte("not json"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := TaskReconciliationProvenance(dir, "S001")
	if err != nil {
		t.Fatalf("unreadable: %v", err)
	}
	if got.Present || got.Reconciled {
		t.Errorf("unreadable = %+v, want absent", got)
	}
}

// assertOnlyKnownStateFiles proves the reads introduced no new file under
// .agent-sdlc beyond the machine plan and its provenance.
func assertOnlyKnownStateFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dir, config.DirName))
	if err != nil {
		t.Fatalf("read state dir: %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		switch name {
		// The project state lock is a supported state file: reconciliation creates it
		// and holds it across the read-check-write it performs.
		case planFileName, metaFileName, archiveDirName, projectlock.FileName:
		default:
			t.Errorf("unexpected file under .agent-sdlc: %s", name)
		}
	}
}
