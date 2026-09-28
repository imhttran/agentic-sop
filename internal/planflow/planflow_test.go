package planflow

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/config"
	"github.com/imhttran/agentic-sop/internal/domain"
)

// fakeStore is an in-memory TaskStore and GraphStore.
type fakeStore struct {
	tasks []*domain.Task
	// replaceErr, when set, makes ReplaceGraph fail, modelling a persistence
	// failure during reconciliation.
	replaceErr error
}

func (f *fakeStore) List() ([]*domain.Task, error) { return f.tasks, nil }
func (f *fakeStore) SaveTasks(tasks []*domain.Task) error {
	f.tasks = append(f.tasks, tasks...)
	return nil
}
func (f *fakeStore) ClearTasks() error {
	f.tasks = nil
	return nil
}

// ReplaceGraph mirrors the store primitive: it removes the listed IDs and
// upserts the rest, preserving each upserted task's full value.
func (f *fakeStore) ReplaceGraph(tasks []*domain.Task, remove []string) error {
	if f.replaceErr != nil {
		return f.replaceErr
	}
	removed := make(map[string]bool, len(remove))
	for _, id := range remove {
		removed[id] = true
	}
	kept := make([]*domain.Task, 0, len(f.tasks))
	for _, t := range f.tasks {
		if !removed[t.ID] {
			kept = append(kept, t)
		}
	}
	for _, t := range tasks {
		replaced := false
		for i, existing := range kept {
			if existing.ID == t.ID {
				kept[i] = t
				replaced = true
				break
			}
		}
		if !replaced {
			kept = append(kept, t)
		}
	}
	f.tasks = kept
	return nil
}

// fakeAgent returns fixed content.
type fakeAgent struct{ content string }

func (f fakeAgent) Generate(context.Context, agent.Request) (agent.Response, error) {
	return agent.Response{Content: f.content}, nil
}

const planDoc = "# Implementation Plan\n\n## Project\n\nWidget\n\n## Summary\n\nAdd a widget.\n\n## S001 — Application skeleton\n\nCreate it.\n\n### Acceptance Criteria\n\n- starts\n"

const generatedPlanJSON = `{"project":"Generated","summary":"s","stages":[{"id":"S001","title":"One","objective":"o","dependencies":[],"deliverables":["d"],"acceptance_criteria":["a"]}]}`

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareCompilesPlanAndCreatesTasks(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, filepath.Join("docs", "PLAN.md"), planDoc)
	st := &fakeStore{}

	res, err := Prepare(context.Background(), Options{Dir: dir, Store: st})
	if err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}
	if res.Source != filepath.Join("docs", "PLAN.md") || res.SourceKind != KindPlan {
		t.Errorf("res = %+v", res)
	}
	if !res.PlanRebuilt || res.TasksCreated != 1 {
		t.Errorf("res = %+v", res)
	}
	if len(st.tasks) != 1 || st.tasks[0].ID != "S001" {
		t.Errorf("tasks = %+v", st.tasks)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agent-sdlc", "plan.json")); err != nil {
		t.Errorf("plan.json not written: %v", err)
	}
}

func TestPrepareReusesUnchangedPlan(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "PLAN.md", planDoc)
	if _, err := Prepare(context.Background(), Options{Dir: dir, Store: &fakeStore{}}); err != nil {
		t.Fatal(err)
	}

	// A fresh (task-less) store with the same source reuses the machine plan.
	res, err := Prepare(context.Background(), Options{Dir: dir, Store: &fakeStore{}})
	if err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}
	if res.PlanRebuilt {
		t.Error("an unchanged source must reuse the plan, not rebuild it")
	}
	if res.TasksCreated != 1 {
		t.Errorf("tasks = %d, want 1", res.TasksCreated)
	}
}

func TestPrepareRebuildsWhenSourceChanges(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "PLAN.md", planDoc)
	if _, err := Prepare(context.Background(), Options{Dir: dir, Store: &fakeStore{}}); err != nil {
		t.Fatal(err)
	}

	write(t, dir, "PLAN.md", strings.ReplaceAll(planDoc, "Application skeleton", "Changed title"))
	res, err := Prepare(context.Background(), Options{Dir: dir, Store: &fakeStore{}})
	if err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}
	if !res.PlanRebuilt {
		t.Error("a changed source must trigger a rebuild")
	}
}

