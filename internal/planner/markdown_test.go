package planner

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/domain"
)

// fakeAgent returns fixed content for any request.
type fakeAgent struct{ content string }

func (f fakeAgent) Generate(context.Context, agent.Request) (agent.Response, error) {
	return agent.Response{Content: f.content}, nil
}

func TestPlanFromMarkdownRoundTrip(t *testing.T) {
	original := &Plan{
		Project: "Widget Service",
		Summary: "Add a widget endpoint.",
		Stages: []Stage{
			{
				ID: "S001", Title: "Application skeleton", Objective: "Create the skeleton.",
				Dependencies: []string{}, Deliverables: []string{"Go application"},
				AcceptanceCriteria: []string{"Application starts"},
			},
			{
				ID: "S002", Title: "Health endpoint", Objective: "Add /health.",
				Dependencies: []string{"S001"}, Deliverables: []string{"GET /health"},
				AcceptanceCriteria: []string{"GET /health returns 200"},
				ExecutionMode:      domain.ExecutionVerifyFirst,
			},
		},
	}

	if !strings.Contains(original.RenderMarkdown(), "### Execution") {
		t.Errorf("rendered plan should carry the execution section:\n%s", original.RenderMarkdown())
	}

	got, err := PlanFromMarkdown(original.RenderMarkdown())
	if err != nil {
		t.Fatalf("PlanFromMarkdown failed: %v", err)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("round-tripped plan invalid: %v", err)
	}
	if got.Project != original.Project || got.Summary != original.Summary {
		t.Errorf("project/summary = %q/%q", got.Project, got.Summary)
	}
	if len(got.Stages) != 2 {
		t.Fatalf("stages = %d, want 2", len(got.Stages))
	}
	if got.Stages[1].ID != "S002" || got.Stages[1].Title != "Health endpoint" {
		t.Errorf("stage[1] = %+v", got.Stages[1])
	}
	if !reflect.DeepEqual(got.Stages[1].Dependencies, []string{"S001"}) {
		t.Errorf("dependencies = %v", got.Stages[1].Dependencies)
	}
	if !reflect.DeepEqual(got.Stages[0].AcceptanceCriteria, []string{"Application starts"}) {
		t.Errorf("criteria = %v", got.Stages[0].AcceptanceCriteria)
	}
	if got.Stages[1].ExecutionMode != domain.ExecutionVerifyFirst {
		t.Errorf("execution mode = %q, want %q", got.Stages[1].ExecutionMode, domain.ExecutionVerifyFirst)
	}
	if got.Stages[0].ExecutionMode != "" {
		t.Errorf("a stage without the setting must stay implement mode, got %q", got.Stages[0].ExecutionMode)
	}
}

func TestPlanFromMarkdownVariants(t *testing.T) {
	doc := "# Implementation Plan\n\n## Project\n\nP\n\n## Summary\n\nS\n\n## T001 - Add widget\n\nDo it.\n\n### Dependencies\n\n- [ ] T000\n\n### Scope\n\n* widget\n\n### Acceptance Criteria\n\n1. must compile\n\n### Rules\n\n- ignored\n"
	plan, err := PlanFromMarkdown(doc)
	if err != nil {
		t.Fatalf("PlanFromMarkdown failed: %v", err)
	}
	if len(plan.Stages) != 1 {
		t.Fatalf("stages = %d, want 1", len(plan.Stages))
	}
	st := plan.Stages[0]
	if st.ID != "T001" || st.Title != "Add widget" {
		t.Errorf("stage = %+v", st)
	}
	if st.Objective != "Do it." {
		t.Errorf("objective = %q", st.Objective)
	}
	if !reflect.DeepEqual(st.Dependencies, []string{"T000"}) {
		t.Errorf("dependencies = %v (checkbox not stripped?)", st.Dependencies)
	}
	if !reflect.DeepEqual(st.Deliverables, []string{"widget"}) {
		t.Errorf("deliverables = %v", st.Deliverables)
	}
	if !reflect.DeepEqual(st.AcceptanceCriteria, []string{"must compile"}) {
		t.Errorf("criteria = %v", st.AcceptanceCriteria)
	}
}

