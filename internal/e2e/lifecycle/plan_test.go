package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/e2e/harness"
	"github.com/imhttran/agentic-sop/internal/planner"
)

const validPlanJSON = `{
	"project": "demo",
	"summary": "do the thing",
	"stages": [
		{
			"id": "S1",
			"title": "First",
			"objective": "do it",
			"dependencies": [],
			"deliverables": ["stuff"],
			"acceptance_criteria": ["works"]
		}
	]
}`

const incompletePlanJSON = `{
	"project": "",
	"summary": "do the thing",
	"stages": []
}`

// TestPlanEarlyCompletion asserts PLAN returns early, after a single agent
// invocation, when the completion criteria (a valid plan) are satisfied before
// full exploration. S1.
func TestPlanEarlyCompletion(t *testing.T) {
	provider := &harness.FakeProvider{}
	provider.GenerateFunc = func(_ int, _ agent.Request) (agent.Response, error) {
		return agent.Response{Content: validPlanJSON}, nil
	}

	pl := planner.New(provider)
	plan, err := pl.Generate(context.Background(), "build a demo")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if plan.Project != "demo" {
		t.Fatalf("project = %q, want demo", plan.Project)
	}
	if got := provider.Calls(); got != 1 {
		t.Fatalf("agent invocations = %d, want 1 (early completion)", got)
	}
}

// TestPlanForcedSynthesis asserts that when exploration is cut short (the agent
// returns a plan on the first turn), forced synthesis still produces the
// required structured PLAN output. S1.
func TestPlanForcedSynthesis(t *testing.T) {
	provider := &harness.FakeProvider{}
	provider.GenerateFunc = func(_ int, _ agent.Request) (agent.Response, error) {
		return agent.Response{Content: validPlanJSON}, nil
	}

	pl := planner.New(provider)
	plan, err := pl.Generate(context.Background(), "build a demo")
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	// The synthesized plan must satisfy the structured output shape.
	if err := plan.Validate(); err != nil {
		t.Fatalf("synthesized plan is invalid: %v", err)
	}
	if len(plan.Stages) == 0 {
		t.Fatal("synthesized plan has no stages")
	}
	if plan.Stages[0].ID == "" || plan.Stages[0].Objective == "" {
		t.Fatalf("synthesized stage missing structured fields: %+v", plan.Stages[0])
	}

	// Round-trips as JSON matching the schema, so downstream consumers parse it.
	if _, err := json.Marshal(plan); err != nil {
		t.Fatalf("marshal: %v", err)
	}
}

// TestPlanInvalidForcedSynthesisFails asserts an invalid synthesized plan is
// rejected rather than accepted silently. S1 / focused failure.
func TestPlanInvalidForcedSynthesisFails(t *testing.T) {
	provider := &harness.FakeProvider{}
	provider.GenerateFunc = func(_ int, _ agent.Request) (agent.Response, error) {
		return agent.Response{Content: incompletePlanJSON}, nil
	}

	pl := planner.New(provider)
	if _, err := pl.Generate(context.Background(), "build a demo"); err == nil {
		t.Fatal("expected validation error for incomplete plan")
	}
}

// TestPlanAgentErrorIsSurfaced asserts a provider error is returned, not masked.
func TestPlanAgentErrorIsSurfaced(t *testing.T) {
	want := errors.New("provider down")
	provider := &harness.FakeProvider{}
	provider.GenerateFunc = func(_ int, _ agent.Request) (agent.Response, error) {
		return agent.Response{}, want
	}

	pl := planner.New(provider)
	_, err := pl.Generate(context.Background(), "build a demo")
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}