func TestPrepareGeneratesFromPRD(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, filepath.Join("docs", "PRD.md"), "# PRD\n\nDo the thing.\n")

	res, err := Prepare(context.Background(), Options{Dir: dir, Agent: fakeAgent{content: generatedPlanJSON}, Store: &fakeStore{}})
	if err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}
	if res.SourceKind != KindPRD || !res.PlanRebuilt || res.TasksCreated != 1 {
		t.Errorf("res = %+v", res)
	}
}

func TestPreparePrefersPlanOverPRD(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, filepath.Join("docs", "PRD.md"), "# PRD\n")
	write(t, dir, filepath.Join("docs", "PLAN.md"), planDoc)

	// The plan compiles deterministically, so the agent is never consulted.
	res, err := Prepare(context.Background(), Options{Dir: dir, Agent: fakeAgent{content: "SHOULD NOT BE USED"}, Store: &fakeStore{}})
	if err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}
	if res.SourceKind != KindPlan || res.Source != filepath.Join("docs", "PLAN.md") {
		t.Errorf("res = %+v", res)
	}
}

func TestPrepareNoSource(t *testing.T) {
	dir := t.TempDir()
	_, err := Prepare(context.Background(), Options{Dir: dir, Store: &fakeStore{}})
	if err == nil {
		t.Fatal("expected an error when there is no source")
	}
	if !strings.Contains(err.Error(), "PLAN.md") {
		t.Errorf("error = %q, want it to name the sources", err)
	}
}

func TestPrepareSkipsWhenTasksExist(t *testing.T) {
	dir := t.TempDir()
	st := &fakeStore{tasks: []*domain.Task{{ID: "T001"}}}

	res, err := Prepare(context.Background(), Options{Dir: dir, Store: st})
	if err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}
	if res.SourceKind != KindExisting || res.TasksCreated != 0 {
		t.Errorf("res = %+v", res)
	}
}

func TestPrepareGeneratesPlanDocFromPRD(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, filepath.Join("docs", "PRD.md"), "# PRD\n\nDo it.\n")

	res, err := Prepare(context.Background(), Options{Dir: dir, Agent: fakeAgent{content: generatedPlanJSON}, Store: &fakeStore{}})
	if err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}
	if res.PlanDoc != filepath.Join("docs", "reports", "prd.md") {
		t.Errorf("PlanDoc = %q", res.PlanDoc)
	}
	if _, err := os.Stat(filepath.Join(dir, "docs", "reports", "prd.md")); err != nil {
		t.Errorf("docs/reports/prd.md not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "PLAN.md")); !os.IsNotExist(err) {
		t.Error("generated plan must not be written to the project root")
	}
}

func TestPrepareReconcilesChangedPlanWithTasks(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "PLAN.md", planDoc)
	st := &fakeStore{}

	if _, err := Prepare(context.Background(), Options{Dir: dir, Store: st}); err != nil {
		t.Fatalf("first Prepare failed: %v", err)
	}
	if len(st.tasks) == 0 {
		t.Fatal("expected tasks to be created")
	}

	// Change the plan after tasks exist.
	write(t, dir, "PLAN.md", strings.ReplaceAll(planDoc, "Application skeleton", "Renamed"))
	_, err := Prepare(context.Background(), Options{Dir: dir, Store: st})
	if err == nil {
		t.Fatal("expected a reconciliation error")
	}
	if !strings.Contains(err.Error(), "plan changed") {
		t.Errorf("error = %q, want a reconciliation diagnostic", err)
	}
}

func TestPreparePlainRunResumesUnresolvedActivePlan(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "PLAN-A.md", planDoc)
	// An unrelated plan that discovery would otherwise prefer by precedence.
	write(t, dir, filepath.Join("docs", "PLAN.md"), planDocB)
	st := &fakeStore{}
	if _, err := Prepare(context.Background(), Options{Dir: dir, PlanSource: filepath.Join(dir, "PLAN-A.md"), Store: st}); err != nil {
		t.Fatalf("prepare PLAN-A: %v", err)
	}

	// The active plan (PLAN-A.md) still has unresolved work. A plain run with no
	// plan argument must resume it rather than discover docs/PLAN.md and report a
	// different plan is active.
	res, err := Prepare(context.Background(), Options{Dir: dir, Store: st})
	if err != nil {
		t.Fatalf("plain run must resume the active plan: %v", err)
	}
	if res.Source != "PLAN-A.md" || res.PlanRebuilt || res.TasksCreated != 0 {
		t.Errorf("res = %+v, want a resume of PLAN-A.md", res)
	}
	if len(st.tasks) != 1 || st.tasks[0].ID != "S001" {
		t.Errorf("tasks = %+v, want the active graph preserved", st.tasks)
	}
}

