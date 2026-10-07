package continuation

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/domain"
	"github.com/imhttran/agentic-sop/internal/planflow"
)

// memStore is the minimal graph/task store planflow uses. List is the only method
// Inspect calls; the rest let Prepare create the initial graph.
type memStore struct{ tasks []*domain.Task }

func (m *memStore) List() ([]*domain.Task, error) { return m.tasks, nil }
func (m *memStore) SaveTasks(tasks []*domain.Task) error {
	m.tasks = append(m.tasks, tasks...)
	return nil
}
func (m *memStore) ClearTasks() error {
	m.tasks = nil
	return nil
}
func (m *memStore) ReplaceGraph(tasks []*domain.Task, remove []string) error {
	removed := make(map[string]bool, len(remove))
	for _, id := range remove {
		removed[id] = true
	}
	kept := m.tasks[:0]
	for _, t := range m.tasks {
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
	m.tasks = kept
	return nil
}

const planDocClean = "# Implementation Plan\n\n## Project\n\nWidget\n\n## Summary\n\nAdd a widget.\n\n## S001 — Application skeleton\n\nCreate it.\n\n### Acceptance Criteria\n\n- starts\n"

func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// prepareReal writes a plan document, prepares the project with the real planflow
// path, and returns the store and the absolute source path.
func prepareReal(t *testing.T, dir, rel, doc string) (*memStore, string) {
	t.Helper()
	writeFile(t, dir, rel, doc)
	source := filepath.Join(dir, rel)
	st := &memStore{}
	if _, err := planflow.Prepare(context.Background(), planflow.Options{
		Dir:        dir,
		PlanSource: source,
		Store:      st,
	}); err != nil {
		t.Fatalf("prepare: %v", err)
	}
	return st, source
}

func markExecuted(t *testing.T, task *domain.Task) {
	t.Helper()
	task.Status = domain.LOCAL_DONE
	task.Attempt = 1
	task.Attempts = []domain.Attempt{{Number: 1, Status: domain.LOCAL_DONE, Reason: "done"}}
}

// Scenario 1 (real planflow): an unchanged source reconciles cleanly and the gate
// authorizes continuation, with the persisted graph untouched.
func TestCheckRealPlanflowClean(t *testing.T) {
	dir := t.TempDir()
	st, source := prepareReal(t, dir, "docs/plans/PLAN.md", planDocClean)

	res := Check(context.Background(), Options{Dir: dir, PlanSource: source, Tasks: st.tasks, Store: st})
	if res.Classification != Clean {
		t.Fatalf("classification = %s, want %s (%s)", res.Classification, Clean, res.Reason)
	}
	if res.NextAction != ContinueSafe {
		t.Fatalf("next action = %s, want %s (%s)", res.NextAction, ContinueSafe, res.Reason)
	}
	if res.Source != "docs/plans/PLAN.md" {
		t.Errorf("source = %q, want docs/plans/PLAN.md", res.Source)
	}
}

// Scenario 5 (real planflow): a material acceptance change on an executed task is
// classified a semantic change and stops, and the source/graph are not mutated.
func TestCheckRealPlanflowExecutedAcceptanceChangeStops(t *testing.T) {
	dir := t.TempDir()
	st, source := prepareReal(t, dir, "docs/plans/PLAN.md", planDocClean)
	markExecuted(t, st.tasks[0])

	changed := strings.Replace(planDocClean, "- starts", "- starts and stops cleanly", 1)
	writeFile(t, dir, "docs/plans/PLAN.md", changed)

	beforePlan := readFile(t, filepath.Join(dir, ".agent-sdlc", "plan.json"))
	res := Check(context.Background(), Options{Dir: dir, PlanSource: source, Tasks: st.tasks, Store: st})
	if res.Classification != SemanticChange {
		t.Fatalf("classification = %s, want %s (%s)", res.Classification, SemanticChange, res.Reason)
	}
	if res.NextAction != HumanReviewRequired {
		t.Fatalf("next action = %s, want %s", res.NextAction, HumanReviewRequired)
	}
	if len(res.ChangedMaterial) != 1 || res.ChangedMaterial[0] != "S001" {
		t.Errorf("changed material = %v, want [S001]", res.ChangedMaterial)
	}
	if got := readFile(t, filepath.Join(dir, ".agent-sdlc", "plan.json")); got != beforePlan {
		t.Error("the read-only check mutated the recorded machine plan")
	}
	if st.tasks[0].Status != domain.LOCAL_DONE {
		t.Errorf("executed task status changed to %s", st.tasks[0].Status)
	}
}

// Scenario 6 (real planflow): a changed source that requires a capability whose
// owner it does not determine is a capability-gap semantic change; the source, the
// machine plan, and the graph are untouched and no prerequisite is added.
func TestCheckRealPlanflowCapabilityGapStops(t *testing.T) {
	dir := t.TempDir()
	st, source := prepareReal(t, dir, "docs/plans/PLAN.md", planDocClean)

	gapDoc := "# Implementation Plan\n\n## Project\n\nWidget\n\n## Summary\n\nAdd a widget.\n\n## Capabilities\n\n### External Tool — MISSING\n\n- Evidence: no external tool found\n\n## S001 — Application skeleton\n\nCreate it.\n\n### Requires\n\n- External Tool\n\n### Acceptance Criteria\n\n- starts\n"
	writeFile(t, dir, "docs/plans/PLAN.md", gapDoc)

	beforePlan := readFile(t, filepath.Join(dir, ".agent-sdlc", "plan.json"))
	beforeTasks := len(st.tasks)
	res := Check(context.Background(), Options{Dir: dir, PlanSource: source, Tasks: st.tasks, Store: st})
	if res.Classification != SemanticChange || !res.CapabilityGap {
		t.Fatalf("classification = %s capability_gap=%v, want %s/gap (%s)", res.Classification, res.CapabilityGap, SemanticChange, res.Reason)
	}
	if len(st.tasks) != beforeTasks {
		t.Errorf("task count = %d, want %d (no prerequisite may be added)", len(st.tasks), beforeTasks)
	}
	if got := readFile(t, filepath.Join(dir, ".agent-sdlc", "plan.json")); got != beforePlan {
		t.Error("the read-only check mutated the recorded machine plan")
	}
}

// Scenario 11 (real planflow): untracked evidence is preserved across the check.
func TestCheckRealPlanflowPreservesUntrackedEvidence(t *testing.T) {
	dir := t.TempDir()
	st, source := prepareReal(t, dir, "docs/plans/PLAN.md", planDocClean)
	writeFile(t, dir, "evidence/report.md", "durable evidence")

	res := Check(context.Background(), Options{Dir: dir, PlanSource: source, Tasks: st.tasks, Store: st})
	if res.NextAction != ContinueSafe {
		t.Fatalf("next action = %s, want %s", res.NextAction, ContinueSafe)
	}
	if got := readFile(t, filepath.Join(dir, "evidence", "report.md")); got != "durable evidence" {
		t.Errorf("untracked evidence changed to %q", got)
	}
}

// Scenario 2 (real planflow): an executed task whose descriptive text alone changed
// is a semantics-preserving change (Changed) that authorizes continuation.
func TestCheckRealPlanflowEquivalentExecutedChangeIsChanged(t *testing.T) {
	dir := t.TempDir()
	st, source := prepareReal(t, dir, "docs/plans/PLAN.md", planDocClean)
	markExecuted(t, st.tasks[0])

	// Only the objective prose changes; acceptance, mode, and dependencies stay.
	reworded := strings.Replace(planDocClean, "Create it.", "Create the project skeleton.", 1)
	writeFile(t, dir, "docs/plans/PLAN.md", reworded)

	res := Check(context.Background(), Options{Dir: dir, PlanSource: source, Tasks: st.tasks, Store: st})
	if res.Classification != Changed {
		t.Fatalf("classification = %s, want %s (%s)", res.Classification, Changed, res.Reason)
	}
	if len(res.ChangedEquivalent) != 1 || res.ChangedEquivalent[0] != "S001" {
		t.Errorf("changed equivalent = %v, want [S001]", res.ChangedEquivalent)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
