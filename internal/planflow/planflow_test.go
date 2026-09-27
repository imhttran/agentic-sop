package planflow

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/domain"
)

// fakeStore is an in-memory TaskStore.
type fakeStore struct{ tasks []*domain.Task }

func (f *fakeStore) List() ([]*domain.Task, error) { return f.tasks, nil }
func (f *fakeStore) SaveTasks(tasks []*domain.Task) error {
	f.tasks = append(f.tasks, tasks...)
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
	if res.PlanDoc != filepath.Join("docs", "PLAN.md") {
		t.Errorf("PlanDoc = %q", res.PlanDoc)
	}
	if _, err := os.Stat(filepath.Join(dir, "docs", "PLAN.md")); err != nil {
		t.Errorf("docs/PLAN.md not written: %v", err)
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
