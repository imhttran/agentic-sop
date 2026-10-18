package planner

import (
	"os"
	"strings"
	"testing"
)

// TestSecurityScannerPlanCompiles proves the security scanner add-on plan
// (converted from the PRD into the canonical rendered shape) compiles
// deterministically and retains every stage's acceptance criteria. It is the
// executable proof behind the plan's own SEC-005 acceptance check.
func TestSecurityScannerPlanCompiles(t *testing.T) {
	const path = "../../docs/plans/PLAN-Security-Scanner-Addon.md"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	plan, err := PlanFromMarkdown(string(data))
	if err != nil {
		t.Fatalf("PlanFromMarkdown: %v", err)
	}
	if err := plan.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if plan.Project != "agentic-sop" {
		t.Errorf("project = %q, want agentic-sop", plan.Project)
	}

	want := map[string]struct {
		dependency string
		criteria   int
	}{
		"SEC-001": {"", 5},
		"SEC-002": {"SEC-001", 6},
		"SEC-003": {"SEC-002", 6},
		"SEC-004": {"SEC-003", 7},
		"SEC-005": {"SEC-004", 5},
	}
	if len(plan.Stages) != len(want) {
		t.Fatalf("stages = %d, want %d", len(plan.Stages), len(want))
	}
	for _, stage := range plan.Stages {
		check, ok := want[stage.ID]
		if !ok {
			t.Errorf("unexpected stage %s", stage.ID)
			continue
		}
		if check.dependency != "" {
			if len(stage.Dependencies) != 1 || stage.Dependencies[0] != check.dependency {
				t.Errorf("%s dependencies = %v, want [%s]", stage.ID, stage.Dependencies, check.dependency)
			}
		} else if len(stage.Dependencies) != 0 {
			t.Errorf("%s dependencies = %v, want none", stage.ID, stage.Dependencies)
		}
		if len(stage.AcceptanceCriteria) != check.criteria {
			t.Errorf("%s acceptance criteria = %d, want %d", stage.ID, len(stage.AcceptanceCriteria), check.criteria)
		}
		if strings.TrimSpace(stage.Objective) == "" {
			t.Errorf("%s has no objective", stage.ID)
		}
		if len(stage.AcceptanceChecks) == 0 {
			t.Errorf("%s has no acceptance checks", stage.ID)
		}
	}
}