func TestPlanFromMarkdownExecutionMode(t *testing.T) {
	doc := "# Implementation Plan\n\n## Project\n\nP\n\n## Summary\n\nS\n\n## S001 — Verify boundary\n\nCheck it.\n\n### Dependencies\n\nNone\n\n### Acceptance Criteria\n\n- boundary holds\n\n### Execution\n\n- verify first\n"
	plan, err := PlanFromMarkdown(doc)
	if err != nil {
		t.Fatalf("PlanFromMarkdown failed: %v", err)
	}
	if len(plan.Stages) != 1 {
		t.Fatalf("stages = %d, want 1", len(plan.Stages))
	}
	if plan.Stages[0].ExecutionMode != domain.ExecutionVerifyFirst {
		t.Errorf("execution mode = %q, want %q (normalized)", plan.Stages[0].ExecutionMode, domain.ExecutionVerifyFirst)
	}
}

func TestCompileFallsBackToAgent(t *testing.T) {
	// A document with no usable stages cannot be compiled deterministically.
	const planJSON = `{"project":"P","summary":"S","stages":[{"id":"S001","title":"One","objective":"o","dependencies":[],"deliverables":["d"],"acceptance_criteria":["a"]}]}`

	got, err := New(fakeAgent{content: planJSON}).Compile(context.Background(), "not a plan")
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	if len(got.Stages) != 1 || got.Stages[0].ID != "S001" {
		t.Errorf("compiled plan = %+v", got)
	}
}

func TestCompilePrefersDeterministic(t *testing.T) {
	doc := "## Project\n\nP\n\n## Summary\n\nS\n\n## S001 — One\n\no\n\n### Acceptance Criteria\n\n- a\n"
	got, err := New(fakeAgent{content: "SHOULD NOT BE USED"}).Compile(context.Background(), doc)
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	if got.Project != "P" || len(got.Stages) != 1 {
		t.Errorf("compiled plan = %+v", got)
	}
}

func TestCompileRequiresAgentForFallback(t *testing.T) {
	if _, err := New(nil).Compile(context.Background(), "not a plan"); err == nil {
		t.Error("expected an error when there is no agent to normalize")
	}
}

func TestValidateRejectsUnknownExecutionMode(t *testing.T) {
	plan := &Plan{
		Project: "P", Summary: "S",
		Stages: []Stage{{
			ID: "S001", Title: "One", Objective: "o",
			AcceptanceCriteria: []string{"a"}, ExecutionMode: "verify-soon",
		}},
	}
	err := plan.Validate()
	if err == nil || !strings.Contains(err.Error(), "execution_mode") {
		t.Errorf("err = %v, want an unknown execution_mode error", err)
	}
}

func TestValidateRejectsCycle(t *testing.T) {
	plan := &Plan{Project: "P", Summary: "S", Stages: []Stage{
		{ID: "S001", Title: "a", Objective: "o", AcceptanceCriteria: []string{"x"}, Dependencies: []string{"S002"}},
		{ID: "S002", Title: "b", Objective: "o", AcceptanceCriteria: []string{"x"}, Dependencies: []string{"S001"}},
	}}
	err := plan.Validate()
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("Validate() = %v, want a cycle error", err)
	}
}

func TestCompileSurfacesInvalidRecognizedPlan(t *testing.T) {
	// A cyclic document is recognizably a plan, so its structure is authoritative:
	// the compiler reports the problem instead of asking the agent to replace it.
	doc := "# Plan\n\n## Project\n\nP\n\n## Summary\n\nS\n\n## S001 — a\n\no\n\n### Dependencies\n\n- S002\n\n### Acceptance Criteria\n\n- x\n\n## S002 — b\n\no\n\n### Dependencies\n\n- S001\n\n### Acceptance Criteria\n\n- x\n"
	_, err := New(fakeAgent{content: "SHOULD NOT BE USED"}).Compile(context.Background(), doc)
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("Compile() = %v, want a cycle error (no silent agent fallback)", err)
	}
}
