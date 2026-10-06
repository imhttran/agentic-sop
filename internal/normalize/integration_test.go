package normalize_test

import (
	"context"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/agent"
	"github.com/imhttran/agentic-sop/internal/normalize"
	"github.com/imhttran/agentic-sop/internal/planner"
	"github.com/imhttran/agentic-sop/internal/review"
)

// stubAgent returns a fixed content for every capability request.
type stubAgent struct{ content string }

func (s stubAgent) Generate(context.Context, agent.Request) (agent.Response, error) {
	return agent.Response{Content: s.content}, nil
}

// fence wraps body in a backtick code fence with an optional language token.
func fence(lang, body string) string {
	f := strings.Repeat(string(rune(96)), 3)
	nl := string(rune(10))
	return f + lang + nl + body + nl + f
}

// TestParseOutcomeNormalizesFence proves the harness accepts a fenced structured outcome.
func TestParseOutcomeNormalizesFence(t *testing.T) {
	got := agent.ParseOutcome(fence("json", `{"status":"completed","summary":"done"}`))
	if got == nil || got.Status != agent.OutcomeCompleted || got.Summary != "done" {
		t.Errorf("fenced outcome = %+v, want completed/done", got)
	}
}

// TestPlannerNormalizesFence proves the planner parses a fenced plan.
func TestPlannerNormalizesFence(t *testing.T) {
	plan, err := planner.New(stubAgent{content: fence("json", `{"project":"p","summary":"s","stages":[{"id":"S1","title":"t","objective":"o","acceptance_criteria":["a"]}]}`)}).Generate(context.Background(), "a prd")
	if err != nil {
		t.Fatalf("fenced plan must normalize: %v", err)
	}
	if plan.Project != "p" || len(plan.Stages) != 1 {
		t.Errorf("plan = %+v", plan)
	}
}

// TestReviewNormalizesFence proves the review parser accepts a fenced report.
func TestReviewNormalizesFence(t *testing.T) {
	rep, err := review.NewAgentProvider(stubAgent{content: fence("json", `{"summary":"ok","findings":[]}`)}).Review(context.Background(), review.Request{Task: "T1"})
	if err != nil {
		t.Fatalf("fenced review must normalize: %v", err)
	}
	if rep.Summary != "ok" {
		t.Errorf("summary = %q", rep.Summary)
	}
}

// TestNormalizerNeverRepairs proves malformed structured output still fails closed.
func TestNormalizerNeverRepairs(t *testing.T) {
	if got := agent.ParseOutcome(fence("json", `{"status":`)); got != nil {
		t.Errorf("malformed fenced outcome must fail closed, got %+v", got)
	}
	if _, err := normalize.JSON(fence("json", `{"status":`)); err == nil {
		t.Error("normalize.JSON must reject malformed fenced content")
	}
}
