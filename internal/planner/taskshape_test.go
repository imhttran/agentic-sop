package planner

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/domain"
)

// repoPlan is one of the repository's own phase plans: the "## Tasks" shape with
// level-3 stage headings and inline "- **Field:**" lines.
type repoPlan struct {
	path   string
	stages int
}

// repoPlans is the contract for the second plan shape. Each entry lists a plan whose
// stages are named "### <id> — <title>" under "## Tasks"; a plan that moves or changes
// shape updates this list deliberately. See docs/specs/EXECUTION.md §3.
var repoPlans = []repoPlan{
	{"../../docs/plans/PLAN-Phase-3-OpenJEV.md", 17},
	{"../../docs/plans/PLAN-Phase-3.5-Model-Routing.md", 8},
	{"../../docs/plans/PLAN-Phase-4-Provider-Runtime.md", 9},
	{"../../docs/plans/PLAN-Phase-5-Execution-Recovery.md", 10},
}

// TestPlanFromMarkdownCompilesRepoPlans proves the repository's own phase plans
// compile deterministically. Without this, `sop run <plan>.md` needs an agent to
// interpret a document that already states its stages, dependencies, and execution
// modes explicitly.
func TestPlanFromMarkdownCompilesRepoPlans(t *testing.T) {
	for _, want := range repoPlans {
		data, err := os.ReadFile(want.path)
		if err != nil {
			t.Fatalf("read %s: %v", want.path, err)
		}
		plan, err := PlanFromMarkdown(string(data))
		if err != nil {
			t.Errorf("%s: %v", want.path, err)
			continue
		}
		if len(plan.Stages) != want.stages {
			t.Errorf("%s: stages = %d, want %d", want.path, len(plan.Stages), want.stages)
		}
		if err := plan.Validate(); err != nil {
			t.Errorf("%s: %v", want.path, err)
		}
		// Every stage must carry the prose and evidence a task needs.
		for _, stage := range plan.Stages {
			if strings.TrimSpace(stage.Objective) == "" {
				t.Errorf("%s: stage %s has no objective", want.path, stage.ID)
			}
			if len(stage.AcceptanceCriteria) == 0 {
				t.Errorf("%s: stage %s has no acceptance criteria", want.path, stage.ID)
			}
		}
	}
}

// TestPlanFromMarkdownRepoPlansHonourExecutionMode proves the explicit Execution field
// is load-bearing: the already-implemented Phase 3 stages declare verify-first, and
// that instruction is read from the document as written instead of being lost.
func TestPlanFromMarkdownRepoPlansHonourExecutionMode(t *testing.T) {
	data, err := os.ReadFile("../../docs/plans/PLAN-Phase-3-OpenJEV.md")
	if err != nil {
		t.Fatalf("read plan: %v", err)
	}
	plan, err := PlanFromMarkdown(string(data))
	if err != nil {
		t.Fatalf("PlanFromMarkdown: %v", err)
	}
	verifyFirst := 0
	for _, stage := range plan.Stages {
		if stage.ExecutionMode == domain.ExecutionVerifyFirst {
			verifyFirst++
		}
	}
	if verifyFirst == 0 {
		t.Error("the plan marks its already-committed stages verify-first; none were read")
	}
}

// TestPlanFromMarkdownTaskShape covers the "## Tasks" shape field by field: the
// recognized fields become plan fields, wrapped values are joined, and the
// documentation-only fields (Status, Validation) are not consulted.
func TestPlanFromMarkdownTaskShape(t *testing.T) {
	doc := "# PLAN — Sample\n\n## Project\n\nWidget\n\n## Summary\n\nAdd a widget.\n\n## Tasks\n\n" +
		"### P1-001 — Build the skeleton\n\n" +
		"- **Status:** Done.\n" +
		"- **Objective:** Create the skeleton.\n" +
		"- **Scope:** one package, wrapping onto\n  a second indented line.\n" +
		"- **Files:** `cmd/sop/main.go`, `internal/app/app.go`.\n" +
		"- **Depends on:** —\n" +
		"- **Acceptance:** the binary runs.\n" +
		"- **Validation:** `go test ./...`.\n" +
		"- **Execution:** verify-first — the work is already committed.\n\n" +
		"### P1-002 — Add the endpoint\n\n" +
		"- **Status:** Not started.\n" +
		"- **Scope:** Add GET /health.\n" +
		"- **Depends on:** P1-001.\n" +
		"- **Acceptance:** GET /health returns 200.\n"

	plan, err := PlanFromMarkdown(doc)
	if err != nil {
		t.Fatalf("PlanFromMarkdown: %v", err)
	}
	if err := plan.Validate(); err != nil {
		t.Fatalf("compiled plan invalid: %v", err)
	}
	if plan.Project != "Widget" || plan.Summary != "Add a widget." {
		t.Errorf("project/summary = %q/%q", plan.Project, plan.Summary)
	}
	if len(plan.Stages) != 2 {
		t.Fatalf("stages = %d, want 2", len(plan.Stages))
	}

	first := plan.Stages[0]
	if first.ID != "P1-001" || first.Title != "Build the skeleton" {
		t.Errorf("stage[0] = %q/%q", first.ID, first.Title)
	}
	if first.Objective != "Create the skeleton." {
		t.Errorf("objective = %q, want the Objective field", first.Objective)
	}
	if len(first.Dependencies) != 0 {
		t.Errorf("dependencies = %v, want none for an em-dash marker", first.Dependencies)
	}
	if !reflect.DeepEqual(first.Deliverables, []string{"`cmd/sop/main.go`, `internal/app/app.go`."}) {
		t.Errorf("deliverables = %v", first.Deliverables)
	}
	if !reflect.DeepEqual(first.AcceptanceCriteria, []string{"the binary runs."}) {
		t.Errorf("acceptance = %v", first.AcceptanceCriteria)
	}
	if first.ExecutionMode != domain.ExecutionVerifyFirst {
		t.Errorf("execution mode = %q, want %q from the Execution field", first.ExecutionMode, domain.ExecutionVerifyFirst)
	}

	second := plan.Stages[1]
	if !reflect.DeepEqual(second.Dependencies, []string{"P1-001"}) {
		t.Errorf("dependencies = %v, want [P1-001]", second.Dependencies)
	}
	// With no Objective, the Scope describes the work.
	if second.Objective != "Add GET /health." {
		t.Errorf("objective = %q", second.Objective)
	}
}

