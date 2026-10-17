package cli

import (
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/planner"
	"github.com/imhttran/agentic-sop/internal/taskfile"
)

// The three descriptive outcomes of the observed NEXT-002 task.
var next002Deliverables = []string{
	"The generated validation lint command is shell-free and executable by the agent command harness, with a regression test that fails on the previous shell-substitution command.",
	"The validation runner treats a gofmt listing as the formatting gate, with a regression test that fails without it.",
	"A recorded note that the continuation-budget question was closed as a confirmed non-defect.",
}

func groupByHeading(groups []taskfile.DeliverableGroup) map[string]taskfile.DeliverableGroup {
	out := map[string]taskfile.DeliverableGroup{}
	for _, g := range groups {
		out[g.Heading] = g
	}
	return out
}

// TestDeliverableGroupsNEXT002: the three descriptive outcomes are presented once, as
// descriptive outcomes -- never as files the agent must create.
func TestDeliverableGroupsNEXT002(t *testing.T) {
	spec := &taskfile.Spec{Deliverables: next002Deliverables}
	groups := deliverableGroups(spec)
	spec.DeliverableGroups = groups
	if len(groups) != 1 {
		t.Fatalf("groups = %d, want 1: %+v", len(groups), groups)
	}
	if groups[0].Heading != "Expected outcomes (descriptive)" || len(groups[0].Items) != 3 {
		t.Fatalf("group = %+v, want one descriptive group of 3", groups[0])
	}
	text := spec.Render()
	if strings.Contains(text, "## Deliverables") {
		t.Errorf("the task text still frames descriptive outcomes as deliverables:\n%s", text)
	}
	if !strings.Contains(text, "do not require a new file") {
		t.Errorf("the task text does not say a file is not required:\n%s", text)
	}
	// The verified gate is unchanged: descriptive items are never report deliverables.
	if got := taskReportDeliverables(spec); len(got) != 0 {
		t.Fatalf("prose must not be gated as a report deliverable: %v", got)
	}
}

// TestDeliverableGroupsReportFile: a genuine docs/reports report file is rendered as a
// required file and is still gated, unchanged.
func TestDeliverableGroupsReportFile(t *testing.T) {
	spec := &taskfile.Spec{Deliverables: []string{"`docs/reports/PERFORMANCE-BASELINE.md`"}}
	spec.DeliverableGroups = deliverableGroups(spec)
	groups := groupByHeading(deliverableGroups(spec))
	g, ok := groups["Required deliverable files"]
	if !ok || len(g.Items) != 1 {
		t.Fatalf("want one required-file group: %+v", deliverableGroups(spec))
	}
	if !strings.Contains(spec.Render(), "Create each of these repository files") {
		t.Errorf("the required file is not presented as required:\n%s", spec.Render())
	}
	if got := taskReportDeliverables(spec); len(got) != 1 || got[0] != "docs/reports/PERFORMANCE-BASELINE.md" {
		t.Fatalf("the verified gate changed: %v", got)
	}
}

// TestDeliverableGroupsUngatedPath: a repository path outside docs/reports is not a
// verified report deliverable. It is presented neutrally, never as a required report
// file, and the gate still declines it.
func TestDeliverableGroupsUngatedPath(t *testing.T) {
	spec := &taskfile.Spec{Deliverables: []string{"`internal/cli/drive.go`"}}
	groups := groupByHeading(deliverableGroups(spec))
	g, ok := groups["Declared repository paths"]
	if !ok || len(g.Items) != 1 {
		t.Fatalf("want one declared-path group: %+v", deliverableGroups(spec))
	}
	if _, labelledRequired := groups["Required deliverable files"]; labelledRequired {
		t.Fatal("an ungated path must not be labelled a required report file")
	}
	if got := taskReportDeliverables(spec); len(got) != 0 {
		t.Fatalf("an ungated path must stay ungated: %v", got)
	}
}

// TestDeliverableGroupsMixed: all three kinds render in a deterministic order.
func TestDeliverableGroupsMixed(t *testing.T) {
	spec := &taskfile.Spec{Deliverables: []string{
		"`docs/reports/X.md`",
		"`internal/cli/drive.go`",
		"A recorded note that the question was closed.",
	}}
	groups := deliverableGroups(spec)
	if len(groups) != 3 {
		t.Fatalf("groups = %d, want 3: %+v", len(groups), groups)
	}
	want := []string{"Required deliverable files", "Declared repository paths", "Expected outcomes (descriptive)"}
	for i, h := range want {
		if groups[i].Heading != h {
			t.Fatalf("group %d = %q, want %q", i, groups[i].Heading, h)
		}
	}
}

// TestDeliverableRenderPlanRoundTripCompat: the plan document keeps its parseable
// "### Deliverables" vocabulary, so compile -> render -> compile is unchanged.
func TestDeliverableRenderPlanRoundTripCompat(t *testing.T) {
	stage := planner.Stage{
		ID: "T-001", Title: "t", Objective: "o",
		Deliverables:       []string{"`docs/reports/X.md`", "A recorded note."},
		AcceptanceCriteria: []string{"c"},
	}
	plan := &planner.Plan{Project: "p", Summary: "s", Stages: []planner.Stage{stage}}
	md := plan.RenderMarkdown()
	if !strings.Contains(md, "### Deliverables") {
		t.Fatalf("the plan document lost its parseable Deliverables section:\n%s", md)
	}
	re, err := planner.PlanFromMarkdown(md)
	if err != nil {
		t.Fatalf("recompile: %v", err)
	}
	if len(re.Stages) != 1 || len(re.Stages[0].Deliverables) != 2 {
		t.Fatalf("round trip changed the deliverables: %+v", re.Stages)
	}
	for i, want := range stage.Deliverables {
		if re.Stages[0].Deliverables[i] != want {
			t.Fatalf("deliverable %d = %q, want %q", i, re.Stages[0].Deliverables[i], want)
		}
	}
}
