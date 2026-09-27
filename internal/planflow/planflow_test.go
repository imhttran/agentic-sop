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
