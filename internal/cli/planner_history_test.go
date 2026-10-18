package cli

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/imhttran/agentic-sop/internal/question"
)

// TestWithPlannerDecisionsFormat proves the history block: earlier decisions are listed
// oldest first on single lines, the latest keeps its "Planner decision:" line last, and
// no history leaves the single-decision input byte-identical.
func TestWithPlannerDecisionsFormat(t *testing.T) {
	if got, want := withPlannerDecisions("base", nil, "yes"), withPlannerAnswer("base", "yes"); got != want {
		t.Fatalf("no history: got %q, want %q", got, want)
	}
	earlier := []plannerDecision{
		{Question: "which\nformat?", Answer: "Markdown"},
		{Question: "keep duplicates?", Answer: "remove  them"},
	}
	want := "base\n" +
		"Earlier planner decisions in this run (oldest first):\n" +
		"- Q: which format? A: Markdown\n" +
		"- Q: keep duplicates? A: remove them\n" +
		"Planner decision: yes"
	if got := withPlannerDecisions("base", earlier, "yes"); got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
	if got := withPlannerDecisions("", earlier[:1], "yes"); !strings.HasPrefix(got, "Earlier planner decisions") {
		t.Fatalf("blank base must not start with a newline: %q", got)
	}
	if got := withPlannerDecisions("base", earlier, " "); got != "base" {
		t.Fatalf("no latest answer must leave the input unchanged: %q", got)
	}
}

// TestEarlierPlannerDecisionsScope proves only APPLIED questions for the same task, run
// epoch, and plan revision that precede the current question are carried forward,
// oldest first.
func TestEarlierPlannerDecisionsScope(t *testing.T) {
	ctx := context.Background()
	svc := question.NewService(question.NewMemoryStore(), nil)
	base := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

	mk := func(kind, body, identity, epoch, plan string, at time.Time, apply bool) *question.Question {
		t.Helper()
		q, err := svc.CreateQuestion(&question.Question{
			Identity: identity, Kind: kind, Body: body, Revision: 1,
			RunEpoch: epoch, PlanRevision: plan, ReturnStage: "IMPLEMENTING",
		})
		if err != nil {
			t.Fatalf("create %s: %v", kind, err)
		}
		if apply {
			if _, err := svc.CreateAnswer(&question.Answer{QuestionID: q.ID, Body: body + " answer"}); err != nil {
				t.Fatalf("answer %s: %v", kind, err)
			}
			if _, err := svc.ApplyAnswer(ctx, q.ID); err != nil {
				t.Fatalf("apply %s: %v", kind, err)
			}
			if q, _, err = svc.ReadQuestion(q.ID); err != nil {
				t.Fatalf("read %s: %v", kind, err)
			}
		}
		// The store stamps creation with the wall clock; pin it so ordering is explicit.
		q.CreatedAt = at
		return q
	}

	second := mk("r2", "second", "T001", "e1", "p1", base.Add(2*time.Minute), true)
	first := mk("r1", "first", "T001", "e1", "p1", base.Add(time.Minute), true)
	current := mk("r4", "current", "T001", "e1", "p1", base.Add(4*time.Minute), true)
	qs := []*question.Question{
		second,
		first,
		current,
		mk("r3", "unapplied", "T001", "e1", "p1", base.Add(3*time.Minute), false),
		mk("old-epoch", "old epoch", "T001", "e0", "p1", base, true),
		mk("old-plan", "old plan", "T001", "e1", "p0", base, true),
		mk("other-task", "other task", "T002", "e1", "p1", base, true),
		mk("later", "later", "T001", "e1", "p1", base.Add(5*time.Minute), true),
	}

	got := earlierPlannerDecisions(svc, qs, current)
	want := []plannerDecision{
		{Question: "first", Answer: "first answer"},
		{Question: "second", Answer: "second answer"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("decision %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}
