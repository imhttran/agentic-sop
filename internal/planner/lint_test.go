package planner

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// lintStageDoc builds a rendered-shape plan with one stage declaring the given
// deliverables and checks.
func lintStageDoc(deliverables, checks []string) *Plan {
	return &Plan{
		Project: "P", Summary: "S",
		Stages: []Stage{{
			ID:                 "S001",
			Title:              "One",
			Objective:          "o",
			Deliverables:       deliverables,
			AcceptanceCriteria: []string{"works"},
			AcceptanceChecks:   checks,
		}},
	}
}

// checkWarnings keeps only the missing-acceptance-checks warnings, so these tests
// stay independent of the acceptance-mapping warning.
func checkWarnings(ws []Warning) []Warning {
	var out []Warning
	for _, w := range ws {
		if strings.Contains(w.Message, "no acceptance checks") {
			out = append(out, w)
		}
	}
	return out
}

// TestLintWarnsDeliverablesWithoutChecks proves the core rule: a stage with
// deliverables and zero checks yields exactly one warning naming the stage, and
// the plan still validates — the warning is never a hard failure.
func TestLintWarnsDeliverablesWithoutChecks(t *testing.T) {
	plan := lintStageDoc([]string{"a binary"}, nil)

	warnings := checkWarnings(plan.Lint())
	if len(warnings) != 1 {
		t.Fatalf("warnings = %d (%v), want 1", len(warnings), warnings)
	}
	if warnings[0].StageID != "S001" {
		t.Errorf("warning stage = %q, want S001", warnings[0].StageID)
	}
	if !strings.Contains(warnings[0].String(), "S001") || !strings.Contains(warnings[0].String(), "acceptance checks") {
		t.Errorf("warning %q must name the stage and the missing checks", warnings[0].String())
	}
	if err := plan.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil (a missing check is a warning, not an error)", err)
	}
}

// TestLintNoWarningWhenChecksOrNoDeliverables proves the warning is specific: a
// stage with checks yields none, and a stage with no deliverables yields none
// even without checks.
func TestLintNoWarningWhenChecksOrNoDeliverables(t *testing.T) {
	withChecks := lintStageDoc([]string{"a binary"}, []string{"go test ./..."})
	if got := checkWarnings(withChecks.Lint()); len(got) != 0 {
		t.Errorf("with checks: warnings = %v, want none", got)
	}

	noDeliverables := lintStageDoc(nil, nil)
	if got := checkWarnings(noDeliverables.Lint()); len(got) != 0 {
		t.Errorf("no deliverables: warnings = %v, want none", got)
	}
}

// TestLintWarnsPerOffendingStage proves the warning set names every offending
// stage, so a migrating plan is flagged stage by stage.
func TestLintWarnsPerOffendingStage(t *testing.T) {
	plan := &Plan{Project: "P", Summary: "S", Stages: []Stage{
		{ID: "S001", Title: "a", Objective: "o", AcceptanceCriteria: []string{"x"}, Deliverables: []string{"d"}},
		{ID: "S002", Title: "b", Objective: "o", AcceptanceCriteria: []string{"x"}, Deliverables: []string{"d"}, AcceptanceChecks: []string{"true"}},
		{ID: "S003", Title: "c", Objective: "o", AcceptanceCriteria: []string{"x"}, Deliverables: []string{"d"}},
	}}

	warnings := checkWarnings(plan.Lint())
	ids := make([]string, len(warnings))
	for i, w := range warnings {
		ids[i] = w.StageID
	}
	if want := []string{"S001", "S003"}; !reflect.DeepEqual(ids, want) {
		t.Errorf("warned stages = %v, want %v", ids, want)
	}
}

// TestLintRAG025FixtureLoadsAndIsFlagged proves the RAG-025-shaped fixture —
// deliverables declared, no checks — stays loadable and valid while every stage
// is flagged. This is the migration path: flagged, never rejected.
func TestLintRAG025FixtureLoadsAndIsFlagged(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join("testdata", "rag025-shaped-plan.md"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	plan, err := PlanFromMarkdown(string(doc))
	if err != nil {
		t.Fatalf("PlanFromMarkdown: %v", err)
	}
	if err := plan.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil (the fixture must stay loadable)", err)
	}

	warnings := checkWarnings(LintWarnings(plan))
	if len(warnings) != len(plan.Stages) {
		t.Fatalf("warnings = %d, want one per stage (%d): %v", len(warnings), len(plan.Stages), warnings)
	}
	for _, w := range warnings {
		if !strings.Contains(w.String(), "RAG025") {
			t.Errorf("warning %q must name the offending RAG-025 stage", w.String())
		}
	}
}

// TestCompileSurfacesWarningsWithoutFailing proves Compile still succeeds for a
// check-less recognized plan, so the warning never becomes an error during
// compilation.
func TestCompileSurfacesWarningsWithoutFailing(t *testing.T) {
	doc := "# Plan\n\n## Project\n\nP\n\n## Summary\n\nS\n\n## S001 - One\n\no\n\n### Deliverables\n\n- d\n\n### Acceptance Criteria\n\n- x\n"
	got, err := New(fakeAgent{content: "SHOULD NOT BE USED"}).Compile(context.Background(), doc)
	if err != nil {
		t.Fatalf("Compile() = %v, want success for a check-less plan", err)
	}
	if len(got.Lint()) == 0 {
		t.Error("compiled check-less plan should be flagged by Lint")
	}
}