// TestPlanFromMarkdownScopeBecomesObjective proves a stage with no Objective falls back
// to its Scope, which is how the phase plans that omit Objective describe the work.
func TestPlanFromMarkdownScopeBecomesObjective(t *testing.T) {
	doc := "## Project\n\nP\n\n## Summary\n\nS\n\n## Tasks\n\n### P1-001 — One\n\n- **Scope:** do the thing,\n  and the rest of it.\n- **Acceptance:** it works.\n"
	plan, err := PlanFromMarkdown(doc)
	if err != nil {
		t.Fatalf("PlanFromMarkdown: %v", err)
	}
	if err := plan.Validate(); err != nil {
		t.Fatalf("compiled plan invalid: %v", err)
	}
	if got, want := plan.Stages[0].Objective, "do the thing, and the rest of it."; got != want {
		t.Errorf("objective = %q, want %q (the Scope value, joined across lines)", got, want)
	}
}

// TestPlanFromMarkdownRejectsUnreadableDependency proves an ambiguous dependency is an
// error rather than a guess: a dropped edge would silently reorder the graph.
func TestPlanFromMarkdownRejectsUnreadableDependency(t *testing.T) {
	for _, value := range []string{"P1-001..P1-003", "all", "P1-001 (see the note)", "P1-002 through P1-004"} {
		doc := "## Project\n\nP\n\n## Summary\n\nS\n\n## Tasks\n\n### P1-001 — One\n\n- **Scope:** x\n- **Depends on:** " + value + "\n- **Acceptance:** it works.\n"
		if _, err := PlanFromMarkdown(doc); err == nil {
			t.Errorf("Depends on %q: expected an unreadable-dependency error", value)
		} else if !strings.Contains(err.Error(), "unreadable dependency") {
			t.Errorf("Depends on %q: err = %v", value, err)
		}
	}
}

// TestPlanFromMarkdownIgnoresOtherLevel3Headings proves the shape is read only under a
// "## Tasks" section: a level-3 heading elsewhere is not a stage.
func TestPlanFromMarkdownIgnoresOtherLevel3Headings(t *testing.T) {
	doc := "## Project\n\nP\n\n## Summary\n\nS\n\n## Tasks\n\n### P1-001 — One\n\n- **Scope:** x\n- **Acceptance:** a\n\n## Definition of Done\n\n### P1-002 — Not a stage\n\n- **Scope:** y\n"
	plan, err := PlanFromMarkdown(doc)
	if err != nil {
		t.Fatalf("PlanFromMarkdown: %v", err)
	}
	if len(plan.Stages) != 1 || plan.Stages[0].ID != "P1-001" {
		t.Errorf("stages = %+v, want only P1-001", plan.Stages)
	}
}

// TestPlanFromMarkdownRenderedShapeUnchanged proves the second shape is a fallback:
// a document the canonical rendering parses is still parsed by it.
func TestPlanFromMarkdownRenderedShapeUnchanged(t *testing.T) {
	original := &Plan{
		Project: "P", Summary: "S",
		Stages: []Stage{{
			ID: "S001", Title: "One", Objective: "o",
			AcceptanceCriteria: []string{"a"}, ExecutionMode: domain.ExecutionDone,
		}},
	}
	got, err := PlanFromMarkdown(original.RenderMarkdown())
	if err != nil {
		t.Fatalf("PlanFromMarkdown: %v", err)
	}
	if len(got.Stages) != 1 || got.Stages[0].ExecutionMode != domain.ExecutionDone {
		t.Errorf("round-trip = %+v", got.Stages)
	}
}

// TestPlanFromMarkdownIDTokenKeepsNumericSubSegment proves an id like "P5-001" is not
// truncated to "P5", which would collide with every other stage of the same plan.
func TestPlanFromMarkdownIDTokenKeepsNumericSubSegment(t *testing.T) {
	for in, want := range map[string]string{
		"P5-001 — Recovery vocabulary": "P5-001",
		"P3-002 --- Define early":      "P3-002",
		"PREJEV012-S6 — Sub-stage":     "PREJEV012-S6",
		"AHV2001 --- Correct":          "AHV2001",
		"S001 — Skeleton":              "S001",
		"Tasks":                        "",
	} {
		id, _ := splitStageHeading(in)
		if id != want {
			t.Errorf("splitStageHeading(%q) id = %q, want %q", in, id, want)
		}
	}
}
