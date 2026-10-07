package planner

import (
	"strings"
	"testing"
)

// TestValidateCapabilitiesDuplicateDiagnosticIsDeterministic proves that repeated
// validation of the same plan whose capability names differ only by case and/or
// repeated whitespace produces the same first duplicate-pair diagnostic every time,
// even though the duplicate check no longer ranges over a map. Iterating the declared
// slice makes the diagnostic stable for identical input.
func TestValidateCapabilitiesDuplicateDiagnosticIsDeterministic(t *testing.T) {
	plan := func() *Plan {
		return &Plan{
			Project: "p",
			Summary: "s",
			Capabilities: []Capability{
				{Name: "External Tool", Status: CapabilityExists},
				{Name: "external   tool", Status: CapabilityExists},
				{Name: "EXTERNAL TOOL", Status: CapabilityExists},
			},
			Stages: []Stage{{
				ID: "S1", Title: "t", Objective: "o", AcceptanceCriteria: []string{"a"},
			}},
		}
	}

	first := ""
	for i := 0; i < 20; i++ {
		err := plan().Validate()
		if err == nil {
			t.Fatal("expected a duplicate capability diagnostic")
		}
		if !strings.Contains(err.Error(), "differ only by case or whitespace") {
			t.Fatalf("error = %q, want the duplicate case/whitespace diagnostic", err)
		}
		if i == 0 {
			first = err.Error()
			continue
		}
		if err.Error() != first {
			t.Fatalf("diagnostic %d differs:\n%q\nvs\n%q", i, first, err.Error())
		}
	}

	// The first duplicate pair is the first declared name and the one that
	// normalizes to it: deterministic across runs.
	if !strings.Contains(first, "\"External Tool\"") || !strings.Contains(first, "\"external   tool\"") {
		t.Errorf("diagnostic = %q, want the External Tool / external   tool pair", first)
	}
}

// TestCapabilityGapsNormalizesCaseVariantRequires proves the ownership gate matches a
// stage Require against a declared capability by normalized (case/repeated-whitespace)
// name, so an unresolved non-EXISTS capability cannot be bypassed with a spelling
// variant. Validate runs first and must accept the plan, so the gate is reached only
// through a structurally valid plan.
func TestCapabilityGapsNormalizesCaseVariantRequires(t *testing.T) {
	p := &Plan{
		Project: "p",
		Summary: "s",
		Capabilities: []Capability{
			{Name: "External Tool", Status: CapabilityMissing, Evidence: "none", Gap: "absent"},
		},
		Stages: []Stage{{
			ID: "S1", Title: "t", Objective: "o", AcceptanceCriteria: []string{"a"},
			Requires: []string{"external   tool"},
		}},
	}

	// Validate first: the case/repeated-whitespace variant is a recognized match, so
	// the plan is structurally valid and the ownership gate is the only thing left.
	if err := p.Validate(); err != nil {
		t.Fatalf("Validate() = %v, want nil for a case/whitespace variant of a declared capability", err)
	}

	handled, needsHuman := p.CapabilityGaps()
	if len(handled) != 0 {
		t.Errorf("handled = %+v, want none", handled)
	}
	if len(needsHuman) != 1 || needsHuman[0].Name != "External Tool" {
		t.Errorf("needsHuman = %+v, want [External Tool]", needsHuman)
	}
}

// TestCapabilityWordBoundariesRemainMeaningful proves normalization collapses
// whitespace but never removes it: "External Tool" and "ExternalTool" are distinct
// declared capabilities (one EXISTS, one UNKNOWN), a stage requiring the UNKNOWN
// exact name is rejected by Validate, and a stage requiring an undeclared
// "ExternalTool" does not resolve to only-declared "External Tool".
func TestCapabilityWordBoundariesRemainMeaningful(t *testing.T) {
	base := func(caps []Capability, requires []string) *Plan {
		return &Plan{
			Project:      "p",
			Summary:      "s",
			Capabilities: caps,
			Stages: []Stage{{
				ID: "S1", Title: "t", Objective: "o", AcceptanceCriteria: []string{"a"},
				Requires: requires,
			}},
		}
	}

	declared := []Capability{
		{Name: "External Tool", Status: CapabilityExists},
		{Name: "ExternalTool", Status: CapabilityUnknown},
	}

	cases := []struct {
		name     string
		plan     *Plan
		wantErr  bool
		wantFrag string
	}{
		{
			name: "distinct declared capabilities validate without ambiguity",
			plan: base(declared, []string{"External Tool"}),
		},
		{
			name:     "requiring the UNKNOWN exact name is rejected",
			plan:     base(declared, []string{"ExternalTool"}),
			wantErr:  true,
			wantFrag: "UNKNOWN",
		},
		{
			name:     "undeclared ExternalTool does not match only-declared External Tool",
			plan:     base([]Capability{{Name: "External Tool", Status: CapabilityExists}}, []string{"ExternalTool"}),
			wantErr:  true,
			wantFrag: "undeclared capability",
		},
		{
			name: "case/whitespace variant of the declared name still matches",
			plan: base(declared, []string{"external   tool"}),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.plan.Validate()
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Validate() = nil, want an error")
				}
				if tc.wantFrag != "" && !strings.Contains(err.Error(), tc.wantFrag) {
					t.Fatalf("error = %q, want it to contain %q", err, tc.wantFrag)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
		})
	}
}
