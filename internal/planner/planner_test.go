package planner

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
)

// stubAgent is a deterministic Agent for tests: it records the request and
// returns canned content.
type stubAgent struct {
	content string
	err     error
	request agent.Request
}

func (s *stubAgent) Generate(_ context.Context, request agent.Request) (agent.Response, error) {
	s.request = request
	if s.err != nil {
		return agent.Response{}, s.err
	}
	return agent.Response{Content: s.content}, nil
}

const validJSON = `{
  "project": "Book RAG",
  "summary": "Answer questions over books.",
  "stages": [
    {
      "id": "S001",
      "title": "Application skeleton",
      "objective": "Create the Go application skeleton.",
      "dependencies": [],
      "deliverables": ["Go application"],
      "acceptance_criteria": ["Application starts successfully"]
    },
    {
      "id": "S002",
      "title": "Ingestion",
      "objective": "Ingest books into the index.",
      "dependencies": ["S001"],
      "deliverables": ["Ingester"],
      "acceptance_criteria": ["A book can be ingested"]
    }
  ]
}`

func TestGenerateHappyPath(t *testing.T) {
	stub := &stubAgent{content: validJSON}
	plan, err := New(stub).Generate(context.Background(), "A PRD about books.")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	if plan.Project != "Book RAG" {
		t.Errorf("Project = %q, want Book RAG", plan.Project)
	}
	if plan.Summary == "" {
		t.Errorf("Summary is empty")
	}
	if len(plan.Stages) != 2 {
		t.Fatalf("got %d stages, want 2", len(plan.Stages))
	}
	if plan.Stages[1].ID != "S002" || len(plan.Stages[1].Dependencies) != 1 || plan.Stages[1].Dependencies[0] != "S001" {
		t.Errorf("stage S002 dependencies = %v, want [S001]", plan.Stages[1].Dependencies)
	}
	if len(plan.Stages[0].AcceptanceCriteria) != 1 {
		t.Errorf("stage S001 acceptance criteria missing")
	}

	// The request must carry the task, the PRD input, and the output contract.
	if stub.request.Input != "A PRD about books." {
		t.Errorf("request input = %q, want the PRD", stub.request.Input)
	}
	if stub.request.Task == "" || stub.request.OutputRequirements == "" {
		t.Errorf("request missing task/output requirements: %+v", stub.request)
	}
	if stub.request.Capability != agent.Plan {
		t.Errorf("capability = %q, want PLAN", stub.request.Capability)
	}
}

func TestGenerateRejectsInvalidOutput(t *testing.T) {
	cases := []struct {
		name    string
		content string
		wantErr string
	}{
		{"malformed json", `{"project":`, "parse agent response"},
		{"missing project", `{"summary":"s","stages":[{"id":"S1","title":"t","objective":"o","acceptance_criteria":["a"]}]}`, "project is empty"},
		{"missing summary", `{"project":"p","stages":[{"id":"S1","title":"t","objective":"o","acceptance_criteria":["a"]}]}`, "summary is empty"},
		{"no stages", `{"project":"p","summary":"s","stages":[]}`, "no stages"},
		{"duplicate ids", `{"project":"p","summary":"s","stages":[{"id":"S1","title":"t","objective":"o","acceptance_criteria":["a"]},{"id":"S1","title":"t2","objective":"o2","acceptance_criteria":["a"]}]}`, "duplicate stage id"},
		{"missing objective", `{"project":"p","summary":"s","stages":[{"id":"S1","title":"t","acceptance_criteria":["a"]}]}`, "empty objective"},
		{"missing acceptance criteria", `{"project":"p","summary":"s","stages":[{"id":"S1","title":"t","objective":"o"}]}`, "no acceptance criteria"},
		{"unknown dependency", `{"project":"p","summary":"s","stages":[{"id":"S1","title":"t","objective":"o","acceptance_criteria":["a"],"dependencies":["S9"]}]}`, "unknown dependency"},
		{"self dependency", `{"project":"p","summary":"s","stages":[{"id":"S1","title":"t","objective":"o","acceptance_criteria":["a"],"dependencies":["S1"]}]}`, "depends on itself"},
		{"padded id", `{"project":"p","summary":"s","stages":[{"id":" S1 ","title":"t","objective":"o","acceptance_criteria":["a"]}]}`, "surrounding whitespace"},
		{"unknown kind", `{"project":"p","summary":"s","stages":[{"id":"S1","title":"t","objective":"o","acceptance_criteria":["a"],"kind":"mystery"}]}`, "unknown kind"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(&stubAgent{content: tc.content}).Generate(context.Background(), "a prd")
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want containing %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestGenerateRejectsEmptyPRD(t *testing.T) {
	stub := &stubAgent{content: validJSON}
	if _, err := New(stub).Generate(context.Background(), "   \n"); err == nil {
		t.Fatal("expected error for empty PRD")
	}
	if stub.request.Task != "" {
		t.Errorf("agent should not be called for an empty PRD")
	}
}

func TestGeneratePropagatesAgentError(t *testing.T) {
	stub := &stubAgent{err: errors.New("model unavailable")}
	if _, err := New(stub).Generate(context.Background(), "a prd"); err == nil {
		t.Fatal("expected error to propagate")
	}
}

func TestRenderMarkdown(t *testing.T) {
	plan := &Plan{
		Project: "Book RAG",
		Summary: "Build it.",
		Stages: []Stage{{
			ID:                 "S001",
			Title:              "Skeleton",
			Objective:          "Create skeleton.",
			Dependencies:       nil,
			Deliverables:       []string{"Go app"},
			AcceptanceCriteria: []string{"Starts"},
		}},
	}

	want := "# Implementation Plan\n\n" +
		"## Project\n\nBook RAG\n\n" +
		"## Summary\n\nBuild it.\n\n" +
		"## S001 — Skeleton\n\nCreate skeleton.\n\n" +
		"### Dependencies\n\nNone\n\n" +
		"### Deliverables\n\n- Go app\n\n" +
		"### Acceptance Criteria\n\n- Starts\n\n"

	if got := plan.RenderMarkdown(); got != want {
		t.Errorf("RenderMarkdown mismatch:\n got %q\nwant %q", got, want)
	}
}
