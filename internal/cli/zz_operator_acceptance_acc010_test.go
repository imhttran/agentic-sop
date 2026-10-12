//go:build acceptance

// Operator-authored acceptance check for ACC-010: a multi-file Files value in the
// "## Tasks" plan shape becomes one deliverable per file, and each is verified
// individually by the outcome.

package cli

import (
	"reflect"
	"strings"
	"testing"

	"github.com/imhttran/agentic-sop/internal/outcome"
	"github.com/imhttran/agentic-sop/internal/planner"
	"github.com/imhttran/agentic-sop/internal/taskfile"
)

const accOp010Plan = `# PLAN — accop

## Project
accop

## Summary
Deliverable parsing.

## Tasks

### T-001 — Multi-file stage
- **Objective:** Write two files.
- **Files:** ` + "`a/x.go`, `a/y.go`" + `
- **Files:** ` + "`b/z.go`" + `; ` + "`b/w.go`" + `
- **Depends on:** —
- **Acceptance:** both files exist.
- **Acceptance check:** go vet ./...
`

func accOp010Stage(t *testing.T) planner.Stage {
	t.Helper()
	plan, err := planner.PlanFromMarkdown(accOp010Plan)
	if err != nil {
		t.Fatalf("compile plan: %v", err)
	}
	if plan == nil || len(plan.Stages) != 1 {
		t.Fatalf("plan = %+v, want one stage", plan)
	}
	return plan.Stages[0]
}

func TestOperatorAcceptanceACC010MultiFileDeliverablesParsedIndividually(t *testing.T) {
	got := accOp010Stage(t).Deliverables
	want := []string{"a/x.go", "a/y.go", "b/z.go", "b/w.go"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Deliverables = %q, want %q", got, want)
	}
	for _, d := range got {
		if strings.ContainsAny(d, "`,;") {
			t.Fatalf("deliverable %q still carries list punctuation", d)
		}
	}
}

func TestOperatorAcceptanceACC010EachDeliverableVerifiedIndividually(t *testing.T) {
	stage := accOp010Stage(t)
	dir := accOpModule(t, map[string]string{
		"a/x.go": "package a\n\n// X is a value.\nconst X = 1\n",
		"a/y.go": "package a\n\n// Y is a value.\nconst Y = 2\n",
		"b/z.go": "package b\n\n// Z is a value.\nconst Z = 3\n",
		"b/w.go": "package b\n\n// W is a value.\nconst W = 4\n",
	})
	spec := &taskfile.Spec{ID: stage.ID, Title: stage.Title, Deliverables: stage.Deliverables,
		AcceptanceCriteria: stage.AcceptanceCriteria, AcceptanceChecks: stage.AcceptanceChecks}
	res := deriveOutcome(spec, dir, accOpPassGate())
	if res == nil || res.Status != outcome.Verified {
		t.Fatalf("outcome = %+v, want VERIFIED with every deliverable present", res)
	}
	met := 0
	for _, c := range res.Criteria {
		if strings.HasPrefix(c.ID, "deliverable") {
			if !c.Met {
				t.Fatalf("deliverable criterion %+v not met", c)
			}
			met++
		}
	}
	if met != 4 {
		t.Fatalf("deliverable criteria = %d, want 4 (one per file)", met)
	}

	// Negative: removing one file makes exactly that deliverable unmet.
	missing := accOpModule(t, map[string]string{
		"a/x.go": "package a\n\n// X is a value.\nconst X = 1\n",
		"b/z.go": "package b\n\n// Z is a value.\nconst Z = 3\n",
		"b/w.go": "package b\n\n// W is a value.\nconst W = 4\n",
	})
	res = deriveOutcome(spec, missing, accOpPassGate())
	if res == nil || res.Status == outcome.Verified {
		t.Fatalf("outcome with a/y.go missing = %+v, want not VERIFIED", res)
	}
	unmet := 0
	for _, c := range res.Criteria {
		if strings.HasPrefix(c.ID, "deliverable") && !c.Met {
			unmet++
			if !strings.Contains(c.Description+c.Evidence, "a/y.go") {
				t.Fatalf("unmet deliverable %+v does not name a/y.go", c)
			}
		}
	}
	if unmet != 1 {
		t.Fatalf("unmet deliverable criteria = %d, want exactly 1", unmet)
	}
}