func TestPreparePlainRunStillGatesChangedActiveSource(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "PLAN-A.md", planDoc)
	write(t, dir, filepath.Join("docs", "PLAN.md"), planDocB)
	st := &fakeStore{}
	if _, err := Prepare(context.Background(), Options{Dir: dir, PlanSource: filepath.Join(dir, "PLAN-A.md"), Store: st}); err != nil {
		t.Fatalf("prepare PLAN-A: %v", err)
	}
	// The active plan's own source changed: provenance must still gate, even on a
	// plain run that no longer discovers the unrelated docs/PLAN.md.
	write(t, dir, "PLAN-A.md", strings.ReplaceAll(planDoc, "Application skeleton", "Renamed"))

	_, err := Prepare(context.Background(), Options{Dir: dir, Store: st})
	if err == nil || !strings.Contains(err.Error(), "plan changed") {
		t.Fatalf("err = %v, want a plan-changed diagnostic for the active source", err)
	}
}

func TestPreparePlainRunResumesWhenActiveSourceMissing(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "PLAN-A.md", planDoc)
	write(t, dir, filepath.Join("docs", "PLAN.md"), planDocB)
	st := &fakeStore{}
	if _, err := Prepare(context.Background(), Options{Dir: dir, PlanSource: filepath.Join(dir, "PLAN-A.md"), Store: st}); err != nil {
		t.Fatalf("prepare PLAN-A: %v", err)
	}
	if err := os.Remove(filepath.Join(dir, "PLAN-A.md")); err != nil {
		t.Fatal(err)
	}

	// The recorded source is gone; the persisted graph is still authoritative, so
	// a plain run resumes it rather than discovering docs/PLAN.md.
	res, err := Prepare(context.Background(), Options{Dir: dir, Store: st})
	if err != nil {
		t.Fatalf("plain run must resume the persisted graph: %v", err)
	}
	if res.Source != "PLAN-A.md" || res.PlanRebuilt {
		t.Errorf("res = %+v, want a resume of the recorded source", res)
	}
}

func TestPrepareSurfacesUnknownDependency(t *testing.T) {
	dir := t.TempDir()
	doc := "# Plan\n\n## Project\n\nP\n\n## Summary\n\nS\n\n## S001 — a\n\no\n\n### Dependencies\n\n- S999\n\n### Acceptance Criteria\n\n- x\n"
	write(t, dir, "PLAN.md", doc)

	_, err := Prepare(context.Background(), Options{Dir: dir, Store: &fakeStore{}})
	if err == nil {
		t.Fatal("expected a validation error")
	}
	if !strings.Contains(err.Error(), "Plan validation failed") || !strings.Contains(err.Error(), "unknown dependency") {
		t.Errorf("error = %q, want an actionable validation diagnostic", err)
	}
}

func TestPrepareRequiresAgentForPRD(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, filepath.Join("docs", "PRD.md"), "# PRD\n")

	_, err := Prepare(context.Background(), Options{Dir: dir, Store: &fakeStore{}})
	if err == nil {
		t.Fatal("expected an error when no agent is available to generate a plan")
	}
	if !strings.Contains(err.Error(), "no agent") {
		t.Errorf("error = %q, want an actionable no-agent message", err)
	}
}

const planDocB = "# Implementation Plan\n\n## Project\n\nWidget\n\n## Summary\n\nSecond plan.\n\n## B001 — Second stage\n\nDo it.\n\n### Acceptance Criteria\n\n- works\n"

// completePlan prepares a plan and marks every created task satisfied, modelling
// a finished active plan.
func completePlan(t *testing.T, dir, name string, status domain.TaskStatus) *fakeStore {
	t.Helper()
	st := &fakeStore{}
	if _, err := Prepare(context.Background(), Options{Dir: dir, PlanSource: filepath.Join(dir, name), Store: st}); err != nil {
		t.Fatalf("prepare %s: %v", name, err)
	}
	for _, task := range st.tasks {
		task.Status = status
	}
	return st
}

