package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/imhttran/agentic-sop/internal/planner"
)

// planJSONPath is the machine plan path under dir.
func planJSONPath(dir string) string {
	return filepath.Join(dir, stateDirName, "plan.json")
}

const clefLikePlanJSON = `{
  "project": "Clef Provider Integration",
  "summary": "adapter-side provider",
  "capabilities": [
    {"name": "Go build/test toolchain", "status": "EXISTS", "evidence": "go1.27.1"},
    {"name": "agentic-sop provider-neutral decision seam", "status": "EXISTS"},
    {"name": "Apple Silicon MLX runtime / oMLX with clef-4bit", "status": "UNKNOWN"}
  ],
  "stages": [
    {"id": "CLEF-001", "title": "Capture Adapter Repository Truth", "objective": "Inspect and record the repository.", "requires": ["Go build/test toolchain"], "acceptance_criteria": ["recorded"]},
    {"id": "CLEF-002", "title": "Verify capability", "objective": "o", "dependencies": ["CLEF-001"], "requires": ["Go build/test toolchain", "agentic-sop provider-neutral decision seam"], "acceptance_criteria": ["a"]}
  ]
}`

// TestLoadPlanContextReadsAuthoritativeInventory proves the active machine plan's
// capability inventory and the task's own compiled requires are delivered to the
// task-level planner, so a known capability is not re-derived from task prose.
func TestLoadPlanContextReadsAuthoritativeInventory(t *testing.T) {
	dir := t.TempDir()
	writePlanJSON(t, dir, clefLikePlanJSON)

	got := loadPlanContext(dir, "CLEF-001")
	if len(got.Capabilities) != 3 {
		t.Fatalf("capabilities = %+v, want the compiled inventory", got.Capabilities)
	}
	if got.Capabilities[0].Name != "Go build/test toolchain" || got.Capabilities[0].Status != planner.CapabilityExists {
		t.Errorf("capability[0] = %+v, want the authoritative EXISTS entry", got.Capabilities[0])
	}
	if !equalStrings(got.Requires, []string{"Go build/test toolchain"}) {
		t.Errorf("requires = %v, want the task's own compiled requires", got.Requires)
	}
}

// TestLoadPlanContextDoesNotMutateRepoState proves the loader is read-only: it
// returns the plan context without modifying plan.json, and it never creates or
// touches state.db (SOP-PLANNER-CAP-001 must not mutate persisted SOP state).
func TestLoadPlanContextDoesNotMutateRepoState(t *testing.T) {
	dir := t.TempDir()
	path := planJSONPath(dir)
	writePlanJSON(t, dir, clefLikePlanJSON)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read plan.json: %v", err)
	}

	_ = loadPlanContext(dir, "CLEF-002")

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("re-read plan.json: %v", err)
	}
	if string(before) != string(after) {
		t.Errorf("plan.json changed: loadPlanContext must be read-only")
	}
	if _, err := os.Stat(filepath.Join(dir, stateDirName, "state.db")); !os.IsNotExist(err) {
		t.Errorf("state.db exists or stat failed unexpectedly (%v); the loader must not touch persisted state", err)
	}
}

// TestLoadPlanContextMissingPlanIsEmpty proves an ad-hoc run with no machine plan
// (for example `sop run --task`) yields an empty context, so the task-level planner
// behaves exactly as before the fix.
func TestLoadPlanContextMissingPlanIsEmpty(t *testing.T) {
	dir := t.TempDir()
	got := loadPlanContext(dir, "CLEF-001")
	if !got.IsZero() {
		t.Errorf("context = %+v, want an empty context when no machine plan exists", got)
	}
}

// TestLoadPlanContextUnrelatedTaskHasNoRequires proves a task that is not a stage of
// the active plan still receives the plan's authoritative inventory but no
// task-specific requires, so the inventory is authoritative without inventing a
// prerequisite set for an unrelated task.
func TestLoadPlanContextUnrelatedTaskHasNoRequires(t *testing.T) {
	dir := t.TempDir()
	writePlanJSON(t, dir, clefLikePlanJSON)

	got := loadPlanContext(dir, "NOT-A-STAGE")
	if len(got.Capabilities) != 3 {
		t.Errorf("capabilities = %d, want the plan inventory", len(got.Capabilities))
	}
	if len(got.Requires) != 0 {
		t.Errorf("requires = %v, want none for a task that is not a plan stage", got.Requires)
	}
}

// TestLoadPlanContextIgnoresUnreadablePlan proves a corrupt machine plan degrades to
// an empty context (never an error), so planning still works and no partial or
// fabricated inventory is delivered.
func TestLoadPlanContextIgnoresUnreadablePlan(t *testing.T) {
	dir := t.TempDir()
	writePlanJSON(t, dir, "{not json")

	if got := loadPlanContext(dir, "CLEF-001"); !got.IsZero() {
		t.Errorf("context = %+v, want empty for an unreadable plan", got)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
