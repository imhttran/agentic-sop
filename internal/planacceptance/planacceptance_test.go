package planacceptance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/outcome"
	"github.com/imhttran/agentic-sop/internal/planflow"
	"github.com/imhttran/agentic-sop/internal/planner"
)

type fixture struct {
	dir         string
	archiveRoot string
	archiveDir  string
	source      string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir := t.TempDir()
	source := "docs/plans/PLAN-TEST.md"
	f := &fixture{
		dir:         dir,
		archiveRoot: filepath.Join(dir, ".agent-sdlc", "archive", "worktree-evidence", "test-plan"),
		source:      source,
	}
	f.archiveDir = filepath.Join(f.archiveRoot, "archive")
	for _, p := range []string{
		filepath.Join(dir, ".agent-sdlc"),
		filepath.Join(dir, "docs", "plans"),
		filepath.Join(dir, "docs", "reports"),
		f.archiveDir,
		filepath.Join(f.archiveRoot, "runs"),
	} {
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	writeFile(t, filepath.Join(dir, ".agent-sdlc", "config.yaml"), "project:\n  name: \"test\"\nhuman:\n  designated_approvers:\n    - \"Approver\"\n")
	writeFile(t, filepath.Join(dir, source), "# Test plan\n")
	writeFile(t, filepath.Join(dir, "docs", "reports", "evidence.md"), "# Evidence\n\n## Reviewed section\nordinary evidence\n")

	stageA := planner.Stage{
		ID:                 "STAGE-A",
		AcceptanceCriteria: []string{"A one", "A two", "A three", "A four", "A five"},
		AcceptanceMapping: []planner.AcceptanceMapping{
			{Criterion: 1, Manual: "docs/reports/evidence.md §Reviewed section"},
			{Criterion: 2, Manual: "docs/reports/evidence.md §Reviewed section"},
			{Criterion: 3, Manual: "docs/reports/evidence.md §Reviewed section"},
			{Criterion: 4, Manual: "docs/reports/evidence.md §Reviewed section"},
			{Criterion: 5, Manual: "docs/reports/evidence.md §Reviewed section"},
		},
	}
	stageB := planner.Stage{
		ID:                 "STAGE-B",
		AcceptanceCriteria: []string{"B one", "B two", "B three", "B four", "B five"},
		AcceptanceChecks:   []string{"git rev-parse HEAD", "git status --porcelain"},
		AcceptanceMapping: []planner.AcceptanceMapping{
			{Criterion: 1, Check: 1},
			{Criterion: 2, Check: 2},
			{Criterion: 3, Check: 1},
			{Criterion: 4, Manual: "docs/reports/evidence.md §Reviewed section"},
			{Criterion: 5, Manual: "docs/reports/evidence.md §Reviewed section"},
		},
	}
	stageC := planner.Stage{
		ID:                 "STAGE-C",
		AcceptanceCriteria: []string{"C one", "C two", "C three", "C four", "C five", "C six"},
		AcceptanceMapping: []planner.AcceptanceMapping{
			{Criterion: 1, Manual: "docs/reports/evidence.md §Reviewed section"},
			{Criterion: 2, Manual: "docs/reports/evidence.md §Reviewed section"},
			{Criterion: 3, Manual: "docs/reports/evidence.md §Reviewed section"},
			{Criterion: 4, Manual: "docs/reports/evidence.md §Reviewed section"},
			{Criterion: 5, Manual: "docs/reports/evidence.md §Reviewed section"},
			{Criterion: 6, Manual: "docs/reports/evidence.md §Reviewed section"},
		},
	}
	plan := planner.Plan{Project: "test", Summary: "test", Stages: []planner.Stage{stageA, stageB, stageC}}
	writeJSON(t, filepath.Join(f.archiveDir, "plan.json"), plan)

	tasks := []domain.Task{
		{ID: "STAGE-A", AcceptanceCriteria: strings.Join(stageA.AcceptanceCriteria, "\n")},
		{ID: "STAGE-B", AcceptanceCriteria: strings.Join(stageB.AcceptanceCriteria, "\n")},
		{ID: "STAGE-C", AcceptanceCriteria: strings.Join(stageC.AcceptanceCriteria, "\n")},
	}
	writeJSON(t, filepath.Join(f.archiveDir, "tasks.json"), tasks)
	writeJSON(t, filepath.Join(f.archiveDir, "lifecycle.json"), map[string]string{
		"plan_id": "plan-test", "source": source, "disposition": "COMPLETE", "recorded_at": "2026-11-10T00:00:00Z",
	})
	return f
}

func (f *fixture) context(t *testing.T) *Context {
	t.Helper()
	ctx, err := LoadContext(f.dir, f.source, f.archiveRoot)
	if err != nil {
		t.Fatalf("LoadContext: %v", err)
	}
	return ctx
}

func (f *fixture) ref(t *testing.T) outcome.EvidenceRef {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.dir, "docs", "reports", "evidence.md"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	return outcome.EvidenceRef{Path: "docs/reports/evidence.md", SHA256: hex.EncodeToString(sum[:]), Section: "Reviewed section"}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestMappingsAndCountsAreGeneric(t *testing.T) {
	f := newFixture(t)
	ctx := f.context(t)
	if len(ctx.Mappings) != 16 {
		t.Fatalf("mappings = %d, want 16", len(ctx.Mappings))
	}
	counts := CountsFromMappings(ctx.Mappings)
	if counts.Manual != 13 || counts.Check != 3 || counts.DecisionEligible != 13 || counts.Unmapped != 0 {
		t.Fatalf("counts = %+v, want 13 manual / 3 check / 13 eligible", counts)
	}
	m, ok := FindMapping(ctx, "STAGE-A.criterion-1")
	if !ok || m.DisplayID != "STAGE-A.1" || m.Kind != outcome.EvidenceManual {
		t.Fatalf("STAGE-A.1 mapping = %+v, %v", m, ok)
	}
	c, ok := FindMapping(ctx, "STAGE-B.criterion-1")
	if !ok || c.Kind != outcome.EvidenceCheck || c.Command != "git rev-parse HEAD" {
		t.Fatalf("STAGE-B.criterion-1 mapping = %+v, %v", c, ok)
	}
}

func TestPlanIdentityIsDeterministicAndExcludesMutableFields(t *testing.T) {
	f := newFixture(t)
	rec1, err := planflow.LoadArchivedPlan(f.archiveDir)
	if err != nil {
		t.Fatal(err)
	}
	rec2, _ := planflow.LoadArchivedPlan(f.archiveDir)
	id1, err := PlanIdentity(rec1)
	if err != nil {
		t.Fatal(err)
	}
	id2, _ := PlanIdentity(rec2)
	if id1 != id2 {
		t.Fatalf("identity not deterministic: %s vs %s", id1, id2)
	}

	// Identical copy at another path retains the identity.
	copyDir := filepath.Join(t.TempDir(), "archive")
	if err := os.MkdirAll(copyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"plan.json", "tasks.json", "lifecycle.json"} {
		data, err := os.ReadFile(filepath.Join(f.archiveDir, name))
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(copyDir, name), string(data))
	}
	copied, err := planflow.LoadArchivedPlan(copyDir)
	if err != nil {
		t.Fatal(err)
	}
	copyID, _ := PlanIdentity(copied)
	if copyID != id1 {
		t.Fatalf("identical copy identity = %s, want %s", copyID, id1)
	}

	// Mutable task status/timestamps do not change the identity.
	tasksPath := filepath.Join(copyDir, "tasks.json")
	var tasks []domain.Task
	data, _ := os.ReadFile(tasksPath)
	if err := json.Unmarshal(data, &tasks); err != nil {
		t.Fatal(err)
	}
	tasks[0].Status = domain.LOCAL_DONE
	tasks[0].UpdatedAt = tasks[0].UpdatedAt.AddDate(1, 0, 0)
	writeJSON(t, tasksPath, tasks)
	mutable, _ := planflow.LoadArchivedPlan(copyDir)
	mutableID, _ := PlanIdentity(mutable)
	if mutableID != id1 {
		t.Fatalf("mutable task fields changed identity: %s vs %s", mutableID, id1)
	}

	// A changed requirement changes the identity.
	planPath := filepath.Join(copyDir, "plan.json")
	var plan planner.Plan
	data, _ = os.ReadFile(planPath)
	if err := json.Unmarshal(data, &plan); err != nil {
		t.Fatal(err)
	}
	plan.Stages[0].AcceptanceCriteria[0] = "changed requirement"
	writeJSON(t, planPath, plan)
	changed, _ := planflow.LoadArchivedPlan(copyDir)
	changedID, _ := PlanIdentity(changed)
	if changedID == id1 {
		t.Fatal("changed requirement retained the plan identity")
	}
}

func TestAnnotationBindsDecisionAndLaterAnnotationDoesNotAlterIt(t *testing.T) {
	f := newFixture(t)
	ctx := f.context(t)
	designated := []string{"Approver"}
	ref := f.ref(t)
	m, _ := FindMapping(ctx, "STAGE-C.criterion-1")

	ann1, err := RecordAnnotation(ctx, Annotation{
		CriterionID:      m.CriterionID,
		ReviewedEvidence: "reviewed evidence one",
		Reason:           "archived observation superseded by observed evidence",
		EvidenceRefs:     []outcome.EvidenceRef{ref},
		Approver:         "Approver",
	}, designated)
	if err != nil {
		t.Fatalf("RecordAnnotation: %v", err)
	}
	if ann1.ArchivedManualEvidence != m.ManualEvidence || ann1.ArchivedEvidenceSHA256 != sha256Hex(m.ManualEvidence) {
		t.Fatalf("annotation did not preserve archived evidence: %+v", ann1)
	}
	if _, err := RecordDecision(ctx, outcome.AcceptanceDecision{
		CriterionID:  m.CriterionID,
		Decision:     outcome.DecisionAccept,
		Evidence:     "reviewed evidence one",
		Approver:     "Approver",
		AnnotationID: ann1.ID,
		EvidenceRefs: []outcome.EvidenceRef{ref},
	}, designated); err != nil {
		t.Fatalf("RecordDecision: %v", err)
	}

	rep, err := Evaluate(ctx, designated)
	if err != nil {
		t.Fatal(err)
	}
	if statusFor(rep, m.CriterionID) != StatusManuallyAccepted || rep.Accepted != 1 {
		t.Fatalf("status = %s accepted=%d, want manually-accepted/1", statusFor(rep, m.CriterionID), rep.Accepted)
	}

	ann2, err := RecordAnnotation(ctx, Annotation{
		CriterionID:      m.CriterionID,
		ReviewedEvidence: "reviewed evidence two",
		Reason:           "another later annotation",
		EvidenceRefs:     []outcome.EvidenceRef{ref},
		Approver:         "Approver",
	}, designated)
	if err != nil {
		t.Fatalf("RecordAnnotation 2: %v", err)
	}
	if ann2.ID == ann1.ID {
		t.Fatal("different annotations produced the same identity")
	}
	rep, err = Evaluate(ctx, designated)
	if err != nil {
		t.Fatal(err)
	}
	if statusFor(rep, m.CriterionID) != StatusManuallyAccepted || rep.Accepted != 1 {
		t.Fatalf("later annotation altered the earlier decision: status=%s accepted=%d", statusFor(rep, m.CriterionID), rep.Accepted)
	}
}

func TestEvidenceChangeMakesDecisionStale(t *testing.T) {
	f := newFixture(t)
	ctx := f.context(t)
	designated := []string{"Approver"}
	ref := f.ref(t)
	m, _ := FindMapping(ctx, "STAGE-A.criterion-1")
	if _, err := RecordDecision(ctx, outcome.AcceptanceDecision{
		CriterionID:  m.CriterionID,
		Decision:     outcome.DecisionAccept,
		Evidence:     "evidence",
		Approver:     "Approver",
		EvidenceRefs: []outcome.EvidenceRef{ref},
	}, designated); err != nil {
		t.Fatalf("RecordDecision: %v", err)
	}
	rep, _ := Evaluate(ctx, designated)
	if statusFor(rep, m.CriterionID) != StatusManuallyAccepted {
		t.Fatalf("status = %s, want manually-accepted", statusFor(rep, m.CriterionID))
	}

	writeFile(t, filepath.Join(f.dir, "docs", "reports", "evidence.md"), "# Evidence changed\n")
	rep, _ = Evaluate(ctx, designated)
	if statusFor(rep, m.CriterionID) != StatusStale || rep.Accepted != 0 {
		t.Fatalf("changed evidence status=%s accepted=%d, want stale/0", statusFor(rep, m.CriterionID), rep.Accepted)
	}
}

func TestWrongEvidenceHashIsRejected(t *testing.T) {
	f := newFixture(t)
	ctx := f.context(t)
	ref := f.ref(t)
	ref.SHA256 = strings.Repeat("0", 64)
	m, _ := FindMapping(ctx, "STAGE-A.criterion-1")
	if _, err := RecordDecision(ctx, outcome.AcceptanceDecision{
		CriterionID: m.CriterionID, Decision: outcome.DecisionAccept, Evidence: "e", Approver: "Approver", EvidenceRefs: []outcome.EvidenceRef{ref},
	}, []string{"Approver"}); err == nil {
		t.Fatal("wrong evidence hash was accepted")
	}
}

func TestCorruptLedgerFailsClosed(t *testing.T) {
	f := newFixture(t)
	ctx := f.context(t)
	path := filepath.Join(ctx.LedgerDir, "decisions.jsonl")
	if err := os.MkdirAll(ctx.LedgerDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, `{"criterion_id":"STAGE-A.criterion-1"`)

	rep, err := Evaluate(ctx, []string{"Approver"})
	if err != nil {
		t.Fatalf("Evaluate: %v", err)
	}
	if rep.Corrupt == nil {
		t.Fatal("corrupt ledger was not reported")
	}
	if _, err := RecordDecision(ctx, outcome.AcceptanceDecision{
		CriterionID: "STAGE-A.criterion-1", Decision: outcome.DecisionAccept, Evidence: "e", Approver: "Approver", EvidenceRefs: []outcome.EvidenceRef{f.ref(t)},
	}, []string{"Approver"}); err == nil {
		t.Fatal("decision was appended after a corrupt ledger")
	}
}

func TestCheckEvidenceStates(t *testing.T) {
	base := func(t *testing.T, passed, unchanged, dirty bool) *Report {
		f := newFixture(t)
		ctx := f.context(t)
		m, _ := FindMapping(ctx, "STAGE-B.criterion-1")
		if _, err := RecordCheck(ctx, CheckEvidence{
			CriterionID: m.CriterionID, CheckRef: m.CheckRef, Command: m.Command,
			Argv: []string{"git", "rev-parse", "HEAD"}, Class: "SAFE", Executed: true,
			Passed: passed, ExitCode: 0, TestedRevision: "abc123", TestedDirty: dirty, SourceUnchanged: unchanged,
		}); err != nil {
			t.Fatalf("RecordCheck: %v", err)
		}
		rep, err := Evaluate(ctx, []string{"Approver"})
		if err != nil {
			t.Fatal(err)
		}
		return rep
	}
	if got := statusFor(base(t, true, true, false), "STAGE-B.criterion-1"); got != StatusCheckPassed {
		t.Fatalf("passed check status = %s", got)
	}
	if got := statusFor(base(t, false, true, false), "STAGE-B.criterion-1"); got != StatusRejected {
		t.Fatalf("failed check status = %s", got)
	}
	if got := statusFor(base(t, true, false, false), "STAGE-B.criterion-1"); got != StatusStale {
		t.Fatalf("source-changed check status = %s", got)
	}
	if got := statusFor(base(t, true, true, true), "STAGE-B.criterion-1"); got != StatusStale {
		t.Fatalf("dirty check status = %s", got)
	}
}

func TestCheckMappedCriterionRejectsManualDecision(t *testing.T) {
	f := newFixture(t)
	ctx := f.context(t)
	_, err := RecordDecision(ctx, outcome.AcceptanceDecision{
		CriterionID: "STAGE-B.criterion-1", Decision: outcome.DecisionAccept, Evidence: "e", Approver: "Approver", EvidenceRefs: []outcome.EvidenceRef{f.ref(t)},
	}, []string{"Approver"})
	if err == nil || !strings.Contains(err.Error(), "check-mapped") {
		t.Fatalf("error = %v, want check-mapped rejection", err)
	}
}

func statusFor(rep *Report, id string) CriterionStatus {
	for _, c := range rep.Criteria {
		if c.Mapping.CriterionID == id {
			return c.Status
		}
	}
	return ""
}

func TestRecordingDoesNotMutateArchive(t *testing.T) {
	f := newFixture(t)
	before := archiveHashes(t, f.archiveDir)
	ctx := f.context(t)
	ref := f.ref(t)
	m, _ := FindMapping(ctx, "STAGE-A.criterion-1")
	if _, err := RecordDecision(ctx, outcome.AcceptanceDecision{
		CriterionID: m.CriterionID, Decision: outcome.DecisionAccept, Evidence: "e", Approver: "Approver", EvidenceRefs: []outcome.EvidenceRef{ref},
	}, []string{"Approver"}); err != nil {
		t.Fatal(err)
	}
	after := archiveHashes(t, f.archiveDir)
	if len(before) != len(after) {
		t.Fatalf("archive file set changed: %v -> %v", before, after)
	}
	for name, sum := range before {
		if after[name] != sum {
			t.Fatalf("archive file %s changed", name)
		}
	}
}

func TestVerifyDoesNotCreateLedger(t *testing.T) {
	f := newFixture(t)
	ctx := f.context(t)
	if _, err := Evaluate(ctx, []string{"Approver"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(ctx.LedgerDir); !os.IsNotExist(err) {
		t.Fatalf("verify created the ledger directory %s", ctx.LedgerDir)
	}
}

func archiveHashes(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		out[e.Name()] = hex.EncodeToString(sum[:])
	}
	return out
}

func TestArchivedHistoricalObservationIsNotInferred(t *testing.T) {
	f := newFixture(t)
	planPath := filepath.Join(f.archiveDir, "plan.json")
	var plan planner.Plan
	data, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &plan); err != nil {
		t.Fatal(err)
	}
	for i := range plan.Stages {
		if plan.Stages[i].ID != "STAGE-C" {
			continue
		}
		for j := range plan.Stages[i].AcceptanceMapping {
			plan.Stages[i].AcceptanceMapping[j].Manual = "live run artifact recording outcomes (currently NOT_RUN)"
		}
	}
	writeJSON(t, planPath, plan)

	ctx := f.context(t)
	rep, err := Evaluate(ctx, []string{"Approver"})
	if err != nil {
		t.Fatal(err)
	}
	if got := statusFor(rep, "STAGE-C.criterion-1"); got != StatusDecisionEligible {
		t.Fatalf("archived observation text was inferred as %s, want decision-eligible", got)
	}
	if rep.Counts.Stale != 0 {
		t.Fatalf("stale = %d, want 0 (no observation flag survives the archive)", rep.Counts.Stale)
	}
}
