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

// Invalid generated plans used by the repair tests.
const (
	selfDepJSON     = `{"project":"p","summary":"s","stages":[{"id":"S1","title":"t","objective":"o","acceptance_criteria":["a"],"dependencies":["S1"]}]}`
	cyclicJSON      = `{"project":"p","summary":"s","stages":[{"id":"S1","title":"t","objective":"o","acceptance_criteria":["a"],"dependencies":["S2"]},{"id":"S2","title":"t","objective":"o","acceptance_criteria":["a"],"dependencies":["S1"]}]}`
	unknownDepJSON  = `{"project":"p","summary":"s","stages":[{"id":"S1","title":"t","objective":"o","acceptance_criteria":["a"],"dependencies":["S9"]}]}`
	duplicateIDJSON = `{"project":"p","summary":"s","stages":[{"id":"S1","title":"t","objective":"o","acceptance_criteria":["a"]},{"id":"S1","title":"t2","objective":"o2","acceptance_criteria":["a"]}]}`
	malformedJSON   = `{"project":`
)

// scriptedAgent returns a canned response per call, in order, and records every
// request so a test can assert how many generation and repair calls were made.
type scriptedAgent struct {
	responses []string
	requests  []agent.Request
}

func (s *scriptedAgent) Generate(_ context.Context, request agent.Request) (agent.Response, error) {
	s.requests = append(s.requests, request)
	if len(s.responses) == 0 {
		return agent.Response{}, errors.New("scriptedAgent: no scripted response left")
	}
	content := s.responses[0]
	s.responses = s.responses[1:]
	return agent.Response{Content: content}, nil
}

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

// TestGenerateRepairsInvalidPlan proves that a generated plan which is not a
// valid execution graph is returned to the agent for correction. Each case
// scripts one invalid response followed by a valid one, and asserts the
// deterministic error was fed back in the repair request.
func TestGenerateRepairsInvalidPlan(t *testing.T) {
	cases := []struct {
		name     string
		rejected string
		wantErr  string
	}{
		{
			"self dependency",
			selfDepJSON,
			"depends on itself",
		},
		{
			"cyclic dependency",
			cyclicJSON,
			"dependency cycle",
		},
		{
			"nonexistent dependency",
			unknownDepJSON,
			"unknown dependency",
		},
		{
			"duplicate stage id",
			duplicateIDJSON,
			"duplicate stage id",
		},
		{
			"malformed json",
			malformedJSON,
			"parse agent response",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stub := &scriptedAgent{responses: []string{tc.rejected, validJSON}}
			plan, err := New(stub).Generate(context.Background(), "a prd")
			if err != nil {
				t.Fatalf("Generate failed to repair an invalid plan: %v", err)
			}
			if len(plan.Stages) != 2 {
				t.Fatalf("repaired plan has %d stages, want the 2 from the valid response", len(plan.Stages))
			}
			if len(stub.requests) != 2 {
				t.Fatalf("agent calls = %d, want 2 (generation + one repair)", len(stub.requests))
			}
			if got := stub.requests[1].Input; !strings.Contains(got, tc.wantErr) {
				t.Errorf("repair request did not carry the validation error %q:\n%s", tc.wantErr, got)
			}
			if got := stub.requests[1].Capability; got != agent.Plan {
				t.Errorf("repair capability = %q, want PLAN", got)
			}
		})
	}
}

// TestGenerateValidPlanMakesNoRepairCall proves a valid first response is used
// as-is: no repair call is made.
func TestGenerateValidPlanMakesNoRepairCall(t *testing.T) {
	stub := &scriptedAgent{responses: []string{validJSON, "SHOULD NOT BE USED"}}
	plan, err := New(stub).Generate(context.Background(), "a prd")
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if plan == nil {
		t.Fatal("nil plan")
	}
	if len(stub.requests) != 1 {
		t.Errorf("agent calls = %d, want 1 (a valid plan needs no repair)", len(stub.requests))
	}
}

// TestGenerateGivesUpAfterBoundedRepairs proves a plan that never becomes valid
// is corrected at most maxPlanRepairs times, then generation fails cleanly with
// the deterministic error.
func TestGenerateGivesUpAfterBoundedRepairs(t *testing.T) {
	stub := &scriptedAgent{responses: []string{selfDepJSON, selfDepJSON, selfDepJSON, selfDepJSON}}
	_, err := New(stub).Generate(context.Background(), "a prd")
	if err == nil {
		t.Fatal("expected a bounded failure for a persistently invalid plan")
	}
	if !strings.Contains(err.Error(), "depends on itself") {
		t.Errorf("error = %q, want the deterministic validation error", err)
	}
	if want := 1 + maxPlanRepairs; len(stub.requests) != want {
		t.Errorf("agent calls = %d, want %d (initial + bounded repairs)", len(stub.requests), want)
	}
}

// TestGenerateNotifiesOnRepair proves the observer fires once per repair with
// the 1-based attempt number and the deterministic cause, so a caller can trace
// and count the recovery.
func TestGenerateNotifiesOnRepair(t *testing.T) {
	var attempts []int
	var causes []string
	stub := &scriptedAgent{responses: []string{selfDepJSON, validJSON}}
	p := New(stub).OnRepair(func(attempt int, cause error) {
		attempts = append(attempts, attempt)
		causes = append(causes, cause.Error())
	})
	if _, err := p.Generate(context.Background(), "a prd"); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if len(attempts) != 1 || attempts[0] != 1 {
		t.Errorf("attempts = %v, want [1]", attempts)
	}
	if len(causes) != 1 || !strings.Contains(causes[0], "depends on itself") {
		t.Errorf("causes = %v, want the deterministic validation error", causes)
	}
}

// TestGenerateValidPlanDoesNotNotify proves a valid plan never fires the
// repair observer.
func TestGenerateValidPlanDoesNotNotify(t *testing.T) {
	called := 0
	stub := &scriptedAgent{responses: []string{validJSON}}
	p := New(stub).OnRepair(func(int, error) { called++ })
	if _, err := p.Generate(context.Background(), "a prd"); err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if called != 0 {
		t.Errorf("observer called %d times for a valid plan, want 0", called)
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
