package cli

import (
	"os"
	"path/filepath"
	"testing"

	runpkg "github.com/imhttran/agentic-sop/internal/run"
)

// resumePlanFixture writes a project with one active plan document and an empty
// run directory, mirroring a task parked by `sop planner ask` before any run.
func resumePlanFixture(t *testing.T, planID, stageID string) (dir, runDir string) {
	t.Helper()
	dir = t.TempDir()
	stateDir := filepath.Join(dir, ".agent-sdlc")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	doc := "# Implementation Plan\n\n## Project\n\nresume fixture\n\n## Summary\n\nresume fixture\n\n" +
		"## " + stageID + " — Fixture stage\n\nDo the fixture work.\n\n### Dependencies\n\nNone\n\n" +
		"### Acceptance Criteria\n\n- the fixture stage runs.\n"
	source := filepath.Join(dir, "docs", "plans", "PLAN-Fixture.md")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	meta := `{"source":"docs/plans/PLAN-Fixture.md","source_kind":"plan","source_sha256":"abc","plan_id":"` + planID + `"}`
	if err := os.WriteFile(filepath.Join(stateDir, "plan.meta.json"), []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}
	runDir = filepath.Join(stateDir, "runs", stageID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir, runDir
}

// TestLoadPriorPlanFallsBackToTheActivePlan proves a parked run that never reached
// the PLAN stage resumes from the active plan instead of failing on a missing
// per-run plan.md.
func TestLoadPriorPlanFallsBackToTheActivePlan(t *testing.T) {
	dir, runDir := resumePlanFixture(t, "plan-fixture", "FX-1")

	plan, err := loadPriorPlan(dir, runpkg.At(runDir))
	if err != nil {
		t.Fatalf("loadPriorPlan: %v", err)
	}
	if len(plan.Stages) != 1 || plan.Stages[0].ID != "FX-1" {
		t.Fatalf("stages = %+v, want the active plan's FX-1", plan.Stages)
	}
	if plan.Project == "" {
		t.Error("the fallback plan has no project")
	}
}

// TestLoadPriorPlanPrefersTheRunPlan proves the run's own plan still wins when it
// exists, so a resumed run preserves what that run produced.
func TestLoadPriorPlanPrefersTheRunPlan(t *testing.T) {
	dir, runDir := resumePlanFixture(t, "plan-fixture", "FX-1")
	runDoc := "# Implementation Plan\n\n## Project\n\nrun plan\n\n## Summary\n\nrun plan\n\n" +
		"## FX-9 — Run stage\n\nRun stage work.\n\n### Dependencies\n\nNone\n\n" +
		"### Acceptance Criteria\n\n- the run stage runs.\n"
	if err := os.WriteFile(filepath.Join(runDir, "plan.md"), []byte(runDoc), 0o644); err != nil {
		t.Fatal(err)
	}

	plan, err := loadPriorPlan(dir, runpkg.At(runDir))
	if err != nil {
		t.Fatalf("loadPriorPlan: %v", err)
	}
	if len(plan.Stages) != 1 || plan.Stages[0].ID != "FX-9" {
		t.Fatalf("stages = %+v, want the run plan's FX-9", plan.Stages)
	}
}

// TestLoadPriorPlanFailsClosedWithoutAnyPlan proves the resume still fails with a
// clear error when neither the run plan nor an active plan source exists.
func TestLoadPriorPlanFailsClosedWithoutAnyPlan(t *testing.T) {
	dir := t.TempDir()
	runDir := filepath.Join(dir, ".agent-sdlc", "runs", "FX-1")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := loadPriorPlan(dir, runpkg.At(runDir)); err == nil {
		t.Fatal("loadPriorPlan succeeded with no run plan and no active plan")
	}
}
