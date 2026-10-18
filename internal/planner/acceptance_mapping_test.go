package planner

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

const mappedPlanDoc = "# Implementation Plan\n\n## Project\n\nP\n\n## Summary\n\nS\n\n" +
	"## S001 — Stage\n\nDo it.\n\n### Dependencies\n\nNone\n\n### Deliverables\n\n- a.go\n\n" +
	"### Acceptance Criteria\n\n- it builds\n- docs are clear\n\n" +
	"### Acceptance Checks\n\n- go build ./...\n\n" +
	"### Acceptance Mapping\n\n- criterion 1: check 1\n- criterion 2: manual: reviewer reads docs/x.md\n"

func TestAcceptanceMappingRoundTrips(t *testing.T) {
	plan, err := PlanFromMarkdown(mappedPlanDoc)
	if err != nil {
		t.Fatalf("PlanFromMarkdown: %v", err)
	}
	if err := plan.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	want := []AcceptanceMapping{{Criterion: 1, Check: 1}, {Criterion: 2, Manual: "reviewer reads docs/x.md"}}
	if got := plan.Stages[0].AcceptanceMapping; !reflect.DeepEqual(got, want) {
		t.Fatalf("mapping = %+v, want %+v", got, want)
	}
	if len(plan.Lint()) != 0 {
		t.Errorf("fully mapped stage with checks should not warn: %v", plan.Lint())
	}

	rendered := plan.RenderMarkdown()
	again, err := PlanFromMarkdown(rendered)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if again.RenderMarkdown() != rendered || !reflect.DeepEqual(again.Stages[0].AcceptanceMapping, want) {
		t.Fatalf("mapping did not round-trip:\n%s", rendered)
	}
}

func TestAcceptanceMappingValidationErrors(t *testing.T) {
	for name, m := range map[string][]AcceptanceMapping{
		"criterion out of range": {{Criterion: 3, Check: 1}},
		"malformed":              {ParseAcceptanceMapping("whatever")},
		"check out of range":     {{Criterion: 1, Check: 2}},
		"both":                   {{Criterion: 1, Check: 1, Manual: "x"}},
		"neither":                {{Criterion: 1}},
		"duplicate":              {{Criterion: 1, Check: 1}, {Criterion: 1, Manual: "x"}},
	} {
		plan := lintStageDoc(nil, []string{"go build ./..."})
		plan.Stages[0].AcceptanceCriteria = []string{"a", "b"}
		plan.Stages[0].AcceptanceMapping = m
		err := plan.Validate()
		if err == nil || !strings.Contains(err.Error(), "S001") {
			t.Errorf("%s: Validate = %v, want an error naming stage S001", name, err)
		}
	}
}

func TestDecodePlanDropsModelAcceptanceMapping(t *testing.T) {
	const planJSON = `{"project":"P","summary":"S","stages":[{"id":"S001","title":"T","objective":"o","dependencies":[],"deliverables":[],"acceptance_criteria":["x"],"acceptance_mapping":[{"criterion":1,"manual":"trust me"}]}]}`
	plan, err := decodePlan(planJSON, PlanContext{})
	if err != nil {
		t.Fatalf("decodePlan: %v", err)
	}
	if got := plan.Stages[0].AcceptanceMapping; got != nil {
		t.Errorf("model acceptance mapping survived decodePlan: %+v", got)
	}
}

func TestLegacyPlanWithoutMappingUnchanged(t *testing.T) {
	plan := lintStageDoc([]string{"a.go"}, []string{"go build ./..."})
	plan.Stages = append(plan.Stages, Stage{ID: "S002", Title: "Two", Objective: "o", AcceptanceCriteria: []string{"x", "y"}, AcceptanceMapping: []AcceptanceMapping{{Criterion: 2, Manual: "m"}}})
	if err := plan.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	data, _ := json.Marshal(plan.Stages[0])
	if strings.Contains(string(data), "acceptance_mapping") {
		t.Errorf("legacy stage JSON gained acceptance_mapping: %s", data)
	}
	if strings.Contains(lintStageDoc(nil, nil).RenderMarkdown(), "Acceptance Mapping") {
		t.Error("legacy stage rendered an Acceptance Mapping section")
	}

	var unmapped []string
	for _, w := range plan.Lint() {
		if strings.Contains(w.Message, "no acceptance mapping") {
			unmapped = append(unmapped, w.String())
		}
	}
	if len(unmapped) != 2 || !strings.Contains(unmapped[0], "S001") || !strings.Contains(unmapped[1], "S002") || !strings.Contains(unmapped[1], "[1]") {
		t.Errorf("unmapped warnings = %v, want S001 and S002 (criterion 1)", unmapped)
	}
}