func TestPrepareHandsOffCompletedPlan(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "PLAN-A.md", planDoc)
	write(t, dir, "PLAN-B.md", planDocB)
	st := completePlan(t, dir, "PLAN-A.md", domain.LOCAL_DONE)

	res, err := Prepare(context.Background(), Options{Dir: dir, PlanSource: filepath.Join(dir, "PLAN-B.md"), Store: st})
	if err != nil {
		t.Fatalf("handoff failed: %v", err)
	}
	if res.Source != "PLAN-B.md" || !res.PlanRebuilt || res.TasksCreated != 1 {
		t.Errorf("res = %+v", res)
	}
	// The graph now holds only the requested plan's tasks: two plans are never
	// mixed.
	if len(st.tasks) != 1 || st.tasks[0].ID != "B001" {
		t.Errorf("tasks = %+v, want only B001", st.tasks)
	}
}

func TestPrepareHandoffAcceptsSatisfiedStates(t *testing.T) {
	for _, status := range []domain.TaskStatus{domain.DONE, domain.LOCAL_DONE, domain.MERGED} {
		t.Run(string(status), func(t *testing.T) {
			dir := t.TempDir()
			write(t, dir, "PLAN-A.md", planDoc)
			write(t, dir, "PLAN-B.md", planDocB)
			st := completePlan(t, dir, "PLAN-A.md", status)

			res, err := Prepare(context.Background(), Options{Dir: dir, PlanSource: filepath.Join(dir, "PLAN-B.md"), Store: st})
			if err != nil {
				t.Fatalf("status %s must be satisfied: %v", status, err)
			}
			if res.Source != "PLAN-B.md" || len(st.tasks) != 1 || st.tasks[0].ID != "B001" {
				t.Errorf("status %s: res=%+v tasks=%+v", status, res, st.tasks)
			}
		})
	}
}

func TestPrepareHandoffBlocksUnresolvedWork(t *testing.T) {
	for _, status := range []domain.TaskStatus{
		domain.PLANNED, domain.READY, domain.BRANCH_CREATED, domain.IMPLEMENTING,
		domain.FIX_REQUIRED, domain.REVIEW, domain.BLOCKED,
	} {
		t.Run(string(status), func(t *testing.T) {
			dir := t.TempDir()
			write(t, dir, "PLAN-A.md", planDoc)
			write(t, dir, "PLAN-B.md", planDocB)
			st := completePlan(t, dir, "PLAN-A.md", status)

			_, err := Prepare(context.Background(), Options{Dir: dir, PlanSource: filepath.Join(dir, "PLAN-B.md"), Store: st})
			if err == nil {
				t.Fatalf("status %s: expected NEEDS_HUMAN, got handoff", status)
			}
			if !strings.Contains(err.Error(), "different plan is already active") {
				t.Errorf("status %s: error = %q", status, err)
			}
			// The unfinished graph is left exactly as it was.
			if len(st.tasks) != 1 || st.tasks[0].ID != "S001" {
				t.Errorf("status %s: tasks = %+v, want untouched S001", status, st.tasks)
			}
		})
	}
}

func TestPrepareHandoffArchivesCompletedRecord(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "PLAN-A.md", planDoc)
	write(t, dir, "PLAN-B.md", planDocB)
	st := completePlan(t, dir, "PLAN-A.md", domain.LOCAL_DONE)

	if _, err := Prepare(context.Background(), Options{Dir: dir, PlanSource: filepath.Join(dir, "PLAN-B.md"), Store: st}); err != nil {
		t.Fatalf("handoff failed: %v", err)
	}

	archive := filepath.Join(dir, config.DirName, "archive", "plan-a")
	for _, name := range []string{"plan.json", "plan.meta.json", "tasks.json"} {
		if _, err := os.Stat(filepath.Join(archive, name)); err != nil {
			t.Errorf("archived %s missing: %v", name, err)
		}
	}
	data, err := os.ReadFile(filepath.Join(archive, "tasks.json"))
	if err != nil {
		t.Fatalf("read archived tasks: %v", err)
	}
	if !strings.Contains(string(data), "S001") {
		t.Errorf("archived tasks do not record the completed plan: %s", data)
	}
}

func TestPrepareHandoffSamePlanIsUnchanged(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "PLAN-A.md", planDoc)
	st := completePlan(t, dir, "PLAN-A.md", domain.LOCAL_DONE)

	res, err := Prepare(context.Background(), Options{Dir: dir, PlanSource: filepath.Join(dir, "PLAN-A.md"), Store: st})
	if err != nil {
		t.Fatalf("re-request failed: %v", err)
	}
	if res.TasksCreated != 0 || len(st.tasks) != 1 || st.tasks[0].ID != "S001" {
		t.Errorf("res=%+v tasks=%+v", res, st.tasks)
	}
	if _, err := os.Stat(filepath.Join(dir, config.DirName, "archive")); !os.IsNotExist(err) {
		t.Errorf("same-plan request must not archive, stat err = %v", err)
	}
}

func TestPrepareHandoffRejectsInvalidNewPlan(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "PLAN-A.md", planDoc)
	// The requested plan references a stage that does not exist.
	write(t, dir, "PLAN-B.md", "# Plan\n\n## Project\n\nP\n\n## Summary\n\nS\n\n## B001 — b\n\no\n\n### Dependencies\n\n- B999\n\n### Acceptance Criteria\n\n- x\n")
	st := completePlan(t, dir, "PLAN-A.md", domain.LOCAL_DONE)

	_, err := Prepare(context.Background(), Options{Dir: dir, PlanSource: filepath.Join(dir, "PLAN-B.md"), Store: st})
	if err == nil {
		t.Fatal("expected the invalid new plan to fail")
	}
	if !strings.Contains(err.Error(), "Plan validation failed") {
		t.Errorf("error = %q", err)
	}
	// Nothing was mutated: the completed plan's record is intact and no archive
	// (which would imply a partial handoff) was written.
	if len(st.tasks) != 1 || st.tasks[0].ID != "S001" {
		t.Errorf("tasks = %+v, want untouched S001", st.tasks)
	}
	if _, statErr := os.Stat(filepath.Join(dir, config.DirName, "archive")); !os.IsNotExist(statErr) {
		t.Errorf("failed handoff must not archive, stat err = %v", statErr)
	}
}

func TestPlanID(t *testing.T) {
	cases := map[string]string{
		"docs/PLAN-Hardening.md": "plan-hardening",
		"PLAN.md":                "plan",
		"docs/PLAN-Jev.md":       "plan-jev",
		"":                       "",
	}
	for in, want := range cases {
		if got := planID(in); got != want {
			t.Errorf("planID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolvePlanPath(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "PLAN-C.md", "x")
	write(t, dir, filepath.Join("docs", "PLAN-D.md"), "x")
	write(t, dir, "PLAN-E.md", "x")
	write(t, dir, filepath.Join("docs", "PLAN-E.md"), "x")

	if p, err := ResolvePlanPath(dir, "PLAN-C.md"); err != nil || p != filepath.Join(dir, "PLAN-C.md") {
		t.Errorf("root: p=%q err=%v", p, err)
	}
	if p, err := ResolvePlanPath(dir, "PLAN-D.md"); err != nil || p != filepath.Join(dir, "docs", "PLAN-D.md") {
		t.Errorf("docs fallback: p=%q err=%v", p, err)
	}
	if p, err := ResolvePlanPath(dir, filepath.Join("docs", "PLAN-D.md")); err != nil || p != filepath.Join(dir, "docs", "PLAN-D.md") {
		t.Errorf("explicit docs: p=%q err=%v", p, err)
	}
	if p, err := ResolvePlanPath(dir, filepath.Join(dir, "PLAN-C.md")); err != nil || p != filepath.Join(dir, "PLAN-C.md") {
		t.Errorf("absolute: p=%q err=%v", p, err)
	}

	if _, err := ResolvePlanPath(dir, "PLAN-E.md"); err == nil || !strings.Contains(err.Error(), "multiple plans match") {
		t.Errorf("ambiguous: err=%v", err)
	}
	if _, err := ResolvePlanPath(dir, "NOPE.md"); err == nil || !strings.Contains(err.Error(), "PLAN not found") {
		t.Errorf("missing: err=%v", err)
	}
}
